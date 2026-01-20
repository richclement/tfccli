package tfcapi

import (
	"net/url"
	"strings"
)

// NormalizeAddress ensures the address has an https:// scheme.
// Accepts formats like:
//   - app.terraform.io
//   - https://app.terraform.io
//   - app.terraform.io/eu
//   - https://tfe.example.com:8443/path
//
// Returns the address with https:// scheme prepended if missing.
func NormalizeAddress(address string) string {
	if address == "" {
		return ""
	}

	// If scheme already present, return as-is
	if strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "http://") {
		return address
	}

	// Add https:// scheme
	return "https://" + address
}

// APIBaseURL returns the API base URL for a given address.
// Appends /api/v2 to the normalized address.
func APIBaseURL(address string) string {
	normalized := NormalizeAddress(address)
	if normalized == "" {
		return ""
	}

	// Remove trailing slash if present
	normalized = strings.TrimSuffix(normalized, "/")

	return normalized + "/api/v2"
}

// ExtractHostFromAddress returns the hostname from an address for token lookup.
// This differs from NormalizeAddress - it only returns the host portion.
func ExtractHostFromAddress(address string) (string, error) {
	normalized := NormalizeAddress(address)
	if normalized == "" {
		return "", nil
	}

	u, err := url.Parse(normalized)
	if err != nil {
		return "", err
	}

	return u.Hostname(), nil
}
