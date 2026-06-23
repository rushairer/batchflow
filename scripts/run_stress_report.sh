#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TS="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="$ROOT_DIR/reports/stress/$TS"
RAW_DIR="$OUT_DIR/raw"
REPORT="$OUT_DIR/stress-report.md"
COMPOSE_FILE="$ROOT_DIR/docker-compose.integration.yml"
OVERRIDE_FILE="$OUT_DIR/docker-compose.stress.override.yml"

BACKENDS=("mysql" "postgres" "redis")
INCLUDE_SQLITE=false
RUN_TESTS=true

usage() {
  cat <<EOF
Usage: $0 [--backend mysql|postgres|redis|sqlite]... [--include-sqlite] [--collect-only] [--help]

Environment overrides:
  TEST_DURATION       default: 120s
  CONCURRENT_WORKERS  default: 10
  RECORDS_PER_WORKER  default: 5000
  BATCH_SIZE          default: 500
  BUFFER_SIZE         default: 10000
  FLUSH_INTERVAL      default: 50ms

Output:
  reports/stress/<timestamp>/stress-report.md
  reports/stress/<timestamp>/raw/
EOF
}

custom_backends=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --backend)
      [[ $# -ge 2 ]] || { echo "--backend requires a value" >&2; exit 1; }
      if [[ "$custom_backends" == false ]]; then
        BACKENDS=()
        custom_backends=true
      fi
      BACKENDS+=("$2")
      shift 2
      ;;
    --include-sqlite)
      INCLUDE_SQLITE=true
      shift
      ;;
    --collect-only)
      RUN_TESTS=false
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "$INCLUDE_SQLITE" == true ]]; then
  BACKENDS+=("sqlite")
fi

mkdir -p "$RAW_DIR"

export TEST_DURATION="${TEST_DURATION:-120s}"
export CONCURRENT_WORKERS="${CONCURRENT_WORKERS:-10}"
export RECORDS_PER_WORKER="${RECORDS_PER_WORKER:-5000}"
export BATCH_SIZE="${BATCH_SIZE:-500}"
export BUFFER_SIZE="${BUFFER_SIZE:-10000}"
export FLUSH_INTERVAL="${FLUSH_INTERVAL:-50ms}"

cat > "$OVERRIDE_FILE" <<EOF
services:
  mysql-test:
    environment:
      TEST_DURATION: "${TEST_DURATION}"
      CONCURRENT_WORKERS: "${CONCURRENT_WORKERS}"
      RECORDS_PER_WORKER: "${RECORDS_PER_WORKER}"
      BATCH_SIZE: "${BATCH_SIZE}"
      BUFFER_SIZE: "${BUFFER_SIZE}"
      FLUSH_INTERVAL: "${FLUSH_INTERVAL}"
  postgres-test:
    environment:
      TEST_DURATION: "${TEST_DURATION}"
      CONCURRENT_WORKERS: "${CONCURRENT_WORKERS}"
      RECORDS_PER_WORKER: "${RECORDS_PER_WORKER}"
      BATCH_SIZE: "${BATCH_SIZE}"
      BUFFER_SIZE: "${BUFFER_SIZE}"
      FLUSH_INTERVAL: "${FLUSH_INTERVAL}"
  redis-test:
    environment:
      TEST_DURATION: "${TEST_DURATION}"
      CONCURRENT_WORKERS: "${CONCURRENT_WORKERS}"
      RECORDS_PER_WORKER: "${RECORDS_PER_WORKER}"
      BATCH_SIZE: "${BATCH_SIZE}"
      BUFFER_SIZE: "${BUFFER_SIZE}"
      FLUSH_INTERVAL: "${FLUSH_INTERVAL}"
  sqlite-test:
    environment:
      TEST_DURATION: "${TEST_DURATION}"
      CONCURRENT_WORKERS: "${CONCURRENT_WORKERS}"
      RECORDS_PER_WORKER: "${RECORDS_PER_WORKER}"
      BATCH_SIZE: "${BATCH_SIZE}"
      BUFFER_SIZE: "${BUFFER_SIZE}"
      FLUSH_INTERVAL: "${FLUSH_INTERVAL}"
EOF

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }
command -v ruby >/dev/null 2>&1 || { echo "ruby is required to summarize JSON reports" >&2; exit 1; }

