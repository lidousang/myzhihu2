1. 初始化
#1 初始化与启动，new(conf.Config) readconfig-> init other
   本项目具有明确先后关系
#2 引入flags使用命令行参数， 激活automigrate

========================================================

2. 定义的表关系

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

=====================================================
3. feed机制，深入rabbitmq架构
双链路，
发布者：发布文章->写入sql->通过链路发布消息->触发redis更新  生产消费
浏览者：浏览->互动->链路发布消息->更新
.............

=====================================================
4. JWT, 机制
   软删除？
   自动降级？
=====================================================
5. 