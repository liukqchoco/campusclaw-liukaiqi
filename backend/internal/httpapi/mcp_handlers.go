package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	servers, err := s.store.ListMCPServers(r.Context(), user.ClassID)
	if err != nil {
		s.databaseUnavailable(w, "查询 MCP Server 失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (s *Server) handleRegisterMCPServer(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以登记 MCP Server")
		return
	}
	var request struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		Token   string `json:"token"`
	}
	if !decodeJSONRequest(w, r, 16<<10, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.BaseURL = strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	request.Token = strings.TrimSpace(request.Token)
	if !serverNamePattern.MatchString(request.Name) {
		writeError(w, http.StatusBadRequest, "服务器名必须以小写字母开头，只能包含小写字母、数字和连字符")
		return
	}
	parsed, err := url.Parse(request.BaseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		writeError(w, http.StatusBadRequest, "Base URL 必须是有效的 http 或 https 地址且不能包含用户信息")
		return
	}
	if request.Token == "" || len(request.Token) > 500 {
		writeError(w, http.StatusBadRequest, "必须提供服务端 MCP Token")
		return
	}
	id, err := s.store.RegisterMCPServer(r.Context(), user.ClassID, request.Name, request.BaseURL, request.Token)
	if err != nil {
		s.internalError(w, "登记 MCP Server 失败", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        id,
		"name":      request.Name,
		"base_url":  request.BaseURL,
		"has_token": true,
		"connected": false,
	})
}

func (s *Server) handleConnectMCPServer(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以连接 MCP Server")
		return
	}
	id, ok := positivePathID(w, r, "MCP Server 不存在")
	if !ok {
		return
	}
	server, err := s.store.GetMCPServer(r.Context(), id, user.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "MCP Server 不存在")
		return
	}
	if err != nil {
		s.databaseUnavailable(w, "查询 MCP Server 失败", err)
		return
	}
	if err := s.mcp.Initialize(r.Context(), server); err != nil {
		_ = s.store.SetMCPConnectionError(r.Context(), id, user.ClassID, truncateRunes(err.Error(), 500))
		writeError(w, http.StatusBadGateway, "MCP 握手失败："+err.Error())
		return
	}
	tools, err := s.mcp.ListTools(r.Context(), server)
	if err != nil {
		_ = s.store.SetMCPConnectionError(r.Context(), id, user.ClassID, truncateRunes(err.Error(), 500))
		writeError(w, http.StatusBadGateway, "MCP 工具发现失败："+err.Error())
		return
	}
	if err := s.store.ReplaceMCPTools(r.Context(), id, user.ClassID, tools); err != nil {
		s.databaseUnavailable(w, "保存 MCP 工具清单失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":    "握手与工具发现成功",
		"connected":  true,
		"tool_count": len(tools),
		"tools":      tools,
	})
}

func (s *Server) handleDisconnectMCPServer(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user.Role != "teacher" {
		writeError(w, http.StatusForbidden, "仅教师可以断开 MCP Server")
		return
	}
	id, ok := positivePathID(w, r, "MCP Server 不存在")
	if !ok {
		return
	}
	if err := s.store.DisconnectMCPServer(r.Context(), id, user.ClassID); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "MCP Server 不存在")
		return
	} else if err != nil {
		s.databaseUnavailable(w, "断开 MCP Server 失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "MCP Server 已断开，发现清单已清空",
		"connected": false,
	})
}

func (s *Server) handleListMCPTools(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	tools, err := s.store.ListMCPTools(r.Context(), user.ClassID)
	if err != nil {
		s.databaseUnavailable(w, "查询 MCP 工具失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (s *Server) handleListMCPLogs(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	logs, err := s.store.ListToolCallLogs(r.Context(), user)
	if err != nil {
		s.databaseUnavailable(w, "查询 MCP 调用记录失败", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}
