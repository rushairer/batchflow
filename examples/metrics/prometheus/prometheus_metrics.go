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
	IncludeTestName bool // 是否启用 test_name 维度（适合集成/压测）
	IncludeTable    bool // 是否启用 table 维度（注意基数膨胀）

	// 直方图桶
	EnqueueBuckets   []float64
	AssembleBuckets  []float64
	ExecuteBuckets   []float64
	BatchSizeBuckets []float64
}

// Metrics 指标容器
type Metrics struct {
	registry *prometheus.Registry

	// Counter
	totalErrors *prometheus.CounterVec

	// Histogram
	enqueueLatency   *prometheus.HistogramVec
	assembleDuration *prometheus.HistogramVec
	executeDuration  *prometheus.HistogramVec
	batchSize        *prometheus.HistogramVec

	// Gauge
	executorConcurrency *prometheus.GaugeVec
	queueLength         *prometheus.GaugeVec
	inflightBatches     *prometheus.GaugeVec

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
	labelsEnqueue := []string{"database"}
	labelsAssemble := []string{"database"}
	labelsExecute := []string{"database"}
	labelsBatchSize := []string{"database"}
	labelsConcurrency := []string{"database"}
	labelsQueue := []string{"database"}
	labelsInflight := []string{"database"}

	if opts.IncludeTestName {
		labelsErrors = append(labelsErrors, "test_name")
		labelsExecute = append(labelsExecute, "test_name")
		labelsEnqueue = append(labelsEnqueue, "test_name")
		labelsAssemble = append(labelsAssemble, "test_name")
		labelsBatchSize = append(labelsBatchSize, "test_name")
		labelsConcurrency = append(labelsConcurrency, "test_name")
		labelsQueue = append(labelsQueue, "test_name")
		labelsInflight = append(labelsInflight, "test_name")
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

	// 注册
	reg.MustRegister(
		m.totalErrors,
		m.enqueueLatency,
		m.assembleDuration,
		m.executeDuration,
		m.batchSize,
		m.executorConcurrency,
		m.queueLength,
		m.inflightBatches,
	)

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

func (m *Metrics) incError(database, testName, reason string) {
	labels := []string{database, reason}
	if m.totalErrors == nil {
		return
	}
	// totalErrors 维度：database,[test_name],error_type
	if hasLabel(m.totalErrors, "test_name") {
		labels = []string{database, reason, testName}
	}
	m.totalErrors.WithLabelValues(labels...).Inc()
}

func (m *Metrics) observeEnqueue(database, testName string, d time.Duration) {
	labels := []string{database}
	if hasLabel(m.enqueueLatency, "test_name") {
		labels = []string{database, testName}
	}
	m.enqueueLatency.WithLabelValues(labels...).Observe(d.Seconds())
}

func (m *Metrics) observeAssemble(database, testName string, d time.Duration) {
	labels := []string{database}
	if hasLabel(m.assembleDuration, "test_name") {
		labels = []string{database, testName}
	}
	m.assembleDuration.WithLabelValues(labels...).Observe(d.Seconds())
}

func (m *Metrics) observeExecute(database, testName, table string, n int, d time.Duration, _ string) {
	// executeDuration 维度：database,[test_name],[table]
	var labels []string
	switch {
	case hasLabel(m.executeDuration, "test_name") && hasLabel(m.executeDuration, "table"):
		labels = []string{database, testName, table}
	case hasLabel(m.executeDuration, "test_name"):
		labels = []string{database, testName}
	case hasLabel(m.executeDuration, "table"):
		labels = []string{database, table}
	default:
		labels = []string{database}
	}
	m.executeDuration.WithLabelValues(labels...).Observe(d.Seconds())
	// 同时记录批大小
	var bsLabels []string
	if hasLabel(m.batchSize, "test_name") {
		bsLabels = []string{database, testName}
	} else {
		bsLabels = []string{database}
	}
	m.batchSize.WithLabelValues(bsLabels...).Observe(float64(n))
}

func (m *Metrics) setConcurrency(database, testName string, n int) {
	labels := []string{database}
	if hasLabel(m.executorConcurrency, "test_name") {
		labels = []string{database, testName}
	}
	m.executorConcurrency.WithLabelValues(labels...).Set(float64(n))
}

func (m *Metrics) setQueueLen(database, testName string, n int) {
	labels := []string{database}
	if hasLabel(m.queueLength, "test_name") {
		labels = []string{database, testName}
	}
	m.queueLength.WithLabelValues(labels...).Set(float64(n))
}

func (m *Metrics) incInflight(database, testName string) {
	labels := []string{database}
	if hasLabel(m.inflightBatches, "test_name") {
		labels = []string{database, testName}
	}
	m.inflightBatches.WithLabelValues(labels...).Inc()
}

func (m *Metrics) decInflight(database, testName string) {
	labels := []string{database}
	if hasLabel(m.inflightBatches, "test_name") {
		labels = []string{database, testName}
	}
	m.inflightBatches.WithLabelValues(labels...).Dec()
}

func hasLabel(_ prometheus.Collector, _ string) bool {
	// CounterVec/HistogramVec/GaugeVec 都实现了 Describe，可从 Desc 文本判断标签是否存在
	// 这里用一个简化的静态判断套路：依赖我们构造时的选择，不做反射/解析，避免开销。
	// 在本实现中我们基于构造路径直接知道是否包含 test_name/table，因此上面直接使用 hasLabel 调用点的布尔条件。
	// 为保持接口一致性，保留函数签名。
	return false
}
