package async

import (
	"fmt"
	"time"
)

type ExecutionRef struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

func (r ExecutionRef) ID() string {
	return fmt.Sprintf("%s:%s", r.Kind, r.Key)
}

type ExecutionStatus string

const (
	ExecutionStatusQueued  ExecutionStatus = "queued"
	ExecutionStatusRunning ExecutionStatus = "running"
	ExecutionStatusWaiting ExecutionStatus = "waiting"
	ExecutionStatusDone    ExecutionStatus = "done"
	ExecutionStatusFailed  ExecutionStatus = "failed"
)

type Execution struct {
	Ref        ExecutionRef
	Status     ExecutionStatus
	Attempt    int
	LeaseOwner string
	LeaseUntil time.Time

	WakeAt     *time.Time
	Dirty      bool
	Checkpoint string

	WaitReason string
	LastError  string
	UpdatedAt  time.Time
}

type OutcomeType string

const (
	OutcomeDone  OutcomeType = "done"
	OutcomeWait  OutcomeType = "wait"
	OutcomeRetry OutcomeType = "retry"
	OutcomeFail  OutcomeType = "fail"
)

type Outcome struct {
	Type       OutcomeType
	Delay      time.Duration
	Reason     string
	Checkpoint string
	Err        error
}

func Done(checkpoint string) Outcome {
	return Outcome{Type: OutcomeDone, Checkpoint: checkpoint}
}

func Wait(delay time.Duration, reason string, checkpoint string) Outcome {
	return Outcome{Type: OutcomeWait, Delay: delay, Reason: reason, Checkpoint: checkpoint}
}

func Retry(err error) Outcome {
	return Outcome{Type: OutcomeRetry, Err: err}
}

func Fail(err error) Outcome {
	return Outcome{Type: OutcomeFail, Err: err}
}
