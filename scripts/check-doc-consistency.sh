#!/usr/bin/env bash

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DOCS=(
  "$ROOT/README.md"
  "$ROOT/README.zh-CN.md"
  "$ROOT/docs/index.md"
  "$ROOT/docs/api/reference.md"
  "$ROOT/docs/api/configuration.md"
  "$ROOT/docs/guides/examples.md"
  "$ROOT/docs/guides/production.md"
  "$ROOT/docs/guides/testing.md"
  "$ROOT/docs/guides/monitoring.md"
  "$ROOT/docs/guides/monitoring-quickstart.md"
  "$ROOT/docs/guides/custom-metrics-reporter.md"
  "$ROOT/docs/guides/go-pipeline-metrics.md"
  "$ROOT/docs/guides/metrics-spec.md"
  "$ROOT/examples/metrics/prometheus/README.md"
)

for doc in "${DOCS[@]}"; do
  [[ -f "$doc" ]] || { echo "missing doc: $doc" >&2; exit 1; }
done

forbidden_patterns=(
  'github.com/rushairer/batchflow/drivers/'
  'github.com/rushairer/batchflow"'
  '\.\(\*batchflow\.ThrottledBatchExecutor\)'
  'go-pipeline v2\.2\.0'
  'batchflow_batch_execution_duration_ms'
  'batchflow_records_processed_total'
  'batchflow_current_rps'
  'test_name'
)

for pattern in "${forbidden_patterns[@]}"; do
  if rg -n "$pattern" "${DOCS[@]}" >/dev/null; then
    echo "forbidden stale doc pattern found: $pattern" >&2
    rg -n "$pattern" "${DOCS[@]}" >&2
    exit 1
  fi
done

required_patterns=(
  'Close\(\)'
  'Done\(\)'
  'pipeline_flush_size'
  'submit_rejected_total'
  'GenerateSQLPreview'
  'RegisterErrorClassifier'
  'ConflictColumns'
  'Coalescer'
)

for pattern in "${required_patterns[@]}"; do
  if ! rg -n "$pattern" "$ROOT/README.md" "$ROOT/docs/api/reference.md" "$ROOT/docs/guides/metrics-spec.md" >/dev/null; then
    echo "required contract pattern missing: $pattern" >&2
    exit 1
  fi
done

request_contract_docs=(
  "$ROOT/README.md"
  "$ROOT/docs/api/reference.md"
  "$ROOT/docs/guides/examples.md"
)

request_required_patterns=(
  'SetUint64'
  'SetInt'
)

for pattern in "${request_required_patterns[@]}"; do
  if ! rg -n "$pattern" "${request_contract_docs[@]}" >/dev/null; then
    echo "required request contract pattern missing: $pattern" >&2
    exit 1
  fi
done

install_docs=(
  "$ROOT/README.md"
  "$ROOT/README.zh-CN.md"
)

for doc in "${install_docs[@]}"; do
  if ! rg -n 'go get github.com/rushairer/batchflow/v2' "$doc" >/dev/null; then
    echo "v2 install command missing: $doc" >&2
    exit 1
  fi
done

english_docs=(
  "$ROOT/README.md"
  "$ROOT/docs/index.md"
  "$ROOT/docs/api/configuration.md"
  "$ROOT/docs/guides/examples.md"
  "$ROOT/docs/guides/production.md"
  "$ROOT/docs/guides/testing.md"
)

for doc in "${english_docs[@]}"; do
  if rg -n '[一-龥]' "$doc" >/dev/null; then
    echo "canonical English doc contains CJK text: $doc" >&2
    rg -n '[一-龥]' "$doc" >&2
    exit 1
  fi
done

echo "docs consistency check passed"
