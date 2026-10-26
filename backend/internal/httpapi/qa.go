package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"campusclaw/backend/internal/store"
)

type askRequest struct {
	Question string `json:"q"`
}

type askCitation struct {
	ChunkID       int64  `json:"chunk_id"`
	MaterialID    int64  `json:"material_id"`
	MaterialTitle string `json:"material_title"`
	ChunkIndex    int    `json:"chunk_index"`
	StartOffset   int    `json:"start_offset"`
	EndOffset     int    `json:"end_offset"`
	Snippet       string `json:"snippet"`
}

type knowledgeChunk struct {
	askCitation
	Text        string
	Score       int
	Similarity  float64
	RankKeyword int
	RankVector  int
	ScoreFinal  float64
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request askRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	question := strings.TrimSpace(request.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "问题不能为空")
		return
	}
	if len([]rune(question)) > 300 {
		writeError(w, http.StatusBadRequest, "问题不能超过 300 字")
		return
	}

	user := userFromContext(r.Context())
	sources, err := s.store.ListKnowledgeSources(r.Context(), user.ClassID)
	if err != nil {
		s.databaseUnavailable(w, "检索知识库失败", err)
		return
	}
	hits, _, err := s.retrieveKnowledge(r.Context(), question, sources, 4)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "混合检索依赖暂时不可用，请稍后重试")
		s.logger.Error("知识库向量检索失败", "error", err)
		return
	}
	if len(hits) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "no_evidence",
			"answer":    "资料中未找到相关内容",
			"citations": []askCitation{},
		})
		return
	}

	citations := []askCitation{hits[0].askCitation}
	answer := groundedAnswer(question, hits[0].Text) + " [1]"
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "grounded",
		"answer":    answer,
		"citations": citations,
	})
}

func retrieveKnowledge(question string, sources []store.KnowledgeSource, limit int) []knowledgeChunk {
	features := queryFeatures(question)
	if len(features.bigrams) == 0 && len(features.words) == 0 && len(features.characters) == 0 {
		return nil
	}

	hits := make([]knowledgeChunk, 0)
	for _, source := range sources {
		chunks := splitKnowledgeSource(source)
		for index := range chunks {
			score := relevanceScore(features, source.MaterialTitle+" "+chunks[index].Text)
			if score < 4 {
				continue
			}
			chunks[index].Score = score
			hits = append(hits, chunks[index])
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].MaterialID != hits[j].MaterialID {
			return hits[i].MaterialID < hits[j].MaterialID
		}
		return hits[i].ChunkIndex < hits[j].ChunkIndex
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func splitKnowledgeSource(source store.KnowledgeSource) []knowledgeChunk {
	runes := []rune(source.Content)
	chunks := make([]knowledgeChunk, 0)
	start := 0
	chunkIndex := 0

	emit := func(rawStart, rawEnd int) {
		for rawStart < rawEnd && unicode.IsSpace(runes[rawStart]) {
			rawStart++
		}
		for rawEnd > rawStart && unicode.IsSpace(runes[rawEnd-1]) {
			rawEnd--
		}
		if rawStart >= rawEnd {
			return
		}
		const maxRunes = 600
		for windowStart := rawStart; windowStart < rawEnd; {
			windowEnd := windowStart + maxRunes
			if windowEnd > rawEnd {
				windowEnd = rawEnd
			}
			text := strings.TrimSpace(string(runes[windowStart:windowEnd]))
			headingOnly := strings.HasPrefix(text, "#") && !strings.Contains(text, "\n")
			if text != "" && !headingOnly {
				chunks = append(chunks, knowledgeChunk{
					askCitation: askCitation{
						MaterialID:    source.MaterialID,
						MaterialTitle: source.MaterialTitle,
						ChunkIndex:    chunkIndex,
						StartOffset:   windowStart,
						EndOffset:     windowEnd,
						Snippet:       text,
					},
					Text: text,
				})
				chunkIndex++
			}
			windowStart = windowEnd
		}
	}

	for index := 0; index < len(runes)-1; index++ {
		if runes[index] == '\n' && runes[index+1] == '\n' {
			emit(start, index)
			start = index + 2
			index++
		}
	}
	emit(start, len(runes))
	return chunks
}

type searchFeatures struct {
	bigrams    map[string]struct{}
	words      map[string]struct{}
	characters map[rune]struct{}
}

func queryFeatures(value string) searchFeatures {
	normalized := []rune(strings.ToLower(strings.TrimSpace(value)))
	features := searchFeatures{
		bigrams:    make(map[string]struct{}),
		words:      make(map[string]struct{}),
		characters: make(map[rune]struct{}),
	}
	stopCharacters := map[rune]struct{}{
		'的': {}, '了': {}, '吗': {}, '呢': {}, '是': {}, '有': {}, '没': {}, '能': {},
		'不': {}, '可': {}, '以': {}, '如': {}, '何': {}, '什': {}, '么': {}, '在': {},
		'和': {}, '与': {}, '及': {}, '把': {}, '被': {}, '会': {}, '？': {}, '?': {},
	}

	chineseRun := make([]rune, 0)
	flushChinese := func() {
		for index, character := range chineseRun {
			if _, stopped := stopCharacters[character]; !stopped {
				features.characters[character] = struct{}{}
			}
			if index+1 < len(chineseRun) {
				features.bigrams[string(chineseRun[index:index+2])] = struct{}{}
			}
		}
		chineseRun = chineseRun[:0]
	}
	word := make([]rune, 0)
	flushWord := func() {
		if len(word) >= 2 {
			features.words[string(word)] = struct{}{}
		}
		word = word[:0]
	}

	for _, character := range normalized {
		switch {
		case unicode.Is(unicode.Han, character):
			flushWord()
			chineseRun = append(chineseRun, character)
		case unicode.IsLetter(character) || unicode.IsDigit(character):
			flushChinese()
			word = append(word, character)
		default:
			flushChinese()
			flushWord()
		}
	}
	flushChinese()
	flushWord()
	return features
}

func relevanceScore(features searchFeatures, text string) int {
	normalized := strings.ToLower(text)
	score := 0
	for bigram := range features.bigrams {
		if strings.Contains(normalized, bigram) {
			score += 4
		}
	}
	for word := range features.words {
		if strings.Contains(normalized, word) {
			score += 4
		}
	}
	for character := range features.characters {
		if strings.ContainsRune(normalized, character) {
			score++
		}
	}
	return score
}

func groundedAnswer(question, chunk string) string {
	candidates := splitSentences(chunk)
	if len(candidates) == 0 {
		return strings.TrimSpace(chunk)
	}
	features := queryFeatures(question)
	best := candidates[0]
	bestScore := relevanceScore(features, best)
	for _, candidate := range candidates[1:] {
		if score := relevanceScore(features, candidate); score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return strings.TrimSpace(strings.TrimLeft(best, "# "))
}

func splitSentences(text string) []string {
	runes := []rune(text)
	sentences := make([]string, 0)
	start := 0
	for index, character := range runes {
		if character != '。' && character != '！' && character != '？' && character != '!' && character != '?' {
			continue
		}
		if sentence := strings.TrimSpace(string(runes[start : index+1])); sentence != "" {
			sentences = append(sentences, sentence)
		}
		start = index + 1
	}
	if trailing := strings.TrimSpace(string(runes[start:])); trailing != "" {
		sentences = append(sentences, trailing)
	}
	return sentences
}
