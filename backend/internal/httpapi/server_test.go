package httpapi

import (
	"strings"
	"testing"
	"time"

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

func TestAccessTokenRoundTrip(t *testing.T) {
	server := &Server{config: config.Config{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		JWTTTL:    time.Hour,
	}}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	token, err := server.issueAccessToken(42, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "eyJ") || strings.Count(token, ".") != 2 {
		t.Fatalf("issueAccessToken() = %q, want compact JWT", token)
	}
	userID, err := server.verifyAccessToken(token, now.Add(time.Minute))
	if err != nil || userID != 42 {
		t.Fatalf("verifyAccessToken() = (%d, %v), want (42, nil)", userID, err)
	}
}

func TestAccessTokenRejectsTamperingAndExpiry(t *testing.T) {
	server := &Server{config: config.Config{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		JWTTTL:    time.Hour,
	}}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	token, err := server.issueAccessToken(7, now)
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(token, ".")
	replacement := "A"
	if strings.HasPrefix(parts[1], replacement) {
		replacement = "B"
	}
	tampered := parts[0] + "." + replacement + parts[1][1:] + "." + parts[2]
	if _, err := server.verifyAccessToken(tampered, now); err == nil {
		t.Fatal("verifyAccessToken() accepted tampered token")
	}
	if _, err := server.verifyAccessToken(token, now.Add(2*time.Hour)); err == nil {
		t.Fatal("verifyAccessToken() accepted expired token")
	}
}

func TestAccessTokenRejectsWrongSecret(t *testing.T) {
	issuer := &Server{config: config.Config{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		JWTTTL:    time.Hour,
	}}
	verifier := &Server{config: config.Config{
		JWTSecret: "abcdef0123456789abcdef0123456789",
		JWTTTL:    time.Hour,
	}}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	token, err := issuer.issueAccessToken(7, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.verifyAccessToken(token, now); err == nil {
		t.Fatal("verifyAccessToken() accepted token signed with another secret")
	}
}
