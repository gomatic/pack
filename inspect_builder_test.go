package pack_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/buildpack/pack"
	"github.com/buildpack/pack/fs"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectBuilder(t *testing.T) {
	spec.Run(t, "create-builder", testInspectBuilder, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testInspectBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller
		mockImage      *mocks.MockImage
		mockDocker     *mocks.MockDocker
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImage = mocks.NewMockImage(mockController)
		mockDocker = mocks.NewMockDocker(mockController)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#Inspect", func() {
		it("prints the builder name", func() {
			order := `
[[groups]]
  [[groups.buildpacks]]
    id = "mock.bp.first"
    version = "0.0.1-mock"

  [[groups.buildpacks]]
    id = "mock.bp.second"
    version = "0.0.2-mock"

  [[groups.buildpacks]]
    id = "mock.bp.third"
    version = "0.0.3-mock"
`
			fs := fs.FS{}
			orderTarReader, err := fs.CreateSingleFileTar("/buildpacks/order.toml", order)
			h.AssertNil(t, err)
			orderReadCloser := ioutils.NewReadCloserWrapper(orderTarReader, func() error { return nil })

			buildpacksTarReader, _ := fs.CreateTarReader(
				filepath.Join("testdata", "buildpacks-dir"),
				"/buildpacks",
				os.Getuid(),
				os.Getegid(),
			)

			buildpacksReadCloser := ioutils.NewReadCloserWrapper(buildpacksTarReader, func() error { return nil })

			mockImage.EXPECT().Name().Return("some-builder").AnyTimes()
			mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("io.buildpacks.stacks.bionic", nil)
			mockDocker.EXPECT().
				ContainerCreate(gomock.Any(), gomock.Any(), gomock.Any(), nil, "").
				Do(func(_ context.Context, config *container.Config, _ *container.HostConfig, _ *network.NetworkingConfig, _ string) {
					h.AssertEq(t, config.Image, "some-builder")
				}).Return(container.ContainerCreateCreatedBody{ID: "some-container-id"}, nil)
			mockDocker.EXPECT().
				CopyFromContainer(gomock.Any(), "some-container-id", "/buildpacks/order.toml").
				Return(orderReadCloser, types.ContainerPathStat{}, nil)
			mockDocker.EXPECT().
				CopyFromContainer(gomock.Any(), "some-container-id", "/buildpacks").
				Return(buildpacksReadCloser, types.ContainerPathStat{}, nil)

			var buf bytes.Buffer
			factory := pack.BuilderFactory{
				Log:    log.New(&buf, "", 0),
				Docker: mockDocker,
			}

			err = factory.Inspect(mockImage)

			h.AssertNil(t, err)
			expectedOutput := fmt.Sprintf(`Builder:  %s`, "some-builder")
			if !strings.Contains(buf.String(), expectedOutput) {
				t.Fatalf(`Expected inspect-builder output to contain '%s', got '%s'`, expectedOutput, buf.String())
			}
			expectedOutput = "Stack:  io.buildpacks.stacks.bionic"
			if !strings.Contains(buf.String(), expectedOutput) {
				t.Fatalf(`Expected inspect-builder output to contain '%s', got '%s'`, expectedOutput, buf.String())
			}
			expectedOutput = `
Detection Order:
mock.bp.first@0.0.1-mock | mock.bp.second@0.0.2-mock | mock.bp.third@0.0.3-mock
`
			if !strings.Contains(buf.String(), expectedOutput) {
				t.Fatalf(`Expected inspect-builder output to contain '%s', got '%s'`, expectedOutput, buf.String())
			}

			expectedOutput = `
Buildpacks:
ID					VERSION
mock.bp.first		version-1
mock.bp.first		version-2
mock.bp.second		version-1
mock.bp.third		version-1
`
			if !strings.Contains(buf.String(), expectedOutput) {
				t.Fatalf(`Expected inspect-builder output to contain '%s', got '%s'`, expectedOutput, buf.String())
			}
		})
	})
}
