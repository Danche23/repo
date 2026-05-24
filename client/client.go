package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	// 连接服务器
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		fmt.Println("client dial err:", err)
		return
	}
	defer conn.Close()

	// 创建带缓冲的连接读取器，用于处理粘包 [新增]
	connReader := bufio.NewReader(conn)

	stdinReader := bufio.NewReader(os.Stdin)

	// ===== [修改] 输入并验证昵称 =====
	var name string
	for {
		fmt.Print("请输入你的网名：")
		n, _ := stdinReader.ReadString('\n')
		n = strings.Trim(n, "\r\n")

		if n == "" || strings.Contains(n, " ") {
			fmt.Println("昵称不能包含空格或是空白内容，请重新输入")
			continue
		}
		name = n
		break
	}

	// 发送昵称给服务器
	conn.Write([]byte(name + "\n"))

	// ===== [新增] 读取服务器对昵称的验证结果 =====
	resp, _ := connReader.ReadString('\n')
	resp = strings.Trim(resp, "\r\n")

	if resp == "OK" {
		fmt.Println("验证通过，进入聊天室...")
	} else if strings.HasPrefix(resp, "ERR:") {
		fmt.Println(resp[4:]) // 去掉 "ERR:" 前缀显示错误原因
		return
	}

	// ===== [修改] 协程接收服务器消息 =====
	// 使用 ReadString 逐行读取，不会出现粘包问题 [新增]
	go func() {
		for {
			msg, err := connReader.ReadString('\n')
			if err != nil {
				fmt.Println("\n服务器已断开连接")
				os.Exit(1)
			}

			msg = strings.Trim(msg, "\r\n")

			// ===== [新增] 响应心跳检测 =====
			if msg == "PING" {
				conn.Write([]byte("PONG\n"))
				continue
			}

			// ===== [新增] 收到退出确认 =====
			if msg == "BYE" {
				continue
			}

			fmt.Print(msg + "\n")
		}
	}()

	// 循环发送聊天消息
	for {
		line, err := stdinReader.ReadString('\n')
		if err != nil {
			fmt.Println("readString err:", err)
			continue
		}

		line = strings.Trim(line, "\r\n")

		// ===== [修改] 退出命令 =====
		if line == "exit" || line == "/exit" {
			// 通知服务器
			conn.Write([]byte("exit\n"))
			fmt.Println("客户端退出")
			return
		}

		// ===== [修改] 私聊命令也发送（@用户名 消息） =====
		_, err = conn.Write([]byte(line + "\n"))
		if err != nil {
			fmt.Println("conn write err:", err)
		}
	}
}
