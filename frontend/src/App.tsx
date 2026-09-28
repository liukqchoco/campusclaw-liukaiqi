import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type User = {
  id: number;
  username: string;
  display_name: string;
  role: "teacher" | "student";
  class_id: number;
  class_name: string;
};

type Material = {
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

type Toast = {
  kind: "success" | "error";
  message: string;
};

class RequestError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });
  const value = (await response.json().catch(() => ({}))) as {
    error?: string;
  };
  if (!response.ok) {
    throw new RequestError(response.status, value.error ?? "请求失败，请稍后重试");
  }
  return value as T;
}

function Icon({
  name,
  size = 20,
}: {
  name: "search" | "grid" | "list" | "sun" | "moon" | "upload" | "logout" | "file" | "download" | "close" | "command";
  size?: number;
}) {
  const paths = {
    search: <><circle cx="11" cy="11" r="7" /><path d="m20 20-4-4" /></>,
    grid: <><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><rect x="14" y="14" width="7" height="7" rx="1" /></>,
    list: <><path d="M8 6h13M8 12h13M8 18h13" /><path d="M3 6h.01M3 12h.01M3 18h.01" /></>,
    sun: <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.42 1.42M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.42-1.42M17.66 6.34l1.41-1.41" /></>,
    moon: <path d="M20.4 15.2A9 9 0 0 1 8.8 3.6 9 9 0 1 0 20.4 15.2Z" />,
    upload: <><path d="M12 16V4M7 9l5-5 5 5" /><path d="M4 15v5h16v-5" /></>,
    logout: <><path d="M10 17l5-5-5-5M15 12H3" /><path d="M14 3h5a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-5" /></>,
    file: <><path d="M6 2h8l4 4v16H6z" /><path d="M14 2v5h5" /></>,
    download: <><path d="M12 3v12M7 10l5 5 5-5" /><path d="M4 20h16" /></>,
    close: <path d="m6 6 12 12M18 6 6 18" />,
    command: <><path d="M9 6V5a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v14a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3z" /></>,
  };

  return (
    <svg
      aria-hidden="true"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {paths[name]}
    </svg>
  );
}

