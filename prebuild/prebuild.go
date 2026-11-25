// Package prebuild downloads the prebuilt Bare executables that the releases
// of this module provide.
package prebuild

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Version is the version of this module, whose release provides the
// prebuilds.
const Version = "0.0.0"

// The SHA-256 checksums of the prebuilds attached to the release of this
// version, which the publish workflow checks before releasing them.
var checksums = map[string]string{}

const mirror = "https://github.com/holepunchto/bare-sidecar-golang/releases/download"

// Options are the options of Download.
type Options struct {
	// Dir is the directory to download to, which defaults to a directory of
	// the version in the cache directory of the user.
	Dir string

	// Target is the target to download for, such as `darwin-arm64`, which
	// defaults to the running platform.
	Target string

	// Mirror is the URL to download releases from, which defaults to the
	// GitHub releases of this module.
	Mirror string
}

// Download downloads the prebuilt Bare executable of this version and
// extracts it, unless an earlier download already did, and returns its path.
func Download(ctx context.Context, opts *Options) (string, error) {
	if opts == nil {
		opts = &Options{}
	}

	target := opts.Target

	if target == "" {
		var err error

		target, err = Target(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", err
		}
	}

	checksum, ok := checksums[target]
	if !ok {
		return "", fmt.Errorf("no Bare sidecar prebuild for %s", target)
	}

	dir := opts.Dir

	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}

		dir = filepath.Join(cache, "bare-sidecar", Version)
	}

	archive := filepath.Join(dir, target+".tar.gz")
	out := filepath.Join(dir, "prebuilds", target)
	executable := filepath.Join(out, Executable(target))

	if exists(executable) && verify(archive, checksum) {
		return executable, nil
	}

	base := opts.Mirror

	if base == "" {
		base = mirror
	}

	url := fmt.Sprintf("%s/v%s/%s.tar.gz", strings.TrimSuffix(base, "/"), Version, target)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	if err := download(ctx, url, archive, checksum); err != nil {
		return "", err
	}

	// Extracted aside and renamed into place, so that processes downloading at
	// the same time never run a partial copy.
	temporary, err := os.MkdirTemp(dir, "extract-")
	if err != nil {
		return "", err
	}

	defer os.RemoveAll(temporary)

	if err := extract(archive, temporary); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}

	os.RemoveAll(out)

	err = os.Rename(filepath.Join(temporary, "prebuilds", target), out)

	// Another process may have extracted it in the meantime, which Windows
	// refuses to replace while it is running.
	if err != nil && !exists(executable) {
		return "", err
	}

	return executable, nil
}

// Target returns the target named as by Bare, such as `darwin-arm64`, of an
// operating system and architecture named as by Go.
func Target(goos, goarch string) (string, error) {
	platform, ok := map[string]string{
		"darwin":  "darwin",
		"linux":   "linux",
		"windows": "win32",
	}[goos]
	if !ok {
		return "", fmt.Errorf("unsupported operating system %s", goos)
	}

	arch, ok := map[string]string{
		"amd64": "x64",
		"arm64": "arm64",
	}[goarch]
	if !ok {
		return "", fmt.Errorf("unsupported architecture %s", goarch)
	}

	return platform + "-" + arch, nil
}

// Executable returns the file name of the Bare executable of a target.
func Executable(target string) string {
	if strings.HasPrefix(target, "win32-") {
		return "bare.exe"
	}

	return "bare"
}

// Written aside and renamed into place once verified, so that a failed
// download never replaces a verified archive.
func download(ctx context.Context, url, archive, checksum string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, res.Status)
	}

	file, err := os.CreateTemp(filepath.Dir(archive), "download-")
	if err != nil {
		return err
	}

	defer os.Remove(file.Name())

	sum, err := digest(io.TeeReader(res.Body, file))

	if closeErr := file.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		return err
	}

	if sum != checksum {
		return fmt.Errorf("checksum mismatch for %s", url)
	}

	return os.Rename(file.Name(), archive)
}

// Rooted at dir, so that no entry is written outside of it.
func extract(archive, dir string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}

	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}

	defer root.Close()

	reader := tar.NewReader(gz)

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		name := filepath.FromSlash(header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			err = root.MkdirAll(name, 0o755)
		case tar.TypeReg:
			err = extractFile(root, name, header.FileInfo().Mode().Perm(), reader)
		}

		if err != nil {
			return err
		}
	}
}

func extractFile(root *os.Root, name string, perm os.FileMode, data io.Reader) error {
	if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}

	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}

	_, err = io.Copy(file, data)

	if closeErr := file.Close(); err == nil {
		err = closeErr
	}

	return err
}

func verify(archive, checksum string) bool {
	file, err := os.Open(archive)
	if err != nil {
		return false
	}

	defer file.Close()

	sum, err := digest(file)

	return err == nil && sum == checksum
}

func digest(r io.Reader) (string, error) {
	hash := sha256.New()

	if _, err := io.Copy(hash, r); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
