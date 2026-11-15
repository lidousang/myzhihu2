下面是部署 myzhihu2 需要的全部配置文件，只在聊天里展示，不创建、不修改任何文件。

我做了两个假设：服务器是 x86_64 架构，系统是 Ubuntu。如果你执行 uname -m 得到的是 aarch64，只需要把编译命令里的 amd64 改成 arm64。

部署目录结构

在本地建一个独立目录，不要放进项目的 git 仓库，因为里面会有密码：

~/myzhihu2-deploy/
├── compose.yml                 # ① 编排所有容器
├── .env                        # ② 数据库、Redis、RabbitMQ 的密码，供 compose.yml 读取
├── backend/
│   ├── Dockerfile              # ③ 后端镜像
│   ├── server                  # 本地编译好的后端程序
│   ├── seed                    # 本地编译好的种子数据程序
│   └── settings.prod.yaml      # ④ 后端的生产环境配置
├── nginx/
│   └── default.conf            # ⑤ Nginx 配置（分两个阶段）
├── web/html/                   # 前端 npm run build 生成的 dist/ 里的全部内容
└── certbot/{www,conf}/         # 申请 HTTPS 证书时使用，空目录即可

---

② .env：Compose 使用的密码

# MySQL 超级管理员 root 的密码。只在初始化数据库、备份、健康检查时使用，后端不用 root 连接
MYSQL_ROOT_PASSWORD=替换为随机密码1
# 后端连接数据库用的普通账号。MySQL 容器首次启动时会自动创建它，并授予 myzhihu 库的全部权限
DB_USER=myzhihu_app
DB_PASSWORD=替换为随机密码2
# Redis 访问密码。即使 Redis 不对外开放，也加一层保护
REDIS_PASSWORD=替换为随机密码3
# RabbitMQ 账号。容器首次启动时自动创建；同时也是管理页面的登录账号
RMQ_USER=myzhihu2
RMQ_PASSWORD=替换为随机密码4

┌───────────┬─────────────────────────────────────────────────────────────┐
│   要点    │                            说明                             │
├───────────┼─────────────────────────────────────────────────────────────┤
│ 怎么生成  │ 执行 openssl rand -base64 18 | tr -d                        │
│ 密码      │ '/+='，每条执行一次。去掉 /+= 这几个字符，是为了避免它们在  │
│           │ YAML 或连接串里引发转义问题                                 │
├───────────┼─────────────────────────────────────────────────────────────┤
│ 为什么后  │ 最小权限原则：后端只能操作 myzhihu                          │
│ 端不用    │ 这一个库，即使被攻破也无法影响 MySQL 的系统表               │
│ root      │                                                             │
├───────────┼─────────────────────────────────────────────────────────────┤
│ 文件权限  │ 上传前执行 chmod 600 .env，只有文件所有者能读               │
├───────────┼─────────────────────────────────────────────────────────────┤
│ 一个需要  │ MySQL 和 RabbitMQ                                           │
│ 注意的地  │ 的账号只在第一次启动、数据卷还是空的时候创建。之后再修改这  │
│ 方        │ 里的密码不会生效，需要进入容器手动修改，或者删除数据卷重建  │
└───────────┴─────────────────────────────────────────────────────────────┘

---

③ backend/Dockerfile：后端镜像

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

