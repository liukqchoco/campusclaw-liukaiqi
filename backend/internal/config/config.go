package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIPort           string
	DBHost            string
	DBPort            string
	DBName            string
	DBUser            string
	DBPassword        string
	SessionSecret     string
	SessionTTL        time.Duration
	JWTSecret         string
	JWTTTL            time.Duration
	CookieSecure      bool
	UploadDir         string
	MaxUploadBytes    int64
	TeacherPassword   string
	StudentPassword   string
	LoginMaxFailures  int
	LoginLockDuration time.Duration
	LLMBaseURL        string
	LLMAPIKey         string
	LLMModel          string
	LLMTimeout        time.Duration
	EmbeddingBaseURL  string
	EmbeddingAPIKey   string
	EmbeddingModel    string
	EmbeddingDim      int
	QdrantURL         string
	QdrantCollection  string
	VectorThreshold   float64
	RRFK              int
	MCPBaseURL        string
	MCPAuthToken      string
}

func Load() (Config, error) {
	required := []string{
		"API_PORT",
		"DB_HOST",
		"DB_PORT",
		"DB_NAME",
		"DB_USER",
		"DB_PASSWORD",
		"SESSION_SECRET",
		"SESSION_TTL_HOURS",
		"JWT_SECRET",
		"JWT_TTL_MINUTES",
		"UPLOAD_DIR",
		"MAX_UPLOAD_BYTES",
		"SEED_TEACHER_PASSWORD",
		"SEED_STUDENT_PASSWORD",
		"LOGIN_MAX_FAILURES",
		"LOGIN_LOCK_MINUTES",
	}

	values := make(map[string]string, len(required))
	var missing []string
	for _, key := range required {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			missing = append(missing, key)
			continue
		}
		values[key] = value
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("缺少必需环境变量: %s", strings.Join(missing, ", "))
	}

	sessionHours, err := positiveInt(values["SESSION_TTL_HOURS"])
	if err != nil {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS: %w", err)
	}
	jwtMinutes, err := positiveInt(values["JWT_TTL_MINUTES"])
	if err != nil {
		return Config{}, fmt.Errorf("JWT_TTL_MINUTES: %w", err)
	}
	maxUploadBytes, err := positiveInt64(values["MAX_UPLOAD_BYTES"])
	if err != nil {
		return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES: %w", err)
	}
	loginMaxFailures, err := positiveInt(values["LOGIN_MAX_FAILURES"])
	if err != nil {
		return Config{}, fmt.Errorf("LOGIN_MAX_FAILURES: %w", err)
	}
	lockMinutes, err := positiveInt(values["LOGIN_LOCK_MINUTES"])
	if err != nil {
		return Config{}, fmt.Errorf("LOGIN_LOCK_MINUTES: %w", err)
	}
	cookieSecure := false
	if raw := strings.TrimSpace(os.Getenv("COOKIE_SECURE")); raw != "" {
		cookieSecure, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE 必须为 true 或 false")
		}
	}

	if len(values["SESSION_SECRET"]) < 32 {
		return Config{}, fmt.Errorf("SESSION_SECRET 至少需要 32 个字符")
	}
	if len(values["JWT_SECRET"]) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET 至少需要 32 个字符")
	}
	if len(values["SEED_TEACHER_PASSWORD"]) < 8 || len(values["SEED_STUDENT_PASSWORD"]) < 8 {
		return Config{}, fmt.Errorf("预置账号口令至少需要 8 个字符")
	}
	llmTimeout := 90
	if raw := strings.TrimSpace(os.Getenv("LLM_TIMEOUT_SECONDS")); raw != "" {
		llmTimeout, err = positiveInt(raw)
		if err != nil {
			return Config{}, fmt.Errorf("LLM_TIMEOUT_SECONDS: %w", err)
		}
	}
	embeddingDim := 0
	if raw := strings.TrimSpace(os.Getenv("EMBEDDING_DIM")); raw != "" {
		embeddingDim, err = positiveInt(raw)
		if err != nil {
			return Config{}, fmt.Errorf("EMBEDDING_DIM: %w", err)
		}
	}
	vectorThreshold := 0.35
	if raw := strings.TrimSpace(os.Getenv("VECTOR_SCORE_THRESHOLD")); raw != "" {
		vectorThreshold, err = strconv.ParseFloat(raw, 64)
		if err != nil || vectorThreshold < 0 || vectorThreshold > 1 {
			return Config{}, fmt.Errorf("VECTOR_SCORE_THRESHOLD 必须是 0～1 的数")
		}
	}
	rrfK := 60
	if raw := strings.TrimSpace(os.Getenv("RRF_K")); raw != "" {
		rrfK, err = positiveInt(raw)
		if err != nil {
			return Config{}, fmt.Errorf("RRF_K: %w", err)
		}
	}

	return Config{
		APIPort:           values["API_PORT"],
		DBHost:            values["DB_HOST"],
		DBPort:            values["DB_PORT"],
		DBName:            values["DB_NAME"],
		DBUser:            values["DB_USER"],
		DBPassword:        values["DB_PASSWORD"],
		SessionSecret:     values["SESSION_SECRET"],
		SessionTTL:        time.Duration(sessionHours) * time.Hour,
		JWTSecret:         values["JWT_SECRET"],
		JWTTTL:            time.Duration(jwtMinutes) * time.Minute,
		CookieSecure:      cookieSecure,
		UploadDir:         values["UPLOAD_DIR"],
		MaxUploadBytes:    maxUploadBytes,
		TeacherPassword:   values["SEED_TEACHER_PASSWORD"],
		StudentPassword:   values["SEED_STUDENT_PASSWORD"],
		LoginMaxFailures:  loginMaxFailures,
		LoginLockDuration: time.Duration(lockMinutes) * time.Minute,
		LLMBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/"),
		LLMAPIKey:         strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		LLMModel:          strings.TrimSpace(os.Getenv("LLM_MODEL")),
		LLMTimeout:        time.Duration(llmTimeout) * time.Second,
		EmbeddingBaseURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")), "/"),
		EmbeddingAPIKey:   strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY")),
		EmbeddingModel:    strings.TrimSpace(os.Getenv("EMBEDDING_MODEL")),
		EmbeddingDim:      embeddingDim,
		QdrantURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("QDRANT_URL")), "/"),
		QdrantCollection:  strings.TrimSpace(os.Getenv("QDRANT_COLLECTION")),
		VectorThreshold:   vectorThreshold,
		RRFK:              rrfK,
		MCPBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("MCP_BASE_URL")), "/"),
		MCPAuthToken:      strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN")),
	}, nil
}

func positiveInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("必须是正整数")
	}
	return parsed, nil
}

func positiveInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("必须是正整数")
	}
	return parsed, nil
}
