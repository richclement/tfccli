package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/go-logr/logr"

	internalcmd "github.com/richclement/tfccli/internal/cmd"
	"github.com/richclement/tfccli/internal/config"
	"github.com/richclement/tfccli/internal/logging"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func versionString() string {
	v := version
	if commit != "" {
		v += " (" + commit + ")"
	}
	if date != "" {
		v += " " + date
	}
	return v
}

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

	Version kong.VersionFlag `name:"version" short:"v" help:"Print version and exit."`

	// Setup commands
	Doctor   DoctorCmd   `cmd:"" help:"Validate settings, token discovery, and connectivity." group:"setup"`
	Init     InitCmd     `cmd:"" help:"Initialize CLI settings." group:"setup"`
	Contexts ContextsCmd `cmd:"" help:"Manage named contexts." group:"setup"`

	// Resource commands
	Organizations      OrganizationsCmd      `cmd:"" help:"Manage organizations." group:"resources"`
	Projects           ProjectsCmd           `cmd:"" help:"Manage projects." group:"resources"`
	Workspaces         WorkspacesCmd         `cmd:"" help:"Manage workspaces." group:"resources"`
	WorkspaceVariables WorkspaceVariablesCmd `cmd:"" name:"workspace-variables" help:"Manage workspace variables." group:"resources"`
	WorkspaceResources WorkspaceResourcesCmd `cmd:"" name:"workspace-resources" help:"List workspace resources." group:"resources"`

	// Operations commands
	Runs                  RunsCmd                  `cmd:"" help:"Manage runs." group:"operations"`
	Plans                 PlansCmd                 `cmd:"" help:"View and download plans." group:"operations"`
	Applies               AppliesCmd               `cmd:"" help:"View applies and download errored state." group:"operations"`
	ConfigurationVersions ConfigurationVersionsCmd `cmd:"" name:"configuration-versions" help:"Manage configuration versions." group:"operations"`

	// Account commands
	Users    UsersCmd    `cmd:"" help:"Manage users." group:"account"`
	Invoices InvoicesCmd `cmd:"" help:"Manage invoices (HCP Terraform Cloud only)." group:"account"`

	// Logger is the logr.Logger used for debug output. Set by run() based on --debug flag and settings.
	Logger logr.Logger `kong:"-"`

	// Ctx is the context for command execution. Set by run() with signal handling for cancellation.
	Ctx context.Context `kong:"-"`
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
		kong.Vars{"version": versionString()},
		kong.ConfigureHelp(kong.HelpOptions{
			NoExpandSubcommands: true,
		}),
		kong.Groups{
			"setup":      "Setup:",
			"resources":  "Resources:",
			"operations": "Operations:",
			"account":    "Account:",
		},
		kong.Exit(func(code int) {
			panic(exitError{code: code})
		}),
		kong.Bind(&cli),
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

	// Create logger based on --debug flag and settings log_level.
	// Default to info level if settings don't exist (e.g., before init).
	logLevel := "info"
	if settings, err := config.Load(""); err == nil {
		contextName := cli.Context
		if contextName == "" {
			contextName = settings.CurrentContext
		}
		if ctx, exists := settings.Contexts[contextName]; exists {
			resolved := ctx.WithDefaults()
			logLevel = resolved.LogLevel
		}
	}
	cli.Logger = logging.NewLogger(logLevel, cli.Debug)

	// Create context with signal handling for cancellation (Ctrl+C).
	signalCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	cli.Ctx = signalCtx

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
