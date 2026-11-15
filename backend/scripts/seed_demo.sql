-- myzhihu2 demo data for MySQL 8.x
--
-- Creates:
--   50 users: user1 ... user50
--   a follow graph (each user follows the next five users; users 2-50 follow user1)
--   50 articles, 20 questions, 30 answers, and 20 comments
--   deterministic likes, collections, views, and reading-duration events
--
-- Every demo account uses this password: DemoPass123!
-- The stored value below is a bcrypt hash accepted by UserSrv.Login.
--
-- This script is idempotent for the fixed IDs it owns. Run it against a demo
-- database, not a production database containing unrelated rows with IDs 1-50
-- or content IDs 1001-4050.

USE myzhihu;
SET NAMES utf8mb4;

DROP PROCEDURE IF EXISTS seed_myzhihu2_demo;

DELIMITER $$

CREATE PROCEDURE seed_myzhihu2_demo()
BEGIN
    DECLARE i INT DEFAULT 1;
    DECLARE j INT DEFAULT 1;
    DECLARE target_user INT DEFAULT 0;
    DECLARE target_content INT DEFAULT 0;
    DECLARE parent_question INT DEFAULT 0;
    DECLARE created_time DATETIME;

    -- 1. Users. All accounts can log in with DemoPass123!.
    SET i = 1;
    WHILE i <= 50 DO
        SET created_time = TIMESTAMPADD(DAY, i - 60, NOW());

        INSERT INTO users (
            id,
            username,
            password_hash,
            display_name,
            avatar,
            bio,
            created_at,
            updated_at
        ) VALUES (
            i,
            CONCAT('user', i),
            '$2a$10$HIKJoE4bXKvV8UajrEu0iutKoHMp.aH7yt5yrFxjWoH1C9zvEe1g2',
            CONCAT('演示用户 ', i),
            CONCAT('https://api.dicebear.com/9.x/initials/svg?seed=user', i),
            CONCAT('这是 user', i, ' 的演示账号，用于测试关注、内容和 Feed。'),
            created_time,
            created_time
        )
        ON DUPLICATE KEY UPDATE
            password_hash = VALUES(password_hash),
            display_name = VALUES(display_name),
            avatar = VALUES(avatar),
            bio = VALUES(bio),
            updated_at = VALUES(updated_at);

        SET i = i + 1;
    END WHILE;

    -- 2. Follow graph.
    -- user1 follows user2-user6; every other user follows the next five users.
    -- The ring makes every account have both followers and followings.
    SET i = 1;
    WHILE i <= 50 DO
        SET j = 1;
        WHILE j <= 5 DO
            SET target_user = MOD(i + j - 1, 50) + 1;

            INSERT INTO user_follows (follower_id, following_id, created_at)
            VALUES (i, target_user, TIMESTAMPADD(DAY, j - 10, NOW()))
            ON DUPLICATE KEY UPDATE created_at = VALUES(created_at);

            SET j = j + 1;
        END WHILE;

        -- Make user1 a moderately popular author for fanout demonstrations.
        IF i <> 1 THEN
            INSERT INTO user_follows (follower_id, following_id, created_at)
            VALUES (i, 1, TIMESTAMPADD(DAY, -3, NOW()))
            ON DUPLICATE KEY UPDATE created_at = VALUES(created_at);
        END IF;

        SET i = i + 1;
    END WHILE;

    -- Give user1 enough followed authors for several frontend Feed pages.
    SET target_user = 2;
    WHILE target_user <= 20 DO
        INSERT INTO user_follows (follower_id, following_id, created_at)
        VALUES (1, target_user, TIMESTAMPADD(DAY, -2, NOW()))
        ON DUPLICATE KEY UPDATE created_at = VALUES(created_at);

        SET target_user = target_user + 1;
    END WHILE;

    -- 3. One article per user. IDs are 1001-1050.
    SET i = 1;
    WHILE i <= 50 DO
        SET created_time = TIMESTAMPADD(HOUR, i - 121, NOW());

        INSERT INTO contents (
            id,
            author_id,
            kind,
            parent_id,
            title,
            body,
            created_at,
            updated_at,
            deleted_at
        ) VALUES (
            1000 + i,
            i,
            'article',
            NULL,
            CONCAT('user', i, ' 的 Feed 系统实践 ', i),
            CONCAT(
                '这是 user', i,
                ' 发布的演示文章。正文用于展示首页滚动、作者信息、点赞和收藏状态。'
            ),
            created_time,
            created_time,
            NULL
        )
        ON DUPLICATE KEY UPDATE
            author_id = VALUES(author_id),
            kind = VALUES(kind),
            parent_id = VALUES(parent_id),
            title = VALUES(title),
            body = VALUES(body),
            created_at = VALUES(created_at),
            updated_at = VALUES(updated_at),
            deleted_at = NULL;

        SET i = i + 1;
    END WHILE;

    -- 4. Twenty questions by user1-user20. IDs are 2001-2020.
    SET i = 1;
    WHILE i <= 20 DO
        SET created_time = TIMESTAMPADD(HOUR, i - 71, NOW());

        INSERT INTO contents (
            id,
            author_id,
            kind,
            parent_id,
            title,
            body,
            created_at,
            updated_at,
            deleted_at
        ) VALUES (
            2000 + i,
            i,
            'question',
            NULL,
            CONCAT('如何理解 Feed 推荐系统中的问题 ', i, '？'),
            CONCAT('这是问题 ', i, ' 的补充描述，欢迎演示用户回答。'),
            created_time,
            created_time,
            NULL
        )
        ON DUPLICATE KEY UPDATE
            author_id = VALUES(author_id),
            kind = VALUES(kind),
            parent_id = VALUES(parent_id),
            title = VALUES(title),
            body = VALUES(body),
            created_at = VALUES(created_at),
            updated_at = VALUES(updated_at),
            deleted_at = NULL;

        SET i = i + 1;
    END WHILE;

    -- 5. Thirty answers by user21-user50, distributed across the 20 questions.
    -- Answer IDs are 3021-3050.
    SET i = 21;
    WHILE i <= 50 DO
        SET parent_question = 2000 + MOD(i - 21, 20) + 1;
        SET created_time = TIMESTAMPADD(HOUR, i - 71, NOW());

        INSERT INTO contents (
            id,
            author_id,
            kind,
            parent_id,
            title,
            body,
            created_at,
            updated_at,
            deleted_at
        ) VALUES (
            3000 + i,
            i,
            'answer',
            parent_question,
            '',
            CONCAT(
                'user', i,
                ' 的回答：可以从 MySQL 权威数据、RabbitMQ 异步事件和 Redis Feed 索引三个层次理解。'
            ),
            created_time,
            created_time,
            NULL
        )
        ON DUPLICATE KEY UPDATE
            author_id = VALUES(author_id),
            kind = VALUES(kind),
            parent_id = VALUES(parent_id),
            title = VALUES(title),
            body = VALUES(body),
            created_at = VALUES(created_at),
            updated_at = VALUES(updated_at),
            deleted_at = NULL;

        SET i = i + 1;
    END WHILE;

    -- 6. Twenty comments, showing that comments share the contents table but
    -- should not become normal Feed candidates. IDs are 4001-4020.
    SET i = 1;
    WHILE i <= 20 DO
        SET target_content = 1000 + MOD(i + 19, 50) + 1;
        SET created_time = TIMESTAMPADD(MINUTE, i - 45, NOW());

        INSERT INTO contents (
            id,
            author_id,
            kind,
            parent_id,
            title,
            body,
            created_at,
            updated_at,
            deleted_at
        ) VALUES (
            4000 + i,
            i,
            'comment',
            target_content,
            '',
            CONCAT('user', i, ' 的演示评论：这个内容很有帮助。'),
            created_time,
            created_time,
            NULL
        )
        ON DUPLICATE KEY UPDATE
            author_id = VALUES(author_id),
            kind = VALUES(kind),
            parent_id = VALUES(parent_id),
            title = VALUES(title),
            body = VALUES(body),
            created_at = VALUES(created_at),
            updated_at = VALUES(updated_at),
            deleted_at = NULL;

        SET i = i + 1;
    END WHILE;

    -- 7. Current reaction state: every user likes three articles and collects
    -- one different article. Composite primary keys make this idempotent.
    SET i = 1;
    WHILE i <= 50 DO
        SET j = 1;
        WHILE j <= 3 DO
            SET target_content = 1000 + MOD(i + j * 7 - 1, 50) + 1;

            INSERT INTO reactions (user_id, content_id, kind, created_at)
            VALUES (
                i,
                target_content,
                'like',
                TIMESTAMPADD(MINUTE, -(i + j), NOW())
            )
            ON DUPLICATE KEY UPDATE created_at = VALUES(created_at);

            SET j = j + 1;
        END WHILE;

        SET target_content = 1000 + MOD(i + 27 - 1, 50) + 1;
        INSERT INTO reactions (user_id, content_id, kind, created_at)
        VALUES (
            i,
            target_content,
            'collect',
            TIMESTAMPADD(MINUTE, -(i + 10), NOW())
        )
        ON DUPLICATE KEY UPDATE created_at = VALUES(created_at);

        SET i = i + 1;
    END WHILE;

    -- 8. Historical behavior. Fixed IDs make rerunning the script idempotent.
    SET i = 1;
    WHILE i <= 50 DO
        SET target_content = 1000 + MOD(i + 10 - 1, 50) + 1;

        INSERT INTO behavior_events (
            id,
            user_id,
            content_id,
            kind,
            duration,
            created_at
        ) VALUES (
            10000 + i * 10 + 1,
            i,
            target_content,
            'view',
            0,
            TIMESTAMPADD(MINUTE, -(i * 2), NOW())
        )
        ON DUPLICATE KEY UPDATE
            user_id = VALUES(user_id),
            content_id = VALUES(content_id),
            kind = VALUES(kind),
            duration = VALUES(duration),
            created_at = VALUES(created_at);

        INSERT INTO behavior_events (
            id,
            user_id,
            content_id,
            kind,
            duration,
            created_at
        ) VALUES (
            10000 + i * 10 + 2,
            i,
            target_content,
            'read_time',
            15 + MOD(i * 7, 90),
            TIMESTAMPADD(MINUTE, -(i * 2 - 1), NOW())
        )
        ON DUPLICATE KEY UPDATE
            user_id = VALUES(user_id),
            content_id = VALUES(content_id),
            kind = VALUES(kind),
            duration = VALUES(duration),
            created_at = VALUES(created_at);

        SET i = i + 1;
    END WHILE;
