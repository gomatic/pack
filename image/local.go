package image

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildpack/pack/fs"
	"github.com/docker/docker/api/types"
	dockercli "github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

type local struct {
	RepoName         string
	Docker           Docker
	Inspect          types.ImageInspect
	layerPaths       []string
	Stdout           io.Writer
	FS               *fs.FS
	currentTempImage string
}

func (f *Factory) NewLocal(repoName string, pull bool) (Image, error) {
	if pull {
		f.Log.Printf("Pulling image '%s'\n", repoName)
		if err := f.Docker.PullImage(repoName); err != nil {
			return nil, fmt.Errorf("failed to pull image '%s' : %s", repoName, err)
		}
	}

	inspect, _, err := f.Docker.ImageInspectWithRaw(context.Background(), repoName)
	if err != nil && !dockercli.IsErrNotFound(err) {
		return nil, errors.Wrap(err, "analyze read previous image config")
	}

	return &local{
		Docker:     f.Docker,
		RepoName:   repoName,
		Inspect:    inspect,
		layerPaths: make([]string, len(inspect.RootFS.Layers)),
		Stdout:     f.Stdout,
		FS:         f.FS,
	}, nil
}

func (l *local) Label(key string) (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get label, image '%s' does not exist", l.RepoName)
	}
	labels := l.Inspect.Config.Labels
	return labels[key], nil
}

func (l *local) SetName(name string) {
	l.RepoName = name
}

func (l *local) Name() string {
	return l.RepoName
}

func (l *local) Digest() (string, error) {
	if l.Inspect.Config == nil {
		return "", fmt.Errorf("failed to get digest, image '%s' does not exist", l.RepoName)
	}
	if len(l.Inspect.RepoDigests) == 0 {
		return "", nil
	}
	parts := strings.Split(l.Inspect.RepoDigests[0], "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("failed to get digest, image '%s' has malformed digest '%s'", l.RepoName, l.Inspect.RepoDigests[0])
	}
	return parts[1], nil
}

func (l *local) Rebase(baseTopLayer string, newBase Image) error {
	ctx := context.Background()

	// FIND TOP LAYER
	keepLayers := -1
	for i, diffID := range l.Inspect.RootFS.Layers {
		if diffID == baseTopLayer {
			keepLayers = len(l.Inspect.RootFS.Layers) - i - 1
			break
		}
	}
	if keepLayers == -1 {
		return fmt.Errorf("'%s' not found in '%s' during rebase", baseTopLayer, l.RepoName)
	}

	// SWITCH BASE LAYERS
	newBaseInspect, _, err := l.Docker.ImageInspectWithRaw(ctx, newBase.Name())
	if err != nil {
		return errors.Wrap(err, "analyze read previous image config")
	}
	l.Inspect.RootFS.Layers = newBaseInspect.RootFS.Layers
	l.layerPaths = make([]string, len(l.Inspect.RootFS.Layers))

	// SAVE CURRENT IMAGE TO DISK
	tmpDir, err := ioutil.TempDir("", "packs.local.rebase.")
	if err != nil {
		return errors.Wrap(err, "local rebase create temp dir")
	}
	defer os.RemoveAll(tmpDir)

	rc, err := l.Docker.ImageSave(ctx, []string{l.RepoName})
	if err != nil {
		return errors.Wrap(err, "local rebase access old image")
	}
	defer rc.Close()

	if err := l.FS.Untar(rc, tmpDir); err != nil {
		return errors.Wrap(err, "local rebase untar old image")
	}

	// READ MANIFEST.JSON
	b, err := ioutil.ReadFile(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest []struct{ Layers []string }
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if len(manifest) != 1 {
		return fmt.Errorf("expected 1 image received %d", len(manifest))
	}

	// ADD EXISTING LAYERS
	for _, filename := range manifest[0].Layers[(len(manifest[0].Layers) - keepLayers):] {
		if err := l.AddLayer(filepath.Join(tmpDir, filename)); err != nil {
			return err
		}
	}

	if _, err = l.Save(); err != nil {
		return err
	}
	l.layerPaths = make([]string, len(l.Inspect.RootFS.Layers))
	return nil
}

func (l *local) SetLabel(key, val string) error {
	if l.Inspect.Config == nil {
		return fmt.Errorf("failed to set label, image '%s' does not exist", l.RepoName)
	}
	l.Inspect.Config.Labels[key] = val
	return nil
}

func (l *local) TopLayer() (string, error) {
	all := l.Inspect.RootFS.Layers
	topLayer := all[len(all)-1]
	return topLayer, nil
}

func (l *local) AddLayer(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "AddLayer: open layer: %s", path)
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrapf(err, "AddLayer: calculate checksum: %s", path)
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	l.Inspect.RootFS.Layers = append(l.Inspect.RootFS.Layers, "sha256:"+sha)
	l.layerPaths = append(l.layerPaths, path)

	return nil
}

