import { FormEvent, useCallback, useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  api,
  clearAccessToken,
  RequestError,
  saveAccessToken,
  streamConversationMessage,
} from "./api";
import type {
  AIApp,
  Conversation,
  ConversationMessage,
  Material,
  MCPServer,
  MCPTool,
  PlatformStatus,
  PromptTemplate,
  Skill,
  ToolCallLog,
  TraceDetail,
  TraceSummary,
  User,
} from "./types";

type Tab = "chat" | "apps" | "materials" | "skills" | "mcp" | "traces";
type Toast = { kind: "success" | "error"; message: string };

export default function App() {
  const [session, setSession] = useState<"checking" | "guest" | "ready">("checking");
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then(({ user: current }) => {
        setUser(current);
        setSession("ready");
      })
      .catch(() => {
        clearAccessToken();
        setSession("guest");
      });
  }, []);

  if (session === "checking") {
    return (
      <main className="platform-loading">
        <Brand />
        <span />
        <p>正在确认登录状态</p>
      </main>
    );
  }
  if (session === "guest" || !user) {
    return (
      <Login
        onLoggedIn={(nextUser) => {
          setUser(nextUser);
          setSession("ready");
        }}
      />
    );
  }
  return (
    <Platform
      user={user}
      onLogout={() => {
        clearAccessToken();
        setUser(null);
        setSession("guest");
      }}
    />
  );
}

function Brand() {
  return (
    <div className="platform-brand">
      <span>C</span>
      <div>
        <strong>CampusClaw</strong>
        <small>AI 应用验证平台</small>
      </div>
    </div>
  );
}

function Login({ onLoggedIn }: { onLoggedIn: (user: User) => void }) {
  const [username, setUsername] = useState("teacher_a");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      const result = await api<{
        access_token: string;
        user: User;
      }>("/api/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      });
      saveAccessToken(result.access_token);
      onLoggedIn(result.user);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "登录失败");
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="platform-login">
      <section className="login-presentation">
        <Brand />
        <div>
          <p className="eyebrow">PERSONAL AI PLATFORM</p>
          <h1>让每一次模型调用<br />都能被验证。</h1>
          <p>多个 AI 应用、提示词模板、知识库、Skill 与 MCP 工具在同一条可追溯链路中协作。</p>
        </div>
        <p className="login-footnote">第 3～6 课个人项目阶段性验收</p>
      </section>
      <section className="login-entry">
        <form onSubmit={submit}>
          <p className="eyebrow">WELCOME</p>
          <h2>登录个人项目</h2>
          <label>
            <span>用户名</span>
            <input value={username} onChange={(event) => setUsername(event.target.value)} required />
          </label>
          <label>
            <span>密码</span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
          </label>
          {error && <p className="inline-error">{error}</p>}
          <button className="solid-button" disabled={loading}>
            {loading ? "正在登录…" : "登录"}
          </button>
          <div className="quick-users">
            <button type="button" onClick={() => setUsername("teacher_a")}>教师账号</button>
            <button type="button" onClick={() => setUsername("student_a1")}>学生账号</button>
          </div>
        </form>
      </section>
    </main>
  );
}

