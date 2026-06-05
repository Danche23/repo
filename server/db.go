package main

// =====  数据库连接 [新增文件] =====

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
)

// DB 全局数据库连接池
var DB *sql.DB

// InitDB 初始化数据库连接
// 如果密码不是 123456，在这里改
func InitDB() error {
	dsn := "root:123456@tcp(127.0.0.1:3306)/chat_project?charset=utf8mb4&parseTime=true"

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("连接字符串格式错误: %v", err)
	}

	// 真正尝试连接
	err = DB.Ping()
	if err != nil {
		return fmt.Errorf("数据库连接失败，检查MySQL是否启动、密码是否正确: %v", err)
	}

	// 连接池设置
	DB.SetMaxOpenConns(10) // 最多同时10个连接
	DB.SetMaxIdleConns(5)  // 最多保持5个空闲连接

	fmt.Println("✅ 数据库连接成功")
	return nil
}