export default function App() {
  const [sessionState, setSessionState] = useState<"checking" | "guest" | "authenticated">("checking");
  const [user, setUser] = useState<User | null>(null);
  const [materials, setMaterials] = useState<Material[]>([]);
  const [selectedMaterial, setSelectedMaterial] = useState<Material | null>(null);
  const [loadingMaterials, setLoadingMaterials] = useState(false);
  const [search, setSearch] = useState("");
  const [theme, setTheme] = useState<"light" | "dark">(() =>
    localStorage.getItem("campusclaw-theme") === "dark" ? "dark" : "light",
  );
  const [view, setView] = useState<"grid" | "list">(() =>
    localStorage.getItem("campusclaw-view") === "list" ? "list" : "grid",
  );
  const [toast, setToast] = useState<Toast | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const [uploadOpen, setUploadOpen] = useState(false);

  const notify = useCallback((nextToast: Toast) => {
    setToast(nextToast);
    window.setTimeout(() => setToast(null), 3200);
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("campusclaw-theme", theme);
  }, [theme]);

  useEffect(() => {
    localStorage.setItem("campusclaw-view", view);
  }, [view]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((current) => !current);
      }
      if (event.key === "Escape") {
        setCommandOpen(false);
        setSelectedMaterial(null);
        setUploadOpen(false);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    api<{ user: User }>("/api/me")
      .then(({ user: currentUser }) => {
        setUser(currentUser);
        setSessionState("authenticated");
      })
      .catch(() => setSessionState("guest"));
  }, []);

  const loadMaterials = useCallback(async (query: string) => {
    setLoadingMaterials(true);
    try {
      const params = new URLSearchParams();
      if (query.trim()) params.set("q", query.trim());
      const suffix = params.size ? `?${params.toString()}` : "";
      const response = await api<{ materials: Material[] }>(`/api/materials${suffix}`);
      setMaterials(response.materials);
    } catch (error) {
      if (error instanceof RequestError && error.status === 401) {
        setUser(null);
        setSessionState("guest");
      } else {
        notify({ kind: "error", message: error instanceof Error ? error.message : "材料加载失败" });
      }
    } finally {
      setLoadingMaterials(false);
    }
  }, [notify]);

  useEffect(() => {
    if (!user) return;
    const timeout = window.setTimeout(() => void loadMaterials(search), 260);
    return () => window.clearTimeout(timeout);
  }, [loadMaterials, search, user]);

  const handleLogin = async (username: string, password: string) => {
    const response = await api<{ user: User }>("/api/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    setUser(response.user);
    setSessionState("authenticated");
    notify({ kind: "success", message: `欢迎回来，${response.user.display_name}` });
  };

  const handleLogout = async () => {
    try {
      await api("/api/logout", { method: "POST" });
    } finally {
      setUser(null);
      setMaterials([]);
      setSessionState("guest");
      setCommandOpen(false);
    }
  };

  const openMaterial = async (id: number) => {
    try {
      const response = await api<{ material: Material }>(`/api/materials/${id}`);
      setSelectedMaterial(response.material);
    } catch (error) {
      notify({ kind: "error", message: error instanceof Error ? error.message : "材料详情加载失败" });
    }
  };

  if (sessionState === "checking") {
    return (
      <div className="boot-screen">
        <BrandMark />
        <div className="boot-line" />
        <p>正在确认登录状态</p>
      </div>
    );
  }

  if (sessionState === "guest" || !user) {
    return <LoginScreen onLogin={handleLogin} theme={theme} setTheme={setTheme} />;
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand-lockup">
          <BrandMark />
          <div>
            <strong>CampusClaw</strong>
            <span>教研知识库</span>
          </div>
        </div>
        <div className="topbar-actions">
          <button className="command-trigger" onClick={() => setCommandOpen(true)}>
            <Icon name="search" size={16} />
            <span>快捷操作</span>
            <kbd>⌘ K</kbd>
          </button>
          <button
            className="icon-button"
            aria-label={theme === "light" ? "切换深色主题" : "切换浅色主题"}
            onClick={() => setTheme(theme === "light" ? "dark" : "light")}
          >
            <Icon name={theme === "light" ? "moon" : "sun"} />
          </button>
          <div className="user-chip">
            <span className="avatar">{user.display_name.slice(0, 1)}</span>
            <span className="user-copy">
              <strong>{user.display_name}</strong>
              <small>{user.role === "teacher" ? "教师" : "学生"} · {user.class_name}</small>
            </span>
          </div>
          <button className="icon-button" aria-label="退出登录" onClick={() => void handleLogout()}>
            <Icon name="logout" />
          </button>
        </div>
      </header>

      <main className="workspace">
        <section className="workspace-heading">
          <div>
            <p className="section-label">CLASS KNOWLEDGE BASE</p>
            <h1>{user.class_name}资料库</h1>
            <p>本班共收录 <strong>{materials.length}</strong> 份材料，所有内容均经过服务端权限校验。</p>
          </div>
          {user.role === "teacher" && (
            <button className="primary-button" onClick={() => setUploadOpen(true)}>
              <Icon name="upload" size={18} />
              上传材料
            </button>
          )}
        </section>

        <section className="content-panel">
          <div className="panel-toolbar">
            <label className="search-field">
              <Icon name="search" size={18} />
              <input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="搜索本班材料"
                aria-label="搜索本班材料"
              />
              {search && <button onClick={() => setSearch("")} aria-label="清空搜索">×</button>}
            </label>
            <div className="view-switch" aria-label="切换材料视图">
              <button className={view === "grid" ? "active" : ""} onClick={() => setView("grid")} aria-label="网格视图">
                <Icon name="grid" size={17} />
              </button>
              <button className={view === "list" ? "active" : ""} onClick={() => setView("list")} aria-label="列表视图">
                <Icon name="list" size={18} />
              </button>
            </div>
          </div>

          {loadingMaterials ? (
            <div className="loading-grid">
              <span /><span /><span />
            </div>
          ) : materials.length === 0 ? (
            <div className="empty-state">
              <div className="empty-file"><Icon name="file" size={28} /></div>
              <h2>{search ? "没有匹配的材料" : "本班还没有材料"}</h2>
              <p>{search ? "换一个关键词试试" : user.role === "teacher" ? "上传第一份教学材料吧" : "等待教师上传教学材料"}</p>
            </div>
          ) : (
            <div className={`materials ${view}`}>
              {materials.map((material) => (
                <MaterialCard
                  key={material.id}
                  material={material}
                  view={view}
                  onOpen={() => void openMaterial(material.id)}
                />
              ))}
            </div>
          )}
        </section>
      </main>

      {selectedMaterial && (
        <MaterialDialog
          material={selectedMaterial}
          onClose={() => setSelectedMaterial(null)}
        />
      )}

      {uploadOpen && (
        <UploadDialog
          onClose={() => setUploadOpen(false)}
          onUploaded={() => {
            setUploadOpen(false);
            notify({ kind: "success", message: "材料已上传并写入知识库" });
            void loadMaterials(search);
          }}
          onError={(message) => notify({ kind: "error", message })}
        />
      )}

      {commandOpen && (
        <CommandPalette
          canUpload={user.role === "teacher"}
          onClose={() => setCommandOpen(false)}
          onUpload={() => {
            setCommandOpen(false);
            setUploadOpen(true);
          }}
          onTheme={() => {
            setTheme(theme === "light" ? "dark" : "light");
            setCommandOpen(false);
          }}
          onView={() => {
            setView(view === "grid" ? "list" : "grid");
            setCommandOpen(false);
          }}
          onLogout={() => void handleLogout()}
        />
      )}

      {toast && <div className={`toast ${toast.kind}`} role="status">{toast.message}</div>}
    </div>
  );
}

