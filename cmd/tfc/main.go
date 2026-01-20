package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/alecthomas/kong"

	internalcmd "github.com/richclement/tfccli/internal/cmd"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	os.Exit(run())
}

type CLI struct {
	Context      string `help:"Select a named context from settings."`
	Address      string `help:"Override the API address for this invocation."`
	Org          string `help:"Override the default organization for this invocation."`
	OutputFormat string `name:"output-format" enum:"table,json," default:"" help:"Output format: table or json."`
	Debug        bool   `help:"Enable debug logging for this invocation."`
	Force        bool   `help:"Bypass confirmation prompts for destructive operations."`

	Version VersionCmd `cmd:"" help:"Print version information."`
	Doctor  DoctorCmd  `cmd:"" help:"Validate settings, token discovery, and connectivity."`
}

// VersionCmd prints the CLI version info.
type VersionCmd struct{}

func (v *VersionCmd) Run() error {
	fmt.Printf("version: %s\n", version)
	fmt.Printf("commit:  %s\n", commit)
	fmt.Printf("date:    %s\n", date)
	return nil
}

// DoctorCmd is a placeholder for the full doctor implementation.
type DoctorCmd struct{}

func (d *DoctorCmd) Run() error {
	// Placeholder - full implementation in Task 14
	return internalcmd.NewRuntimeError(errors.New("doctor not yet implemented"))
}

type exitError struct {
	code int
}

func run() (exitCode int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if exitErr, ok := recovered.(exitError); ok {
				exitCode = exitErr.code
				return
			}
			fmt.Fprintln(os.Stderr, "unexpected error")
			exitCode = 3
		}
	}()

	cli := CLI{}
	parser, err := kong.New(
		&cli,
		kong.Name("tfc"),
		kong.Description("Terraform Cloud API CLI"),
		kong.Exit(func(code int) {
			panic(exitError{code: code})
		}),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 3
	}

	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		printParseError(err)
		return 1
	}

	if err := ctx.Run(); err != nil {
		return exitCodeForError(err)
	}
	return 0
}

func printParseError(err error) {
	fmt.Fprintln(os.Stderr, err)
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) {
		_ = parseErr.Context.PrintUsage(true)
	}
}

func exitCodeForError(err error) int {
	var runtimeErr internalcmd.RuntimeError
	if errors.As(err, &runtimeErr) {
		fmt.Fprintln(os.Stderr, runtimeErr.Error())
		return 2
	}
	fmt.Fprintln(os.Stderr, err)
	return 3
}
