# myzhihu2

基于 Go + Vue 3 的知乎风格内容社区后端，含账号、问答与文章、评论、点赞收藏、关注、个性化 Feed 流，支持推拉结合的大 V 扇出、Outbox 可靠消息、Redis 投影缓存、RabbitMQ 异步消费、Docker Compose + Nginx + HTTPS 部署与 GitHub Actions CI/CD。

## 功能

| 模块 | 功能 |
|------|------|
| 账号 | 注册、登录、JWT（HS256）鉴权、bcrypt 密码哈希、个人资料、并发注册冲突返回 409 |
| 内容 | 问题、文章、回答、评论统一存储；5 个话题；作者编辑与软删除；回答与评论分页 |
| 互动 | 点赞、收藏、关注（幂等）；浏览与阅读时长行为上报 |
| Feed | 关注流 + 兴趣流 + 探索流三路召回；按内容 ID 游标分页；推荐来源标注 |
| 大 V | 推拉结合：粉丝数 ≥ 阈值的作者只写发件箱，读取时与收件箱归并；阈值滞回（≥T 升级，<0.8T 降级）；只推活跃粉丝；Pipeline 批量写入；收件箱限长 1000 |
| 可靠消息 | Outbox 与业务数据同事务写入，后台投递 + Publisher Confirm；消费端手动 ack，Redis Lua 去重保证幂等 |
| 缓存 | Redis ZSET 收件箱/发件箱/探索池，singleflight 合并并发重建，ready 标记区分"空"与"未构建"，可随时从 MySQL 重建 |
| 演示模式 | 一键体验登录（读者/大 V）、关闭公开注册（403）、演示账号限权与发帖频率限制、种子数据每日重置 |
| 工程 | Docker Compose、Nginx 反向代理与限流、Let's Encrypt HTTPS、GitHub Actions CI/CD（失败自动回滚） |

## 生产部署（Docker Compose）

部署文件位于 `deployment/prod/`：`compose.yml`、`backend/Dockerfile`、`nginx/default.conf`（HTTP，用于申请证书）、`nginx/default.https.conf`（HTTPS）。Nginx 配置中的 `example.com` 需替换为实际域名。

```bash
cd deployment/prod
# 准备 .env 与 backend/settings.prod.yaml（含凭据，不入库，见"配置"）
docker compose up -d mysql redis rabbitmq
docker compose build backend
docker compose run --rm backend -f /app/settings.yaml -db   # 建表
docker compose run --rm seed                                # 演示数据
docker compose up -d backend nginx
```

完整步骤（服务器初始化、DNS、证书、定时任务、踩坑记录）见 `deploy_record.md`。

## 本地开发

```bash
# 启动依赖（仅绑定 127.0.0.1）
docker compose -f deployment/compose.middleware.yml up -d

# 后端（需自行准备 backend/settings/settings.yaml，含本地凭据，已加入 .gitignore）
cd backend
go run ./cmd -f settings/settings.yaml -db        # 建表后退出
go run ./cmd/seed -f settings/settings.yaml -profile demo -seed 20261107 -reset
go run ./cmd -f settings/settings.yaml

# 前端（Vite 将 /api 代理到 127.0.0.1:8080）
cd frontend
npm install && npm run dev                        # http://127.0.0.1:5173
```

## CI/CD

GitHub Actions 配置位于 `.github/workflows/ci-cd.yml`。

- 触发：Pull Request 只运行 CI；推送到 `main` 运行 CI 后部署（`production` 环境，可设人工审批）
- 后端：Go（版本取自 `go.mod`），执行 `go vet ./...`、`go test -race -count=1 ./...`，交叉编译 `server`、`seed`
- 前端：Node.js 22，执行 `npm ci`、`npm run build`
- 部署：SSH 上传到服务器暂存目录，执行 `deployment/scripts/remote-deploy.sh`：备份 → 替换 → 重建后端 → 健康检查，失败自动回滚，成功记录到 `releases.log`
- 所需 Secrets（`production` 环境）：`DEPLOY_HOST`、`DEPLOY_PASSWORD`、`DEPLOY_KNOWN_HOSTS`，可选 `DEPLOY_USER`、`DEPLOY_PORT`；业务凭据只保存在服务器上，不经过 CI

