# 精简后的数据库模型与 API 设计

下面以重构后的目标为准，围绕两个核心目标设计：

1. 完成注册、内容发布、评论、点赞、收藏、关注等通用后端功能。
2. 用关注流、兴趣流和探索流组成一个主页 Feed，并体现 Redis、RabbitMQ 的作用。

为了让混合内容 Feed 和通用内容 API 更简单，建议用一张 contents 表统一存放问题、回答、文章和评论。这样不同类型的内容共用一个 ID，也能复用 Feed、详情和互动接口。

## 一、数据库模型

初版建议保留 5 张核心表：

users
contents
user_follows
reactions
behavior_events

Redis 和 RabbitMQ 是基础设施，不属于 MySQL 模型：Redis 保存可重建的 Feed 和兴趣数据，RabbitMQ 负责异步传递事件。

### 1. users：用户

| 字段 | 含义 |
|---|---|
| id | 用户 ID |
| username | 用户名，唯一 |
| password_hash | 密码哈希 |
| display_name | 显示名称 |
| avatar | 头像 |
| bio | 个人简介 |
| created_at、updated_at | 创建和更新时间 |

**作用：**支持注册、登录、JWT 身份识别、个人页面和内容作者展示。密码不能以明文保存。

### 2. contents：问题、回答、文章、评论

| 字段 | 含义 |
|---|---|
| id | 全局唯一内容 ID |
| author_id | 作者 ID |
| kind | question、answer、article 或 comment |
| parent_id | 父内容 ID，可为空 |
| title | 标题；问题和文章使用 |
| body | 正文 |
| created_at、updated_at | 创建和更新时间 |
| deleted_at | 可选软删除时间 |

parent_id 表达内容之间的从属关系：

- 问题和文章没有父内容。
- 回答的 parent_id 指向问题。
- 评论的 parent_id 指向它评论的问题、回答或文章。

应用层校验类型约束，例如回答必须属于问题，评论不能作为普通 Feed 内容展示。字段可以用普通字符串加应用层校验，不必使用复杂的数据库枚举。

**作用：**统一内容创建、详情读取、评论、Feed 混排和互动目标。Feed 可以只引用 content_id，不必额外处理不同内容表之间可能重复的 ID。

### 3. user_follows：用户关注关系

| 字段 | 含义 |
|---|---|
| follower_id | 发起关注的用户 |
| following_id | 被关注用户 |
| created_at | 关注时间 |

对 (follower_id, following_id) 建唯一约束，并禁止用户关注自己。

**作用：**支持关注操作，也用于找出作者的粉丝，从而把新内容加入粉丝的 Feed。

### 4. reactions：点赞与收藏

| 字段 | 含义 |
|---|---|
| user_id | 操作用户 |
| content_id | 互动目标 |
| kind | like 或 collect |
| created_at | 创建时间 |

对 (user_id, content_id, kind) 建唯一约束。同一用户可以同时点赞和收藏同一条内容。取消互动时删除对应关系记录。

**作用：**用一张关系表支持点赞和收藏，避免分别维护 Vote、Collection 两套相似逻辑。初版不包含点踩。

### 5. behavior_events：用户行为记录

| 字段 | 含义 |
|---|---|
| id | 行为记录 ID |
| user_id | 行为用户 |
| content_id | 行为目标 |
| kind | view、read_time、like、collect 等 |
| duration | 阅读时长，可为空 |
| created_at | 行为发生时间 |

reactions 保存用户当前是否点赞或收藏；behavior_events 保存用户做过什么行为，供兴趣推荐参考。这两个表记录的是不同含义。

**作用：**为兴趣流提供简单行为信号。点赞和收藏成功后，服务端可以记录行为或发布行为事件；查看内容时可以通过行为 API 上报浏览或阅读时长。

初版不需要额外建话题表、问题关注表、Feed 内容表或 Outbox 表。后续确实要做话题推荐或更可靠的事件投递时，再按需求增加。

## 二、数据库模型关系

┌─────────────────┐
│ USERS           │
│                 ├──(1) authors────────┐
│                 ├──(1) follower───────╪───┐
│                 ├──(1) following──────╪───╪─┐
│                 ├──(1) creates────────╪───╪─╪─┐
│                 ├──(1) performs───────╪───╪─╪─╪───┐
└─────────────────┘                     │   │ │ │   │
                                        │   │ │ │   │
                                        │   │ │ │   │
┌─────────────────┐                     │   │ │ │   │
│ CONTENTS        │                     │   │ │ │   │
│                 ├──(0..many)──────────┘   │ │ │   │
│                 ├──(1) parent_child─────┐ │ │ │   │
│                 ├──(0..many)────────────┘ │ │ │   │
│                 ├──(1) receives───────────╪─╪─╪─┐ │
│                 ├──(1) receives───────────╪─╪─╪─╪─╪─┐
└─────────────────┘                         │ │ │ │ │ │
                                            │ │ │ │ │ │
                                            │ │ │ │ │ │
┌─────────────────┐                         │ │ │ │ │ │
│ USER_FOLLOWS    │                         │ │ │ │ │ │
│                 ├──(0..many)──────────────┘ │ │ │ │ │
│                 ├──(0..many)────────────────┘ │ │ │ │
└─────────────────┘                             │ │ │ │
                                                │ │ │ │
                                                │ │ │ │
┌─────────────────┐                             │ │ │ │
│ REACTIONS       │                             │ │ │ │
│                 ├──(0..many)──────────────────┘ │ │ │
│                 ├──(0..many)────────────────────┘ │ │
└─────────────────┘                                 │ │
                                                    │ │
                                                    │ │
┌─────────────────┐                                 │ │
│ BEHAVIOR_EVENTS │                                 │ │
│                 ├──(0..many)──────────────────────┘ │
│                 ├──(0..many)────────────────────────┘
└─────────────────┘

换成业务语言：

- 一个用户可以发布多条内容。
- 一条问题可以有多条回答；一条内容可以有多条评论。
- 用户通过 user_follows 形成用户到用户的多对多关注关系。
- 用户通过 reactions 对内容点赞或收藏。
- 用户通过 behavior_events 留下浏览、阅读等推荐信号。

关系表可使用外键指向 users.id 和 contents.id。contents.parent_id 是自关联。应用层负责校验父子内容类型。

## 三、通用后端 API

### 注册、登录与个人资料

| 方法与路径 | 功能 | 认证 |
|---|---|---|
| POST /api/v1/auth/register | 注册用户 | 否 |
| POST /api/v1/auth/login | 登录并获取 JWT | 否 |
| GET /api/v1/users/me | 获取当前用户资料 | 是 |
| PUT /api/v1/users/me | 修改当前用户资料 | 是 |
| GET /api/v1/users/:userID | 获取公开个人资料 | 否 |

### 内容发布和读取

