package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// 用户结构体
type Client struct {
	Name string
	Conn net.Conn
}

// 在线用户列表（以 conn 作为 key，防止网名一样时引发冲突）
var clients = make(map[net.Conn]Client)

// 互斥锁（解决并发冲突）
var lock sync.Mutex

// 带缓冲的消息通道，避免广播阻塞 [修改]
var message = make(chan string, 100)

// ===== [新增] 广播消息 =====
func broadcast() {
	for {
		msg := <-message

		// 服务器显示消息
		fmt.Print(msg)

		lock.Lock()
		for _, cli := range clients {
			// 设置写超时，防止慢客户端阻塞所有人 [新增]
			cli.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err := cli.Conn.Write([]byte(msg))
			if err != nil {
				// 写失败不处理，由 process() 的心跳检测来清理 [修改]
			}
		}
		lock.Unlock()
	}
}

// ===== [新增] 昵称验证 =====
func isValidName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if strings.Contains(name, " ") {
		return false
	}
	return true
}

// ===== [新增] 检查昵称是否已存在 =====
func isNameExists(name string) bool {
	lock.Lock()
	defer lock.Unlock()
	for _, cli := range clients {
		if cli.Name == name {
			return true
		}
	}
	return false
}

// ===== [新增] 按昵称查找用户 =====
func getClientByName(name string) (net.Conn, bool) {
	lock.Lock()
	defer lock.Unlock()
	for _, cli := range clients {
		if cli.Name == name {
			return cli.Conn, true
		}
	}
	return nil, false
}

// ===== [新增] 心跳检测协程 =====
func heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		select {
		case message <- "PING\n":
		default:
			// 队列满了就跳过本次心跳
		}
	}
}

// 处理用户连接
func process(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// ===== [修改] 读取并验证昵称 =====
	name, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	name = strings.Trim(name, "\r\n")

	// 验证昵称格式
	if !isValidName(name) {
		conn.Write([]byte("ERR:昵称包含空格或是空白内容，请重新连接\n"))
		return
	}

	// 检查昵称是否已存在
	if isNameExists(name) {
		conn.Write([]byte("ERR:昵称已被使用，请重新连接\n"))
		return
	}

	// 通知客户端昵称验证通过
	conn.Write([]byte("OK\n"))

	client := Client{
		Name: name,
		Conn: conn,
	}

	lock.Lock()
	clients[conn] = client
	count := len(clients)
	lock.Unlock()

	// 广播加入消息
	message <- fmt.Sprintf("【系统】%s 加入了聊天室（当前在线：%d人）\n", name, count)

	// ===== [修改] 循环接收消息，加入心跳超时机制 =====
	for {
		// 设置读取超时：60 秒内没有收到任何数据（含 PONG）则认为客户端已断开 [新增]
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		msg, err := reader.ReadString('\n')
		if err != nil {
			// 读取失败（断开或超时），清理客户端 [修改]
			lock.Lock()
			delete(clients, conn)
			count = len(clients)
			lock.Unlock()

			message <- fmt.Sprintf("【系统】%s 离开了聊天室（当前在线：%d人）\n", name, count)
			return
		}

		msg = strings.Trim(msg, "\r\n")

		// ===== [新增] 心跳响应，忽略 PONG =====
		if msg == "PONG" {
			continue
		}

		// ===== [新增] 处理退出命令 =====
		if msg == "exit" || msg == "/exit" {
			// 通知对方退出成功
			conn.Write([]byte("BYE\n"))

			lock.Lock()
			delete(clients, conn)
			count = len(clients)
			lock.Unlock()

			message <- fmt.Sprintf("【系统】%s 离开了聊天室（当前在线：%d人）\n", name, count)
			return
		}

		// ===== [新增] 私聊功能：@用户名 消息 =====
		if strings.HasPrefix(msg, "@") {
			// 按第一个空格分割
			spaceIdx := strings.Index(msg, " ")
			if spaceIdx > 1 { // @ 后面至少跟一个字符
				targetName := msg[1:spaceIdx]
				content := strings.TrimSpace(msg[spaceIdx+1:])
				if content != "" {
					targetConn, ok := getClientByName(targetName)
					if ok {
						// 发送给目标
						targetConn.Write([]byte(fmt.Sprintf("【私聊】%s：%s\n", name, content)))
						// 发送给自己（提示已发出）
						if targetConn != conn {
							conn.Write([]byte(fmt.Sprintf("【私聊】对 %s：%s\n", targetName, content)))
						}
					} else {
						conn.Write([]byte(fmt.Sprintf("【系统】用户 %s 不在线\n", targetName)))
					}
					continue
				}
			}
		}

		// 广播普通消息
		if msg != "" {
			sendMsg := fmt.Sprintf("%s：%s\n", name, msg)
			message <- sendMsg
		}
	}
}

func main() {
	// 开启监听
	listen, err := net.Listen("tcp", "0.0.0.0:8888")
	if err != nil {
		fmt.Println("listen err:", err)
		return
	}
	defer listen.Close()

	fmt.Println("聊天室服务器启动成功...")

	// 开启广播协程
	go broadcast()

	// ===== [新增] 开启心跳检测协程 =====
	go heartbeat()

	// ===== 循环等待连接 =====
	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println("accept err:", err)
			continue
		}

		go process(conn)
	}
}
