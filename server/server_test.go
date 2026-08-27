package main

import (
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeConn 模拟一个网络连接。block=true 时 Write 会阻塞，模拟慢客户端。
type fakeConn struct {
	mu      sync.Mutex
	block   bool
	entered chan struct{} // Write 进入阻塞时关闭，让测试感知阻塞发生
	unblock chan struct{} // Close 时关闭，用于解除 Write 的阻塞
	closed  bool
	written []byte
}

func newFakeConn(block bool) *fakeConn {
	return &fakeConn{
		block:   block,
		entered: make(chan struct{}),
		unblock: make(chan struct{}),
	}
}

func (c *fakeConn) Read([]byte) (int, error) { return 0, io.EOF }

func (c *fakeConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}

	if c.block {
		// 通知测试：已经进入阻塞写
		select {
		case <-c.entered:
		default:
			close(c.entered)
		}
		// 阻塞直到 Close
		<-c.unblock
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	c.written = append(c.written, b...)
	return len(b), nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	select {
	case <-c.unblock:
	default:
		close(c.unblock)
	}
	return nil
}

func (c *fakeConn) LocalAddr() net.Addr              { return nil }
func (c *fakeConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// TestBroadcastDoesNotHoldLockDuringWrite 验证广播写慢客户端（Write 阻塞）时不会持有全局锁。
// 修复前（锁内写）该测试会失败：锁被写操作占用，无法在超时内获取。
func TestBroadcastDoesNotHoldLockDuringWrite(t *testing.T) {
	slow := newFakeConn(true)  // Write 会阻塞的慢客户端
	fast := newFakeConn(false) // 正常客户端

	lock.Lock()
	clients[slow] = Client{Name: "slow", Conn: slow}
	clients[fast] = Client{Name: "fast", Conn: fast}
	lock.Unlock()

	defer func() {
		slow.Close()
		fast.Close()
		lock.Lock()
		delete(clients, slow)
		delete(clients, fast)
		lock.Unlock()
	}()

	go broadcast()

	// 发一条消息触发广播
	message <- "test broadcast\n"

	// 等待 slow 进入阻塞写
	select {
	case <-slow.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("慢客户端没有进入阻塞写状态，无法验证")
	}

	// 关键断言：broadcast 正在写慢客户端（阻塞中），此时全局锁必须是空闲的
	got := make(chan struct{})
	go func() {
		lock.Lock()
		close(got)
		lock.Unlock()
	}()

	select {
	case <-got:
		// 锁空闲 —— 写操作没有持有锁，修复生效
	case <-time.After(1 * time.Second):
		t.Fatal("broadcast 在写慢客户端时仍持有全局锁，慢客户端会阻塞整个服务器")
	}
}

// TestHelpText 验证 /help 的输出覆盖所有命令，且每条命令都带用法说明。
func TestHelpText(t *testing.T) {
	cmds := []string{
		"/help",
		"/排行榜 [数量]",
		"/排名",
		"/exit",
		"@用户名 消息",
		"直接输入文字",
	}
	for _, cmd := range cmds {
		if !strings.Contains(helpText, cmd) {
			t.Errorf("helpText 缺少命令 %q", cmd)
		}
	}

	// 命令行的说明列非空：每行命令后面应有空格分隔的说明文字
	for _, line := range strings.Split(helpText, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "@") {
			if !strings.Contains(trimmed, " ") {
				t.Errorf("命令缺少说明: %q", trimmed)
			}
		}
	}
}
