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

// 在线用户列表（以 conn 作为 key）
var clients = make(map[net.Conn]Client)

// 互斥锁（解决并发冲突）
var lock sync.Mutex

// 带缓冲的消息通道，避免广播阻塞
var message = make(chan string, 100)

// =====  广播消息 =====
func broadcast() {
	for {
		msg := <-message

		// 服务器显示消息
		fmt.Print(msg)

		lock.Lock()
		for _, cli := range clients {
			// 设置写超时，防止慢客户端阻塞所有人
			cli.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, err := cli.Conn.Write([]byte(msg))
			if err != nil {
				// 写失败不处理，由 process() 的心跳检测来清理 [修改]
			}
		}
		lock.Unlock()
	}
}

// =====  按昵称查找用户 =====
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

// =====  心跳检测协程 =====
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

	// =====  登录/注册流程 [修改] =====
	var name string

	// 读取选择：1=登录  2=注册
	choice, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	choice = strings.Trim(choice, "\r\n")

	// 读取用户名
	username, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	username = strings.Trim(username, "\r\n")

	// 读取密码
	password, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	password = strings.Trim(password, "\r\n")

	switch choice {
	case "1": // 登录 [修改]
		user, err := Login(username, password)
		if err != nil {
			conn.Write([]byte("ERR:" + err.Error() + "\n"))
			return
		}
		conn.Write([]byte("OK\n"))
		name = user.Nickname

	case "2": // 注册 [新增]
		// 读取确认密码
		pwd2, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		pwd2 = strings.Trim(pwd2, "\r\n")
		if password != pwd2 {
			conn.Write([]byte("ERR:两次密码输入不一致\n"))
			return
		}
		// 注册 [新增]
		err = Register(username, password)
		if err != nil {
			conn.Write([]byte("ERR:" + err.Error() + "\n"))
			return
		}
		conn.Write([]byte("OK\n"))
		name = username

	default:
		conn.Write([]byte("ERR:无效选择，请选择 1(登录) 或 2(注册)\n"))
		return
	}

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

	// =====  循环接收消息，加入心跳超时机制 =====
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

		// =====  心跳响应，忽略 PONG =====
		if msg == "PONG" {
			continue
		}

		// =====  处理退出命令 =====
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

		// ===== 私聊功能：@用户名 消息 =====
		if strings.HasPrefix(msg, "@") {
			spaceIdx := strings.Index(msg, " ")

			// 格式不对（@、@名字）
			if spaceIdx == -1 || spaceIdx <= 1 {
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.Write([]byte("【系统】私聊格式：@用户名 消息\n"))
				continue
			}

			targetName := msg[1:spaceIdx]
			content := strings.TrimSpace(msg[spaceIdx+1:])

			if content == "" {
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.Write([]byte(fmt.Sprintf("【系统】请输入要发送给 %s 的消息内容\n", targetName)))
				continue
			}

			// 内容以 @ 开头，可能是用户误写了多个 @
			if strings.HasPrefix(content, "@") {
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.Write([]byte("【系统】一次只能私聊一个人，消息内容不能以 @ 开头\n"))
				continue
			}

			targetConn, ok := getClientByName(targetName)
			if ok {
				targetConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				targetConn.Write([]byte(fmt.Sprintf("【私聊】%s：%s\n", name, content)))
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.Write([]byte(fmt.Sprintf("【私聊】对 %s：%s\n", targetName, content)))
			} else {
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.Write([]byte(fmt.Sprintf("【系统】用户 %s 不在线\n", targetName)))
			}
			continue
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

	// =====  初始化数据库连接 [新增] =====
	err = InitDB()
	if err != nil {
		fmt.Println("数据库初始化失败:", err)
		return
	}

	// 开启广播协程
	go broadcast()

	// =====  开启心跳检测协程 =====
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
