# bare-sidecar-golang

Start and manage Bare sidecar processes from Go, each running Bare alongside the application with an IPC to it exposed as `Bare.IPC` inside the process.

```
go get github.com/holepunchto/bare-sidecar-golang
```

## Usage

```go
package main

import (
	"fmt"
	"io"
	"os/exec"

	sidecar "github.com/holepunchto/bare-sidecar-golang"
)

func main() {
	cmd := exec.Command("path/to/bare", "app.js")

	s, err := sidecar.Start(cmd)
	if err != nil {
		panic(err)
	}

	defer s.Close()

	s.Write([]byte("Hello world"))

	data := make([]byte, 11)

	if _, err := io.ReadFull(s, data); err != nil {
		panic(err)
	}

	fmt.Println(string(data))
}
```

Where `app.js` is a Bare entry point using `Bare.IPC`:

```js
Bare.IPC.on('data', (data) => Bare.IPC.write(data))
```

The command is an ordinary `exec.Cmd`, so its standard I/O, environment, working directory, and cancellation are configured as usual before starting it. Wait for it with `s.Wait()` rather than `cmd.Wait()`. Reading to `io.EOF` and closing with `s.Close()` also wait for it, so the exit status in `cmd.ProcessState` is set by the time they return. Closing terminates the process first.

## Prebuilds

The command must run a Bare executable built with the embedder of this module. The releases of the module provide prebuilt executables, which the `prebuild` package downloads, verifies, and caches:

```go
path, err := prebuild.Download(ctx, nil)
```

To download them as a build step instead, add the `bare-sidecar-prebuild` tool to the module, which then always downloads the executables of the version that the module depends on:

```
go get -tool github.com/holepunchto/bare-sidecar-golang/cmd/bare-sidecar-prebuild
go tool bare-sidecar-prebuild -dir prebuilds -target win32-x64
```

## Versions

`sidecar.Versions()` returns the versions of the components bundled with the executable, keyed by name like `Bare.versions`. The `bare` key holds the version of Bare itself:

```go
fmt.Println(sidecar.Versions()["bare"])
```

The versions come from `versions.json`, which is also where the build reads the version of Bare from.

## License

Apache-2.0
