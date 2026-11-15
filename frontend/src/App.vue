<script setup>
import { computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { clearSession, session } from "./session";
import { loadMeta, meta } from "./meta";

const router = useRouter();
const isBigV = computed(() => meta.demo_enabled && session.user?.id === meta.bigv_user_id);
const roleLabel = computed(() => {
  if (!meta.demo_enabled || !session.user) return "";
  if (session.user.id === meta.bigv_user_id) return "演示大 V";
  if (session.user.id === meta.reader_user_id) return "演示普通用户";
  return "";
});

onMounted(loadMeta);

function logout() {
  clearSession();
  router.push("/login");
}
</script>

<template>
  <header class="topbar">
    <div class="topbar-inner">
      <div class="topbar-left">
        <RouterLink class="logo" to="/">知乎</RouterLink>
        <nav class="main-nav" aria-label="主导航">
          <RouterLink to="/">推荐</RouterLink>
          <RouterLink to="/">关注</RouterLink>
          <RouterLink to="/">热榜</RouterLink>
          <RouterLink to="/">圈子</RouterLink>
        </nav>
      </div>
      <div class="topbar-right">
        <label class="search-box">
          <span class="sr-only">搜索</span>
          <input type="search" placeholder="搜索你想了解的内容…" disabled>
        </label>
        <template v-if="session.token">
          <RouterLink v-if="isBigV" class="primary-button small-button" to="/compose">发布</RouterLink>
          <span v-if="roleLabel" class="role-badge">{{ roleLabel }}</span>
          <span class="welcome-name">{{ session.user?.display_name || session.user?.username || "已登录" }}</span>
          <button class="text-button" type="button" @click="logout">切换身份</button>
        </template>
        <template v-else>
          <RouterLink class="text-button" to="/login">登录</RouterLink>
          <RouterLink class="primary-button small-button" to="/register">注册</RouterLink>
        </template>
      </div>
    </div>
  </header>

  <RouterView />
</template>