function Platform({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [tab, setTab] = useState<Tab>("chat");
  const [status, setStatus] = useState<PlatformStatus | null>(null);
  const [apps, setApps] = useState<AIApp[]>([]);
  const [templates, setTemplates] = useState<PromptTemplate[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [materials, setMaterials] = useState<Material[]>([]);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [tools, setTools] = useState<MCPTool[]>([]);
  const [toolLogs, setToolLogs] = useState<ToolCallLog[]>([]);
  const [traces, setTraces] = useState<TraceSummary[]>([]);
  const [toast, setToast] = useState<Toast | null>(null);
  const [loading, setLoading] = useState(true);

  const notify = useCallback((next: Toast) => {
    setToast(next);
    window.setTimeout(() => setToast(null), 3500);
  }, []);

  const reload = useCallback(async (showSpinner = false) => {
    if (showSpinner) {
      setLoading(true);
    }
    try {
      const [
        statusResult,
        appResult,
        templateResult,
        skillResult,
        materialResult,
        conversationResult,
        serverResult,
        toolResult,
        logResult,
        traceResult,
      ] = await Promise.all([
        api<PlatformStatus>("/api/platform/status"),
        api<{ apps: AIApp[] }>("/api/apps"),
        api<{ templates: PromptTemplate[] }>("/api/prompt-templates"),
        api<{ skills: Skill[] }>("/api/skills"),
        api<{ materials: Material[] }>("/api/materials"),
        api<{ conversations: Conversation[] }>("/api/conversations"),
        api<{ servers: MCPServer[] }>("/api/mcp/servers"),
        api<{ tools: MCPTool[] }>("/api/mcp/tools"),
        api<{ logs: ToolCallLog[] }>("/api/mcp/logs"),
        api<{ traces: TraceSummary[] }>("/api/traces"),
      ]);
      setStatus(statusResult);
      setApps(appResult.apps);
      setTemplates(templateResult.templates);
      setSkills(skillResult.skills);
      setMaterials(materialResult.materials);
      setConversations(conversationResult.conversations);
      setServers(serverResult.servers);
      setTools(toolResult.tools);
      setToolLogs(logResult.logs);
      setTraces(traceResult.traces);
    } catch (error) {
      if (error instanceof RequestError && error.status === 401) {
        onLogout();
      } else {
        notify({ kind: "error", message: error instanceof Error ? error.message : "加载失败" });
      }
    } finally {
      if (showSpinner) {
        setLoading(false);
      }
    }
  }, [notify, onLogout]);

  useEffect(() => {
    void reload(true);
  }, [reload]);

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "chat", label: "AI 对话" },
    { id: "apps", label: "AI 应用" },
    { id: "materials", label: "知识库" },
    { id: "skills", label: "Skill 技能" },
    { id: "mcp", label: "MCP 工具" },
    { id: "traces", label: "调用验证" },
  ];

  return (
    <div className="platform-shell">
      <header className="platform-header">
        <Brand />
        <nav>
          {tabs.map((item) => (
            <button key={item.id} className={tab === item.id ? "active" : ""} onClick={() => setTab(item.id)}>
              {item.label}
            </button>
          ))}
        </nav>
        <div className="account-area">
          <span>
            <strong>{user.display_name}</strong>
            <small>{user.role === "teacher" ? "教师" : "学生"} · {user.class_name}</small>
          </span>
          <button
            onClick={async () => {
              try {
                await api("/api/logout", { method: "POST" });
              } finally {
                onLogout();
              }
            }}
          >
            退出
          </button>
        </div>
      </header>

      {(!status?.llm_configured || !status?.embedding_configured || !status?.qdrant_configured) && !loading && (
        <div className="configuration-banner">
          <strong>AI 检索或对话依赖尚未完整配置</strong>
          <span>请检查服务端课程网关与 Qdrant 配置后重新启动。</span>
        </div>
      )}

      <main className="platform-main">
        {loading ? (
          <div className="content-loading">正在加载平台数据…</div>
        ) : (
          <>
            {tab === "chat" && (
              <ChatPanel
                apps={apps.filter((app) => app.enabled)}
                conversations={conversations}
                status={status}
                notify={notify}
                onChanged={reload}
              />
            )}
            {tab === "apps" && (
              <AppsPanel
                user={user}
                apps={apps}
                templates={templates}
                skills={skills}
                materials={materials}
                notify={notify}
                onChanged={reload}
              />
            )}
            {tab === "materials" && (
              <MaterialsPanel
                user={user}
                materials={materials}
                status={status}
                notify={notify}
                onChanged={reload}
              />
            )}
            {tab === "skills" && (
              <SkillsPanel user={user} skills={skills} notify={notify} onChanged={reload} />
            )}
            {tab === "mcp" && (
              <MCPPanel
                user={user}
                servers={servers}
                tools={tools}
                logs={toolLogs}
                notify={notify}
                onChanged={reload}
              />
            )}
            {tab === "traces" && <TracePanel traces={traces} notify={notify} />}
          </>
        )}
      </main>
      {toast && <div className={`platform-toast ${toast.kind}`}>{toast.message}</div>}
    </div>
  );
}

function SectionHeading({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <header className="section-heading">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action}
    </header>
  );
}

