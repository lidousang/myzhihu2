<script setup>
import { onBeforeUnmount, onMounted, ref } from "vue";
import { api } from "../api";
import { loadMeta, meta, topicLabels } from "../meta";

const kind = ref("article");
const topic = ref("tech");
const title = ref("");
const body = ref("");
const busy = ref(false);
const errorMessage = ref("");
const created = ref(null);
const fanout = ref(null);
const waiting = ref(false);
let timer;

onMounted(loadMeta);
onBeforeUnmount(() => clearTimeout(timer));

// The post is delivered asynchronously: outbox relay -> RabbitMQ -> consumer.
// Poll the fan-out statistics the consumer records in Redis.
function pollFanout(contentID, attempt = 0) {
  waiting.value = true;
  timer = setTimeout(async () => {
    try {
      const result = await api.getFanout(contentID);
      if (!result.pending) {
        fanout.value = result.data;
        waiting.value = false;
        return;
      }
    } catch (_) {
      /* keep polling until the attempt limit */
    }
    if (attempt < 20) pollFanout(contentID, attempt + 1);
    else waiting.value = false;
  }, 500);
}

async function submit() {
  errorMessage.value = "";
  fanout.value = null;
  busy.value = true;
  try {
    created.value = await api.createContent({
      kind: kind.value, topic: topic.value, title: title.value.trim(), body: body.value.trim(),
    });
    title.value = "";
    body.value = "";
    pollFanout(created.value.id);
  } catch (error) {
    errorMessage.value = error.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <main class="page-shell">
    <div class="compose-layout">
      <section class="surface compose-card">
        <p class="eyebrow">发布内容</p>
        <h1>写点什么</h1>
        <form class="auth-form" @submit.prevent="submit">
          <div class="compose-row">
            <label>类型
              <select v-model="kind">
                <option value="article">文章</option>
                <option value="question">问题</option>
              </select>
            </label>
            <label>主题
              <select v-model="topic">
                <option v-for="(label, value) in topicLabels" :key="value" :value="value">{{ label }}</option>
              </select>
            </label>
          </div>
          <label for="compose-title">标题</label>
          <input id="compose-title" v-model="title" required maxlength="255" placeholder="一句话说明主题">
          <label for="compose-body">正文</label>
          <textarea id="compose-body" v-model="body" required maxlength="20000" rows="8" placeholder="正文内容"></textarea>
          <p v-if="errorMessage" class="form-message error-panel">{{ errorMessage }}</p>
          <button class="primary-button full-button" type="submit" :disabled="busy">{{ busy ? "正在发布…" : "发布" }}</button>
        </form>
      </section>

      <aside class="surface compose-side">
        <h2>这条内容会怎样送达粉丝？</h2>
        <p v-if="!created">
          粉丝数达到 {{ meta.bigv_threshold || "阈值" }} 的作者是大 V：内容只写入自己的发件箱，粉丝刷新时再拉取；
          普通作者的内容会推送到每个在线粉丝的收件箱。
        </p>
        <template v-else>
          <p class="fanout-title">已发布：{{ created.title }}（ID {{ created.id }}）</p>
          <p v-if="waiting && !fanout" class="muted">等待异步投递：Outbox → RabbitMQ → 消费者…</p>
          <div v-if="fanout" class="fanout-result" :class="fanout.mode">
            <p class="fanout-mode">{{ fanout.mode === "pull" ? "拉模式（大 V）" : "推模式（普通作者）" }}</p>
            <ul>
              <li>作者粉丝数：{{ fanout.followers }}</li>
              <li v-if="fanout.mode === 'pull'">Redis 写入：{{ fanout.redis_writes }} 次（只写作者发件箱）</li>
              <li v-if="fanout.mode === 'pull'">如果用推模式，需要写入 {{ fanout.followers }} 个收件箱</li>
              <li v-if="fanout.mode === 'push'">推送给在线粉丝：{{ fanout.active_targets }} 人，Redis 写入 {{ fanout.redis_writes }} 次</li>
              <li>消费者处理耗时：{{ fanout.duration_ms }} ms</li>
            </ul>
          </div>
          <p class="muted">切换到“普通用户”身份，这条内容会出现在首页最上方，标签为“来自关注 · 拉取”。</p>
          <RouterLink class="outline-button" to="/">回到首页</RouterLink>
        </template>
      </aside>
    </div>
  </main>
</template>
