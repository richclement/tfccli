# tfccli

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

# Binary is at bin/tfccli
./bin/tfccli version
```

### Go Install

```bash
go install github.com/richclement/tfccli/cmd/tfccli@latest
```

### Releases

Download pre-built binaries from the [releases page](https://github.com/richclement/tfccli/releases).

## Quick Start

### 1. Initialize Configuration

```bash
# Interactive setup
tfccli init

# Non-interactive setup (for CI/agents)
tfccli init --non-interactive --address app.terraform.io --default-org my-org --yes
```

This creates `~/.tfccli/settings.json` with your default context.

### 2. Authenticate with Terraform Cloud

`tfccli` uses Terraform CLI's authentication system. Run `terraform login` to authenticate:

```bash
terraform login
# Follow the prompts to generate and store your API token
```

### 3. Verify Setup

```bash
tfccli doctor
```

This validates your settings, token discovery, and API connectivity.

## Authentication

`tfccli` discovers API tokens using Terraform CLI conventions. No tokens are stored in `~/.tfccli/settings.json`.

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

If no token is found, `tfccli` suggests running `terraform login`.

## Configuration

### Settings Location

`~/.tfccli/settings.json`

### Multi-Context Support

Manage multiple TFC/TFE environments with named contexts:

```bash
# List contexts
tfccli contexts list

# Add a new context
tfccli contexts add prod --ctx-address tfe.example.com --default-org acme

# Switch contexts
tfccli contexts use prod

# Show current context config
tfccli contexts show

# Remove a context (requires confirmation)
tfccli contexts remove dev
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
tfccli organizations list

# Get organization details
tfccli organizations get my-org

# Create an organization
tfccli organizations create --name new-org --email admin@example.com

# Update an organization
tfccli organizations update my-org --email new-email@example.com

# Delete an organization (requires confirmation)
tfccli organizations delete old-org
tfccli organizations delete old-org --force  # Skip confirmation
```

### Projects

```bash
# List projects (uses --org or default_org)
tfccli projects list

# List projects for a specific org
tfccli projects list --org my-org

# Get project by ID
tfccli projects get prj-abc123

# Create a project
tfccli projects create --name my-project

# Update a project
tfccli projects update prj-abc123 --name new-name --description "Updated desc"

# Delete a project
tfccli projects delete prj-abc123 --force
```

### Workspaces

```bash
# List workspaces
tfccli workspaces list --org my-org

# Get workspace by ID
tfccli workspaces get ws-abc123

# Create a workspace
tfccli workspaces create --name my-workspace --org my-org

# Create workspace in a specific project
tfccli workspaces create --name my-workspace --org my-org --project-id prj-abc123

# Update a workspace
tfccli workspaces update ws-abc123 --name renamed-workspace

# Delete a workspace
tfccli workspaces delete ws-abc123 --force
```

### Workspace Variables

```bash
# List variables for a workspace
tfccli workspace-variables list --workspace-id ws-abc123

# Create an environment variable
tfccli workspace-variables create --workspace-id ws-abc123 \
  --key AWS_REGION --value us-east-1 --category env

# Create a sensitive terraform variable
tfccli workspace-variables create --workspace-id ws-abc123 \
  --key db_password --value secret123 --category terraform --sensitive

# Create an HCL variable
tfccli workspace-variables create --workspace-id ws-abc123 \
  --key tags --value '{"env":"prod"}' --category terraform --hcl

# Update a variable
tfccli workspace-variables update var-abc123 --value new-value

# Delete a variable
tfccli workspace-variables delete var-abc123 --force
```

### Workspace Resources

```bash
# List resources in a workspace (read-only)
tfccli workspace-resources list --workspace-id ws-abc123
```

### Runs

```bash
# List runs for a workspace
tfccli runs list --workspace-id ws-abc123

# Get run details
tfccli runs get run-abc123

# Create a run
tfccli runs create --workspace-id ws-abc123 --message "Deploying new feature"

# Create a run with auto-apply
tfccli runs create --workspace-id ws-abc123 --auto-apply --message "Auto-deploy"

# Apply a run (requires confirmation)
tfccli runs apply run-abc123
tfccli runs apply run-abc123 --force  # Skip confirmation

# Discard a run
tfccli runs discard run-abc123 --force

# Cancel a run
tfccli runs cancel run-abc123 --force

