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

	// 创建带缓冲的连接读取器，用于处理粘包
	connReader := bufio.NewReader(conn)

	stdinReader := bufio.NewReader(os.Stdin)

	// ===== 登录/注册流程 =====
	for {
		fmt.Println("=== 欢迎来到聊天室 ===")
		fmt.Print("1. 登录  2. 注册  请选择：")
		choice, _ := stdinReader.ReadString('\n')
		choice = strings.Trim(choice, "\r\n")

		// 允许退出
		if choice == "exit" || choice == "/exit" {
			fmt.Println("客户端退出")
			return
		}

		if choice != "1" && choice != "2" {
			fmt.Println("❌ 请选择 1（登录）或 2（注册）")
			continue
		}

		// 发送选择给服务器
		conn.Write([]byte(choice + "\n"))

		// 输入用户名
		fmt.Print("请输入用户名：")
		username, _ := stdinReader.ReadString('\n')
		username = strings.Trim(username, "\r\n")
		conn.Write([]byte(username + "\n"))

		// 输入密码
		fmt.Print("请输入密码：")
		password, _ := stdinReader.ReadString('\n')
		password = strings.Trim(password, "\r\n")
		conn.Write([]byte(password + "\n"))

		// 注册需要确认密码
		if choice == "2" {
			fmt.Print("请再次输入密码：")
			pwd2, _ := stdinReader.ReadString('\n')
			pwd2 = strings.Trim(pwd2, "\r\n")
			conn.Write([]byte(pwd2 + "\n"))
		}

		// 读取服务器验证结果
		resp, _ := connReader.ReadString('\n')
		resp = strings.Trim(resp, "\r\n")

		if resp == "OK" {
			if choice == "2" {
				fmt.Println("✅ 注册成功！")
			}
			fmt.Println("验证通过，进入聊天室...")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Println("直接输入文字即可聊天，输入 /help 查看所有命令")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━")
			break
		} else if strings.HasPrefix(resp, "ERR:") {
			fmt.Println("❌ " + resp[4:]) // 去掉 "ERR:" 前缀，显示错误原因
			return
		}
	}

	// =====  协程接收服务器消息 =====
	// 使用 ReadString 逐行读取，不会出现粘包问题
	go func() {
		for {
			msg, err := connReader.ReadString('\n')
			if err != nil {
				fmt.Println("\n服务器已断开连接")
				os.Exit(1)
			}

			msg = strings.Trim(msg, "\r\n")

		// =====  响应心跳检测 =====
		if msg == "PING" {
			conn.Write([]byte("PONG\n"))
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

		// =====  退出命令（统一以 / 开头） =====
		if line == "/exit" {
			// 通知服务器
			conn.Write([]byte("/exit\n"))
			fmt.Println("客户端退出")
			return
		}

		// =====  私聊命令也发送（@用户名 消息） =====
		_, err = conn.Write([]byte(line + "\n"))
		if err != nil {
			fmt.Println("conn write err:", err)
		}
	}
}
