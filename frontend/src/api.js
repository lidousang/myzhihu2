const API_ROOT = "/api/v1";

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  const token = localStorage.getItem("access_token");
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_ROOT}${path}`, { ...options, headers });
  if (response.status === 204) return null;

  let payload;
  try {
    payload = await response.json();
  } catch (_) {
    payload = null;
  }

  if (!response.ok) {
    // An expired or invalid token sends the user back to the login page. Auth
    // endpoints are excluded so a failed login shows its own error message.
    if (response.status === 401 && !path.startsWith("/auth/")) {
      localStorage.removeItem("access_token");
      localStorage.removeItem("current_user");
      if (window.location.pathname !== "/login") window.location.assign("/login");
    }
    throw new ApiError(
      payload?.error || payload?.message || `请求失败（${response.status}）`,
      response.status,
    );
  }
  return payload?.data ?? payload;
}

// fetchRaw returns the whole JSON body ({data, pending}) for endpoints whose
// envelope carries more than data.
async function fetchRaw(path) {
  const token = localStorage.getItem("access_token");
  const response = await fetch(`${API_ROOT}${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok) throw new ApiError(payload?.error || `请求失败（${response.status}）`, response.status);
  return payload;
}

export const api = {
  meta() {
    return request("/meta");
  },
  demoLogin(role) {
    return request("/auth/demo-login", {
      method: "POST",
      body: JSON.stringify({ role }),
    });
  },
  createContent(content) {
    return request("/contents", {
      method: "POST",
      body: JSON.stringify(content),
    });
  },
  getFanout(contentID) {
    return fetchRaw(`/contents/${encodeURIComponent(contentID)}/fanout`);
  },
  setFollow(userID, enabled) {
    return request(`/users/${encodeURIComponent(userID)}/follow`, {
      method: enabled ? "POST" : "DELETE",
    });
  },
  login(credentials) {
    return request("/auth/login", {
      method: "POST",
      body: JSON.stringify(credentials),
    });
  },
  register(account) {
    return request("/auth/register", {
      method: "POST",
      body: JSON.stringify(account),
    });
  },
  getFeed({ cursor = "", limit = 5 } = {}) {
    const query = new URLSearchParams({ limit: String(limit) });
    if (cursor) query.set("cursor", cursor);
    return request(`/feed?${query.toString()}`);
  },
  getContent(id) {
    return request(`/contents/${encodeURIComponent(id)}`);
  },
  getAnswers(questionID, page = 1, limit = 5) {
    const query = new URLSearchParams({ page: String(page), limit: String(limit) });
    return request(`/questions/${encodeURIComponent(questionID)}/answers?${query}`);
  },
  getComments(contentID, page = 1, limit = 10) {
    const query = new URLSearchParams({ page: String(page), limit: String(limit) });
    return request(`/contents/${encodeURIComponent(contentID)}/comments?${query}`);
  },
  createComment(contentID, body) {
    return request(`/contents/${encodeURIComponent(contentID)}/comments`, {
      method: "POST",
      body: JSON.stringify({ body }),
    });
  },
  setReaction(contentID, kind, enabled) {
    const action = kind === "like" ? "like" : "collection";
    return request(`/contents/${encodeURIComponent(contentID)}/${action}`, {
      method: enabled ? "POST" : "DELETE",
    });
  },
};
