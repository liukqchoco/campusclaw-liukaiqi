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
	CookieSecure      bool
	UploadDir         string
	MaxUploadBytes    int64
	TeacherPassword   string
	StudentPassword   string
	LoginMaxFailures  int
	LoginLockDuration time.Duration
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
	if len(values["SEED_TEACHER_PASSWORD"]) < 8 || len(values["SEED_STUDENT_PASSWORD"]) < 8 {
		return Config{}, fmt.Errorf("预置账号口令至少需要 8 个字符")
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
		CookieSecure:      cookieSecure,
		UploadDir:         values["UPLOAD_DIR"],
		MaxUploadBytes:    maxUploadBytes,
		TeacherPassword:   values["SEED_TEACHER_PASSWORD"],
		StudentPassword:   values["SEED_STUDENT_PASSWORD"],
		LoginMaxFailures:  loginMaxFailures,
		LoginLockDuration: time.Duration(lockMinutes) * time.Minute,
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
