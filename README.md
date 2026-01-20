# tfc

A CLI for interacting with the Terraform Cloud / HCP Terraform API. Built with Go, designed for both human operators and AI coding agents.

## Features

- **Full CRUD support** for core Terraform Cloud resources (organizations, projects, workspaces, runs, plans, applies, etc.)
- **Terraform CLI authentication conventions** - uses your existing `terraform login` credentials
- **Multiple contexts** - manage multiple TFC/TFE environments with named profiles
- **Automation-friendly** - consistent JSON output, exit codes, and `--force` flag for non-interactive use
- **Download support** - fetch plan JSON, sanitized plans, errored state, and configuration versions

## Installation

### From Source

```bash
# Clone and build
git clone https://github.com/richclement/tfccli.git
cd tfccli
make build

# Binary is at bin/tfc
./bin/tfc version
```

### Go Install

```bash
go install github.com/richclement/tfccli/cmd/tfc@latest
```

### Releases

Download pre-built binaries from the [releases page](https://github.com/richclement/tfccli/releases).

## Quick Start

### 1. Initialize Configuration

```bash
# Interactive setup
tfc init

# Non-interactive setup (for CI/agents)
tfc init --non-interactive --address app.terraform.io --default-org my-org --yes
```

This creates `~/.tfccli/settings.json` with your default context.

### 2. Authenticate with Terraform Cloud

`tfc` uses Terraform CLI's authentication system. Run `terraform login` to authenticate:

```bash
terraform login
# Follow the prompts to generate and store your API token
```

### 3. Verify Setup

```bash
tfc doctor
```

This validates your settings, token discovery, and API connectivity.

## Authentication

`tfc` discovers API tokens using Terraform CLI conventions. No tokens are stored in `~/.tfccli/settings.json`.

Token discovery precedence (highest to lowest):

1. **Environment variable**: `TF_TOKEN_<sanitized_host>`
   ```bash
   # For app.terraform.io (dots become underscores)
   export TF_TOKEN_app_terraform_io="your-token-here"
   ```

2. **Terraform CLI config**: `TF_CLI_CONFIG_FILE` environment variable or `~/.terraformrc`
   ```hcl
   credentials "app.terraform.io" {
     token = "your-token-here"
   }
   ```

3. **Terraform login credentials**: `~/.terraform.d/credentials.tfrc.json`
   (Created automatically by `terraform login`)

If no token is found, `tfc` suggests running `terraform login`.

## Configuration

### Settings Location

`~/.tfccli/settings.json`

### Multi-Context Support

Manage multiple TFC/TFE environments with named contexts:

```bash
# List contexts
tfc contexts list

# Add a new context
tfc contexts add prod --ctx-address tfe.example.com --default-org acme

# Switch contexts
tfc contexts use prod

# Show current context config
tfc contexts show

# Remove a context (requires confirmation)
tfc contexts remove dev
```

### Global Flags

| Flag | Description |
|------|-------------|
| `--context <name>` | Select a named context from settings |
| `--address <addr>` | Override the API address for this invocation |
| `--org <org>` | Override the default organization |
| `--output-format <table\|json>` | Output format (default: table on TTY, json otherwise) |
| `--debug` | Enable debug logging |
| `--force` | Bypass confirmation prompts for destructive operations |

## Commands

### Organizations

```bash
# List all organizations
tfc organizations list

# Get organization details
tfc organizations get my-org

# Create an organization
tfc organizations create --name new-org --email admin@example.com

# Update an organization
tfc organizations update my-org --email new-email@example.com

# Delete an organization (requires confirmation)
tfc organizations delete old-org
tfc organizations delete old-org --force  # Skip confirmation
```

### Projects

```bash
# List projects (uses --org or default_org)
tfc projects list

# List projects for a specific org
tfc projects list --org my-org

# Get project by ID
tfc projects get prj-abc123

# Create a project
tfc projects create --name my-project

# Update a project
tfc projects update prj-abc123 --name new-name --description "Updated desc"

# Delete a project
tfc projects delete prj-abc123 --force
```

### Workspaces

```bash
# List workspaces
tfc workspaces list --org my-org

# Get workspace by ID
tfc workspaces get ws-abc123

# Create a workspace
tfc workspaces create --name my-workspace --org my-org

# Create workspace in a specific project
tfc workspaces create --name my-workspace --org my-org --project-id prj-abc123

# Update a workspace
tfc workspaces update ws-abc123 --name renamed-workspace

# Delete a workspace
tfc workspaces delete ws-abc123 --force
```

### Workspace Variables

```bash
# List variables for a workspace
tfc workspace-variables list --workspace-id ws-abc123

# Create an environment variable
tfc workspace-variables create --workspace-id ws-abc123 \
  --key AWS_REGION --value us-east-1 --category env

# Create a sensitive terraform variable
tfc workspace-variables create --workspace-id ws-abc123 \
  --key db_password --value secret123 --category terraform --sensitive

# Create an HCL variable
tfc workspace-variables create --workspace-id ws-abc123 \
  --key tags --value '{"env":"prod"}' --category terraform --hcl

# Update a variable
tfc workspace-variables update var-abc123 --value new-value

# Delete a variable
tfc workspace-variables delete var-abc123 --force
```

### Workspace Resources

```bash
# List resources in a workspace (read-only)
tfc workspace-resources list --workspace-id ws-abc123
```

### Runs

```bash
# List runs for a workspace
tfc runs list --workspace-id ws-abc123

# Get run details
tfc runs get run-abc123

# Create a run
tfc runs create --workspace-id ws-abc123 --message "Deploying new feature"

# Create a run with auto-apply
tfc runs create --workspace-id ws-abc123 --auto-apply --message "Auto-deploy"

# Apply a run (requires confirmation)
tfc runs apply run-abc123
tfc runs apply run-abc123 --force  # Skip confirmation

# Discard a run
tfc runs discard run-abc123 --force

# Cancel a run
tfc runs cancel run-abc123 --force

# Force-cancel a run
tfc runs force-cancel run-abc123 --force
```

### Plans

```bash
# Get plan details
tfc plans get plan-abc123

# Download JSON plan output
tfc plans json-output plan-abc123
tfc plans json-output plan-abc123 --out plan.json  # Save to file

# Download sanitized plan (HYOK feature)
tfc plans sanitized-plan plan-abc123
tfc plans sanitized-plan plan-abc123 --out sanitized.json
```

### Applies

```bash
# Get apply details
tfc applies get apply-abc123

# Download errored state (for recovery)
tfc applies errored-state apply-abc123
tfc applies errored-state apply-abc123 --out errored.tfstate
```

### Configuration Versions

```bash
# List configuration versions
tfc configuration-versions list --workspace-id ws-abc123

# Get configuration version details
tfc configuration-versions get cv-abc123

# Create a new configuration version
tfc configuration-versions create --workspace-id ws-abc123

# Create without auto-queuing runs
tfc configuration-versions create --workspace-id ws-abc123 --auto-queue-runs=false

# Upload configuration (tar.gz)
tfc configuration-versions upload cv-abc123 --file ./config.tar.gz

# Download configuration
tfc configuration-versions download cv-abc123
tfc configuration-versions download cv-abc123 --out config.tar.gz

# Archive a configuration version
tfc configuration-versions archive cv-abc123 --force
```

### Users

```bash
# Get user by ID
tfc users get user-abc123
```

### Invoices (HCP Terraform Cloud only)

```bash
# List invoices
tfc invoices list --org my-org

# Get next/upcoming invoice
tfc invoices next --org my-org
```

## Output Formats

### JSON Output

Use `--output-format=json` or pipe output (auto-detects non-TTY):

```bash
# Explicit JSON
tfc organizations list --output-format=json

# Piped (auto-JSON)
tfc organizations list | jq '.data[].attributes.name'

# JSON for empty responses (204 status)
tfc organizations delete old-org --force --output-format=json
# Output: {"meta":{"status":204}}
```

### Table Output

Default when stdout is a TTY:

```bash
tfc organizations list
# NAME        EMAIL               EXTERNAL-ID
# my-org      admin@example.com   ext-123
# other-org   other@example.com   ext-456
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | CLI usage/validation error (missing flags, invalid enum) |
| 2 | Runtime error (config missing, auth failed, API error) |
| 3 | Unexpected/internal error |

## Automation and Agent Usage

For CI/CD pipelines and AI coding agents:

```bash
# Non-interactive initialization
tfc init --non-interactive --address app.terraform.io --default-org my-org --yes

# Always use --force for destructive operations
tfc organizations delete old-org --force
tfc runs apply run-abc123 --force

# Explicit JSON output for parsing
tfc workspaces list --output-format=json | jq '.data[].id'

# Check exit codes
if tfc doctor; then
  echo "TFC connection healthy"
else
  echo "TFC connection failed"
  exit 1
fi
```

## Development

```bash
# Build
make build

# Run tests
make test

# Run linter
make lint

# Format code
make fmt

# CI pipeline (fmt-check + lint + test)
make ci

# Build and run with args
make tfc ARGS="version"
```

## License

See [LICENSE](LICENSE) for details.
