# BatchFlow Makefile
# 提供便捷的开发和测试命令

.PHONY: \
  help build test test-race test-integration test-integration-with-monitoring \
  docker-mysql-test docker-postgres-test docker-sqlite-test docker-redis-test docker-all-tests \
  docker-mysql-test-with-monitoring docker-postgres-test-with-monitoring docker-sqlite-test-with-monitoring docker-redis-test-with-monitoring docker-all-tests-with-monitoring \
  deps deps-update \
  monitoring monitoring-foreground monitoring-stop monitoring-status monitoring-logs monitoring-cleanup \
  dev-setup fmt lint clean clean-all benchmark docs docs-check ci release-check docker-build docker-test quick-test full-test dev info cover

# 默认目标
help: ## 显示帮助信息
	@echo "BatchFlow 项目管理命令"
	@echo ""
	@echo "🔨 构建与测试:"
	@echo "  \033[36mbuild\033[0m                 构建项目"
	@echo "  \033[36mtest\033[0m                  运行单元测试"
	@echo "  \033[36mtest-race\033[0m             运行竞态检测测试"
	@echo "  \033[36mbenchmark\033[0m             运行性能基准测试"
	@echo "  \033[36mcover\033[0m                 运行覆盖率（排除 test/ 包）"
	@echo ""
	@echo "🔬 集成测试（本地）:"
	@echo "  \033[36mtest-integration\033[0m      运行本地集成测试"
	@echo "  \033[36mtest-integration-with-monitoring\033[0m 本地集成测试 + 监控"
	@echo ""
	@echo "🐳 Docker 集成测试:"
	@echo "  \033[36mdocker-all-tests\033[0m      运行所有数据库 Docker 测试"
	@echo "  \033[36mdocker-mysql-test\033[0m     运行 MySQL Docker 测试"
	@echo "  \033[36mdocker-postgres-test\033[0m  运行 PostgreSQL Docker 测试"
	@echo "  \033[36mdocker-sqlite-test\033[0m    运行 SQLite Docker 测试"
	@echo "  \033[36mdocker-redis-test\033[0m     运行 Redis Docker 测试"
	@echo ""
	@echo "🐳📊 Docker 测试 + 监控:"
	@echo "  \033[36mdocker-all-tests-with-monitoring\033[0m      所有数据库 + 监控"
	@echo "  \033[36mdocker-mysql-test-with-monitoring\033[0m     MySQL + 监控"
	@echo "  \033[36mdocker-postgres-test-with-monitoring\033[0m  PostgreSQL + 监控"
	@echo "  \033[36mdocker-sqlite-test-with-monitoring\033[0m    SQLite + 监控"
	@echo "  \033[36mdocker-redis-test-with-monitoring\033[0m     Redis + 监控"
	@echo ""
	@echo "📊 监控相关（本地开发）:"
	@echo "  \033[36mmonitoring\033[0m            后台启动监控（Prometheus + Grafana）"
	@echo "  \033[36mmonitoring-foreground\033[0m 前台启动监控环境"
	@echo "  \033[36mmonitoring-stop\033[0m       停止监控服务"
	@echo "  \033[36mmonitoring-status\033[0m     查看监控服务状态"
	@echo "  \033[36mmonitoring-logs\033[0m       查看监控服务日志"
	@echo "  \033[36mmonitoring-cleanup\033[0m    清理并重启监控服务"
	@echo ""
	@echo "🛠️ 依赖与维护:"
	@echo "  \033[36mdeps\033[0m                 安装依赖（download + tidy）"
	@echo "  \033[36mdeps-update\033[0m          更新依赖（go get -u + tidy）"
	@echo "  \033[36mfmt\033[0m                  格式化代码"
	@echo "  \033[36mlint\033[0m                 运行代码检查"
	@echo "  \033[36mclean\033[0m                清理构建文件与缓存"
	@echo "  \033[36mclean-all\033[0m            完全清理（含 docker system prune）"
	@echo ""
	@echo "🧪 快捷命令:"
	@echo "  \033[36mquick-test\033[0m            快速测试（fmt + unit tests）"
	@echo "  \033[36mfull-test\033[0m             完整测试（fmt + unit + race + integration）"
	@echo "  \033[36mdev\033[0m                   开发模式（启动监控 + 集成测试）"
	@echo ""
	@echo "🚀 CI/CD 与发布:"
	@echo "  \033[36mci\033[0m                   本地模拟 CI（deps + fmt + lint + tests）"
	@echo "  \033[36mrelease-check\033[0m         发布前检查"
	@echo "  \033[36mdocker-build\033[0m          构建 Docker 镜像"
	@echo "  \033[36mdocker-test\033[0m           在 Docker 中运行测试"
	@echo ""
	@echo "ℹ️ 其他:"
	@echo "  \033[36mdev-setup\033[0m             设置开发环境"
	@echo "  \033[36mdocs\033[0m                  生成/预览文档"
	@echo "  \033[36mdocs-check\033[0m            检查关键文档是否引用过期 API/指标"
	@echo "  \033[36minfo\033[0m                  显示项目信息"