┌────────────────┬────────────────────────────────────────────────────────┐
│       行       │                          意义                          │
├────────────────┼────────────────────────────────────────────────────────┤
│ FROM           │ 基础镜像只有约 8MB。程序在本地已经编译好，镜像里不需要 │
│ alpine:3.20    │  Go 工具链                                             │
├────────────────┼────────────────────────────────────────────────────────┤
│ apk add ca-cer │ 根证书。程序如果访问 HTTPS                             │
│ tificates      │ 服务会用到，这里是预防性安装                           │
├────────────────┼────────────────────────────────────────────────────────┤
│ apk add tzdata │ 必须有。数据库连接串里写了                             │
│  + ENV TZ=Asia │ loc=Local，时间按容器时区解析。缺少时区数据的话会按    │
│ /Shanghai      │ UTC 处理，文章的发布时间会差 8 小时                    │
├────────────────┼────────────────────────────────────────────────────────┤
│ adduser -D -H  │ 创建一个普通用户：没有密码（-D），没有家目录（-H）     │
│ -u 10001 app   │                                                        │
├────────────────┼────────────────────────────────────────────────────────┤
│ ENV GIN_MODE=r │ 关闭 Gin 的调试输出（路由表、彩色日志），性能也更好    │
│ elease         │                                                        │
├────────────────┼────────────────────────────────────────────────────────┤
│ COPY server    │ 后端程序和种子程序放进同一个镜像，种子程序可以直接复用 │
│ seed /app/     │ 这个镜像                                               │
├────────────────┼────────────────────────────────────────────────────────┤
│                │ 以非 root                                              │
│ USER app       │ 用户运行。即使程序被攻破，攻击者在容器里也没有 root    │
│                │ 权限                                                   │
├────────────────┼────────────────────────────────────────────────────────┤
│ EXPOSE 8080    │ 只是标注用途，并不会把端口暴露到外网。真正对外开放的端 │
│                │ 口由 compose.yml 决定                                  │
├────────────────┼────────────────────────────────────────────────────────┤
│                │ 两者拼接成默认启动命令 /app/server -f                  │
│ ENTRYPOINT +   │ /app/settings.yaml。把参数单独放在 CMD 里，执行 docker │
│ CMD            │  compose run backend -f ... -db 时传入的参数只会替换   │
│                │ CMD，这样就能用同一个镜像执行数据库迁移                │
└────────────────┴────────────────────────────────────────────────────────┘

配置文件 settings.yaml 不打进镜像，而是运行时挂载进去。这样修改配置只需要重启容器，不用重新构建镜像；密码也不会留在镜像的分层里。

---

④ backend/settings.prod.yaml：后端的生产环境配置

