package async

import (
	"context"
	"time"
)

type Enqueuer interface {
	EnqueueRun(ctx context.Context, ref ExecutionRef, delay time.Duration) error
}
