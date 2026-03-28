package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/richclement/tfccli/internal/releaseverify"
)

func main() {
	var binaryPath string
	var version string

	flag.StringVar(&binaryPath, "binary", "", "path to the built tfccli binary to verify")
	flag.StringVar(&version, "version", "dev", "version string expected from the built binary")
	flag.Parse()

	if binaryPath == "" {
		fmt.Fprintln(os.Stderr, "error: -binary is required")
		os.Exit(2)
	}

	if err := releaseverify.Verify(binaryPath, version); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