# 构建相关
build: ## 构建项目
	@echo "🔨 构建 BatchFlow..."
	@go build ./...

test: ## 运行单元测试
	@echo "🧪 运行单元测试..."
	@go test ./...

test-race: ## 运行竞态检测测试
	@echo "🏃 运行竞态检测测试..."
	@go test -race ./...

# 覆盖率（排除 test/ 包）
cover: ## 运行覆盖率并输出 coverage.txt、coverage_total.txt（排除 test/ 与 examples/ 包）
	@echo "🧪 运行覆盖率（排除 test/ 与 examples/ 包）..."
	@PKGS=$$(go list ./... | grep -v '^github.com/rushairer/batchflow/test/' | grep -v '^github.com/rushairer/batchflow/examples/'); \
	go test -v -cover -coverpkg="$$(echo $$PKGS | tr ' ' ',')" $$PKGS -coverprofile=coverage.out; \
	go tool cover -func=coverage.out | tee coverage.txt; \
	awk '/total:/ {gsub("%","", $$3); print $$3}' coverage.txt > coverage_total.txt

# 集成测试相关
test-integration: ## 运行集成测试
	@echo "🔬 运行集成测试..."
	cd test/integration && go run .

test-integration-with-monitoring: monitoring ## 启动监控后运行集成测试
	@echo "📊 启动监控环境后运行集成测试..."
	@sleep 5  # 等待监控服务启动
	cd test/integration && PROMETHEUS_ENABLED=true go run .

# Docker 集成测试 - 单数据库高性能压力测试
docker-mysql-test: ## 运行 MySQL Docker 压力测试
	@echo "🐳 Starting MySQL pressure test..."
	docker compose -f ./docker-compose.integration.yml down mysql mysql-test -v --remove-orphans
	docker compose -f ./docker-compose.integration.yml build mysql mysql-test --no-cache
	docker compose -f ./docker-compose.integration.yml up mysql mysql-test --abort-on-container-exit --exit-code-from mysql-test

docker-postgres-test: ## 运行 PostgreSQL Docker 压力测试
	@echo "🐳 Starting PostgreSQL pressure test..."
	docker compose -f ./docker-compose.integration.yml down postgres postgres-test -v --remove-orphans
	docker compose -f ./docker-compose.integration.yml build postgres postgres-test --no-cache
	docker compose -f ./docker-compose.integration.yml up postgres postgres-test --abort-on-container-exit --exit-code-from postgres-test

docker-sqlite-test: ## 运行 SQLite Docker 压力测试
	@echo "🐳 Starting SQLite pressure test..."
	docker compose -f ./docker-compose.integration.yml down sqlite sqlite-test -v --remove-orphans
	docker compose -f ./docker-compose.integration.yml build sqlite sqlite-test --no-cache
	docker compose -f ./docker-compose.integration.yml up sqlite sqlite-test --abort-on-container-exit --exit-code-from sqlite-test

docker-redis-test: ## 运行 Redis Docker 压力测试
	@echo "🐳 Starting Redis pressure test..."
	docker compose -f ./docker-compose.integration.yml down redis redis-test -v --remove-orphans
	docker compose -f ./docker-compose.integration.yml build redis redis-test --no-cache
	docker compose -f ./docker-compose.integration.yml up redis redis-test --abort-on-container-exit --exit-code-from redis-test