| 方法与路径 | 功能 | 认证 |
|---|---|---|
| POST /api/v1/contents | 创建问题或文章 | 是 |
| POST /api/v1/questions/:questionID/answers | 给问题创建回答 | 是 |
| GET /api/v1/contents/:contentID | 获取内容详情 | 否 |
| PUT /api/v1/contents/:contentID | 修改本人内容 | 是 |
| DELETE /api/v1/contents/:contentID | 删除本人内容 | 是 |
| GET /api/v1/questions/:questionID/answers | 获取问题的回答列表 | 否 |
| GET /api/v1/users/:userID/contents?kind=answer | 获取用户发布的内容 | 否 |

POST /contents 的请求通过 kind 表示创建问题还是文章：

{
  "kind": "question",
  "title": "如何理解 Feed？",
  "body": "问题正文……"
}

回答使用嵌套路由，是因为回答必须属于一个问题。

### 评论、点赞、收藏和关注

| 方法与路径 | 功能 | 认证 |
|---|---|---|
| GET /api/v1/contents/:contentID/comments | 获取评论 | 否 |
| POST /api/v1/contents/:contentID/comments | 创建评论 | 是 |
| POST /api/v1/contents/:contentID/like | 点赞 | 是 |
| DELETE /api/v1/contents/:contentID/like | 取消点赞 | 是 |
| POST /api/v1/contents/:contentID/collection | 收藏 | 是 |
| DELETE /api/v1/contents/:contentID/collection | 取消收藏 | 是 |
| POST /api/v1/users/:userID/follow | 关注用户 | 是 |
| DELETE /api/v1/users/:userID/follow | 取消关注 | 是 |

所有写操作都从 JWT 中获取操作者 ID，不信任请求体提交的 user_id。

## 四、Feed 功能 API

### 主页混合 Feed

GET /api/v1/feed?page=1&limit=20

这一个 API 返回关注、兴趣、探索三类候选混合后的内容。前端不需要分别请求三种 Feed。

响应中的 source 字段用于说明内容来自哪条候选流：

{
  "items": [
    {
      "kind": "answer",
      "id": 101,
      "title": "如何理解 Feed？",
      "source": "following"
    },
    {
      "kind": "article",
      "id": 205,
      "title": "推荐系统入门",
      "source": "interest"
    },
    {
      "kind": "question",
      "id": 310,
      "title": "最近有哪些 Go 新特性？",
      "source": "explore"
    }
  ],
  "page": 1,
  "limit": 20
}

初版使用 page/limit 即可。允许不同请求中重复推荐内容，不做已读历史过滤或复杂的防重复推荐机制。

### 浏览和阅读行为上报

POST /api/v1/behaviors

请求示例：

{
  "content_id": 101,
  "kind": "read_time",
  "duration": 45
}

点赞和收藏可以由对应互动 API 在服务端记录，不要求前端在每次点赞后再发一个行为请求。

**作用：**为兴趣流提供输入。Feed 暂时只使用简单规则，例如最近互动内容的作者或内容类型；不引入机器学习或复杂标签画像。

## 五、Feed 的工作流程

### 1. 发布链路

创建问题、回答或文章
        ↓
写入 MySQL
        ↓
发布 RabbitMQ content.created 事件
        ↓
Feed 消费者读取作者的粉丝关系
        ↓
将内容引用写入粉丝的 Redis Feed

Redis 中保存的是内容引用，例如 answer:101，而不是整篇回答。内容本身以 MySQL 为准。

用户关注作者时，API 更新 user_follows；Feed 处理逻辑可以把该作者近期内容补入用户 Feed。

### 2. 浏览链路

GET /api/v1/feed
        ↓
Feed Service 获取关注、兴趣、探索候选
        ↓
按固定比例简单混排
        ↓
补充内容详情和当前用户互动状态
        ↓
返回统一 Feed

三类候选的初版来源：

| 流 | 候选来源 |
|---|---|
| 关注流 | Redis 中关注作者发布的 Feed 引用；缺失时查 MySQL |
| 兴趣流 | 最近的浏览、点赞、收藏行为所关联的作者或内容类型 |
| 探索流 | 从公共内容中简单随机抽取一批内容 |

混排可以从固定比例开始，例如每 10 条中约 5 条关注内容、3 条兴趣内容、2 条探索内容。某一类候选不够时，用其他类别补足。比例属于服务端策略，之后可以调整。

对于同一条内容同时出现在多个候选池的情况，可以在当前页合并时按 kind + id 去重；这不会维护用户已读历史，也不会阻止它在之后的请求中再次出现。

### 3. Redis、RabbitMQ 与 MySQL 的职责

- MySQL 保存用户、内容、关注关系、互动关系和行为记录，是业务数据来源。
- RabbitMQ 异步传递内容发布、关注变化和用户行为事件。
- Redis 保存 Feed 引用和简单兴趣权重，作为可重新生成的加速数据。
- Feed Service 合并三类候选并生成主页响应。

这样可以在有限的表和接口内跑通注册、发布、互动，以及“关注流 + 兴趣流 + 探索流”的首页 Feed。

================================================

我阅读了 /home/lds/reference/TodoList-2。理解了：这个项目是我们后端重构时采用的目录组织和分层参考，重点是按业务域拆分 Service，并通过各自的单例服务对象调用功能，而不是用一个通用的 service 对象承载所有业务。

## TodoList 的分层方式

routes
  → api Handler
      → 业务 Service 单例
          → DAO / Cache
              → MySQL / Redis

- routes：注册路由，将 URL 绑定到 API Handler。
- api：绑定和校验 HTTP 请求参数，调用对应 Service，返回 HTTP 响应。
- service：按业务域拆分，例如 UserSrv 和 TaskSrv，各自负责相关业务规则。
- repository/db/dao：封装具体数据库查询与写入。
- repository/cache：封装 Redis 客户端和缓存相关操作。
- repository/db/model：定义数据库模型。
- types：放请求和响应使用的数据结构。
- cmd/main.go：负责初始化配置、日志、数据库、Redis，然后构建路由并启动服务。

## Service 单例的使用方式

TodoList 的 service/user.go 和 service/task.go 分别定义业务服务类型和单例获取函数：

var UserSrvIns *UserSrv
var UserSrvOnce sync.Once

func GetUserSrv() *UserSrv {
    UserSrvOnce.Do(func() {
        UserSrvIns = &UserSrv{}
    })
    return UserSrvIns
}

Task Service 有自己独立的 TaskSrv 和 GetTaskSrv()。因此 API Handler 按业务选择服务：

userService := service.GetUserSrv()
resp, err := userService.Register(ctx, req)

或：

taskService := service.GetTaskSrv()
resp, err := taskService.CreateTask(ctx, req)

每次请求的参数通过方法参数传入；单例服务对象不保存某个用户的请求状态。

## 后续映射到 myzhihu

按这个架构，知乎项目可以按业务域定义自己的 Service：

service/
  user.go        UserSrv
  content.go     ContentSrv
  interaction.go InteractionSrv
  feed.go        FeedSrv

