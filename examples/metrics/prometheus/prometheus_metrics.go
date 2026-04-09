package prometheusmetrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Options 配置项（可选）
type Options struct {
	// 指标命名
	Namespace   string            // 如 "batchflow"
	Subsystem   string            // 可为空
	ConstLabels map[string]string // 追加到所有指标的常量标签，如 {"env":"prod","region":"cn"}

	// 标签维度开关（保持开箱可用的最小集）
	IncludeInstanceID bool // 是否启用 instance_id 维度（推荐开启，支持多实例）
	IncludeTable      bool // 是否启用 table 维度（注意基数膨胀）

	// 直方图桶
	EnqueueBuckets   []float64
	AssembleBuckets  []float64
	ExecuteBuckets   []float64
	BatchSizeBuckets []float64

	// 是否启用管道级指标（PipelineMetricsReporter）
	EnablePipelineMetrics bool
}

// Metrics 指标容器
type Metrics struct {
	registry *prometheus.Registry

	// Counter
	totalErrors         *prometheus.CounterVec
	submitRejectedTotal *prometheus.CounterVec

	// Histogram
	enqueueLatency       *prometheus.HistogramVec
	assembleDuration     *prometheus.HistogramVec
	executeDuration      *prometheus.HistogramVec
	batchSize            *prometheus.HistogramVec
	pipelineFlushSize    *prometheus.HistogramVec
	schemaGroupsPerFlush *prometheus.HistogramVec

	// Gauge
	executorConcurrency *prometheus.GaugeVec
	queueLength         *prometheus.GaugeVec
	inflightBatches     *prometheus.GaugeVec

	// Pipeline 指标（可选）
	pipelineDequeueLatency  *prometheus.HistogramVec
	pipelineProcessDuration *prometheus.HistogramVec
	pipelineDroppedTotal    *prometheus.CounterVec

	server *http.Server
}

