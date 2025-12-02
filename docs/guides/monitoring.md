# BatchFlow 监控系统指南

## 📊 监控系统概览

BatchFlow 提供完整的监控解决方案，基于 Prometheus + Grafana 技术栈，实现实时性能监控、数据完整性验证和系统健康检查。

### 🏗️ 监控架构

```
BatchFlow Application
        ↓ (metrics)
   Prometheus Server  
        ↓ (query)
    Grafana Dashboard
        ↓ (alerts)
   Alert Manager
```

## 🚀 快速开始

### 1. 启动监控栈

```bash
# 使用 Docker Compose 启动完整监控环境
docker-compose -f docker-compose.integration.yml up -d

# 验证服务状态
docker-compose ps

# 访问服务
# Grafana: http://localhost:3000 (admin/admin)
# Prometheus: http://localhost:9090
```

### 2. 配置应用监控

```go
package main

import (
    "github.com/rushairer/batchflow"
    "github.com/rushairer/batchflow/drivers/mysql"
    "github.com/rushairer/batchflow/test/integration"
)

func main() {
    // 1. 创建 Prometheus 指标收集器
    prometheusMetrics := integration.NewPrometheusMetrics()
    
    // 2. 启动指标服务器
    go prometheusMetrics.StartServer(9090)
    defer prometheusMetrics.StopServer()
    
    // 3. 创建带监控的执行器
    executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
    metricsReporter := integration.NewPrometheusMetricsReporter(
        prometheusMetrics, "mysql", "production")
    executor = executor.WithMetricsReporter(metricsReporter)
    
    // 4. 创建 BatchFlow 实例
    batchFlow := batchflow.NewBatchFlow(ctx, 5000, 200, 100*time.Millisecond, executor)
    defer batchFlow.Close()
    
    // 5. 正常使用，指标自动收集
    // ...
}
```

## 📈 核心指标

### 性能指标

| 指标名称 | 类型 | 描述 | 标签 | 查询示例 |
|---------|------|------|------|----------|
| `batchflow_records_processed_total` | Counter | 已处理记录总数（按状态分类） | `database`, `instance_id`, `status` | `sum(batchflow_records_processed_total)` |
| `batchflow_records_rate_total` | Counter | 用于 RPS 计算的累计记录数 | `database`, `instance_id` | `rate(batchflow_records_rate_total[5m])` |
| `batchflow_current_rps` | Gauge | 当前每秒处理记录数（瞬时值） | `database`, `instance_id` | `batchflow_current_rps` |
| `batchflow_batch_execution_duration_ms` | Histogram | 批次执行耗时分布 | `database`, `table`, `instance_id`, `status` | `histogram_quantile(0.95, rate(batchflow_batch_execution_duration_ms_bucket[5m]))` |
| `batchflow_batch_size` | Histogram | 批次大小分布 | `database`, `table`, `instance_id` | `histogram_quantile(0.50, rate(batchflow_batch_size_bucket[5m]))` |

### 管道级指标

| 指标名称 | 类型 | 描述 | 标签 | 查询示例 |
|---------|------|------|------|----------|
| `batchflow_pipeline_dequeue_latency_seconds` | Histogram | 管道出队延迟 | `database`, `instance_id` | `histogram_quantile(0.95, rate(batchflow_pipeline_dequeue_latency_seconds_bucket[5m]))` |
| `batchflow_pipeline_process_duration_seconds` | Histogram | 管道处理耗时 | `database`, `instance_id`, `status` | `histogram_quantile(0.95, rate(batchflow_pipeline_process_duration_seconds_bucket[5m]))` |
| `batchflow_pipeline_dropped_total` | Counter | 管道丢弃计数 | `database`, `instance_id`, `reason` | `sum(rate(batchflow_pipeline_dropped_total[5m])) by (reason)` |

### 质量指标

