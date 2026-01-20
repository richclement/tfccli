package tfcapi

import "testing"

func TestNormalizeAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{
			name:    "bare host becomes https URL",
			address: "app.terraform.io",
			want:    "https://app.terraform.io",
		},
		{
			name:    "host with path preserves path",
			address: "app.terraform.io/eu",
			want:    "https://app.terraform.io/eu",
		},
		{
			name:    "already has https scheme",
			address: "https://app.terraform.io",
			want:    "https://app.terraform.io",
		},
		{
			name:    "already has https scheme with path",
			address: "https://app.terraform.io/eu",
			want:    "https://app.terraform.io/eu",
		},
		{
			name:    "http scheme preserved",
			address: "http://tfe.local",
			want:    "http://tfe.local",
		},
		{
			name:    "host with port",
			address: "tfe.example.com:8443",
			want:    "https://tfe.example.com:8443",
		},
		{
			name:    "host with port and path",
			address: "tfe.example.com:8443/tfe",
			want:    "https://tfe.example.com:8443/tfe",
		},
		{
			name:    "empty address",
			address: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeAddress(tt.address)
			if got != tt.want {
				t.Errorf("NormalizeAddress(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{
			name:    "bare host gets api/v2 appended",
			address: "app.terraform.io",
			want:    "https://app.terraform.io/api/v2",
		},
		{
			name:    "host with path preserves path and appends api/v2",
			address: "app.terraform.io/eu",
			want:    "https://app.terraform.io/eu/api/v2",
		},
		{
			name:    "https URL gets api/v2 appended",
			address: "https://tfe.example.com",
			want:    "https://tfe.example.com/api/v2",
		},
		{
			name:    "trailing slash is removed before appending",
			address: "https://app.terraform.io/",
			want:    "https://app.terraform.io/api/v2",
		},
		{
			name:    "host with port and path",
			address: "tfe.example.com:8443/tfe",
			want:    "https://tfe.example.com:8443/tfe/api/v2",
		},
		{
			name:    "empty address",
			address: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := APIBaseURL(tt.address)
			if got != tt.want {
				t.Errorf("APIBaseURL(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestExtractHostFromAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
		wantErr bool
	}{
		{
			name:    "bare host",
			address: "app.terraform.io",
			want:    "app.terraform.io",
		},
		{
			name:    "host with path",
			address: "app.terraform.io/eu",
			want:    "app.terraform.io",
		},
		{
			name:    "full URL",
			address: "https://tfe.example.com",
			want:    "tfe.example.com",
		},
		{
			name:    "URL with port",
			address: "https://tfe.example.com:8443/tfe",
			want:    "tfe.example.com",
		},
		{
			name:    "empty address",
			address: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractHostFromAddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractHostFromAddress(%q) error = %v, wantErr %v", tt.address, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractHostFromAddress(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}
