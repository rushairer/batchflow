# Grafana Dashboard 更新指南

> 基于 BatchFlow v2.0 指标体系更新  
> 更新时间：2025-12-02

## 核心变更

### 标签名称变更

| 旧标签 | 新标签 | 说明 |
|-------|-------|------|
| `test_name` | `instance_id` | 更清晰的语义，支持多实例 |
| - | `status` | 新增：区分 success/fail |

### 新增指标

#### 1. 执行耗时（按状态）
- **指标**：`batchflow_execute_duration_seconds`
- **新增标签**：`status` (success/fail)
- **用途**：区分成功和失败的执行耗时

#### 2. 管道级指标
- `batchflow_pipeline_dequeue_latency_seconds`
- `batchflow_pipeline_process_duration_seconds{status}`
- `batchflow_pipeline_dropped_total{reason}`

## 必需的查询更新

### 1. 变量定义

**Dashboard Variables**：

```json
{
  "name": "instance_id",
  "label": "实例",
  "type": "query",
  "query": "label_values(batchflow_execute_duration_seconds, instance_id)",
  "multi": true,
  "includeAll": true
}
```

### 2. 关键查询模板

#### 执行耗时（P99，按状态）

**成功执行**：
```promql
histogram_quantile(0.99,
  sum(rate(batchflow_execute_duration_seconds_bucket{
    database=~"$database",
    instance_id=~"$instance_id",
    status="success"
  }[5m])) by (database, instance_id, le)
)
```

**失败执行**：
```promql
histogram_quantile(0.99,
  sum(rate(batchflow_execute_duration_seconds_bucket{
    database=~"$database",
    instance_id=~"$instance_id",
    status="fail"
  }[5m])) by (database, instance_id, le)
)
```

#### 错误率（按类型）

**重试错误率**：
```promql
sum(rate(batchflow_errors_total{
  database=~"$database",
  instance_id=~"$instance_id",
  error_type=~"retry:.*"
}[5m])) by (database, instance_id, error_type)
```

**最终失败率**：
```promql
sum(rate(batchflow_errors_total{
  database=~"$database",
  instance_id=~"$instance_id",
  error_type=~"final:.*"
}[5m])) by (database, instance_id, error_type)
```

#### 队列和在途批次

```promql
# 队列长度
batchflow_pipeline_queue_length{
  database=~"$database",
  instance_id=~"$instance_id"
}

# 在途批次数
batchflow_inflight_batches{
  database=~"$database",
  instance_id=~"$instance_id"
}

# 执行器并发度
batchflow_executor_concurrency{
  database=~"$database",
  instance_id=~"$instance_id"
}
```

#### 管道级指标（新增）

**出队延迟（P99）**：
```promql
histogram_quantile(0.99,
  sum(rate(batchflow_pipeline_dequeue_latency_seconds_bucket{
    database=~"$database",
    instance_id=~"$instance_id"
  }[5m])) by (database, instance_id, le)
)
```

**管道处理耗时（按状态）**：
```promql
histogram_quantile(0.99,
  sum(rate(batchflow_pipeline_process_duration_seconds_bucket{
    database=~"$database",
    instance_id=~"$instance_id",
    status="success"
  }[5m])) by (database, instance_id, le)
)
```

**丢弃事件率**：
```promql
sum(rate(batchflow_pipeline_dropped_total{
  database=~"$database",
  instance_id=~"$instance_id"
}[5m])) by (database, instance_id, reason)
```

## 面板布局建议

### Row 1: 核心吞吐量指标
- **批次大小分布**（Histogram）
- **执行耗时 P99**（Time Series，按 status 分组）
- **错误率趋势**（Time Series，按 error_type 前缀分组）

### Row 2: 延迟分解
- **入队延迟 P99**
- **组装耗时 P99**
- **执行耗时 P99**（按 status）
- **出队延迟 P99**（新增）

### Row 3: 状态观测
- **队列长度**（Gauge）
- **在途批次数**（Gauge）
- **执行器并发度**（Gauge）
- **丢弃事件率**（Graph，新增）

### Row 4: 错误分析
- **重试错误分布**（Pie Chart，按 error_type）
- **最终失败分布**（Pie Chart，按 error_type）
- **错误趋势**（Time Series）

## 自动化更新脚本

```bash
#!/bin/bash
# 更新 dashboard JSON 中的标签引用

DASHBOARD_FILE="provisioning/dashboards/batchflow-performance.json"

# 备份原文件
cp "$DASHBOARD_FILE" "${DASHBOARD_FILE}.bak"

# 替换标签名称（使用 jq）
jq '
  walk(
    if type == "string" then
      gsub("test_name"; "instance_id")
    else
      .
    end
  )
' "${DASHBOARD_FILE}.bak" > "$DASHBOARD_FILE"

echo "✅ Dashboard 已更新: $DASHBOARD_FILE"
echo "💾 备份文件: ${DASHBOARD_FILE}.bak"
```

## 测试验证

### 1. 验证变量加载

访问 Grafana → 选择 dashboard → Settings → Variables

确认：
- ✅ `instance_id` 变量正确加载
- ✅ 可以看到所有测试实例（高吞吐量测试、并发工作线程测试等）

### 2. 验证面板查询

在任意面板点击 "Edit"：
- ✅ 查询不报错
- ✅ 图表有数据显示
- ✅ 可以通过 `instance_id` 过滤

### 3. 验证新增指标

检查以下面板是否正常：
- ✅ 执行耗时（success vs fail）
- ✅ 出队延迟
- ✅ 管道处理耗时
- ✅ 丢弃事件

## 常见问题

### Q: 为什么某些面板显示"No data"？

**A**: 可能原因：
1. 测试尚未运行，没有生成指标数据
2. 时间范围选择不当（调整为 Last 1 hour）
3. instance_id 过滤器选择错误（尝试选择 "All"）

### Q: 如何同时查看多个实例？

**A**: 在 `instance_id` 变量下拉框中：
- 勾选多个实例
- 或选择 "All" 查看所有实例

### Q: 如何导出/导入 dashboard？

**A**: 
```bash
# 导出
curl -u admin:admin http://localhost:3000/api/dashboards/uid/batchflow \
  | jq .dashboard > batchflow-performance.json

# 导入
curl -X POST -u admin:admin \
  -H "Content-Type: application/json" \
  -d @batchflow-performance.json \
  http://localhost:3000/api/dashboards/db
```

## 回滚方案

如果更新后出现问题，可以回滚到 v1.0：

```bash
# 恢复备份
cp provisioning/dashboards/batchflow-performance.json.bak \
   provisioning/dashboards/batchflow-performance.json

# 重启 Grafana
docker-compose restart grafana
```

## 参考资源

- [指标设计文档](../METRICS_DESIGN.md)
- [Grafana Query Editor](https://grafana.com/docs/grafana/latest/panels/query-a-data-source/)
- [PromQL 查询指南](https://prometheus.io/docs/prometheus/latest/querying/basics/)