commit_sha="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
go_version="$(go version 2>/dev/null || echo unknown)"
docker_version="$(docker --version 2>/dev/null || echo unknown)"
os_name="$(uname -a 2>/dev/null || echo unknown)"
cpu_info="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || lscpu 2>/dev/null | awk -F: '/Model name/ {gsub(/^[ \t]+/, "", $2); print $2; exit}' || echo unknown)"
memory_info="$(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%.2f GB", $1/1024/1024/1024}' || free -h 2>/dev/null | awk '/Mem:/ {print $2}' || echo unknown)"

run_backend() {
  local backend="$1"
  local target
  case "$backend" in
    mysql) target="docker-mysql-test" ;;
    postgres) target="docker-postgres-test" ;;
    redis) target="docker-redis-test" ;;
    sqlite) target="docker-sqlite-test" ;;
    *) echo "unsupported backend: $backend" >&2; exit 1 ;;
  esac

  local before_file="$OUT_DIR/${backend}.before"
  local after_file="$OUT_DIR/${backend}.after"
  find "$ROOT_DIR/test/reports" -maxdepth 1 -type f -name 'integration_test_report_*' -print 2>/dev/null | sort > "$before_file" || true

  echo "==> Running $backend stress test with $target"
  (
    cd "$ROOT_DIR"
    docker compose -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE" down "$backend" "$backend-test" -v --remove-orphans
    docker compose -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE" build "$backend" "$backend-test" --no-cache
    docker compose -f "$COMPOSE_FILE" -f "$OVERRIDE_FILE" up "$backend" "$backend-test" --abort-on-container-exit --exit-code-from "$backend-test"
  )

  find "$ROOT_DIR/test/reports" -maxdepth 1 -type f -name 'integration_test_report_*' -print 2>/dev/null | sort > "$after_file" || true
  comm -13 "$before_file" "$after_file" | while IFS= read -r file; do
    [[ -n "$file" ]] || continue
    cp "$file" "$RAW_DIR/${backend}_$(basename "$file")"
  done
}

if [[ "$RUN_TESTS" == true ]]; then
  for backend in "${BACKENDS[@]}"; do
    run_backend "$backend"
  done
else
  echo "==> Collect-only mode: using existing test/reports files"
  find "$ROOT_DIR/test/reports" -maxdepth 1 -type f -name 'integration_test_report_*' -print 2>/dev/null | sort | while IFS= read -r file; do
    cp "$file" "$RAW_DIR/$(basename "$file")"
  done
fi

ruby -rjson -rtime -e '
report = ARGV[0]
raw_dir = ARGV[1]
context = {
  "timestamp" => ARGV[2],
  "commit" => ARGV[3],
  "go" => ARGV[4],
  "docker" => ARGV[5],
  "os" => ARGV[6],
  "cpu" => ARGV[7],
  "memory" => ARGV[8],
  "duration" => ENV.fetch("TEST_DURATION"),
  "workers" => ENV.fetch("CONCURRENT_WORKERS"),
  "records_per_worker" => ENV.fetch("RECORDS_PER_WORKER"),
  "batch_size" => ENV.fetch("BATCH_SIZE"),
  "buffer_size" => ENV.fetch("BUFFER_SIZE"),
  "flush_interval" => ENV.fetch("FLUSH_INTERVAL")
}

json_files = Dir.glob(File.join(raw_dir, "*.json")).sort
rows = []
json_files.each do |file|
  data = JSON.parse(File.read(file))
  data.fetch("results", []).each do |result|
    rows << {
      file: File.basename(file),
      database: result["database"],
      test_name: result["test_name"],
      success: result["success"],
      total_records: result["total_records"],
      actual_records: result["actual_records"],
      integrity: result["data_integrity_rate"],
      rps: result["records_per_second"],
      rps_valid: result["rps_valid"],
      duration_ns: result["duration"],
      workers: result["concurrent_workers"],
      batch_size: result.dig("test_parameters", "batch_size"),
      buffer_size: result.dig("test_parameters", "buffer_size"),
      flush_interval_ns: result.dig("test_parameters", "flush_interval"),
      errors: Array(result["errors"]).size,
      alloc_mb: result.dig("memory_usage", "alloc_mb"),
      total_alloc_mb: result.dig("memory_usage", "total_alloc_mb"),
      sys_mb: result.dig("memory_usage", "sys_mb"),
      gc: result.dig("memory_usage", "num_gc")
    }
  end
