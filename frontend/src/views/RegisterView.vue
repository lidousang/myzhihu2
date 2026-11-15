<script setup>
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "../api";
import { saveSession } from "../session";
import { loadMeta, meta } from "../meta";

const router = useRouter();
const username = ref("");
const displayName = ref("");
const password = ref("");
const confirmPassword = ref("");
const busy = ref(false);
const errorMessage = ref("");

onMounted(loadMeta);

async function submitRegistration() {
  errorMessage.value = "";
  if (password.value !== confirmPassword.value) {
    errorMessage.value = "两次输入的密码不一致。";
    return;
  }
  const passwordBytes = new TextEncoder().encode(password.value).length;
  if (passwordBytes < 8 || passwordBytes > 72) {
    errorMessage.value = "密码需为 8–72 个 UTF-8 字节。";
    return;
  }

  busy.value = true;
  try {
    const account = { username: username.value.trim(), password: password.value };
    if (displayName.value.trim()) account.display_name = displayName.value.trim();
    const auth = await api.register(account);
    saveSession(auth);
    router.replace("/");
  } catch (error) {
    errorMessage.value = error.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <main class="auth-page">
    <section class="surface auth-card">
      <p class="eyebrow">加入知乎风格 Demo</p>
      <h1>创建你的账号</h1>
      <p class="auth-intro">注册后即可发布内容、参与讨论并获取个性化推荐。</p>
      <p v-if="meta.loaded && !meta.registration_enabled" class="form-message notice-panel">
        演示环境暂未开放注册，提交会被服务器拒绝。请返回<RouterLink to="/login">登录页</RouterLink>使用演示账号体验。
      </p>
      <form class="auth-form" @submit.prevent="submitRegistration">
        <label for="register-username">用户名</label>
        <input id="register-username" v-model="username" autocomplete="username" required minlength="3" maxlength="64" placeholder="3–64 个字符">
        <label for="register-display-name">显示名称 <span class="optional-label">选填</span></label>
        <input id="register-display-name" v-model="displayName" autocomplete="nickname" maxlength="64" placeholder="留空时使用用户名">
        <label for="register-password">密码</label>
        <input id="register-password" v-model="password" type="password" autocomplete="new-password" required minlength="8" maxlength="72" placeholder="至少 8 个字符">
        <label for="register-confirm">确认密码</label>
        <input id="register-confirm" v-model="confirmPassword" type="password" autocomplete="new-password" required>
        <p v-if="errorMessage" class="form-message error-panel">{{ errorMessage }}</p>
        <button class="primary-button full-button" type="submit" :disabled="busy">
          {{ busy ? "正在创建账号…" : "注册" }}
        </button>
      </form>
      <p class="auth-footer">已有账号？<RouterLink to="/login">登录</RouterLink></p>
    </section>
  </main>
</template>
