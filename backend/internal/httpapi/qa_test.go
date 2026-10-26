package httpapi

import (
	"strings"
	"testing"

	"campusclaw/backend/internal/store"
)

func TestRetrieveKnowledgeReturnsTraceableMagnetEvidence(t *testing.T) {
	sources := []store.KnowledgeSource{{
		MaterialID:    8,
		MaterialTitle: "A 班小学科学课程资料",
		Content:       "# 科学\n\n磁铁能吸引铁钉，不能吸引木块。磁铁不会吸引塑料。",
	}}

	hits := retrieveKnowledge("磁铁能吸木块吗？", sources, 4)
	if len(hits) == 0 {
		t.Fatal("retrieveKnowledge() returned no evidence")
	}
	hit := hits[0]
	if hit.MaterialID != 8 || hit.MaterialTitle != "A 班小学科学课程资料" {
		t.Fatalf("unexpected source: %+v", hit.askCitation)
	}
	if hit.StartOffset >= hit.EndOffset || !strings.Contains(hit.Snippet, "木块") {
		t.Fatalf("invalid traceability fields: %+v", hit.askCitation)
	}
	answer := groundedAnswer("磁铁能吸木块吗？", hit.Text)
	if !strings.Contains(answer, "不能吸引木块") {
		t.Fatalf("groundedAnswer() = %q", answer)
	}
}

func TestRetrieveKnowledgeReturnsNoEvidenceForWeather(t *testing.T) {
	sources := []store.KnowledgeSource{{
		MaterialID:    8,
		MaterialTitle: "A 班小学科学课程资料",
		Content:       "磁铁能吸引铁钉，不能吸引木块。水加热后会变成水蒸气。",
	}}

	if hits := retrieveKnowledge("明天的天气预报是什么？", sources, 4); len(hits) != 0 {
		t.Fatalf("retrieveKnowledge() returned weak evidence: %+v", hits)
	}
}

func TestSplitKnowledgeSourcePreservesOffsets(t *testing.T) {
	source := store.KnowledgeSource{
		MaterialID:    1,
		MaterialTitle: "讲义",
		Content:       "# 标题\n\n第一段。\n\n第二段。",
	}
	chunks := splitKnowledgeSource(source)
	if len(chunks) != 2 {
		t.Fatalf("splitKnowledgeSource() produced %d chunks, want 2 content chunks", len(chunks))
	}
	for _, chunk := range chunks {
		runes := []rune(source.Content)
		if got := string(runes[chunk.StartOffset:chunk.EndOffset]); got != chunk.Text {
			t.Fatalf("offset slice = %q, chunk text = %q", got, chunk.Text)
		}
	}
}
