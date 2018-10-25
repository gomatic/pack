package testhelpers

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
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

func Contains(arr []string, val string) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func proxyDockerHostPort(port string) (string, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
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
				cmd := exec.Command("docker", "run", "-i", "--log-driver=none", "-a", "stdin", "-a", "stdout", "-a", "stderr", "--network=host", "alpine/socat", "-", "TCP:localhost:"+port)
				cmd.Stdin = conn
				cmd.Stdout = conn
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					log.Println(err)
				}
			}(conn)
		}
	}()
	addr := ln.Addr().String()
	i := strings.LastIndex(addr, ":")
	if i == -1 {
		return "", fmt.Errorf("finding port: ':' not found in '%s'", addr)
	}
	return addr[(i + 1):], nil
}

var runRegistryName, runRegistryLocalPort, runRegistryRemotePort string
var runRegistryOnce sync.Once

func RunRegistry(t *testing.T) (name, localPort string) {
	t.Helper()
	runRegistryOnce.Do(func() {
		runRegistryName = "test-registry-" + RandString(10)
		AssertNil(t, exec.Command("docker", "run", "-d", "--rm", "-p", ":5000", "--name", runRegistryName, "registry:2").Run())
		port, err := exec.Command("docker", "inspect", runRegistryName, "-f", `{{index (index (index .NetworkSettings.Ports "5000/tcp") 0) "HostPort"}}`).Output()
		AssertNil(t, err)
		runRegistryRemotePort = strings.TrimSpace(string(port))
		if os.Getenv("DOCKER_HOST") != "" {
			runRegistryLocalPort, err = proxyDockerHostPort(runRegistryRemotePort)
			AssertNil(t, err)
		} else {
			runRegistryLocalPort = runRegistryRemotePort
		}
	})
	return runRegistryName, runRegistryLocalPort
}

func StopRegistry(t *testing.T) {
	if runRegistryName != "" {
		Run(t, exec.Command("docker", "kill", runRegistryName))
	}
}

func HttpGet(t *testing.T, url string) string {
	t.Helper()
	if os.Getenv("DOCKER_HOST") == "" {
		resp, err := http.Get(url)
		AssertNil(t, err)
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Fatalf("HTTP Status was bad: %s => %d", url, resp.StatusCode)
		}
		b, err := ioutil.ReadAll(resp.Body)
		AssertNil(t, err)
		return string(b)
	} else {
		return Run(t, exec.Command("docker", "run", "--log-driver=none", "--entrypoint=", "--network=host", "packs/samples", "wget", "-q", "-O", "-", url))
	}
}

func CopyWorkspaceToDocker(t *testing.T, srcPath, destVolume string) {
	t.Helper()
	ctrName := uuid.New().String()
	defer exec.Command("docker", "rm", ctrName).Run()
	Run(t, exec.Command("docker", "create", "--name", ctrName, "-v", destVolume+":/workspace", "packs/samples", "true"))
	Run(t, exec.Command("docker", "cp", srcPath+"/.", ctrName+":/workspace/"))
}

func ReadFromDocker(t *testing.T, volume, path string) string {
	t.Helper()

	var buf bytes.Buffer
	cmd := exec.Command("docker", "run", "-v", volume+":/workspace", "packs/samples", "cat", path)
	cmd.Stdout = &buf
	AssertNil(t, cmd.Run())
	return buf.String()
}

func Run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()

	if runRegistryLocalPort != "" && runRegistryRemotePort != "" {
		for i, arg := range cmd.Args {
			cmd.Args[i] = strings.Replace(arg, "localhost:"+runRegistryLocalPort, "localhost:"+runRegistryRemotePort, -1)
		}
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to execute command: %v, %s, %s, %s", cmd.Args, err, stderr.String(), output)
	}

	return string(output)
}
