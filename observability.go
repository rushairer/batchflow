package batchflow

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"time"
)

const (
	BackendSQL    = "sql"
	BackendRedis  = "redis"
	BackendCustom = "custom"

	OperationInsert  = "insert"
	OperationUpsert  = "upsert"
	OperationCommand = "command"
	OperationCustom  = "custom"

	BatchStageValidate = "validate"
	BatchStageGenerate = "generate"
	BatchStageExecute  = "execute"
	BatchStageRetry    = "retry"
	BatchStageFinal    = "final"
)

// OperationPreview is a backend-neutral dry-run summary of generated work.
// Attributes must stay small and low-cardinality. Raw payloads, SQL args,
// Redis keys, API bodies, and other sensitive values should not be stored here.
type OperationPreview struct {
	Backend     string
	Operation   string
	Schema      string
	InputItems  int
	OutputItems int
	ArgCount    int
	Fingerprint string
	Attributes  map[string]any
}

// OperationPreviewer lets processors generate operations and a diagnostic preview together.
// Custom processors can implement this interface to participate in logs, metrics, and tracing.
type OperationPreviewer interface {
	GenerateOperationPreview(ctx context.Context, schema SchemaInterface, data []map[string]any) (Operations, OperationPreview, error)
}

// BatchError wraps backend-neutral batch failures with safe diagnostic metadata.
type BatchError struct {
	Stage       string
	Backend     string
	Schema      string
	BatchSize   int
	Fingerprint string
	Attributes  map[string]any
	Cause       error
}

func (e *BatchError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("batch %s failed: backend=%s schema=%s batch_size=%d fingerprint=%s attribute_keys=%v: %v",
		e.Stage, e.Backend, e.Schema, e.BatchSize, e.Fingerprint, attributeKeys(e.Attributes), e.Cause)
}

func (e *BatchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type BatchEvent struct {
	Stage       string
	Backend     string
	Operation   string
	Schema      string
	Status      string
	Attempt     int
	BatchSize   int
	InputItems  int
	OutputItems int
	ArgCount    int
	Duration    time.Duration
	Fingerprint string
	Reason      string
	Attributes  map[string]any
	Err         error
}

type Observer interface {
	OnBatchEvent(ctx context.Context, event BatchEvent)
}

type ObserverFunc func(context.Context, BatchEvent)

func (f ObserverFunc) OnBatchEvent(ctx context.Context, event BatchEvent) {
	if f != nil {
		f(ctx, event)
	}
}

type Sampler interface {
	ShouldSample(event BatchEvent) bool
}

type SamplerFunc func(BatchEvent) bool

func (f SamplerFunc) ShouldSample(event BatchEvent) bool {
	if f == nil {
		return false
	}
	return f(event)
}

type Redactor interface {
	Redact(key string, value any) any
}

type RedactorFunc func(key string, value any) any

func (f RedactorFunc) Redact(key string, value any) any {
	if f == nil {
		return value
	}
	return f(key, value)
}

type ObservabilityConfig struct {
	Observer           Observer
	Logger             *slog.Logger
	Sampler            Sampler
	Redactor           Redactor
	SlowBatchThreshold time.Duration
}

func (c ObservabilityConfig) observer() Observer {
	if c.Observer != nil {
		return c.Observer
	}
	if c.Logger != nil {
		return NewSlogObserver(c.Logger, c.Sampler, c.Redactor, c.SlowBatchThreshold)
	}
	return nil
}

func NewErrorAndSlowSampler(slowThreshold time.Duration) Sampler {
	return SamplerFunc(func(event BatchEvent) bool {
		if event.Err != nil || event.Status == "fail" {
			return true
		}
		return slowThreshold > 0 && event.Duration >= slowThreshold
	})
}

func NewRatioSampler(ratio float64) Sampler {
	if ratio <= 0 {
		return SamplerFunc(func(BatchEvent) bool { return false })
	}
	if ratio >= 1 {
		return SamplerFunc(func(BatchEvent) bool { return true })
	}
	return SamplerFunc(func(event BatchEvent) bool {
		key := event.Schema + "|" + event.Fingerprint + "|" + event.Stage
		return hashStringRatio(key) < ratio
	})
}

func NewFieldNameRedactor(fields ...string) Redactor {
	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		set[strings.ToLower(field)] = struct{}{}
	}
	return RedactorFunc(func(key string, value any) any {
		lower := strings.ToLower(key)
		if _, ok := set[lower]; ok {
			return "[REDACTED]"
		}
		for field := range set {
			if strings.Contains(lower, field) {
				return "[REDACTED]"
			}
		}
		return value
	})
}

