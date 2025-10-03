-- SQLite 初始化脚本
-- SQLite 特性：轻量级、嵌入式、无服务器架构

-- 启用 WAL 模式以提高并发性能
PRAGMA journal_mode = WAL;

-- 优化 SQLite 性能配置
PRAGMA synchronous = NORMAL;        -- 平衡性能和安全性
PRAGMA cache_size = 10000;          -- 10MB 缓存
PRAGMA temp_store = MEMORY;         -- 临时表存储在内存中
PRAGMA mmap_size = 268435456;       -- 256MB 内存映射
PRAGMA optimize;                    -- 启用查询优化器

-- 创建测试表
CREATE TABLE IF NOT EXISTS integration_test (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    data TEXT,
    value REAL,
    is_active INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_integration_test_name ON integration_test(name);
CREATE INDEX IF NOT EXISTS idx_integration_test_email ON integration_test(email);
CREATE INDEX IF NOT EXISTS idx_integration_test_created_at ON integration_test(created_at);

-- 注意：只创建实际使用的表
-- 测试代码只使用 integration_test 表，避免创建无用的表

-- 创建用于监控的视图（SQLite 版本）
CREATE VIEW IF NOT EXISTS test_stats AS
SELECT 
    name as table_name,
    sql as table_definition
FROM sqlite_master 
WHERE type = 'table' 
AND name NOT LIKE 'sqlite_%';

-- 清理现有测试数据
DELETE FROM integration_test;

-- 执行 VACUUM 以优化数据库
VACUUM;

-- 分析表以更新统计信息
ANALYZE integration_test;

-- 显示初始化信息
SELECT 'SQLite Integration Test Environment Initialized' as status;
SELECT sqlite_version() as sqlite_version;

-- 显示当前配置
PRAGMA journal_mode;
PRAGMA synchronous;
PRAGMA cache_size;
PRAGMA temp_store;

-- 显示表信息
SELECT 
    name as table_name,
    'Main test table for basic operations' as description
FROM sqlite_master 
WHERE type = 'table' 
AND name = 'integration_test'
ORDER BY name;