| 指标名称 | 类型 | 描述 | 标签 |
|---------|------|------|------|
| `batchflow_data_integrity_rate` | Gauge | 数据完整性率 (0-1) | `database`, `instance_id` |
| `batchflow_error_rate` | Gauge | 错误率 (0-1) | `database`, `instance_id` |
| `batchflow_batch_success_total` | Counter | 成功批次总数 | `database`, `table`, `instance_id` |
| `batchflow_batch_failed_total` | Counter | 失败批次总数 | `database`, `table`, `instance_id` |

### 系统指标

| 指标名称 | 类型 | 描述 | 标签 |
|---------|------|------|------|
| `batchflow_memory_usage_bytes` | Gauge | 内存使用量 | `database`, `instance_id` |
| `batchflow_active_connections` | Gauge | 活跃连接数 | `database` |
| `batchflow_buffer_utilization` | Gauge | 缓冲区利用率 (0-1) | `database`, `instance_id` |

## 🔁 Retry 指标与查询示例

### 指标语义
- 可重试错误（retry:*）：当执行器将错误分类为可重试时计数递增（每次重试前一次）
- 最终失败（final:*）：达到最大尝试次数或被判定为不可重试时计数递增一次
- 执行耗时：批次执行耗时直方图包含所有尝试与退避时间（status=success/fail）

常见原因标签（reason）
- deadlock、lock_timeout、timeout、connection、io、context、non_retryable

提示：
- 默认分类器会将 context.Canceled 与 context.DeadlineExceeded 计入 final:context（不可重试）。
- 若使用处理器的 WithTimeoutCause 并在超时返回 cause（如 "execute batch timeout"），你可以在自定义分类器中将其归为 retry:processor_timeout，以区分并观测处理器内部的短暂性超时。

### PromQL 示例
```promql
# 重试速率（按表和原因）
sum(rate(batchflow_errors_total{type=~"retry:.*"}[5m])) by (table, type)

# 最终失败速率（按表和原因）
sum(rate(batchflow_errors_total{type=~"final:.*"}[5m])) by (table, type)

# 重试占比（近5分钟窗口）
sum(rate(batchflow_errors_total{type=~"retry:.*"}[5m]))
/
sum(rate(batchflow_errors_total{type=~"(retry:|final:).*"}[5m]))

# 95分位执行耗时（含重试与退避）
histogram_quantile(0.95, rate(batchflow_batch_execution_duration_ms_bucket[5m]))
```

### 可视化建议
- timeseries：retry:* 与 final:* 分别曲线，按 table/type 分组
- stat：近5分钟最终失败率
- timeseries：P95 执行耗时，结合队列/并发变化观察退避影响

## 🚀 开箱即用 Prometheus（示例）
- 示例代码：examples/metrics/prometheus
- 提供：
  - 指标注册与 HTTP /metrics 服务（prometheus_metrics.go）
  - 执行器/BatchFlow 对接的 Reporter（prometheus_reporter.go）
  - 单一 Grafana Dashboard（test/integration/grafana/provisioning/dashboards/batchflow-performance.json）
- 适用场景：希望"快速可视化 + 按需裁剪"的团队
- 使用步骤：
  1) NewMetrics + StartServer(2112)
  2) NewReporter(metrics, database, instanceID)
  3) executor.WithMetricsReporter(reporter) 并传入 NewBatchFlow
  4) 导入上述 Dashboard
- 标签说明：
  - database：数据库类型（mysql/postgres/sqlite/redis）
  - instance_id：实例标识，用于区分多个 BatchFlow 实例
    - 集成测试：使用测试名称（如 "高吞吐量测试"）
    - 生产环境：使用业务标识（如 "order_writer", "log_collector"）

提示：生产中建议按需配置 Namespace/ConstLabels/Buckets，并谨慎开启 table 维度，避免标签基数膨胀。

## 📊 Grafana 指标修复说明

### 已修复的指标问题

