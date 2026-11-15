import { reactive } from "vue";

function readUser() {
  try {
    return JSON.parse(localStorage.getItem("current_user") || "null");
  } catch (_) {
    return null;
  }
}

export const session = reactive({
  token: localStorage.getItem("access_token") || "",
  user: readUser(),
});

export function saveSession(auth) {
  session.token = auth.access_token;
  session.user = auth.user;
  localStorage.setItem("access_token", auth.access_token);
  localStorage.setItem("current_user", JSON.stringify(auth.user));
}

export function clearSession() {
  session.token = "";
  session.user = null;
  localStorage.removeItem("access_token");
  localStorage.removeItem("current_user");
}
