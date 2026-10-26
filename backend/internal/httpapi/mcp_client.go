package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"campusclaw/backend/internal/store"
)

type mcpClient struct {
	httpClient *http.Client
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type mcpDiscoveredTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func newMCPClient() *mcpClient {
	return &mcpClient{httpClient: &http.Client{Timeout: 12 * time.Second}}
}

func (client *mcpClient) Initialize(ctx context.Context, server store.MCPServer) error {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
	}
	if err := client.call(ctx, server, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "campusclaw-host", "version": "1.0.0"},
	}, &result); err != nil {
		return err
	}
	if result.ProtocolVersion == "" || result.Capabilities.Tools == nil {
		return errors.New("MCP initialize 未返回 tools 能力")
	}
	return nil
}

func (client *mcpClient) ListTools(ctx context.Context, server store.MCPServer) ([]store.MCPTool, error) {
	var result struct {
		Tools []mcpDiscoveredTool `json:"tools"`
	}
	if err := client.call(ctx, server, "tools/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	tools := make([]store.MCPTool, 0, len(result.Tools))
	seen := make(map[string]struct{}, len(result.Tools))
	for _, item := range result.Tools {
		if !validToolName(item.Name) || strings.TrimSpace(item.Description) == "" || item.InputSchema == nil {
			return nil, fmt.Errorf("工具声明不完整: %q", item.Name)
		}
		if _, duplicate := seen[item.Name]; duplicate {
			return nil, fmt.Errorf("工具名重复: %s", item.Name)
		}
		seen[item.Name] = struct{}{}
		tools = append(tools, store.MCPTool{
			ServerID:    server.ID,
			ServerName:  server.Name,
			Name:        item.Name,
			Description: item.Description,
			InputSchema: item.InputSchema,
		})
	}
	if len(tools) == 0 {
		return nil, errors.New("MCP Server 未发现任何工具")
	}
	return tools, nil
}

func (client *mcpClient) CallTool(
	ctx context.Context,
	server store.MCPServer,
	toolName string,
	arguments map[string]any,
) (string, error) {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := client.call(ctx, server, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": arguments,
	}, &result); err != nil {
		return "", err
	}
	var parts []string
	for _, item := range result.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	text := strings.Join(parts, "\n")
	if result.IsError {
		return "", fmt.Errorf("MCP 工具执行失败: %s", truncateRunes(text, 500))
	}
	if text == "" {
		return "", errors.New("MCP 工具未返回文本")
	}
	return safeToolResult(text)
}

func (client *mcpClient) call(ctx context.Context, server store.MCPServer, method string, params any, target any) error {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server.BaseURL, "/")+"/mcp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+server.AuthToken)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("MCP 请求失败: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("MCP Server 返回 %d: %s", response.StatusCode, truncateRunes(string(raw), 300))
	}
	var rpc mcpRPCResponse
	if err := json.Unmarshal(raw, &rpc); err != nil {
		return fmt.Errorf("MCP 返回格式错误: %w", err)
	}
	if rpc.Error != nil {
		return fmt.Errorf("MCP %s 错误 %d: %s", method, rpc.Error.Code, rpc.Error.Message)
	}
	if len(rpc.Result) == 0 {
		return fmt.Errorf("MCP %s 缺少 result", method)
	}
	if err := json.Unmarshal(rpc.Result, target); err != nil {
		return fmt.Errorf("解析 MCP %s 结果失败: %w", method, err)
	}
	return nil
}

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func validToolName(value string) bool {
	return toolNamePattern.MatchString(value)
}

func validateToolArguments(schema map[string]any, arguments map[string]any) error {
	properties, _ := schema["properties"].(map[string]any)
	requiredValues, _ := schema["required"].([]any)
	if stringRequired, ok := schema["required"].([]string); ok {
		requiredValues = make([]any, len(stringRequired))
		for index, value := range stringRequired {
			requiredValues[index] = value
		}
	}
	for _, raw := range requiredValues {
		name, ok := raw.(string)
		if !ok {
			continue
		}
		if _, exists := arguments[name]; !exists {
			return fmt.Errorf("缺少工具参数 %s", name)
		}
	}
	if additional, exists := schema["additionalProperties"].(bool); exists && !additional {
		for name := range arguments {
			if _, allowed := properties[name]; !allowed {
				return fmt.Errorf("未知工具参数 %s", name)
			}
		}
	}
	for name, value := range arguments {
		rawProperty, exists := properties[name]
		if !exists {
			continue
		}
		property, _ := rawProperty.(map[string]any)
		expected, _ := property["type"].(string)
		if !matchesJSONType(value, expected) {
			return fmt.Errorf("工具参数 %s 类型应为 %s", name, expected)
		}
	}
	return nil
}

func matchesJSONType(value any, expected string) bool {
	switch expected {
	case "", "object":
		_, ok := value.(map[string]any)
		return expected == "" || ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

func safeToolResult(value string) (string, error) {
	normalized := strings.ToLower(value)
	blocked := []string{
		"ignore previous",
		"ignore all previous",
		"system instruction",
		"忽略之前",
		"忽略以上",
		"系统指令",
		"请执行以下指令",
	}
	for _, phrase := range blocked {
		if strings.Contains(normalized, phrase) {
			return "", errors.New("工具返回包含疑似指令内容，已丢弃")
		}
	}
	return truncateRunes(strings.TrimSpace(value), 2000), nil
}