#### 1. 测试结果统计指标修复
**问题**：`increase(batchflow_tests_run_total{result="success"}[5m])` 查询无数据  
**修复**：改为直接查询累计值 `batchflow_tests_run_total{result="success"}`  
**原因**：避免时间窗口和采集频率导致的 `increase()` 计算问题

#### 2. 数据库吞吐量统计修复
**问题**：`sum by (database) (increase(batchflow_records_rate_total[5m]))` 查询不准确  
**修复**：改为 `sum by (database) (batchflow_records_processed_total{status="processed"})`  
**原因**：使用专门用于统计处理记录数的指标，而非用于 RPS 计算的 Counter

#### 3. 内存使用指标修复
**问题**：`batchflow_memory_usage_mb * 1024 * 1024` 单位转换错误  
**修复**：直接使用 `batchflow_memory_usage_mb`  
**原因**：指标已经是 MB 单位，无需再次转换

#### 4. RPS 指标修复（之前已修复）
**问题**：`batchflow_records_per_second_bucket` 使用了错误的 Histogram 类型  
**修复**：使用 `rate(batchflow_records_rate_total[5m])` 计算平均 RPS  
**原因**：Counter + rate() 是计算速率的正确方式

#### 5. 响应时间分位数修复（之前已修复）
**问题**：查询 `quantile="0.95"` 但配置中只有 0.5, 0.9, 0.99  
**修复**：使用实际可用的分位数 `quantile="0.99"`  
**原因**：确保查询与 Summary 指标配置匹配

#### 6. 测试名称标签不匹配修复
**问题**：`batchflow_current_rps` 等指标中英文测试名称（如 `batch_insert`）没有数据  
**修复**：将初始化的测试类型改为实际使用的中文名称  
**原因**：初始化使用英文名称，但实际测试使用中文名称，导致标签不匹配

**实际的实例标识**：
- 集成测试：使用测试名称
  - `"高吞吐量测试"` (原 `batch_insert`)
  - `"并发工作线程测试"` (原 `concurrent_workers`) 
  - `"大批次测试"` (原 `large_batch`)
  - `"内存压力测试"` (原 `stress_test`)
  - `"长时间运行测试"` (新增)
- 生产环境：使用业务标识
  - `"order_writer"` - 订单写入服务
  - `"log_collector"` - 日志收集服务
  - `"user_sync"` - 用户同步服务

### 正确的查询方法

```promql
# ✅ 正确的 RPS 查询
rate(batchflow_records_rate_total[5m])           # 平均 RPS
batchflow_current_rps                            # 瞬时 RPS

# ✅ 正确的处理记录数查询
sum by (database) (batchflow_records_processed_total{status="processed"})

# ✅ 正确的测试结果统计
batchflow_tests_run_total{result="success"}     # 成功测试总数
batchflow_tests_run_total{result="failure"}     # 失败测试总数

# ✅ 正确的内存使用查询
batchflow_memory_usage_mb{type="alloc"}         # 当前分配内存 (MB)
batchflow_memory_usage_mb{type="sys"}           # 系统内存 (MB)

# ✅ 正确的响应时间分位数
batchflow_response_time_seconds{quantile="0.99"} # 99th percentile
batchflow_response_time_seconds{quantile="0.9"}  # 90th percentile
batchflow_response_time_seconds{quantile="0.5"}  # 50th percentile (median)
```

## 🎛️ Grafana 面板配置

### Retry 指标仪表板（可直接导入）

将以下 JSON 保存为 dashboards/batchflow-retry.json 并在 Grafana 中导入。该面板包含：
- 重试速率（retry:*）与最终失败速率（final:*）
- 近 5 分钟重试占比（retry / (retry+final)）
- P95 执行耗时（含重试与退避）
- 可选变量：database 与 table