# Force-cancel a run
tfccli runs force-cancel run-abc123 --force
```

### Plans

```bash
# Get plan details
tfccli plans get plan-abc123

# Download JSON plan output
tfccli plans json-output plan-abc123
tfccli plans json-output plan-abc123 --out plan.json  # Save to file

# Download sanitized plan (HYOK feature)
tfccli plans sanitized-plan plan-abc123
tfccli plans sanitized-plan plan-abc123 --out sanitized.json
```

### Applies

```bash
# Get apply details
tfccli applies get apply-abc123

# Download errored state (for recovery)
tfccli applies errored-state apply-abc123
tfccli applies errored-state apply-abc123 --out errored.tfstate
```

### Configuration Versions

```bash
# List configuration versions
tfccli configuration-versions list --workspace-id ws-abc123

# Get configuration version details
tfccli configuration-versions get cv-abc123

# Create a new configuration version
tfccli configuration-versions create --workspace-id ws-abc123

# Create without auto-queuing runs
tfccli configuration-versions create --workspace-id ws-abc123 --auto-queue-runs=false

# Upload configuration (tar.gz)
tfccli configuration-versions upload cv-abc123 --file ./config.tar.gz

# Download configuration
tfccli configuration-versions download cv-abc123
tfccli configuration-versions download cv-abc123 --out config.tar.gz

# Archive a configuration version
tfccli configuration-versions archive cv-abc123 --force
```

### Users

```bash
# Get current authenticated user
tfccli users me

# Get user by ID
tfccli users get user-abc123
```

### Invoices (HCP Terraform Cloud only)

```bash
# List invoices
tfccli invoices list --org my-org

# Get next/upcoming invoice
tfccli invoices next --org my-org
```

## Output Formats

### JSON Output

Use `--output-format=json` or pipe output (auto-detects non-TTY):

```bash
# Explicit JSON
tfccli organizations list --output-format=json

# Piped (auto-JSON)
tfccli organizations list | jq '.data[].attributes.name'

# JSON for empty responses (204 status)
tfccli organizations delete old-org --force --output-format=json
# Output: {"meta":{"status":204}}
```

### Table Output

Default when stdout is a TTY:

```bash
tfccli organizations list
# NAME        EMAIL               EXTERNAL-ID
# my-org      admin@example.com   ext-123
# other-org   other@example.com   ext-456
```

### JSON Output Contract

JSON output follows three patterns depending on the command type:

#### 1. Data Envelope (API Resources)

Commands that fetch or modify API resources wrap results in a `data` key:

```json
// Single resource (get/create/update)
{"data": {"id": "org-abc123", "type": "organizations", "attributes": {...}}}

// List of resources
{"data": [{"id": "ws-abc123", "type": "workspaces", ...}, ...]}

// Empty list
{"data": []}

// runs get includes plan_id and apply_id for cross-referencing
{"data": {"id": "run-abc123", "status": "applied", "plan_id": "plan-xyz", "apply_id": "apply-def", ...}}
```

Applies to: `organizations`, `projects`, `workspaces`, `runs`, `plans get`, `applies get`, `configuration-versions`, `workspace-variables`, `workspace-resources`

#### 2. Meta Envelope (File Operations)

Commands that write to files (using `--out`) emit metadata about the operation:

```json
// After writing a file
{"meta": {"written_to": "/path/to/file.json", "bytes": 12345}}

// After uploading
{"meta": {"status": "uploaded", "cv_id": "cv-abc123", "bytes": 5678}}
```

Applies to: `plans json-output --out`, `plans sanitized-plan --out`, `applies errored-state --out`, `configuration-versions download --out`, `configuration-versions upload`

#### 3. Raw JSON:API (Pass-through)

Some commands pass through the raw JSON:API response from the TFC API:

```json
// users get / users me
{"data": {"id": "user-abc", "type": "users", "attributes": {...}}}

// invoices list (includes pagination)
{"data": [...], "links": {"self": "...", "next": "..."}}
```

Applies to: `users get`, `users me`, `invoices list`, `invoices next`

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
tfccli init --non-interactive --address app.terraform.io --default-org my-org --yes

# Always use --force for destructive operations
tfccli organizations delete old-org --force
tfccli runs apply run-abc123 --force

# Explicit JSON output for parsing
tfccli workspaces list --output-format=json | jq '.data[].id'

# Check exit codes
if tfccli doctor; then
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
make tfccli ARGS="version"
```

## License

See [LICENSE](LICENSE) for details.
