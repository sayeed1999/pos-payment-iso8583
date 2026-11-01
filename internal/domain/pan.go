package domain

import "strings"

// MaskPAN keeps the first 6 and last 4 digits.
func MaskPAN(p string) string {
	if len(p) <= 10 {
		return strings.Repeat("*", len(p))
	}
	return p[:6] + strings.Repeat("*", len(p)-10) + p[len(p)-4:]
}
