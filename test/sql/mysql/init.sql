-- MySQL 初始化脚本
-- 创建测试数据库和用户（如果不存在）

-- 确保数据库存在
CREATE DATABASE IF NOT EXISTS batchflow_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 使用测试数据库
USE batchflow_test;

-- 创建测试表
CREATE TABLE IF NOT EXISTS integration_test (
    id BIGINT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    data TEXT,
    value DECIMAL(10,2),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_name (name),
    INDEX idx_email (email),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 注意：只创建实际使用的表
-- 测试代码只使用 integration_test 表，避免创建无用的表

-- 优化 MySQL 配置（通过变量设置）
-- 注意：某些变量是只读的，只能在启动时设置
-- SET GLOBAL innodb_buffer_pool_size = 268435456;  -- 256MB (只读变量)
-- SET GLOBAL innodb_log_file_size = 67108864;      -- 64MB (只读变量)
SET GLOBAL innodb_flush_log_at_trx_commit = 2;   -- 性能优化
SET GLOBAL sync_binlog = 0;                      -- 测试环境性能优化

-- 显示配置信息
SELECT 'MySQL Integration Test Environment Initialized' as status;
SELECT VERSION() as mysql_version;
SHOW VARIABLES LIKE 'innodb_buffer_pool_size';
SHOW VARIABLES LIKE 'max_connections';