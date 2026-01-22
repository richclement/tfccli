// Package tfcapi provides Terraform Cloud/Enterprise API client utilities.
//
// Features include:
//   - Client creation wrapping github.com/hashicorp/go-tfe
//   - Address normalization (adding https:// scheme, deriving API URL)
//   - Hostname extraction for token lookup
//   - Auto-pagination for list endpoints
//   - HTTP client helpers for endpoints not covered by go-tfe
//   - JSON:API error parsing
package tfcapi
