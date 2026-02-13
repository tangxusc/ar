//go:build !linux

package pipeline

import (
	"context"
	"errors"
)

// RunStep 在非 Linux 上为未实现，保证编译通过。
func RunStep(ctx context.Context, runtimeRoot, imagesStoreDir, runDir, nodeDir, containerID string, step *PipelineStepState) RunStepResult {
	_ = ctx
	_ = runtimeRoot
	_ = imagesStoreDir
	_ = runDir
	_ = nodeDir
	_ = containerID
	_ = step
	return RunStepResult{ExitCode: -1, Err: errors.New("pipeline run 仅在 Linux 上支持")}
}