function ChatPanel({
  apps,
  conversations,
  status,
  notify,
  onChanged,
}: {
  apps: AIApp[];
  conversations: Conversation[];
  status: PlatformStatus | null;
  notify: (toast: Toast) => void;
  onChanged: () => Promise<void>;
}) {
  const [appID, setAppID] = useState<number>(apps[0]?.id ?? 0);
  const [conversationID, setConversationID] = useState<number | null>(null);
  const [messages, setMessages] = useState<ConversationMessage[]>([]);
  const [question, setQuestion] = useState("");
  const [streaming, setStreaming] = useState("");
  const [meta, setMeta] = useState<Record<string, unknown> | null>(null);
  const [toolEvents, setToolEvents] = useState<Array<Record<string, unknown>>>([]);
  const [sending, setSending] = useState(false);

  const openConversation = useCallback(async (id: number, preserveLiveTrace = false) => {
    try {
      const result = await api<{ messages: ConversationMessage[] }>(`/api/conversations/${id}`);
      setConversationID(id);
      setMessages(result.messages);
      setStreaming("");
      if (!preserveLiveTrace) {
        setMeta(null);
        setToolEvents([]);
      }
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "会话加载失败" });
    }
  }, [notify]);

  const currentApp = apps.find((app) => app.id === appID);
  const appConversations = conversations.filter((item) => item.app_id === appID);

  useEffect(() => {
    if (!apps.some((app) => app.id === appID)) {
      setAppID(apps[0]?.id ?? 0);
    }
  }, [appID, apps]);

  useEffect(() => {
    if (sending) return;
    if (conversationID && appConversations.some((item) => item.id === conversationID)) {
      return;
    }
    const first = appConversations[0];
    if (first) {
      void openConversation(first.id);
    } else {
      setConversationID(null);
      setMessages([]);
      setStreaming("");
      setMeta(null);
      setToolEvents([]);
    }
  }, [appID, appConversations, conversationID, openConversation, sending]);

  const createConversation = async () => {
    if (!appID) return;
    try {
      const result = await api<{ id: number }>("/api/conversations", {
        method: "POST",
        body: JSON.stringify({ app_id: appID, title: "" }),
      });
      setConversationID(result.id);
      setMessages([]);
      setStreaming("");
      setMeta(null);
      setToolEvents([]);
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "创建会话失败" });
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const content = question.trim();
    if (!content || sending) return;
    if (!conversationID) {
      notify({ kind: "error", message: "请先创建或选择一个会话" });
      return;
    }
    setSending(true);
    setQuestion("");
    setStreaming("");
    setMeta(null);
    setToolEvents([]);
    setMessages((current) => [
      ...current,
      {
        id: -Date.now(),
        conversation_id: conversationID,
        role: "user",
        content,
        created_at: new Date().toISOString(),
      },
    ]);
    let liveText = "";
    let streamError = "";
    try {
      await streamConversationMessage(conversationID, content, (name, raw) => {
        const data = (raw ?? {}) as Record<string, unknown>;
        if (name === "meta") setMeta(data);
        if (name === "tool") setToolEvents((current) => [...current, data]);
        if (name === "delta") {
          liveText += String(data.content ?? "");
          setStreaming(liveText);
        }
        if (name === "error") {
          streamError = String(data.message ?? "流式回答失败");
        }
      });
      if (streamError) {
        notify({ kind: "error", message: streamError });
      }
      await openConversation(conversationID, true);
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "发送失败" });
    } finally {
      setSending(false);
    }
  };

  return (
    <>
      <SectionHeading
        eyebrow="AI APPLICATIONS"
        title="选择应用，观察一次真实调用"
        description="提示词、知识库、Skill 与 MCP 工具均由当前应用的服务端配置决定。"
        action={
          <button className="solid-button" onClick={() => void createConversation()} disabled={!appID}>
            新建会话
          </button>
        }
      />
      <div className="app-selector">
        {apps.map((app) => (
          <button key={app.id} className={appID === app.id ? "active" : ""} onClick={() => setAppID(app.id)}>
            <span>{app.name}</span>
            <small>{app.description}</small>
            <em>{app.material_ids.length} 份材料 · {app.skill_ids.length} 个 Skill</em>
          </button>
        ))}
      </div>
      <div className="chat-layout">
        <aside className="conversation-list">
          <h2>会话</h2>
          {appConversations.length === 0 ? (
            <p>当前应用还没有会话。</p>
          ) : (
            appConversations.map((conversation) => (
              <button
                key={conversation.id}
                className={conversationID === conversation.id ? "active" : ""}
                onClick={() => void openConversation(conversation.id)}
              >
                <strong>{conversation.title}</strong>
                <small>{formatDate(conversation.updated_at)}</small>
              </button>
            ))
          )}
          <div className={`provider-state ${status?.llm_configured ? "ready" : "missing"}`}>
            <span />
            {status?.llm_configured ? `真实模型：${status.llm_model}` : "等待配置真实模型"}
          </div>
        </aside>
        <section className="chat-surface">
          <div className="message-list">
            {!conversationID ? (
              <div className="chat-empty">
                <strong>{currentApp?.name ?? "请选择 AI 应用"}</strong>
                <p>创建会话后发送问题，页面会逐段展示回答并生成可核验轨迹。</p>
              </div>
            ) : messages.length === 0 && !streaming ? (
              <div className="chat-empty">
                <strong>会话已创建</strong>
                <p>可以询问“磁铁能吸木块吗？”或“250 厘米等于多少米？”。</p>
              </div>
            ) : (
              messages.map((message) => <Message key={message.id} message={message} />)
            )}
            {streaming && (
              <Message
                message={{
                  id: -1,
                  conversation_id: conversationID ?? 0,
                  role: "assistant",
                  content: streaming,
                  created_at: new Date().toISOString(),
                }}
              />
            )}
          </div>
          {(meta || toolEvents.length > 0) && (
            <details className="live-trace" open>
              <summary>本次编排过程</summary>
              {meta && <JsonBlock value={meta} />}
              {toolEvents.map((event, index) => <JsonBlock key={index} value={event} />)}
            </details>
          )}
          <form className="chat-composer" onSubmit={submit}>
            <textarea
              value={question}
              onChange={(event) => setQuestion(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
                  event.preventDefault();
                  event.currentTarget.form?.requestSubmit();
                }
              }}
              placeholder={conversationID ? "输入 2～500 字的问题…" : "请先创建或选择会话"}
              disabled={!conversationID || sending}
              maxLength={500}
            />
            <button className="solid-button" disabled={!conversationID || !question.trim() || sending}>
              {sending ? "生成中…" : "发送"}
            </button>
          </form>
        </section>
      </div>
    </>
  );
}

