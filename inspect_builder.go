package pack

import (
	"archive/tar"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle"
	"github.com/buildpack/pack/image"
	"github.com/docker/docker/api/types/container"
	"io"
	"strings"
)

func (bf *BuilderFactory) ImageFromFlags(builderName string) (image.Image, error) {
	return nil, nil
}

func (bf *BuilderFactory) Inspect(builderImage image.Image) (error) {
	bf.logName(builderImage)
	if err := bf.logStack(builderImage); err != nil {
		return err
	}
	cont, err := bf.Docker.ContainerCreate(nil, &container.Config{
		Image: builderImage.Name(),
	}, nil, nil, "")
	if err != nil {
		return err
	}
	err = bf.logDetectionOrder(cont.ID)
	if err != nil {
		return err
	}
	return bf.logBuildpacks(cont.ID)
}

func (bf *BuilderFactory) logName(builderImage image.Image) {
	bf.Log.Println(fmt.Sprintf(`Builder:  %s`, builderImage.Name()))
}

func (bf *BuilderFactory) logStack(builderImage image.Image) error {
	labelValue, err := builderImage.Label("io.buildpacks.stack.id")
	if err != nil {
		return err
	}
	bf.Log.Println(fmt.Sprintf(`Stack:  %s`, labelValue))
	return nil
}

func (bf *BuilderFactory) logDetectionOrder(containerId string) error {
	bufReader, _, err := bf.Docker.CopyFromContainer(nil, containerId, "/buildpacks/order.toml")
	tr := tar.NewReader(bufReader)
	header, err := tr.Next()
	buf := make([]byte, header.Size, header.Size)
	tr.Read(buf)
	var orderData struct {
		Groups lifecycle.BuildpackOrder
	}
	err = toml.Unmarshal(buf, &orderData)
	if err != nil {
		return err
	}
	bf.Log.Println("Detection Order:")
	for _, group := range orderData.Groups {
		strOut := ""
		for _, bp := range group.Buildpacks {
			strOut += bp.ID + "@" + bp.Version + " | "
		}
		bf.Log.Println(strings.TrimRight(strOut, " | "))
	}
	return nil
}

func (bf *BuilderFactory) logBuildpacks(containerId string) error {
	bufReader, _, err := bf.Docker.CopyFromContainer(nil, containerId, "/buildpacks")
	if err != nil {
		return err
	}
	tr := tar.NewReader(bufReader)
	buildpacks := make(map[string][]string)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		parts := strings.Split(header.Name, "/")
		bpID := parts[2]
		bpVersion := parts[3]
		_, ok := buildpacks[bpID]
		if !ok {
			buildpacks[bpID] = make([]string, 0, 0)
		}
		buildpacks[bpID] = append(buildpacks[bpID], bpVersion)
	}

	bf.Log.Println("Buildpacks:")
	bf.Log.Println("ID\t\t\t\t\tVERSION")
	for k, v := range buildpacks {
		for _, b := range v {
			bf.Log.Printf("%s\t\t%s\n", k, b)
		}
	}

	return nil
}
