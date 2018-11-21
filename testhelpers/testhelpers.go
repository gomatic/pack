package testhelpers

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buildpack/pack/docker"
	"github.com/buildpack/pack/fs"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
)

func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}

// Assert deep equality (and provide useful difference as a test failure)
func AssertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}

// Assert the simplistic pointer (or literal value) equality
func AssertSameInstance(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if actual != expected {
		t.Fatalf("Expected %s and %s to be pointers to the variable", actual, expected)
	}
}

func AssertMatch(t *testing.T, actual string, expected *regexp.Regexp) {
	t.Helper()
	if !expected.Match([]byte(actual)) {
		t.Fatal(cmp.Diff(actual, expected))
	}
}

func AssertError(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatalf("Expected an error but got nil")
	}
	if actual.Error() != expected {
		t.Fatalf(`Expected error to equal "%s", got "%s"`, expected, actual.Error())
	}
}

func AssertContains(t *testing.T, actual, expected string) {
	t.Helper()
	if !strings.Contains(actual, expected) {
		t.Fatalf("Expected: '%s' inside '%s'", expected, actual)
	}
}

func AssertSliceContains(t *testing.T, slice []string, value string) {
	t.Helper()
	for _, s := range slice {
		if value == s {
			return
		}
	}
	t.Fatalf("Expected: '%s' inside '%s'", value, slice)
}

func AssertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}

func AssertNotNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual == nil {
		t.Fatal("Expected not nil")
	}
}

func AssertNotEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff == "" {
		t.Fatalf("Expected values to differ: %s", actual)
	}
}

func AssertDirContainsFileWithContents(t *testing.T, dir string, file string, expected string) {
	t.Helper()
	path := filepath.Join(dir, file)
	bytes, err := ioutil.ReadFile(path)
	AssertNil(t, err)
	if string(bytes) != expected {
		t.Fatalf("file %s in dir %s has wrong contents: %s != %s", file, dir, string(bytes), expected)
	}
}

var dockerCliVal *docker.Client
var dockerCliOnce sync.Once
var dockerCliErr error

func dockerCli(t *testing.T) *docker.Client {
	dockerCliOnce.Do(func() {
		dockerCliVal, dockerCliErr = docker.New()
	})
	AssertNil(t, dockerCliErr)
	return dockerCliVal
}

func proxyDockerHostPort(dockerCli *docker.Client, port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	go func() {
		// TODO exit somehow.
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var stderr bytes.Buffer

				ctx := context.Background()
				err := dockerCli.PullImage("alpine/socat")
				if err != nil {
					fmt.Println("ERR: proxyDockerHostPort: PULL:", err)
					return
				}
				ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
					Image:        "alpine/socat",
					Cmd:          []string{"-", "TCP:localhost:" + port},
					OpenStdin:    true,
					StdinOnce:    true,
					AttachStdin:  true,
					AttachStdout: true,
					AttachStderr: true,
				}, &container.HostConfig{
					AutoRemove:  true,
					NetworkMode: "host",
				}, nil, "")
				if err != nil {
					fmt.Println("ERR: proxyDockerHostPort: CREATE:", err)
					return
				}

				res, err := dockerCli.ContainerAttach(ctx, ctr.ID, dockertypes.ContainerAttachOptions{
					Stream: true,
					Stdin:  true,
					Stdout: true,
					Stderr: true,
					Logs:   true,
				})
				if err != nil {
					fmt.Println("ERR: proxyDockerHostPort: ATTACH:", err)
					return
				}
				defer res.Close()

				bodyChan, errChan := dockerCli.ContainerWait(ctx, ctr.ID, container.WaitConditionNextExit)
				dockerCli.ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
				if err != nil {
					fmt.Println("ERR: proxyDockerHostPort: START:", err)
					return
				}

				go func() { stdcopy.StdCopy(conn, &stderr, res.Reader) }()
				go func() { io.Copy(res.Conn, conn) }()

				select {
				case body := <-bodyChan:
					if body.StatusCode != 0 {
						fmt.Println("ERR: proxyDockerHostPort: failed with status code:", body.StatusCode)
						fmt.Println("ERR: proxyDockerHostPort: STDERR:", stderr.String())
					}
				case err := <-errChan:
					fmt.Println("ERR: proxyDockerHostPort:", err)
					fmt.Println("ERR: proxyDockerHostPort: STDERR:", stderr.String())
				}
			}(conn)
		}
	}()
	return nil
}