docker-all-tests: docker-mysql-test docker-postgres-test docker-sqlite-test docker-redis-test ## 运行所有数据库 Docker 压力测试
	@echo "🎉 All pressure tests completed!"
	@echo "📊 Check ./test/reports/ for detailed performance reports"

# Docker 测试 + 监控（使用统一的 docker compose 文件）
docker-mysql-test-with-monitoring: ## MySQL Docker 测试 + 监控
	@echo "🐳📊 Starting MySQL pressure test with monitoring..."
	docker compose -f ./docker-compose.integration-with-monitoring.yml down mysql mysql-test prometheus grafana -v --remove-orphans
	docker compose -f ./docker-compose.integration-with-monitoring.yml build mysql mysql-test --no-cache
	docker compose -f ./docker-compose.integration-with-monitoring.yml up mysql mysql-test prometheus grafana --abort-on-container-exit --exit-code-from mysql-test

docker-postgres-test-with-monitoring: ## PostgreSQL Docker 测试 + 监控
	@echo "🐳📊 Starting PostgreSQL pressure test with monitoring..."
	docker compose -f ./docker-compose.integration-with-monitoring.yml down postgres postgres-test prometheus grafana -v --remove-orphans
	docker compose -f ./docker-compose.integration-with-monitoring.yml build postgres postgres-test --no-cache
	docker compose -f ./docker-compose.integration-with-monitoring.yml up postgres postgres-test prometheus grafana --abort-on-container-exit --exit-code-from postgres-test

docker-sqlite-test-with-monitoring: ## SQLite Docker 测试 + 监控
	@echo "🐳📊 Starting SQLite pressure test with monitoring..."
	docker compose -f ./docker-compose.integration-with-monitoring.yml down sqlite sqlite-test prometheus grafana -v --remove-orphans
	docker compose -f ./docker-compose.integration-with-monitoring.yml build sqlite sqlite-test --no-cache
	docker compose -f ./docker-compose.integration-with-monitoring.yml up sqlite sqlite-test prometheus grafana --abort-on-container-exit --exit-code-from sqlite-test

docker-redis-test-with-monitoring: ## Redis Docker 测试 + 监控
	@echo "🐳📊 Starting Redis pressure test with monitoring..."
	docker compose -f ./docker-compose.integration-with-monitoring.yml down redis redis-test prometheus grafana -v --remove-orphans
	docker compose -f ./docker-compose.integration-with-monitoring.yml build redis redis-test --no-cache
	docker compose -f ./docker-compose.integration-with-monitoring.yml up redis redis-test prometheus grafana --abort-on-container-exit --exit-code-from redis-test

docker-all-tests-with-monitoring: docker-mysql-test-with-monitoring docker-postgres-test-with-monitoring docker-sqlite-test-with-monitoring docker-redis-test-with-monitoring## 所有数据库 Docker 测试 + 监控
	@echo "🎉 All pressure tests completed!"
	@echo "📊 Check ./test/reports/ for detailed performance reports"
	
# 依赖管理
deps: ## 安装/更新依赖
	@echo "📦 安装依赖..."
	@go mod download
	@go mod tidy

deps-update: ## 更新所有依赖到最新版本
	@echo "⬆️ 更新依赖..."
	@go get -u ./...
	@go mod tidy

# 监控相关
monitoring: ## 启动 Prometheus + Grafana 监控环境
	@echo "📊 启动监控环境..."
	docker compose -f ./docker-compose.integration.yml up prometheus grafana -d

monitoring-foreground: ## 前台启动监控环境
	@echo "📊 前台启动监控环境..."
	docker compose -f ./docker-compose.integration.yml up prometheus grafana

monitoring-stop: ## 停止监控服务
	@echo "🛑 停止监控服务..."
	docker compose -f ./docker-compose.integration.yml down prometheus grafana

monitoring-status: ## 查看监控服务状态
	@echo "📊 监控服务状态:"
	docker compose -f ./docker-compose.integration.yml ps prometheus grafana

monitoring-logs: ## 查看监控服务日志
	@echo "📋 监控服务日志:"
	docker compose -f ./docker-compose.integration.yml logs -f prometheus grafana