字段名和你项目里 conf/*.go 的结构完全一致，包括数据库密码的字段名 passport。

system:
  ip: 0.0.0.0              # 监听所有网卡。写 127.0.0.1 的话，Nginx 容器无法访问后端
  port: 8080               # 容器内部端口，不对外开放
  mode: "release"          # 目前代码没有读取这个字段；运行模式由 Dockerfile 里的 GIN_MODE 控制

db:
  user: myzhihu_app        # 与 .env 的 DB_USER 一致
  passport: 替换为随机密码2  # 与 .env 的 DB_PASSWORD 一致。字段名是代码里的拼写，不能改成 password
  host: mysql              # Compose 的服务名；容器之间通过内置 DNS 用服务名互相访问
  port: 3306
  db_name: myzhihu         # 与 compose.yml 里的 MYSQL_DATABASE 一致
  debug: false             # true 时打印每条 SQL；生产环境关闭，只记录警告和慢查询
  source: mysql
  max_idle_conns: 10       # 空闲连接上限。代码会限制它不超过 10
  max_open_conns: 10       # 新代码已经固定为 10，这一项不再生效，写成 10 只是为了和代码保持一致
  conn_max_lifetime: "30m" # 连接使用 30 分钟后更换，避免被 MySQL 的 wait_timeout 断开

redis:
  enabled: true
  host: "redis"            # Compose 服务名
  port: 6379
  password: "替换为随机密码3" # 与 .env 的 REDIS_PASSWORD 一致
  db: 0                    # 种子程序重置数据时会对这个库执行 FLUSHDB。服务器上的 Redis 是专用的，所以没问题
  pool_size: 10
  min_idle_conns: 2
  dial_timeout: "5s"
  read_timeout: "3s"
  write_timeout: "3s"

rabbitmq:
  enabled: true
  host: "rabbitmq"         # Compose 服务名
  port: 5672
  username: "myzhihu2"     # 与 .env 的 RMQ_USER 一致
  password: "替换为随机密码4"
  vhost: "/"
  exchange: "myzhihu2.events"        # 后端启动时会自动声明交换机和队列，不需要手动创建
  exchange_type: "direct"
  queue: "myzhihu2.user_behavior"    # 用户行为事件 → 兴趣权重
  routing_key: "user.behavior"
  feed_queue: "myzhihu2.feed"        # 内容和关注事件 → Feed 写扩散
  feed_routing_key: "feed.content"

jwt:
  algorithm: "HS256"
  issuer: "myzhihu2"
  secret: "替换为新生成的JWT密钥"     # 用 openssl rand -base64 48 生成。绝不能沿用 git 里的旧密钥
  expire_hours: 24

feed:
  bigv_threshold: 100      # 粉丝数 ≥ 100 的作者改为读时拉取。演示数据里的 3 个大 V 粉丝数在 140–320 之间
  inbox_limit: 1000        # 每个用户收件箱 ZSET 最多保留 1000 条
  outbox_limit: 1000       # 每个大 V 发件箱 ZSET 最多保留 1000 条
  fanout_batch: 500        # 写扩散时每批处理的粉丝数
  explore_window_days: 7   # 探索流只从最近 7 天的内容里选
  explore_pool_size: 500   # 探索候选池的大小
  explore_refresh_seconds: 60

demo:
  enabled: true                    # 开启演示模式：一键体验登录，以及对演示账号的限制
  registration_enabled: false      # 关闭注册：注册页面保留，提交时返回 403 和中文提示
  reader_user_id: 10               # 与种子程序里 ReaderID = 10 一致
  bigv_user_id: 1                  # 与种子程序里 BigVTechID = 1 一致
  protected_max_content_id: 29999  # ID 不超过这个值的预置内容，演示账号不能修改或删除
  posts_per_hour: 10               # 演示大 V 每小时最多发 10 条

┌─────────────────────────────┬───────────────────────────────────────────┐
│ 和本地 settings.yaml 的区别 │                   原因                    │
├─────────────────────────────┼───────────────────────────────────────────┤
│ 各服务地址从 127.0.0.1      │ 在容器里，127.0.0.1                       │
│ 改为服务名                  │ 指的是容器自己，而不是宿主机              │
├─────────────────────────────┼───────────────────────────────────────────┤
│ 数据库账号从 root 改为      │                                           │
│ myzhihu_app，库名改为       │ 最小权限；生产环境的库名更清晰            │
│ myzhihu                     │                                           │
├─────────────────────────────┼───────────────────────────────────────────┤
│ 新的 JWT 密钥               │ 旧密钥已经提交进                          │
│                             │ git，任何拿到代码的人都能伪造登录 token   │
├─────────────────────────────┼───────────────────────────────────────────┤
│ Redis 和 RabbitMQ           │ 本地开发时没有密码；生产环境多一层保护    │
│ 都设置了密码                │                                           │
└─────────────────────────────┴───────────────────────────────────────────┘

demo 这一段必须写全。enabled 和 registration_enabled 没有默认值，漏写就都是 false：演示模式会整体关闭，而注册接口反而会开放。

---

① compose.yml：容器编排

name: myzhihu2

x-logging: &logging              # 公共日志配置，用 YAML 锚点让每个服务复用
  driver: json-file
  options: { max-size: "10m", max-file: "3" }   # 每个容器的日志最多 3 个文件、每个 10MB，防止写满磁盘

services:
  mysql:
    image: mysql:8.4             # 长期支持版本，和你本地的 8.4 一致
    restart: unless-stopped      # 崩溃或服务器重启后自动恢复；只有手动 stop 后才不会自动启动
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD:?}   # 从 .env 读取；:? 表示没有设置时 Compose 直接报错，不会用空密码启动
      MYSQL_DATABASE: myzhihu    # 首次启动时自动创建这个库
      MYSQL_USER: ${DB_USER:?}   # 首次启动时自动创建这个账号，并授予上面这个库的权限
      MYSQL_PASSWORD: ${DB_PASSWORD:?}
      TZ: Asia/Shanghai          # 和后端时区一致
    command:
      - --innodb-buffer-pool-size=256M   # 缓冲池默认 128M。2G 内存的机器给 256M，兼顾性能和内存
      - --performance-schema=OFF         # 关闭性能监控表，能省下大约 200–400MB 内存
      - --max-connections=60             # 后端 10 个连接，加上种子程序、备份、手动连接，60 个足够
    volumes:
      - mysql_data:/var/lib/mysql        # 数据存在命名卷里，删除容器后数据仍然保留
    healthcheck:                         # 其他服务依靠这个检查判断 MySQL 是否真正可用
      test: ["CMD-SHELL", "mysqladmin ping -h127.0.0.1 -uroot -p$${MYSQL_ROOT_PASSWORD} --silent"]
      interval: 10s
      retries: 12
      start_period: 40s                  # 首次启动初始化需要时间，这段时间内的失败不计入重试次数
    deploy: { resources: { limits: { memory: 700M } } }   # 内存上限，防止 MySQL 挤占其他服务
    logging: *logging

  redis:
    image: redis:7.4-alpine
    restart: unless-stopped
    command:
      - redis-server
      - --appendonly
      - "yes"                            # 开启 AOF 持久化，重启后 Feed 投影和兴趣权重还在
      - --requirepass
      - ${REDIS_PASSWORD:?}
      - --maxmemory
      - 256mb                            # 内存上限
      - --maxmemory-policy
      - noeviction                       # 满了就拒绝写入，而不是悄悄删掉 key（见下方说明）
    environment:
      REDIS_PASSWORD: ${REDIS_PASSWORD}  # 给下面的健康检查命令使用
    volumes: [redis_data:/data]
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a \"$$REDIS_PASSWORD\" --no-auth-warning ping | grep -q PONG"]
      interval: 10s
    logging: *logging

  rabbitmq:
    image: rabbitmq:4.3.6-management     # 和项目里 compose.middleware.yml 用的版本一致，自带管理页面
    restart: unless-stopped
    environment:
      RABBITMQ_DEFAULT_USER: ${RMQ_USER:?}
      RABBITMQ_DEFAULT_PASS: ${RMQ_PASSWORD:?}
    ports:
      - "127.0.0.1:15672:15672"          # 管理页面只绑定服务器本机，通过 SSH 隧道访问，公网访问不到
    volumes: [rabbitmq_data:/var/lib/rabbitmq]   # 持久化队列里的消息，重启后不丢
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 15s
      start_period: 30s
    deploy: { resources: { limits: { memory: 400M } } }
    logging: *logging

  backend:
    build: ./backend                     # 用 backend/Dockerfile 构建镜像
    image: myzhihu2-backend:latest       # 给构建好的镜像起名，下面的 seed 服务直接复用
    restart: unless-stopped
    volumes:
      - ./backend/settings.prod.yaml:/app/settings.yaml:ro   # 挂载生产配置；:ro 表示只读，容器内无法修改
    depends_on:                          # 三个依赖都通过健康检查后，才启动后端
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
      rabbitmq: { condition: service_healthy }
    deploy: { resources: { limits: { memory: 256M } } }
    logging: *logging
    # 注意：这里没有 ports，后端不对外开放，只有 Nginx 能通过内部网络访问 backend:8080

  seed:                                  # 种子数据和每日重置工具；平时不运行
    image: myzhihu2-backend:latest
    entrypoint: ["/app/seed"]            # 覆盖镜像默认的 /app/server，改为运行种子程序
    command: ["-f", "/app/settings.yaml", "-profile", "demo", "-seed", "20261107", "-reset", "-purge-queues"]
    # -profile demo：演示规模的数据；-seed：固定随机种子，每次生成的数据完全一样
    # -reset：清空业务表后重新写入（程序要求必须显式传这个参数）；-purge-queues：清空消息队列里引用旧数据的消息
    volumes:
      - ./backend/settings.prod.yaml:/app/settings.yaml:ro
    depends_on:
      mysql: { condition: service_healthy }
      redis: { condition: service_healthy }
      rabbitmq: { condition: service_healthy }
    profiles: ["tools"]                  # 属于 tools 分组，docker compose up 时不会启动，只能用 run 手动执行
    restart: "no"
    logging: *logging

  nginx:
    image: nginx:1.29-alpine
    restart: unless-stopped
    ports:
      - "80:80"                          # 整个系统只对外开放这两个端口
      - "443:443"
    volumes:
      - ./web/html:/usr/share/nginx/html:ro            # 前端静态文件；更新前端时替换这个目录即可，不用重启
      - ./nginx/default.conf:/etc/nginx/conf.d/default.conf:ro
      - ./certbot/www:/var/www/certbot:ro              # 申请证书时的验证文件
      - ./certbot/conf:/etc/letsencrypt:ro             # 证书文件
    depends_on: [backend]
    logging: *logging

volumes:                                 # 命名卷由 Docker 管理，位于 /var/lib/docker/volumes/ 下
  mysql_data: {}
  redis_data: {}
  rabbitmq_data: {}

┌────────────┬────────────────────────────────────────────────────────────┐
│  设计要点  │                            说明                            │
├────────────┼────────────────────────────────────────────────────────────┤
│ 只有 Nginx │ MySQL、Redis、RabbitMQ 的 5672 端口、后端，都没有映射到宿  │
│            │ 主机，公网完全无法访问。它们之间通过 Compose               │
│ 映射了端口 │ 默认网络、用服务名互相访问                                 │
├────────────┼────────────────────────────────────────────────────────────┤
│ $$ 和 $    │ 健康检查里写 $$，表示交给容器内的 shell 去展开变量；单个 $ │
│ 的区别     │  会被 Compose 在解析文件时就替换掉                         │
├────────────┼────────────────────────────────────────────────────────────┤
│ 内存分配（ │                                                            │
│ 总共       │ MySQL 700M + RabbitMQ 400M + Redis 256M + 后端 256M +      │
│ 2G，另有   │ Nginx 约 20M，再给系统留一些                               │
│ 2G swap）  │                                                            │
├────────────┼────────────────────────────────────────────────────────────┤
│ Redis      │ Feed 投影依赖"就绪标记 + 收件箱"两个 key 同时存在。如果用  │
│ 为什么选   │ LRU 策略只删掉了收件箱，就绪标记还在，用户看到的就是空     │
│ noeviction │ Feed，而且不会触发重建。宁可写入报错（错误会出现在日志里） │
│            │ ，也不要悄悄出错                                           │
├────────────┼────────────────────────────────────────────────────────────┤
│ 为什么单独 │ 复用后端镜像，不用另外构建；用 profiles 防止它随 up        │
│ 定义 seed  │ 自动运行，因为它会清空全部业务数据                         │
│ 服务       │                                                            │
└────────────┴────────────────────────────────────────────────────────────┘

---

⑤ nginx/default.conf：分两个阶段

阶段 1：只开 HTTP，用于申请证书和首次验证

# 写在 conf.d 目录下的文件会被包含在 nginx.conf 的 http{} 块里，所以这里可以定义限流区域
limit_req_zone $binary_remote_addr zone=api:10m  rate=10r/s;   # 普通接口：每个 IP 每秒 10 次
limit_req_zone $binary_remote_addr zone=auth:10m rate=10r/m;   # 登录、注册：每个 IP 每分钟 10 次，防止暴力破解密码

server {
    listen 80;
    server_name example.com www.example.com;     # 只响应这两个域名

    # Let's Encrypt 会访问 http://域名/.well-known/acme-challenge/xxx，来确认你确实控制这个域名
    location /.well-known/acme-challenge/ { root /var/www/certbot; }

    root /usr/share/nginx/html;                    # 前端打包后的文件
    index index.html;

    gzip on;                                       # 压缩文本类资源，JS 能从约 120KB 降到约 45KB
    gzip_types text/css application/javascript application/json image/svg+xml;

    add_header X-Content-Type-Options nosniff always;   # 禁止浏览器猜测文件类型，防止把上传内容当脚本执行
    add_header X-Frame-Options SAMEORIGIN always;      # 禁止被其他网站用 iframe 嵌入，防止点击劫持
    add_header Referrer-Policy strict-origin-when-cross-origin always;

    # Vite 打包的文件名里带内容哈希，内容一变文件名就变，所以可以放心让浏览器缓存 30 天
    location /assets/ { expires 30d; add_header Cache-Control "public, immutable"; }

    location /api/v1/auth/ {                       # 登录、一键体验、注册使用更严格的限流
        limit_req zone=auth burst=5 nodelay;       # 允许 5 次突发请求，超出返回 503
        proxy_pass http://backend:8080;
        include /etc/nginx/proxy_params_myzhihu;   # 见下方说明：实际部署时把这几行直接写在这里
    }

    location /api/ {
        limit_req zone=api burst=20 nodelay;
        client_max_body_size 256k;                 # 请求体上限。正文最多 2 万字符，按 UTF-8 计算不超过 80KB，256k 足够
        proxy_pass http://backend:8080;            # 原样转发路径（proxy_pass 后面没有 URI 部分）
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;   # 把真实客户端 IP 传给后端
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 5s;                  # 后端没启动时，5 秒内返回 502，不会一直卡住
        proxy_read_timeout 60s;
    }

    # 前端用的是 history 路由，像 /question/123 这样的路径并不对应真实文件，所以统一返回 index.html
    location / { try_files $uri $uri/ /index.html; }
}

▎ include /etc/nginx/proxy_params_myzhihu; 这一行只是为了展示时少重复几行。**实际写配置时，请把 /api/ 里那 6 行 proxy_set_header 和 proxy_*_timeout 原样复制到 /api/v1/auth/ 里，并删掉这行 include。**否则 Nginx 找不到这个文件，会启动失败。

阶段 2：拿到证书后替换为下面的版本

limit_req_zone $binary_remote_addr zone=api:10m  rate=10r/s;
limit_req_zone $binary_remote_addr zone=auth:10m rate=10r/m;

server {                                           # HTTP 只保留两个功能：证书续期验证，以及跳转到 HTTPS
    listen 80;
    server_name example.com www.example.com;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }   # 续期时仍然需要走 HTTP 验证
    location / { return 301 https://example.com$request_uri; }       # 永久跳转到 HTTPS 主域名
}

server {                                           # www 统一跳转到不带 www 的主域名，避免同一个站点有两个地址
    listen 443 ssl;
    http2 on;
    server_name www.example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
    return 301 https://example.com$request_uri;
}

server {
    listen 443 ssl;
    http2 on;                                      # HTTP/2：一个连接上并行传输多个文件
    server_name example.com;

    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;  # 证书链（站点证书 + 中间证书）
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;    # 私钥
    ssl_protocols TLSv1.2 TLSv1.3;                 # 禁用已不安全的 TLS 1.0 和 1.1
    ssl_session_cache shared:SSL:10m;              # 复用 TLS 会话，减少重复握手的开销

    add_header Strict-Transport-Security "max-age=31536000" always;   # HSTS：一年内浏览器只会用 HTTPS 访问本站
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options SAMEORIGIN always;
    add_header Referrer-Policy strict-origin-when-cross-origin always;

    root /usr/share/nginx/html;
    index index.html;
    gzip on;
    gzip_types text/css application/javascript application/json image/svg+xml;

    location /assets/ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        add_header X-Content-Type-Options nosniff always;   # Nginx 的规则：location 里一旦写了 add_header，外层的 add_header 就不再继承，所以要重写一遍
    }

    location /api/v1/auth/ {
        limit_req zone=auth burst=5 nodelay;
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
        client_max_body_size 256k;
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 5s;
        proxy_read_timeout 60s;
    }

    location / { try_files $uri $uri/ /index.html; }
}

要点: 为什么分两个阶段
说明: 阶段 2 引用了证书文件，证书还不存在时 Nginx 会启动失败。所以先用阶段 1
  跑起来、申请证书，再切换到阶段 2
────────────────────────────────────────
要点: 为什么不在 Nginx 拦截注册接口
说明: 你希望保留注册功能并展示 403 拦截，所以交给后端的 registration_enabled
  来拒绝，返回中文提示。Nginx 只负责限流
────────────────────────────────────────
要点: 和 Cloudflare 的关系
说明: 这套配置假设 DNS 记录是灰色云朵（不走代理），$remote_addr 就是用户的真实
  IP。如果以后开启橙色云朵，需要加上 set_real_ip_from（Cloudflare 的 IP 段）和
  real_ip_header CF-Connecting-IP，否则限流拿到的全是 Cloudflare 节点的
  IP，会误伤所有用户

---

⑥ 服务器上的定时任务（crontab -e 中添加）

cron
# 每周一 03:00 检查证书，剩余有效期不足 30 天时才真正续期；之后让 Nginx 重新加载证书
0 3 * * 1 cd ~/myzhihu2 && docker run --rm -v "$PWD/certbot/www:/var/www/certbot" -v "$PWD/certbot/conf:/etc/letsencrypt" certbot/certbot renew --quiet && docker compose exec -T nginx nginx -s reload

# 每天 04:00 备份数据库：导出后压缩，只保留最近 7 天
0 4 * * * cd ~/myzhihu2 && mkdir -p backups && docker compose exec -T mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction myzhihu' | gzip > backups/myzhihu_$(date +\%F).sql.gz && find backups -name '*.sql.gz' -mtime +7 -delete

# 每天 04:30 重置演示数据（放在备份之后）；然后重启后端，清空内存里的限流计数和缓存
30 4 * * * cd ~/myzhihu2 && docker compose run --rm seed >> backups/reset.log 2>&1 && docker compose restart backend

┌──────────────────┬──────────────────────────────────────────────────────┐
│       要点       │                         说明                         │
├──────────────────┼──────────────────────────────────────────────────────┤
│ exec -T          │ 定时任务没有终端，-T                                 │
│                  │ 表示不分配伪终端，否则命令会报错                     │
├──────────────────┼──────────────────────────────────────────────────────┤
│ --single-transac │ 在一个一致性快照里导出数据，不锁表，备份期间网站仍然 │
│ tion             │ 可以正常访问                                         │
├──────────────────┼──────────────────────────────────────────────────────┤
│ \%               │ crontab 里 % 是特殊字符，需要转义                    │
├──────────────────┼──────────────────────────────────────────────────────┤
│ 为什么先备份再重 │ 万一种子程序出问题，还能用前半小时的备份恢复         │
│ 置               │                                                      │
└──────────────────┴──────────────────────────────────────────────────────┘

---

⑦ 配套的本地编译命令（不是配置文件，只是让上面的文件能用起来）

cd ~/my_project/myzhihu2/backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ~/myzhihu2-deploy/backend/server ./cmd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ~/myzhihu2-deploy/backend/seed   ./cmd/seed
cd ../frontend && npm ci && npm run build && mkdir -p ~/myzhihu2-deploy/web/html && cp -r dist/* ~/myzhihu2-deploy/web/html/

┌─────────────────┬───────────────────────────────────────────────────────┐
│      参数       │                         意义                          │
├─────────────────┼───────────────────────────────────────────────────────┤
│ CGO_ENABLED=0   │ 编译成纯静态程序，不依赖系统的 C 库。Alpine 用的是    │
│                 │ musl 而不是 glibc，动态链接的程序在 Alpine 里跑不起来 │
├─────────────────┼───────────────────────────────────────────────────────┤
│ GOOS=linux      │ 交叉编译出 Linux x86_64 版本；ARM 服务器改成 arm64    │
│ GOARCH=amd64    │                                                       │
├─────────────────┼───────────────────────────────────────────────────────┤
│ -trimpath       │                                                       │
│ -ldflags="-s    │ 去掉本地路径和调试符号，程序体积能减小约 30%          │
│ -w"             │                                                       │
└─────────────────┴───────────────────────────────────────────────────────┘

---

⑧ 域名解析（example.com）

┌──────┬──────────┬──────────┬────────────────────────────────────────────┐
│ 类型 │ 主机记录 │  记录值  │                    说明                    │
├──────┼──────────┼──────────┼────────────────────────────────────────────┤
│ A    │ @        │ 服务器   │ 主域名 example.com                        │
│      │          │ IP       │                                            │
├──────┼──────────┼──────────┼────────────────────────────────────────────┤
│ A    │ www      │ 服务器   │ www.example.com，访问后由 Nginx           │
│      │          │ IP       │ 跳转到主域名                               │
└──────┴──────────┴──────────┴────────────────────────────────────────────┘

如果用 Cloudflare 管理 DNS，选灰色云朵。如果是在国内注册商买的域名，要先完成域名实名认证，否则解析不会生效。服务器在香港，不需要 ICP 备案。

---

首次部署的执行顺序

本地：生成 4 个随机密码和 JWT 密钥 → 填写 .env 和 settings.prod.yaml → 编译 server、seed 和前端 → rsync 上传
服务器：安装 Docker → docker compose up -d mysql redis rabbitmq（等待全部 healthy）
      → docker compose build backend
      → docker compose run --rm backend -f /app/settings.yaml -db      （建表）
      → docker compose run --rm seed                                  （写入演示数据）
      → docker compose up -d backend nginx（使用阶段 1 的 Nginx 配置）→ 用 http://example.com 验证
      → 用 certbot 申请证书 → 换成阶段 2 的配置 → nginx -s reload → 用 https://example.com 验证
      → 设置定时任务

需要的话，我可以接着带你一步步完成服务器上的操作，从首次登录做安全初始化开始。你也可以先自己按上面的文件创建 ~/myzhihu2-deploy，遇到报错时把输出贴给我。
