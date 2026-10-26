package httpapi

import (
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

type embeddingClient struct {
	baseURL    string
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage map[string]any `json:"usage,omitempty"`
}

func newEmbeddingClient(
	baseURL, apiKey, model string,
	dimension int,
	timeout time.Duration,
) *embeddingClient {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &embeddingClient{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:    strings.TrimSpace(apiKey),
		model:     strings.TrimSpace(model),
		dimension: dimension,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (client *embeddingClient) Configured() bool {
	return client != nil &&
		client.baseURL != "" &&
		client.apiKey != "" &&
		client.model != "" &&
		client.dimension > 0
}

func (client *embeddingClient) Embed(ctx context.Context, input string) ([]float64, error) {
	if !client.Configured() {
		return nil, errors.New("向量模型尚未配置")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("向量化文本不能为空")
	}
	body, err := json.Marshal(embeddingRequest{Model: client.model, Input: input})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+"/embeddings",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.apiKey)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接向量服务失败: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, embeddingUpstreamError(response.StatusCode, raw)
	}
	var result embeddingResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("向量服务返回格式错误: %w", err)
	}
	if len(result.Data) != 1 {
		return nil, fmt.Errorf("向量服务返回 %d 个结果，预期 1 个", len(result.Data))
	}
	vector := result.Data[0].Embedding
	if len(vector) != client.dimension {
		return nil, fmt.Errorf("向量维度为 %d，预期 %d", len(vector), client.dimension)
	}
	return vector, nil
}

func embeddingUpstreamError(status int, raw []byte) error {
	var value struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(raw, &value) == nil && value.Error != nil {
		encoded, _ := json.Marshal(value.Error)
		return fmt.Errorf("向量服务返回 %d: %s", status, truncateRunes(string(encoded), 500))
	}
	return fmt.Errorf(
		"向量服务返回 %d: %s",
		status,
		truncateRunes(strings.TrimSpace(string(raw)), 500),
	)
}