monitoring-cleanup: ## 清理并重启监控服务
	@echo "🧹 清理并重启监控服务..."
	docker compose -f ./docker-compose.integration.yml down prometheus grafana -v --remove-orphans
	docker compose -f ./docker-compose.integration.yml up prometheus grafana -d

# 开发相关
dev-setup: deps ## 设置开发环境
	@echo "🛠️ 设置开发环境..."
	@echo "✅ 依赖已安装"
	@echo "💡 运行 'make monitoring' 启动监控环境"
	@echo "💡 运行 'make test-integration-with-monitoring' 进行完整测试"

fmt: ## 格式化代码
	@echo "🎨 格式化代码..."
	@go fmt ./... > /dev/null

lint: ## 运行代码检查
	@echo "🔍 运行代码检查..."
	@LINTER="$$(go env GOPATH)/bin/golangci-lint"; \
	if [ -x "$$LINTER" ]; then \
		"$$LINTER" run; \
	elif command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "⚠️ golangci-lint 未安装，跳过代码检查"; \
		echo "💡 安装方法: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
	fi

# 清理相关
clean: ## 清理构建文件和缓存
	@echo "🧹 清理构建文件..."
	go clean -cache -testcache -modcache
	rm -rf test/reports/*

clean-all: clean monitoring-stop ## 完全清理（包括停止监控服务）
	@echo "🧹 完全清理..."
	docker system prune -f

# 性能测试
benchmark: ## 运行性能基准测试
	@echo "⚡ 运行性能基准测试..."
	go test -bench=. -benchmem ./...

# 文档相关
docs: ## 生成文档
	@echo "📚 生成文档..."
	@if command -v godoc >/dev/null 2>&1; then \
		echo "📖 启动文档服务器: http://localhost:6060"; \
		godoc -http=:6060; \
	else \
		echo "⚠️ godoc 未安装"; \
		echo "💡 安装方法: go install golang.org/x/tools/cmd/godoc@latest"; \
	fi

docs-check: ## 检查关键文档与当前 API/指标契约是否一致
	@echo "📘 检查关键文档一致性..."
	@chmod +x ./scripts/check-doc-consistency.sh
	@./scripts/check-doc-consistency.sh

# CI/CD 相关
ci: deps fmt lint docs-check test test-race ## CI 流程（依赖安装 + 格式化 + 文档检查 + 代码检查 + 测试）
	@echo "🚀 CI 流程完成 - 所有检查通过"

# 发布相关
release-check: docs-check lint test  ## 发布前检查
	@echo "🔍 发布前检查..."
	@echo "✅ 测试通过"
	@echo "✅ 代码检查通过"
	@echo "🎉 可以发布！"

# Docker 相关
docker-build: ## 构建 Docker 镜像
	@echo "🐳 构建 Docker 镜像..."
	docker build -t batchflow:latest .

docker-test: ## 在 Docker 中运行测试
	@echo "🐳 在 Docker 中运行测试..."
	docker run --rm -v $(PWD):/app -w /app golang:1.21 make test

# 快捷命令组合
quick-test: fmt docs-check test ## 快速测试（格式化 + 文档检查 + 单元测试）
	@echo "⚡ 快速测试完成"

full-test: fmt docs-check test test-race test-integration ## 完整测试套件
	@echo "🎉 完整测试套件完成"

dev: monitoring test-integration-with-monitoring ## 开发模式（启动监控 + 集成测试）
	@echo "🚀 开发环境就绪"

# 显示项目信息
info: ## 显示项目信息
	@echo "📋 BatchFlow 项目信息"
	@echo "  版本: $(shell git describe --tags --always --dirty 2>/dev/null || echo 'unknown')"
	@echo "  分支: $(shell git branch --show-current 2>/dev/null || echo 'unknown')"
	@echo "  Go 版本: $(shell go version)"
	@echo "  项目路径: $(PWD)"
	@echo ""
	@echo "📊 监控地址:"
	@echo "  Grafana:    http://localhost:3000 (admin/admin)"
	@echo "  Prometheus: http://localhost:9091"
	@echo "  指标端点:   http://localhost:9090/metrics"
