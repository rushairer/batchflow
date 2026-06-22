package prometheusmetrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rushairer/batchflow/v2"
)

func TestReporter_OperationMetrics(t *testing.T) {
	metrics := NewMetrics(Options{
		Namespace:         "batchflow_test",
		IncludeInstanceID: true,
		IncludeTable:      true,
	})
	reporter := NewReporter(metrics, "custom", "worker_a")

	reporter.ObserveOperationGenerated(batchflow.OperationPreview{
		Backend:     "http",
		Operation:   "post",
		Schema:      "api_events",
		InputItems:  10,
		OutputItems: 1,
		ArgCount:    3,
	})
	reporter.IncOperationError("api_events", "http", batchflow.BatchStageExecute, "timeout")

	foundOperationItems := false
	foundOperationErrors := false
	for _, metricFamily := range gather(t, metrics.registry) {
		switch metricFamily.GetName() {
		case "batchflow_test_operation_generated_items":
			foundOperationItems = true
		case "batchflow_test_operation_errors_total":
			foundOperationErrors = true
		}
	}
	if !foundOperationItems {
		t.Fatal("operation generated items metric not found")
	}
	if !foundOperationErrors {
		t.Fatal("operation errors metric not found")
	}
}

func gather(t *testing.T, gatherer prometheus.Gatherer) []*dto.MetricFamily {
	t.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather error: %v", err)
	}
	return families
}
