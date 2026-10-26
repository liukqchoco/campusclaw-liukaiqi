package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type qdrantClient struct {
	baseURL    string
	collection string
	dimension  int
	httpClient *http.Client
}

type qdrantPoint struct {
	ID      int64          `json:"id"`
	Vector  []float64      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type qdrantHit struct {
	ID      int64
	Score   float64
	Payload map[string]any
}

func newQdrantClient(baseURL, collection string, dimension int) *qdrantClient {
	return &qdrantClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		collection: strings.TrimSpace(collection),
		dimension:  dimension,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (client *qdrantClient) Configured() bool {
	return client != nil && client.baseURL != "" && client.collection != "" && client.dimension > 0
}

func (client *qdrantClient) EnsureCollection(ctx context.Context) error {
	if !client.Configured() {
		return errors.New("Qdrant 尚未配置")
	}
	endpoint := client.collectionEndpoint()
	response, raw, err := client.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusNotFound {
		createBody := map[string]any{
			"vectors": map[string]any{
				"size":     client.dimension,
				"distance": "Cosine",
			},
		}
		createResponse, createRaw, err := client.do(ctx, http.MethodPut, endpoint, createBody)
		if err != nil {
			return err
		}
		if createResponse.StatusCode < 200 || createResponse.StatusCode >= 300 {
			return qdrantStatusError("创建 collection", createResponse.StatusCode, createRaw)
		}
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return qdrantStatusError("读取 collection", response.StatusCode, raw)
	}
	var value struct {
		Result struct {
			Config struct {
				Params struct {
					Vectors struct {
						Size     int    `json:"size"`
						Distance string `json:"distance"`
					} `json:"vectors"`
				} `json:"params"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("解析 Qdrant collection 配置失败: %w", err)
	}
	if value.Result.Config.Params.Vectors.Size != client.dimension {
		return fmt.Errorf(
			"Qdrant collection 维度为 %d，预期 %d",
			value.Result.Config.Params.Vectors.Size,
			client.dimension,
		)
	}
	if !strings.EqualFold(value.Result.Config.Params.Vectors.Distance, "Cosine") {
		return fmt.Errorf(
			"Qdrant collection 距离为 %s，预期 Cosine",
			value.Result.Config.Params.Vectors.Distance,
		)
	}
	return nil
}

func (client *qdrantClient) UpsertPoint(ctx context.Context, point qdrantPoint) error {
	response, raw, err := client.do(
		ctx,
		http.MethodPut,
		client.collectionEndpoint()+"/points?wait=true",
		map[string]any{"points": []qdrantPoint{point}},
	)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return qdrantStatusError("写入向量", response.StatusCode, raw)
	}
	return nil
}

func (client *qdrantClient) Search(
	ctx context.Context,
	vector []float64,
	classID int64,
	materialIDs []int64,
	limit int,
	scoreThreshold float64,
) ([]qdrantHit, error) {
	if len(vector) != client.dimension {
		return nil, fmt.Errorf("查询向量维度为 %d，预期 %d", len(vector), client.dimension)
	}
	if len(materialIDs) == 0 {
		return []qdrantHit{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	body := map[string]any{
		"vector": vector,
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "class_id", "match": map[string]any{"value": classID}},
				{"key": "material_id", "match": map[string]any{"any": materialIDs}},
			},
		},
		"limit":           limit,
		"score_threshold": scoreThreshold,
		"with_payload":    true,
		"with_vector":     false,
	}
	response, raw, err := client.do(
		ctx,
		http.MethodPost,
		client.collectionEndpoint()+"/points/search",
		body,
	)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, qdrantStatusError("检索向量", response.StatusCode, raw)
	}
	var value struct {
		Result []struct {
			ID      json.RawMessage `json:"id"`
			Score   float64         `json:"score"`
			Payload map[string]any  `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("解析 Qdrant 检索结果失败: %w", err)
	}
	hits := make([]qdrantHit, 0, len(value.Result))
	for _, rawHit := range value.Result {
		id, err := parseQdrantPointID(rawHit.ID)
		if err != nil {
			return nil, err
		}
		hits = append(hits, qdrantHit{ID: id, Score: rawHit.Score, Payload: rawHit.Payload})
	}
	return hits, nil
}

func (client *qdrantClient) collectionEndpoint() string {
	return client.baseURL + "/collections/" + url.PathEscape(client.collection)
}

func (client *qdrantClient) do(
	ctx context.Context,
	method, endpoint string,
	body any,
) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("Qdrant 请求失败: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	return response, raw, nil
}

func parseQdrantPointID(raw json.RawMessage) (int64, error) {
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		number, err := strconv.ParseInt(text, 10, 64)
		if err == nil {
			return number, nil
		}
	}
	return 0, fmt.Errorf("Qdrant 返回非法 point ID: %s", truncateRunes(string(raw), 100))
}

func qdrantStatusError(operation string, status int, raw []byte) error {
	return fmt.Errorf(
		"Qdrant %s返回 %d: %s",
		operation,
		status,
		truncateRunes(strings.TrimSpace(string(raw)), 500),
	)
}