function Message({ message }: { message: ConversationMessage }) {
  return (
    <article className={`platform-message ${message.role}`}>
      <span>{message.role === "user" ? "你" : "AI"}</span>
      <div>
        {message.role === "assistant" ? (
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
        ) : (
          <p>{message.content}</p>
        )}
      </div>
    </article>
  );
}

function AppsPanel({
  user,
  apps,
  templates,
  skills,
  materials,
  notify,
  onChanged,
}: {
  user: User;
  apps: AIApp[];
  templates: PromptTemplate[];
  skills: Skill[];
  materials: Material[];
  notify: (toast: Toast) => void;
  onChanged: () => Promise<void>;
}) {
  const [selectedID, setSelectedID] = useState(apps[0]?.id ?? 0);
  const selected = apps.find((app) => app.id === selectedID);
  const [draft, setDraft] = useState<AIApp | null>(selected ?? null);
  const [templateContent, setTemplateContent] = useState("");

  useEffect(() => {
    const next = apps.find((app) => app.id === selectedID) ?? apps[0] ?? null;
    setDraft(next ? { ...next, material_ids: [...next.material_ids], skill_ids: [...next.skill_ids] } : null);
    setSelectedID(next?.id ?? 0);
    const template = templates.find((item) => item.id === next?.prompt_template_id);
    setTemplateContent(template?.content ?? "");
  }, [apps, selectedID, templates]);

  const saveApp = async () => {
    if (!draft) return;
    try {
      await api(`/api/apps/${draft.id}`, {
        method: "PUT",
        body: JSON.stringify({
          name: draft.name,
          description: draft.description,
          prompt_template_id: draft.prompt_template_id,
          enabled: draft.enabled,
          material_ids: draft.material_ids,
          skill_ids: draft.skill_ids,
        }),
      });
      notify({ kind: "success", message: "AI 应用及绑定已保存" });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "保存失败" });
    }
  };

  const saveTemplate = async () => {
    const template = templates.find((item) => item.id === draft?.prompt_template_id);
    if (!template) return;
    try {
      await api(`/api/prompt-templates/${template.id}`, {
        method: "PUT",
        body: JSON.stringify({ name: template.name, content: templateContent }),
      });
      notify({ kind: "success", message: "提示词模板已保存，下次对话生效" });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "模板保存失败" });
    }
  };

  return (
    <>
      <SectionHeading
        eyebrow="MULTIPLE AI APPLICATIONS"
        title="多个 AI 应用与提示词模板"
        description="每个应用分别绑定一个模板、一组本班材料和一组 Skill；绑定关系在每次请求时重新读取。"
      />
      <div className="configuration-layout">
        <aside className="config-list">
          {apps.map((app) => (
            <button key={app.id} className={selectedID === app.id ? "active" : ""} onClick={() => setSelectedID(app.id)}>
              <strong>{app.name}</strong>
              <small>{app.enabled ? "已启用" : "已停用"} · 模板 {app.prompt_template_name}</small>
            </button>
          ))}
        </aside>
        {draft && (
          <section className="config-editor">
            <div className="form-grid">
              <label>
                <span>应用名称</span>
                <input value={draft.name} disabled={user.role !== "teacher"} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
              </label>
              <label>
                <span>提示词模板</span>
                <select
                  value={draft.prompt_template_id}
                  disabled={user.role !== "teacher"}
                  onChange={(event) => {
                    const id = Number(event.target.value);
                    setDraft({ ...draft, prompt_template_id: id });
                    setTemplateContent(templates.find((item) => item.id === id)?.content ?? "");
                  }}
                >
                  {templates.map((template) => <option key={template.id} value={template.id}>{template.name}</option>)}
                </select>
              </label>
              <label className="wide">
                <span>应用说明</span>
                <input value={draft.description} disabled={user.role !== "teacher"} onChange={(event) => setDraft({ ...draft, description: event.target.value })} />
              </label>
            </div>
            <label className="block-field">
              <span>模板正文</span>
              <textarea
                value={templateContent}
                disabled={user.role !== "teacher"}
                onChange={(event) => setTemplateContent(event.target.value)}
                rows={6}
              />
              <small>可使用 {"{{class_name}}"}、{"{{user_name}}"}、{"{{app_name}}"}；渲染结果会完整进入系统消息。</small>
            </label>
            <div className="binding-columns">
              <fieldset>
                <legend>绑定知识库材料</legend>
                {materials.map((material) => (
                  <label key={material.id}>
                    <input
                      type="checkbox"
                      disabled={user.role !== "teacher"}
                      checked={draft.material_ids.includes(material.id)}
                      onChange={() => setDraft({ ...draft, material_ids: toggleID(draft.material_ids, material.id) })}
                    />
                    <span>{material.title}</span>
                  </label>
                ))}
              </fieldset>
              <fieldset>
                <legend>绑定 Skill</legend>
                {skills.map((skill) => (
                  <label key={skill.id}>
                    <input
                      type="checkbox"
                      disabled={user.role !== "teacher"}
                      checked={draft.skill_ids.includes(skill.id)}
                      onChange={() => setDraft({ ...draft, skill_ids: toggleID(draft.skill_ids, skill.id) })}
                    />
                    <span>{skill.name} {!skill.enabled && "（停用）"}</span>
                  </label>
                ))}
              </fieldset>
            </div>
            {user.role === "teacher" && (
              <div className="editor-actions">
                <label className="toggle-row">
                  <input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} />
                  启用该应用
                </label>
                <button className="outline-button" onClick={() => void saveTemplate()}>保存模板</button>
                <button className="solid-button" onClick={() => void saveApp()}>保存应用与绑定</button>
              </div>
            )}
          </section>
        )}
      </div>
    </>
  );
}

