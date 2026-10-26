export type User = {
  id: number;
  username: string;
  display_name: string;
  role: "teacher" | "student";
  class_id: number;
  class_name: string;
};

export type PlatformStatus = {
  llm_configured: boolean;
  llm_model: string;
  embedding_configured: boolean;
  embedding_model: string;
  embedding_dim: number;
  qdrant_configured: boolean;
  qdrant_collection: string;
  retrieval_mode: string;
  vector_threshold: number;
  rrf_k: number;
  mcp_configured: boolean;
  trace_enabled: boolean;
};

export type PromptTemplate = {
  id: number;
  class_id: number;
  name: string;
  content: string;
  built_in: boolean;
  updated_at: string;
};

export type AIApp = {
  id: number;
  class_id: number;
  name: string;
  description: string;
  prompt_template_id: number;
  prompt_template_name: string;
  prompt_template: string;
  enabled: boolean;
  material_ids: number[];
  skill_ids: number[];
  created_at: string;
  updated_at: string;
};

export type Skill = {
  id: number;
  class_id: number;
  name: string;
  trigger_text: string;
  description: string;
  tools: string[];
  source_text: string;
  subject_text: string;
  enabled: boolean;
  built_in: boolean;
  created_at: string;
  updated_at: string;
};

export type Material = {
  id: number;
  class_id: number;
  class_name: string;
  uploader_name: string;
  title: string;
  original_name: string;
  media_type: string;
  size_bytes: number;
  content_preview?: string;
  content?: string;
  created_at: string;
};

export type Conversation = {
  id: number;
  class_id: number;
  user_id: number;
  app_id: number;
  app_name: string;
  title: string;
  created_at: string;
  updated_at: string;
};

export type ConversationMessage = {
  id: number;
  conversation_id: number;
  role: "user" | "assistant";
  content: string;
  created_at: string;
};

export type MCPServer = {
  id: number;
  class_id: number;
  name: string;
  base_url: string;
  has_token: boolean;
  connected: boolean;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type MCPTool = {
  id: number;
  server_id: number;
  server_name: string;
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
};

export type ToolCallLog = {
  id: number;
  class_id: number;
  user_id: number;
  conversation_id: number;
  server_id: number;
  tool_ref: string;
  round: number;
  status: string;
  arguments_json: string;
  result_preview: string;
  error_message?: string;
  created_at: string;
};

export type TraceSummary = {
  id: number;
  class_id: number;
  user_id: number;
  username: string;
  conversation_id: number;
  app_id: number;
  app_name: string;
  question: string;
  template_name: string;
  selected_skill_id?: number;
  selected_skill_name?: string;
  final_answer: string;
  created_at: string;
};

export type TraceDetail = TraceSummary & {
  template_source: string;
  template_rendered: string;
  template_variables: Record<string, unknown>;
  skill_injected: string;
  skill_route: Record<string, unknown>;
  retrieval: Record<string, unknown>;
  llm_requests: Array<Record<string, unknown>>;
  llm_responses: Array<Record<string, unknown>>;
  mcp_events: Array<Record<string, unknown>>;
};
