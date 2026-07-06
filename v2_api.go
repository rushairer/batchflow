package batchflow

import "context"

// V2BatchFlow is the stable public runtime surface for the v2 module path.
type V2BatchFlow = RuntimeEngine

// NewV2BatchFlow creates the converged v2 runtime. This is the preferred
// constructor for new code using github.com/rushairer/batchflow/v2.
func NewV2BatchFlow(ctx context.Context, cfg V2Config) (*V2BatchFlow, error) {
	return NewRuntimeEngine(ctx, cfg)
}
