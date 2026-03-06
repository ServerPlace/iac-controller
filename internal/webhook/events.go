// FILE: internal/webhook/event.go
package webhook

// EventType representa os tipos de eventos normalizados
type EventType string

const (
	EventTypeComment    EventType = "COMMENT"
	EventTypePRUpdate   EventType = "PR_UPDATE"
	EventTypePRApproved EventType = "PR_APPROVED"
	EventTypePRClosed   EventType = "PR_CLOSED"
	EventTypePing       EventType = "PING"
	EventTypeUnknown    EventType = "UNKNOWN"
)

// NormalizedEvent é o evento unificado entre providers
type NormalizedEvent struct {
	Type       EventType
	Provider   string
	Repo       string
	PRNumber   int
	Sender     string
	Body       string
	CommitSHA  string
	IsApproved bool
	IsMerged   bool
	Action     string // Raw action para debug
}

// ProcessorResult encapsula o resultado do processamento
type ProcessorResult struct {
	Event       *NormalizedEvent
	ShouldQueue bool
	Message     string
}