// NewMetrics 创建并注册一套与 BatchFlow 对齐的指标
func NewMetrics(opts Options) *Metrics {
	ns := opts.Namespace
	ss := opts.Subsystem
	cl := opts.ConstLabels

	// 默认桶
	if len(opts.EnqueueBuckets) == 0 {
		opts.EnqueueBuckets = prometheus.ExponentialBuckets(0.0005, 2, 18) // 0.5ms ~
	}
	if len(opts.AssembleBuckets) == 0 {
		opts.AssembleBuckets = prometheus.ExponentialBuckets(0.0005, 2, 18)
	}
	if len(opts.ExecuteBuckets) == 0 {
		opts.ExecuteBuckets = prometheus.ExponentialBuckets(0.0005, 2, 18)
	}
	if len(opts.BatchSizeBuckets) == 0 {
		opts.BatchSizeBuckets = prometheus.ExponentialBuckets(1, 2, 12)
	}

	reg := prometheus.NewRegistry()

	labelsErrors := []string{"database", "error_type"}
	labelsRejected := []string{"database", "reason"}
	labelsEnqueue := []string{"database"}
	labelsAssemble := []string{"database"}
	labelsExecute := []string{"database"}
	labelsBatchSize := []string{"database"}
	labelsFlushSize := []string{"database"}
	labelsSchemaGroups := []string{"database"}
	labelsConcurrency := []string{"database"}
	labelsQueue := []string{"database"}
	labelsInflight := []string{"database"}
	labelsPipelineDequeue := []string{"database"}
	labelsPipelineProcess := []string{"database", "status"}
	labelsPipelineDropped := []string{"database", "reason"}

	if opts.IncludeInstanceID {
		labelsErrors = append(labelsErrors[:1], append([]string{"instance_id"}, labelsErrors[1:]...)...)
		labelsRejected = append(labelsRejected[:1], append([]string{"instance_id"}, labelsRejected[1:]...)...)
		labelsEnqueue = append(labelsEnqueue, "instance_id")
		labelsAssemble = append(labelsAssemble, "instance_id")
		labelsExecute = append(labelsExecute, "instance_id")
		labelsBatchSize = append(labelsBatchSize, "instance_id")
		labelsFlushSize = append(labelsFlushSize, "instance_id")
		labelsSchemaGroups = append(labelsSchemaGroups, "instance_id")
		labelsConcurrency = append(labelsConcurrency, "instance_id")
		labelsQueue = append(labelsQueue, "instance_id")
		labelsInflight = append(labelsInflight, "instance_id")
		labelsPipelineDequeue = append(labelsPipelineDequeue, "instance_id")
		labelsPipelineProcess = []string{"database", "instance_id", "status"}
		labelsPipelineDropped = []string{"database", "instance_id", "reason"}
	}
	if opts.IncludeTable {
		labelsExecute = append(labelsExecute, "table")
	}

	m := &Metrics{
		registry: reg,
		totalErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "errors_total",
				Help:        "Total number of errors encountered (error_type starts with retry:/final: etc.)",
				ConstLabels: cl,
			},
			labelsErrors,
		),
		submitRejectedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "submit_rejected_total",
				Help:        "Total number of rejected Submit attempts",
				ConstLabels: cl,
			},
			labelsRejected,
		),
		enqueueLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "enqueue_latency_seconds",
				Help:        "Latency from submit to enqueue",
				Buckets:     opts.EnqueueBuckets,
				ConstLabels: cl,
			},
			labelsEnqueue,
		),
		assembleDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "batch_assemble_duration_seconds",
				Help:        "Time to assemble a batch",
				Buckets:     opts.AssembleBuckets,
				ConstLabels: cl,
			},
			labelsAssemble,
		),
		executeDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "execute_duration_seconds",
				Help:        "Execute duration per batch (includes retry/backoff)",
				Buckets:     opts.ExecuteBuckets,
				ConstLabels: cl,
			},
			labelsExecute,
		),
		batchSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "batch_size",
				Help:        "Batch size distribution",
				Buckets:     opts.BatchSizeBuckets,
				ConstLabels: cl,
			},
			labelsBatchSize,
		),
		pipelineFlushSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "pipeline_flush_size",
				Help:        "Number of requests received by a single pipeline flush",
				Buckets:     opts.BatchSizeBuckets,
				ConstLabels: cl,
			},
			labelsFlushSize,
		),
		schemaGroupsPerFlush: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "schema_groups_per_flush",
				Help:        "Number of schema groups produced from a single pipeline flush",
				Buckets:     prometheus.ExponentialBuckets(1, 2, 8),
				ConstLabels: cl,
			},
			labelsSchemaGroups,
		),
		executorConcurrency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "executor_concurrency",
				Help:        "Current executor concurrency",
				ConstLabels: cl,
			},
			labelsConcurrency,
		),
		queueLength: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "pipeline_queue_length",
				Help:        "Current pipeline queue length",
				ConstLabels: cl,
			},
			labelsQueue,
		),
		inflightBatches: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "inflight_batches",
				Help:        "Current in-flight batch count",
				ConstLabels: cl,
			},
			labelsInflight,
		),
	}

	// 可选：创建管道级指标
	if opts.EnablePipelineMetrics {
		m.pipelineDequeueLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "pipeline_dequeue_latency_seconds",
				Help:        "Time waiting in pipeline queue before processing",
				Buckets:     opts.EnqueueBuckets, // 复用相同桶配置
				ConstLabels: cl,
			},
			labelsPipelineDequeue,
		)
		m.pipelineProcessDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "pipeline_process_duration_seconds",
				Help:        "Pipeline-level process duration per flush",
				Buckets:     opts.ExecuteBuckets, // 复用执行桶配置
				ConstLabels: cl,
			},
			labelsPipelineProcess,
		)
		m.pipelineDroppedTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   ns,
				Subsystem:   ss,
				Name:        "pipeline_dropped_total",
				Help:        "Total number of dropped events (e.g., error channel full)",
				ConstLabels: cl,
			},
			labelsPipelineDropped,
		)
	}

	// 注册核心指标
	reg.MustRegister(
		m.totalErrors,
		m.submitRejectedTotal,
		m.enqueueLatency,
		m.assembleDuration,
		m.executeDuration,
		m.batchSize,
		m.pipelineFlushSize,
		m.schemaGroupsPerFlush,
		m.executorConcurrency,
		m.queueLength,
		m.inflightBatches,
	)

	// 注册管道级指标（如果启用）
	if opts.EnablePipelineMetrics {
		reg.MustRegister(
			m.pipelineDequeueLatency,
			m.pipelineProcessDuration,
			m.pipelineDroppedTotal,
		)
	}

	// 常规运行时指标（可选）
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return m
}

// Handler 返回 /metrics 的 http.Handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: false})
}

// StartServer 启动一个简易 HTTP 服务（/metrics）
func (m *Metrics) StartServer(port int) error {
	if m.server != nil {
		return errors.New("metrics server already running")
	}
	m.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           m.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = m.server.ListenAndServe() }()
	time.Sleep(100 * time.Millisecond)
	return nil
}

// StopServer 停止 HTTP 服务
func (m *Metrics) StopServer(ctx context.Context) error {
	if m.server == nil {
		return nil
	}
	defer func() { m.server = nil }()
	return m.server.Shutdown(ctx)
}

// 下面是内部便捷写入方法（供 reporter 使用）

