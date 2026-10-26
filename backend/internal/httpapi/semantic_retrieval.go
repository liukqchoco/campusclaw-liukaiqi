package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"

	"campusclaw/backend/internal/store"
)

type knowledgeChunkKey struct {
	KnowledgeEntryID int64
	ChunkIndex       int
}

type indexedKnowledgeChunk struct {
	Chunk            knowledgeChunk
	KnowledgeEntryID int64
	ClassID          int64
	ContentHash      string
}

func (s *Server) retrieveKnowledge(
	ctx context.Context,
	question string,
	sources []store.KnowledgeSource,
	limit int,
) ([]knowledgeChunk, map[string]any, error) {
	if !s.embedding.Configured() || !s.qdrant.Configured() {
		hits := retrieveKnowledge(question, sources, limit)
		return hits, map[string]any{
			"method":                 "lexical_fallback",
			"embedding_configured":   s.embedding.Configured(),
			"qdrant_configured":      s.qdrant.Configured(),
			"candidate_chunks":       countKnowledgeChunks(sources),
			"knowledge_sent_to_llm":  len(hits),
			"query_vector_requested": false,
		}, nil
	}
	if len(sources) == 0 {
		return []knowledgeChunk{}, s.emptyHybridRetrievalTrace(), nil
	}

	indexed, cacheHits, indexedNow, err := s.prepareKnowledgeChunks(ctx, sources, false)
	if err != nil {
		return nil, nil, err
	}
	queryVector, err := s.embedding.Embed(ctx, question)
	if err != nil {
		return nil, nil, fmt.Errorf("生成查询向量失败: %w", err)
	}
	classID := sources[0].ClassID
	materialIDs := uniqueMaterialIDs(sources)
	vectorHits, err := s.qdrant.Search(
		ctx,
		queryVector,
		classID,
		materialIDs,
		32,
		s.config.VectorThreshold,
	)
	if err != nil {
		return nil, nil, err
	}

	byID := make(map[int64]indexedKnowledgeChunk, len(indexed))
	for _, item := range indexed {
		byID[item.Chunk.ChunkID] = item
	}
	allowedMaterials := make(map[int64]bool, len(materialIDs))
	for _, materialID := range materialIDs {
		allowedMaterials[materialID] = true
	}

	keywordRanked := make([]indexedKnowledgeChunk, 0)
	features := queryFeatures(question)
	for _, item := range indexed {
		item.Chunk.Score = relevanceScore(features, item.Chunk.MaterialTitle+" "+item.Chunk.Text)
		if item.Chunk.Score >= 4 {
			keywordRanked = append(keywordRanked, item)
		}
	}
	sort.SliceStable(keywordRanked, func(i, j int) bool {
		if keywordRanked[i].Chunk.Score != keywordRanked[j].Chunk.Score {
			return keywordRanked[i].Chunk.Score > keywordRanked[j].Chunk.Score
		}
		return keywordRanked[i].Chunk.ChunkID < keywordRanked[j].Chunk.ChunkID
	})

	fused := make(map[int64]knowledgeChunk)
	for index, item := range keywordRanked {
		chunk := item.Chunk
		chunk.RankKeyword = index + 1
		chunk.ScoreFinal += 1 / float64(s.config.RRFK+chunk.RankKeyword)
		fused[chunk.ChunkID] = chunk
	}
	validVectorCount := 0
	for _, hit := range vectorHits {
		item, exists := byID[hit.ID]
		if !exists ||
			item.ClassID != classID ||
			!allowedMaterials[item.Chunk.MaterialID] ||
			!qdrantPayloadMatches(hit.Payload, item) {
			continue
		}
		validVectorCount++
		chunk := fused[hit.ID]
		if chunk.ChunkID == 0 {
			chunk = item.Chunk
		}
		chunk.Similarity = hit.Score
		chunk.RankVector = validVectorCount
		chunk.ScoreFinal += 1 / float64(s.config.RRFK+chunk.RankVector)
		fused[hit.ID] = chunk
	}

	hits := make([]knowledgeChunk, 0, len(fused))
	for _, hit := range fused {
		hits = append(hits, hit)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].ScoreFinal != hits[j].ScoreFinal {
			return hits[i].ScoreFinal > hits[j].ScoreFinal
		}
		bestI := bestPositiveRank(hits[i].RankKeyword, hits[i].RankVector)
		bestJ := bestPositiveRank(hits[j].RankKeyword, hits[j].RankVector)
		if bestI != bestJ {
			return bestI < bestJ
		}
		return hits[i].ChunkID < hits[j].ChunkID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	return hits, map[string]any{
		"method":                 "hybrid_rrf",
		"embedding_configured":   true,
		"embedding_model":        s.config.EmbeddingModel,
		"embedding_dim":          s.config.EmbeddingDim,
		"qdrant_configured":      true,
		"qdrant_collection":      s.config.QdrantCollection,
		"vector_score_threshold": s.config.VectorThreshold,
		"rrf_k":                  s.config.RRFK,
		"candidate_chunks":       len(indexed),
		"keyword_candidates":     len(keywordRanked),
		"vector_candidates":      validVectorCount,
		"rrf_candidates":         len(fused),
		"cached_chunk_records":   cacheHits,
		"reindexed_chunks":       indexedNow,
		"query_vector_requested": true,
		"query_vector_dimension": len(queryVector),
		"query_vector_norm":      vectorNorm(queryVector),
		"query_vector_preview":   vectorPreview(queryVector, 8),
		"knowledge_sent_to_llm":  len(hits),
	}, nil
}

