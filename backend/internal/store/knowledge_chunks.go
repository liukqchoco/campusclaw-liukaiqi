package store

import (
	"context"
	"database/sql"
)

type KnowledgeChunk struct {
	ID               int64
	KnowledgeEntryID int64
	MaterialID       int64
	ClassID          int64
	ChunkIndex       int
	StartOffset      int
	EndOffset        int
	ChunkText        string
	ContentHash      string
	EmbeddingModel   string
	ChunkStrategy    string
	IndexStatus      string
	EmbeddingError   string
}

func (s *Store) UpsertKnowledgeChunk(ctx context.Context, chunk KnowledgeChunk) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO knowledge_chunks (
			knowledge_entry_id, material_id, class_id, chunk_index,
			start_offset, end_offset, chunk_text, content_hash,
			embedding_model, chunk_strategy, index_status, embedding_error
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '')
		ON DUPLICATE KEY UPDATE
			id = LAST_INSERT_ID(id),
			index_status = IF(
				content_hash = VALUES(content_hash)
				AND embedding_model = VALUES(embedding_model)
				AND start_offset = VALUES(start_offset)
				AND end_offset = VALUES(end_offset),
				index_status,
				'pending'
			),
			embedding_error = IF(
				content_hash = VALUES(content_hash)
				AND embedding_model = VALUES(embedding_model),
				embedding_error,
				''
			),
			material_id = VALUES(material_id),
			class_id = VALUES(class_id),
			start_offset = VALUES(start_offset),
			end_offset = VALUES(end_offset),
			chunk_text = VALUES(chunk_text),
			content_hash = VALUES(content_hash),
			embedding_model = VALUES(embedding_model),
			chunk_strategy = VALUES(chunk_strategy)`,
		chunk.KnowledgeEntryID,
		chunk.MaterialID,
		chunk.ClassID,
		chunk.ChunkIndex,
		chunk.StartOffset,
		chunk.EndOffset,
		chunk.ChunkText,
		chunk.ContentHash,
		chunk.EmbeddingModel,
		chunk.ChunkStrategy,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListKnowledgeChunksForClass(
	ctx context.Context,
	classID int64,
) ([]KnowledgeChunk, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, knowledge_entry_id, material_id, class_id, chunk_index,
		       start_offset, end_offset, chunk_text, content_hash, embedding_model,
		       chunk_strategy, index_status, embedding_error
		FROM knowledge_chunks
		WHERE class_id = ?
		ORDER BY material_id, chunk_index`,
		classID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chunks := make([]KnowledgeChunk, 0)
	for rows.Next() {
		var chunk KnowledgeChunk
		if err := rows.Scan(
			&chunk.ID,
			&chunk.KnowledgeEntryID,
			&chunk.MaterialID,
			&chunk.ClassID,
			&chunk.ChunkIndex,
			&chunk.StartOffset,
			&chunk.EndOffset,
			&chunk.ChunkText,
			&chunk.ContentHash,
			&chunk.EmbeddingModel,
			&chunk.ChunkStrategy,
			&chunk.IndexStatus,
			&chunk.EmbeddingError,
		); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (s *Store) SetKnowledgeChunkIndexState(
	ctx context.Context,
	chunkID, classID int64,
	status, embeddingError string,
) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE knowledge_chunks
		SET index_status = ?, embedding_error = ?
		WHERE id = ? AND class_id = ?`,
		status, embeddingError, chunkID, classID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteStaleKnowledgeChunks(
	ctx context.Context,
	knowledgeEntryID int64,
	fromIndex int,
) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM knowledge_chunks
		WHERE knowledge_entry_id = ? AND chunk_index >= ?`,
		knowledgeEntryID, fromIndex,
	)
	return err
}
