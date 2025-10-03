-- PostgreSQL 初始化脚本

-- 创建测试表
CREATE TABLE IF NOT EXISTS integration_test (
    id BIGINT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    data TEXT,
    value DECIMAL(10,2),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_integration_test_name ON integration_test(name);
CREATE INDEX IF NOT EXISTS idx_integration_test_email ON integration_test(email);
CREATE INDEX IF NOT EXISTS idx_integration_test_created_at ON integration_test(created_at);

-- 注意：只创建实际使用的表
-- 测试代码只使用 integration_test 表，避免创建无用的表

-- 优化 PostgreSQL 配置
ALTER SYSTEM SET shared_buffers = '128MB';
ALTER SYSTEM SET effective_cache_size = '256MB';
ALTER SYSTEM SET maintenance_work_mem = '64MB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET default_statistics_target = 100;

-- 重新加载配置
SELECT pg_reload_conf();

-- 创建用于监控的视图
CREATE OR REPLACE VIEW test_stats AS
SELECT 
    schemaname,
    relname as tablename,
    n_tup_ins as inserts,
    n_tup_upd as updates,
    n_tup_del as deletes,
    n_live_tup as live_tuples,
    n_dead_tup as dead_tuples,
    last_vacuum,
    last_autovacuum,
    last_analyze,
    last_autoanalyze
FROM pg_stat_user_tables 
WHERE schemaname = 'public';

-- 显示初始化信息
SELECT 'PostgreSQL Integration Test Environment Initialized' as status;
SELECT version() as postgresql_version;
SHOW shared_buffers;
SHOW max_connections;

-- 清理现有测试数据
TRUNCATE TABLE integration_test;

-- 分析表以更新统计信息
ANALYZE integration_test;