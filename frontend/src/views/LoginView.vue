<script setup>
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../api";
import { saveSession } from "../session";
import { loadMeta, meta } from "../meta";

const route = useRoute();
const router = useRouter();
const username = ref("");
const password = ref("");
const busy = ref("");
const errorMessage = ref("");

const roles = [
  {
    role: "reader",
    title: "以普通用户体验",
    name: "路过的读者",
    points: [
      "首页混合“关注 / 兴趣 / 发现”三种来源，每条都有标签",
      "关注了两位大 V：他们的内容在刷新时“拉取”",
      "关注了六位作者：他们的内容被“推送”进收件箱",
      "点赞过摄影作品，所以会推荐未关注的摄影作者",
    ],
  },
  {
    role: "bigv",
    title: "以大 V 体验",
    name: "极客老K · 约 300 位粉丝",
    points: [
      "演示环境中粉丝数超过 100 即为大 V",
      "发布一篇内容，查看它是以“拉模式”只写入 1 次",
      "再切换到普通用户，新内容会出现在首页最上方",
      "预置内容受保护，只能修改自己新发布的内容",
    ],
  },
];

onMounted(loadMeta);

function redirectTarget() {
  return typeof route.query.redirect === "string" ? route.query.redirect : "/";
}

async function demoLogin(role) {
  errorMessage.value = "";
  busy.value = role;
  try {
    saveSession(await api.demoLogin(role));
    router.replace(role === "bigv" && redirectTarget() === "/" ? "/compose" : redirectTarget());
  } catch (error) {
    errorMessage.value = error.message;
  } finally {
    busy.value = "";
  }
}

async function submitLogin() {
  errorMessage.value = "";
  busy.value = "password";
  try {
    saveSession(await api.login({ username: username.value.trim(), password: password.value }));
    router.replace(redirectTarget());
  } catch (error) {
    errorMessage.value = error.message;
  } finally {
    busy.value = "";
  }
}
</script>

<template>
  <main class="auth-page">
    <section class="login-wrap">
      <header class="login-header">
        <p class="eyebrow">内容社区 Feed 演示</p>
        <h1>选择一个身份开始体验</h1>
        <p class="auth-intro">
          这是一个演示环境：数据每天凌晨重置，不开放注册。两个演示账号可以直接登录。
        </p>
      </header>

      <div class="role-grid">
        <article v-for="item in roles" :key="item.role" class="surface role-card">
          <h2>{{ item.title }}</h2>
          <p class="role-name">{{ item.name }}</p>
          <ul>
            <li v-for="point in item.points" :key="point">{{ point }}</li>
          </ul>
          <button class="primary-button full-button" type="button" :disabled="Boolean(busy)" @click="demoLogin(item.role)">
            {{ busy === item.role ? "正在登录…" : item.title }}
          </button>
        </article>
      </div>

      <p v-if="errorMessage" class="form-message error-panel">{{ errorMessage }}</p>

      <details class="surface password-login">
        <summary>使用账号密码登录</summary>
        <form class="auth-form" @submit.prevent="submitLogin">
          <label for="login-username">用户名</label>
          <input id="login-username" v-model="username" autocomplete="username" required minlength="3" maxlength="64">
          <label for="login-password">密码</label>
          <input id="login-password" v-model="password" type="password" autocomplete="current-password" required maxlength="72">
          <button class="outline-button full-button" type="submit" :disabled="Boolean(busy)">
            {{ busy === "password" ? "正在登录…" : "登录" }}
          </button>
        </form>
      </details>

      <p class="auth-footer">
        还没有账号？<RouterLink to="/register">注册</RouterLink>
        <span v-if="meta.loaded && !meta.registration_enabled">（演示环境暂未开放）</span>
      </p>
    </section>
  </main>
</template>
