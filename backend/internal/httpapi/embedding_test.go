package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmbeddingClientSendsOnlyModelAndInputAndChecksDimension(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer embedding-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		if len(raw) != 2 || raw["model"] != "course-embedding" || raw["input"] != "测试文本" {
			t.Fatalf("request = %#v", raw)
		}
		if _, exists := raw["dimensions"]; exists {
			t.Fatal("EMBEDDING_DIM was incorrectly sent as an API parameter")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"model": "course-embedding",
			"data": []map[string]any{{
				"index":     0,
				"embedding": []float64{0.1, 0.2, 0.3},
			}},
		})
	}))
	defer upstream.Close()

	client := newEmbeddingClient(
		upstream.URL,
		"embedding-key",
		"course-embedding",
		3,
		time.Second,
	)
	vector, err := client.Embed(context.Background(), "测试文本")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 3 || vector[2] != 0.3 {
		t.Fatalf("vector = %#v", vector)
	}
}

func TestEmbeddingClientRejectsUnexpectedDimension(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{
				"index":     0,
				"embedding": []float64{0.1, 0.2},
			}},
		})
	}))
	defer upstream.Close()

	client := newEmbeddingClient(upstream.URL, "key", "model", 3, time.Second)
	if _, err := client.Embed(context.Background(), "文本"); err == nil {
		t.Fatal("unexpected embedding dimension was accepted")
	}
}