```json
{
  "title": "BatchFlow Retry 与执行耗时",
  "timezone": "browser",
  "editable": true,
  "schemaVersion": 36,
  "version": 1,
  "refresh": "10s",
  "tags": ["batchflow", "retry"],
  "time": { "from": "now-1h", "to": "now" },
  "templating": {
    "list": [
      {
        "name": "database",
        "label": "Database",
        "type": "query",
        "datasource": null,
        "query": "label_values(batchflow_errors_total, database)",
        "refresh": 2,
        "includeAll": true,
        "multi": true
      },
      {
        "name": "table",
        "label": "Table",
        "type": "query",
        "datasource": null,
        "query": "label_values(batchflow_errors_total, table)",
        "refresh": 2,
        "includeAll": true,
        "multi": true
      }
    ]
  },
  "panels": [
    {
      "type": "timeseries",
      "title": "重试速率（retry:*）",
      "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8},
      "targets": [
        {
          "expr": "sum(rate(batchflow_errors_total{type=~"retry:.*",database=~"$database",table=~"$table"}[5m])) by (table, type)",
          "legendFormat": "{{table}} {{type}}"
        }
      ]
    },
    {
      "type": "timeseries",
      "title": "最终失败速率（final:*）",
      "gridPos": {"x": 12, "y": 0, "w": 12, "h": 8},
      "targets": [
        {
          "expr": "sum(rate(batchflow_errors_total{type=~"final:.*",database=~"$database",table=~"$table"}[5m])) by (table, type)",
          "legendFormat": "{{table}} {{type}}"
        }
      ]
    },
    {
      "type": "stat",
      "title": "重试占比（近5m）",
      "gridPos": {"x": 0, "y": 8, "w": 6, "h": 6},
      "targets": [
        {
          "expr": "sum(rate(batchflow_errors_total{type=~"retry:.*",database=~"$database",table=~"$table"}[5m])) / sum(rate(batchflow_errors_total{type=~"(retry:|final:).*",database=~"$database",table=~"$table"}[5m]))",
          "legendFormat": "retry_ratio"
        }
      ],
      "options": {
        "reduceOptions": {"calcs": ["lastNotNull"]},
        "orientation": "horizontal",
        "colorMode": "value",
        "graphMode": "none",
        "justifyMode": "auto"
      },
      "fieldConfig": {
        "defaults": {
          "unit": "percentunit",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              {"color": "green", "value": 0},
              {"color": "yellow", "value": 0.2},
              {"color": "red", "value": 0.5}
            ]
          }
        }
      }
    },
    {
      "type": "timeseries",
      "title": "P95 执行耗时（含重试与退避）",
      "gridPos": {"x": 6, "y": 8, "w": 18, "h": 6},
      "targets": [
        {
          "expr": "histogram_quantile(0.95, rate(batchflow_batch_execution_duration_ms_bucket{database=~"$database"}[5m]))",
          "legendFormat": "P95 {{database}}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "ms"
        }
      }
    }
  ]
}
```

导入方式
```bash
curl -X POST \
  http://admin:admin@localhost:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -d @dashboards/batchflow-retry.json
```

### 主要面板

#### 1. 性能概览面板

```json
{
  "title": "BatchFlow 性能概览",
  "panels": [
    {
      "title": "当前 RPS（瞬时值）",
      "type": "stat",
      "targets": [
        {
          "expr": "sum(batchflow_current_rps) by (database)",
          "legendFormat": "{{database}}"
        }
      ]
    },
    {
      "title": "平均 RPS（5分钟）",
      "type": "stat",
      "targets": [
        {
          "expr": "sum(rate(batchflow_records_rate_total[5m])) by (database)",
          "legendFormat": "{{database}}"
        }
      ]
    },
    {
      "title": "累计处理记录数",
      "type": "stat", 
      "targets": [
        {
          "expr": "sum(batchflow_records_rate_total)",
          "legendFormat": "总记录数"
        }
      ]
    }
  ]
}
```

#### 2. 数据完整性面板