## 接口清单

公共前缀 `/api/v1`。

### 账号
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST | `/auth/register` | 否 | 注册；演示环境关闭，返回 403 |
| POST | `/auth/login` | 否 | 登录，返回 access_token |
| POST | `/auth/demo-login` | 否 | 一键体验：`role` 为 `reader` 或 `bigv` |
| GET | `/meta` | 否 | 公开配置：演示模式、是否开放注册、大 V 阈值 |
| GET | `/me` | JWT | 当前用户资料 |
| PUT | `/me` | JWT | 更新资料（演示账号禁止） |
| GET | `/users/:userID` | 否 | 用户公开资料 |

### 内容
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST | `/contents` | JWT | 发布问题或文章（需选择话题） |
| GET | `/contents/:contentID` | 否 | 内容详情 |
| PUT | `/contents/:contentID` | JWT | 作者编辑 |
| DELETE | `/contents/:contentID` | JWT | 作者软删除 |
| GET | `/users/:userID/contents` | 否 | 用户主页内容（`kind` 过滤，分页） |
| GET | `/questions/:questionID/answers` | 否 | 回答列表（分页） |
| POST | `/questions/:questionID/answers` | JWT | 发布回答 |
| GET | `/contents/:contentID/comments` | 否 | 评论列表（分页） |
| POST | `/contents/:contentID/comments` | JWT | 发布评论 |
| GET | `/contents/:contentID/fanout` | JWT | 扇出统计：推/拉模式、Redis 写入次数、耗时 |

### 互动
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST / DELETE | `/contents/:contentID/like` | JWT | 点赞 / 取消 |
| POST / DELETE | `/contents/:contentID/collection` | JWT | 收藏 / 取消 |
| POST / DELETE | `/users/:userID/follow` | JWT | 关注 / 取关 |
| POST | `/behaviors` | JWT | 行为上报：`view`、`read_time` |

### Feed
| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | `/feed?cursor=&limit=` | JWT | 个性化首页：三路召回 + 推拉归并，ID 游标分页（limit ≤ 50） |

## 配置

后端读取 YAML 配置（`-f` 指定路径），主要配置段：

| 配置段 | 关键项 | 说明 |
|--------|--------|------|
| `system` | `ip`、`port` | 容器内须为 `0.0.0.0` |
| `db` | `user`、`passport`、`host`、`db_name`、`conn_max_lifetime` | 最大连接数固定为 10，连接失败重试 10 次 |
| `redis` | `host`、`password`、`db` | Feed 投影所在库，种子程序会清空该库 |
| `rabbitmq` | `exchange`、`queue`、`feed_queue` 及路由键 | 交换机与队列启动时自动声明 |
| `jwt` | `secret`、`expire_hours` | 密钥至少 32 字符，生产须使用新密钥 |
| `feed` | `bigv_threshold`、`inbox_limit`、`outbox_limit`、`fanout_batch` | 演示环境阈值 100，压测建议 5000 |
| `demo` | `enabled`、`registration_enabled`、`reader_user_id`、`bigv_user_id`、`posts_per_hour` | 演示模式开关与限制 |

生产环境的 Compose 另需 `deployment/prod/.env`：`MYSQL_ROOT_PASSWORD`、`DB_USER`、`DB_PASSWORD`、`REDIS_PASSWORD`、`RMQ_USER`、`RMQ_PASSWORD`。

## 运维与可观测性

- 仅 Nginx 暴露 80/443；MySQL、Redis、后端不对外，RabbitMQ 管理台只绑定 `127.0.0.1:15672`（SSH 隧道访问）。
- Nginx 限流：接口 10 次/秒/IP，登录类接口 10 次/分钟/IP，超限返回 429；请求体上限 256 KB。
- 定时任务：每周续期证书、每日备份数据库（保留 7 天）、每日重置演示数据。
- 部署健康检查使用 `GET /api/v1/meta`；发布记录写入服务器 `releases.log`。
- Redis 用于 Feed 收件箱/发件箱、大 V 名单、探索池、兴趣权重、扇出统计；均可从 MySQL 重建。
