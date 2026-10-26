package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQdrantCollectionUpsertAndScopedSearch(t *testing.T) {
	var upsertBody map[string]any
	var searchBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/campusclaw_chunks":
			writeJSON(w, http.StatusOK, map[string]any{
				"result": map[string]any{
					"config": map[string]any{
						"params": map[string]any{
							"vectors": map[string]any{"size": 3, "distance": "Cosine"},
						},
					},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/collections/campusclaw_chunks/points":
			if err := json.NewDecoder(r.Body).Decode(&upsertBody); err != nil {
				t.Fatal(err)
			}
			writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"status": "completed"}})
		case r.Method == http.MethodPost && r.URL.Path == "/collections/campusclaw_chunks/points/search":
			if err := json.NewDecoder(r.Body).Decode(&searchBody); err != nil {
				t.Fatal(err)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"result": []map[string]any{{
					"id":    42,
					"score": 0.82,
					"payload": map[string]any{
						"class_id":           1,
						"material_id":        8,
						"knowledge_entry_id": 9,
						"chunk_id":           42,
						"chunk_index":        0,
					},
				}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer upstream.Close()

	client := newQdrantClient(upstream.URL, "campusclaw_chunks", 3)
	if err := client.EnsureCollection(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.UpsertPoint(context.Background(), qdrantPoint{
		ID:     42,
		Vector: []float64{0.1, 0.2, 0.3},
		Payload: map[string]any{
			"class_id":           int64(1),
			"material_id":        int64(8),
			"knowledge_entry_id": int64(9),
			"chunk_id":           int64(42),
			"chunk_index":        0,
		},
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := client.Search(
		context.Background(),
		[]float64{0.1, 0.2, 0.3},
		1,
		[]int64{8},
		10,
		0.35,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != 42 || hits[0].Score != 0.82 {
		t.Fatalf("hits = %#v", hits)
	}

	rawUpsert, _ := json.Marshal(upsertBody)
	if string(rawUpsert) == "" || containsJSONKey(upsertBody, "chunk_text") {
		t.Fatalf("Qdrant payload contains source text: %s", rawUpsert)
	}
	if searchBody["score_threshold"] != 0.35 {
		t.Fatalf("score threshold = %#v", searchBody["score_threshold"])
	}
	filter, ok := searchBody["filter"].(map[string]any)
	if !ok || len(filter["must"].([]any)) != 2 {
		t.Fatalf("search filter = %#v", searchBody["filter"])
	}
}

func TestQdrantRejectsCollectionDimensionMismatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"result": map[string]any{
				"config": map[string]any{
					"params": map[string]any{
						"vectors": map[string]any{"size": 2, "distance": "Cosine"},
					},
				},
			},
		})
	}))
	defer upstream.Close()

	client := newQdrantClient(upstream.URL, "campusclaw_chunks", 3)
	if err := client.EnsureCollection(context.Background()); err == nil {
		t.Fatal("dimension mismatch was accepted")
	}
}

func containsJSONKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			if name == key || containsJSONKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsJSONKey(child, key) {
				return true
			}
		}
	}
	return false
}
