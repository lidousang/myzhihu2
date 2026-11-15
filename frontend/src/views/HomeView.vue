<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import ContentCard from "../components/ContentCard.vue";
import { ApiError, api } from "../api";
import { clearSession, session } from "../session";
import { meta } from "../meta";

const PAGE_SIZE = 5;
const router = useRouter();
const items = ref([]);
const cursor = ref("");
const hasMore = ref(true);
const loading = ref(false);
const loadError = ref("");
const sentinel = ref(null);
let observer;

const loginRequired = computed(() => !session.token);

async function loadNextPage() {
  if (loading.value || !hasMore.value || !session.token) return;
  loading.value = true;
  loadError.value = "";
  try {
    const page = await api.getFeed({ cursor: cursor.value, limit: PAGE_SIZE });
    items.value.push(...(page.items || []));
    cursor.value = page.next_cursor || "";
    hasMore.value = Boolean(page.has_more && cursor.value);
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      clearSession();
    }
    loadError.value = error.message || "Feed 加载失败。";
  } finally {
    loading.value = false;
  }
}

async function refreshFeed() {
  items.value = [];
  cursor.value = "";
  hasMore.value = true;
  loadError.value = "";
  await loadNextPage();
  window.scrollTo({ top: 0, behavior: "smooth" });
}

function openContent(content) {
  const questionID = content.kind === "answer" && content.parent_id ? content.parent_id : content.id;
  router.push(`/question/${questionID}`);
}

// Following changes take effect asynchronously (outbox -> consumer); update the
// buttons now and let the next refresh show the new following stream.
function updateFollow({ authorID, following }) {
  for (const item of items.value) {
    if (item.content.author?.id === authorID) item.following_author = following;
  }
}

const isReader = computed(() => meta.demo_enabled && session.user?.id === meta.reader_user_id);

function updateReaction({ contentID, state }) {
  const target = items.value.find((item) => item.content.id === contentID);
  if (!target) return;
  target.like_count = state.like_count;
  target.collection_count = state.collection_count;
  target.liked = state.liked;
  target.collected = state.collected;
}

onMounted(() => {
  if (session.token) loadNextPage();
  observer = new IntersectionObserver((entries) => {
    if (entries.some((entry) => entry.isIntersecting)) loadNextPage();
  }, { rootMargin: "320px 0px" });
  if (sentinel.value) observer.observe(sentinel.value);
});

onBeforeUnmount(() => observer?.disconnect());
</script>

<template>
  <main class="page-shell">
    <div class="feed-layout">
      <section class="feed-column">
        <div class="feed-tabs">
          <div>
            <span class="feed-tab active">推荐</span>
            <span class="feed-tab muted-tab">关注 · 兴趣 · 发现</span>
          </div>
          <button class="text-button refresh-button" type="button" :disabled="loading" @click="refreshFeed">
            ↻ 刷新
          </button>
        </div>

        <div v-if="loginRequired" class="surface login-prompt">
          <p class="eyebrow">个性化首页</p>
          <h1>登录后查看推荐内容</h1>
          <p>主页 Feed 由关注、兴趣和发现内容组成，需要登录令牌。</p>
          <div class="prompt-actions">
            <RouterLink class="primary-button" to="/login">登录</RouterLink>
            <RouterLink class="outline-button" to="/register">创建账号</RouterLink>
          </div>
        </div>

        <template v-else>
          <ContentCard
            v-for="item in items"
            :key="item.content.id"
            :content="item.content"
            :source="item.source"
            :initial-like-count="item.like_count"
            :initial-collection-count="item.collection_count"
            :initial-liked="item.liked"
            :initial-collected="item.collected"
            :via="item.via || ''"
            :following-author="item.following_author"
            @open="openContent"
            @updated="updateReaction"
            @followed="updateFollow"
          />

          <div v-if="loading" class="surface feed-status">正在加载内容…</div>
          <div v-else-if="loadError" class="surface feed-status error-panel">
            <p>{{ loadError }}</p>
            <button class="outline-button" type="button" @click="loadNextPage">重试</button>
          </div>
          <div v-else-if="!items.length" class="surface feed-status">还没有可展示的内容。</div>
          <div v-else-if="!hasMore" class="feed-end">已经看到当前内容的末尾</div>
          <div v-if="hasMore" ref="sentinel" class="feed-sentinel" aria-hidden="true"></div>
        </template>
      </section>

      <aside class="side-column">
        <section class="surface side-card">
          <h2>{{ isReader ? "你正在以普通用户体验" : "首页内容从哪里来" }}</h2>
          <p><b>来自关注 · 推送</b>：普通作者发帖时写入你的收件箱。</p>
          <p><b>来自关注 · 拉取</b>：大 V（粉丝 ≥ {{ meta.bigv_threshold || "阈值" }}）的内容在你刷新时合并进来。</p>
          <p><b>兴趣推荐</b>：未关注作者中，与你点赞过的主题或作者相关的内容。</p>
          <p><b>发现内容</b>：近期内容候选池中的随机样本。</p>
          <p class="muted">把鼠标停在标签上可以看到说明；切换到大 V 身份可以发帖。</p>
        </section>
        <section class="surface side-card">
          <h2>本页加载方式</h2>
          <p>每次从后端读取 5 条；向下滚动到底部后，使用游标继续读取。</p>
        </section>
      </aside>
    </div>
  </main>
</template>
