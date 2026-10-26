package store

import (
	"encoding/json"
	"time"
)

type PromptTemplate struct {
	ID        int64     `json:"id"`
	ClassID   int64     `json:"class_id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	BuiltIn   bool      `json:"built_in"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AIApp struct {
	ID                 int64     `json:"id"`
	ClassID            int64     `json:"class_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	PromptTemplateID   int64     `json:"prompt_template_id"`
	PromptTemplateName string    `json:"prompt_template_name"`
	PromptTemplate     string    `json:"prompt_template"`
	Enabled            bool      `json:"enabled"`
	MaterialIDs        []int64   `json:"material_ids"`
	SkillIDs           []int64   `json:"skill_ids"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Skill struct {
	ID          int64     `json:"id"`
	ClassID     int64     `json:"class_id"`
	Name        string    `json:"name"`
	TriggerText string    `json:"trigger_text"`
	Description string    `json:"description"`
	Tools       []string  `json:"tools"`
	SourceText  string    `json:"source_text"`
	SubjectText string    `json:"subject_text"`
	Enabled     bool      `json:"enabled"`
	BuiltIn     bool      `json:"built_in"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Conversation struct {
	ID        int64     `json:"id"`
	ClassID   int64     `json:"class_id"`
	UserID    int64     `json:"user_id"`
	AppID     int64     `json:"app_id"`
	AppName   string    `json:"app_name"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConversationMessage struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

type MCPServer struct {
	ID        int64     `json:"id"`
	ClassID   int64     `json:"class_id"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	AuthToken string    `json:"-"`
	HasToken  bool      `json:"has_token"`
	Connected bool      `json:"connected"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MCPTool struct {
	ID          int64          `json:"id"`
	ServerID    int64          `json:"server_id"`
	ServerName  string         `json:"server_name"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type ToolCallLog struct {
	ID             int64     `json:"id"`
	ClassID        int64     `json:"class_id"`
	UserID         int64     `json:"user_id"`
	ConversationID int64     `json:"conversation_id"`
	ServerID       int64     `json:"server_id"`
	ToolRef        string    `json:"tool_ref"`
	Round          int       `json:"round"`
	Status         string    `json:"status"`
	ArgumentsJSON  string    `json:"arguments_json"`
	ResultPreview  string    `json:"result_preview"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type LLMTrace struct {
	ID                    int64            `json:"id"`
	ClassID               int64            `json:"class_id"`
	UserID                int64            `json:"user_id"`
	Username              string           `json:"username,omitempty"`
	ConversationID        int64            `json:"conversation_id"`
	AppID                 int64            `json:"app_id"`
	AppName               string           `json:"app_name,omitempty"`
	Question              string           `json:"question"`
	TemplateName          string           `json:"template_name"`
	TemplateSource        string           `json:"template_source"`
	TemplateRendered      string           `json:"template_rendered"`
	TemplateVariablesJSON string           `json:"template_variables_json"`
	SelectedSkillID       *int64           `json:"selected_skill_id,omitempty"`
	SelectedSkillName     string           `json:"selected_skill_name,omitempty"`
	SkillInjected         string           `json:"skill_injected"`
	SkillRouteJSON        string           `json:"skill_route_json"`
	RetrievalJSON         string           `json:"retrieval_json"`
	LLMRequestsJSON       string           `json:"llm_requests_json"`
	LLMResponsesJSON      string           `json:"llm_responses_json"`
	MCPEventsJSON         string           `json:"mcp_events_json"`
	FinalAnswer           string           `json:"final_answer"`
	CreatedAt             time.Time        `json:"created_at"`
	TemplateVariables     map[string]any   `json:"template_variables"`
	SkillRoute            map[string]any   `json:"skill_route"`
	Retrieval             map[string]any   `json:"retrieval"`
	LLMRequests           []map[string]any `json:"llm_requests"`
	LLMResponses          []map[string]any `json:"llm_responses"`
	MCPEvents             []map[string]any `json:"mcp_events"`
}

func (trace *LLMTrace) DecodeJSON() {
	decodeObject(trace.TemplateVariablesJSON, &trace.TemplateVariables)
	decodeObject(trace.SkillRouteJSON, &trace.SkillRoute)
	decodeObject(trace.RetrievalJSON, &trace.Retrieval)
	decodeObject(trace.LLMRequestsJSON, &trace.LLMRequests)
	decodeObject(trace.LLMResponsesJSON, &trace.LLMResponses)
	decodeObject(trace.MCPEventsJSON, &trace.MCPEvents)
	if trace.TemplateVariables == nil {
		trace.TemplateVariables = map[string]any{}
	}
	if trace.SkillRoute == nil {
		trace.SkillRoute = map[string]any{}
	}
	if trace.Retrieval == nil {
		trace.Retrieval = map[string]any{}
	}
	if trace.LLMRequests == nil {
		trace.LLMRequests = []map[string]any{}
	}
	if trace.LLMResponses == nil {
		trace.LLMResponses = []map[string]any{}
	}
	if trace.MCPEvents == nil {
		trace.MCPEvents = []map[string]any{}
	}
}

func decodeObject(raw string, target any) {
	if raw == "" {
		return
	}
	_ = json.Unmarshal([]byte(raw), target)
}

func aiMigrationStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS prompt_templates (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			content TEXT NOT NULL,
			built_in BOOLEAN NOT NULL DEFAULT FALSE,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_prompt_class_name (class_id, name),
			CONSTRAINT fk_prompt_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS ai_apps (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			description VARCHAR(500) NOT NULL,
			prompt_template_id BIGINT NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_app_class_name (class_id, name),
			CONSTRAINT fk_app_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE,
			CONSTRAINT fk_app_prompt FOREIGN KEY (prompt_template_id) REFERENCES prompt_templates(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS ai_skills (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(32) NOT NULL,
			trigger_text VARCHAR(120) NOT NULL,
			description TEXT NOT NULL,
			tools_json TEXT NOT NULL,
			source_text MEDIUMTEXT NOT NULL,
			subject_text VARCHAR(32) NOT NULL DEFAULT '',
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			built_in BOOLEAN NOT NULL DEFAULT FALSE,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_ai_skill_class_name (class_id, name),
			CONSTRAINT fk_ai_skill_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS ai_app_materials (
			app_id BIGINT NOT NULL,
			material_id BIGINT NOT NULL,
			PRIMARY KEY (app_id, material_id),
			CONSTRAINT fk_appmat_app FOREIGN KEY (app_id) REFERENCES ai_apps(id) ON DELETE CASCADE,
			CONSTRAINT fk_appmat_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS ai_app_skills (
			app_id BIGINT NOT NULL,
			skill_id BIGINT NOT NULL,
			PRIMARY KEY (app_id, skill_id),
			CONSTRAINT fk_appskill_app FOREIGN KEY (app_id) REFERENCES ai_apps(id) ON DELETE CASCADE,
			CONSTRAINT fk_appskill_skill FOREIGN KEY (skill_id) REFERENCES ai_skills(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS knowledge_chunks (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			knowledge_entry_id BIGINT NOT NULL,
			material_id BIGINT NOT NULL,
			class_id BIGINT NOT NULL,
			chunk_index INT NOT NULL,
			start_offset INT NOT NULL,
			end_offset INT NOT NULL,
			chunk_text TEXT NOT NULL,
			content_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			embedding_model VARCHAR(100) NOT NULL,
			chunk_strategy VARCHAR(16) NOT NULL DEFAULT 'auto',
			index_status VARCHAR(16) NOT NULL DEFAULT 'pending',
			embedding_error VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_knowledge_chunk (knowledge_entry_id, chunk_index),
			INDEX idx_knowledge_chunk_class_status (class_id, index_status),
			INDEX idx_knowledge_chunk_material (material_id, chunk_index),
			CONSTRAINT fk_knowledge_chunk_entry FOREIGN KEY (knowledge_entry_id) REFERENCES knowledge_entries(id) ON DELETE CASCADE,
			CONSTRAINT fk_knowledge_chunk_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE,
			CONSTRAINT fk_knowledge_chunk_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			app_id BIGINT NOT NULL,
			title VARCHAR(120) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			INDEX idx_conversation_owner (class_id, user_id, updated_at),
			CONSTRAINT fk_conversation_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_conversation_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			CONSTRAINT fk_conversation_app FOREIGN KEY (app_id) REFERENCES ai_apps(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS conversation_messages (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			conversation_id BIGINT NOT NULL,
			role VARCHAR(16) NOT NULL,
			content MEDIUMTEXT NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_message_conversation (conversation_id, id),
			CONSTRAINT fk_message_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			CONSTRAINT chk_message_role CHECK (role IN ('user', 'assistant'))
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			base_url VARCHAR(500) NOT NULL,
			auth_token VARCHAR(500) NOT NULL,
			connected BOOLEAN NOT NULL DEFAULT FALSE,
			last_error VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			UNIQUE KEY uq_mcp_class_name (class_id, name),
			CONSTRAINT fk_mcp_class FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS mcp_tools (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			server_id BIGINT NOT NULL,
			name VARCHAR(64) NOT NULL,
			description VARCHAR(1000) NOT NULL,
			input_schema TEXT NOT NULL,
			UNIQUE KEY uq_mcp_tool (server_id, name),
			CONSTRAINT fk_mcp_tool_server FOREIGN KEY (server_id) REFERENCES mcp_servers(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS tool_call_logs (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			conversation_id BIGINT NOT NULL,
			server_id BIGINT NOT NULL,
			tool_ref VARCHAR(140) NOT NULL,
			round_number INT NOT NULL,
			status VARCHAR(16) NOT NULL,
			arguments_json TEXT NOT NULL,
			result_preview TEXT NOT NULL,
			error_message VARCHAR(500) NOT NULL DEFAULT '',
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_tool_log_class_created (class_id, created_at),
			CONSTRAINT fk_tool_log_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_tool_log_user FOREIGN KEY (user_id) REFERENCES users(id),
			CONSTRAINT fk_tool_log_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			CONSTRAINT fk_tool_log_server FOREIGN KEY (server_id) REFERENCES mcp_servers(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS llm_traces (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			conversation_id BIGINT NOT NULL,
			app_id BIGINT NOT NULL,
			question TEXT NOT NULL,
			template_name VARCHAR(64) NOT NULL,
			template_source TEXT NOT NULL,
			template_rendered TEXT NOT NULL,
			template_variables_json TEXT NOT NULL,
			selected_skill_id BIGINT NULL,
			selected_skill_name VARCHAR(32) NOT NULL DEFAULT '',
			skill_injected TEXT NOT NULL,
			skill_route_json TEXT NOT NULL,
			retrieval_json MEDIUMTEXT NOT NULL,
			llm_requests_json LONGTEXT NOT NULL,
			llm_responses_json LONGTEXT NOT NULL,
			mcp_events_json MEDIUMTEXT NOT NULL,
			final_answer MEDIUMTEXT NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_trace_class_created (class_id, created_at),
			CONSTRAINT fk_trace_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_trace_user FOREIGN KEY (user_id) REFERENCES users(id),
			CONSTRAINT fk_trace_conversation FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			CONSTRAINT fk_trace_app FOREIGN KEY (app_id) REFERENCES ai_apps(id),
			CONSTRAINT fk_trace_skill FOREIGN KEY (selected_skill_id) REFERENCES ai_skills(id) ON DELETE SET NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
}
