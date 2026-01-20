package output

// Format represents the output format.
type Format string

const (
	FormatJSON  Format = "json"
	FormatTable Format = "table"
)

// ResolveOutputFormat determines the effective output format.
// If flagValue is explicitly set ("json" or "table"), it takes precedence.
// Otherwise, defaults to "json" when stdout is not a TTY, "table" when it is.
func ResolveOutputFormat(flagValue string, isTTY bool) Format {
	if flagValue == "json" {
		return FormatJSON
	}
	if flagValue == "table" {
		return FormatTable
	}
	// Default based on TTY
	if isTTY {
		return FormatTable
	}
	return FormatJSON
}
