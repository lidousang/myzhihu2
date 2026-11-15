import { createRouter, createWebHistory } from "vue-router";
import HomeView from "./views/HomeView.vue";
import QuestionView from "./views/QuestionView.vue";
import LoginView from "./views/LoginView.vue";
import RegisterView from "./views/RegisterView.vue";
import ComposeView from "./views/ComposeView.vue";
import { session } from "./session";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", name: "home", component: HomeView },
    { path: "/question/:id", name: "question", component: QuestionView },
    { path: "/compose", name: "compose", component: ComposeView },
    { path: "/login", name: "login", component: LoginView, meta: { public: true } },
    { path: "/register", name: "register", component: RegisterView, meta: { public: true } },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
  scrollBehavior() {
    return { top: 0 };
  },
});

// Every page except login and register requires a session. This is a user
// experience rule; the API enforces authentication independently.
router.beforeEach((to) => {
  if (!to.meta.public && !session.token) {
    return { name: "login", query: to.fullPath === "/" ? {} : { redirect: to.fullPath } };
  }
  return true;
});

export default router;
