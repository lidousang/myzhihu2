1. 框架指令 go get gopkg.in/yaml.v2 github.com/gin-gonic/gin gorm.io/gorm gorm.io/driver/mysql github.com/redis/go-redis/v9 github.com/rabbitmq/amqp091-go
2. 表结构与缓存总结
下面沿用我们讨论的五张 MySQL 表：users、contents、user_follows、reactions、behavior_events。我会先解释字段，再用一些用户行为演示 MySQL 和 Redis 分别发生什么变化。

需要先记住两点：

- MySQL 是关系型数据库：数据放在有列和行的表里，外键表达表之间的关系。
- Redis 不是 MySQL 的另一套表：它用 key 保存不同数据结构。我们会用 Redis 保存可重建的 Feed 索引、兴趣信号和可选的内容缓存。

———

# 一、五个结构体的字段

## 1. User：用户表

type User struct {
    ID           uint
    Username     string
    PasswordHash string
    DisplayName  string
    Avatar       string
    Bio          string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

| 字段 | MySQL 含义 |
|---|---|
| ID | 用户主键，其他表通过它标识用户 |
| Username | 登录名，要求唯一 |
| PasswordHash | 密码的哈希值，不保存明文密码 |
| DisplayName | 页面显示的名字 |
| Avatar | 头像地址 |
| Bio | 个人简介 |
| CreatedAt | 用户创建时间 |
| UpdatedAt | 最近修改时间 |

PasswordHash 应当使用 json:"-"，序列化响应时不返回。用户发布内容时，contents.author_id 会指向 users.id。

## 2. Content：问题、回答、文章和评论

type Content struct {
    ID        uint
    AuthorID  uint
    Kind      string
    ParentID  *uint
    Title     string
    Body      string
    CreatedAt time.Time
    UpdatedAt time.Time
    DeletedAt gorm.DeletedAt

    Author User
    Parent *Content
}

| 字段 | MySQL 含义 |
|---|---|
| ID | 内容主键；所有内容类型共用同一个 ID 空间 |
| AuthorID | 作者 ID，外键指向 users.id |
| Kind | 内容类型：question、answer、article 或 comment |
| ParentID | 父内容 ID，允许为空，自关联到 contents.id |
| Title | 标题，主要供问题和文章使用 |
| Body | 正文 |
| CreatedAt | 创建时间 |
| UpdatedAt | 最近修改时间 |
| DeletedAt | 软删除时间；为空表示未删除 |
| Author | GORM 的关联字段，不是额外的一列；对应 AuthorID |
| Parent | GORM 的自关联字段，不是额外的一列；对应 ParentID |

ParentID 的业务含义取决于 Kind：

| Kind | ParentID |
|---|---|
| question | 空 |
| article | 空 |
| answer | 指向所属问题 |
| comment | 指向被评论的问题、回答或文章 |

外键只能保证 ParentID 指向存在的内容，不能单独保证“回答的父级一定是问题”。这类类型规则要由 Content Service 检查。

## 3. UserFollow：用户关注关系

type UserFollow struct {
    FollowerID  uint
    FollowingID uint
    CreatedAt   time.Time

    Follower User
    Following User
}

| 字段 | 含义 |
|---|---|
| FollowerID | 发起关注的人，外键指向 users.id |
| FollowingID | 被关注的人，外键指向 users.id |
| CreatedAt | 关注关系创建时间 |
| Follower | GORM 关联字段，不是额外列 |
| Following | GORM 关联字段，不是额外列 |

FollowerID + FollowingID 组成复合主键，防止重复关注。业务层还应阻止用户关注自己。

例如“Alice 关注 Bob”时：

FollowerID  = Alice 的用户 ID
FollowingID = Bob 的用户 ID

## 4. Reaction：点赞和收藏关系

type Reaction struct {
    UserID    uint
    ContentID uint
    Kind      string
    CreatedAt time.Time

    User    User
    Content Content
}

| 字段 | 含义 |
|---|---|
| UserID | 做出互动的用户，外键指向 users.id |
| ContentID | 互动目标，外键指向 contents.id |
| Kind | 互动类型：like 或 collect |
| CreatedAt | 互动创建时间 |
| User | GORM 关联字段，不是额外列 |
| Content | GORM 关联字段，不是额外列 |

UserID + ContentID + Kind 组成唯一约束。这样同一个用户可以点赞一条内容、收藏同一条内容，但不能重复点赞两次。

## 5. BehaviorEvent：用户行为记录

type BehaviorEvent struct {
    ID        uint
    UserID    uint
    ContentID uint
    Kind      string
    Duration  int
    CreatedAt time.Time

    User    User
    Content Content
}

| 字段 | 含义 |
|---|---|
| ID | 行为记录主键 |
| UserID | 行为用户，外键指向 users.id |
| ContentID | 产生行为的内容，外键指向 contents.id |
| Kind | 行为类型，例如 view、read_time、like、collect |
| Duration | 阅读时长，单位为秒；非阅读行为可为 0 |
| CreatedAt | 行为发生时间 |
| User | GORM 关联字段，不是额外列 |
| Content | GORM 关联字段，不是额外列 |

Reaction 和 BehaviorEvent 有意区分：

- Reaction 表示“用户现在是否点赞或收藏”。
- BehaviorEvent 表示“用户曾经做过什么”，可以为兴趣流提供信号。

———

# 二、这些表之间的关系

users 1 ─── N contents
contents 1 ─── N contents       // 问题→回答，内容→评论
users N ─── N users              // 通过 user_follows
users N ─── N contents           // 通过 reactions
users 1 ─── N behavior_events
contents 1 ─── N behavior_events

例如，Alice 发布一个问题、Bob 回答、Carol 点赞，就会分别通过 contents.author_id、contents.parent_id 和 reactions 关联起来。

———

# 三、行为发生时 MySQL 如何变化

下面假定：

Alice 的 ID = 1
Bob 的 ID   = 2
问题 ID     = 100
回答 ID     = 101

## 1. 注册用户

Alice 注册成功后，users 增加一行：

| id | username | password_hash | display_name |
|---:|---|---|---|
| 1 | alice | 哈希后的密码 | Alice |

密码原文不会进入数据库。

## 2. Bob 发布问题

POST /api/v1/contents

请求中的 kind 是 question。MySQL 的 contents 增加：

| id | author_id | kind | parent_id | title | body |
|---:|---:|---|---:|---|---|
| 100 | 2 | question | NULL | 如何学习 Go？ | 问题正文 |

问题创建后，业务服务还会发布一个 RabbitMQ content.created 事件。Feed 消费者处理后，可能把内容 ID 100 写入关注 Bob 的用户 Redis Feed。

## 3. Alice 关注 Bob

MySQL 的 user_follows 增加：

| follower_id | following_id | created_at |
|---:|---:|---|
| 1 | 2 | 关注时间 |

这条记录是关注关系的权威数据。之后 Bob 发布新内容时，Feed 消费者可以用 following_id = 2 查询粉丝 follower_id = 1。

## 4. Bob 发布回答

回答通过嵌套路由创建：

POST /api/v1/questions/100/answers

contents 增加一行：

| id | author_id | kind | parent_id | title | body |
|---:|---:|---|---:|---|---|
| 101 | 2 | answer | 100 | NULL | 回答正文 |

这里 parent_id = 100 表示回答属于问题 100。

然后发布 RabbitMQ 事件。Feed Consumer 查询 Bob 的粉丝，并将 101 写入每位粉丝的 Redis Feed。

## 5. Alice 点赞回答

POST /api/v1/contents/101/like

MySQL reactions 增加：

| user_id | content_id | kind |
|---:|---:|---|
| 1 | 101 | like |

同时，按设计可以在 behavior_events 增加：

| id | user_id | content_id | kind | duration |
|---:|---:|---:|---|---:|
| 1 | 1 | 101 | like | 0 |

之后可以发送 RabbitMQ user.behavior 事件，让行为消费者异步更新 Alice 的 Redis 兴趣信号。

如果 Alice 取消点赞，删除 reactions 中对应的 like 行。是否记录 unlike 行为取决于推荐逻辑；初版可以不记录。

## 6. Alice 收藏回答

POST /api/v1/contents/101/collection

reactions 增加另一行：

| user_id | content_id | kind |
|---:|---:|---|
| 1 | 101 | collect |

同一内容可以同时有 like 和 collect，因为唯一约束包含 kind。

## 7. Alice 浏览和阅读

浏览或阅读时，可以调用行为接口：

POST /api/v1/behaviors

例如阅读 45 秒后，behavior_events 增加：

| id | user_id | content_id | kind | duration |
|---:|---:|---:|---|---:|
| 2 | 1 | 101 | read_time | 45 |

这条行为可以发布到 user.behavior 路由键，消费者更新 Redis 中的兴趣权重。

———

# 四、Redis 不是关系表：用 key 和数据结构缓存

可以把 Redis 看成一个运行在服务端的高速 key-value 数据库。Go 服务通过 Redis 客户端发送命令，Redis 返回结果：

Go 服务 -- GET key --> Redis
Go 服务 <-- value --- Redis

Redis 中没有 MySQL 式的列、行和外键。每个数据项由 key、类型和值组成。MySQL 负责可靠保存业务事实；Redis 保存用于加速、可以重建的派生数据。

## 1. 关注 Feed：Sorted Set

当 Bob 发布回答 101，RabbitMQ 消费者处理后：

Key:    feed:user:1
Type:   ZSET
Member: 101
Score:  回答发布时间

例如：

feed:user:1
  "101" → 1790000000
  "100" → 1789990000

- feed:user:1：Alice 的 Feed。
- member 101：内容 ID。
- score：发布时间，用于排序。
- ZADD 写入或更新条目。
- ZREVRANGE 按 score 从高到低读取最新内容。

因为 contents.id 全局唯一，Feed 只需保存内容 ID；读取时再查询内容详情。若希望 key 更可读，也可用 content:101 作为 member。

## 2. 兴趣信号：Hash

Alice 点赞和收藏了 Bob 的回答后，行为消费者可以维护：

Key: interest:user:1
Type: HASH

例如：

author:2    → 3
kind:answer → 2

它表达的是 Alice 对作者 Bob 和回答类型有一定兴趣。消费者可以通过类似 HINCRBY 的命令增加权重。

用户请求主页时，Feed Service：

1. 读取 interest:user:1。
2. 根据较高的作者或内容类型权重查询 MySQL。
3. 将查询结果作为兴趣流候选。
4. 和关注流、探索流混合。

这个 Hash 是兴趣摘要，不是完整的行为记录。完整行为仍在 behavior_events 表中。

## 3. 内容详情缓存：String 存 JSON

可选地缓存内容详情：

Key:  content:101
Type: STRING
Value: JSON 格式的回答详情
TTL:  例如 5 分钟

缓存读取采用常见的 Cache Aside（旁路缓存）：

读取 content:101
  ├─ Redis 命中：解析 JSON 并返回
  └─ Redis 未命中：
       查询 MySQL contents.id=101
       将详情写入 Redis，设定过期时间
       返回详情

修改内容时，更新 MySQL 后删除 content:101，让下一次读取重新加载新数据。Redis 故障或 key 过期时仍可查 MySQL。

———

# 五、主页读取时两边如何配合

假设 Alice 打开主页：

GET /api/v1/feed?page=1&limit=20

Feed Service 可以按简单流程工作：

1. 从 Redis feed:user:1 读取关注流内容 ID
2. 从 Redis interest:user:1 读取兴趣信号
3. 根据兴趣信号查 MySQL，生成兴趣流候选
4. 从 MySQL 随机取探索流候选
5. 按固定比例合并三路候选
6. 对内容 ID 查询 Redis 详情缓存
7. 缓存未命中时查询 MySQL contents
8. 返回统一 Feed 卡片

数据变化和读取来源可总结为：

| 数据 | 权威来源/生成方式 | Redis 的用途 |
|---|---|---|
| 用户资料 | MySQL users | 可选详情缓存 |
| 内容正文 | MySQL contents | 可选 JSON 详情缓存 |
| 关注关系 | MySQL user_follows | 可选关注 Feed 加速 |
| 点赞、收藏当前状态 | MySQL reactions | 可以批量查询后附加状态 |
| 浏览、阅读历史 | MySQL behavior_events | 汇总为兴趣 Hash |
| 关注流候选 | 由发布事件和关注关系生成 | feed:user:{id} ZSET |
| 兴趣信号 | 从行为事件异步汇总 | interest:user:{id} HASH |
| 探索候选 | 初版从 MySQL 简单抽取 | 暂时不必专设 Redis key |

## 一条重要的异步边界

MySQL 写入成功之后，服务再发布 RabbitMQ 事件。消费者稍后更新 Redis，因此有一个短暂时间窗口：

MySQL 已有新回答
Redis Feed 还没包含该回答

这就是异步处理带来的短暂延迟。RabbitMQ 消费成功后，Redis 会被更新；浏览接口也可以在 Redis Feed 为空时从 MySQL 重新构建候选。

在初版中，建议把关注流 Redis ZSET和兴趣信号 Redis Hash作为 Feed 相关的主要缓存结构，详情 JSON 缓存作为可选优化。这样能清楚展示 MySQL、RabbitMQ、Redis 各自承担的工作


==========================================
- Feed Handler (my_project/myzhihu2/backend/api/feed/feed.go)：读取 JWT 用户 ID，解析 cursor 和 limit，调用 Feed Service。
- Feed Service (my_project/myzhihu2/backend/service/feed.go)：收集关注、兴趣、探索候选；按内容 ID 去重；批量读取详情和互动状态；过滤软删除内容。
- Feed Redis 封装 (my_project/myzhihu2/backend/repository/cache/feed.go)：维护关注流 ZSET、Feed 初始化标记和兴趣权重 Hash。
- Feed DAO (my_project/myzhihu2/backend/repository/dao/content.go)：提供基于 ID 游标的关注/兴趣/探索查询，以及批量详情查询。
- RabbitMQ 消费者 (my_project/myzhihu2/backend/feed/consumer.go)：消费内容、关注和行为事件，更新 Redis。
- Feed 响应 DTO (my_project/myzhihu2/backend/serializer/feed.go)：定义统一的 Feed 卡片和分页结果。

## 三路数据如何进入 Feed

- 关注流：feed:user:{userID} 是 Redis ZSET，member 和 score 都使用内容 ID。发布内容后，RabbitMQ Feed 消费者查作者粉丝，把内容 ID 加进粉丝的 ZSET；新关注时回填该作者最近 100 条内容。
- 兴趣流：点赞、收藏和 /behaviors 上报会写 behavior_events 并发布 user.behavior 消息。行为消费者根据内容作者和类型更新 interest:user:{userID} Hash。读取 Feed 时，根据权重较高的作者和内容类型从 MySQL 查询候选；Redis 信号为空时，从 MySQL 最近行为记录计算候选。
- 探索流：从 MySQL 对未删除的问题、回答、文章中简单随机取样，不维护已读历史。

三类候选合并时按 ID 去重，并按内容 ID 倒序排列。响应带 source，用于区分 following、interest 和 explore。当前实现不保证每页严格符合 5:3:2 比例：严格交错和单一 ID 游标会导致跨页漏项；这里优先保证稳定翻页和完整性，来源比例由实际候选数量决定。

## 三个问题如何处理

1. 深分页：移除 offset 翻页。第一页 cursor 为空；后续页面使用上一页最后一条内容 ID，Redis 和 MySQL 都只查询 id < cursor，避免深 OFFSET 扫描和新内容插入引起的页码漂移。
2. 重复和漏内容：Redis ZSET 的 member 唯一，RabbitMQ 重复投递同一内容不会产生重复成员；三个候选池在当前页按内容 ID 去重；每个候选源都使用同一 ID 游标，并对最终结果按 ID 倒序，因此已返回 ID 不会出现在下一页。Service 会超取候选、过滤已软删除详情，并清理关注 ZSET 中发现的失效 ID。刷新第一页时仍可能再次看到旧内容；没有实现已读过滤。
3. 缓存击穿：关注 Feed 投影不设短 TTL，使用 feed:ready:{userID} 区分“已构建但为空”和“尚未构建”。缓存未构建时，Service 以 singleflight 合并同一进程内同一用户的并发重建，并在重建前二次检查标记；Redis 读取失败或 RabbitMQ 未启用时，关注候选退回 MySQL 查询。

## 发布与浏览流程

发布问题/回答/文章
  → Content Service 写 MySQL
  → 发布 content.created 到 feed.content
  → Feed Consumer 查询作者粉丝
  → ZADD 写入各粉丝 feed:user:{id}

用户点赞/收藏/上报阅读
  → 写入 reactions / behavior_events
  → 发布 user.behavior
  → Behavior Consumer 更新 interest:user:{id}

GET /api/v1/feed
  → 读关注 ZSET / 兴趣候选 / 随机探索候选
  → 去重、详情补全、过滤删除内容、批量补互动状态
  → 按 ID 游标返回一页

cmd/main.go 现在会初始化 RabbitMQ 并启动消费者。若 rabbitmq.enabled 为 true，启动应用前需要 RabbitMQ 已经运行；若将其设为 false，Feed 读取可退回 MySQL，但 RabbitMQ 异步更新不会运行。

## API 使用

获取第一页：

GET /api/v1/feed?limit=20
Authorization: Bearer <access_token>

响应大致如下：

{
  "data": {
    "items": [
      {
        "content": {
          "id": 101,
          "kind": "answer",
          "title": "问题标题",
          "body": "回答正文"
        },
        "source": "following",
        "like_count": 3,
        "collection_count": 1,
        "liked": false,
        "collected": true
      }
    ],
    "next_cursor": "101",
    "has_more": true
  }
}

请求下一页时，把 next_cursor 原样传回：

GET /api/v1/feed?limit=20&cursor=101
Authorization: Bearer <access_token>

行为上报 API 沿用现有定义；浏览或阅读时长可调用：

POST /api/v1/behaviors
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "content_id": 101,
  "kind": "read_time",
  "duration": 45
}

点赞与收藏仍使用已有的 /api/v1/contents/:contentID/like 和 /collection 接口；首次成功互动会同时记录行为并向兴趣队列发布事件。

