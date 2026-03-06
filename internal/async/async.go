package async

import (
	"context"
	"time"
)

type Store interface {
	UpsertQueued(ctx context.Context, ref ExecutionRef, wakeNow bool) (bool, error)
	AcquireLease(ctx context.Context, ref ExecutionRef, owner string, ttl time.Duration) (bool, Execution, error)
	Get(ctx context.Context, ref ExecutionRef) (Execution, error)
	MarkWaiting(ctx context.Context, ref ExecutionRef, wakeAt time.Time, reason string, checkpoint string) error
	MarkDone(ctx context.Context, ref ExecutionRef, checkpoint string) error
	MarkFailed(ctx context.Context, ref ExecutionRef, errMsg string) error
	FinalizeAfterRun(ctx context.Context, ref ExecutionRef) (bool, error)
}