function MaterialsPanel({
  user,
  materials,
  status,
  notify,
  onChanged,
}: {
  user: User;
  materials: Material[];
  status: PlatformStatus | null;
  notify: (toast: Toast) => void;
  onChanged: () => Promise<void>;
}) {
  const [file, setFile] = useState<File | null>(null);
  const upload = async () => {
    if (!file) return;
    const form = new FormData();
    form.append("file", file);
    try {
      await api("/api/materials", { method: "POST", body: form });
      setFile(null);
      notify({ kind: "success", message: "材料已上传并写入本班知识库" });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "上传失败" });
    }
  };
  return (
    <>
      <SectionHeading
        eyebrow="SCOPED KNOWLEDGE BASE"
        title="班级知识库"
        description={
          status?.embedding_configured
            ? `使用 ${status.embedding_model}（${status.embedding_dim} 维）与 Qdrant 执行关键字 + 向量 RRF 混合检索，只向 LLM 发送 Top-4 切片。`
            : "向量模型未配置，当前使用本地词法检索；只向 LLM 发送至多四个命中切片。"
        }
        action={user.role === "teacher" ? (
          <div className="file-action">
            <label className="file-picker">
              <input
                type="file"
                accept=".txt,.md"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <span className="file-picker-trigger">选择文件</span>
              <span className="file-picker-name">{file ? file.name : "支持 .txt / .md"}</span>
            </label>
            <button className="solid-button" disabled={!file} onClick={() => void upload()}>上传材料</button>
          </div>
        ) : undefined}
      />
      <div className="card-grid">
        {materials.map((material) => (
          <article className="info-card" key={material.id}>
            <p className="eyebrow">MATERIAL #{material.id}</p>
            <h2>{material.title}</h2>
            <p>{material.content_preview || "暂无预览"}</p>
            <footer>{material.original_name} · {formatBytes(material.size_bytes)}</footer>
          </article>
        ))}
      </div>
    </>
  );
}

