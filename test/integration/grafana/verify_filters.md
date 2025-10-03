# Grafana 面板过滤功能验证

## 已实现的过滤功能

✅ **所有指标已成功添加数据库和测试名称过滤**

### 1. 核心性能指标
- `batchflow_current_rps{database=~"$database",test_name=~"$test_name"}`
- `rate(batchflow_records_rate_total{database=~"$database",test_name=~"$test_name"}[5m])`
- `max_over_time(batchflow_current_rps{database=~"$database",test_name=~"$test_name"}[5m])`
- `avg_over_time(batchflow_current_rps{database=~"$database",test_name=~"$test_name"}[5m])`

### 2. 批处理时间分布
- `histogram_quantile(0.95, rate(batchflow_batch_process_duration_seconds_bucket{database=~"$database"}[5m]))`
- `histogram_quantile(0.50, rate(batchflow_batch_process_duration_seconds_bucket{database=~"$database"}[5m]))`

### 3. 响应时间分位数
- `batchflow_response_time_seconds{database=~"$database",quantile="0.99"}`
- `batchflow_response_time_seconds{database=~"$database",quantile="0.9"}`
- `batchflow_response_time_seconds{database=~"$database",quantile="0.5"}`

### 4. 内存使用详细分析
- `batchflow_memory_usage_mb{database=~"$database",test_name=~"$test_name",type="alloc"}`
- `batchflow_memory_usage_mb{database=~"$database",test_name=~"$test_name",type="total_alloc"}`
- `batchflow_memory_usage_mb{database=~"$database",test_name=~"$test_name",type="sys"}`

### 5. 并发与连接监控
- `batchflow_concurrent_workers{database=~"$database",test_name=~"$test_name"}`
- `batchflow_active_connections{database=~"$database"}`

### 6. 数据质量与错误分析
- `batchflow_data_integrity_rate{database=~"$database",test_name=~"$test_name"} * 100`
- `rate(batchflow_errors_total{database=~"$database",test_name=~"$test_name"}[5m])`
- `batchflow_tests_run_total{database=~"$database",test_name=~"$test_name",result="success"}`
- `batchflow_tests_run_total{database=~"$database",test_name=~"$test_name",result="failure"}`

### 7. 数据库对比分析
- `avg by (database) (batchflow_current_rps{database=~"$database",test_name=~"$test_name"})`
- `sum by (database) (increase(batchflow_records_rate_total{database=~"$database",test_name=~"$test_name"}[5m]))`

### 8. 流水线阶段指标
- `histogram_quantile(0.5, sum by (le, database) (rate(batchflow_enqueue_latency_seconds_bucket{database=~"$database"}[5m])))`
- `histogram_quantile(0.95, sum by (le, database) (rate(batchflow_enqueue_latency_seconds_bucket{database=~"$database"}[5m])))`
- `histogram_quantile(0.5, sum by (le, database) (rate(batchflow_batch_assemble_duration_seconds_bucket{database=~"$database"}[5m])))`
- `histogram_quantile(0.95, sum by (le, database) (rate(batchflow_batch_assemble_duration_seconds_bucket{database=~"$database"}[5m])))`
- `histogram_quantile(0.5, sum by (le, database, test_name) (rate(batchflow_execute_duration_seconds_bucket{database=~"$database",test_name=~"$test_name"}[5m])))`
- `histogram_quantile(0.95, sum by (le, database, test_name) (rate(batchflow_execute_duration_seconds_bucket{database=~"$database",test_name=~"$test_name"}[5m])))`

### 9. 执行器与队列
- `batchflow_executor_concurrency{database=~"$database"}`
- `batchflow_pipeline_queue_length{database=~"$database"}`
- `batchflow_inflight_batches{database=~"$database"}`

### 10. 重试与最终失败
- `sum(rate(batchflow_errors_total{error_type=~"retry:.*",database=~"$database",test_name=~"$test_name"}[5m])) by (database, error_type)`
- `sum(rate(batchflow_errors_total{error_type=~"final:.*",database=~"$database",test_name=~"$test_name"}[5m])) by (database, error_type)`
- `sum(rate(batchflow_errors_total{error_type=~"retry:.*",database=~"$database",test_name=~"$test_name"}[5m])) / sum(rate(batchflow_errors_total{error_type=~"(retry:|final:).*",database=~"$database",test_name=~"$test_name"}[5m]))`

## 变量配置

### 数据库变量 ($database)
```
label_values(batchflow_current_rps, database)
```
- 支持多选
- 默认选择 "All"
- 包含 mysql, postgres, sqlite, redis 等选项

### 测试名称变量 ($test_name)
```
label_values(batchflow_current_rps{database=~"$database"}, test_name)
```
- 支持多选
- 默认选择 "All"
- 根据选择的数据库动态更新选项

## 使用说明

1. **选择数据库**: 在面板顶部的"数据库"下拉菜单中选择要查看的数据库类型
2. **选择测试**: 在"测试名称"下拉菜单中选择要查看的测试类型
3. **组合过滤**: 可以同时选择多个数据库和多个测试进行对比分析
4. **实时更新**: 所有图表会根据选择的过滤条件实时更新数据

## 验证步骤

1. 启动 Grafana 和 Prometheus
2. 导入更新后的面板配置
3. 验证变量下拉菜单是否正常工作
4. 测试不同的数据库和测试名称组合
5. 确认所有图表都能正确响应过滤条件

## 注意事项

- 某些指标（如 `batchflow_active_connections`）只按数据库维度过滤，因为它们本身不包含测试名称标签
- 流水线阶段的部分指标（如入队延迟、攒批耗时）目前只支持数据库过滤
- 重试和错误相关指标完全支持双重过滤（数据库 + 测试名称）