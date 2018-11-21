package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/image"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var registryPort string

func TestBuild(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	registryPort = h.RunRegistry(t, true)
	defer h.StopRegistry(t)
	packHome, err := ioutil.TempDir("", "build-test-pack-home")
	h.AssertNil(t, err)
	defer os.RemoveAll(packHome)
	h.ConfigurePackHome(t, packHome, registryPort)
	defer h.CleanDefaultImages(t, registryPort)

	spec.Run(t, "build", testBuild, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var subject *pack.BuildConfig
	var buf bytes.Buffer
	var dockerCli *docker.Client

	it.Before(func() {
		var err error
		subject = &pack.BuildConfig{
			AppDir:          "acceptance/testdata/node_app",
			Builder:         h.DefaultBuilderImage(t, registryPort),
			RunImage:        h.DefaultRunImage(t, registryPort),
			RepoName:        "pack.build." + h.RandString(10),
			Publish:         false,
			WorkspaceVolume: fmt.Sprintf("pack-workspace-%x", uuid.New().String()),
			CacheVolume:     fmt.Sprintf("pack-cache-%x", uuid.New().String()),
			Stdout:          &buf,
			Stderr:          &buf,
			Log:             log.New(&buf, "", log.LstdFlags|log.Lshortfile),
			FS:              &fs.FS{},
			Images:          &image.Client{},
		}
		log.SetOutput(ioutil.Discard)
		dockerCli, err = docker.New()
		subject.Cli = dockerCli
		h.AssertNil(t, err)
	})
	it.After(func() {
		for _, volName := range []string{subject.WorkspaceVolume, subject.CacheVolume} {
			h.AssertNil(t, dockerCli.VolumeRemove(context.TODO(), volName, true))
		}
	})

	when("#BuildConfigFromFlags", func() {
		var (
			factory        *pack.BuildFactory
			mockController *gomock.Controller
			mockImages     *mocks.MockImages
			mockDocker     *mocks.MockDocker
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockImages = mocks.NewMockImages(mockController)
			mockDocker = mocks.NewMockDocker(mockController)

			factory = &pack.BuildFactory{
				Images: mockImages,
				Config: &config.Config{
					DefaultBuilder: "some/builder",
					Stacks: []config.Stack{
						{
							ID:        "some.stack.id",
							RunImages: []string{"some/run", "registry.com/some/run"},
						},
					},
				},
				Cli: mockDocker,
				Log: log.New(&buf, "", log.LstdFlags|log.Lshortfile),
			}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("defaults to daemon, default-builder, pulls builder and run images, selects run-image using builder's stack", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
		})

		it("respects builder from flags", func() {
			mockDocker.EXPECT().PullImage("custom/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "custom/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "custom/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
		})

		it("selects run images with matching registry", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("registry.com/some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "registry.com/some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "registry.com/some/app",
				Builder:  "some/builder",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "registry.com/some/run")
		})

		it("doesn't pull run images when --publish is passed", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("some/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "some.stack.id",
					},
				},
			}, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "some/run")
		})

		it("allows run-image from flags if the stacks match", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("override/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "some.stack.id",
					},
				},
			}, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
		})

		it("doesn't allows run-image from flags if the stacks are difference", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("override/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "other.stack.id",
					},
				},
			}, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
			})
			h.AssertError(t, err, `invalid stack: stack "other.stack.id" from run image "override/run" does not match stack "some.stack.id" from builder image "some/builder"`)
		})

		it("uses working dir if appDir is set to placeholder value", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockRunImage := mocks.NewMockV1Image(mockController)
			mockImages.EXPECT().ReadImage("override/run", false).Return(mockRunImage, nil)
			mockRunImage.EXPECT().ConfigFile().Return(&v1.ConfigFile{
				Config: v1.Config{
					Labels: map[string]string{
						"io.buildpacks.stack.id": "some.stack.id",
					},
				},
			}, nil)

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				RunImage: "override/run",
				Publish:  true,
				AppDir:   "current working directory",
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.RunImage, "override/run")
			h.AssertEq(t, config.AppDir, os.Getenv("PWD"))
		})

		it("returns an errors when the builder stack label is missing", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{},
				},
			}, nil, nil)

			_, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
			})
			h.AssertError(t, err, `invalid builder image "some/builder": missing required label "io.buildpacks.stack.id"`)
		})

		it("sets EnvFile", func() {
			mockDocker.EXPECT().PullImage("some/builder")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/builder").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)
			mockDocker.EXPECT().PullImage("some/run")
			mockDocker.EXPECT().ImageInspectWithRaw(gomock.Any(), "some/run").Return(dockertypes.ImageInspect{
				Config: &dockercontainer.Config{
					Labels: map[string]string{"io.buildpacks.stack.id": "some.stack.id"},
				},
			}, nil, nil)

			envFile, err := ioutil.TempFile("", "pack.build.enfile")
			h.AssertNil(t, err)
			defer os.Remove(envFile.Name())

			_, err = envFile.Write([]byte(`
VAR1=value1
VAR2=value2 with spaces	
USER
				`))
			h.AssertNil(t, err)
			envFile.Close()

			config, err := factory.BuildConfigFromFlags(&pack.BuildFlags{
				RepoName: "some/app",
				Builder:  "some/builder",
				EnvFile:  envFile.Name(),
			})
			h.AssertNil(t, err)
			h.AssertEq(t, config.EnvFile, map[string]string{
				"VAR1": "value1",
				"VAR2": "value2 with spaces",
				"USER": os.Getenv("USER"),
			})
			h.AssertNotEq(t, os.Getenv("USER"), "")
		})
	})

	when("#Detect", func() {
		it("copies the app in to docker and chowns it (including directories)", func() {
			_, err := subject.Detect()
			h.AssertNil(t, err)

			for _, name := range []string{"/workspace/app", "/workspace/app/app.js", "/workspace/app/mydir", "/workspace/app/mydir/myfile.txt"} {
				txt := runInImage(t, dockerCli, []string{subject.WorkspaceVolume + ":/workspace"}, subject.Builder, "ls", "-ld", name)
				h.AssertContains(t, txt, "pack pack")
			}
		})

		when("app is detected", func() {
			it("returns the successful group with node", func() {
				group, err := subject.Detect()
				h.AssertNil(t, err)
				h.AssertEq(t, group.Buildpacks[0].ID, "io.buildpacks.samples.nodejs")
			})
		})

		when("app is not detectable", func() {
			var badappDir string
			it.Before(func() {
				var err error
				badappDir, err = ioutil.TempDir("/tmp", "pack.build.badapp.")
				h.AssertNil(t, err)
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(badappDir, "file.txt"), []byte("content"), 0644))
				subject.AppDir = badappDir
			})
			it.After(func() { os.RemoveAll(badappDir) })
			it("returns the successful group with node", func() {
				_, err := subject.Detect()

				h.AssertNotNil(t, err)
				h.AssertEq(t, err.Error(), "run detect container: failed with status code: 6")
			})
		})

		when("buildpacks are specified", func() {
			when("directory buildpack", func() {
				var bpDir string
				it.Before(func() {
					var err error
					bpDir, err = ioutil.TempDir("/tmp", "pack.build.bpdir.")
					h.AssertNil(t, err)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "buildpack.toml"), []byte(`
					[buildpack]
					id = "com.example.mybuildpack"
					version = "1.2.3"
					name = "My Sample Buildpack"

					[[stacks]]
					id = "io.buildpacks.stacks.bionic"
					`), 0666))
					h.AssertNil(t, os.MkdirAll(filepath.Join(bpDir, "bin"), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "detect"), []byte(`#!/usr/bin/env bash
					exit 0
					`), 0777))
				})
				it.After(func() { os.RemoveAll(bpDir) })

				it("copies directories to workspace and sets order.toml", func() {
					subject.Buildpacks = []string{
						bpDir,
					}

					_, err := subject.Detect()
					h.AssertNil(t, err)

					h.AssertMatch(t, buf.String(), regexp.MustCompile(`DETECTING WITH MANUALLY-PROVIDED GROUP:\n[0-9\s:\/]* Group: My Sample Buildpack: pass\n`))
				})
			})
			when("id@version buildpack", func() {
				it("symlinks directories to workspace and sets order.toml", func() {
					subject.Buildpacks = []string{
						"io.buildpacks.samples.nodejs@latest",
					}

					_, err := subject.Detect()
					h.AssertNil(t, err)

					h.AssertMatch(t, buf.String(), regexp.MustCompile(`DETECTING WITH MANUALLY-PROVIDED GROUP:\n[0-9\s:\/]* Group: Sample Node.js Buildpack: pass\n`))
				})
			})
		})
	})

	when("#Analyze", func() {
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("/tmp", "pack.build.analyze.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, "group.toml"), []byte(`[[buildpacks]]
			  id = "io.buildpacks.samples.nodejs"
				version = "0.0.1"
			`), 0666))

			h.CopyWorkspaceToDocker(t, tmpDir, subject.WorkspaceVolume)
		})
		when("no previous image exists", func() {
			when("publish", func() {
				it.Before(func() {
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true
				})

				it("informs the user", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)
					h.AssertContains(t, buf.String(), "WARNING: skipping analyze, image not found or requires authentication to access")
				})
			})
			when("daemon", func() {
				it.Before(func() { subject.Publish = false })
				it("informs the user", func() {
					err := subject.Analyze()
					h.AssertNil(t, err)
					h.AssertContains(t, buf.String(), "WARNING: skipping analyze, image not found\n")
				})
			})
		})

		when("previous image exists", func() {
			var dockerFile string
			it.Before(func() {
				dockerFile = fmt.Sprintf(`
					FROM scratch
					LABEL io.buildpacks.lifecycle.metadata='{"buildpacks":[{"key":"io.buildpacks.samples.nodejs","layers":{"node_modules":{"sha":"sha256:99311ec03d790adf46d35cd9219ed80a7d9a4b97f761247c02c77e7158a041d5","data":{"lock_checksum":"eb04ed1b461f1812f0f4233ef997cdb5"}}}}]}'
					LABEL repo_name_for_randomisation=%s
				`, subject.RepoName)
			})

			when("publish", func() {
				it.Before(func() {
					subject.RepoName = "localhost:" + registryPort + "/" + subject.RepoName
					subject.Publish = true

					h.CreateImageOnRemote(t, dockerCli, subject.RepoName, dockerFile)
				})

				it("tells the user nothing", func() {
					h.AssertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					h.AssertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					h.AssertNil(t, subject.Analyze())

					txt := h.ReadFromDocker(t, subject.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")

					h.AssertEq(t, txt, "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})

			when("daemon", func() {
				it.Before(func() {
					subject.Publish = false

					h.CreateImageOnLocal(t, dockerCli, subject.RepoName, dockerFile)
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
				})

				it("tells the user nothing", func() {
					h.AssertNil(t, subject.Analyze())

					txt := string(bytes.Trim(buf.Bytes(), "\x00"))
					h.AssertEq(t, txt, "")
				})

				it("places files in workspace", func() {
					h.AssertNil(t, subject.Analyze())

					txt := h.ReadFromDocker(t, subject.WorkspaceVolume, "/workspace/io.buildpacks.samples.nodejs/node_modules.toml")
					h.AssertEq(t, txt, "lock_checksum = \"eb04ed1b461f1812f0f4233ef997cdb5\"\n")
				})
			})
		})
	})

	when("#Build", func() {
		when("buildpacks are specified", func() {
			when("directory buildpack", func() {
				var bpDir string
				it.Before(func() {
					var err error
					bpDir, err = ioutil.TempDir("/tmp", "pack.build.bpdir.")
					h.AssertNil(t, err)
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "buildpack.toml"), []byte(`
					[buildpack]
					id = "com.example.mybuildpack"
					version = "1.2.3"
					name = "My Sample Buildpack"

					[[stacks]]
					id = "io.buildpacks.stacks.bionic"
					`), 0666))
					h.AssertNil(t, os.MkdirAll(filepath.Join(bpDir, "bin"), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "detect"), []byte(`#!/usr/bin/env bash
					exit 0
					`), 0777))
					h.AssertNil(t, ioutil.WriteFile(filepath.Join(bpDir, "bin", "build"), []byte(`#!/usr/bin/env bash
					echo "BUILD OUTPUT FROM MY SAMPLE BUILDPACK"
					exit 0
					`), 0777))
				})
				it.After(func() { os.RemoveAll(bpDir) })

				it("runs the buildpacks bin/build", func() {
					subject.Buildpacks = []string{bpDir}
					_, err := subject.Detect()
					h.AssertNil(t, err)

					err = subject.Build()
					h.AssertNil(t, err)

					h.AssertContains(t, buf.String(), "BUILD OUTPUT FROM MY SAMPLE BUILDPACK")
				})
			})
			when("id@version buildpack", func() {
				it("runs the buildpacks bin/build", func() {
					subject.Buildpacks = []string{"io.buildpacks.samples.nodejs@latest"}
					_, err := subject.Detect()
					h.AssertNil(t, err)

					err = subject.Build()
					h.AssertNil(t, err)

					h.AssertContains(t, buf.String(), "npm notice created a lockfile as package-lock.json. You should commit this file.")
				})
			})
		})

		when("EnvFile is specified", func() {
			it("sets specified env variables in /platform/env/...", func() {
				subject.EnvFile = map[string]string{
					"VAR1": "value1",
					"VAR2": "value2 with spaces",
				}
				subject.Buildpacks = []string{"acceptance/testdata/mock_buildpacks/printenv"}
				_, err := subject.Detect()
				h.AssertNil(t, err)

				err = subject.Build()
				h.AssertNil(t, err)

				h.AssertContains(t, buf.String(), "ENV: VAR1 is value1;")
				h.AssertContains(t, buf.String(), "ENV: VAR2 is value2 with spaces;")
			})
		})
	})

	when("#Export", func() {
		var (
			group       *lifecycle.BuildpackGroup
			runSHA      string
			runTopLayer string
		)
		it.Before(func() {
			tmpDir, err := ioutil.TempDir("/tmp", "pack.build.export.")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)
			files := map[string]string{
				"group.toml":           "[[buildpacks]]\n" + `id = "io.buildpacks.samples.nodejs"` + "\n" + `version = "0.0.1"`,
				"app/file.txt":         "some text",
				"config/metadata.toml": "stuff = \"text\"",
				"io.buildpacks.samples.nodejs/mylayer.toml":     `key = "myval"`,
				"io.buildpacks.samples.nodejs/mylayer/file.txt": "content",
				"io.buildpacks.samples.nodejs/other.toml":       "",
				"io.buildpacks.samples.nodejs/other/file.txt":   "something",
			}
			for name, txt := range files {
				h.AssertNil(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, name)), 0777))
				h.AssertNil(t, ioutil.WriteFile(filepath.Join(tmpDir, name), []byte(txt), 0666))
			}
			h.CopyWorkspaceToDocker(t, tmpDir, subject.WorkspaceVolume)

			group = &lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "io.buildpacks.samples.nodejs", Version: "0.0.1"},
				},
			}
			runSHA = imageSHA(t, dockerCli, subject.RunImage)
			runTopLayer = topLayer(t, dockerCli, subject.RunImage)
		})

		when("publish", func() {
			var oldRepoName string
			it.Before(func() {
				oldRepoName = subject.RepoName

				subject.RepoName = "localhost:" + registryPort + "/" + oldRepoName
				subject.Publish = true
			})

			it("creates the image on the registry", func() {
				h.AssertNil(t, subject.Export(group))
				images := h.HttpGet(t, "http://localhost:"+registryPort+"/v2/_catalog")
				h.AssertContains(t, images, oldRepoName)
			})

			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export(group))

				h.AssertNil(t, dockerCli.PullImage(subject.RepoName))
				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})

			it("sets the metadata on the image", func() {
				h.AssertNil(t, subject.Export(group))

				h.AssertNil(t, dockerCli.PullImage(subject.RepoName))
				var metadata lifecycle.AppImageMetadata
				metadataJSON := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, json.Unmarshal([]byte(metadataJSON), &metadata))

				h.AssertEq(t, metadata.RunImage.SHA, runSHA)
				h.AssertEq(t, metadata.RunImage.TopLayer, runTopLayer)
				h.AssertContains(t, metadata.App.SHA, "sha256:")
				h.AssertContains(t, metadata.Config.SHA, "sha256:")
				h.AssertEq(t, len(metadata.Buildpacks), 1)
				h.AssertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
				h.AssertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
			})
		})

		when("daemon", func() {
			it.Before(func() { subject.Publish = false })

			it.After(func() {
				h.AssertNil(t, h.DockerRmi(dockerCli, subject.RepoName))
			})

			it("creates the image on the daemon", func() {
				h.AssertNil(t, subject.Export(group))
				images := imageList(t, dockerCli)
				h.AssertSliceContains(t, images, subject.RepoName+":latest")
			})
			it("puts the files on the image", func() {
				h.AssertNil(t, subject.Export(group))

				txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/app/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "some text")

				txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
				h.AssertNil(t, err)
				h.AssertEq(t, string(txt), "content")
			})
			it("sets the metadata on the image", func() {
				h.AssertNil(t, subject.Export(group))

				var metadata lifecycle.AppImageMetadata
				metadataJSON := imageLabel(t, dockerCli, subject.RepoName, "io.buildpacks.lifecycle.metadata")
				h.AssertNil(t, json.Unmarshal([]byte(metadataJSON), &metadata))

				h.AssertEq(t, metadata.RunImage.SHA, runSHA)
				h.AssertEq(t, metadata.RunImage.TopLayer, runTopLayer)
				h.AssertContains(t, metadata.App.SHA, "sha256:")
				h.AssertContains(t, metadata.Config.SHA, "sha256:")
				h.AssertEq(t, len(metadata.Buildpacks), 1)
				h.AssertContains(t, metadata.Buildpacks[0].Layers["mylayer"].SHA, "sha256:")
				h.AssertEq(t, metadata.Buildpacks[0].Layers["mylayer"].Data, map[string]interface{}{"key": "myval"})
				h.AssertContains(t, metadata.Buildpacks[0].Layers["other"].SHA, "sha256:")
			})

			when("PACK_USER_ID and PACK_GROUP_ID are set on builder", func() {
				it.Before(func() {
					subject.Builder = "packs/samples-" + h.RandString(8)
					h.CreateImageOnLocal(t, dockerCli, subject.Builder, fmt.Sprintf(`
						FROM %s
						ENV PACK_USER_ID 1234
						ENV PACK_GROUP_ID 5678
						LABEL repo_name_for_randomisation=%s
					`, h.DefaultBuilderImage(t, registryPort), subject.Builder))
				})

				it.After(func() {
					h.AssertNil(t, h.DockerRmi(dockerCli, subject.Builder))
				})

				it("sets owner of layer files to PACK_USER_ID:PACK_GROUP_ID", func() {
					h.AssertNil(t, subject.Export(group))
					txt := runInImage(t, dockerCli, nil, subject.RepoName, "ls", "-la", "/workspace/app/file.txt")
					h.AssertContains(t, txt, " 1234 5678 ")
				})
			})

			when("previous image exists", func() {
				it("reuses images from previous layers", func() {
					t.Log("create image and h.Assert add new layer")
					h.AssertNil(t, subject.Export(group))

					origImageID := h.ImageID(t, subject.RepoName)
					defer func() { h.AssertNil(t, h.DockerRmi(dockerCli, origImageID)) }()

					txt, err := h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")

					t.Log("setup workspace to reuse layer")
					buf.Reset()
					runInImage(t, dockerCli,
						[]string{subject.WorkspaceVolume + ":/workspace"},
						h.DefaultBuilderImage(t, registryPort),
						"rm", "-rf", "/workspace/io.buildpacks.samples.nodejs/mylayer",
					)

					t.Log("recreate image and h.Assert copying layer from previous image")
					h.AssertNil(t, subject.Export(group))
					txt, err = h.CopySingleFileFromImage(dockerCli, subject.RepoName, "workspace/io.buildpacks.samples.nodejs/mylayer/file.txt")
					h.AssertNil(t, err)
					h.AssertEq(t, txt, "content")
				})
			})
		})
	})
}

func imageSHA(t *testing.T, dockerCli *docker.Client, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	sha := strings.Split(inspect.RepoDigests[0], "@")[1]
	return sha
}

func topLayer(t *testing.T, dockerCli *docker.Client, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	layers := inspect.RootFS.Layers
	return layers[len(layers)-1]
}

func imageLabel(t *testing.T, dockerCli *docker.Client, repoName, labelName string) string {
	t.Helper()
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.Background(), repoName)
	h.AssertNil(t, err)
	return inspect.Config.Labels[labelName]
}

func imageList(t *testing.T, dockerCli *docker.Client) []string {
	t.Helper()
	var out []string
	list, err := dockerCli.ImageList(context.Background(), dockertypes.ImageListOptions{})
	h.AssertNil(t, err)
	for _, s := range list {
		out = append(out, s.RepoTags...)
	}
	return out
}

func runInImage(t *testing.T, dockerCli *docker.Client, volumes []string, repoName string, args ...string) string {
	t.Helper()
	ctx := context.Background()

	ctr, err := dockerCli.ContainerCreate(ctx, &dockercontainer.Config{
		Image: repoName,
		Cmd:   args,
		User:  "root",
	}, &dockercontainer.HostConfig{
		Binds: volumes,
	}, nil, "")
	h.AssertNil(t, err)
	defer dockerCli.ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	var buf bytes.Buffer
	err = dockerCli.RunContainer(ctx, ctr.ID, &buf, &buf)
	if err != nil {
		t.Fatalf("Expected nil: %s", errors.Wrap(err, buf.String()))
	}

	return buf.String()
}
