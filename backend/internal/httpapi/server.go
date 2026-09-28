package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "campusclaw_session"

type contextKey string

const userContextKey contextKey = "user"

type Server struct {
	config    config.Config
	store     *store.Store
	logger    *slog.Logger
	dummyHash []byte
	handler   http.Handler
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func New(cfg config.Config, dataStore *store.Store, logger *slog.Logger) (*Server, error) {
	dummyHash, err := bcrypt.GenerateFromPassword([]byte("campusclaw-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	server := &Server{
		config:    cfg,
		store:     dataStore,
		logger:    logger,
		dummyHash: dummyHash,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.handleHealth)
	mux.HandleFunc("POST /api/login", server.handleLogin)
	mux.Handle("POST /api/logout", server.requireAuth(http.HandlerFunc(server.handleLogout)))
	mux.Handle("GET /api/me", server.requireAuth(http.HandlerFunc(server.handleMe)))
	mux.Handle("GET /api/materials", server.requireAuth(http.HandlerFunc(server.handleListMaterials)))
	mux.Handle("POST /api/materials", server.requireAuth(http.HandlerFunc(server.handleUploadMaterial)))
	mux.Handle("GET /api/materials/{id}", server.requireAuth(http.HandlerFunc(server.handleGetMaterial)))
	mux.Handle("GET /api/materials/{id}/file", server.requireAuth(http.HandlerFunc(server.handleDownloadMaterial)))

	server.handler = server.withRecovery(server.withSecurityHeaders(server.withRequestLog(mux)))
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request loginRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	request.Username = strings.ToLower(strings.TrimSpace(request.Username))
	if request.Username == "" || request.Password == "" {
		writeError(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}

	ip := clientIP(r)
	allowed, err := s.store.LoginAllowed(r.Context(), request.Username, ip, time.Now())
	if err != nil {
		s.databaseUnavailable(w, "检查登录限流失败", err)
		return
	}
	if !allowed {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(request.Password))
		s.loginFailed(w)
		return
	}

	user, err := s.store.FindUserByUsername(r.Context(), request.Username)
	hash := s.dummyHash
	userExists := err == nil
	if userExists {
		hash = []byte(user.PasswordHash)
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.databaseUnavailable(w, "查询用户失败", err)
		return
	}

	passwordMatches := bcrypt.CompareHashAndPassword(hash, []byte(request.Password)) == nil
	if !userExists || !passwordMatches {
		if err := s.store.RecordLoginFailure(
			r.Context(),
			request.Username,
			ip,
			s.config.LoginMaxFailures,
			s.config.LoginLockDuration,
			time.Now(),
		); err != nil {
			s.logger.Error("记录登录失败次数失败", "error", err)
		}
		s.loginFailed(w)
		return
	}

	token, err := randomToken(32)
	if err != nil {
		s.internalError(w, "生成会话失败", err)
		return
	}
	expiresAt := time.Now().Add(s.config.SessionTTL)
	if err := s.store.CreateSession(r.Context(), s.sessionHash(token), user.ID, expiresAt); err != nil {
		s.databaseUnavailable(w, "创建会话失败", err)
		return
	}
	if err := s.store.ClearLoginFailures(r.Context(), request.Username, ip); err != nil {
		s.logger.Warn("清除登录失败记录失败", "error", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.config.SessionTTL.Seconds()),
		Expires:  expiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if err := s.store.DeleteSession(r.Context(), s.sessionHash(cookie.Value)); err != nil {
			s.databaseUnavailable(w, "删除会话失败", err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "已退出登录"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(userFromContext(r.Context()))})
}

func (s *Server) handleListMaterials(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	materials, err := s.store.ListMaterials(r.Context(), user.ClassID, r.URL.Query().Get("q"))
	if err != nil {
		s.databaseUnavailable(w, "查询材料列表失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": materials})
}

func (s *Server) handleGetMaterial(w http.ResponseWriter, r *http.Request) {
	id, ok := materialID(w, r)
	if !ok {
		return
	}
	user := userFromContext(r.Context())
	material, err := s.store.GetMaterial(r.Context(), id, user.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "材料不存在")
		return
	}
	if err != nil {
		s.databaseUnavailable(w, "查询材料详情失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"material": material})
}

func (s *Server) handleDownloadMaterial(w http.ResponseWriter, r *http.Request) {
	id, ok := materialID(w, r)
	if !ok {
		return
	}
	user := userFromContext(r.Context())
	material, err := s.store.GetMaterial(r.Context(), id, user.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "材料不存在")
		return
	}
	if err != nil {
		s.databaseUnavailable(w, "查询材料文件失败", err)
		return
	}
	if material.StoredName == "" || filepath.Base(material.StoredName) != material.StoredName {
		writeError(w, http.StatusNotFound, "材料不存在")
		return
	}

	file, err := os.Open(filepath.Join(s.config.UploadDir, material.StoredName))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "材料不存在")
		return
	}
	if err != nil {
		s.internalError(w, "读取材料文件失败", err)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		s.internalError(w, "读取材料信息失败", err)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": material.OriginalName})
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Type", material.MediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, material.OriginalName, info.ModTime(), file)
}

func (s *Server) handleUploadMaterial(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "无权上传材料")
		return
	}

	maxRequestBytes := s.config.MaxUploadBytes + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(maxRequestBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
			return
		}
		writeError(w, http.StatusBadRequest, "上传请求格式错误")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择文件")
		return
	}
	defer file.Close()

	originalName := safeClientFilename(header.Filename)
	extension := strings.ToLower(filepath.Ext(originalName))
	if originalName == "" || (extension != ".txt" && extension != ".md") {
		writeError(w, http.StatusBadRequest, "仅支持 .txt 和 .md 文件")
		return
	}

	content, err := io.ReadAll(io.LimitReader(file, s.config.MaxUploadBytes+1))
	if err != nil {
		s.internalError(w, "读取上传文件失败", err)
		return
	}
	if int64(len(content)) > s.config.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
		return
	}
	if len(content) == 0 || strings.TrimSpace(string(content)) == "" || !utf8.Valid(content) {
		writeError(w, http.StatusBadRequest, "文件必须是非空 UTF-8 文本")
		return
	}

	storageToken, err := randomToken(18)
	if err != nil {
		s.internalError(w, "生成存储文件名失败", err)
		return
	}
	storedName := storageToken + extension
	if err := os.MkdirAll(s.config.UploadDir, 0o750); err != nil {
		s.internalError(w, "创建上传目录失败", err)
		return
	}
	fullPath := filepath.Join(s.config.UploadDir, storedName)
	if err := os.WriteFile(fullPath, content, 0o640); err != nil {
		s.internalError(w, "保存上传文件失败", err)
		return
	}

	mediaType := "text/plain; charset=utf-8"
	if extension == ".md" {
		mediaType = "text/markdown; charset=utf-8"
	}
	title := strings.TrimSpace(strings.TrimSuffix(originalName, extension))
	if title == "" {
		title = originalName
	}
	id, err := s.store.CreateMaterial(r.Context(), store.NewMaterial{
		ClassID:      user.ClassID,
		UploaderID:   user.ID,
		Title:        title,
		OriginalName: originalName,
		StoredName:   storedName,
		MediaType:    mediaType,
		SizeBytes:    int64(len(content)),
		Content:      string(content),
	})
	if err != nil {
		if removeErr := os.Remove(fullPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			s.logger.Error("数据库失败后清理上传文件失败", "path", fullPath, "error", removeErr)
		}
		s.databaseUnavailable(w, "材料入库失败", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"material": map[string]any{
			"id":            id,
			"title":         title,
			"original_name": originalName,
			"class_id":      user.ClassID,
		},
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		user, err := s.store.UserForSession(r.Context(), s.sessionHash(cookie.Value))
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		if err != nil {
			s.databaseUnavailable(w, "读取会话失败", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) loginFailed(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "用户名或密码错误")
}

func (s *Server) databaseUnavailable(w http.ResponseWriter, message string, err error) {
	s.logger.Error(message, "error", err)
	writeError(w, http.StatusServiceUnavailable, "服务暂时不可用")
}

func (s *Server) internalError(w http.ResponseWriter, message string, err error) {
	s.logger.Error(message, "error", err)
	writeError(w, http.StatusInternalServerError, "服务器内部错误")
}

func (s *Server) sessionHash(token string) string {
	mac := hmac.New(sha256.New, []byte(s.config.SessionSecret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("请求处理发生 panic", "value", recovered)
				writeError(w, http.StatusInternalServerError, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func materialID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "材料不存在")
		return 0, false
	}
	return id, true
}

func randomToken(byteLength int) (string, error) {
	buffer := make([]byte, byteLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func safeClientFilename(name string) string {
	normalized := strings.ReplaceAll(name, "\\", "/")
	base := filepath.Base(normalized)
	if base == "." || base == "/" {
		return ""
	}
	return strings.TrimSpace(base)
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(forwarded) != nil {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func userFromContext(ctx context.Context) store.User {
	user, _ := ctx.Value(userContextKey).(store.User)
	return user
}

func publicUser(user store.User) map[string]any {
	return map[string]any{
		"id":           user.ID,
		"username":     user.Username,
		"display_name": user.DisplayName,
		"role":         user.Role,
		"class_id":     user.ClassID,
		"class_name":   user.ClassName,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, "encode response:", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
