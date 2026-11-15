# myzhihu2 上线部署记录与学习笔记

> 部署时间：2026-11-08 ～ 2026-11-10
> 最终形态：香港云服务器（Ubuntu 22.04，x86_64）+ Docker Compose + Nginx + Let's Encrypt HTTPS
> 访问地址：`https://example.com`
> 本文按实际执行顺序记录每一步的命令、配置文件及其含义，最后附语法速查和踩坑总结。
> 文中不包含任何真实密码、密钥和服务器 IP，用 `<服务器IP>`、`<随机密码>` 等占位符表示。

---

## 目录

- 第 0 部分：整体架构与部署思路
- 第 1 部分：上线前的代码修改
- 第 2 部分：本地准备（编译、目录、配置文件）
- 第 3 部分：SSH 密钥与服务器登录
- 第 4 部分：域名与 DNS
- 第 5 部分：服务器初始化（时区、swap、防火墙、Docker）
- 第 6 部分：上传与启动服务
- 第 7 部分：申请 HTTPS 证书并切换配置
- 第 8 部分：定时任务（续期、备份、重置）
- 第 9 部分：日常运维与更新
- 第 10 部分：踩坑记录与排查方法
- 第 11 部分：语法与工具速查

---

## 第 0 部分：整体架构与部署思路

```
浏览器 ──HTTPS(443)──▶ Nginx 容器 ──┬── 静态文件（Vue 打包产物）
         HTTP(80)→301跳转           └── /api/ 反向代理 ──▶ backend 容器（Go/Gin，8080）
                                                             ├──▶ mysql 容器（3306）
                                                             ├──▶ redis 容器（6379）
                                                             └──▶ rabbitmq 容器（5672）
```

| 设计决定 | 原因 |
|---|---|
| 程序在**本地编译**、前端在**本地打包**，只上传产物 | 2G 内存的服务器编译 Go/Node 容易内存不足；服务器上不需要源码和工具链 |
| 镜像在**服务器上拉取** | 本地不需要下载 1.5GB 中间件镜像；香港服务器访问 Docker Hub 快 |
| 只有 Nginx 映射端口（80、443） | 数据库、缓存、消息队列、后端都不对公网开放 |
| 用 Let's Encrypt 免费证书，不用 Cloudflare | 证书免费且可自动续期；不经过 CDN，国内访问香港服务器更直接 |
| 配置和代码分离 | 生产配置挂载进容器，改配置不用重建镜像，密码也不会进入镜像 |

---

## 第 1 部分：上线前的代码修改

### 1.1 数据库初始化 `backend/core/init_db.go`

**原代码的问题**：连接失败时只打印错误、继续运行，随后出现空指针 panic；"DB connected" 无论成败都会打印；最大连接数没有上限。

**修改后的逻辑**（全部在 `InitDB()` 一个函数里）：

```go
const (
	dbMaxOpenConns    = 10              // 最大连接数固定为 10
	dbConnectAttempts = 10              // 最多重试 10 次
	dbRetryInterval   = 3 * time.Second // 每次间隔 3 秒
	dbPingTimeout     = 5 * time.Second
)
```

| 改动 | 意义 |
|---|---|
| DSN 加 `timeout=5s&readTimeout=30s&writeTimeout=30s` | 建立连接最多等 5 秒；单条查询最多阻塞 30 秒 |
| 循环里 `gorm.Open` + `PingContext` | `Open` 只创建连接池，`Ping` 才真正确认数据库可用 |
| 失败时 `sqlDB.Close()` | 避免重试过程中泄漏连接 |
| `SetMaxOpenConns(10)`、`SetMaxIdleConns`、`SetConnMaxLifetime`、`SetConnMaxIdleTime` | 限制连接数、定期更换旧连接，避免被 MySQL 的 `wait_timeout` 断开 |
| 日志里打印 `用户名@主机:端口/库名`，不打印密码 | 防止密码进入日志 |
| 全部失败后 `log.Fatalf` 退出 | 给出清楚的错误，交给容器的重启策略处理 |

**为什么这对容器部署很重要**：Compose 启动时，后端可能比 MySQL 先就绪。重试机制把"启动竞争"变成"短暂等待"，而不是反复崩溃重启。

**验证方式**：

```bash
go vet ./... && go test ./...                         # 静态检查和单元测试
MYZHIHU2_DB_TEST=1 MYZHIHU2_DB_USER=root MYZHIHU2_DB_PASSWORD=root go test ./core/   # 连真实 MySQL 的集成测试
```

### 1.2 关闭注册但保留注册页面

- 路由 `v1.POST("/auth/register", ...)` **保留**（曾尝试注释掉，后来还原）；
- 由配置 `demo.registration_enabled: false` 控制，业务层返回 **403** 和中文提示；
- 前端通过 `GET /api/v1/meta` 提前得知注册已关闭，在注册页显示提示。

| 做法 | 访客看到的结果 |
|---|---|
| 注释掉路由 | 404，前端只显示"请求失败（404）"，像网站坏了 |
| 用配置关闭（最终采用） | 403 + "演示环境已关闭注册，请使用演示账号体验"，体现出这是有意的安全设计 |

---

## 第 2 部分：本地准备

### 2.1 目录结构

源码复制到 `~/my_project/myzhihu2_deploy/`，部署文件全部放在新建的 `deployment/prod/` 下，**只上传这个目录**：

```
deployment/prod/
├── compose.yml                # 容器编排
├── .env                       # 中间件密码（Compose 读取）
├── backend/
│   ├── Dockerfile             # 后端镜像
│   ├── server                 # 编译好的后端程序
│   ├── seed                   # 编译好的种子数据程序
│   └── settings.prod.yaml     # 后端生产配置（挂载进容器）
├── nginx/
│   ├── default.conf           # 当前使用的 Nginx 配置（最终为 HTTPS 版）
│   └── default.https.conf     # HTTPS 版本的备份
├── web/html/                  # 前端打包产物
└── certbot/{www,conf}/        # 证书验证目录、证书目录（服务器上生成内容）
```

**原则**：不修改原有的 `backend/settings/settings.yaml`、`deployment/compose.middleware.yml` 等开发文件，新文件放在独立子目录，避免冲突。

### 2.2 编译后端和种子程序