func DefaultRedactor() Redactor {
	return NewFieldNameRedactor("password", "passwd", "secret", "token", "credential", "authorization", "email", "phone")
}

type slogObserver struct {
	logger   *slog.Logger
	sampler  Sampler
	redactor Redactor
}

func NewSlogObserver(logger *slog.Logger, sampler Sampler, redactor Redactor, slowBatchThreshold time.Duration) Observer {
	if logger == nil {
		logger = slog.Default()
	}
	if sampler == nil {
		sampler = NewErrorAndSlowSampler(slowBatchThreshold)
	}
	if redactor == nil {
		redactor = DefaultRedactor()
	}
	return &slogObserver{
		logger:   logger,
		sampler:  sampler,
		redactor: redactor,
	}
}

func (o *slogObserver) OnBatchEvent(ctx context.Context, event BatchEvent) {
	if o == nil || o.logger == nil {
		return
	}
	if o.sampler != nil && !o.sampler.ShouldSample(event) {
		return
	}
	attrs := []any{
		"stage", event.Stage,
		"backend", event.Backend,
		"operation", event.Operation,
		"schema", event.Schema,
		"status", event.Status,
		"attempt", event.Attempt,
		"batch_size", event.BatchSize,
		"input_items", event.InputItems,
		"output_items", event.OutputItems,
		"arg_count", event.ArgCount,
		"duration_ms", float64(event.Duration.Microseconds()) / 1000,
		"fingerprint", event.Fingerprint,
		"reason", event.Reason,
	}
	for key, value := range event.Attributes {
		attrs = append(attrs, key, o.redact(key, value))
	}
	if event.Err != nil {
		attrs = append(attrs, "error", event.Err.Error())
		o.logger.ErrorContext(ctx, "batchflow batch event", attrs...)
		return
	}
	o.logger.InfoContext(ctx, "batchflow batch event", attrs...)
}

func (o *slogObserver) redact(key string, value any) any {
	if o.redactor == nil {
		return value
	}
	return o.redactor.Redact(key, value)
}

func FingerprintText(text string) string {
	normalized := strings.Join(strings.Fields(text), " ")
	h := fnv.New64a()
	_, _ = h.Write([]byte(normalized))
	return fmt.Sprintf("%016x", h.Sum64())
}

func OperationFingerprint(parts ...string) string {
	return FingerprintText(strings.Join(parts, "|"))
}

func hashStringRatio(s string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return float64(h.Sum64()%10000) / 10000
}

func batchErrorFromError(stage string, preview OperationPreview, batchSize int, err error) error {
	if err == nil {
		return nil
	}
	var batchErr *BatchError
	if errors.As(err, &batchErr) {
		return err
	}
	return &BatchError{
		Stage:       stage,
		Backend:     preview.Backend,
		Schema:      preview.Schema,
		BatchSize:   batchSize,
		Fingerprint: preview.Fingerprint,
		Attributes:  cloneAttributes(preview.Attributes),
		Cause:       err,
	}
}

func cloneAttributes(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for k, v := range attrs {
		out[k] = v
	}
	return out
}

func attributeKeys(attrs map[string]any) []string {
	if len(attrs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	return keys
}