function SkillsPanel({
  user,
  skills,
  notify,
  onChanged,
}: {
  user: User;
  skills: Skill[];
  notify: (toast: Toast) => void;
  onChanged: () => Promise<void>;
}) {
  const [file, setFile] = useState<File | null>(null);
  const upload = async () => {
    if (!file) return;
    const form = new FormData();
    form.append("file", file);
    try {
      await api("/api/skills/upload", { method: "POST", body: form });
      setFile(null);
      notify({ kind: "success", message: "Skill 已解析并上传" });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "Skill 上传失败" });
    }
  };
  const toggle = async (skill: Skill) => {
    try {
      await api(`/api/skills/${skill.id}`, {
        method: "PATCH",
        body: JSON.stringify({ enabled: !skill.enabled }),
      });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "状态更新失败" });
    }
  };
  return (
    <>
      <SectionHeading
        eyebrow="BUILT-IN & UPLOADED SKILLS"
        title="Skill 技能"
        description="内置与上传 Skill 使用同一解析、绑定和路由机制；一次请求至多注入一个命中的说明正文。"
        action={user.role === "teacher" ? (
          <div className="file-action">
            <label className="file-picker">
              <input
                type="file"
                accept=".txt,.md"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <span className="file-picker-trigger">选择文件</span>
              <span className="file-picker-name">{file ? file.name : "支持 .txt / .md"}</span>
            </label>
            <button className="solid-button" disabled={!file} onClick={() => void upload()}>上传 Skill</button>
          </div>
        ) : undefined}
      />
      <div className="card-grid">
        {skills.map((skill) => (
          <article className={`info-card skill-card ${skill.enabled ? "" : "disabled"}`} key={skill.id}>
            <div className="card-title-row">
              <p className="eyebrow">{skill.built_in ? "BUILT-IN" : "UPLOADED"}</p>
              <span className={skill.enabled ? "status-good" : "status-muted"}>{skill.enabled ? "启用" : "停用"}</span>
            </div>
            <h2>{skill.name}</h2>
            <p>{skill.description}</p>
            <dl>
              <div><dt>适用条件</dt><dd>{skill.trigger_text}</dd></div>
              <div><dt>资料范围</dt><dd>{skill.subject_text || "全部绑定材料"}</dd></div>
              <div><dt>工具</dt><dd>{skill.tools.join("、") || "无"}</dd></div>
            </dl>
            {user.role === "teacher" && <button className="outline-button" onClick={() => void toggle(skill)}>{skill.enabled ? "停用" : "启用"}</button>}
          </article>
        ))}
      </div>
    </>
  );
}