```json
{
  "title": "数据完整性监控",
  "panels": [
    {
      "title": "数据完整性率",
      "type": "table",
      "targets": [
        {
          "expr": "batchflow_data_integrity_rate * 100",
          "format": "table",
          "legendFormat": "{{database}} - {{test_name}}"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "percent",
          "thresholds": {
            "steps": [
              {"color": "red", "value": 0},
              {"color": "yellow", "value": 95},
              {"color": "green", "value": 99}
            ]
          }
        }
      }
    }
  ]
}
```

#### 3. 性能趋势面板

```json
{
  "title": "性能趋势分析",
  "panels": [
    {
      "title": "RPS 趋势",
      "type": "timeseries",
      "targets": [
        {
          "expr": "batchflow_current_rps",
          "legendFormat": "当前 RPS - {{database}} - {{test_name}}"
        },
        {
          "expr": "rate(batchflow_records_rate_total[5m])",
          "legendFormat": "平均 RPS (5m) - {{database}} - {{test_name}}"
        }
      ]
    },
    {
      "title": "批次执行耗时",
      "type": "timeseries", 
      "targets": [
        {
          "expr": "histogram_quantile(0.95, batchflow_batch_execution_duration_ms_bucket)",
          "legendFormat": "P95 - {{database}}"
        },
        {
          "expr": "histogram_quantile(0.50, batchflow_batch_execution_duration_ms_bucket)",
          "legendFormat": "P50 - {{database}}"
        }
      ]
    }
  ]
}
```

### 完整面板导入

```bash
# 导入预配置的 Grafana 面板
curl -X POST \
  http://admin:admin@localhost:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -d @test/integration/grafana/provisioning/dashboards/batchflow-performance.json
```

## 🔍 监控查询示例

### Prometheus 查询语句

#### 性能分析查询

```promql
# 各数据库的当前 RPS（瞬时值）
avg(batchflow_current_rps) by (database)

# 最近5分钟的平均 RPS（推荐用于趋势分析）
rate(batchflow_records_rate_total[5m])

# 最近1分钟的 RPS（更敏感的监控）
rate(batchflow_records_rate_total[1m])

# 各数据库的 RPS 对比
sum(rate(batchflow_records_rate_total[5m])) by (database)

# 批次执行耗时的95分位数
histogram_quantile(0.95, rate(batchflow_batch_execution_duration_ms_bucket[5m]))

# 错误率趋势
rate(batchflow_batch_failed_total[5m]) / rate(batchflow_batch_success_total[5m] + batchflow_batch_failed_total[5m])
```

#### 容量规划查询

```promql
# 内存使用趋势
batchflow_memory_usage_mb  # 已经是 MB 单位，无需转换

# 缓冲区利用率
avg(batchflow_buffer_utilization) by (database, instance_id)

# 连接池使用情况
batchflow_active_connections / on(database) group_left() max_connections
```

#### 数据质量查询

```promql
# 数据完整性低于99%的测试
batchflow_data_integrity_rate < 0.99

# 各数据库的数据完整性对比
batchflow_data_integrity_rate * 100

# 数据完整性变化趋势
delta(batchflow_data_integrity_rate[1h])
```

## 🚨 告警配置

### Prometheus 告警规则

```yaml
# prometheus/alert_rules.yml
groups:
  - name: batchflow_alerts
    rules:
      - alert: BatchFlowHighErrorRate
        expr: rate(batchflow_batch_failed_total[5m]) / rate(batchflow_batch_success_total[5m] + batchflow_batch_failed_total[5m]) > 0.05
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "BatchFlow 错误率过高"
          description: "数据库 {{ $labels.database }} 的错误率为 {{ $value | humanizePercentage }}"
      
      - alert: BatchFlowLowDataIntegrity
        expr: batchflow_data_integrity_rate < 0.95
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "BatchFlow 数据完整性异常"
          description: "测试 {{ $labels.test_name }} 的数据完整性仅为 {{ $value | humanizePercentage }}"
      
      - alert: BatchFlowLowPerformance
        expr: batchflow_current_rps < 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "BatchFlow 性能下降"
          description: "数据库 {{ $labels.database }} 的 RPS 降至 {{ $value }}"
      
      - alert: BatchFlowHighMemoryUsage
        expr: batchflow_memory_usage_bytes > 1073741824  # 1GB
        for: 3m
        labels:
          severity: warning
        annotations:
          summary: "BatchFlow 内存使用过高"
          description: "内存使用量达到 {{ $value | humanizeBytes }}"
```

