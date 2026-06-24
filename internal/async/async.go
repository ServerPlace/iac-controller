package async

import (
	"context"
	"time"
)

type Store interface {
	UpsertQueued(ctx context.Context, ref ExecutionRef, wakeNow bool) (bool, error)
	AcquireLease(ctx context.Context, ref ExecutionRef, owner string, ttl time.Duration) (bool, Execution, error)
	Get(ctx context.Context, ref ExecutionRef) (Execution, error)
	// Mark* transitions the execution to the given state and checks the dirty flag
	// atomically. Returns true if a Kick() arrived during execution and the engine
	// should re-enqueue immediately.
	MarkDone(ctx context.Context, ref ExecutionRef, checkpoint string) (bool, error)
	MarkWaiting(ctx context.Context, ref ExecutionRef, wakeAt time.Time, reason string, checkpoint string) (bool, error)
	MarkFailed(ctx context.Context, ref ExecutionRef, errMsg string) (bool, error)
	ClearLease(ctx context.Context, ref ExecutionRef) error
}