function MCPPanel({
  user,
  servers,
  tools,
  logs,
  notify,
  onChanged,
}: {
  user: User;
  servers: MCPServer[];
  tools: MCPTool[];
  logs: ToolCallLog[];
  notify: (toast: Toast) => void;
  onChanged: () => Promise<void>;
}) {
  const [showRegister, setShowRegister] = useState(false);
  const [form, setForm] = useState({ name: "", base_url: "", token: "" });
  const connect = async (server: MCPServer, action: "connect" | "disconnect") => {
    try {
      await api(`/api/mcp/servers/${server.id}/${action}`, { method: "POST" });
      notify({ kind: "success", message: action === "connect" ? "握手与工具发现成功" : "服务器已断开" });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "MCP 操作失败" });
      await onChanged();
    }
  };
  const register = async (event: FormEvent) => {
    event.preventDefault();
    try {
      await api("/api/mcp/servers", { method: "POST", body: JSON.stringify(form) });
      setForm({ name: "", base_url: "", token: "" });
      setShowRegister(false);
      notify({ kind: "success", message: "MCP Server 已登记，Token 未返回浏览器" });
      await onChanged();
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "登记失败" });
    }
  };
  return (
    <>
      <SectionHeading
        eyebrow="MODEL CONTEXT PROTOCOL"
        title="外部工具与 MCP 接入"
        description="连接包含握手和工具发现；模型只看到当前 Skill 允许的最小工具候选集，执行权始终在宿主。"
        action={user.role === "teacher" ? <button className="solid-button" onClick={() => setShowRegister(!showRegister)}>登记服务器</button> : undefined}
      />
      {showRegister && (
        <form className="register-form" onSubmit={register}>
          <input placeholder="服务器名，如 campus-tools" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
          <input placeholder="Base URL" value={form.base_url} onChange={(event) => setForm({ ...form, base_url: event.target.value })} required />
          <input type="password" placeholder="Bearer Token（仅发送到服务端）" value={form.token} onChange={(event) => setForm({ ...form, token: event.target.value })} required />
          <button className="solid-button">保存</button>
        </form>
      )}
      <div className="mcp-grid">
        <section className="panel">
          <h2>服务器</h2>
          {servers.map((server) => (
            <article className="server-row" key={server.id}>
              <div>
                <strong>{server.name}</strong>
                <small>{server.base_url}</small>
                {server.last_error && <em>{server.last_error}</em>}
              </div>
              <span className={server.connected ? "status-good" : "status-muted"}>{server.connected ? "已连接" : "未连接"}</span>
              {user.role === "teacher" && (
                <button className="outline-button" onClick={() => void connect(server, server.connected ? "disconnect" : "connect")}>
                  {server.connected ? "断开" : "连接并发现"}
                </button>
              )}
            </article>
          ))}
        </section>
        <section className="panel">
          <h2>发现的工具</h2>
          {tools.length === 0 ? <p className="muted">连接服务器后显示工具声明。</p> : tools.map((tool) => (
            <article className="tool-row" key={tool.id}>
              <strong>{tool.name}@{tool.server_name}</strong>
              <p>{tool.description}</p>
              <JsonBlock value={tool.input_schema} />
            </article>
          ))}
        </section>
      </div>
      <section className="panel log-panel">
        <h2>调用记录</h2>
        {logs.length === 0 ? <p className="muted">尚无工具调用。</p> : (
          <div className="table-scroll">
            <table>
              <thead><tr><th>时间</th><th>工具</th><th>轮次</th><th>状态</th><th>参数</th><th>结果 / 错误</th></tr></thead>
              <tbody>
                {logs.map((log) => (
                  <tr key={log.id}>
                    <td>{formatDate(log.created_at)}</td>
                    <td>{log.tool_ref}</td>
                    <td>{log.round}</td>
                    <td>{log.status}</td>
                    <td><code>{log.arguments_json}</code></td>
                    <td>{log.result_preview || log.error_message || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}

function TracePanel({ traces, notify }: { traces: TraceSummary[]; notify: (toast: Toast) => void }) {
  const [selectedID, setSelectedID] = useState<number | null>(traces[0]?.id ?? null);
  const [detail, setDetail] = useState<TraceDetail | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (traces.length === 0) {
      setSelectedID(null);
      return;
    }
    if (!selectedID || !traces.some((trace) => trace.id === selectedID)) {
      setSelectedID(traces[0].id);
    }
  }, [selectedID, traces]);

  useEffect(() => {
    if (!selectedID) {
      setDetail(null);
      return;
    }
    setLoading(true);
    api<{ trace: TraceDetail }>(`/api/traces/${selectedID}`)
      .then(({ trace }) => setDetail(trace))
      .catch((error) => notify({ kind: "error", message: error instanceof Error ? error.message : "轨迹读取失败" }))
      .finally(() => setLoading(false));
  }, [notify, selectedID]);

  const retrieval = detail?.retrieval ?? {};
  const route = detail?.skill_route ?? {};
  const llmRequests = detail?.llm_requests ?? [];
  const llmResponses = detail?.llm_responses ?? [];
  const mcpEvents = detail?.mcp_events ?? [];
  return (
    <>
      <SectionHeading
        eyebrow="ACCEPTANCE EVIDENCE"
        title="LLM 调用验证"
        description="所有结论均来自实际保存的请求、返回、检索与工具事件；认证信息不会进入轨迹。"
      />
      <div className="trace-layout">
        <aside className="trace-list">
          {traces.length === 0 ? <p>完成一次真实对话后生成轨迹。</p> : traces.map((trace) => (
            <button key={trace.id} className={selectedID === trace.id ? "active" : ""} onClick={() => setSelectedID(trace.id)}>
              <strong>#{trace.id} · {trace.app_name}</strong>
              <span>{trace.question}</span>
              <small>{formatDate(trace.created_at)} · {trace.username}</small>
            </button>
          ))}
        </aside>
        <section className="trace-detail">
          {loading ? <p>正在读取轨迹…</p> : !detail ? <p>请选择一条调用轨迹。</p> : (
            <>
              <header>
                <p className="eyebrow">TRACE #{detail.id}</p>
                <h2>{detail.question}</h2>
                <p>{detail.app_name} · {detail.template_name} · {detail.selected_skill_name || "未命中 Skill"}</p>
              </header>
              <div className="verification-grid">
                <Verification
                  number="01"
                  title="LLM 参数与提示词模板"
                  conclusion={`模板“${detail.template_name}”已渲染并进入 system 消息；共记录 ${llmRequests.length} 次真实 LLM 请求。`}
                >
                  <p><strong>原模板：</strong>{detail.template_source}</p>
                  <p><strong>渲染结果：</strong>{detail.template_rendered}</p>
                  <JsonBlock value={llmRequests} />
                  <JsonBlock value={llmResponses} />
                </Verification>
                <Verification
                  number="02"
                  title="知识库是否全部传递"
                  conclusion={`否。使用 ${String(retrieval.method || "检索器")}（${String(retrieval.embedding_model || "本地")}）检查 ${String(retrieval.bound_sources_examined ?? 0)} 份绑定材料，仅向 LLM 发送 ${String(retrieval.chunks_sent ?? 0)} 个 Top-K 命中切片。`}
                >
                  <JsonBlock value={retrieval} />
                </Verification>
                <Verification
                  number="03"
                  title="Skill 是否全部传递"
                  conclusion={detail.selected_skill_name
                    ? `只传递被选 Skill“${detail.selected_skill_name}”的完整说明正文；原文件元数据不作为提示词传递。`
                    : "本次未命中 Skill，因此没有 Skill 正文进入 LLM。"}
                >
                  <p>{detail.skill_injected || "无注入内容"}</p>
                  <JsonBlock value={route} />
                </Verification>
                <Verification
                  number="04"
                  title="Skill 执行逻辑"
                  conclusion="候选集限定为应用已绑定且启用的 Skill；按适用条件词法计分，零分不选，同分取编号较小者，一次至多一个。"
                >
                  <JsonBlock value={route.candidates ?? []} />
                </Verification>
                <Verification
                  number="05"
                  title="MCP 传给 LLM 的参数"
                  conclusion={mcpEvents.length > 0
                    ? `LLM tools 声明触发了 ${mcpEvents.length} 个 MCP 事件；宿主完成参数校验和执行。`
                    : "本次请求没有实际执行 MCP；可使用解题引导助手询问单位换算后再次查看。"}
                >
                  <JsonBlock value={mcpEvents} />
                </Verification>
              </div>
            </>
          )}
        </section>
      </div>
    </>
  );
}

function Verification({
  number,
  title,
  conclusion,
  children,
}: {
  number: string;
  title: string;
  conclusion: string;
  children: React.ReactNode;
}) {
  return (
    <article className="verification-card">
      <div><span>{number}</span><h3>{title}</h3></div>
      <p className="verification-conclusion">{conclusion}</p>
      <details>
        <summary>查看本次真实证据</summary>
        <div>{children}</div>
      </details>
    </article>
  );
}

function JsonBlock({ value }: { value: unknown }) {
  return <pre className="json-block">{JSON.stringify(value, null, 2)}</pre>;
}

function toggleID(values: number[], id: number) {
  return values.includes(id) ? values.filter((item) => item !== id) : [...values, id];
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(bytes < 10240 ? 1 : 0)} KB`;
}
