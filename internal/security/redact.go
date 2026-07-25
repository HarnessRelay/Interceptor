package security

import "strings"

const RedactedValue = "[REDACTED]"

func RedactSecret(name, value string) string {
	if value == "" {
		return value
	}
	upper := strings.ToUpper(name)
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "PASS", "API_KEY", "PRIVATE_KEY"} {
		if strings.Contains(upper, marker) {
			return RedactedValue
		}
	}
	return value
}
