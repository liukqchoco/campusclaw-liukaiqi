package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"campusclaw/backend/internal/store"
)

type appRequest struct {
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	PromptTemplateID int64   `json:"prompt_template_id"`
	Enabled          bool    `json:"enabled"`
	MaterialIDs      []int64 `json:"material_ids"`
	SkillIDs         []int64 `json:"skill_ids"`
}

func (s *Server) handlePlatformStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"llm_configured":       s.llm.Configured(),
		"llm_model":            s.config.LLMModel,
		"embedding_configured": s.embedding.Configured(),
		"embedding_model":      s.config.EmbeddingModel,
		"embedding_dim":        s.config.EmbeddingDim,
		"qdrant_configured":    s.qdrant.Configured(),
		"qdrant_collection":    s.config.QdrantCollection,
		"retrieval_mode":       "hybrid_rrf",
		"vector_threshold":     s.config.VectorThreshold,
		"rrf_k":                s.config.RRFK,
		"mcp_configured":       s.config.MCPBaseURL != "" && s.config.MCPAuthToken != "",
		"trace_enabled":        true,
	})
}

func (s *Server) handleListPromptTemplates(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	templates, err := s.store.ListPromptTemplates(r.Context(), user.ClassID)
	if err != nil {
		s.databaseUnavailable(w, "查询提示词模板失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

func (s *Server) handleUpdatePromptTemplate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以修改提示词模板")
		return
	}
	id, ok := positivePathID(w, r, "模板不存在")
	if !ok {
		return
	}
	var request struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if !decodeJSONRequest(w, r, 16<<10, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Content = strings.TrimSpace(request.Content)
	if request.Name == "" || len([]rune(request.Name)) > 64 {
		writeError(w, http.StatusBadRequest, "模板名称必须为 1～64 字")
		return
	}
	if request.Content == "" || len([]rune(request.Content)) > 2000 {
		writeError(w, http.StatusBadRequest, "提示词模板必须为 1～2000 字")
		return
	}
	if err := s.store.UpdatePromptTemplate(r.Context(), id, user.ClassID, request.Name, request.Content); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "模板不存在")
		return
	} else if err != nil {
		s.databaseUnavailable(w, "更新提示词模板失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "提示词模板已更新，下次对话立即生效"})
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	apps, err := s.store.ListAIApps(r.Context(), user.ClassID, user.Role == "teacher")
	if err != nil {
		s.databaseUnavailable(w, "查询 AI 应用失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": apps})
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以创建 AI 应用")
		return
	}
	var request appRequest
	if !decodeJSONRequest(w, r, 32<<10, &request) || !validateAppRequest(w, request) {
		return
	}
	id, err := s.store.CreateAIApp(r.Context(), user.ClassID, store.AIAppInput{
		Name:             strings.TrimSpace(request.Name),
		Description:      strings.TrimSpace(request.Description),
		PromptTemplateID: request.PromptTemplateID,
		Enabled:          request.Enabled,
		MaterialIDs:      request.MaterialIDs,
		SkillIDs:         request.SkillIDs,
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "模板、材料或 Skill 不存在")
		return
	}
	if err != nil {
		s.internalError(w, "创建 AI 应用失败", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以修改 AI 应用")
		return
	}
	id, ok := positivePathID(w, r, "AI 应用不存在")
	if !ok {
		return
	}
	var request appRequest
	if !decodeJSONRequest(w, r, 32<<10, &request) || !validateAppRequest(w, request) {
		return
	}
	err := s.store.UpdateAIApp(r.Context(), id, user.ClassID, store.AIAppInput{
		Name:             strings.TrimSpace(request.Name),
		Description:      strings.TrimSpace(request.Description),
		PromptTemplateID: request.PromptTemplateID,
		Enabled:          request.Enabled,
		MaterialIDs:      request.MaterialIDs,
		SkillIDs:         request.SkillIDs,
	})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "AI 应用、模板、材料或 Skill 不存在")
		return
	}
	if err != nil {
		s.internalError(w, "更新 AI 应用失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "AI 应用已更新，下次对话立即生效"})
}

func validateAppRequest(w http.ResponseWriter, request appRequest) bool {
	name := strings.TrimSpace(request.Name)
	description := strings.TrimSpace(request.Description)
	if name == "" || len([]rune(name)) > 64 {
		writeError(w, http.StatusBadRequest, "应用名称必须为 1～64 字")
		return false
	}
	if description == "" || len([]rune(description)) > 500 {
		writeError(w, http.StatusBadRequest, "应用说明必须为 1～500 字")
		return false
	}
	if request.PromptTemplateID <= 0 {
		writeError(w, http.StatusBadRequest, "必须选择提示词模板")
		return false
	}
	return true
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	skills, err := s.store.ListSkills(r.Context(), user.ClassID)
	if err != nil {
		s.databaseUnavailable(w, "查询 Skill 失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

func (s *Server) handleUploadSkill(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以上传 Skill")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseMultipartForm(64 << 10); err != nil {
		writeError(w, http.StatusBadRequest, "Skill 上传请求格式错误")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择 Skill 文件")
		return
	}
	defer file.Close()
	extension := strings.ToLower(filepath.Ext(header.Filename))
	if extension != ".txt" && extension != ".md" {
		writeError(w, http.StatusBadRequest, "Skill 只支持 .txt 或 .md 文件")
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, 32<<10))
	if err != nil {
		s.internalError(w, "读取 Skill 文件失败", err)
		return
	}
	if !utf8.Valid(content) {
		writeError(w, http.StatusBadRequest, "Skill 文件必须是 UTF-8 文本")
		return
	}
	skill, err := parseSkillFile(string(content))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	externalRefs := make([]string, 0)
	for _, tool := range skill.Tools {
		if tool != "knowledge_search" {
			externalRefs = append(externalRefs, tool)
		}
	}
	resolved, err := s.store.ResolveMCPToolRefs(r.Context(), user.ClassID, externalRefs)
	if err != nil {
		s.databaseUnavailable(w, "校验 Skill 工具失败", err)
		return
	}
	for _, ref := range externalRefs {
		if _, exists := resolved[ref]; !exists {
			writeError(w, http.StatusBadRequest, "Skill 引用的 MCP 工具尚未连接或不存在："+ref)
			return
		}
	}
	id, err := s.store.CreateSkill(r.Context(), user.ClassID, skill)
	if err != nil {
		s.internalError(w, "保存 Skill 失败", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "skill": skill})
}

func (s *Server) handleSetSkillEnabled(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以启停 Skill")
		return
	}
	id, ok := positivePathID(w, r, "Skill 不存在")
	if !ok {
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSONRequest(w, r, 4<<10, &request) {
		return
	}
	if err := s.store.SetSkillEnabled(r.Context(), id, user.ClassID, request.Enabled); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Skill 不存在")
		return
	} else if err != nil {
		s.databaseUnavailable(w, "更新 Skill 失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Skill 状态已更新"})
}

func (s *Server) handleListTraces(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	traces, err := s.store.ListLLMTraces(r.Context(), user)
	if err != nil {
		s.databaseUnavailable(w, "查询 LLM 轨迹失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"traces": traces})
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, ok := positivePathID(w, r, "调用轨迹不存在")
	if !ok {
		return
	}
	trace, err := s.store.GetLLMTrace(r.Context(), id, user)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "调用轨迹不存在")
		return
	}
	if err != nil {
		s.databaseUnavailable(w, "查询调用轨迹失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trace": trace})
}

func positivePathID(w http.ResponseWriter, r *http.Request, notFound string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, notFound)
		return 0, false
	}
	return id, true
}

func decodeJSONRequest(w http.ResponseWriter, r *http.Request, maxBytes int64, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "请求只能包含一个 JSON 对象")
		return false
	}
	return true
}
