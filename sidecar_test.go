package sidecar_test

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sidecar "github.com/holepunchto/bare-sidecar-golang"
	"github.com/holepunchto/bare-sidecar-golang/prebuild"
)

func command(t *testing.T, fixture string, args ...string) *exec.Cmd {
	t.Helper()

	target, err := prebuild.Target(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}

	executable, err := filepath.Abs(filepath.Join("prebuilds", target, prebuild.Executable(target)))
	if err != nil {
		t.Fatal(err)
	}

	return exec.Command(executable, append([]string{filepath.Join("testdata", fixture)}, args...)...)
}

func start(t *testing.T, cmd *exec.Cmd) *sidecar.Sidecar {
	t.Helper()

	s, err := sidecar.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		s.Close()
	})

	return s
}

func wait(s *sidecar.Sidecar) int {
	s.Wait()

	return s.Cmd.ProcessState.ExitCode()
}

func TestEchoesIPCData(t *testing.T) {
	s := start(t, command(t, "echo.js"))

	if _, err := s.Write([]byte("Hello world")); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 11)

	if _, err := io.ReadFull(s, data); err != nil {
		t.Fatal(err)
	}

	if string(data) != "Hello world" {
		t.Fatalf("got %q", data)
	}
}

func TestPassesArguments(t *testing.T) {
	cmd := command(t, "argv.js", "foo", "bar")
	s := start(t, cmd)

	data, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != `["foo","bar"]` {
		t.Fatalf("got %q", data)
	}

	if code := wait(s); code != 0 {
		t.Fatalf("got exit code %d", code)
	}
}

func TestReportsTheExitCode(t *testing.T) {
	cmd := command(t, "exit.js")
	s := start(t, cmd)

	if code := wait(s); code != 3 {
		t.Fatalf("got exit code %d", code)
	}

	data, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != 0 {
		t.Fatalf("got %q", data)
	}
}

func TestWaitsForTheProcessOnceClosed(t *testing.T) {
	cmd := command(t, "sleep.js")
	s := start(t, cmd)

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if cmd.ProcessState == nil {
		t.Fatal("process not waited for")
	}

	var exit *exec.ExitError

	if err := s.Wait(); !errors.As(err, &exit) {
		t.Fatalf("got %v", err)
	}
}

func TestExitsAfterIPCDestroyWhileWriting(t *testing.T) {
	cmd := command(t, "ipc-destroy-exit.js")
	s := start(t, cmd)

	done := make(chan struct{})

	go func() {
		s.Wait()
		close(done)
	}()

	for {
		select {
		case <-done:
			if code := cmd.ProcessState.ExitCode(); code != 3 {
				t.Fatalf("got exit code %d", code)
			}

			return
		case <-time.After(10 * time.Millisecond):
			s.Write([]byte("Hello world"))
		}
	}
}

func TestPipesStdout(t *testing.T) {
	var stdout bytes.Buffer

	cmd := command(t, "log.js")
	cmd.Stdout = &stdout

	s := start(t, cmd)

	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}

	if out := strings.TrimSpace(stdout.String()); out != "Hello world" {
		t.Fatalf("got %q", stdout.String())
	}
}

func TestTerminates(t *testing.T) {
	cmd := command(t, "sleep.js")
	s := start(t, cmd)

	if err := s.Terminate(); err != nil {
		t.Fatal(err)
	}

	s.Wait()

	if runtime.GOOS == "windows" {
		if code := cmd.ProcessState.ExitCode(); code != 1 {
			t.Fatalf("got exit code %d", code)
		}
	} else if cmd.ProcessState.String() != "signal: terminated" {
		t.Fatalf("got %s", cmd.ProcessState)
	}

	if err := s.Terminate(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamsLargeData(t *testing.T) {
	data := make([]byte, 256*4096)

	for i := range data {
		data[i] = byte(i)
	}

	s := start(t, command(t, "echo.js"))

	written := make(chan error)

	go func() {
		_, err := s.Write(data)
		written <- err
	}()

	echoed := make([]byte, len(data))

	if _, err := io.ReadFull(s, echoed); err != nil {
		t.Fatal(err)
	}

	if err := <-written; err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(echoed, data) {
		t.Fatal("echoed data differs")
	}
}

func TestBundlesTheVersionsOfTheManifest(t *testing.T) {
	cmd := command(t, "version.js")
	s := start(t, cmd)

	data, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}

	if version := strings.TrimPrefix(string(data), "v"); version != sidecar.Versions()["bare"] {
		t.Fatalf("got %q", data)
	}
}

func TestWaitsForTheProcessBeforeEnding(t *testing.T) {
	cmd := command(t, "end-exit.js")
	s := start(t, cmd)

	if _, err := io.ReadAll(s); err != nil {
		t.Fatal(err)
	}

	if cmd.ProcessState == nil {
		t.Fatal("process not waited for")
	}

	if code := cmd.ProcessState.ExitCode(); code != 3 {
		t.Fatalf("got exit code %d", code)
	}
}
