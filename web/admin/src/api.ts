export type ApiResult<T> = T;

const API_PREFIX = "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = sessionStorage.getItem("bagualu_admin_token");
  const response = await fetch(`${API_PREFIX}${path}`, {
    ...init,
    headers: { Accept: "application/json", "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}), ...(init?.headers || {}) },
  });
  if (response.status === 401) {
    sessionStorage.removeItem("bagualu_admin_token");
    window.dispatchEvent(new Event("bagualu-auth-expired"));
  }
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
  return response.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: "POST", body: body === undefined ? undefined : JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) => request<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  remove: <T>(path: string) => request<T>(path, { method: "DELETE" }),
  upload: async <T>(path: string, file: File) => {
    const token = sessionStorage.getItem("bagualu_admin_token");
    const form = new FormData();
    form.append("file", file);
    const response = await fetch(`${API_PREFIX}${path}`, { method: "POST", headers: token ? { Authorization: `Bearer ${token}` } : {}, body: form });
    if (response.status === 401) {
      sessionStorage.removeItem("bagualu_admin_token");
      window.dispatchEvent(new Event("bagualu-auth-expired"));
    }
    if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
    return response.json() as Promise<T>;
  },
  auth: {
    login: async (username: string, password: string) => {
      const result = await request<{ token: string }>("/auth/login", { method: "POST", body: JSON.stringify({ username, password }) });
      sessionStorage.setItem("bagualu_admin_token", result.token);
      return result;
    },
    me: () => request<{ username: string }>("/auth/me"),
    logout: async () => {
      await request<void>("/auth/logout", { method: "POST" });
      sessionStorage.removeItem("bagualu_admin_token");
    },
  },
};

export type CoreStatus = { version?: string; running?: boolean; status?: string; pid?: number; error?: string; state?: string; error_code?: string };
export type CoreInstallStatus = { installed?: boolean; path?: string; version?: string; architecture?: string; source?: string; error?: string };
export type Summary = { node_count?: number; group_count?: number; upstream_count?: number; node_status_counts?: Record<string, number>; queue_length?: number; running?: boolean; core?: CoreStatus; traffic?: Record<string, number> };
