package main

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newTestRedis 连接本地 Redis 的独立测试库（DB 15）。
// Redis 不可用时跳过测试，保证不影响正常 go test。
func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		t.Skipf("本地 Redis 不可用，跳过集成测试: %v", err)
	}
	return client
}

// TestRecoverPendingMessages 端到端验证 Pending 恢复链路：
// 生产 → 读到未 ACK（进入 Pending，模拟崩溃遗留）→ recoverPendingMessages 认领
// → handleStreamMessage 处理（离线目标 → 存入离线 List）→ ACK 后 Pending 清空。
//
// 用与线上相同的 stream/group 名，但落在独立的 DB 15 上（线上服务用 DB 1），
// 因此 AckMessage 里写死的 stream/group 能正确命中，且不会干扰线上数据。
func TestRecoverPendingMessages(t *testing.T) {
	testRdb := newTestRedis(t)
	defer testRdb.Close()

	// 把全局 rdb 临时切到测试库，使 recoverPendingMessages / handleStreamMessage 都落在 DB 15
	oldRdb := rdb
	rdb = testRdb
	defer func() { rdb = oldRdb }()

	const (
		stream      = "chat:messages"
		group       = "chat-group"
		offlineUser = "ghost-test"
	)
	ctx := context.Background()

	// 先清掉可能残留的测试数据，保证消费组是新建的
	testRdb.Del(ctx, stream, "offline:"+offlineUser)
	defer testRdb.Del(ctx, stream, "offline:"+offlineUser)

	// 1. 创建消费组
	if err := testRdb.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil {
		t.Fatalf("创建消费组失败: %v", err)
	}

	// 2. 生产一条私聊消息
	_, err := testRdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"sender":  "alice",
			"content": "hello offline",
			"type":    "private",
			"target":  offlineUser,
		},
	}).Result()
	if err != nil {
		t.Fatalf("XAdd 失败: %v", err)
	}

	// 3. 模拟"崩溃前已读取但未 ACK"：旧消费者读到消息但不 ACK
	msgs, err := testRdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "old-consumer",
		Streams:  []string{stream, ">"},
		Count:    10,
	}).Result()
	if err != nil || len(msgs) == 0 || len(msgs[0].Messages) != 1 {
		t.Fatalf("XReadGroup 读取失败: %v", err)
	}
	msgID := msgs[0].Messages[0].ID

	// 4. 断言消息已进入 Pending
	pending, err := testRdb.XPending(ctx, stream, group).Result()
	if err != nil || pending.Count != 1 {
		t.Fatalf("消息应进入 Pending，Count=%d err=%v", pending.Count, err)
	}

	// 5. 等消息空闲时间超过 MinIdle，再用恢复函数认领（线上用 10s，这里用小值加速验证）
	time.Sleep(300 * time.Millisecond)
	recovered, err := recoverPendingMessages(stream, group, "consumer-1", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("recoverPendingMessages 失败: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != msgID {
		t.Fatalf("应认领到 1 条消息 %s，实际得到 %v", msgID, recovered)
	}

	// 6. 走正常处理路径：目标离线 → 存入离线 List，并 ACK
	handleStreamMessage(recovered[0])

	offline, err := testRdb.LRange(ctx, "offline:"+offlineUser, 0, -1).Result()
	if err != nil || len(offline) != 1 {
		t.Fatalf("离线消息应被存入 List，len=%d err=%v", len(offline), err)
	}

	// 7. 断言 ACK 后 Pending 清空（原有 ACK 逻辑未被破坏）
	pending2, err := testRdb.XPending(ctx, stream, group).Result()
	if err != nil || pending2.Count != 0 {
		t.Fatalf("ACK 后 Pending 应清空，Count=%d err=%v", pending2.Count, err)
	}
}
