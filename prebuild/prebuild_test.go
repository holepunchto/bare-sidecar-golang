package prebuild_test

import (
	"testing"

	"github.com/holepunchto/bare-sidecar-golang/prebuild"
)

func TestTarget(t *testing.T) {
	for _, test := range []struct {
		goos, goarch, target string
	}{
		{"darwin", "arm64", "darwin-arm64"},
		{"darwin", "amd64", "darwin-x64"},
		{"linux", "arm64", "linux-arm64"},
		{"linux", "amd64", "linux-x64"},
		{"windows", "arm64", "win32-arm64"},
		{"windows", "amd64", "win32-x64"},
	} {
		target, err := prebuild.Target(test.goos, test.goarch)
		if err != nil {
			t.Fatal(err)
		}

		if target != test.target {
			t.Fatalf("got %s for %s/%s", target, test.goos, test.goarch)
		}
	}
}

func TestTargetUnsupported(t *testing.T) {
	if _, err := prebuild.Target("plan9", "amd64"); err == nil {
		t.Fatal("expected an error for plan9")
	}

	if _, err := prebuild.Target("linux", "riscv64"); err == nil {
		t.Fatal("expected an error for riscv64")
	}
}

func TestDownloadUnknownTarget(t *testing.T) {
	_, err := prebuild.Download(t.Context(), &prebuild.Options{
		Dir:    t.TempDir(),
		Target: "darwin-riscv64",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}