var runRegistryName, runRegistryPort string
var runRegistryOnce sync.Once

func RunRegistry(t *testing.T, seedRegistry bool) (localPort string) {
	t.Log("run registry")
	t.Helper()
	runRegistryOnce.Do(func() {
		runRegistryName = "test-registry-" + RandString(10)

		AssertNil(t, dockerCli(t).PullImage("registry:2"))
		ctx := context.Background()
		ctr, err := dockerCli(t).ContainerCreate(ctx, &container.Config{
			Image: "registry:2",
		}, &container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				"5000/tcp": []nat.PortBinding{{}},
			},
		}, nil, runRegistryName)
		AssertNil(t, err)
		defer dockerCli(t).ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
		err = dockerCli(t).ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
		AssertNil(t, err)

		inspect, err := dockerCli(t).ContainerInspect(context.TODO(), ctr.ID)
		AssertNil(t, err)
		runRegistryPort = inspect.NetworkSettings.Ports["5000/tcp"][0].HostPort

		if os.Getenv("DOCKER_HOST") != "" {
			err := proxyDockerHostPort(dockerCli(t), runRegistryPort)
			AssertNil(t, err)
		}

		Eventually(t, func() bool {
			txt, err := HttpGetE(dockerCli(t), fmt.Sprintf("http://localhost:%s/v2/", runRegistryPort))
			return err == nil && txt != ""
		}, 10*time.Millisecond, 2*time.Second)

		if seedRegistry {
			t.Log("seed registry")
			for _, f := range []func(*testing.T, string) string{DefaultBuildImage, DefaultRunImage, DefaultBuilderImage} {
				AssertNil(t, pushImage(dockerCli(t), f(t, runRegistryPort)))
			}
		}
	})
	return runRegistryPort
}

func Eventually(t *testing.T, test func() bool, every time.Duration, timeout time.Duration) {
	t.Helper()

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if test() {
				return
			}
		case <-timer.C:
			t.Fatalf("timeout on eventually: %v", timeout)
		}
	}
}

func ConfigurePackHome(t *testing.T, packHome, registryPort string) {
	t.Helper()
	AssertNil(t, ioutil.WriteFile(filepath.Join(packHome, "config.toml"), []byte(fmt.Sprintf(`
				default-stack-id = "io.buildpacks.stacks.bionic"
                default-builder = "%s"

				[[stacks]]
				  id = "io.buildpacks.stacks.bionic"
				  build-images = ["%s"]
				  run-images = ["%s"]
			`, DefaultBuilderImage(t, registryPort), DefaultBuildImage(t, registryPort), DefaultRunImage(t, registryPort))), 0666))
}

func StopRegistry(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	if runRegistryName != "" {
		dockerCli(t).ContainerKill(context.Background(), runRegistryName, "SIGKILL")
		dockerCli(t).ContainerRemove(context.TODO(), runRegistryName, dockertypes.ContainerRemoveOptions{Force: true})
	}
}

var getBuildImageOnce sync.Once

func DefaultBuildImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuildImageOnce.Do(func() {
		if tag == "latest" {
			AssertNil(t, dockerCli(t).PullImage(fmt.Sprintf("packs/build:%s", tag)))
		}
		AssertNil(t, dockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/build:%s", tag),
			fmt.Sprintf("localhost:%s/packs/build:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/build:%s", registryPort, tag)
}

var getRunImageOnce sync.Once

func DefaultRunImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getRunImageOnce.Do(func() {
		if tag == "latest" {
			AssertNil(t, dockerCli(t).PullImage(fmt.Sprintf("packs/run:%s", tag)))
		}
		AssertNil(t, dockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/run:%s", tag),
			fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/run:%s", registryPort, tag)
}

var getBuilderImageOnce sync.Once

func DefaultBuilderImage(t *testing.T, registryPort string) string {
	t.Helper()
	tag := packTag()
	getBuilderImageOnce.Do(func() {
		if tag == "latest" {
			AssertNil(t, dockerCli(t).PullImage(fmt.Sprintf("packs/samples:%s", tag)))
		}
		AssertNil(t, dockerCli(t).ImageTag(
			context.Background(),
			fmt.Sprintf("packs/samples:%s", tag),
			fmt.Sprintf("localhost:%s/packs/samples:%s", registryPort, tag),
		))
	})
	return fmt.Sprintf("localhost:%s/packs/samples:%s", registryPort, tag)
}

func CreateImageOnLocal(t *testing.T, dockerCli *docker.Client, repoName, dockerFile string) {
	ctx := context.Background()

	buildContext, err := (&fs.FS{}).CreateSingleFileTar("Dockerfile", dockerFile)
	AssertNil(t, err)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:           []string{repoName},
		SuppressOutput: true,
		Remove:         true,
		ForceRemove:    true,
	})
	AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}

func CreateImageOnRemote(t *testing.T, dockerCli *docker.Client, repoName, dockerFile string) string {
	t.Helper()
	defer DockerRmi(dockerCli, repoName)

	CreateImageOnLocal(t, dockerCli, repoName+":latest", dockerFile)

	var topLayer string
	inspect, _, err := dockerCli.ImageInspectWithRaw(context.TODO(), repoName+":latest")
	AssertNil(t, err)
	if len(inspect.RootFS.Layers) > 0 {
		topLayer = inspect.RootFS.Layers[len(inspect.RootFS.Layers)-1]
	} else {
		topLayer = "N/A"
	}

	AssertNil(t, pushImage(dockerCli, repoName))

	return topLayer
}

func DockerRmi(dockerCli *docker.Client, repoNames ...string) error {
	var err error
	ctx := context.Background()
	for _, name := range repoNames {
		_, e := dockerCli.ImageRemove(ctx, name, dockertypes.ImageRemoveOptions{Force: true, PruneChildren: true})
		if e != nil && err == nil {
			err = e
		}
	}
	return err
}

func CopySingleFileFromContainer(dockerCli *docker.Client, ctrID, path string) (string, error) {
	r, _, err := dockerCli.CopyFromContainer(context.Background(), ctrID, path)
	if err != nil {
		return "", err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if hdr.Name != path && hdr.Name != filepath.Base(path) {
		return "", fmt.Errorf("filenames did not match: %s and %s (%s)", hdr.Name, path, filepath.Base(path))
	}
	b, err := ioutil.ReadAll(tr)
	return string(b), err
}

func CopySingleFileFromImage(dockerCli *docker.Client, repoName, path string) (string, error) {
	ctr, err := dockerCli.ContainerCreate(context.Background(),
		&container.Config{
			Image: repoName,
		}, &container.HostConfig{
			AutoRemove: true,
		}, nil, "",
	)
	if err != nil {
		return "", err
	}
	defer dockerCli.ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
	return CopySingleFileFromContainer(dockerCli, ctr.ID, path)
}

func pushImage(dockerCli *docker.Client, ref string) error {
	rc, err := dockerCli.ImagePush(context.Background(), ref, dockertypes.ImagePushOptions{RegistryAuth: "{}"})
	if err != nil {
		return err
	}
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return err
	}
	return rc.Close()
}

func packTag() string {
	tag := os.Getenv("PACK_TAG")
	if tag == "" {
		return "latest"
	}
	return tag
}

func HttpGet(t *testing.T, url string) string {
	t.Helper()
	txt, err := HttpGetE(dockerCli(t), url)
	AssertNil(t, err)
	return txt
}

var pullPacksSamplesOnce sync.Once

func pullPacksSamples(d *docker.Client) {
	pullPacksSamplesOnce.Do(func() {
		d.PullImage("packs/samples")
	})
}

func HttpGetE(dockerCli *docker.Client, url string) (string, error) {
	if os.Getenv("DOCKER_HOST") == "" {
		resp, err := http.Get(url)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return "", fmt.Errorf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
		}
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(b), nil
	} else {
		var stderr bytes.Buffer
		ctx := context.Background()
		pullPacksSamples(dockerCli)
		ctr, err := dockerCli.ContainerCreate(ctx, &container.Config{
			Image:        "packs/samples",
			Cmd:          []string{"wget", "-q", "-O", "-", url},
			Entrypoint:   nil,
			AttachStdout: true,
			AttachStderr: true,
		}, &container.HostConfig{
			AutoRemove:  true,
			NetworkMode: "host",
		}, nil, "")
		if err != nil {
			return "", err
		}

		var buf bytes.Buffer
		err = dockerCli.RunContainer(ctx, ctr.ID, &buf, &stderr)
		if err != nil {
			return "", fmt.Errorf("Expected nil: %s", errors.Wrap(err, buf.String()))
		}
		if txt := strings.TrimSpace(stderr.String()); txt != "" {
			return "", fmt.Errorf("unexpected stderr: %s", txt)
		}

		return buf.String(), nil
	}
}

func CopyWorkspaceToDocker(t *testing.T, srcPath, destVolume string) {
	t.Helper()

	ctx := context.Background()
	pullPacksSamples(dockerCli(t))
	ctr, err := dockerCli(t).ContainerCreate(ctx, &container.Config{
		Image: "packs/samples",
		Cmd:   []string{"true"},
	}, &container.HostConfig{
		AutoRemove: true,
		Binds:      []string{destVolume + ":/workspace"},
	}, nil, "")
	AssertNil(t, err)
	defer dockerCli(t).ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})

	tr, errChan := (&fs.FS{}).CreateTarReader(srcPath, "/workspace", 1000, 1000)
	err = dockerCli(t).CopyToContainer(ctx, ctr.ID, "/", tr, dockertypes.CopyToContainerOptions{})
	AssertNil(t, err)
	AssertNil(t, <-errChan)
}