### Grafana 告警配置

```json
{
  "alert": {
    "name": "数据完整性告警",
    "message": "BatchFlow 数据完整性低于阈值",
    "frequency": "10s",
    "conditions": [
      {
        "query": {
          "queryType": "",
          "refId": "A",
          "model": {
            "expr": "batchflow_data_integrity_rate * 100",
            "interval": "",
            "legendFormat": "",
            "refId": "A"
          }
        },
        "reducer": {
          "type": "last",
          "params": []
        },
        "evaluator": {
          "params": [95],
          "type": "lt"
        }
      }
    ],
    "executionErrorState": "alerting",
    "noDataState": "no_data",
    "for": "1m"
  }
}
```

## 🔧 高级配置

### 自定义指标收集器

```go
type CustomMetricsCollector struct {
    prometheus *PrometheusMetrics
    database   string
    instanceID string
    
    // 自定义指标
    customCounter   prometheus.Counter
    customHistogram prometheus.Histogram
}

func NewCustomMetricsCollector(pm *PrometheusMetrics, database, testName string) *CustomMetricsCollector {
    collector := &CustomMetricsCollector{
        prometheus: pm,
        database:   database,
        testName:   testName,
    }
    
    // 注册自定义指标
    collector.customCounter = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "batchflow_custom_operations_total",
        Help: "Total number of custom operations",
    })
    
    collector.customHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "batchflow_custom_duration_seconds",
        Help:    "Duration of custom operations",
        Buckets: prometheus.DefBuckets,
    })
    
    prometheus.MustRegister(collector.customCounter, collector.customHistogram)
    
    return collector
}

func (c *CustomMetricsCollector) RecordCustomOperation(duration time.Duration) {
    c.customCounter.Inc()
    c.customHistogram.Observe(duration.Seconds())
}
```

### 多环境监控配置

```go
type EnvironmentConfig struct {
    Name              string
    PrometheusPort    int
    GrafanaURL        string
    AlertManagerURL   string
    MetricsPrefix     string
}

func SetupMonitoringForEnvironment(env EnvironmentConfig) *PrometheusMetrics {
    // 创建带环境标识的指标收集器
    prometheusMetrics := NewPrometheusMetrics()
    prometheusMetrics.SetEnvironment(env.Name)
    prometheusMetrics.SetMetricsPrefix(env.MetricsPrefix)
    
    // 启动指标服务器
    go prometheusMetrics.StartServer(env.PrometheusPort)
    
    // 配置告警
    if env.AlertManagerURL != "" {
        prometheusMetrics.ConfigureAlertManager(env.AlertManagerURL)
    }
    
    return prometheusMetrics
}

// 使用示例
func main() {
    envs := []EnvironmentConfig{
        {Name: "production", PrometheusPort: 9090, MetricsPrefix: "prod_"},
        {Name: "staging", PrometheusPort: 9091, MetricsPrefix: "staging_"},
        {Name: "development", PrometheusPort: 9092, MetricsPrefix: "dev_"},
    }
    
    for _, env := range envs {
        metrics := SetupMonitoringForEnvironment(env)
        defer metrics.StopServer()
    }
}
```

## 📊 监控最佳实践

### 1. 指标命名规范

```go
// ✅ 好的命名
batchflow_records_processed_total
batchflow_batch_execution_duration_ms
batchflow_data_integrity_rate

// ❌ 避免的命名
records_count
duration
integrity
```

