<script setup>
import { computed, ref, watch } from "vue";
import { useRoute } from "vue-router";
import ContentCard from "../components/ContentCard.vue";
import { ApiError, api } from "../api";
import { session } from "../session";

const route = useRoute();
const content = ref(null);
const answers = ref([]);
const comments = ref([]);
const answerPage = ref(1);
const answerTotal = ref(0);
const answerLimit = 5;
const loading = ref(false);
const answersLoading = ref(false);
const commentText = ref("");
const commentBusy = ref(false);
const errorMessage = ref("");
const commentMessage = ref("");

const isQuestion = computed(() => content.value?.kind === "question");
const hasMoreAnswers = computed(() => answers.value.length < answerTotal.value);

async function loadContent() {
  const id = route.params.id;
  loading.value = true;
  errorMessage.value = "";
  content.value = null;
  answers.value = [];
  comments.value = [];
  answerPage.value = 1;
  try {
    content.value = await api.getContent(id);
    if (content.value.kind === "question") await loadAnswers(true);
    await loadComments();
  } catch (error) {
    errorMessage.value = error.message;
  } finally {
    loading.value = false;
  }
}

async function loadAnswers(reset = false) {
  if (!content.value || content.value.kind !== "question" || answersLoading.value) return;
  answersLoading.value = true;
  const requestedPage = reset ? 1 : answerPage.value + 1;
  try {
    const result = await api.getAnswers(content.value.id, requestedPage, answerLimit);
    if (reset) answers.value = result.items || [];
    else answers.value.push(...(result.items || []));
    answerPage.value = requestedPage;
    answerTotal.value = result.total || 0;
  } catch (error) {
    errorMessage.value = error.message;
  } finally {
    answersLoading.value = false;
  }
}

async function loadComments() {
  if (!content.value) return;
  try {
    const result = await api.getComments(content.value.id, 1, 10);
    comments.value = result.items || [];
  } catch (_) {
    // Comments are supplementary; keep the main content and answer list usable.
  }
}

async function submitComment() {
  if (!session.token) {
    commentMessage.value = "请先登录后再评论。";
    return;
  }
  const body = commentText.value.trim();
  if (!body || commentBusy.value) return;
  commentBusy.value = true;
  commentMessage.value = "";
  try {
    const created = await api.createComment(content.value.id, body);
    comments.value.unshift(created);
    commentText.value = "";
    commentMessage.value = "评论已发布。";
  } catch (error) {
    commentMessage.value = error instanceof ApiError ? error.message : "评论发布失败。";
  } finally {
    commentBusy.value = false;
  }
}

watch(() => route.params.id, loadContent, { immediate: true });
</script>

<template>
  <main class="page-shell detail-shell">
    <div v-if="loading" class="surface detail-status">正在加载内容…</div>
    <div v-else-if="errorMessage && !content" class="surface detail-status error-panel">{{ errorMessage }}</div>
    <template v-else-if="content">
      <section class="surface detail-heading">
        <div class="detail-topic">{{ isQuestion ? "问题详情" : content.kind === "article" ? "文章详情" : "内容详情" }}</div>
        <h1>{{ content.title || "回答" }}</h1>
        <p class="detail-author">
          {{ content.author?.display_name || content.author?.username || "匿名用户" }}
          <span class="meta-dot">·</span>
          {{ new Date(content.created_at * 1000).toLocaleString() }}
        </p>
        <p v-if="isQuestion" class="question-body">{{ content.body }}</p>
      </section>

      <div class="detail-layout">
        <section class="detail-main">
          <ContentCard
            v-if="!isQuestion"
            :content="content"
            :show-counts="false"
          />

          <template v-if="isQuestion">
            <div class="section-heading">
              <h2>{{ answerTotal }} 个回答</h2>
              <span>按最新内容展示</span>
            </div>
            <ContentCard
              v-for="answer in answers"
              :key="answer.id"
              :content="answer"
              :show-counts="false"
            />
            <div v-if="answersLoading" class="surface detail-status">正在加载回答…</div>
            <button v-else-if="hasMoreAnswers" class="outline-button load-answers" type="button" @click="loadAnswers()">
              加载更多回答
            </button>
            <div v-else-if="!answers.length" class="surface detail-status">暂时还没有回答。</div>
          </template>

          <section class="surface comments-panel">
            <div class="section-heading">
              <h2>评论</h2>
              <span>{{ comments.length }} 条</span>
            </div>
            <form class="comment-form" @submit.prevent="submitComment">
              <textarea v-model="commentText" rows="3" maxlength="2000" placeholder="写下你的评论…"></textarea>
              <div class="comment-form-footer">
                <p v-if="commentMessage" class="comment-message">{{ commentMessage }}</p>
                <button class="primary-button" type="submit" :disabled="commentBusy || !commentText.trim()">
                  {{ commentBusy ? "发布中…" : "发布评论" }}
                </button>
              </div>
            </form>
            <article v-for="comment in comments" :key="comment.id" class="comment-item">
              <strong>{{ comment.author?.display_name || comment.author?.username || "匿名用户" }}</strong>
              <p>{{ comment.body }}</p>
              <time>{{ new Date(comment.created_at * 1000).toLocaleString() }}</time>
            </article>
            <p v-if="!comments.length" class="empty-comments">还没有评论，来发表第一条吧。</p>
          </section>
        </section>

        <aside class="detail-aside">
          <section class="surface side-card">
            <h2>内容信息</h2>
            <p>类型：{{ content.kind }}</p>
            <p>作者：{{ content.author?.display_name || content.author?.username || "匿名用户" }}</p>
            <RouterLink class="text-button" to="/">返回推荐首页</RouterLink>
          </section>
        </aside>
      </div>
      <p v-if="errorMessage" class="inline-error">{{ errorMessage }}</p>
    </template>
  </main>
</template>
