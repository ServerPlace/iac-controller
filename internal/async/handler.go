package async

import (
	"context"
)

type Handler interface {
	Run(ctx context.Context, exec Execution) (Outcome, error)
}