### 2. 标签使用原则

```go
// ✅ 合理的标签
labels := map[string]string{
    "database":  "mysql",      // 数据库类型
    "table":     "users",      // 表名
    "instance_id": "batch_insert", // 实例标识
    "env":       "production", // 环境
}

// ❌ 避免高基数标签
labels := map[string]string{
    "record_id": "12345",     // 会产生大量时间序列
    "timestamp": "1609459200", // 时间戳不应作为标签
}
```

### 3. 监控数据保留策略

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "alert_rules.yml"

scrape_configs:
  - job_name: 'batchflow'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 5s
    metrics_path: /metrics

# 数据保留配置
storage:
  tsdb:
    retention.time: 30d
    retention.size: 10GB
```

### 4. 性能优化建议

```go
// 批量更新指标，减少锁竞争
func (pm *PrometheusMetrics) BatchUpdateMetrics(updates []MetricUpdate) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    
    for _, update := range updates {
        switch update.Type {
        case "counter":
            pm.counters[update.Name].Add(update.Value)
        case "gauge":
            pm.gauges[update.Name].Set(update.Value)
        case "histogram":
            pm.histograms[update.Name].Observe(update.Value)
        }
    }
}

// 使用缓冲区减少指标更新频率
type MetricsBuffer struct {
    updates []MetricUpdate
    size    int
    mu      sync.Mutex
}

func (mb *MetricsBuffer) Add(update MetricUpdate) {
    mb.mu.Lock()
    defer mb.mu.Unlock()
    
    mb.updates = append(mb.updates, update)
    
    if len(mb.updates) >= mb.size {
        mb.flush()
    }
}
```

## 🔍 故障排查

### 常见问题及解决方案

#### 1. 指标数据缺失

**症状**：Grafana 面板显示 "No data"

**排查步骤**：
```bash
# 检查 Prometheus 目标状态
curl http://localhost:9090/api/v1/targets

# 检查指标是否存在
curl http://localhost:9090/api/v1/label/__name__/values | grep batchflow

# 检查应用指标端点
curl http://localhost:9090/metrics | grep batchflow
```

**解决方案**：
- 确认应用正确启动指标服务器
- 检查防火墙和网络连接
- 验证 Prometheus 配置文件

#### 2. 数据完整性指标异常

**症状**：显示 10000% 或其他异常值

**排查步骤**：
```bash
# 检查原始指标值
curl -s http://localhost:9090/api/v1/query?query=batchflow_data_integrity_rate

# 检查 Grafana 查询表达式
# 应该是：batchflow_data_integrity_rate * 100
# 而不是：batchflow_data_integrity_rate * 10000
```

**解决方案**：
- 确认指标范围为 0-1
- 修正 Grafana 查询表达式
- 检查应用中的指标计算逻辑

#### 3. 性能指标不准确

**症状**：RPS 显示异常高或异常低

**排查步骤**：
```promql
# 检查计数器增长率
rate(batchflow_records_processed_total[1m])

# 查看累计处理记录数（推荐）
sum by (database) (batchflow_records_processed_total{status="processed"})

# 如需查看时间窗口内的增量
increase(batchflow_records_processed_total[5m])
```

**解决方案**：
- 调整 PromQL 查询的时间窗口
- 确认计数器正确递增
- 检查系统时钟同步

## 📚 相关文档

- [API_REFERENCE.md](API_REFERENCE.md) - WithMetricsReporter 详细用法
- [EXAMPLES.md](EXAMPLES.md) - 监控集成示例
- [TESTING_GUIDE.md](TESTING_GUIDE.md) - 监控测试方法
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 详细故障排查

---

💡 **监控建议**：
1. 从核心指标开始，逐步扩展监控范围
2. 设置合理的告警阈值，避免告警疲劳
3. 定期审查和优化监控配置
4. 建立监控数据的备份和恢复机制