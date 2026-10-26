package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type llmClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type llmMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []llmToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type llmTool struct {
	Type     string          `json:"type"`
	Function llmToolFunction `json:"function"`
}

type llmToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type llmToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function llmToolCallFunction `json:"function"`
}

type llmToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type llmRequest struct {
	Model       string       `json:"model"`
	Messages    []llmMessage `json:"messages"`
	Tools       []llmTool    `json:"tools,omitempty"`
	ToolChoice  any          `json:"tool_choice,omitempty"`
	Temperature float64      `json:"temperature"`
	Stream      bool         `json:"stream"`
}

type llmResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string        `json:"role"`
			Content   string        `json:"content"`
			ToolCalls []llmToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage map[string]any `json:"usage,omitempty"`
}

type llmStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func newLLMClient(baseURL, apiKey, model string, timeout time.Duration) *llmClient {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &llmClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (client *llmClient) Configured() bool {
	return client != nil && client.baseURL != "" && client.apiKey != "" && client.model != ""
}

func (client *llmClient) Complete(ctx context.Context, request llmRequest) (llmResponse, error) {
	request.Model = client.model
	request.Stream = false
	body, err := json.Marshal(request)
	if err != nil {
		return llmResponse{}, err
	}
	response, err := client.do(ctx, body)
	if err != nil {
		return llmResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return llmResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return llmResponse{}, upstreamError(response.StatusCode, raw)
	}
	var result llmResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return llmResponse{}, fmt.Errorf("LLM 返回格式错误: %w", err)
	}
	if len(result.Choices) == 0 {
		return llmResponse{}, errors.New("LLM 未返回候选结果")
	}
	return result, nil
}

func (client *llmClient) Stream(
	ctx context.Context,
	request llmRequest,
	onDelta func(string) error,
) (map[string]any, error) {
	request.Model = client.model
	request.Stream = true
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	response, err := client.do(ctx, body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		if readErr != nil {
			return nil, readErr
		}
		return nil, upstreamError(response.StatusCode, raw)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 16<<10), 2<<20)
	var model string
	var responseID string
	var finishReason string
	chunkCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk llmStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("解析 LLM 流式分片失败: %w", err)
		}
		if responseID == "" {
			responseID = chunk.ID
			model = chunk.Model
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				chunkCount++
				if err := onDelta(choice.Delta.Content); err != nil {
					return nil, err
				}
			}
			if choice.FinishReason != nil {
				finishReason = *choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 LLM 流式响应失败: %w", err)
	}
	return map[string]any{
		"id":            responseID,
		"model":         model,
		"finish_reason": finishReason,
		"delta_count":   chunkCount,
	}, nil
}

func (client *llmClient) do(ctx context.Context, body []byte) (*http.Response, error) {
	endpoint := client.baseURL + "/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接 LLM 服务失败: %w", err)
	}
	return response, nil
}

func upstreamError(status int, raw []byte) error {
	var value struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(raw, &value) == nil && value.Error != nil {
		encoded, _ := json.Marshal(value.Error)
		return fmt.Errorf("LLM 服务返回 %d: %s", status, truncateRunes(string(encoded), 500))
	}
	return fmt.Errorf("LLM 服务返回 %d: %s", status, truncateRunes(strings.TrimSpace(string(raw)), 500))
}

func requestSnapshot(request llmRequest) map[string]any {
	raw, _ := json.Marshal(request)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}

func responseSnapshot(response llmResponse) map[string]any {
	raw, _ := json.Marshal(response)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}