func ReadFromDocker(t *testing.T, volume, path string) string {
	t.Helper()
	pullPacksSamples(dockerCli(t))
	ctr, err := dockerCli(t).ContainerCreate(
		context.Background(),
		&container.Config{Image: "packs/samples"},
		&container.HostConfig{
			AutoRemove: true,
			Binds:      []string{volume + ":/workspace"},
		},
		nil, "",
	)
	AssertNil(t, err)
	defer dockerCli(t).ContainerRemove(context.Background(), ctr.ID, dockertypes.ContainerRemoveOptions{})
	txt, err := CopySingleFileFromContainer(dockerCli(t), ctr.ID, path)
	AssertNil(t, err)
	return txt
}

func ImageID(t *testing.T, repoName string) string {
	t.Helper()
	inspect, _, err := dockerCli(t).ImageInspectWithRaw(context.Background(), repoName)
	AssertNil(t, err)
	return inspect.ID
}

func Run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	txt, err := RunE(cmd)
	AssertNil(t, err)
	return txt
}

func CleanDefaultImages(t *testing.T, registryPort string) {
	t.Helper()
	AssertNil(t,
		DockerRmi(
			dockerCli(t),
			DefaultRunImage(t, registryPort),
			DefaultBuildImage(t, registryPort),
			DefaultBuilderImage(t, registryPort),
		),
	)
}

func RunE(cmd *exec.Cmd) (string, error) {
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Failed to execute command: %v, %s, %s, %s", cmd.Args, err, stderr.String(), output)
	}

	return string(output), nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func trimEmpty(slice []string) []string {
	out := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