func (s *Server) WarmKnowledgeIndex(ctx context.Context) (int, error) {
	if !s.embedding.Configured() || !s.qdrant.Configured() {
		return 0, nil
	}
	if err := s.qdrant.EnsureCollection(ctx); err != nil {
		return 0, err
	}
	sources, err := s.store.ListAllKnowledgeSources(ctx)
	if err != nil {
		return 0, err
	}
	groups := make(map[int64][]store.KnowledgeSource)
	for _, source := range sources {
		groups[source.ClassID] = append(groups[source.ClassID], source)
	}
	indexed := 0
	for _, group := range groups {
		_, _, created, err := s.prepareKnowledgeChunks(ctx, group, true)
		if err != nil {
			return indexed, err
		}
		indexed += created
	}
	return indexed, nil
}

func (s *Server) indexKnowledgeSource(ctx context.Context, source store.KnowledgeSource) (int, error) {
	if !s.embedding.Configured() || !s.qdrant.Configured() {
		return 0, nil
	}
	if err := s.qdrant.EnsureCollection(ctx); err != nil {
		return 0, err
	}
	_, _, created, err := s.prepareKnowledgeChunks(ctx, []store.KnowledgeSource{source}, true)
	return created, err
}

func (s *Server) prepareKnowledgeChunks(
	ctx context.Context,
	sources []store.KnowledgeSource,
	forceIndex bool,
) ([]indexedKnowledgeChunk, int, int, error) {
	if len(sources) == 0 {
		return []indexedKnowledgeChunk{}, 0, 0, nil
	}
	classID := sources[0].ClassID
	for _, source := range sources {
		if source.ClassID != classID {
			return nil, 0, 0, fmt.Errorf("一次知识索引不能混合多个班级")
		}
	}
	stored, err := s.store.ListKnowledgeChunksForClass(ctx, classID)
	if err != nil {
		return nil, 0, 0, err
	}
	cache := make(map[knowledgeChunkKey]store.KnowledgeChunk, len(stored))
	for _, chunk := range stored {
		cache[knowledgeChunkKey{
			KnowledgeEntryID: chunk.KnowledgeEntryID,
			ChunkIndex:       chunk.ChunkIndex,
		}] = chunk
	}

	indexed := make([]indexedKnowledgeChunk, 0)
	cacheHits := 0
	indexedNow := 0
	for _, source := range sources {
		chunks := splitKnowledgeSource(source)
		for _, chunk := range chunks {
			input := embeddingChunkInput(chunk)
			sum := sha256.Sum256([]byte(input))
			contentHash := hex.EncodeToString(sum[:])
			key := knowledgeChunkKey{
				KnowledgeEntryID: source.KnowledgeEntryID,
				ChunkIndex:       chunk.ChunkIndex,
			}
			cached, exists := cache[key]
			validCache := exists &&
				cached.ContentHash == contentHash &&
				cached.EmbeddingModel == s.config.EmbeddingModel &&
				cached.StartOffset == chunk.StartOffset &&
				cached.EndOffset == chunk.EndOffset &&
				cached.IndexStatus == "ready"
			if validCache && !forceIndex {
				chunk.ChunkID = cached.ID
				cacheHits++
				indexed = append(indexed, indexedKnowledgeChunk{
					Chunk:            chunk,
					KnowledgeEntryID: source.KnowledgeEntryID,
					ClassID:          source.ClassID,
					ContentHash:      contentHash,
				})
				continue
			}

			chunkID, err := s.store.UpsertKnowledgeChunk(ctx, store.KnowledgeChunk{
				KnowledgeEntryID: source.KnowledgeEntryID,
				MaterialID:       source.MaterialID,
				ClassID:          source.ClassID,
				ChunkIndex:       chunk.ChunkIndex,
				StartOffset:      chunk.StartOffset,
				EndOffset:        chunk.EndOffset,
				ChunkText:        chunk.Text,
				ContentHash:      contentHash,
				EmbeddingModel:   s.config.EmbeddingModel,
				ChunkStrategy:    "auto",
			})
			if err != nil {
				return nil, cacheHits, indexedNow, err
			}
			vector, err := s.embedding.Embed(ctx, input)
			if err != nil {
				_ = s.store.SetKnowledgeChunkIndexState(
					ctx, chunkID, source.ClassID, "failed", truncateRunes(err.Error(), 500),
				)
				return nil, cacheHits, indexedNow, fmt.Errorf(
					"材料 %d 切片 %d 向量化失败: %w",
					source.MaterialID,
					chunk.ChunkIndex,
					err,
				)
			}
			point := qdrantPoint{
				ID:     chunkID,
				Vector: vector,
				Payload: map[string]any{
					"class_id":           source.ClassID,
					"material_id":        source.MaterialID,
					"knowledge_entry_id": source.KnowledgeEntryID,
					"chunk_id":           chunkID,
					"chunk_index":        chunk.ChunkIndex,
				},
			}
			if err := s.qdrant.UpsertPoint(ctx, point); err != nil {
				_ = s.store.SetKnowledgeChunkIndexState(
					ctx, chunkID, source.ClassID, "failed", truncateRunes(err.Error(), 500),
				)
				return nil, cacheHits, indexedNow, err
			}
			if err := s.store.SetKnowledgeChunkIndexState(
				ctx, chunkID, source.ClassID, "ready", "",
			); err != nil {
				return nil, cacheHits, indexedNow, err
			}
			chunk.ChunkID = chunkID
			indexedNow++
			indexed = append(indexed, indexedKnowledgeChunk{
				Chunk:            chunk,
				KnowledgeEntryID: source.KnowledgeEntryID,
				ClassID:          source.ClassID,
				ContentHash:      contentHash,
			})
		}
		if err := s.store.DeleteStaleKnowledgeChunks(
			ctx,
			source.KnowledgeEntryID,
			len(chunks),
		); err != nil {
			return nil, cacheHits, indexedNow, err
		}
	}
	return indexed, cacheHits, indexedNow, nil
}

