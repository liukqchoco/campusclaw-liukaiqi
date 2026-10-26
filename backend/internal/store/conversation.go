package store

import (
	"context"
	"database/sql"
)

func (s *Store) CreateConversation(ctx context.Context, user User, appID int64, title string) (int64, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ai_apps
		WHERE id = ? AND class_id = ? AND enabled = TRUE`,
		appID, user.ClassID,
	).Scan(&exists); err != nil {
		return 0, err
	}
	if exists != 1 {
		return 0, sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO conversations (class_id, user_id, app_id, title)
		VALUES (?, ?, ?, ?)`,
		user.ClassID, user.ID, appID, title,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListConversations(ctx context.Context, user User) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.class_id, c.user_id, c.app_id, a.name, c.title, c.created_at, c.updated_at
		FROM conversations c
		JOIN ai_apps a ON a.id = c.app_id
		WHERE c.class_id = ? AND c.user_id = ?
		ORDER BY c.updated_at DESC, c.id DESC
		LIMIT 100`,
		user.ClassID, user.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := make([]Conversation, 0)
	for rows.Next() {
		var conversation Conversation
		if err := rows.Scan(
			&conversation.ID,
			&conversation.ClassID,
			&conversation.UserID,
			&conversation.AppID,
			&conversation.AppName,
			&conversation.Title,
			&conversation.CreatedAt,
			&conversation.UpdatedAt,
		); err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func (s *Store) GetConversation(ctx context.Context, id int64, user User) (Conversation, error) {
	var conversation Conversation
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.class_id, c.user_id, c.app_id, a.name, c.title, c.created_at, c.updated_at
		FROM conversations c
		JOIN ai_apps a ON a.id = c.app_id
		WHERE c.id = ? AND c.class_id = ? AND c.user_id = ?`,
		id, user.ClassID, user.ID,
	).Scan(
		&conversation.ID,
		&conversation.ClassID,
		&conversation.UserID,
		&conversation.AppID,
		&conversation.AppName,
		&conversation.Title,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	return conversation, err
}

func (s *Store) ListConversationMessages(ctx context.Context, conversationID int64, user User) ([]ConversationMessage, error) {
	if _, err := s.GetConversation(ctx, conversationID, user); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, created_at
		FROM conversation_messages
		WHERE conversation_id = ?
		ORDER BY id`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]ConversationMessage, 0)
	for rows.Next() {
		var message ConversationMessage
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) RecentConversationMessages(ctx context.Context, conversationID int64, limit int) ([]ConversationMessage, error) {
	if limit <= 0 {
		return []ConversationMessage{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, created_at
		FROM (
			SELECT id, conversation_id, role, content, created_at
			FROM conversation_messages
			WHERE conversation_id = ?
			ORDER BY id DESC
			LIMIT ?
		) recent
		ORDER BY id`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]ConversationMessage, 0)
	for rows.Next() {
		var message ConversationMessage
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) AddConversationMessage(ctx context.Context, conversationID int64, role, content string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO conversation_messages (conversation_id, role, content)
		VALUES (?, ?, ?)`,
		conversationID, role, content,
	)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE conversations SET updated_at = NOW(6) WHERE id = ?`, conversationID); err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) CreateLLMTrace(ctx context.Context, trace LLMTrace) (int64, error) {
	var skillID any
	if trace.SelectedSkillID != nil {
		skillID = *trace.SelectedSkillID
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO llm_traces (
			class_id, user_id, conversation_id, app_id, question,
			template_name, template_source, template_rendered, template_variables_json,
			selected_skill_id, selected_skill_name, skill_injected, skill_route_json,
			retrieval_json, llm_requests_json, llm_responses_json, mcp_events_json, final_answer
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		trace.ClassID,
		trace.UserID,
		trace.ConversationID,
		trace.AppID,
		trace.Question,
		trace.TemplateName,
		trace.TemplateSource,
		trace.TemplateRendered,
		trace.TemplateVariablesJSON,
		skillID,
		trace.SelectedSkillName,
		trace.SkillInjected,
		trace.SkillRouteJSON,
		trace.RetrievalJSON,
		trace.LLMRequestsJSON,
		trace.LLMResponsesJSON,
		trace.MCPEventsJSON,
		trace.FinalAnswer,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListLLMTraces(ctx context.Context, user User) ([]LLMTrace, error) {
	query := `
		SELECT t.id, t.class_id, t.user_id, u.username, t.conversation_id, t.app_id, a.name,
		       t.question, t.template_name, t.selected_skill_id, t.selected_skill_name,
		       t.final_answer, t.created_at
		FROM llm_traces t
		JOIN users u ON u.id = t.user_id
		JOIN ai_apps a ON a.id = t.app_id
		WHERE t.class_id = ?`
	args := []any{user.ClassID}
	if user.Role != "teacher" {
		query += ` AND t.user_id = ?`
		args = append(args, user.ID)
	}
	query += ` ORDER BY t.id DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	traces := make([]LLMTrace, 0)
	for rows.Next() {
		var trace LLMTrace
		var skillID sql.NullInt64
		if err := rows.Scan(
			&trace.ID,
			&trace.ClassID,
			&trace.UserID,
			&trace.Username,
			&trace.ConversationID,
			&trace.AppID,
			&trace.AppName,
			&trace.Question,
			&trace.TemplateName,
			&skillID,
			&trace.SelectedSkillName,
			&trace.FinalAnswer,
			&trace.CreatedAt,
		); err != nil {
			return nil, err
		}
		if skillID.Valid {
			value := skillID.Int64
			trace.SelectedSkillID = &value
		}
		traces = append(traces, trace)
	}
	return traces, rows.Err()
}

func (s *Store) GetLLMTrace(ctx context.Context, id int64, user User) (LLMTrace, error) {
	query := `
		SELECT t.id, t.class_id, t.user_id, u.username, t.conversation_id, t.app_id, a.name,
		       t.question, t.template_name, t.template_source, t.template_rendered,
		       t.template_variables_json, t.selected_skill_id, t.selected_skill_name,
		       t.skill_injected, t.skill_route_json, t.retrieval_json, t.llm_requests_json,
		       t.llm_responses_json, t.mcp_events_json, t.final_answer, t.created_at
		FROM llm_traces t
		JOIN users u ON u.id = t.user_id
		JOIN ai_apps a ON a.id = t.app_id
		WHERE t.id = ? AND t.class_id = ?`
	args := []any{id, user.ClassID}
	if user.Role != "teacher" {
		query += ` AND t.user_id = ?`
		args = append(args, user.ID)
	}
	var trace LLMTrace
	var skillID sql.NullInt64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&trace.ID,
		&trace.ClassID,
		&trace.UserID,
		&trace.Username,
		&trace.ConversationID,
		&trace.AppID,
		&trace.AppName,
		&trace.Question,
		&trace.TemplateName,
		&trace.TemplateSource,
		&trace.TemplateRendered,
		&trace.TemplateVariablesJSON,
		&skillID,
		&trace.SelectedSkillName,
		&trace.SkillInjected,
		&trace.SkillRouteJSON,
		&trace.RetrievalJSON,
		&trace.LLMRequestsJSON,
		&trace.LLMResponsesJSON,
		&trace.MCPEventsJSON,
		&trace.FinalAnswer,
		&trace.CreatedAt,
	)
	if err != nil {
		return LLMTrace{}, err
	}
	if skillID.Valid {
		value := skillID.Int64
		trace.SelectedSkillID = &value
	}
	trace.DecodeJSON()
	return trace, nil
}
