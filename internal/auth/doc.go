// Package auth provides token discovery following Terraform CLI conventions.
//
// Token resolution follows this precedence:
//  1. Environment variable TF_TOKEN_<sanitized_host> (e.g., TF_TOKEN_app_terraform_io)
//  2. Terraform CLI config credentials blocks (honoring TF_CLI_CONFIG_FILE)
//  3. Terraform login credentials file (~/.terraform.d/credentials.tfrc.json)
//
// The package abstracts filesystem and environment access for testability.
package auth
