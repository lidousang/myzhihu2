<script setup>
import { ref, watch } from "vue";
import { api } from "../api";
import { session } from "../session";
import { topicLabels } from "../meta";

const props = defineProps({
  content: { type: Object, required: true },
  source: { type: String, default: "" },
  initialLikeCount: { type: Number, default: 0 },
  initialCollectionCount: { type: Number, default: 0 },
  initialLiked: { type: Boolean, default: false },
  initialCollected: { type: Boolean, default: false },
  showCounts: { type: Boolean, default: true },
  via: { type: String, default: "" },
  // null hides the follow button (for example on the question page).
  followingAuthor: { type: Boolean, default: null },
});
const emit = defineEmits(["open", "updated", "followed"]);
const followBusy = ref(false);

const likeCount = ref(props.initialLikeCount);
const collectionCount = ref(props.initialCollectionCount);
const liked = ref(props.initialLiked);
const collected = ref(props.initialCollected);
const working = ref(false);
const actionError = ref("");

watch(() => [props.initialLikeCount, props.initialCollectionCount, props.initialLiked, props.initialCollected], ([likes, collections, isLiked, isCollected]) => {
  likeCount.value = likes;
  collectionCount.value = collections;
  liked.value = isLiked;
  collected.value = isCollected;
});

const sourceLabel = {
  following: "来自关注",
  interest: "兴趣推荐",
  explore: "发现内容",
};
const viaLabel = { push: "推送", pull: "拉取" };
const sourceHint = {
  push: "作者是普通用户：发帖时内容被推送到你的收件箱（写扩散）",
  pull: "作者是大 V：内容只存在作者发件箱，你刷新时被拉取并合并（读扩散）",
  interest: "你没有关注这位作者，但你点赞过相似主题或这位作者的内容",
  explore: "从近期内容候选池中随机发现",
};

async function toggleFollow() {
  if (followBusy.value) return;
  followBusy.value = true;
  actionError.value = "";
  try {
    await api.setFollow(props.content.author.id, !props.followingAuthor);
    emit("followed", { authorID: props.content.author.id, following: !props.followingAuthor });
  } catch (error) {
    actionError.value = error.message;
  } finally {
    followBusy.value = false;
  }
}

function openContent() {
  emit("open", props.content);
}

async function toggleReaction(kind) {
  if (!session.token) {
    actionError.value = "请先登录后再互动。";
    return;
  }
  if (working.value) return;
  working.value = true;
  actionError.value = "";
  const wasEnabled = kind === "like" ? liked.value : collected.value;
  try {
    const state = await api.setReaction(props.content.id, kind, !wasEnabled);
    likeCount.value = state.like_count;
    collectionCount.value = state.collection_count;
    liked.value = state.liked;
    collected.value = state.collected;
    emit("updated", { contentID: props.content.id, state });
  } catch (error) {
    actionError.value = error.message;
  } finally {
    working.value = false;
  }
}
</script>

<template>
  <article class="content-card">
    <div class="content-meta">
      <span class="content-kind">{{ content.kind === "question" ? "问题" : content.kind === "answer" ? "回答" : "文章" }}</span>
      <span v-if="source" class="source-label" :title="sourceHint[via || source]">
        {{ sourceLabel[source] || source }}<template v-if="via"> · {{ viaLabel[via] }}</template>
      </span>
      <span v-if="content.topic" class="topic-label">{{ topicLabels[content.topic] || content.topic }}</span>
      <span class="meta-dot">·</span>
      <span>{{ content.author?.display_name || content.author?.username || "匿名用户" }}</span>
      <span v-if="content.author?.is_bigv" class="bigv-badge" title="粉丝数超过阈值，内容以拉模式分发">大V</span>
      <button
        v-if="followingAuthor !== null && content.author?.id !== session.user?.id"
        class="follow-button" :class="{ active: followingAuthor }" type="button" :disabled="followBusy" @click="toggleFollow"
      >{{ followingAuthor ? "已关注" : "+ 关注" }}</button>
      <span class="meta-dot">·</span>
      <time>{{ content.created_at ? new Date(content.created_at * 1000).toLocaleDateString() : "刚刚" }}</time>
    </div>
    <button class="content-title" type="button" @click="openContent">
      {{ content.title || (content.kind === "answer" ? "回答" : "查看内容") }}
    </button>
    <button class="content-body" type="button" @click="openContent">
      {{ content.body }}
    </button>
    <div class="content-actions">
      <button class="reaction-button" :class="{ active: liked }" :disabled="working" type="button" @click="toggleReaction('like')">
        ▲ {{ liked ? "已赞同" : "赞同" }}<span v-if="showCounts"> {{ likeCount }}</span>
      </button>
      <button class="reaction-button" :class="{ active: collected }" :disabled="working" type="button" @click="toggleReaction('collect')">
        ☆ {{ collected ? "已收藏" : "收藏" }}<span v-if="showCounts"> {{ collectionCount }}</span>
      </button>
      <button class="text-button" type="button" @click="openContent">查看详情</button>
    </div>
    <p v-if="actionError" class="inline-error">{{ actionError }}</p>
  </article>
</template>