对应 API 调用关系可以是：

api.Register       → GetUserSrv().Register(...)
api.CreateAnswer   → GetContentSrv().CreateAnswer(...)
api.LikeContent    → GetInteractionSrv().Like(...)
api.GetFeed        → GetFeedSrv().GetFeed(...)

Feed Service 负责组织关注、兴趣、探索三种候选流并组合结果；具体 MySQL 查询由对应 DAO 完成，Redis 操作放在缓存仓库包中，RabbitMQ 发布和消费放在消息相关包中。Handler 只处理 HTTP 参数和响应。

TodoList 将数据库初始化放在 DAO 包、Redis 初始化放在 Cache 包，再由启动代码调用。结合我们之前确定的初始化思路，myzhihu 可以保留各自独立的初始化函数，再由 cmd/main.go 统一组织启动顺序；Service 按 TodoList 的方式按业务域拆分为单例。

我会把这种按领域拆分 Service 单例、Handler 调用对应领域服务、仓库层封装存储操作的方式作为后续 myzhihu 架构讨论的基础。

========================================
项目结构：
cmd: 程序路口
api: 路由对应api函数实现， 调用service实现逻辑， 调用response进行返回信息
conf: 部件定义结构体，如redis, rabbitmq结构体的定义，对应了配置文件如yaml
core: 利用结构体信息初始化redis, rabitmq等连接
midware: 中间件如cors跨域， jwt的实现, redis等中间件函数的实现
repository: dao对应底层gorm数据库操作， model对应存储表结构体
response: api函数中返回信息部分的实现，优先级靠后
route: 路由定义
serializer: 接受传来的json信息转化为对应的结构体定义处
service:业务实现， 方式如前文所述
util:工具定义处， 如log, 状态码定义， 参数解析工具等
settings: 配置文件，如后端配置， docker配置等

=======================================================s
三路feed设计

明白。我们把重点放在关注流、兴趣流、探索流的合并呈现，不做复杂的探索排序，也不维护“用户看过什么”的历史来避免重复推荐。推荐内容重复可以接受。

我会把“允许不同页面或不同请求中重复推荐”和“同一页里同一内容重复出现”区分开：前者不做处理；后者可以在合并时按 kind + id 做一次简单去重，避免同一条内容同时来自关注流和兴趣流时占掉两个位置。这不是已读过滤。如果你希望完全保留重复项，这一步也可以省略。

## 精简后的主页方案

主页仍只需要一个主要接口：

GET /api/v1/feed?page=1&limit=20

Feed Service 分别取三类候选，再按简单比例轮流合并。例如：

关注流 5 条 → 兴趣流 3 条 → 探索流 2 条 → 重复此顺序

这是实现 Feed 混排效果的简单起点，不需要复杂打分算法。某个流候选不足时，就从另外两类补足。

返回结果可以标明来源，方便前端展示或调试：

{
  "items": [
    {
      "kind": "answer",
      "id": 101,
      "source": "following",
      "title": "如何学习 Go？"
    },
    {
      "kind": "article",
      "id": 42,
      "source": "interest",
      "title": "数据库索引入门"
    },
    {
      "kind": "question",
      "id": 20,
      "source": "explore",
      "title": "如何理解分布式系统？"
    }
  ],
  "page": 1,
  "limit": 20
}

## 三种流的简单来源

- 关注流：查关注作者发布的内容。发布事件可以由 RabbitMQ 消费者处理，并将内容引用写入关注者的 Redis Feed。
- 兴趣流：根据用户近期点赞、收藏或阅读行为，简单找出相关作者或内容类型，再取这些作者或类型的近期内容。初版不需要复杂推荐模型。
- 探索流：从公共内容中随机取一批内容，作为补充候选。项目数据量较小时，可以先用简单随机查询；暂不做复杂的探索排序和已读过滤。

于是请求链路可以保持清楚：

GET /feed
  → 分别获取关注、兴趣、探索候选
  → 按固定比例合并
  → 返回统一 Feed

RabbitMQ 主要支撑异步部分：内容发布后更新关注流，用户行为发生后记录兴趣信号。Redis 保存关注流等可重建的 Feed 数据；MySQL 保存内容和关注、互动等事实数据。主页 API 负责取候选并合并，前端只消费统一的 items。

接下来设计模型时，我们只需围绕这三类候选需要什么数据来确定表，不必先加入已读历史、推荐快照或复杂的随机流状态。


feed需要解决的问题：
1. 通过游标解决深分页；
2. 通过游标、ZSET 幂等和当前页去重解决重复内容；
3. 通过常驻 Feed 投影和 singleflight 解决缓存击穿。

其他大 Key、推拉模式、大 V fanout 优化暂时只保留为后续扩展说明。

## 一、初版整体结构

用户发布内容
    ↓
ContentSrv 写 MySQL
    ↓
发布 content.created
    ↓
Feed Consumer 查询作者粉丝
    ↓
写入 Redis ZSET
    ↓
feed:user:{userID}


用户请求 GET /feed
    ↓
解析 cursor 和 limit
    ↓
从 Redis ZSET 读取内容 ID
    ├─ 已初始化：直接使用
    └─ 未初始化：
         singleflight 合并并发重建
         → 查询 MySQL
         → 重建 Redis
    ↓
批量查询 MySQL 内容详情
    ↓
过滤删除内容、去重、保持 Redis 顺序
    ↓
返回 items 和 next_cursor

Redis 初版只需要两个 Key：

feed:user:{userID}         ZSET，保存内容 ID
feed:ready:{userID}        STRING，表示该用户 Feed 已构建

## 二、Redis ZSET 设计

建议初版直接使用 content_id 作为排序分数：

Key:    feed:user:{userID}
Member: contentID
Score:  contentID

例如：

feed:user:1

member    score
105       105
101       101
88        88

由于当前项目的 contents.id 是 MySQL 自增主键，新内容通常拥有更大的 ID，因此可以简单地表示创建顺序。

这样有几个优点：

- Score 唯一，不需要处理相同毫秒内创建多条内容；
- content_id 可以直接作为分页游标；
- SQL 和 Redis 使用相同游标；
- 面试时容易解释。

写入：

ZADD feed:user:1 105 105

读取第一页：

ZREVRANGEBYSCORE feed:user:1 +inf -inf LIMIT 0 20

读取 content_id=88 之后的下一页：

