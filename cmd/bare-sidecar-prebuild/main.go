// Command bare-sidecar-prebuild downloads the prebuilt Bare executable of the
// version of this module and prints its path.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/holepunchto/bare-sidecar-golang/prebuild"
)

func main() {
	var opts prebuild.Options

	flag.StringVar(&opts.Dir, "dir", "", "the directory to download to, which defaults to the cache directory of the user")
	flag.StringVar(&opts.Target, "target", "", "the target to download for, such as darwin-arm64, which defaults to the running platform")
	flag.StringVar(&opts.Mirror, "mirror", "", "the URL to download releases from, which defaults to the GitHub releases of the module")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	path, err := prebuild.Download(ctx, &opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(path)
}