func (l *local) ReuseLayer(sha string) error {
	ctx := context.Background()

	t, err := name.NewTag(l.RepoName, name.WeakValidation)
	if err != nil {
		return err
	}
	repoName := t.String()

	tarFile, err := l.Docker.ImageSave(ctx, []string{repoName})
	if err != nil {
		return err
	}
	defer tarFile.Close()

	tmpDir, err := ioutil.TempDir("", "packs.local.reuse-layer.")
	if err != nil {
		return errors.Wrap(err, "local reuse-layer create temp dir")
	}
	// ... how do we delete the tempdir after saving?

	err = l.FS.Untar(tarFile, tmpDir)
	if err != nil {
		return err
	}

	fmt.Println(filepath.Glob(tmpDir + "/*"))

	// manifest.json,   ---> blarg12345.json

	mf, err := os.Open(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		return err
	}
	defer mf.Close()

	var manifest []struct {
		Config string
		Layers []string
	}
	if err := json.NewDecoder(mf).Decode(&manifest); err != nil {
		return err
	}

	if len(manifest) != 1 {
		return fmt.Errorf("manifest.json had unexpected number of entries: %d", len(manifest))
	}

	df, err := os.Open(filepath.Join(tmpDir, manifest[0].Config))
	if err != nil {
		return err
	}
	defer df.Close()

	var details struct {
		RootFS struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}

	if err = json.NewDecoder(df).Decode(&details); err != nil {
		return err
	}

	if len(manifest[0].Layers) != len(details.RootFS.DiffIDs) {
		return fmt.Errorf("layers and diff IDs do not match, there are %d layers and %d diffIDs", len(manifest[0].Layers), len(details.RootFS.DiffIDs))
	}

	layerMap := make(map[string]string, len(manifest[0].Layers))
	for i, diffID := range details.RootFS.DiffIDs {
		layerID := manifest[0].Layers[i]
		layerMap[diffID] = layerID
	}

	fmt.Printf("LAYER MAP: %#v\n", layerMap)

	reuseLayer, ok := layerMap[sha]
	if !ok {
		return fmt.Errorf("SHA %s was not found in %s", sha, l.RepoName)
	}

	return l.AddLayer(filepath.Join(tmpDir, reuseLayer))
}

func (l *local) Save() (string, error) {
	ctx := context.Background()
	done := make(chan error)

	t, err := name.NewTag(l.RepoName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	repoName := t.String()

	pr, pw := io.Pipe()
	go func() {
		res, err := l.Docker.ImageLoad(ctx, pr, true)
		if err != nil {
			io.Copy(ioutil.Discard, res.Body)
		}
		done <- err
	}()

	tw := tar.NewWriter(pw)

	imgConfig := map[string]interface{}{
		"os":      "linux",
		"created": time.Now().Format(time.RFC3339),
		"config":  l.Inspect.Config,
		"rootfs": map[string][]string{
			"diff_ids": l.Inspect.RootFS.Layers,
		},
	}
	formatted, err := json.Marshal(imgConfig)
	if err != nil {
		return "", err
	}
	imgID := fmt.Sprintf("%x", sha256.Sum256(formatted))
	if err := l.FS.AddTextToTar(tw, imgID+".json", formatted); err != nil {
		return "", err
	}

	var layerPaths []string
	for _, path := range l.layerPaths {
		if path == "" {
			layerPaths = append(layerPaths, "")
			continue
		}
		layerName := fmt.Sprintf("/%x.tar", sha256.Sum256([]byte(path)))
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if err := l.FS.AddFileToTar(tw, layerName, f); err != nil {
			return "", err
		}
		f.Close()
		layerPaths = append(layerPaths, layerName)

	}

	formatted, err = json.Marshal([]map[string]interface{}{
		{
			"Config":   imgID + ".json",
			"RepoTags": []string{repoName},
			"Layers":   layerPaths,
		},
	})
	if err != nil {
		return "", err
	}
	if err := l.FS.AddTextToTar(tw, "manifest.json", formatted); err != nil {
		return "", err
	}

	tw.Close()
	pw.Close()

	err = <-done
	return imgID, err
}

// TODO copied from exporter.go
func parseImageBuildBody(r io.Reader, out io.Writer) (string, error) {
	jr := json.NewDecoder(r)
	var id string
	var streamError error
	var obj struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
		Aux    struct {
			ID string `json:"ID"`
		} `json:"aux"`
	}
	for {
		err := jr.Decode(&obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if obj.Aux.ID != "" {
			id = obj.Aux.ID
		}
		if txt := strings.TrimSpace(obj.Stream); txt != "" {
			fmt.Fprintln(out, txt)
		}
		if txt := strings.TrimSpace(obj.Error); txt != "" {
			streamError = errors.New(txt)
		}
	}
	return id, streamError
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(rand.Intn(26))
	}
	return string(b)
}
