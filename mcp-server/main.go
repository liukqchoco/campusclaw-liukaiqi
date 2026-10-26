package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const protocolVersion = "2025-03-26"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type callParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type server struct {
	token  string
	logger *slog.Logger
	tools  []tool
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	token := strings.TrimSpace(os.Getenv("MCP_AUTH_TOKEN"))
	if token == "" {
		logger.Error("缺少 MCP_AUTH_TOKEN")
		os.Exit(1)
	}
	port := strings.TrimSpace(os.Getenv("MCP_PORT"))
	if port == "" {
		port = "8090"
	}

	api := &server{token: token, logger: logger, tools: availableTools()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /mcp", api.handleMCP)

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	logger.Info("MCP 工具服务已启动", "port", port, "tool_count", len(api.tools))
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("MCP 服务退出", "error", err)
		os.Exit(1)
	}
}

func availableTools() []tool {
	return []tool{
		{
			Name:        "formula_hint",
			Description: "当学生询问方程、算式或代数变形且需要下一步提示时使用。只返回检查方向，不给出最终答案。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{"type": "string", "description": "需要分析的算式或方程"},
				},
				"required":             []string{"expression"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "unit_convert",
			Description: "当问题需要在毫米、厘米、米、千米、克、千克之间换算时使用，返回确定性的换算结果。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{"type": "number", "description": "待换算数值"},
					"from":  map[string]any{"type": "string", "description": "原单位，如 cm"},
					"to":    map[string]any{"type": "string", "description": "目标单位，如 m"},
				},
				"required":             []string{"value", "from", "to"},
				"additionalProperties": false,
			},
		},
	}
}

func (s *server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r.Header.Get("Authorization")) {
		writeJSON(w, http.StatusUnauthorized, rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32001, Message: "未授权"},
		})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request rpcRequest
	if err := decoder.Decode(&request); err != nil || request.JSONRPC != "2.0" || len(request.ID) == 0 || request.Method == "" {
		writeJSON(w, http.StatusBadRequest, rpcResponse{
			JSONRPC: "2.0",
			ID:      request.ID,
			Error:   &rpcError{Code: -32600, Message: "无效的 JSON-RPC 请求"},
		})
		return
	}

	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "campusclaw-teaching-tools", "version": "1.0.0"},
		}
	case "tools/list":
		response.Result = map[string]any{"tools": s.tools}
	case "tools/call":
		result, err := s.callTool(request.Params)
		if err != nil {
			response.Error = &rpcError{Code: -32602, Message: err.Error()}
		} else {
			response.Result = result
		}
	default:
		response.Error = &rpcError{Code: -32601, Message: "方法不存在"}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) authorized(header string) bool {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	if len(parts[1]) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(s.token)) == 1
}

func (s *server) callTool(raw json.RawMessage) (map[string]any, error) {
	var params callParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("工具参数格式错误")
	}

	var text string
	var err error
	switch params.Name {
	case "formula_hint":
		text, err = formulaHint(params.Arguments)
	case "unit_convert":
		text, err = unitConvert(params.Arguments)
	default:
		err = fmt.Errorf("未知工具：%s", params.Name)
	}
	if err != nil {
		return nil, err
	}
	s.logger.Info("tool call", "tool", params.Name, "status", "success")
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": false,
	}, nil
}

func formulaHint(arguments map[string]any) (string, error) {
	expression, ok := arguments["expression"].(string)
	expression = strings.TrimSpace(expression)
	if !ok || expression == "" || len([]rune(expression)) > 200 {
		return "", fmt.Errorf("expression 必须是 1～200 字的字符串")
	}

	switch {
	case strings.Contains(expression, "="):
		return "这是一个方程。先检查等号两边是否同时做了相同运算，再尝试把含未知数的项与常数项分别移到等号两侧；暂不计算最终答案。", nil
	case strings.ContainsAny(expression, "/÷"):
		return "算式包含除法或分数。先检查除数是否为零；若要相加减，请先确认分母是否需要通分；暂不计算最终答案。", nil
	case strings.Contains(expression, "(") || strings.Contains(expression, "（"):
		return "算式包含括号。下一步先检查括号外的系数是否乘到了括号内每一项，并留意负号；暂不计算最终答案。", nil
	default:
		return "请先标出已知量、未知量和运算顺序，再逐步检查每次变形是否保持等价；暂不计算最终答案。", nil
	}
}

func unitConvert(arguments map[string]any) (string, error) {
	value, ok := number(arguments["value"])
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("value 必须是有限数值")
	}
	from, okFrom := arguments["from"].(string)
	to, okTo := arguments["to"].(string)
	from = normalizeUnit(from)
	to = normalizeUnit(to)
	if !okFrom || !okTo || from == "" || to == "" {
		return "", fmt.Errorf("from 和 to 必须是支持的单位")
	}

	length := map[string]float64{"mm": 0.001, "cm": 0.01, "m": 1, "km": 1000}
	mass := map[string]float64{"g": 1, "kg": 1000}
	var factors map[string]float64
	if _, exists := length[from]; exists {
		factors = length
	} else {
		factors = mass
	}
	fromFactor, fromExists := factors[from]
	toFactor, toExists := factors[to]
	if !fromExists || !toExists {
		return "", fmt.Errorf("不能在不同量纲之间换算")
	}
	result := value * fromFactor / toFactor
	return fmt.Sprintf("%s %s = %s %s", formatNumber(value), from, formatNumber(result), to), nil
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizeUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "毫米", "mm":
		return "mm"
	case "厘米", "cm":
		return "cm"
	case "米", "m":
		return "m"
	case "千米", "公里", "km":
		return "km"
	case "克", "g":
		return "g"
	case "千克", "公斤", "kg":
		return "kg"
	default:
		return ""
	}
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
