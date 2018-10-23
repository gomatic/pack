package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func writePProf(name string) {
	log.Println("PPROF:", name)
	f, err := os.Create(name)
	if err != nil {
		log.Fatal("could not create memory profile: ", err)
	}
	runtime.GC() // get up-to-date statistics
	if err := pprof.WriteHeapProfile(f); err != nil {
		log.Fatal("could not write memory profile: ", err)
	}
	f.Close()
}

func main() {
	// t, err := name.NewTag("cfbuildpacks/cflinuxfs3-cnb-experimental:build", name.WeakValidation)
	t, err := name.NewTag("packs/run:latest", name.WeakValidation)
	if err != nil {
		log.Fatal(err)
	}
	baseImage, err := daemon.Image(t)
	if err != nil {
		// Assume error is due to non-existent image
		log.Fatal(err)
	}
	writePProf("/tmp/pack.test.1.prof")

	_, err = baseImage.RawManifest()
	if err != nil {
		// Assume error is due to non-existent image
		// This is necessary for registries
		log.Fatal("NOT EXIST:", err)
	}
	writePProf("/tmp/pack.test.2.prof")
	// log.Println(string(m))

	writePProf("/tmp/pack.test.3.prof")

	// layer, err := tarball.LayerFromFile("/tmp/single.tgz")
	layerContents := fmt.Sprintf("my contents: %v", time.Now())
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return createTar("contents.txt", layerContents)
	})
	if err != nil {
		log.Fatal(err)
	}
	writePProf("/tmp/pack.test.4.prof")

	image, err := mutate.AppendLayers(baseImage, layer)
	if err != nil {
		log.Fatal(err)
	}
	writePProf("/tmp/pack.test.5.prof")

	t2, err := name.NewTag("dave:dave", name.WeakValidation)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := daemon.Write(t2, image, daemon.WriteOptions{}); err != nil {
		log.Fatal(err)
	}
	writePProf("/tmp/pack.test.6.prof")
}

func createTar(path, txt string) (io.ReadCloser, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(txt)), Mode: 0666, ModTime: time.Now()}); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(txt)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewReader(buf.Bytes())), nil
}