function BrandMark() {
  return <span className="brand-mark" aria-hidden="true">C</span>;
}

function LoginScreen({
  onLogin,
  theme,
  setTheme,
}: {
  onLogin: (username: string, password: string) => Promise<void>;
  theme: "light" | "dark";
  setTheme: (theme: "light" | "dark") => void;
}) {
  const [username, setUsername] = useState("teacher_a");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await onLogin(username, password);
    } catch (nextError) {
      setError(nextError instanceof Error ? nextError.message : "登录失败");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="login-layout">
      <section className="login-story">
        <div className="brand-lockup login-brand">
          <BrandMark />
          <div><strong>CampusClaw</strong><span>教研知识库</span></div>
        </div>
        <div className="story-copy">
          <p className="section-label">KNOWLEDGE WITH BOUNDARIES</p>
          <h1>知识共享，<br />边界清晰。</h1>
          <p>面向班级的教研材料底座。每一次访问，都经过身份、角色和班级三重校验。</p>
        </div>
        <div className="story-metrics">
          <div><strong>2</strong><span>种角色</span></div>
          <div><strong>3</strong><span>重边界</span></div>
          <div><strong>1</strong><span>可信入口</span></div>
        </div>
      </section>

      <section className="login-form-side">
        <button
          className="icon-button theme-corner"
          onClick={() => setTheme(theme === "light" ? "dark" : "light")}
          aria-label="切换主题"
        >
          <Icon name={theme === "light" ? "moon" : "sun"} />
        </button>
        <form className="login-card" onSubmit={submit}>
          <div>
            <p className="section-label">WELCOME BACK</p>
            <h2>登录 CampusClaw</h2>
            <p>使用课程环境中的预置账号继续。</p>
          </div>
          <label>
            <span>用户名</span>
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              required
            />
          </label>
          <label>
            <span>密码</span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              placeholder="输入预置账号密码"
              required
            />
          </label>
          {error && <p className="form-error" role="alert">{error}</p>}
          <button className="primary-button login-button" disabled={submitting}>
            {submitting ? "正在登录…" : "登录"}
          </button>
          <div className="account-hint">
            <button type="button" onClick={() => setUsername("teacher_a")}>教师 teacher_a</button>
            <button type="button" onClick={() => setUsername("student_a1")}>学生 student_a1</button>
            <button type="button" onClick={() => setUsername("student_b1")}>学生 student_b1</button>
          </div>
          <p className="privacy-note">登录状态保存在 HttpOnly Cookie 中，页面不会保存角色或班级信息。</p>
        </form>
      </section>
    </main>
  );
}

function MaterialCard({
  material,
  view,
  onOpen,
}: {
  material: Material;
  view: "grid" | "list";
  onOpen: () => void;
}) {
  const extension = material.original_name.split(".").pop()?.toUpperCase() ?? "FILE";
  return (
    <article className="material-card" onClick={onOpen}>
      <div className="file-badge"><Icon name="file" size={22} /><span>{extension}</span></div>
      <div className="material-copy">
        <div className="material-meta">
          <span>{material.class_name}</span>
          <span>{formatDate(material.created_at)}</span>
        </div>
        <h2>{material.title}</h2>
        {view === "grid" && <p>{material.content_preview || "暂无内容预览"}</p>}
        <div className="material-footer">
          <span>{material.uploader_name}</span>
          <span>{formatBytes(material.size_bytes)}</span>
        </div>
      </div>
      {view === "list" && <span className="list-open">查看</span>}
    </article>
  );
}

