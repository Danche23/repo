package main

// =====  认证模块：注册 + 登录 [新增文件] =====

import (
	"database/sql"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// User 用户模型
type User struct {
	ID       int64
	Username string
	Nickname string
	Status   int
}

// ---------- 注册 ----------

func Register(username, password string) error {
	// 1. 检查用户名是否已存在
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM sys_user WHERE username = ?", username).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("用户名已存在")
	}

	// 2. bcrypt 加密密码（自动加盐，不可逆）
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// 3. 插入数据库（昵称先用用户名，后续可以改）
	_, err = DB.Exec(
		"INSERT INTO sys_user (username, password, nickname, status) VALUES (?, ?, ?, 1)",
		username, string(hashed), username,
	)
	return err
}

// ---------- 登录 ----------

func Login(username, password string) (*User, error) {
	user := &User{}

	// 1. 按用户名查数据库（包含 password 字段用于 bcrypt 比对）
	row := DB.QueryRow(
		"SELECT id, username, nickname, status, password FROM sys_user WHERE username = ?",
		username,
	)

	// 用来比对密码的哈希值（不返回给调用者）
	var hashedPassword string
	err := row.Scan(&user.ID, &user.Username, &user.Nickname, &user.Status, &hashedPassword)

	// 没查到
	if err == sql.ErrNoRows {
		return nil, errors.New("用户名或密码错误")
	}
	if err != nil {
		return nil, err
	}

	// 2. 检查账号是否被禁用
	if user.Status == 0 {
		return nil, errors.New("账号已被禁用，请联系管理员")
	}

	// 3. bcrypt 验证密码（把用户输入的密码 和 数据库存的哈希 比对）
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	return user, nil
}