end

def ns_duration(value)
  return "n/a" if value.nil?
  seconds = value.to_f / 1_000_000_000
  if seconds >= 60
    format("%.2fm", seconds / 60)
  else
    format("%.2fs", seconds)
  end
end

def bool_status(value)
  value ? "pass" : "fail"
end

File.open(report, "w") do |f|
  f.puts "# BatchFlow Stress Test Report"
  f.puts
  f.puts "- Timestamp: #{context["timestamp"]}"
  f.puts "- Commit: #{context["commit"]}"
  f.puts "- Go: #{context["go"]}"
  f.puts "- Docker: #{context["docker"]}"
  f.puts "- OS: #{context["os"]}"
  f.puts "- CPU: #{context["cpu"]}"
  f.puts "- Memory: #{context["memory"]}"
  f.puts
  f.puts "## Parameters"
  f.puts
  f.puts "| Parameter | Value |"
  f.puts "| --- | --- |"
  f.puts "| TEST_DURATION | #{context["duration"]} |"
  f.puts "| CONCURRENT_WORKERS | #{context["workers"]} |"
  f.puts "| RECORDS_PER_WORKER | #{context["records_per_worker"]} |"
  f.puts "| BATCH_SIZE | #{context["batch_size"]} |"
  f.puts "| BUFFER_SIZE | #{context["buffer_size"]} |"
  f.puts "| FLUSH_INTERVAL | #{context["flush_interval"]} |"
  f.puts
  f.puts "## Summary"
  f.puts
  if rows.empty?
    f.puts "No JSON reports were collected."
  else
    total = rows.size
    passed = rows.count { |r| r[:success] }
    max_rps = rows.map { |r| r[:rps].to_f }.max || 0.0
    avg_rps = rows.empty? ? 0.0 : rows.sum { |r| r[:rps].to_f } / rows.size
    f.puts "- Total result rows: #{total}"
    f.puts "- Passed: #{passed}"
    f.puts "- Failed: #{total - passed}"
    f.puts "- Average RPS: #{format("%.2f", avg_rps)}"
    f.puts "- Max RPS: #{format("%.2f", max_rps)}"
  end
  f.puts
  f.puts "## Backend Results"
  f.puts
  f.puts "| Backend | Test | Status | Submitted | Actual | Integrity | RPS | RPS Valid | Duration | Workers | Batch | Buffer | Errors | Alloc MB | Sys MB | GC |"
  f.puts "| --- | --- | --- | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |"
  rows.each do |r|
    f.puts "| #{r[:database]} | #{r[:test_name]} | #{bool_status(r[:success])} | #{r[:total_records]} | #{r[:actual_records]} | #{format("%.2f", r[:integrity].to_f)}% | #{format("%.2f", r[:rps].to_f)} | #{r[:rps_valid]} | #{ns_duration(r[:duration_ns])} | #{r[:workers]} | #{r[:batch_size]} | #{r[:buffer_size]} | #{r[:errors]} | #{format("%.2f", r[:alloc_mb].to_f)} | #{format("%.2f", r[:sys_mb].to_f)} | #{r[:gc]} |"
  end
  f.puts
  f.puts "## Raw Reports"
  f.puts
  json_files.each do |file|
    f.puts "- `raw/#{File.basename(file)}`"
  end
  f.puts
  f.puts "## Interpretation Rules"
  f.puts
  f.puts "- MySQL, PostgreSQL, and Redis are gating stress backends."
  f.puts "- SQLite is optional and non-gating for high-concurrency write stress."
  f.puts "- RPS should only be compared when data integrity is 100% and `RPS Valid` is true."
  f.puts "- Inspect raw JSON/HTML reports when errors are non-zero or integrity is below 100%."
end
' "$REPORT" "$RAW_DIR" "$TS" "$commit_sha" "$go_version" "$docker_version" "$os_name" "$cpu_info" "$memory_info"

rm -f "$OUT_DIR"/*.before "$OUT_DIR"/*.after

echo "Stress report generated: $REPORT"
