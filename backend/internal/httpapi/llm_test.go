package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLLMClientCompleteSendsOpenAICompatibleParameters(t *testing.T) {
	var captured llmRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":    "chatcmpl-test",
			"model": "test-model",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"id":   "call-1",
						"type": "function",
						"function": map[string]string{
							"name":      "unit_convert",
							"arguments": `{"value":250,"from":"cm","to":"m"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
		})
	}))
	defer upstream.Close()

	client := newLLMClient(upstream.URL, "test-key", "test-model", time.Second)
	response, err := client.Complete(context.Background(), llmRequest{
		Messages: []llmMessage{{Role: "system", Content: "模板正文"}, {Role: "user", Content: "250 cm 等于多少 m"}},
		Tools: []llmTool{{
			Type: "function",
			Function: llmToolFunction{
				Name:        "unit_convert",
				Description: "单位换算",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Model != "test-model" || captured.Stream {
		t.Fatalf("captured request = %+v", captured)
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Content != "模板正文" || len(captured.Tools) != 1 {
		t.Fatalf("captured request missing messages/tools: %+v", captured)
	}
	if len(response.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("response = %+v", response)
	}
}

func TestLLMClientStreamEmitsOnlyNewDeltas(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var captured llmRequest
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		if !captured.Stream {
			t.Fatal("stream flag was false")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"磁铁\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"x\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"不能吸木块\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	client := newLLMClient(upstream.URL, "test-key", "test-model", time.Second)
	var deltas []string
	summary, err := client.Stream(context.Background(), llmRequest{
		Messages: []llmMessage{{Role: "user", Content: "问题"}},
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deltas, "") != "磁铁不能吸木块" || len(deltas) != 2 {
		t.Fatalf("deltas = %#v", deltas)
	}
	if summary["delta_count"] != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}
