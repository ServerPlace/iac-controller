package model

import "errors"

// Erros de Negócio
var (
	ErrInvalidSignature = errors.New("invalid jit token signature")
	ErrJobFinished      = errors.New("job is already finished or expired")
	ErrJobNotFound      = errors.New("job not found")
)