```bash
cd ~/my_project/myzhihu2_deploy/backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ../deployment/prod/backend/server ./cmd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ../deployment/prod/backend/seed   ./cmd/seed
file ../deployment/prod/backend/server       # 确认：ELF 64-bit, x86-64, statically linked
```

| 参数 | 意义 |
|---|---|
| `CGO_ENABLED=0` | 纯 Go 静态编译，不依赖系统 C 库。Alpine 用 musl 而不是 glibc，动态链接的程序跑不起来 |
| `GOOS=linux GOARCH=amd64` | 交叉编译出 Linux x86_64 程序；ARM 服务器用 `arm64`。服务器上用 `uname -m` 确认 |
| `-trimpath` | 去掉二进制里的本地绝对路径 |
| `-ldflags="-s -w"` | 去掉符号表和调试信息，体积减小约 30% |
| `-o 路径 ./cmd` | 输出文件位置和要编译的包 |

### 2.3 打包前端

```bash
cd ~/my_project/myzhihu2_deploy/frontend
npm ci && npm run build
mkdir -p ../deployment/prod/{nginx,web/html,certbot/www,certbot/conf}
cp -r dist/* ../deployment/prod/web/html/
```

| 命令 | 意义 |
|---|---|
| `npm ci` | 严格按 `package-lock.json` 安装依赖，比 `npm install` 更可复现 |
| `npm run build` | 执行 `vite build`，产物在 `dist/`：`index.html` + `assets/` 下带哈希的 js、css |
| `mkdir -p a/{b,c}` | `-p` 自动创建父目录、已存在不报错；`{b,c}` 是 bash 的花括号展开 |

> 踩坑：第一次 `cp` 报 `target ... is not a directory`，原因是 `mkdir` 没执行，目标目录不存在。

### 2.4 生成密码和密钥

```bash
openssl rand -base64 18 | tr -d '/+='    # 中间件密码，执行 4 次
openssl rand -base64 48                  # JWT 密钥
openssl rand -base64 24                  # 系统用户密码、私钥口令
```

| 用途 | 是否去掉 `/+=` | 原因 |
|---|---|---|
| `.env` 和 YAML 中的服务密码 | 去掉 | 避免在 YAML、连接串、URL 中引发转义问题 |
| JWT 密钥 | 不用去掉，YAML 中加双引号 | 只在 YAML 里使用，代码要求至少 32 个字符 |
| 系统密码、私钥口令 | 不用去掉 | `passwd`、`ssh-keygen` 不解析这些字符 |

**先保存到密码管理器，再使用**。openssl 生成的密码不会留下任何记录。

### 2.5 `.env`（Compose 使用的密码）

```bash
MYSQL_ROOT_PASSWORD=<随机密码1>
DB_USER=myzhihu_app
DB_PASSWORD=<随机密码2>
REDIS_PASSWORD=<随机密码3>
RMQ_USER=myzhihu2
RMQ_PASSWORD=<随机密码4>
```

| 变量 | 意义 |
|---|---|
| `MYSQL_ROOT_PASSWORD` | MySQL 管理员密码，只用于初始化、备份、健康检查 |
| `DB_USER` / `DB_PASSWORD` | 后端专用账号，MySQL 首次启动时自动创建，只能访问 `myzhihu` 库（最小权限原则） |
| `REDIS_PASSWORD` | Redis 访问密码 |
| `RMQ_USER` / `RMQ_PASSWORD` | RabbitMQ 账号，首次启动时自动创建 |

**注意**：
- `.env` 中的值**不加引号**；
- MySQL 和 RabbitMQ 的账号**只在数据卷为空时创建**，之后改 `.env` 不会生效，需要进容器修改或删除数据卷重建；
- 权限设为 600：`chmod 600 .env`。

### 2.6 `backend/Dockerfile`

```dockerfile
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app
ENV TZ=Asia/Shanghai GIN_MODE=release
WORKDIR /app
COPY server seed /app/
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
CMD ["-f", "/app/settings.yaml"]
```

| 指令 | 意义 |
|---|---|
| `FROM alpine:3.20` | 约 8MB 的基础镜像。程序已在本地编译，不需要 Go 工具链 |
| `RUN apk add --no-cache ca-certificates tzdata` | 安装根证书和时区数据。`--no-cache` 不保留包索引，镜像更小。缺少 tzdata 时，DSN 里的 `loc=Local` 会按 UTC 处理，时间差 8 小时 |
| `adduser -D -H -u 10001 app` | 创建普通用户：`-D` 无密码，`-H` 无家目录，`-u` 指定 uid |
| `\` 和 `&&` | 一条 `RUN` 里串联多个命令，只生成一层镜像 |
| `ENV` | 设置容器内环境变量：时区、Gin 运行模式 |
| `WORKDIR /app` | 设置工作目录，不存在会自动创建 |
| `COPY server seed /app/` | 把构建上下文（`backend/` 目录）里的文件复制进镜像；会保留原有的权限位 |
| `USER app` | 之后的指令和容器进程都以 app 用户运行，不是 root |
| `EXPOSE 8080` | 仅作声明，不会对外开放端口 |
| `ENTRYPOINT` + `CMD` | 拼成默认命令 `/app/server -f /app/settings.yaml`。`docker run 镜像 参数` 时只替换 `CMD` 部分，所以同一个镜像可以执行 `-db` 建表 |

### 2.7 `backend/settings.prod.yaml`（后端生产配置）

```yaml
system:
  ip: 0.0.0.0              # 监听所有网卡；写 127.0.0.1 的话 Nginx 容器访问不到
  port: 8080
  mode: "release"

db:
  user: myzhihu_app
  passport: "<随机密码2>"   # 字段名是代码中的拼写 passport，不能写成 password
  host: mysql              # Compose 服务名，不是 127.0.0.1
  port: 3306
  db_name: myzhihu
  debug: false
  source: mysql
  max_idle_conns: 10
  max_open_conns: 10       # 代码已固定为 10，这里仅作说明
  conn_max_lifetime: "30m"

redis:
  enabled: true
  host: "redis"
  port: 6379
  password: "<随机密码3>"
  db: 0                    # 种子程序重置数据时会 FLUSHDB 这个库
  pool_size: 10
  min_idle_conns: 2
  dial_timeout: "5s"
  read_timeout: "3s"
  write_timeout: "3s"

