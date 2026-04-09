# BatchFlow 集成测试

集成测试代码位于 `test/integration`，目标是验证多数据库端到端行为和监控面板，而不是作为业务侧首选接入示例。

## 常用命令

```bash
make docker-mysql-test
make docker-postgres-test
make docker-sqlite-test
make docker-redis-test
make docker-all-tests
```

本地运行：

```bash
cd test/integration
go run .
```

## 当前定位

- 验证真实数据库兼容性
- 验证性能和稳定性
- 验证 Prometheus / Grafana 仪表盘

## 不建议的用法

- 不建议把 `test/integration` 下的 reporter 当成业务接入首选。
- 业务侧推荐使用 `examples/metrics/prometheus`。

## 配置来源

- `.env.test`
- `docker-compose.integration.yml`
- `docker-compose.integration-with-monitoring.yml`

## 关注点

- 数据完整性
- 吞吐和尾延迟
- 批量大小与 flush 行为
- 错误通道和丢弃指标

## 相关文档

- [监控指南](monitoring.md)
- [Metrics 规格](metrics-spec.md)