func (m *Metrics) incError(database, instanceID, reason string) {
	if m.totalErrors == nil {
		return
	}
	// totalErrors 维度：database, [instance_id], error_type
	var labels []string
	if hasLabel(m.totalErrors, "instance_id") {
		labels = []string{database, instanceID, reason}
	} else {
		labels = []string{database, reason}
	}
	m.totalErrors.WithLabelValues(labels...).Inc()
}

func (m *Metrics) incSubmitRejected(database, instanceID, reason string) {
	if m.submitRejectedTotal == nil {
		return
	}
	var labels []string
	if hasLabel(m.submitRejectedTotal, "instance_id") {
		labels = []string{database, instanceID, reason}
	} else {
		labels = []string{database, reason}
	}
	m.submitRejectedTotal.WithLabelValues(labels...).Inc()
}

func (m *Metrics) observeEnqueue(database, instanceID string, d time.Duration) {
	var labels []string
	if hasLabel(m.enqueueLatency, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.enqueueLatency.WithLabelValues(labels...).Observe(d.Seconds())
}

func (m *Metrics) observeAssemble(database, instanceID string, d time.Duration) {
	var labels []string
	if hasLabel(m.assembleDuration, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.assembleDuration.WithLabelValues(labels...).Observe(d.Seconds())
}

func (m *Metrics) observeExecute(database, instanceID, table string, n int, d time.Duration, status string) {
	// executeDuration 维度：database, [instance_id], [table], status
	var labels []string
	hasInstanceID := hasLabel(m.executeDuration, "instance_id")
	hasTable := hasLabel(m.executeDuration, "table")
	hasStatus := hasLabel(m.executeDuration, "status")

	switch {
	case hasInstanceID && hasTable && hasStatus:
		labels = []string{database, instanceID, table, status}
	case hasInstanceID && hasStatus:
		labels = []string{database, instanceID, status}
	case hasInstanceID && hasTable:
		labels = []string{database, instanceID, table}
	case hasTable && hasStatus:
		labels = []string{database, table, status}
	case hasInstanceID:
		labels = []string{database, instanceID}
	case hasTable:
		labels = []string{database, table}
	case hasStatus:
		labels = []string{database, status}
	default:
		labels = []string{database}
	}
	m.executeDuration.WithLabelValues(labels...).Observe(d.Seconds())
	_ = n
}

func (m *Metrics) observePipelineFlushSize(database, instanceID string, n int) {
	var labels []string
	if hasLabel(m.pipelineFlushSize, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.pipelineFlushSize.WithLabelValues(labels...).Observe(float64(n))
}

func (m *Metrics) observeSchemaGroups(database, instanceID string, n int) {
	var labels []string
	if hasLabel(m.schemaGroupsPerFlush, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.schemaGroupsPerFlush.WithLabelValues(labels...).Observe(float64(n))
}

func (m *Metrics) setConcurrency(database, instanceID string, n int) {
	var labels []string
	if hasLabel(m.executorConcurrency, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.executorConcurrency.WithLabelValues(labels...).Set(float64(n))
}

func (m *Metrics) setQueueLen(database, instanceID string, n int) {
	var labels []string
	if hasLabel(m.queueLength, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.queueLength.WithLabelValues(labels...).Set(float64(n))
}

func (m *Metrics) incInflight(database, instanceID string) {
	var labels []string
	if hasLabel(m.inflightBatches, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.inflightBatches.WithLabelValues(labels...).Inc()
}

func (m *Metrics) decInflight(database, instanceID string) {
	var labels []string
	if hasLabel(m.inflightBatches, "instance_id") {
		labels = []string{database, instanceID}
	} else {
		labels = []string{database}
	}
	m.inflightBatches.WithLabelValues(labels...).Dec()
}

func hasLabel(collector prometheus.Collector, labelName string) bool {
	// CounterVec/HistogramVec/GaugeVec 都实现了 Describe，可从 Desc 文本判断标签是否存在
	// 这里用一个简化的静态判断套路：依赖我们构造时的选择，不做反射/解析，避免开销。
	// 在本实现中我们基于构造路径直接知道是否包含 instance_id/table，因此上面直接使用 hasLabel 调用点的布尔条件。
	// 为保持接口一致性，保留函数签名。

	// 实际实现：通过反射检查标签（仅在配置阶段调用，运行时开销可接受）
	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	for desc := range ch {
		descStr := desc.String()
		// 简单的字符串匹配（Desc.String() 包含标签名称）
		if contains(descStr, `"`+labelName+`"`) || contains(descStr, labelName+":") {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				indexOfSubstring(s, substr) >= 0)))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