rabbitmq:
  enabled: true
  host: "rabbitmq"
  port: 5672
  username: "myzhihu2"
  password: "<随机密码4>"
  vhost: "/"
  exchange: "myzhihu2.events"
  exchange_type: "direct"
  queue: "myzhihu2.user_behavior"
  routing_key: "user.behavior"
  feed_queue: "myzhihu2.feed"
  feed_routing_key: "feed.content"

jwt:
  algorithm: "HS256"
  issuer: "myzhihu2"
  secret: "<新生成的 JWT 密钥>"   # 绝不能沿用已提交进 git 的旧密钥
  expire_hours: 24

feed:
  bigv_threshold: 100      # 粉丝数 ≥ 100 的作者改为读时拉取
  inbox_limit: 1000
  outbox_limit: 1000
  fanout_batch: 500
  explore_window_days: 7
  explore_pool_size: 500
  explore_refresh_seconds: 60

demo:
  enabled: true                    # 必须写，默认值是 false
  registration_enabled: false      # 必须写，关闭注册
  reader_user_id: 10               # 与种子程序中的账号 ID 一致
  bigv_user_id: 1
  protected_max_content_id: 29999
  posts_per_hour: 10
```

| 与本地开发配置的区别 | 原因 |
|---|---|
| 地址从 `127.0.0.1` 改为服务名 | 容器里的 `127.0.0.1` 是容器自己 |
| 数据库账号改为 `myzhihu_app` | 最小权限 |
| 新的 JWT 密钥 | 旧密钥已在 git 历史里，任何人都能伪造 token |
| 中间件都设置密码 | 多一层保护 |

**一致性要求**：`db.passport`、`redis.password`、`rabbitmq.password` 必须和 `.env` 中对应的值完全相同。部署中曾出现三处不一致，用脚本以 `.env` 为准同步解决（见第 10 部分）。

### 2.8 `compose.yml`（容器编排）

```yaml
name: myzhihu2

x-logging: &logging
  driver: json-file
  options: { max-size: "10m", max-file: "3" }

services:
  mysql:
    image: mysql:8.4
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:?}
      MYSQL_DATABASE: myzhihu
      MYSQL_USER: ${DB_USER:?}
      MYSQL_PASSWORD: ${DB_PASSWORD:?}
      TZ: Asia/Shanghai
    command:
      - --innodb-buffer-pool-size=256M
      - --performance-schema=OFF
      - --max-connections=60
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h127.0.0.1 -uroot -p$${MYSQL_ROOT_PASSWORD} --silent"]
      interval: 10s
      retries: 12
      start_period: 40s
    deploy: { resources: { limits: { memory: 700M } } }
    logging: *logging

  redis:
    image: redis:7.4-alpine
    restart: unless-stopped
    command:
      - redis-server
      - --appendonly
      - "yes"
      - --requirepass
      - ${REDIS_PASSWORD:?}
      - --maxmemory
      - 256mb
      - --maxmemory-policy
      - noeviction
    environment:
      REDIS_PASSWORD: ${REDIS_PASSWORD}
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a \"$$REDIS_PASSWORD\" --no-auth-warning ping | grep -q PONG"]
      interval: 10s
    deploy: { resources: { limits: { memory: 300M } } }
    logging: *logging

  rabbitmq:
    image: rabbitmq:4.3.6-management
    restart: unless-stopped
    environment:
      RABBITMQ_DEFAULT_USER: ${RMQ_USER:?}
      RABBITMQ_DEFAULT_PASS: ${RMQ_PASSWORD:?}
    ports:
      - "127.0.0.1:15672:15672"
    volumes:
      - rabbitmq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 15s
      start_period: 30s
    deploy: { resources: { limits: { memory: 400M } } }
    logging: *logging

  backend:
    build: ./backend
    image: myzhihu2-backend:latest
    restart: unless-stopped
    volumes:
      - ./backend/settings.prod.yaml:/app/settings.yaml:ro
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
      rabbitmq: { condition: service_healthy }
    deploy: { resources: { limits: { memory: 256M } } }
    logging: *logging

  seed:
    image: myzhihu2-backend:latest
    entrypoint: ["/app/seed"]
    command: ["-f", "/app/settings.yaml", "-profile", "demo", "-seed", "20261107", "-reset", "-purge-queues"]
    volumes:
      - ./backend/settings.prod.yaml:/app/settings.yaml:ro
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
      rabbitmq: { condition: service_healthy }
    profiles: ["tools"]
    restart: "no"
    logging: *logging

  nginx:
    image: nginx:1.29-alpine
    restart: unless-stopped
    environment:
      TZ: Asia/Shanghai
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./web/html:/usr/share/nginx/html:ro
      - ./nginx/default.conf:/etc/nginx/conf.d/default.conf:ro
      - ./certbot/www:/var/www/certbot:ro
      - ./certbot/conf:/etc/letsencrypt:ro
    depends_on:
      - backend
    logging: *logging

volumes:
  mysql_data: {}
  redis_data: {}
  rabbitmq_data: {}
```

| 配置项 | 意义 |
|---|---|
| `name: myzhihu2` | 项目名，作为容器名、网络名、卷名的前缀 |
| `x-logging: &logging` / `*logging` | `x-` 开头的字段 Compose 会忽略，用来存放公共配置；`&` 定义锚点，`*` 引用。日志最多 3 个 10MB 文件，防止写满磁盘 |
| `image` | 使用的镜像；本地没有时自动从 Docker Hub 拉取 |
| `build: ./backend` | 用该目录下的 Dockerfile 构建镜像；`image:` 给构建结果命名，供 seed 复用 |
| `restart: unless-stopped` | 崩溃或服务器重启后自动拉起，只有手动 stop 后不再启动 |
| `${VAR:?}` | 从 `.env` 读取；未设置时报错退出，不会用空密码启动 |
| `$$` | 转义为字面量 `$`，交给容器内 shell 展开，而不是被 Compose 提前替换 |
| `MYSQL_DATABASE` / `MYSQL_USER` | 官方镜像约定的环境变量，首次启动自动建库建用户 |
| mysql 的 `command` | 缓冲池 256M、关闭性能监控表（省约 300MB 内存）、最大连接 60 |
| `volumes: - 卷名:/路径` | 命名卷，数据由 Docker 管理，删除容器不丢数据 |
| `volumes: - ./相对路径:/容器路径:ro` | 绑定挂载宿主机文件；`:ro` 只读 |
| `healthcheck` | 判断服务是否真正可用。`start_period` 内的失败不计数 |
| `depends_on: condition: service_healthy` | 依赖通过健康检查后才启动 |
| `deploy.resources.limits.memory` | 容器内存上限 |
| redis `noeviction` | 内存满时拒绝写入而不是悄悄删 key，避免 Feed 就绪标记和收件箱数据不一致 |
| `"127.0.0.1:15672:15672"` | 只绑定本机回环地址，公网无法访问，需要 SSH 隧道 |
| backend 没有 `ports` | 后端不对外，只能通过内部网络被 Nginx 访问 |
| `entrypoint` | 覆盖镜像的 `ENTRYPOINT`，seed 服务借此运行 `/app/seed` |
| `profiles: ["tools"]` | 属于 tools 分组，`up` 时不会启动，只能 `run` 手动执行 |
| `ports: "80:80"` | `宿主机端口:容器端口`，整个系统只开放 80 和 443 |

### 2.9 Nginx 配置

**阶段 1（只有 HTTP，用于申请证书）**：

```nginx
limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=auth:10m rate=10r/m;

