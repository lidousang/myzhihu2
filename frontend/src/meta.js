import { reactive } from "vue";
import { api } from "./api";

// Public deployment settings from GET /api/v1/meta: whether the demo is on,
// whether registration is open, the big V threshold and the demo account IDs.
export const meta = reactive({
  loaded: false,
  demo_enabled: false,
  registration_enabled: true,
  bigv_threshold: 0,
  reader_user_id: 0,
  bigv_user_id: 0,
});

let pending;

export function loadMeta() {
  if (!pending) {
    pending = api.meta()
      .then((data) => Object.assign(meta, data, { loaded: true }))
      .catch(() => { pending = undefined; });
  }
  return pending;
}

export const topicLabels = {
  tech: "科技",
  food: "美食",
  travel: "旅行",
  photo: "摄影",
  fitness: "健身",
};