END$$

DELIMITER ;

START TRANSACTION;
CALL seed_myzhihu2_demo();
COMMIT;

DROP PROCEDURE seed_myzhihu2_demo;

-- Verification summary for the fixed demo ID ranges. Expected counts are:
-- users=50, user_follows=308, contents=120, reactions=200,
-- behavior_events=100.
SELECT 'users' AS table_name, COUNT(*) AS row_count
FROM users WHERE id BETWEEN 1 AND 50
UNION ALL
SELECT 'user_follows', COUNT(*) FROM user_follows
WHERE follower_id BETWEEN 1 AND 50 AND following_id BETWEEN 1 AND 50
UNION ALL
SELECT 'contents', COUNT(*) FROM contents
WHERE id BETWEEN 1001 AND 1050
   OR id BETWEEN 2001 AND 2020
   OR id BETWEEN 3021 AND 3050
   OR id BETWEEN 4001 AND 4020
UNION ALL
SELECT 'reactions', COUNT(*) FROM reactions
WHERE user_id BETWEEN 1 AND 50 AND content_id BETWEEN 1001 AND 1050
UNION ALL
SELECT 'behavior_events', COUNT(*) FROM behavior_events
WHERE id BETWEEN 10011 AND 10502 AND MOD(id, 10) IN (1, 2);

-- Useful checks for the frontend demo.
SELECT id, username, display_name FROM users WHERE id BETWEEN 1 AND 50 ORDER BY id;

SELECT follower_id, following_id, created_at
FROM user_follows
WHERE follower_id = 1
ORDER BY following_id;

SELECT id, author_id, kind, parent_id, title, created_at
FROM contents
WHERE deleted_at IS NULL
  AND kind IN ('question', 'answer', 'article')
ORDER BY id DESC
LIMIT 30;
