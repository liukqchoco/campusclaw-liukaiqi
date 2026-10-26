package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func (s *Store) ListMCPServers(ctx context.Context, classID int64) ([]MCPServer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, class_id, name, base_url, auth_token <> '', connected, last_error, created_at, updated_at
		FROM mcp_servers
		WHERE class_id = ?
		ORDER BY id`,
		classID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := make([]MCPServer, 0)
	for rows.Next() {
		var server MCPServer
		if err := rows.Scan(
			&server.ID,
			&server.ClassID,
			&server.Name,
			&server.BaseURL,
			&server.HasToken,
			&server.Connected,
			&server.LastError,
			&server.CreatedAt,
			&server.UpdatedAt,
		); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) GetMCPServer(ctx context.Context, id, classID int64) (MCPServer, error) {
	var server MCPServer
	err := s.db.QueryRowContext(ctx, `
		SELECT id, class_id, name, base_url, auth_token, auth_token <> '',
		       connected, last_error, created_at, updated_at
		FROM mcp_servers
		WHERE id = ? AND class_id = ?`,
		id, classID,
	).Scan(
		&server.ID,
		&server.ClassID,
		&server.Name,
		&server.BaseURL,
		&server.AuthToken,
		&server.HasToken,
		&server.Connected,
		&server.LastError,
		&server.CreatedAt,
		&server.UpdatedAt,
	)
	return server, err
}

func (s *Store) RegisterMCPServer(ctx context.Context, classID int64, name, baseURL, token string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO mcp_servers (class_id, name, base_url, auth_token, connected, last_error)
		VALUES (?, ?, ?, ?, FALSE, '')`,
		classID, name, baseURL, token,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ReplaceMCPTools(ctx context.Context, serverID, classID int64, tools []MCPTool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_servers WHERE id = ? AND class_id = ?`, serverID, classID).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id = ?`, serverID); err != nil {
		return err
	}
	for _, tool := range tools {
		schemaJSON, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO mcp_tools (server_id, name, description, input_schema)
			VALUES (?, ?, ?, ?)`,
			serverID, tool.Name, tool.Description, string(schemaJSON),
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE mcp_servers
		SET connected = TRUE, last_error = ''
		WHERE id = ? AND class_id = ?`,
		serverID, classID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetMCPConnectionError(ctx context.Context, serverID, classID int64, message string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE mcp_servers
		SET connected = FALSE, last_error = ?
		WHERE id = ? AND class_id = ?`,
		message, serverID, classID,
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
	_, err = s.db.ExecContext(ctx, `DELETE FROM mcp_tools WHERE server_id = ?`, serverID)
	return err
}

func (s *Store) DisconnectMCPServer(ctx context.Context, serverID, classID int64) error {
	return s.SetMCPConnectionError(ctx, serverID, classID, "")
}

func (s *Store) ListMCPTools(ctx context.Context, classID int64) ([]MCPTool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.server_id, s.name, t.name, t.description, t.input_schema
		FROM mcp_tools t
		JOIN mcp_servers s ON s.id = t.server_id
		WHERE s.class_id = ? AND s.connected = TRUE
		ORDER BY s.id, t.id`,
		classID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tools := make([]MCPTool, 0)
	for rows.Next() {
		var item MCPTool
		var schemaJSON string
		if err := rows.Scan(&item.ID, &item.ServerID, &item.ServerName, &item.Name, &item.Description, &schemaJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(schemaJSON), &item.InputSchema); err != nil {
			return nil, fmt.Errorf("解析 MCP 工具结构失败: %w", err)
		}
		tools = append(tools, item)
	}
	return tools, rows.Err()
}

func (s *Store) ResolveMCPToolRefs(ctx context.Context, classID int64, refs []string) (map[string]MCPTool, error) {
	all, err := s.ListMCPTools(ctx, classID)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		wanted[ref] = struct{}{}
	}
	resolved := make(map[string]MCPTool)
	for _, item := range all {
		ref := item.Name + "@" + item.ServerName
		if _, ok := wanted[ref]; ok {
			resolved[ref] = item
		}
	}
	return resolved, nil
}

func (s *Store) CreateToolCallLog(ctx context.Context, log ToolCallLog) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_call_logs (
			class_id, user_id, conversation_id, server_id, tool_ref, round_number,
			status, arguments_json, result_preview, error_message
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ClassID,
		log.UserID,
		log.ConversationID,
		log.ServerID,
		log.ToolRef,
		log.Round,
		log.Status,
		log.ArgumentsJSON,
		log.ResultPreview,
		log.ErrorMessage,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) ListToolCallLogs(ctx context.Context, user User) ([]ToolCallLog, error) {
	query := `
		SELECT id, class_id, user_id, conversation_id, server_id, tool_ref, round_number,
		       status, arguments_json, result_preview, error_message, created_at
		FROM tool_call_logs
		WHERE class_id = ?`
	args := []any{user.ClassID}
	if user.Role != "teacher" {
		query += ` AND user_id = ?`
		args = append(args, user.ID)
	}
	query += ` ORDER BY id DESC LIMIT 100`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := make([]ToolCallLog, 0)
	for rows.Next() {
		var item ToolCallLog
		if err := rows.Scan(
			&item.ID,
			&item.ClassID,
			&item.UserID,
			&item.ConversationID,
			&item.ServerID,
			&item.ToolRef,
			&item.Round,
			&item.Status,
			&item.ArgumentsJSON,
			&item.ResultPreview,
			&item.ErrorMessage,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, item)
	}
	return logs, rows.Err()
}
