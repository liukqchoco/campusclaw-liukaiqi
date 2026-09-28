package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"API_PORT":              "8081",
		"DB_HOST":               "db",
		"DB_PORT":               "3306",
		"DB_NAME":               "campusclaw",
		"DB_USER":               "campusclaw",
		"DB_PASSWORD":           "database-password",
		"SESSION_SECRET":        "0123456789abcdef0123456789abcdef",
		"SESSION_TTL_HOURS":     "24",
		"UPLOAD_DIR":            "/tmp/uploads",
		"MAX_UPLOAD_BYTES":      "1048576",
		"SEED_TEACHER_PASSWORD": "TeacherPassword",
		"SEED_STUDENT_PASSWORD": "StudentPassword",
		"LOGIN_MAX_FAILURES":    "5",
		"LOGIN_LOCK_MINUTES":    "10",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func TestLoadValidConfiguration(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("COOKIE_SECURE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIPort != "8081" || cfg.MaxUploadBytes != 1048576 {
		t.Fatalf("Load() returned unexpected configuration: %+v", cfg)
	}
}

func TestLoadRejectsMissingSessionSecret(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("SESSION_SECRET", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("Load() error = %v, want missing SESSION_SECRET", err)
	}
}

func TestLoadRejectsShortSessionSecret(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("SESSION_SECRET", "too-short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "32") {
		t.Fatalf("Load() error = %v, want minimum length error", err)
	}
}
