#!/usr/bin/env bash
set -euo pipefail

# 基准测试矩阵：在不同 FlushSize / FlushInterval 下运行 go benchmark，建立基线或对比报告。
# 依赖：Go >=1.21，项目已有 benchmark_test.go
# 用法：
#   chmod +x scripts/benchmark_matrix.sh
#   ./scripts/benchmark_matrix.sh
#
# 输出：reports/benchmarks/YYYYMMDD-HHMMSS/bench.txt

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TS="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="$ROOT_DIR/reports/benchmarks/$TS"
mkdir -p "$OUT_DIR"

# 可根据你的环境调整
BUFFER_SIZES=("1000")
FLUSH_SIZES=("50" "100" "200" "500")
FLUSH_INTERVALS=("50ms" "100ms" "200ms" "500ms")

echo "Benchmark start: $TS" | tee "$OUT_DIR/bench.txt"
echo "Go version: $(go version)" | tee -a "$OUT_DIR/bench.txt"
echo "Matrix: Buffer=${BUFFER_SIZES[*]} Flush=${FLUSH_SIZES[*]} Interval=${FLUSH_INTERVALS[*]}" | tee -a "$OUT_DIR/bench.txt"
echo "============================================================" | tee -a "$OUT_DIR/bench.txt"

pushd "$ROOT_DIR" >/dev/null

for buf in "${BUFFER_SIZES[@]}"; do
  for fs in "${FLUSH_SIZES[@]}"; do
    for fi in "${FLUSH_INTERVALS[@]}"; do
      echo "" | tee -a "$OUT_DIR/bench.txt"
      echo ">>> BufferSize=$buf FlushSize=$fs FlushInterval=$fi" | tee -a "$OUT_DIR/bench.txt"
      BUFFER_SIZE="$buf" FLUSH_SIZE="$fs" FLUSH_INTERVAL="$fi" \
        go test -bench . -benchmem -run ^$ ./... 2>&1 | tee -a "$OUT_DIR/bench.txt"
    done
  done
done

popd >/dev/null

echo ""
echo "Benchmark done. Report: $OUT_DIR/bench.txt"