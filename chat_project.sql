-- =============================================================
-- 聊天室项目 - 完整建表脚本
-- 使用方式：在 Navicat 中新建数据库 chat_project，然后运行此脚本
-- =============================================================

-- 1. 用户表
CREATE TABLE IF NOT EXISTS `sys_user` (
    `id`         BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '用户ID',
    `username`   VARCHAR(64)  NOT NULL COMMENT '登录用户名',
    `password`   VARCHAR(255) NOT NULL COMMENT 'bcrypt加密后的密码',
    `nickname`   VARCHAR(64)  NOT NULL COMMENT '显示昵称（聊天室显示用）',
    `email`      VARCHAR(128) DEFAULT '' COMMENT '邮箱',
    `avatar`     VARCHAR(255) DEFAULT '' COMMENT '头像URL',
    `status`     TINYINT      DEFAULT 1 COMMENT '状态：1=启用 0=禁用',
    `created_at` DATETIME     DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME     DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY `uk_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- 2. 角色表
CREATE TABLE IF NOT EXISTS `sys_role` (
    `id`          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '角色ID',
    `name`        VARCHAR(64)  NOT NULL COMMENT '角色名称（如"管理员"）',
    `code`        VARCHAR(64)  NOT NULL COMMENT '角色编码（如 admin）',
    `description` VARCHAR(255) DEFAULT '' COMMENT '描述',
    `status`      TINYINT      DEFAULT 1 COMMENT '状态：1=启用 0=禁用',
    `created_at`  DATETIME     DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at`  DATETIME     DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色表';

-- 3. 权限表
CREATE TABLE IF NOT EXISTS `sys_permission` (
    `id`          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '权限ID',
    `name`        VARCHAR(64)  NOT NULL COMMENT '权限名称（如"用户创建"）',
    `code`        VARCHAR(128) NOT NULL COMMENT '权限编码（如 user:create）',
    `description` VARCHAR(255) DEFAULT '' COMMENT '描述',
    `type`        TINYINT      DEFAULT 1 COMMENT '类型：1=菜单 2=按钮/操作',
    `parent_id`   BIGINT       DEFAULT 0 COMMENT '父权限ID（0表示顶级）',
    `created_at`  DATETIME     DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    UNIQUE KEY `uk_code` (`code`),
    KEY `idx_parent` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='权限表';

-- 4. 菜单表
CREATE TABLE IF NOT EXISTS `sys_menu` (
    `id`          BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '菜单ID',
    `name`        VARCHAR(64)  NOT NULL COMMENT '菜单名称（如"系统设置"）',
    `icon`        VARCHAR(64)  DEFAULT '' COMMENT '图标',
    `path`        VARCHAR(128) DEFAULT '' COMMENT '路由路径',
    `component`   VARCHAR(128) DEFAULT '' COMMENT '前端组件路径',
    `parent_id`   BIGINT       DEFAULT 0 COMMENT '父菜单ID（0表示顶级）',
    `sort`        INT          DEFAULT 0 COMMENT '排序序号',
    `status`      TINYINT      DEFAULT 1 COMMENT '状态：1=显示 0=隐藏',
    `created_at`  DATETIME     DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    KEY `idx_parent` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='菜单表';

-- 5. 用户-角色关联表（多对多）
CREATE TABLE IF NOT EXISTS `sys_user_role` (
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `role_id` BIGINT NOT NULL COMMENT '角色ID',
    PRIMARY KEY (`user_id`, `role_id`),
    KEY `idx_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户角色关联表';

-- 6. 角色-权限关联表（多对多）
CREATE TABLE IF NOT EXISTS `sys_role_permission` (
    `role_id`       BIGINT NOT NULL COMMENT '角色ID',
    `permission_id` BIGINT NOT NULL COMMENT '权限ID',
    PRIMARY KEY (`role_id`, `permission_id`),
    KEY `idx_permission` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='角色权限关联表';

-- =============================================================
-- 基础种子数据
-- =============================================================

-- 插入两个默认角色
INSERT INTO `sys_role` (`name`, `code`, `description`) VALUES
('管理员', 'admin', '系统管理员，拥有所有权限'),
('普通用户', 'user', '普通聊天室用户');

-- 插入基础菜单（你列的那些）
INSERT INTO `sys_menu` (`name`, `icon`, `path`, `component`, `parent_id`, `sort`) VALUES
('首页',        'Home',        '/dashboard',     'Dashboard',     0, 1),
('消息通知',    'Bell',        '/notifications',  'Notifications', 0, 2),
('大屏',        'Monitor',     '/screen',         'Screen',        0, 3),
('系统设置',    'Setting',     '/system',         'System',        0, 4),
('用户管理',    'User',        '/system/users',   'System/User',   4, 1),
('角色管理',    'Safety',      '/system/roles',   'System/Role',   4, 2),
('权限管理',    'Key',         '/system/permissions', 'System/Permission', 4, 3);

-- 插入基础权限
INSERT INTO `sys_permission` (`name`, `code`, `type`, `parent_id`) VALUES
('用户管理',    'user:manage',  1, 0),
('用户列表',    'user:list',    2, 1),
('新增用户',    'user:create',  2, 1),
('编辑用户',    'user:edit',    2, 1),
('删除用户',    'user:delete',  2, 1),
('角色管理',    'role:manage',  1, 0),
('角色列表',    'role:list',    2, 6),
('新增角色',    'role:create',  2, 6),
('编辑角色',    'role:edit',    2, 6),
('删除角色',    'role:delete',  2, 6),
('菜单管理',    'menu:manage',  1, 0),
('菜单列表',    'menu:list',    2, 11),
('新增菜单',    'menu:create',  2, 11),
('编辑菜单',    'menu:edit',    2, 11),
('删除菜单',    'menu:delete',  2, 11);

-- 给管理员角色分配所有权限
INSERT INTO `sys_role_permission` (`role_id`, `permission_id`)
SELECT 1, id FROM `sys_permission`;