ZREVRANGEBYSCORE feed:user:1 (88 -inf LIMIT 0 20

(88 表示严格小于 88，不会再次返回 88。

如果以后使用雪花 ID、数据回填或更复杂的推荐分数，再改为 (score, content_id) 复合游标。初版没有必要提前加入。

———

# 问题一：深分页

## 1. 具体场景

Alice 已经连续查看了 500 页，每页 20 条：

GET /api/v1/feed?page=500&limit=20

如果使用 MySQL OFFSET：

SELECT c.*
FROM contents c
JOIN user_follows f
  ON f.following_id = c.author_id
WHERE f.follower_id = 1
ORDER BY c.id DESC
LIMIT 20 OFFSET 9980;

MySQL 为了返回最后 20 条，需要先扫描和跳过前面 9980 条。

可能产生：

深分页 SQL 变慢
→ HTTP 请求超时
→ 前端重试
→ MySQL连接池被占用
→ 正常首页请求也变慢

即使 Redis 使用：

ZREVRANGE feed:user:1 9980 9999

也仍然属于按位置分页，随着 Feed 增删，还会引起第二个问题。

## 2. 初版解决方式：ID 游标

请求改成：

GET /api/v1/feed?limit=20

返回：

{
  "items": [
    {"id": 105},
    {"id": 101},
    {"id": 88}
  ],
  "next_cursor": "88",
  "has_more": true
}

下一页：

GET /api/v1/feed?limit=20&cursor=88

Redis 查询：

ZREVRANGEBYSCORE feed:user:1 (88 -inf LIMIT 0 20

如果 Redis 未命中，MySQL 降级查询也使用同一游标：

SELECT c.id
FROM contents c
JOIN user_follows f
  ON f.following_id = c.author_id
WHERE f.follower_id = ?
  AND c.id < ?
  AND c.kind IN ('question', 'answer', 'article')
  AND c.deleted_at IS NULL
ORDER BY c.id DESC
LIMIT ?;

数据库可以从主键索引位置继续向后读取，不需要扫描并丢弃前面几千条。

## 3. 应放置的位置

### api/feed/feed.go

负责解析：

cursor
limit
当前登录用户 ID

示例：

func GetFeed(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	limit := parseLimit(c.Query("limit"), 20, 50)
	cursor, err := parseCursor(c.Query("cursor"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid cursor",
		})
		return
	}

	page, err := service.GetFeedSrv().GetFeed(
		c.Request.Context(),
		userID,
		cursor,
		limit,
	)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, page)
}

### serializer/feed.go

定义请求内部参数和响应：

type FeedCursor struct {
	BeforeID uint
}

type FeedItem struct {
	Content ContentResponse `json:"content"`
	Source  string          `json:"source"`
}

type FeedPage struct {
	Items      []FeedItem `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
	HasMore    bool       `json:"has_more"`
}

### repository/cache/feed.go

封装 ZSET 游标读取：

func (r *FeedCache) ListBefore(
	ctx context.Context,
	userID uint,
	beforeID uint,
	limit int,
) ([]uint, error) {
	max := "+inf"
	if beforeID > 0 {
		max = "(" + strconv.FormatUint(uint64(beforeID), 10)
	}

	values, err := r.client.ZRevRangeByScore(
		ctx,
		feedKey(userID),
		&redis.ZRangeBy{
			Max:    max,
			Min:    "-inf",
			Offset: 0,
			Count:  int64(limit),
		},
	).Result()
	if err != nil {
		return nil, err
	}

	return parseContentIDs(values)
}

### repository/dao/content.go

增加 MySQL 游标降级查询：

func (d *ContentDao) ListRecentFromFollowingBefore(
	userID uint,
	beforeID uint,
	limit int,
) ([]model.Content, error) {
	query := d.db.
		Model(&model.Content{}).
		Joins(`
			JOIN user_follows
			ON user_follows.following_id = contents.author_id
		`).
		Where("user_follows.follower_id = ?", userID).
		Where(
			"contents.kind IN ?",
			[]model.ContentKind{
				model.ContentKindQuestion,
				model.ContentKindAnswer,
				model.ContentKindArticle,
			},
		)

	if beforeID > 0 {
		query = query.Where("contents.id < ?", beforeID)
	}

	var items []model.Content
	err := query.
		Order("contents.id DESC").
		Limit(limit).
		Find(&items).
		Error

	return items, err
}

———

# 问题二：重复内容和漏内容

这个问题在当前系统中有三种来源。

## 场景一：OFFSET 分页产生重复

第一页原来是：

[105, 104, 103, ..., 87, 86]

Alice 阅读期间，Bob 发布了内容 106：

[106, 105, 104, ..., 87, 86, 85]

如果第二页继续使用：

OFFSET 20

第二页第一条可能再次返回 86。

### 解决方式

第一页最后一条是 86，下一页明确查询：

content_id < 86

新插入的 106 不会改变这个边界：

ZREVRANGEBYSCORE feed:user:1 (86 -inf LIMIT 0 20

因此不会因为列表整体移动而重复返回 86。

## 场景二：RabbitMQ 重复投递

RabbitMQ 可能把同一条消息投递两次：

content.created: content_id=105
content.created: content_id=105

Consumer 两次执行：

ZADD feed:user:1 105 105

ZSET 的 member 是唯一的，因此最终只有一条 105。

这就是 Consumer 的基本幂等性。

## 场景三：一条内容来自多种候选源

例如内容 105 同时来自：

关注流：105
近期公共内容：105
兴趣候选：105

混排前需要在 Go 中按 content_id 去重：

func deduplicate(candidates []Candidate) []Candidate {
	seen := make(map[uint]struct{}, len(candidates))
	result := make([]Candidate, 0, len(candidates))

	for _, candidate := range candidates {
		if _, exists := seen[candidate.ContentID]; exists {
			continue
		}

		seen[candidate.ContentID] = struct{}{}
		result = append(result, candidate)
	}

	return result
}

初版即使暂时没有完整兴趣流，也建议保留这个简单去重。

## 已删除内容的处理

Redis 中可能仍然存在：

content_id=105

但 MySQL 中该内容已经软删除。

批量补充详情时，GORM 默认不会返回软删除内容：

SELECT *
FROM contents
WHERE id IN (...)
  AND deleted_at IS NULL;

FeedSrv 应当忽略没有查询到的 ID。

可选地顺手清理当前用户的脏引用：

ZREM feed:user:1 105

为了防止删除内容导致一页不足，可以适当超取：

用户需要 20 条
Redis 先读取 30～40 个 ID
过滤删除和重复内容后取前 20 条

这只是简单超取，不需要复杂循环。

## 应放置的位置

### feed/consumer.go

使用 ZSET 唯一 member 实现消息幂等：

func (c *Consumer) addContentToUserFeed(
	ctx context.Context,
	userID uint,
	contentID uint,
) error {
	return c.redis.ZAdd(
		ctx,
		feedKey(userID),
		redis.Z{
			Score:  float64(contentID),
			Member: contentID,
		},
	).Err()
}

### service/feed.go

负责：

- 合并候选；
- 按 content_id 去重；
- 批量补全详情；
- 过滤已删除内容；
- 按 Redis 返回顺序重新排列；
- 生成下一页 cursor。

示例结构：

func (s *FeedSrv) GetFeed(
	ctx context.Context,
	userID uint,
	cursor serializer.FeedCursor,
	limit int,
) (*serializer.FeedPage, error) {
	candidateLimit := limit * 2

	ids, err := s.getFeedIDs(
		ctx,
		userID,
		cursor.BeforeID,
		candidateLimit,
	)
	if err != nil {
		return nil, err
	}

	ids = deduplicateIDs(ids)

	contents, err := s.contentDAO.FindByIDs(ids)
	if err != nil {
		return nil, err
	}

	items := reorderAndFilter(ids, contents)
	if len(items) > limit {
		items = items[:limit]
	}

	var nextCursor string
	if len(items) > 0 {
		lastID := items[len(items)-1].ID
		nextCursor = strconv.FormatUint(uint64(lastID), 10)
	}

	return &serializer.FeedPage{
		Items:      buildFeedItems(items),
		NextCursor: nextCursor,
		HasMore:    len(items) == limit,
	}, nil
}

### repository/cache/feed.go

增加删除脏引用的方法：

func (r *FeedCache) Remove(
	ctx context.Context,
	userID uint,
	contentIDs ...uint,
) error {
	if len(contentIDs) == 0 {
		return nil
	}

	members := make([]any, 0, len(contentIDs))
	for _, id := range contentIDs {
		members = append(members, id)
	}

	return r.client.ZRem(
		ctx,
		feedKey(userID),
		members...,
	).Err()
}

———

# 问题三：缓存击穿

参考文档中描述的是热点 Key 失效后大量请求同时回源，更准确的名称是缓存击穿。

## 1. 具体场景

Alice 打开首页时，前端可能同时产生：

首页 Feed 请求
页面预加载请求
网络重试请求
多端同时登录请求

此时：

feed:user:1

因为 Redis 重启、Key 被删除或缓存尚未构建而不存在。

如果每个请求都执行：

Redis Miss
→ 查询 MySQL
→ 重建 Redis

那么 50 个并发请求会执行 50 次相同 SQL。

对于全局探索流更明显：

feed:explore 失效
→ 2000 个用户同时打开首页
→ 2000 个请求同时查询 MySQL

## 2. 第一层解决：Feed 投影不使用短 TTL

feed:user:{userID} 不是普通页面缓存，而是由 RabbitMQ Consumer 增量维护的 Feed 投影。

因此初版不应该设置 5 分钟之类的短 TTL：

不推荐：
EXPIRE feed:user:1 300

否则每隔五分钟都可能重新构建。

初版可以：

- ZSET 常驻；
- Consumer 持续增量更新；
- Redis 丢失时才从 MySQL 重建；
- 使用 feed:ready:{userID} 标记空 Feed 已经构建。

对于没有任何内容的用户，Redis 不会保留空 ZSET，因此需要：

SET feed:ready:1 1

这样可以区分：

ZSET 不存在，ready 存在
→ 用户确实没有关注内容

ZSET 不存在，ready 也不存在
→ Feed 尚未构建或 Redis 数据丢失

## 3. 第二层解决：singleflight 合并并发重建

项目已经决定使用单进程、Consumer goroutine，因此第一版不需要分布式锁。

使用 Go 的：

golang.org/x/sync/singleflight

为同一个用户只执行一次重建。

type FeedSrv struct {
	rebuildGroup singleflight.Group
}

读取逻辑：

func (s *FeedSrv) getFeedIDs(
	ctx context.Context,
	userID uint,
	beforeID uint,
	limit int,
) ([]uint, error) {
	ids, initialized, err :=
		s.feedCache.ListBefore(
			ctx,
			userID,
			beforeID,
			limit,
		)
	if err != nil {
		return nil, err
	}

	if initialized {
		return ids, nil
	}

	key := fmt.Sprintf("feed-rebuild:%d", userID)

	value, err, _ := s.rebuildGroup.Do(
		key,
		func() (any, error) {
			// 双重检查：等待期间其他请求可能已经完成重建。
			ids, initialized, err :=
				s.feedCache.ListBefore(
					ctx,
					userID,
					beforeID,
					limit,
				)
			if err != nil {
				return nil, err
			}
			if initialized {
				return ids, nil
			}

			recent, err :=
				s.contentDAO.ListRecentFromFollowingBefore(
					userID,
					0,
					500,
				)
			if err != nil {
				return nil, err
			}

			if err := s.feedCache.Replace(
				ctx,
				userID,
				recent,
			); err != nil {
				// Redis 写入失败时，仍可以返回 MySQL 结果。
				return applyCursor(recent, beforeID, limit), nil
			}

			return applyCursor(recent, beforeID, limit), nil
		},
	)
	if err != nil {
		return nil, err
	}

	return value.([]uint), nil
}

如果 50 个请求同时触发：

请求 A：实际查询 MySQL并重建 Redis
请求 B～50：等待 A 的结果

MySQL 只执行一次重建查询。

## 4. 应放置的位置

### service/feed.go

singleflight.Group 放在 FeedSrv 中，因为它控制的是“Feed 重建”业务流程：

type FeedSrv struct {
	rebuildGroup singleflight.Group
}

不建议把它放进 API Handler，因为 Handler 应该只解析 HTTP 参数。

### repository/cache/feed.go

负责判断 Feed 是否已初始化：

func feedKey(userID uint) string {
	return fmt.Sprintf("feed:user:%d", userID)
}

func readyKey(userID uint) string {
	return fmt.Sprintf("feed:ready:%d", userID)
}

func (r *FeedCache) IsInitialized(
	ctx context.Context,
	userID uint,
) (bool, error) {
	pipe := r.client.Pipeline()

	feedExists := pipe.Exists(ctx, feedKey(userID))
	readyExists := pipe.Exists(ctx, readyKey(userID))

	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}

	return feedExists.Val() > 0 || readyExists.Val() > 0, nil
}

重建后：

func (r *FeedCache) Replace(
	ctx context.Context,
	userID uint,
	contents []model.Content,
) error {
	pipe := r.client.TxPipeline()

	key := feedKey(userID)
	pipe.Del(ctx, key)

	for _, content := range contents {
		pipe.ZAdd(ctx, key, redis.Z{
			Score:  float64(content.ID),
			Member: content.ID,
		})
	}

	// 即使 contents 为空，也标记已经构建。
	pipe.Set(ctx, readyKey(userID), "1", 0)

	_, err := pipe.Exec(ctx)
	return err
}

———

## 三个问题在项目中的最终落点

建议新增和修改如下：

backend/
├── api/feed/
│   └── feed.go
│       # 解析 cursor、limit 和 userID
│
├── serializer/
│   └── feed.go
│       # FeedCursor、FeedItem、FeedPage
│
├── service/
│   └── feed.go
│       # Feed 主流程
│       # singleflight 重建
│       # 候选去重
│       # 详情补全
│       # next_cursor 生成
│
├── repository/
│   ├── cache/
│   │   └── feed.go
│   │       # ZADD
│   │       # ZREVRANGEBYSCORE
│   │       # ZREM
│   │       # ready 标记
│   │       # Replace 重建
│   │
│   └── dao/
│       ├── content.go
│       │   # FindByIDs
│       │   # ListRecentFromFollowingBefore
│       │
│       └── user_follow.go
│           # Consumer 查询粉丝
│
├── feed/
│   └── consumer.go
│       # 解析 content.created
│       # 查询粉丝
│       # ZADD 用户 Feed
│       # ACK/NACK
│
├── route/
│   └── router.go
│       # 已有 GET /api/v1/feed
│
└── cmd/
    └── main.go
        # 启动 Feed Consumer goroutine
        # 启动 Gin Server

## 验收测试

三个问题最好分别对应一个测试，这会让面试展示更有说服力。

### 深分页测试

准备 100 条 Feed
第一页返回 100～81
使用 cursor=81 请求下一页
断言第二页返回 80～61
断言查询不使用 OFFSET

位置：

service/feed_test.go

### 重复内容测试

重复消费两次 content.created(content_id=100)
断言 ZCARD feed:user:1 == 1

以及：

获取第一页后插入 content_id=101
使用第一页 cursor 获取第二页
断言第一页和第二页没有重复 ID

位置：

feed/consumer_test.go
service/feed_test.go

### 缓存击穿测试

清空 feed:user:1 和 feed:ready:1
同时发起 100 次 GetFeed
断言 MySQL重建方法只调用一次
断言 100 个请求都能得到结果

位置：

service/feed_test.go

初版可以这样总结：

> Feed 使用 Redis ZSET 保存每个用户的内容 ID，并以自增 content ID 作为排序分数和分页游标。游标分页避免深 OFFSET 以及内容插入时的跨页漂移；ZSET member 唯一和服务层去重避免重复内容；Feed 投影不设置短 TTL，缓存缺失时通过 singleflight 合并并发回源，防止缓存击穿。MySQL仍保存权威内容，Redis 只保存可重建的 Feed 索引


===================================
jwt:
登录
→ 校验用户名和密码
→ 签发 JWT
→ 前端保存 access_token
→ 请求受保护 API 时携带 Bearer Token
→ JWT 中间件验证
→ 将 user_id 写入 Gin Context
→ Handler 读取 user_id
→ Service 执行业务

> JWT 的配置、签发、响应、Handler 读取身份和 Service 权限判断已经基本完成；请求级 Token 验证中间件、路由挂载和服务器启动仍待实现。

## 一、整体对应关系

| 总结中的描述 | 当前代码位置 | 状态 |
|---|---|---|
| 系统实现 JWT 签发与验证机制 | conf、core、service/user.go | 签发已完成，验证未完成 |
| 用户注册或登录成功 | UserRegister、UserLogin | 已完成 |
| 后端签发 JWT | UserSrv.authResponse | 已完成 |
| JWT 返回前端 | AuthResponse | 已完成 |
| 前端使用 Bearer Token | TokenType: "Bearer" | 后端协议已定义，前端调用待实现 |
| 受保护路由使用 JWT 中间件 | route/router.go | 路由组已有，中间件未挂载 |
| 验证签名、签发者和过期时间 | 未来的 midware/jwt | 未实现 |
| 将用户 ID 写入 Gin Context | 未来的 c.Set("user_id", ...) | 未实现 |
| Handler 从 Context 读取当前用户 | currentUserID | 已完成 |
| Handler 解析 Param、Query、JSON | 各 API Handler | 已完成 |
| Service 完成写入和权限检查 | ContentSrv、InteractionSrv | 大部分已完成 |
| Feed 使用 JWT 用户 ID | api/feed/feed.go | 待实现 |

## 二、“系统首先实现 JWT 签发与验证机制”

这句话实际上包含两个不同阶段：

签发机制：登录成功后生成 JWT
验证机制：后续请求到来时验证 JWT

### 已有：JWT 配置

配置结构位于 conf_jwt.go (my_project/myzhihu2/backend/conf/conf_jwt.go:3)：

type JWT struct {
    Algorithm   string
    Issuer      string
    Secret      string
    ExpireHours int
}

配置文件位于 settings.yaml (my_project/myzhihu2/backend/settings/settings.yaml:40)：

jwt:
  algorithm: "HS256"
  issuer: "myzhihu2"
  secret: "..."
  expire_hours: 24

这些配置分别决定：

Algorithm   JWT 签名算法
Issuer      Token 的签发系统
Secret      签名和验签密钥
ExpireHours Token 有效时间

### 已有：启动时检查配置

cmd/main.go (my_project/myzhihu2/backend/cmd/main.go:11) 读取配置后调用：

global.Conf = core.ReadConf()

if err := core.InitJWT(); err != nil {
    panic(err)
}

InitJWT 位于 init_jwt.go (my_project/myzhihu2/backend/core/init_jwt.go:10)，负责检查：

- 是否为 HS256；
- issuer 是否存在；
- secret 是否足够长；
- 过期时间是否合法。

需要特别区分：

InitJWT()
    = 程序启动时检查 JWT 配置

JWT Middleware
    = 每个请求到来时检查具体 Token

所以 InitJWT() 不是用户请求的 Token 验证机制。

### 等待实现：请求级 JWT 验证

建议新增：

backend/midware/jwt/auth.go

它负责：

解析 Authorization: Bearer <token>
验证 HS256 签名
验证 iss=myzhihu2
验证 exp 尚未过期
提取 user_id

## 三、“用户注册或登录成功后，后端签发 JWT”

这部分已经完整实现。

### 注册 Handler

api/system/user.go (my_project/myzhihu2/backend/api/system/user.go:16)：

func UserRegister(c *gin.Context) {
    var req serializer.UserRegisterRequest

    if err := c.ShouldBindJSON(&req); err != nil {
        return
    }

    result, err := service.GetUserSrv().
        Register(c.Request.Context(), req)

    c.JSON(http.StatusCreated, gin.H{
        "data": result,
    })
}

对应链路：

POST /auth/register
→ 解析注册 JSON
→ UserSrv.Register
→ 创建用户
→ 签发 JWT
→ 返回用户和 Token

### 登录 Handler

api/system/user.go (my_project/myzhihu2/backend/api/system/user.go:37)：

func UserLogin(c *gin.Context) {
    var req serializer.UserLoginRequest

    if err := c.ShouldBindJSON(&req); err != nil {
        return
    }

    result, err := service.GetUserSrv().
        Login(c.Request.Context(), req)

    c.JSON(http.StatusOK, gin.H{
        "data": result,
    })
}

UserSrv.Login 完成：

查询用户
→ bcrypt 校验密码
→ authResponse 签发 Token

### JWT 实际签发位置

位于 service/user.go (my_project/myzhihu2/backend/service/user.go:179)：

claims := jwt.MapClaims{
    "user_id": user.ID,
    "iss":     jwtConf.Issuer,
    "iat":     now.Unix(),
    "exp":     now.Add(
        time.Duration(jwtConf.ExpireHours) * time.Hour,
    ).Unix(),
}

然后使用 HS256 签名：

token := jwt.NewWithClaims(
    jwt.SigningMethodHS256,
    claims,
)

signedToken, err := token.SignedString(
    []byte(jwtConf.Secret),
)

这里对应总结中的：

> 后端签发 JWT。

## 四、“后端把 JWT 返回给前端”

响应结构位于 serializer/user.go (my_project/myzhihu2/backend/serializer/user.go:40)：

type AuthResponse struct {
    User        UserResponse `json:"user"`
    AccessToken string       `json:"access_token"`
    TokenType   string       `json:"token_type"`
}

签发后返回：

return &serializer.AuthResponse{
    User:        buildUserResponse(user),
    AccessToken: signedToken,
    TokenType:   "Bearer",
}, nil

前端收到的结构类似：

{
  "data": {
    "user": {
      "id": 12,
      "username": "alice"
    },
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer"
  }
}

这部分已经完成。

## 五、“前端调用受保护 API 时携带 JWT”

这部分属于前端或 API 调用方职责，当前后端已经定义了协议，但仓库中还没有相关前端实现。

请求格式应为：

Authorization: Bearer eyJhbGciOiJIUzI1NiIs...

例如：

curl -X POST http://127.0.0.1:8080/api/v1/contents \
  -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIs...' \
  -H 'Content-Type: application/json' \
  -d '{
    "kind": "article",
    "title": "JWT 的使用",
    "body": "正文"
  }'

JWT 不应放在请求体中：

{
  "token": "...",
  "user_id": 12
}

也不应由前端另外提交当前用户 ID。当前用户 ID必须来自经过验证的 Token。

## 六、“对应路由的 JWT 中间件先验证 Token”

路由分组已经存在。

route/router.go (my_project/myzhihu2/backend/route/router.go:15) 中，公开路由包括：

v1.POST("/auth/register", systemapi.UserRegister)
v1.POST("/auth/login", systemapi.UserLogin)

v1.GET("/users/:userID", systemapi.GetUser)
v1.GET("/contents/:contentID", systemapi.GetContent)

这些不要求 JWT。

受保护路由从 router.go (my_project/myzhihu2/backend/route/router.go:27) 开始：

auth := v1.Group("")

// auth.Use(jwtmiddleware.Required(...))

其中包括：

auth.GET("/me", systemapi.GetMe)
auth.POST("/contents", systemapi.CreateContent)
auth.PUT("/contents/:contentID", systemapi.UpdateContent)
auth.POST("/contents/:contentID/like", systemapi.LikeContent)
auth.POST("/users/:userID/follow", systemapi.FollowUser)
auth.GET("/feed", feedapi.GetFeed)

### 等待实现的位置

需要新增中间件后，在这里挂载：

auth := v1.Group("")
auth.Use(jwtmiddleware.Required(jwtConfig))

完整结构：

func NewRouter(jwtConfig conf.JWT) *gin.Engine {
    router := gin.Default()
    v1 := router.Group("/api/v1")

    // 公开路由……

    auth := v1.Group("")
    auth.Use(jwtmiddleware.Required(jwtConfig))

    // 受保护路由……

    return router
}

## 七、“验证签名、签发者和过期时间”

这部分尚未实现，应该放在：

backend/midware/jwt/auth.go

逻辑应为：

func Required(config conf.JWT) gin.HandlerFunc {
    return func(c *gin.Context) {
        rawToken, ok := parseBearer(
            c.GetHeader("Authorization"),
        )
        if !ok {
            c.AbortWithStatusJSON(
                http.StatusUnauthorized,
                gin.H{"error": "authentication required"},
            )
            return
        }

        claims := &Claims{}

        token, err := jwt.ParseWithClaims(
            rawToken,
            claims,
            func(token *jwt.Token) (any, error) {
                if token.Method.Alg() != config.Algorithm {
                    return nil, errors.New(
                        "invalid signing algorithm",
                    )
                }

                return []byte(config.Secret), nil
            },
            jwt.WithValidMethods(
                []string{config.Algorithm},
            ),
            jwt.WithIssuer(config.Issuer),
            jwt.WithExpirationRequired(),
        )

        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(
                http.StatusUnauthorized,
                gin.H{"error": "invalid or expired token"},
            )
            return
        }

        // 验证成功后继续。
    }
}

这里对应：

验证签名      Secret + HS256
验证签发者    iss == myzhihu2
验证过期时间  exp > 当前时间

## 八、“验证成功后把用户 ID 写入 Gin Context”

这部分也尚未实现，但现有 Handler 已经在等待它。

中间件验证成功后执行：

c.Set("user_id", claims.UserID)
c.Next()

其中 Claims 可以定义为：

type Claims struct {
    UserID uint `json:"user_id"`
    jwt.RegisteredClaims
}

完整关系：

JWT Payload:
{
  "user_id": 12
}

中间件解析后：
claims.UserID == 12

写入 Gin Context：
c.Set("user_id", uint(12))

### 现有读取函数

api/system/user.go (my_project/myzhihu2/backend/api/system/user.go:107) 已有：

func currentUserID(c *gin.Context) (uint, bool) {
    value, exists := c.Get("user_id")
    // 转换成 uint
}

所以当前结构是：

等待实现的中间件：
c.Set("user_id", claims.UserID)

已经实现的 Handler：
currentUserID(c)

两边使用相同的 "user_id" Key，即可连接起来。

## 九、“Handler 再解析路径参数、查询参数或 JSON”

这部分已经大量实现。

### 示例一：创建内容

api/system/content.go (my_project/myzhihu2/backend/api/system/content.go:13)：

func CreateContent(c *gin.Context) {
    // 当前用户 ID：来自 JWT Context
    userID, ok := currentUserID(c)

    // 业务参数：来自 JSON
    var req serializer.CreateContentRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        return
    }

    result, err := service.GetContentSrv().
        CreateContent(
            c.Request.Context(),
            userID,
            req,
        )
}

对应：

当前用户 ID：JWT
kind/title/body：JSON

### 示例二：创建回答

api/system/content.go (my_project/myzhihu2/backend/api/system/content.go:34)：

userID, ok := currentUserID(c)
questionID, ok := routeID(c, "questionID")

var req serializer.CreateAnswerRequest
c.ShouldBindJSON(&req)

service.GetContentSrv().CreateAnswer(
    ctx,
    userID,
    questionID,
    req,
)

对应：

userID：JWT，表示谁回答
questionID：URL Param，表示回答哪个问题
body：JSON，表示回答内容

### 示例三：关注用户

api/system/userfollow.go (my_project/myzhihu2/backend/api/system/userfollow.go:10)：

followerID, ok := currentUserID(c)
followingID, ok := routeID(c, "userID")

service.GetInteractionSrv().FollowUser(
    c.Request.Context(),
    followerID,
    followingID,
)

对应：

followerID：JWT，表示谁发起关注
followingID：Param，表示关注谁

### 示例四：点赞内容

api/system/reaction.go (my_project/myzhihu2/backend/api/system/reaction.go:10)：

userID, ok := currentUserID(c)
contentID, ok := routeID(c, "contentID")

service.GetInteractionSrv().Like(
    c.Request.Context(),
    userID,
    contentID,
)

对应：

userID：JWT
contentID：URL Param

### 示例五：行为上报

api/system/behavior.go (my_project/myzhihu2/backend/api/system/behavior.go:11)：

userID, ok := currentUserID(c)

var req serializer.RecordBehaviorRequest
c.ShouldBindJSON(&req)

service.GetInteractionSrv().RecordBehavior(
    c.Request.Context(),
    userID,
    req,
)

对应：

userID：JWT
content_id/kind/duration：JSON

## 十、“将当前用户 ID 和业务目标 ID 传给 Service”

这部分已经实现。

最典型的是修改内容：

PUT /api/v1/contents/101
Authorization: Bearer <Alice 的 Token>

Handler 得到：

actorID   = 12   来自 JWT
contentID = 101  来自路径参数

然后调用：

UpdateContent(ctx, actorID, contentID, req)

对应 service/content.go (my_project/myzhihu2/backend/service/content.go:112)。

Service 查询内容后进行所有权判断：

if content.AuthorID != actorID {
    return nil, ErrForbidden
}

这对应总结中的：

> Service 完成权限判断。

删除内容也进行了相同判断，service/content.go (my_project/myzhihu2/backend/service/content.go:160)：

if content.AuthorID != actorID {
    return ErrForbidden
}

因此：

JWT 认证：
证明请求者是用户 12

Service 授权：
检查用户 12 是否有权修改内容 101

## 十一、以“修改内容”为例串联完整过程

请求：

PUT /api/v1/contents/101
Authorization: Bearer <token>

{
  "title": "修改后的标题"
}

完整调用关系应该是：

1. Gin 匹配 PUT /contents/:contentID
   route/router.go

2. JWT 中间件执行
   midware/jwt/auth.go（待实现）

3. 读取 Authorization
   Bearer <token>

4. 验证签名、issuer、exp
   midware/jwt/auth.go（待实现）

5. 从 Token 取得 user_id=12
   midware/jwt/auth.go（待实现）

6. 写入 Gin Context
   c.Set("user_id", uint(12))（待实现）

7. 进入 UpdateContent Handler
   api/system/content.go

8. currentUserID(c)
   得到 actorID=12（已实现）

9. routeID(c, "contentID")
   得到 contentID=101（已实现）

10. ShouldBindJSON
    得到 title（已实现）

11. ContentSrv.UpdateContent
    接收 actorID=12、contentID=101、req（已实现）

12. 查询 MySQL
    得到 contents.author_id（已实现）

13. 检查 author_id == actorID
    不相等返回 403（已实现）

14. 更新内容并返回
    （已实现）

缺失的是第 2～6 步。

## 十二、仍待实现的内容及位置

### 1. Claims 和 Token 验证

新增：

backend/midware/jwt/auth.go

包括：

type Claims struct
func Required(config conf.JWT) gin.HandlerFunc
func parseBearer(header string) (string, bool)

### 2. 写入和读取 Gin Context

建议新增：

backend/midware/jwt/context.go

内容：

const ContextUserIDKey = "user_id"

func UserID(c *gin.Context) (uint, bool) {
    value, exists := c.Get(ContextUserIDKey)
    id, ok := value.(uint)
    return id, exists && ok && id > 0
}

中间件写：

c.Set(ContextUserIDKey, claims.UserID)

Handler 读：

userID, ok := jwtmiddleware.UserID(c)

这样可以替代当前 api/system/user.go 中的私有 currentUserID。

这样做还有一个必要原因：Feed Handler 位于另一个 Go 包：

api/feed

它不能调用 api/system 包内未导出的 currentUserID。

### 3. 路由挂载

修改：

backend/route/router.go

将：

// auth.Use(jwtmiddleware.Required(...))

改成：

auth.Use(jwtmiddleware.Required(jwtConfig))

### 4. Router 接收配置

建议把：

func NewRouter() *gin.Engine

改为：

func NewRouter(jwtConfig conf.JWT) *gin.Engine

由 main 调用：

router := route.NewRouter(global.Conf.JWT)

### 5. 启动 HTTP Server

当前 cmd/main.go (my_project/myzhihu2/backend/cmd/main.go:21) 中：

//route.Run()

仍然被注释，并且没有看到当前代码真正运行 router.Run()。

因此还需要在启动入口中：

router := route.NewRouter(global.Conf.JWT)

addr := fmt.Sprintf(
    "%s:%d",
    global.Conf.System.IP,
    global.Conf.System.Port,
)

if err := router.Run(addr); err != nil {
    panic(err)
}

否则即使 JWT 中间件已经实现，也没有 HTTP Server 对外提供这些路由。

### 6. Feed Handler 使用 JWT 用户 ID

当前 api/feed/feed.go (my_project/myzhihu2/backend/api/feed/feed.go:6) 仍是 TODO。

未来应该：

func GetFeed(c *gin.Context) {
    userID, ok := jwtmiddleware.UserID(c)
    if !ok {
        c.JSON(http.StatusUnauthorized, ...)
        return
    }

    cursor := c.Query("cursor")
    limit := parseLimit(c.Query("limit"))

    result, err := service.GetFeedSrv().
        GetFeed(
            c.Request.Context(),
            userID,
            cursor,
            limit,
        )

    // 返回结果
}

这里对应：

JWT user_id
→ 决定读取 feed:user:{userID}

### 7. JWT 测试

建议新增：

backend/midware/jwt/auth_test.go

至少测试：

- 缺少 Authorization；
- Header 不是 Bearer；
- Token 签名被篡改；
- Token 已过期；
- issuer 错误；
- 签名算法错误；
- 合法 Token；
- 合法 Token 能把 user_id 写入 Context。

## 十三、最终状态总结

JWT 配置                   已完成
启动时检查 JWT 配置        已完成
注册/登录                  已完成
密码校验                   已完成
JWT 签发                   已完成
JWT 返回客户端             已完成
Bearer 协议定义            已完成
公开/受保护路由分组        已完成

Bearer Header 解析         待实现
JWT 请求级验证             待实现
user_id 写入 Context       待实现
路由挂载 JWT 中间件        待实现
跨 API 包身份读取工具      待实现
Feed 使用 JWT 用户身份     待实现
HTTP Server 启动           待完成
JWT 中间件测试             待实现

所以那段总结既覆盖了已经完成的内容，也覆盖了待实现部分。最核心的连接点是：

UserSrv.authResponse
    负责签发 user_id

JWT Required Middleware
    负责验证并 c.Set("user_id")

jwtmiddleware.UserID
    负责从 Context 读取 user_id

各 Handler
    负责解析目标 ID 和业务参数

各 Service
    负责数据库操作和资源权限检查

目前中间这一段——“验证 JWT 并把身份写入 Context”——就是下一步需要补上的主要功能。