server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name example.com www.example.com _;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    root /usr/share/nginx/html;
    index index.html;

    gzip on;
    gzip_types text/css application/javascript application/json image/svg+xml;

    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options SAMEORIGIN always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;

    location /assets/ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        add_header X-Content-Type-Options nosniff always;
    }

    location /api/v1/auth/ {
        limit_req zone=auth burst=5 nodelay;
        limit_req_status 429;
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
    }

    location /api/ {
        limit_req zone=api burst=20 nodelay;
        limit_req_status 429;
        client_max_body_size 256k;
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

**阶段 2（HTTPS，最终上线版本）**：内容见 `nginx/default.https.conf`。在阶段 1 基础上：

- 80 端口只保留 `/.well-known/acme-challenge/`，其余 `return 301 https://example.com$request_uri;`
- 新增 `www.example.com` 的 443 server，跳转到主域名；
- 主站 443 server 增加 `ssl_certificate`、`ssl_certificate_key`、`ssl_protocols TLSv1.2 TLSv1.3`、`ssl_session_cache`、`ssl_session_timeout`、`http2 on`、`Strict-Transport-Security`。

| 指令 | 意义 |
|---|---|
| `limit_req_zone $binary_remote_addr zone=名:10m rate=10r/s` | 定义限流区域：按客户端 IP 计数，10MB 内存记录状态，每秒 10 次。必须写在 `http{}` 层级，`conf.d/*.conf` 本身就被包含在 `http{}` 里 |
| `limit_req zone=名 burst=20 nodelay` | 应用限流；允许 20 次突发，超出部分立即拒绝 |
| `limit_req_status 429` | 被限流时返回 429（默认 503 容易被误认为宕机） |
| `listen 80 default_server` | 监听端口并作为默认站点；`[::]:80` 监听 IPv6 |
| `server_name a b _` | 匹配的主机名；`_` 匹配任意主机名，IP 访问也能命中 |
| `location /路径/ { root 目录; }` | 前缀匹配；文件路径 = `root` + 完整 URI |
| `root` / `index` | 静态文件根目录和默认首页 |
| `gzip` / `gzip_types` | 开启压缩及压缩的类型 |
| `add_header 名 值 always` | 添加响应头；`always` 表示错误响应也添加。**location 内写了 add_header，外层的就不再继承** |
| `expires 30d` | 设置缓存过期时间 |
| `proxy_pass http://backend:8080` | 反向代理；地址后面**不带 URI** 时，请求路径原样转发 |
| `proxy_set_header` | 把真实 IP、域名、协议传给后端 |
| `proxy_connect_timeout` / `proxy_read_timeout` | 连接后端、读取响应的超时时间 |
| `client_max_body_size` | 请求体上限，超出返回 413 |
| `try_files $uri $uri/ /index.html` | 依次尝试文件、目录，都不存在就返回 index.html，用于前端 history 路由 |
| `return 301 URL` | 永久重定向；`$request_uri` 保留原路径和参数 |
| `ssl_certificate` / `ssl_certificate_key` | 证书链（fullchain）和私钥 |
| `http2 on` | 启用 HTTP/2（新版 Nginx 写法） |
| `Strict-Transport-Security` | HSTS，一年内浏览器只用 HTTPS 访问。**只在 HTTPS 生效后才能加** |

---

## 第 3 部分：SSH 密钥与服务器登录

### 3.1 生成密钥（本地）

```bash
ssh-keygen -t ed25519 -C "myzhihu2-deploy"
ls -al ~/.ssh
ssh-keygen -lf ~/.ssh/id_ed25519.pub        # 查看公钥指纹
```

| 文件 | 权限 | 意义 |
|---|---|---|
| `~/.ssh/` | 700 | 只有自己能访问 |
| `id_ed25519` | 600 | **私钥**，永远只留在本机 |
| `id_ed25519.pub` | 644 | 公钥，放到服务器 `authorized_keys` 里 |
| `known_hosts` | 600 | 记录连接过的服务器指纹，防止中间人冒充 |

### 3.2 把公钥放到服务器并用密钥登录

```bash
ssh-copy-id -i ~/.ssh/id_ed25519.pub root@<服务器IP>      # 需要输入一次密码
ssh -i ~/.ssh/id_ed25519 root@<服务器IP>
ssh -v root@<服务器IP> 2>&1 | grep -Ei 'offering|accepts|authenticated'   # 确认 using "publickey"
ssh -o PasswordAuthentication=no -o PreferredAuthentications=publickey root@<服务器IP> "echo key-ok"
```

### 3.3 `~/.ssh/config` 别名

```
Host myzhihu2
    HostName <服务器IP>
    User root
    IdentityFile ~/.ssh/id_ed25519
    IdentitiesOnly yes
    ServerAliveInterval 60
```

之后 `ssh myzhihu2`、`rsync ... myzhihu2:~/myzhihu2/`、`scp 文件 myzhihu2:路径` 都可以用别名。

### 3.4 本次的实际情况：一直使用 root

服务器上没有创建普通用户（`usermod: user 'deploy' does not exist`），所以全程以 root 部署。对应的加固方式是**允许 root 登录，但只能用密钥**：

```bash
cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin prohibit-password/; s/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
grep -rnEi 'PasswordAuthentication|PermitRootLogin' /etc/ssh/sshd_config.d/    # 子目录配置会覆盖主配置
sshd -t && systemctl restart ssh
sshd -T | grep -Ei '^(permitrootlogin|passwordauthentication|pubkeyauthentication)'
```

| 要点 | 说明 |
|---|---|
| 改之前先确认密钥登录可用，并保持一个已登录窗口不关 | 防止把自己锁在外面 |
| `sshd -t` | 检查配置语法，通过后才重启 |
| `sshd -T` | 输出**实际生效**的配置，以它为准 |
| `/etc/ssh/sshd_config.d/*.conf` | 云镜像常在 `50-cloud-init.conf` 里写 `PasswordAuthentication yes`，会覆盖主配置 |
| 重启 sshd 不会断开已有会话 | 出问题时在原窗口用 `.bak` 恢复 |
| 最后的兜底 | 云控制台的 VNC 网页终端 |

> 更安全的做法是创建普通用户（`adduser deploy`、`usermod -aG sudo,docker deploy`、复制 authorized_keys）后设置 `PermitRootLogin no`。本次未采用，可作为后续改进。

---

## 第 4 部分：域名与 DNS

### 4.1 发现实际域名

截图中 NameSilo 控制台的网址显示域名是 **`example.com`**（不是 `example.com`）。直接查询 `.top` 注册局确认：`example.com` 为 NXDOMAIN（不存在）。之后所有配置中的域名都改为 `example.com`。

### 4.2 修改 DNS 记录

| 操作 | 原因 |
|---|---|
| 删除 6 条默认 A 记录（`@`、`www` 各 3 个 IP，Service 为 3rd-party） | 注册商默认的停放页。不删的话 DNS 会轮流返回停放页和你的服务器，证书验证也可能失败 |
| 保留 `_domainconnect` TXT 记录 | Domain Connect 协议使用，不影响网站 |
| 新建 `@` → A → `<服务器IP>`、`www` → A → `<服务器IP>` | 主域名和 www 指向服务器 |
| NameServer 保持 `NS1/2/3.DNSOWL.COM` | DNS 由注册商托管，在其 DNS 页面修改的记录才生效 |
| 开启 WHOIS 隐私保护 | 否则注册人姓名、地址公开可查 |

| DNS 表格字段 | 含义 |
|---|---|
| Name | `@` 表示域名本身，`www` 表示子域名 |
| Type | A（IPv4）、AAAA（IPv6）、CNAME（别名）、MX（邮件）、TXT（文本验证） |
| Distance/Prio | 只对 MX 有意义 |
| TTL | 缓存秒数，修改后最长要等这么久才完全生效 |

### 4.3 验证

```bash
dig +short example.com @ns1.dnsowl.com      # 直接问权威 DNS
dig +short www.example.com @ns1.dnsowl.com
dig +short example.com @8.8.8.8             # 在服务器上问公共 DNS
```

> 踩坑：本机开着代理（fake-ip 模式），普通 `dig` 返回 `198.18.x.x` 假地址。验证 DNS 要指定 DNS 服务器，或者在服务器上执行。

---

## 第 5 部分：服务器初始化

建议先执行 `tmux`，断线后 `tmux attach` 回到原会话。

### 5.1 时区

```bash
timedatectl set-timezone Asia/Shanghai
timedatectl set-ntp true
timedatectl | grep -E "Time zone|synchronized"
```

| 层次 | 时区设置方式 |
|---|---|
| 宿主机 | `timedatectl`，影响 `date`、日志、**crontab 执行时间** |
| backend 容器 | Dockerfile 中 `tzdata` + `ENV TZ` |
| mysql、nginx 容器 | compose 中 `environment: TZ: Asia/Shanghai` |

容器**不继承**宿主机时区，需要分别设置。必须在配置 crontab 之前设置宿主机时区。

### 5.2 swap

```bash
free -h
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab      # 开机自动挂载
```

### 5.3 防火墙

```bash
ufw allow 22/tcp && ufw allow 80/tcp && ufw allow 443/tcp
ufw --force enable && ufw status
```

云厂商控制台的**安全组**也要放行同样的端口，两层都要配置。

### 5.4 解决 apt 锁占用

安装 Docker 时报错 `Could not get lock /var/lib/dpkg/lock-frontend. It is held by process 1407 (apt)`。

**排查**：

```bash
ps -o pid,etime,cmd -p 1407
ps aux | grep -E '[a]pt|[d]pkg|[u]nattended'
```

发现 `apt upgrade -y` 已运行 1 天 20 小时，`dpkg --configure --pending` 卡在 `pts/1` 上，等待 needrestart / 配置文件冲突的交互提问，而那个 SSH 会话早已断开。

**解决**：

```bash
who                                                            # 确认 pts/1 已不在线
kill 4008 4007 4006 4005 1407                                  # 先用普通 kill
DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a dpkg --configure -a --force-confold
DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a apt-get -y \
  -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold upgrade
sshd -T | grep -Ei '^(passwordauthentication|permitrootlogin)'    # 确认 SSH 配置没被覆盖
[ -f /var/run/reboot-required ] && echo "需要重启"
```

| 参数 | 意义 |
|---|---|
| `DEBIAN_FRONTEND=noninteractive` | 不弹出交互界面 |
| `NEEDRESTART_MODE=a` | 自动重启需要重启的服务 |
| `--force-confold` | 配置文件冲突时保留当前版本（保护已修改的 sshd_config） |
| `--force-confdef` | 无冲突时用默认值 |

**不要**直接删除锁文件或 `kill -9` 正在写数据库的 apt 进程。

### 5.5 安装 Docker

```bash
curl -fsSL https://get.docker.com | sh
docker --version && docker compose version && docker run --rm hello-world
```

| 选项 | 说明 |
|---|---|
| 官方脚本 `get.docker.com`（采用） | 包含 `docker compose` 插件 |
| `snap install docker` | 不推荐：目录访问受限，挂载可能出问题 |
| `apt install docker.io` | 不推荐：不带 compose 插件 |

root 本身就能使用 Docker，不需要加入 docker 组。

---

## 第 6 部分：上传与启动服务

### 6.1 上传

```bash
rsync -avz --chmod=F600,D700 --exclude certbot \
  ~/my_project/myzhihu2_deploy/deployment/prod/ myzhihu2:~/myzhihu2/
rsync -avzn ...            # 加 -n 先演练，只列出会传的文件
```

| 参数 | 意义 |
|---|---|
| `-a` | 归档模式：递归、保留权限和时间等 |
| `-v` | 显示过程 |
| `-z` | 传输时压缩 |
| `-n` | 演练，不真正传输 |
| `--chmod=F600,D700` | 设置目标端权限：文件 600、目录 700 |
| `--exclude certbot` | 不上传本地空的证书目录，避免覆盖服务器上的证书 |
| 源路径末尾的 `/` | **上传目录里的内容**；不加 `/` 会多一层 `prod/` 目录 |
| `--relative` | 只传单个文件时保留相对路径，如 `nginx/default.conf` |

### 6.2 设置权限

```bash
cd ~/myzhihu2
chmod 755 backend/server backend/seed                  # 程序需要执行权限
chmod -R a+rX web nginx                                # Nginx 容器需要读取
mkdir -p certbot/www certbot/conf && chmod 755 certbot certbot/www certbot/conf
chown root:10001 backend/settings.prod.yaml && chmod 640 backend/settings.prod.yaml
chmod 600 .env
```

| 命令 | 原因 |
|---|---|
| `chmod 755` 程序 | `COPY` 会保留权限位，600 的程序在容器里无法执行 |
| `chmod -R a+rX` | 所有人可读；`X`（大写）只给目录和已有执行权限的文件加执行位 |
| `chown root:10001` + `640` | 容器内 app 用户 uid 是 10001：root 读写、组 10001 只读、其他人无权限 |
| `.env` 600 | 只有 docker compose（以 root 运行）读取 |

### 6.3 启动顺序

```bash
docker compose up -d mysql redis rabbitmq             # 此时自动拉取镜像
watch -n 5 'docker compose ps'                         # 等三个都 (healthy)，Ctrl+C 退出
docker compose build backend                           # 构建后端镜像
docker compose run --rm backend -f /app/settings.yaml -db    # 建表
docker compose run --rm seed                           # 写入演示数据
docker compose up -d backend nginx
docker compose ps
docker compose logs --tail=30 backend
curl -s localhost/api/v1/meta                          # 返回 JSON，registration_enabled 为 false
curl -s -o /dev/null -w "%{http_code}\n" localhost/    # 200
```

| 命令 | 意义 |
|---|---|
| `up -d 服务名` | 后台启动指定服务（及其依赖） |
| `build 服务名` | 按 `build:` 构建镜像 |
| `run --rm 服务 参数` | 临时运行一次，参数替换 `CMD`，结束后删除容器 |
| `ps` | 查看状态和健康情况 |
| `logs --tail=N 服务` | 查看最近 N 行日志；`-f` 持续跟踪 |

> 踩坑：nginx 启动报 `invalid volume specification: '/root/myzhihu2/certbot/conf:/:rw' ... destination can't be '/'`。原因是 compose 中挂载写错（`./certbot/conf:/`、`./nginx/default.t.conf:ro`），且缩进多了 2 格。改正为 `./nginx/default.conf:/etc/nginx/conf.d/default.conf:ro` 和 `./certbot/conf:/etc/letsencrypt:ro` 后解决。用 `docker compose config` 可以提前发现这类问题。

---

## 第 7 部分：申请 HTTPS 证书并切换配置

### 7.1 webroot 验证原理

```
certbot 在 certbot/www/.well-known/acme-challenge/ 写入随机文件
  → Let's Encrypt 访问 http://域名/.well-known/acme-challenge/<文件>
  → Nginx（阶段 1）从 /var/www/certbot 返回该文件
  → 内容一致即证明你控制该域名 → 签发证书到 certbot/conf/live/域名/
```

### 7.2 测试验证路径

```bash
mkdir -p certbot/www/.well-known/acme-challenge
echo ok-123 > certbot/www/.well-known/acme-challenge/test.txt
curl http://example.com/.well-known/acme-challenge/test.txt        # 返回 ok-123
curl http://www.example.com/.well-known/acme-challenge/test.txt
rm certbot/www/.well-known/acme-challenge/test.txt
```

### 7.3 演练与正式申请

```bash
# 演练（测试环境，不计入失败次数）
docker run --rm -v "$PWD/certbot/www:/var/www/certbot" -v "$PWD/certbot/conf:/etc/letsencrypt" \
  certbot/certbot certonly --webroot -w /var/www/certbot \
  -d example.com -d www.example.com --email <你的邮箱> --agree-tos --no-eff-email --dry-run

# 正式申请（最终成功的写法）
mkdir -p certbot/log
docker run -it --rm \
  -v "$PWD/certbot/www:/var/www/certbot" \
  -v "$PWD/certbot/conf:/etc/letsencrypt" \
  -v "$PWD/certbot/log:/var/log/letsencrypt" \
  certbot/certbot certonly --webroot -w /var/www/certbot \
  -d example.com -d www.example.com \
  --email <你的邮箱> --agree-tos --no-eff-email \
  --non-interactive -v

ls certbot/conf/live/example.com/        # fullchain.pem、privkey.pem 等
```

| 参数 | 意义 |
|---|---|
| `docker run -it` | 分配终端，能看到实时进度 |
| `--rm` | 结束后删除容器 |
| `-v 宿主机:容器` | 共享验证目录、证书目录、日志目录 |
| `certonly --webroot -w 目录` | 只申请证书，用 webroot 方式验证，不修改 Nginx 配置 |
| `-d` | 证书包含的域名，可以多个 |
| `--email` | 到期和续期失败的提醒邮箱，**必须是真实地址** |
| `--agree-tos --no-eff-email` | 同意条款、不订阅宣传 |
| `--non-interactive` | 不提问，遇到问题直接报错 |
| `-v`（certbot 的） | 详细输出 |
| `--dry-run` | 测试环境演练。正式环境有失败次数限制（每小时 5 次） |

> 踩坑：第一次正式申请时没有任何输出，以为卡住了就按了 Ctrl+C（`^CExiting due to user request`），所以没有拿到证书。实际上 certbot 是在等待 Let's Encrypt 回访，需要几十秒。加 `-it` 和 `-v` 后能看到进度，耐心等待即可成功。

### 7.4 切换到阶段 2

```bash
# 本地上传完整的 HTTPS 配置（不要在服务器上手动粘贴长配置）
scp nginx/default.https.conf myzhihu2:~/myzhihu2/nginx/default.conf

# 服务器
cd ~/myzhihu2
chmod a+r nginx/default.conf
docker compose exec nginx nginx -t && docker compose exec nginx nginx -s reload
```

| 命令 | 意义 |
|---|---|
| `docker compose exec 服务 命令` | 在运行中的容器里执行命令 |
| `nginx -t` | 检查配置语法和证书文件是否存在 |
| `nginx -s reload` | 平滑重载配置，不中断现有连接 |
| `A && B` | A 成功才执行 B：语法检查失败时不会 reload，旧配置继续工作 |

> 踩坑：在服务器上用 nano 粘贴阶段 2 配置时，内容在复制过程中被截断（如 `listen 80 default_se`），`nginx -t` 报 `invalid parameter "default_se"`。因为有 `&&`，reload 没有执行，线上没受影响。改为从本地 `scp` 完整文件后解决。

### 7.5 验证

```bash
curl -I http://example.com                 # 301 → https://example.com/
curl -I https://www.example.com            # 301 → https://example.com/
curl -I https://example.com                # HTTP/2 200，含 strict-transport-security
curl -s https://example.com/api/v1/meta
curl -s -X POST https://example.com/api/v1/auth/register -H 'Content-Type: application/json' \
     -d '{"username":"probe123","password":"Passw0rd!"}' -w "\n%{http_code}\n"    # 403
```

| curl 参数 | 意义 |
|---|---|
| `-I` | 只取响应头 |
| `-s` | 静默，不显示进度条 |
| `-o /dev/null` | 丢弃响应体 |
| `-w "%{http_code}"` | 输出指定信息，如状态码 |
| `-X POST -H ... -d ...` | 指定方法、请求头、请求体 |
| `-L` | 跟随重定向 |

---

## 第 8 部分：定时任务（`crontab -e`）

```cron
# 每周一 03:00 续期证书（到期前 30 天内才真正续期），然后重载 Nginx
0 3 * * 1 cd /root/myzhihu2 && docker run --rm -v "$PWD/certbot/www:/var/www/certbot" -v "$PWD/certbot/conf:/etc/letsencrypt" certbot/certbot renew --quiet && docker compose exec -T nginx nginx -s reload
# 每天 04:00 备份数据库，保留 7 天
0 4 * * * cd /root/myzhihu2 && mkdir -p backups && docker compose exec -T mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction myzhihu' | gzip > backups/myzhihu_$(date +\%F).sql.gz && find backups -name '*.sql.gz' -mtime +7 -delete
# 每天 04:30 重置演示数据，然后重启后端
30 4 * * * cd /root/myzhihu2 && docker compose run --rm seed >> backups/reset.log 2>&1 && docker compose restart backend
```

续期演练：

```bash
cd /root/myzhihu2 && docker run --rm -v "$PWD/certbot/www:/var/www/certbot" -v "$PWD/certbot/conf:/etc/letsencrypt" certbot/certbot renew --dry-run
```

| 语法 | 意义 |
|---|---|
| `分 时 日 月 周 命令` | cron 时间格式；`*` 表示任意，周的 1 表示周一 |
| 绝对路径 `/root/myzhihu2` | cron 的环境和登录时不同，绝对路径最可靠 |
| `exec -T` | cron 没有终端，`-T` 不分配伪终端 |
| `\%` | crontab 中 `%` 是特殊字符，需要转义 |
| `>> 文件 2>&1` | 标准输出追加到文件，标准错误也一并重定向 |
| `--single-transaction` | 在一致性快照中导出，不锁表 |
| `find ... -mtime +7 -delete` | 删除 7 天前的文件 |

---

## 第 9 部分：日常运维与更新

```bash
# 本地：重新编译 / 打包后上传
rsync -avz --chmod=F600,D700 --exclude certbot \
  ~/my_project/myzhihu2_deploy/deployment/prod/ myzhihu2:~/myzhihu2/

# 服务器：rsync 会重置权限，补回后重建后端
cd ~/myzhihu2 && chmod 755 backend/server backend/seed && chmod -R a+rX web nginx \
  && chown root:10001 backend/settings.prod.yaml && chmod 640 backend/settings.prod.yaml \
  && docker compose up -d --build backend
```

| 场景 | 操作 |
|---|---|
| 只改了前端 | 上传后不用重启，静态文件是直接挂载的 |
| 改了后端 | `docker compose up -d --build backend` |
| 改了 Nginx 配置 | `nginx -t && nginx -s reload` |
| 改了 settings.prod.yaml | `docker compose restart backend` |
| 查看资源 | `docker stats --no-stream`、`df -h`、`free -h` |
| 查看日志 | `docker compose logs -f --tail=100 backend` |
| RabbitMQ 管理页面 | 本地 `ssh -L 15672:127.0.0.1:15672 myzhihu2`，浏览器打开 `http://localhost:15672` |

**重要**：本地 `nginx/default.conf` 已换成 HTTPS 版本，避免整体上传时用 HTTP 版本覆盖服务器配置。

---

## 第 10 部分：踩坑记录与排查方法

| # | 现象 | 原因 | 解决 | 学到的方法 |
|---|---|---|---|---|
| 1 | `cp: target ... is not a directory` | 目标目录未创建 | `mkdir -p` 后重新复制 | 复制前先确认目录存在 |
| 2 | 两个文件的密码不一致 | `.env` 和 YAML 分别生成了不同密码 | 以 `.env` 为准，用脚本同步 | 用脚本比较"是否一致"，不显示密码内容 |
| 3 | 本机 `dig` 返回 198.18.x.x | 代理软件的 fake-ip | `dig @ns1.dnsowl.com` 或在服务器上查 | 网络问题要排除本机代理干扰 |
| 4 | 域名对不上 | 实际注册的是 example.com | 所有配置改为正确域名 | 以注册商控制台和注册局查询结果为准 |
| 5 | apt 锁被占用 | 旧的 `apt upgrade` 卡在交互提示上近 2 天 | 结束卡住的进程，非交互方式补完配置 | 长命令用 `tmux` + 非交互参数 |
| 6 | `user 'deploy' does not exist` | 从未创建普通用户 | 改为以 root 部署，SSH 设为 `prohibit-password` | 执行前先确认前提条件 |
| 7 | nginx 挂载错误 `destination can't be '/'` | compose 挂载路径写错、缩进错误 | 改正挂载路径和缩进 | 启动前先 `docker compose config` |
| 8 | 证书目录不存在 | 正式申请时提前按了 Ctrl+C | 加 `-it -v --non-interactive` 并耐心等待 | 无输出不等于卡死，看日志判断 |
| 9 | `invalid parameter "default_se"` | 终端复制长配置时被截断 | 本地生成完整文件后 `scp` | 长配置不要手动粘贴；`nginx -t && reload` 保护线上 |

**通用排查顺序**：

1. 看完整报错，定位到具体文件和行号；
2. `docker compose ps` 看哪个服务异常；
3. `docker compose logs --tail=50 服务名` 看日志；
4. 配置类问题先跑检查命令：`docker compose config`、`nginx -t`、`sshd -t`；
5. 权限类问题用 `ls -la` 看属主和权限，结合容器内运行的 uid 分析；
6. 网络类问题从内到外：容器内 → 服务器本机 `curl localhost` → 外部 `curl 域名`。

---

## 第 11 部分：语法与工具速查

### 11.1 Linux 权限

```
-rw-r-----  root 10001  settings.prod.yaml
 │└┬┘└┬┘└┬┘
 │ │  │  └─ 其他人：无权限
 │ │  └──── 用户组：r（读）
 │ └─────── 所有者：rw（读写）
 └───────── 文件类型：- 文件，d 目录，l 链接
```

| 数字 | 含义 | 常见用法 |
|---|---|---|
| 600 | 所有者读写 | 密码文件、私钥 |
| 640 | 所有者读写、组只读 | 需要被容器用户组读取的配置 |
| 644 | 所有者读写、其他只读 | 公钥、普通配置 |
| 700 | 所有者全部权限 | `.ssh` 目录 |
| 755 | 所有者全部、其他读和执行 | 程序、公共目录 |

`chown 用户:组 文件` 修改属主；`chmod -R` 递归修改。

### 11.2 Shell 常用语法

| 语法 | 意义 |
|---|---|
| `A && B` | A 成功才执行 B |
| `A \|\| B` | A 失败才执行 B |
| `A \| B` | A 的输出作为 B 的输入 |
| `> 文件` / `>> 文件` | 覆盖 / 追加写入 |
| `2>&1` | 标准错误合并到标准输出 |
| `$(命令)` | 命令替换 |
| `"$PWD"` | 当前目录，加引号防止空格问题 |
| `cmd <<'EOF' ... EOF` | here-document，多行输入；`'EOF'` 加引号时不展开变量 |
| `{a,b}` | 花括号展开 |
| `\`（行尾） | 续行 |

### 11.3 YAML

| 规则 | 示例 |
|---|---|
| 用空格缩进，同级对齐，**不能用 Tab** | 服务名 2 格，属性 4 格 |
| `键: 值`，冒号后有空格 | `port: 8080` |
| 列表用 `- ` | `- "80:80"` |
| 行内写法 | `{ max-size: "10m" }`、`["CMD", "ping"]` |
| 字符串可加引号；含 `#`、`:`、空格等特殊字符时必须加 | `password: "Ab3d..."` |
| `yes/no/on/off/true/null` 不加引号会被当作布尔或空值 | 写成 `"yes"` |
| `#` 后是注释 | |
| `&锚点` / `*引用` | 复用配置 |

### 11.4 Docker / Compose 常用命令

| 命令 | 用途 |
|---|---|
| `docker compose config [--quiet]` | 检查并展开配置 |
| `docker compose up -d [服务]` | 后台启动 |
| `docker compose up -d --build 服务` | 重新构建并启动 |
| `docker compose ps` | 状态 |
| `docker compose logs -f --tail=N 服务` | 日志 |
| `docker compose exec [-T] 服务 命令` | 在运行中的容器执行命令 |
| `docker compose run --rm 服务 参数` | 临时运行一次 |
| `docker compose restart 服务` | 重启 |
| `docker compose down` | 停止并删除容器（**保留数据卷**） |
| `docker compose down -v` | 同时删除数据卷（**会清空数据库**） |
| `docker images` / `docker rmi 镜像` | 查看 / 删除镜像 |
| `docker stats --no-stream` | 资源占用 |

### 11.5 Nginx

| 命令 | 用途 |
|---|---|
| `nginx -t` | 检查配置 |
| `nginx -s reload` | 平滑重载 |
| `server {}` | 一个虚拟主机 |
| `location 前缀 {}` | 路径匹配 |
| 分号 `;` 结尾，花括号成对 | 语法错误时 `nginx -t` 会指出行号 |

### 11.6 SSH / 文件传输

| 命令 | 用途 |
|---|---|
| `ssh-keygen -t ed25519` | 生成密钥 |
| `ssh-copy-id -i 公钥 用户@主机` | 上传公钥 |
| `ssh 别名` | 用 config 中的配置登录 |
| `ssh -L 本地端口:127.0.0.1:远程端口 主机` | 端口转发隧道 |
| `scp 本地文件 主机:远程路径` | 复制单个文件 |
| `rsync -avz 源/ 主机:目标/` | 增量同步目录 |
| `sshd -t` / `sshd -T` | 检查配置 / 查看生效配置 |

### 11.7 系统管理

| 命令 | 用途 |
|---|---|
| `timedatectl set-timezone` | 设置时区 |
| `free -h` / `df -h` | 内存 / 磁盘 |
| `ufw allow 端口/tcp` / `ufw status` | 防火墙 |
| `ps aux \| grep 关键字` | 查找进程 |
| `kill PID` / `kill -9 PID` | 结束进程（优先不加 -9） |
| `crontab -e` / `crontab -l` | 编辑 / 查看定时任务 |
| `tmux` / `tmux attach` | 可恢复的终端会话 |
| `uname -m` | CPU 架构 |
| `dig +short 域名 @DNS服务器` | 查询 DNS |

---

## 后续可改进项

1. 创建普通用户并禁止 root 远程登录；
2. 代码补充 `/healthz` 健康检查、优雅退出、`SetTrustedProxies`；
3. 接入可用性监控（UptimeRobot 等），挂了能收到通知；
4. 定期下载数据库备份到本地，并做一次恢复演练；
5. 用 GitHub Actions 自动构建和部署。
