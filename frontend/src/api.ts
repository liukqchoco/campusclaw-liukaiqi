const ACCESS_TOKEN_KEY = "campusclaw_access_token";

export class RequestError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export function getAccessToken() {
  return sessionStorage.getItem(ACCESS_TOKEN_KEY);
}

export function saveAccessToken(token: string) {
  sessionStorage.setItem(ACCESS_TOKEN_KEY, token);
}

export function clearAccessToken() {
  sessionStorage.removeItem(ACCESS_TOKEN_KEY);
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const accessToken = getAccessToken();
  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers,
  });
  const value = (await response.json().catch(() => ({}))) as { error?: string };
  if (!response.ok) {
    throw new RequestError(response.status, value.error ?? "请求失败，请稍后重试");
  }
  return value as T;
}

export type StreamEventName = "meta" | "tool" | "delta" | "done" | "error";

export async function streamConversationMessage(
  conversationID: number,
  content: string,
  onEvent: (event: StreamEventName, data: unknown) => void,
) {
  const headers = new Headers({ "Content-Type": "application/json", Accept: "text/event-stream" });
  const accessToken = getAccessToken();
  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }
  const response = await fetch(`/api/conversations/${conversationID}/messages`, {
    method: "POST",
    credentials: "same-origin",
    headers,
    body: JSON.stringify({ content }),
  });
  if (!response.ok) {
    const value = (await response.json().catch(() => ({}))) as { error?: string };
    throw new RequestError(response.status, value.error ?? "对话请求失败");
  }
  if (!response.body) {
    throw new Error("浏览器未获得流式响应");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done }).replaceAll("\r\n", "\n");
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      dispatchFrame(frame, onEvent);
      boundary = buffer.indexOf("\n\n");
    }
    if (done) break;
  }
  if (buffer.trim()) {
    dispatchFrame(buffer, onEvent);
  }
}

function dispatchFrame(
  frame: string,
  onEvent: (event: StreamEventName, data: unknown) => void,
) {
  let event: StreamEventName = "delta";
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event:")) {
      event = line.slice(6).trim() as StreamEventName;
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }
  if (dataLines.length === 0) return;
  const raw = dataLines.join("\n");
  try {
    onEvent(event, JSON.parse(raw));
  } catch {
    onEvent(event, raw);
  }
}
