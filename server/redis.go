package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client
var ctx = context.Background()

// InitRedis 初始化 Redis 连接
func InitRedis() error {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       1,
	})

	// 测试连接，和你 db.go 里的 Ping() 一个套路
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("Redis 连接失败，检查Redis是否启动: %v", err)
	}

	fmt.Println("✅  Redis 连接成功")
	return nil
}

func CacheUser(user *User) error {
	// 1. 构造 key
	key := fmt.Sprintf("user:info:%s", user.Username)

	// 2. 写入 Hash
	err := rdb.HSet(ctx, key,
		"id", user.ID,
		"username", user.Username,
		"status", user.Status,
	).Err()
	if err != nil {
		return fmt.Errorf("缓存用户信息失败: %v", err)
	}

	// 3. 设置过期 30 分钟
	rdb.Expire(ctx, key, 30*time.Minute)

	return nil
}

func GetCachedUser(username string) *User {
	// 1. 构造 key
	key := fmt.Sprintf("user:info:%s", username)

	// 2. 读取所有字段
	result, err := rdb.HGetAll(ctx, key).Result()
	if err != nil || len(result) == 0 {
		return nil
	}

	// 3. string -> int64
	id, err := strconv.ParseInt(result["id"], 10, 64)
	if err != nil {
		return nil
	}

	// 4. string -> int
	status, err := strconv.Atoi(result["status"])
	if err != nil {
		return nil
	}

	// 5. 组装返回
	return &User{
		ID:       id,
		Username: result["username"],
		Status:   status,
	}
}

func IncrActivity(username string) {
	rdb.ZIncrBy(ctx, "chat:leaderboard", 1, username)
}

func GetTopN(n int64) ([]redis.Z, error) {
	return rdb.ZRevRangeWithScores(ctx, "chat:leaderboard", 0, n-1).Result()
}

// GetUserRank 获取某用户的排名（从1开始）和分数
func GetUserRank(username string) (int64, float64, error) {
	// ZRevRank 返回的是从0开始的索引，所以要 +1
	rank, err := rdb.ZRevRank(ctx, "chat:leaderboard", username).Result()
	if err != nil {
		return 0, 0, err
	}

	score, err := rdb.ZScore(ctx, "chat:leaderboard", username).Result()
	if err != nil {
		return 0, 0, err
	}
	return rank + 1, score, nil
}

func ProduceMessage(sender, content, msgType, target string) (string, error) {
	values := map[string]interface{}{
		"sender":  sender,
		"content": content,
		"type":    msgType,
	}
	if target != "" {
		values["target"] = target
	}
	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "chat:messages",
		Values: values,
	}).Result()
}

// 超过该空闲时间仍未 ACK 的 Pending 消息，视为崩溃遗留，恢复时认领处理
const pendingRecoverMinIdle = 10 * time.Second

// consumeMessages 消费 Stream 消息：每轮先恢复崩溃遗留的 Pending 消息，再读取新消息。
// 正常消费流程（XReadGroup > + ACK）保持原样，恢复过程不影响它。
func consumeMessages(group, consumer string) {
	rdb.XGroupCreateMkStream(ctx, "chat:messages", group, "0")

	for {
		// 1. 先恢复崩溃遗留的 Pending 消息（空闲超时 = 已确认是遗留）
		recovered, err := recoverPendingMessages("chat:messages", group, consumer, pendingRecoverMinIdle)
		if err == nil && len(recovered) > 0 {
			for _, msg := range recovered {
				handleStreamMessage(msg)
			}
			continue // 优先把恢复的消息处理完，再读新消息
		}

		// 2. 正常读取新消息
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{"chat:messages", ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if err != nil {
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				handleStreamMessage(msg)
			}
		}
	}
}

// recoverPendingMessages 用 XAUTOCLAIM 认领空闲时间超过 minIdle 仍未 ACK 的 Pending 消息。
// XAUTOCLAIM 会自动跳过空闲不足的消息，正常消费流程不会受影响。
func recoverPendingMessages(stream, group, consumer string, minIdle time.Duration) ([]redis.XMessage, error) {
	messages, _, err := rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: consumer,
		MinIdle:  minIdle,
		Start:    "0",
		Count:    10,
	}).Result()
	return messages, err
}

// handleStreamMessage 处理单条消息（新消息与恢复的 Pending 消息共用），处理完立即 ACK
func handleStreamMessage(msg redis.XMessage) {
	sender := msg.Values["sender"].(string)
	content := msg.Values["content"].(string)
	msgType := msg.Values["type"].(string)

	switch msgType {
	case "broadcast":
		sendMsg := fmt.Sprintf("%s：%s\n", sender, content)
		message <- sendMsg
	case "private":
		target := msg.Values["target"].(string)
		if targetConn, ok := getClientByName(target); ok {
			targetConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			targetConn.Write([]byte(fmt.Sprintf("【私聊】%s：%s\n", sender, content)))
		} else {
			// 离线：存进离线 List，等对方上线补发
			SaveOfflineMessage(target, sender, content)
		}
	}

	// ACK 必须在 switch 外面，两种消息都要确认
	AckMessage(msg.ID)
}

// AckMessage 确认消息已处理
func AckMessage(msgID string) error {
	return rdb.XAck(ctx, "chat:messages", "chat-group", msgID).Err()
}

// 存离线消息
func SaveOfflineMessage(username, sender, content string) error {
	key := fmt.Sprintf("offline:%s", username)
	msg := fmt.Sprintf("【私聊】%s：%s\n", sender, content)
	return rdb.RPush(ctx, key, msg).Err()
}

// 取离线消息
func GetOfflineMessages(username string) []string {
	key := fmt.Sprintf("offline:%s", username)

	cmds, err := rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.LRange(ctx, key, 0, -1)
		pipe.Del(ctx, key)
		return nil
	})
	if err != nil {
		return nil
	}
	if lr, ok := cmds[0].(*redis.StringSliceCmd); ok {
		return lr.Val()
	}
	return nil
}