function MaterialDialog({ material, onClose }: { material: Material; onClose: () => void }) {
  return (
    <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="modal detail-modal" role="dialog" aria-modal="true" aria-label={`${material.title}详情`}>
        <header className="modal-header">
          <div>
            <p className="section-label">{material.class_name}</p>
            <h2>{material.title}</h2>
            <span>{material.original_name} · {formatBytes(material.size_bytes)} · {formatDate(material.created_at)}</span>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="关闭详情"><Icon name="close" /></button>
        </header>
        <div className="markdown-body">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{material.content ?? ""}</ReactMarkdown>
        </div>
        <footer className="modal-footer">
          <span>上传者：{material.uploader_name}</span>
          <a className="secondary-button" href={`/api/materials/${material.id}/file`}>
            <Icon name="download" size={17} />下载原文件
          </a>
        </footer>
      </section>
    </div>
  );
}

function UploadDialog({
  onClose,
  onUploaded,
  onError,
}: {
  onClose: () => void;
  onUploaded: () => void;
  onError: (message: string) => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [progress, setProgress] = useState(0);
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const upload = (event: FormEvent) => {
    event.preventDefault();
    if (!file) return;
    setUploading(true);

    const form = new FormData();
    form.append("file", file);
    const request = new XMLHttpRequest();
    request.open("POST", "/api/materials");
    request.withCredentials = true;
    request.upload.addEventListener("progress", (progressEvent) => {
      if (progressEvent.lengthComputable) {
        setProgress(Math.round((progressEvent.loaded / progressEvent.total) * 100));
      }
    });
    request.addEventListener("load", () => {
      setUploading(false);
      if (request.status === 201) {
        onUploaded();
        return;
      }
      try {
        const body = JSON.parse(request.responseText) as { error?: string };
        onError(body.error ?? "上传失败");
      } catch {
        onError("上传失败");
      }
    });
    request.addEventListener("error", () => {
      setUploading(false);
      onError("网络错误，上传失败");
    });
    request.send(form);
  };

  return (
    <div className="modal-backdrop" onMouseDown={(event) => !uploading && event.target === event.currentTarget && onClose()}>
      <form className="modal upload-modal" onSubmit={upload}>
        <header className="modal-header">
          <div>
            <p className="section-label">TEACHER ONLY</p>
            <h2>上传教学材料</h2>
            <span>文件会解析并写入本班知识库</span>
          </div>
          <button type="button" className="icon-button" onClick={onClose} disabled={uploading} aria-label="关闭上传">
            <Icon name="close" />
          </button>
        </header>
        <button type="button" className="drop-zone" onClick={() => inputRef.current?.click()}>
          <span className="upload-circle"><Icon name="upload" size={25} /></span>
          <strong>{file ? file.name : "选择 .txt 或 .md 文件"}</strong>
          <small>{file ? formatBytes(file.size) : "最大 1 MB，内容须为非空 UTF-8 文本"}</small>
        </button>
        <input
          ref={inputRef}
          className="sr-only"
          type="file"
          accept=".txt,.md,text/plain,text/markdown"
          onChange={(event) => setFile(event.target.files?.[0] ?? null)}
        />
        {uploading && <div className="progress-track"><span style={{ width: `${progress}%` }} /></div>}
        <footer className="modal-footer upload-actions">
          <button type="button" className="text-button" onClick={onClose} disabled={uploading}>取消</button>
          <button className="primary-button" disabled={!file || uploading}>
            {uploading ? `上传中 ${progress}%` : "上传并入库"}
          </button>
        </footer>
      </form>
    </div>
  );
}

function CommandPalette({
  canUpload,
  onClose,
  onUpload,
  onTheme,
  onView,
  onLogout,
}: {
  canUpload: boolean;
  onClose: () => void;
  onUpload: () => void;
  onTheme: () => void;
  onView: () => void;
  onLogout: () => void;
}) {
  const commands = useMemo(() => [
    ...(canUpload ? [{ label: "上传教学材料", icon: "upload" as const, action: onUpload }] : []),
    { label: "切换深浅色主题", icon: "sun" as const, action: onTheme },
    { label: "切换列表 / 网格视图", icon: "grid" as const, action: onView },
    { label: "退出当前账号", icon: "logout" as const, action: onLogout },
  ], [canUpload, onLogout, onTheme, onUpload, onView]);

  return (
    <div className="modal-backdrop command-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="command-palette" role="dialog" aria-modal="true" aria-label="快捷操作">
        <div className="command-heading"><Icon name="command" /><span>快捷操作</span><kbd>ESC</kbd></div>
        <div>
          {commands.map((command) => (
            <button key={command.label} onClick={command.action}>
              <Icon name={command.icon} size={18} />
              <span>{command.label}</span>
            </button>
          ))}
        </div>
      </section>
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(bytes < 10240 ? 1 : 0)} KB`;
}
