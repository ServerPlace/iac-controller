package async

import (
	"context"
	"net/http"
	"time"

	"github.com/ServerPlace/iac-controller/pkg/log"
)

type Engine struct {
	store    Store
	enq      Enqueuer
	registry *Registry
	leaseTTL time.Duration
}

func NewEngine(store Store, enq Enqueuer, reg *Registry, leaseTTL time.Duration) *Engine {
	if leaseTTL <= 0 {
		leaseTTL = 2 * time.Minute
	}
	return &Engine{
		store:    store,
		enq:      enq,
		registry: reg,
		leaseTTL: leaseTTL,
	}
}

// detached returns a context detached from cancellation but with an explicit
// timeout for GCP API calls (Firestore mark operations, Cloud Tasks enqueue).
// WithoutCancel alone is not enough — it removes the parent deadline, so without
// a new timeout the call could hang indefinitely if the GCP API is unresponsive.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
}

func (e *Engine) Kick(ctx context.Context, kind, key string, wakeNow bool, delay time.Duration) error {
	logger := log.FromContext(ctx)
	ref := ExecutionRef{Kind: kind, Key: key}
	should, err := e.store.UpsertQueued(ctx, ref, wakeNow)
	if err != nil {
		return err
	}
	if should {
		d := delay
		if wakeNow {
			d = 0
		}
		logger.Debug().Str("key", ref.Key).Dur("delay", d).Msgf("async: kicking execution")
		enqCtx, cancel := detached(ctx)
		defer cancel()
		return e.enq.EnqueueRun(enqCtx, ref, d)
	}
	logger.Debug().Msgf("async: coalesced execution %s (already queued/running)", ref.Key)
	return nil
}

func (e *Engine) RunOnce(ctx context.Context, ref ExecutionRef, owner string) (int, error) {
	acquired, exec, err := e.store.AcquireLease(ctx, ref, owner, e.leaseTTL)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !acquired {
		return http.StatusOK, nil
	}

	h, ok := e.registry.Get(ref.Kind)
	if !ok {
		markCtx, cancel := detached(ctx)
		defer cancel()
		_ = e.store.MarkFailed(markCtx, ref, "no handler registered")
		return http.StatusOK, nil
	}

	outcome, runErr := h.Run(ctx, exec)
	if runErr != nil {
		return http.StatusInternalServerError, runErr
	}

	// Detach from the request context so that a Cloud Tasks timeout doesn't
	// silently skip the Firestore state transition or the re-enqueue call.
	markCtx, markCancel := detached(ctx)
	defer markCancel()

	switch outcome.Type {
	case OutcomeDone:
		_ = e.store.MarkDone(markCtx, ref, outcome.Checkpoint)
		return http.StatusOK, nil

	case OutcomeWait:
		wakeAt := time.Now().Add(outcome.Delay)
		_ = e.store.MarkWaiting(markCtx, ref, wakeAt, outcome.Reason, outcome.Checkpoint)
		_ = e.enq.EnqueueRun(markCtx, ref, outcome.Delay)
		return http.StatusOK, nil

	case OutcomeRetry:
		return http.StatusInternalServerError, outcome.Err

	case OutcomeFail:
		_ = e.store.MarkFailed(markCtx, ref, outcome.Err.Error())
		return http.StatusOK, nil
	}

	return http.StatusInternalServerError, nil
}
