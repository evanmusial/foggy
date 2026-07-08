package foggy

import (
	"regexp"
	"testing"
)

func TestNewBackupCodeFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Z2-7]{4}-[A-Z2-7]{4}-[A-Z2-7]{4}-[A-Z2-7]{4}$`)
	for range 20 {
		code, err := newBackupCode()
		if err != nil {
			t.Fatalf("newBackupCode() error = %v", err)
		}
		if !pattern.MatchString(code) {
			t.Fatalf("newBackupCode() = %q, want XXXX-XXXX-XXXX-XXXX base32 format", code)
		}
		if normalizeCode(stringsWithoutHyphens(code)) != code {
			t.Fatalf("normalizeCode() did not restore hyphenated form for %q", code)
		}
	}
}

func stringsWithoutHyphens(value string) string {
	out := make([]byte, 0, len(value))
	for i := range len(value) {
		if value[i] != '-' {
			out = append(out, value[i])
		}
	}
	return string(out)
}
