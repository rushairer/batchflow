#!/bin/bash

set -e

echo "🚀 启动 BatchFlow 单数据库压力测试"
echo "============================================================"

# 显示环境信息
echo "📋 环境信息："
echo "   容器操作系统：$(cat /etc/os-release | grep PRETTY_NAME | cut -d'"' -f2)"
echo "   可用内存：$(free -h | grep Mem | awk '{print $2}')"
echo "   CPU 核心数：$(nproc)"
echo ""

# 显示配置
echo "⚙️ 优化的高性能测试配置："
echo "   测试类型：${TEST_TYPE:-未配置}"
echo "   MySQL 连接串：${MYSQL_DSN:-未配置}"
echo "   PostgreSQL 连接串：${POSTGRES_DSN:-未配置}"
echo "   SQLite 连接串：${SQLITE_DSN:-未配置}"
echo "   Redis 连接串：${REDIS_DSN:-未配置}"
echo "   测试时长：${TEST_DURATION:-1800s}（无超时限制）"
echo "   并发工作线程：${CONCURRENT_WORKERS:-10}"
echo "   每线程记录数：${RECORDS_PER_WORKER:-2000}"
echo "   批大小：${BATCH_SIZE:-200}"
echo "   预计总记录数：$((${CONCURRENT_WORKERS:-10} * ${RECORDS_PER_WORKER:-2000}))"
echo "   内存优化：为持续高性能进行均衡调整"
echo ""

# 等待数据库服务就绪
echo "⏳ 正在等待数据库服务就绪..."

if [ "$TEST_TYPE" = "mysql" ] && [ ! -z "$MYSQL_DSN" ]; then
    echo "   正在检查 MySQL 连接..."
    MYSQL_READY=false
    for i in {1..30}; do
        if timeout 5 mysql -h mysql -u testuser -ptestpass123 -e "SELECT 1" >/dev/null 2>&1; then
            echo "   ✅ MySQL 已就绪，可进行高性能测试"
            MYSQL_READY=true
            break
        fi
        echo "   ⏳ 正在等待 MySQL……（第 $i/30 次尝试）"
        sleep 5
    done
    
    if [ "$MYSQL_READY" = false ]; then
        echo "   ❌ 150 秒后仍无法连接到 MySQL"
        exit 1
    fi
fi

if [ "$TEST_TYPE" = "postgres" ] && [ ! -z "$POSTGRES_DSN" ]; then
    echo "   正在检查 PostgreSQL 连接..."
    POSTGRES_READY=false
    for i in {1..30}; do
        if timeout 5 pg_isready -h postgres -U testuser >/dev/null 2>&1; then
            echo "   ✅ PostgreSQL 已就绪，可进行高性能测试"
            POSTGRES_READY=true
            break
        fi
        echo "   ⏳ 正在等待 PostgreSQL……（第 $i/30 次尝试）"
        sleep 5
    done
    
    if [ "$POSTGRES_READY" = false ]; then
        echo "   ❌ 150 秒后仍无法连接到 PostgreSQL"
        exit 1
    fi
fi

if [ "$TEST_TYPE" = "sqlite" ] && [ ! -z "$SQLITE_DSN" ]; then
    echo "   正在检查 SQLite 数据库..."
    DB_FILE="$SQLITE_DSN"
    case "$DB_FILE" in
      file:*) DB_FILE="${DB_FILE#file:}";;
    esac
    DB_FILE="${DB_FILE%%\?*}"
    SQLITE_READY=false
    for i in {1..30}; do
        # 就绪判定：文件存在 或 可被 sqlite3 打开
        if [ -f "$DB_FILE" ] || (command -v sqlite3 >/dev/null 2>&1 && sqlite3 "$DB_FILE" ".tables" >/dev/null 2>&1); then
            echo "   ✅ SQLite 已就绪，可进行高性能测试"
            SQLITE_READY=true
            break
        fi
        echo "   ⏳ 正在等待 SQLite……（第 $i/30 次尝试）"
        sleep 5
    done
    
    if [ "$SQLITE_READY" = false ]; then
        echo "   ❌ 150 秒后仍无法连接到 SQLite"
        exit 1
    fi
fi

if [ "$TEST_TYPE" = "redis" ] && [ ! -z "$REDIS_DSN" ]; then
    echo "   正在检查 redis 数据库..."
    REDIS_READY=false
    for i in {1..30}; do
        if timeout 5 redis-cli -h redis -a testpass123 ping >/dev/null 2>&1; then
            echo "   ✅ redis 已就绪，可进行高性能测试"
            REDIS_READY=true
            break
        fi
        echo "   ⏳ 正在等待 redis……（第 $i/30 次尝试）"
        sleep 5
    done
    
    if [ "$REDIS_READY" = false ]; then
        echo "   ❌ 150 秒后仍无法连接到 redis"
        exit 1
    fi
fi

echo ""

# 创建报告目录
mkdir -p /app/reports

# 启动健康检查服务器（后台运行）
echo "🏥 启动健康检查服务器..."
(
    while true; do
        echo -e "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK" | nc -l -p 8888
        sleep 1
    done
) &
HEALTH_PID=$!

# 运行压力测试 - 无超时限制
echo "🧪 运行优化的高性能压力测试..."
echo "============================================================"
echo "🚨 无超时限制——测试将一直运行至完成"
echo "💪 优化并发：${CONCURRENT_WORKERS:-10} 个工作线程"
echo "📊 大规模数据集：总计 $((${CONCURRENT_WORKERS:-10} * ${RECORDS_PER_WORKER:-2000})) 条记录"
echo "⚡ 均衡吞吐：每批 ${BATCH_SIZE:-200} 条记录"
echo "🧠 内存已优化以保证持续性能"
echo ""

# 运行测试 - 移除所有超时限制
echo "🚀 启动压力测试可执行文件..."
./integration-test

TEST_EXIT_CODE=$?

# 停止健康检查服务器
kill $HEALTH_PID 2>/dev/null || true

echo ""
echo "============================================================"

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "🎉 高性能压力测试已成功完成！"
    echo ""
    echo "📊 性能报告已生成于 /app/reports/"
    ls -la /app/reports/
else
    echo "❌ 压力测试失败，退出码：$TEST_EXIT_CODE"
    echo ""
    echo "📊 请查看报告进行性能分析："
    ls -la /app/reports/ 2>/dev/null || echo "未生成任何报告"
fi

echo ""
echo "🔍 最终系统资源使用情况："
echo "   内存使用：$(free -h | grep Mem | awk '{print $3 "/" $2}')"
echo "   磁盘使用：$(df -h /app | tail -1 | awk '{print $3 "/" $2 " (" $5 ")"}')"

exit $TEST_EXIT_CODE