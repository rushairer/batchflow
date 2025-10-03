#!/usr/bin/env bash
set -euo pipefail

# 压力验证脚本：启动监控环境并运行集成测试，输出测试与监控访问方式。
# 依赖：Makefile 目标（参考 README）
# 用法：
#   chmod +x scripts/stress_with_monitoring.sh
#   ./scripts/stress_with_monitoring.sh [db]
# 参数 db：可选 mysql|postgres|sqlite|redis|all，默认 all

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB="${1:-all}"

pushd "$ROOT_DIR" >/dev/null

echo ">> 启动监控环境（Prometheus + Grafana）"
make monitoring

echo ">> 等待监控初始化 5s..."
sleep 5

run_db_test() {
  local db="$1"
  case "$db" in
    mysql)    make docker-mysql-test ;;
    postgres) make docker-postgres-test ;;
    sqlite)   make docker-sqlite-test ;;
    redis)    make docker-redis-test ;;
    all)      make docker-all-tests ;;
    *)        echo "未知数据库: $db"; exit 1 ;;
  esac
}

echo ">> 运行集成测试: $DB"
run_db_test "$DB"

cat <<EOF

================= 访问方式 =================
Grafana:     http://localhost:3000  (admin/admin)
Prometheus:  http://localhost:9091
BatchFlow指标: http://localhost:9090/metrics

提示：
- 可在 Grafana 中添加 p95/p99、批大小、入队/攒批/执行时长、错误率、并发度与队列长度曲线
- 可多次运行该脚本，在不同配置下对比指标趋势（如 FlushSize/Interval、自适应参数）

EOF

popd >/dev/null