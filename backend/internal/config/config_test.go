package config

import (
	"strings"
	"testing"
)

func setValidEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"API_PORT":               "8081",
		"DB_HOST":                "db",
		"DB_PORT":                "3306",
		"DB_NAME":                "campusclaw",
		"DB_USER":                "campusclaw",
		"DB_PASSWORD":            "database-password",
		"SESSION_SECRET":         "0123456789abcdef0123456789abcdef",
		"SESSION_TTL_HOURS":      "24",
		"JWT_SECRET":             "abcdef0123456789abcdef0123456789",
		"JWT_TTL_MINUTES":        "60",
		"UPLOAD_DIR":             "/tmp/uploads",
		"MAX_UPLOAD_BYTES":       "1048576",
		"SEED_TEACHER_PASSWORD":  "TeacherPassword",
		"SEED_STUDENT_PASSWORD":  "StudentPassword",
		"LOGIN_MAX_FAILURES":     "5",
		"LOGIN_LOCK_MINUTES":     "10",
		"EMBEDDING_BASE_URL":     "https://gateway.example/v1",
		"EMBEDDING_API_KEY":      "test-embedding-key",
		"EMBEDDING_MODEL":        "course-embedding",
		"EMBEDDING_DIM":          "2048",
		"QDRANT_URL":             "http://qdrant:6333",
		"QDRANT_COLLECTION":      "campusclaw_chunks",
		"VECTOR_SCORE_THRESHOLD": "0.35",
		"RRF_K":                  "60",
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
	if cfg.APIPort != "8081" || cfg.MaxUploadBytes != 1048576 || cfg.JWTTTL.Minutes() != 60 {
		t.Fatalf("Load() returned unexpected configuration: %+v", cfg)
	}
	if cfg.EmbeddingDim != 2048 || cfg.VectorThreshold != 0.35 || cfg.RRFK != 60 {
		t.Fatalf("Load() returned unexpected retrieval configuration: %+v", cfg)
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

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("JWT_SECRET", "too-short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("Load() error = %v, want JWT_SECRET length error", err)
	}
}

func TestLoadRejectsInvalidVectorThreshold(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("VECTOR_SCORE_THRESHOLD", "1.2")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "VECTOR_SCORE_THRESHOLD") {
		t.Fatalf("Load() error = %v, want vector threshold error", err)
	}
}
