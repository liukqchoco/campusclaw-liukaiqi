package httpapi

import (
	"strings"
	"testing"

	"campusclaw/backend/internal/config"
)

func TestSafeClientFilename(t *testing.T) {
	tests := map[string]string{
		"lesson.md":             "lesson.md",
		"../secret.md":          "secret.md",
		`..\windows\secret.txt`: "secret.txt",
		"  notes.txt  ":         "notes.txt",
	}
	for input, expected := range tests {
		if actual := safeClientFilename(input); actual != expected {
			t.Errorf("safeClientFilename(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestRandomTokenIsURLSafeAndUnique(t *testing.T) {
	first, err := randomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	second, err := randomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("randomToken returned duplicate values")
	}
	if strings.ContainsAny(first, "+/=") {
		t.Fatalf("randomToken(%q) is not raw URL-safe base64", first)
	}
}

func TestSessionHashDependsOnSecret(t *testing.T) {
	first := &Server{config: config.Config{SessionSecret: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	second := &Server{config: config.Config{SessionSecret: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}

	if first.sessionHash("same-token") == second.sessionHash("same-token") {
		t.Fatal("session hash must depend on SESSION_SECRET")
	}
}