func (s *Server) emptyHybridRetrievalTrace() map[string]any {
	return map[string]any{
		"method":                 "hybrid_rrf",
		"embedding_configured":   true,
		"embedding_model":        s.config.EmbeddingModel,
		"embedding_dim":          s.config.EmbeddingDim,
		"qdrant_configured":      true,
		"qdrant_collection":      s.config.QdrantCollection,
		"vector_score_threshold": s.config.VectorThreshold,
		"rrf_k":                  s.config.RRFK,
		"candidate_chunks":       0,
		"keyword_candidates":     0,
		"vector_candidates":      0,
		"rrf_candidates":         0,
		"query_vector_requested": false,
		"knowledge_sent_to_llm":  0,
	}
}

func embeddingChunkInput(chunk knowledgeChunk) string {
	return "材料标题：" + chunk.MaterialTitle + "\n内容：" + chunk.Text
}

func countKnowledgeChunks(sources []store.KnowledgeSource) int {
	count := 0
	for _, source := range sources {
		count += len(splitKnowledgeSource(source))
	}
	return count
}

func uniqueMaterialIDs(sources []store.KnowledgeSource) []int64 {
	seen := make(map[int64]bool)
	result := make([]int64, 0, len(sources))
	for _, source := range sources {
		if !seen[source.MaterialID] {
			seen[source.MaterialID] = true
			result = append(result, source.MaterialID)
		}
	}
	return result
}

func qdrantPayloadMatches(payload map[string]any, item indexedKnowledgeChunk) bool {
	classID, classOK := payloadInt64(payload["class_id"])
	materialID, materialOK := payloadInt64(payload["material_id"])
	entryID, entryOK := payloadInt64(payload["knowledge_entry_id"])
	chunkID, chunkOK := payloadInt64(payload["chunk_id"])
	chunkIndex, chunkIndexOK := payloadInt64(payload["chunk_index"])
	return classOK &&
		materialOK &&
		entryOK &&
		chunkOK &&
		chunkIndexOK &&
		classID == item.ClassID &&
		materialID == item.Chunk.MaterialID &&
		entryID == item.KnowledgeEntryID &&
		chunkID == item.Chunk.ChunkID &&
		chunkIndex == int64(item.Chunk.ChunkIndex)
}

func payloadInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if typed != math.Trunc(typed) {
			return 0, false
		}
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	default:
		return 0, false
	}
}

func bestPositiveRank(first, second int) int {
	switch {
	case first <= 0:
		return second
	case second <= 0:
		return first
	case first < second:
		return first
	default:
		return second
	}
}

func vectorNorm(vector []float64) float64 {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	return math.Sqrt(sum)
}

func vectorPreview(vector []float64, limit int) []float64 {
	if limit <= 0 {
		return []float64{}
	}
	if len(vector) < limit {
		limit = len(vector)
	}
	return append([]float64(nil), vector[:limit]...)
}
