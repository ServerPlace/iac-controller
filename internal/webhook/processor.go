// FILE: internal/webhook/processor.go
package webhook

import "net/http"

// Processor extrai e normaliza eventos do payload
type Processor interface {
	Parse(r *http.Request) (*ProcessorResult, error)
}
