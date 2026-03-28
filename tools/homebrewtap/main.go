package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/richclement/tfccli/internal/homebrewtap"
)

func main() {
	var version string
	var checksumsPath string
	var tapDir string
	var formulaName string
	var sourceRepo string
	var prBodyPath string

	flag.StringVar(&version, "version", "", "release version without the leading v")
	flag.StringVar(&checksumsPath, "checksums-file", "", "path to release checksums.txt")
	flag.StringVar(&tapDir, "tap-dir", "", "path to the checked out homebrew tap")
	flag.StringVar(&formulaName, "formula-name", "", "formula name to update")
	flag.StringVar(&sourceRepo, "source-repo", "", "GitHub owner/repo for release URLs")
	flag.StringVar(&prBodyPath, "pr-body-file", "", "path to write the pull request body markdown")
	flag.Parse()

	if version == "" || checksumsPath == "" || tapDir == "" || formulaName == "" || sourceRepo == "" || prBodyPath == "" {
		fmt.Fprintln(os.Stderr, "error: -version, -checksums-file, -tap-dir, -formula-name, -source-repo, and -pr-body-file are required")
		os.Exit(2)
	}

	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: read checksums: %v\n", err)
		os.Exit(1)
	}

	result, err := homebrewtap.Generate(version, sourceRepo, formulaName, checksums)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: generate homebrew files: %v\n", err)
		os.Exit(1)
	}

	if err := homebrewtap.WriteFiles(homebrewtap.WriteOptions{
		TapDir:     tapDir,
		PRBodyPath: prBodyPath,
	}, formulaName, result); err != nil {
		fmt.Fprintf(os.Stderr, "error: write homebrew files: %v\n", err)
		os.Exit(1)
	}
}
