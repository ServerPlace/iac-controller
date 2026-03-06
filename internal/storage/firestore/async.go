package firestore

import (
	"cloud.google.com/go/firestore"
	"context"
	"fmt"
	"github.com/ServerPlace/iac-controller/internal/async"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type FirestoreStore struct {
	client     *firestore.Client
	collection string
	now        func() time.Time
}

func NewFirestoreStore(client *firestore.Client, collection string) *FirestoreStore {
	if collection == "" {
		collection = "executions"
	}
	return &FirestoreStore{
		client:     client,
		collection: collection,
		now:        time.Now,
	}
}

type fsExecution struct {
	Kind       string    `firestore:"kind"`
	Key        string    `firestore:"key"`
	Status     string    `firestore:"status"`
	Attempt    int       `firestore:"attempt"`
	LeaseOwner string    `firestore:"lease_owner"`
	LeaseUntil time.Time `firestore:"lease_until"`

	WakeAt     *time.Time `firestore:"wake_at"`
	Dirty      bool       `firestore:"dirty"`
	Checkpoint string     `firestore:"checkpoint"`

	WaitReason string    `firestore:"wait_reason"`
	LastError  string    `firestore:"last_error"`
	UpdatedAt  time.Time `firestore:"updated_at"`
}

func (s *FirestoreStore) doc(ref async.ExecutionRef) *firestore.DocumentRef {
	return s.client.Collection(s.collection).Doc(ref.ID())
}

func (s *FirestoreStore) Get(ctx context.Context, ref async.ExecutionRef) (async.Execution, error) {
	ds, err := s.doc(ref).Get(ctx)
	if err != nil {
		return async.Execution{}, err
	}
	var f fsExecution
	if err := ds.DataTo(&f); err != nil {
		return async.Execution{}, err
	}
	return fromFS(f), nil
}

func fromFS(f fsExecution) async.Execution {
	var wakeAt *time.Time
	if f.WakeAt != nil {
		t := *f.WakeAt
		wakeAt = &t
	}
	return async.Execution{
		Ref:        async.ExecutionRef{Kind: f.Kind, Key: f.Key},
		Status:     async.ExecutionStatus(f.Status),
		Attempt:    f.Attempt,
		LeaseOwner: f.LeaseOwner,
		LeaseUntil: f.LeaseUntil,
		WakeAt:     wakeAt,
		Dirty:      f.Dirty,
		Checkpoint: f.Checkpoint,
		WaitReason: f.WaitReason,
		LastError:  f.LastError,
		UpdatedAt:  f.UpdatedAt,
	}
}

func (s *FirestoreStore) UpsertQueued(ctx context.Context, ref async.ExecutionRef, wakeNow bool) (bool, error) {
	now := s.now()
	doc := s.doc(ref)

	var shouldEnqueue bool

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ds, err := tx.Get(doc)
		if err != nil {
			// Se não existe, cria.
			if status.Code(err) == codes.NotFound {
				shouldEnqueue = true
				f := fsExecution{
					Kind:      ref.Kind,
					Key:       ref.Key,
					Status:    string(async.ExecutionStatusQueued),
					Attempt:   0,
					Dirty:     false,
					UpdatedAt: now,
				}
				return tx.Create(doc, f)
			}
			return err
		}

		var cur fsExecution
		if err := ds.DataTo(&cur); err != nil {
			return err
		}

		switch async.ExecutionStatus(cur.Status) {
		case async.ExecutionStatusQueued:
			shouldEnqueue = false
			// coalescing: nada a fazer (mantém queued)
			return tx.Update(doc, []firestore.Update{
				{Path: "updated_at", Value: now},
			})

		case async.ExecutionStatusRunning:
			// dirty=true, não enfileira.
			shouldEnqueue = false
			return tx.Update(doc, []firestore.Update{
				{Path: "dirty", Value: true},
				{Path: "updated_at", Value: now},
			})

		case async.ExecutionStatusWaiting:
			if wakeNow {
				shouldEnqueue = true
				return tx.Update(doc, []firestore.Update{
					{Path: "status", Value: string(async.ExecutionStatusQueued)},
					{Path: "wake_at", Value: firestore.Delete},
					{Path: "wait_reason", Value: ""},
					{Path: "last_error", Value: ""},
					{Path: "dirty", Value: false},
					{Path: "updated_at", Value: now},
				})
			}
			shouldEnqueue = false
			return tx.Update(doc, []firestore.Update{
				{Path: "updated_at", Value: now},
			})

		case async.ExecutionStatusDone, async.ExecutionStatusFailed:
			shouldEnqueue = true
			return tx.Update(doc, []firestore.Update{
				{Path: "status", Value: string(async.ExecutionStatusQueued)},
				{Path: "attempt", Value: 0},
				{Path: "wake_at", Value: firestore.Delete},
				{Path: "wait_reason", Value: ""},
				{Path: "last_error", Value: ""},
				{Path: "dirty", Value: false},
				{Path: "updated_at", Value: now},
			})

		default:
			// estado inválido: normaliza pra queued e enfileira
			shouldEnqueue = true
			return tx.Update(doc, []firestore.Update{
				{Path: "status", Value: string(async.ExecutionStatusQueued)},
				{Path: "attempt", Value: 0},
				{Path: "wake_at", Value: firestore.Delete},
				{Path: "dirty", Value: false},
				{Path: "updated_at", Value: now},
			})
		}
	})
	return shouldEnqueue, err
}

func (s *FirestoreStore) AcquireLease(ctx context.Context, ref async.ExecutionRef, owner string, ttl time.Duration) (bool, async.Execution, error) {
	now := s.now()
	doc := s.doc(ref)

	var acquired bool
	var out async.Execution

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ds, err := tx.Get(doc)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				// Cria doc básico e tenta adquirir imediatamente.
				f := fsExecution{
					Kind:      ref.Kind,
					Key:       ref.Key,
					Status:    string(async.ExecutionStatusQueued),
					Attempt:   0,
					Dirty:     false,
					UpdatedAt: now,
				}
				if err := tx.Create(doc, f); err != nil {
					return err
				}
				ds, err = tx.Get(doc)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		var cur fsExecution
		if err := ds.DataTo(&cur); err != nil {
			return err
		}

		// Lease ativo?
		if !cur.LeaseUntil.IsZero() && cur.LeaseUntil.After(now) {
			acquired = false
			out = fromFS(cur)
			return nil
		}

		leaseUntil := now.Add(ttl)
		newAttempt := cur.Attempt + 1

		updates := []firestore.Update{
			{Path: "status", Value: string(async.ExecutionStatusRunning)},
			{Path: "lease_owner", Value: owner},
			{Path: "lease_until", Value: leaseUntil},
			{Path: "attempt", Value: newAttempt},
			{Path: "updated_at", Value: now},
		}

		// Se estava waiting e wake_at > now, ainda assim setamos running?
		// A guarda "waiting_not_due" é aplicada depois, mas já sob lease (um runner por vez).
		// Isso evita corrida se múltiplos ticks chegarem cedo.
		if err := tx.Update(doc, updates); err != nil {
			return err
		}

		acquired = true
		cur.Status = string(async.ExecutionStatusRunning)
		cur.LeaseOwner = owner
		cur.LeaseUntil = leaseUntil
		cur.Attempt = newAttempt
		cur.UpdatedAt = now
		out = fromFS(cur)
		return nil
	})

	return acquired, out, err
}

func (s *FirestoreStore) MarkWaiting(ctx context.Context, ref async.ExecutionRef, wakeAt time.Time, reason string, checkpoint string) error {
	now := s.now()
	_, err := s.doc(ref).Update(ctx, []firestore.Update{
		{Path: "status", Value: string(async.ExecutionStatusWaiting)},
		{Path: "wake_at", Value: wakeAt},
		{Path: "wait_reason", Value: reason},
		{Path: "checkpoint", Value: checkpoint},
		{Path: "updated_at", Value: now},
		// Limpa o lease para que o próximo run possa adquiri-lo imediatamente.
		{Path: "lease_until", Value: time.Time{}},
		{Path: "lease_owner", Value: ""},
	})
	return err
}

func (s *FirestoreStore) MarkDone(ctx context.Context, ref async.ExecutionRef, checkpoint string) error {
	now := s.now()
	_, err := s.doc(ref).Update(ctx, []firestore.Update{
		{Path: "status", Value: string(async.ExecutionStatusDone)},
		{Path: "wake_at", Value: firestore.Delete},
		{Path: "wait_reason", Value: ""},
		{Path: "last_error", Value: ""},
		{Path: "checkpoint", Value: checkpoint},
		{Path: "updated_at", Value: now},
	})
	return err
}

func (s *FirestoreStore) MarkFailed(ctx context.Context, ref async.ExecutionRef, errMsg string) error {
	now := s.now()
	_, err := s.doc(ref).Update(ctx, []firestore.Update{
		{Path: "status", Value: string(async.ExecutionStatusFailed)},
		{Path: "last_error", Value: errMsg},
		{Path: "updated_at", Value: now},
	})
	return err
}

func (s *FirestoreStore) FinalizeAfterRun(ctx context.Context, ref async.ExecutionRef) (bool, error) {
	now := s.now()
	doc := s.doc(ref)

	var shouldEnqueue bool

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ds, err := tx.Get(doc)
		if err != nil {
			return err
		}
		var cur fsExecution
		if err := ds.DataTo(&cur); err != nil {
			return err
		}

		if cur.Dirty {
			// dirty vence wait: força queued e tick imediato.
			shouldEnqueue = true
			return tx.Update(doc, []firestore.Update{
				{Path: "dirty", Value: false},
				{Path: "status", Value: string(async.ExecutionStatusQueued)},
				{Path: "wake_at", Value: firestore.Delete},
				{Path: "wait_reason", Value: ""},
				{Path: "updated_at", Value: now},
			})
		}

		shouldEnqueue = false
		return tx.Update(doc, []firestore.Update{
			{Path: "updated_at", Value: now},
		})
	})

	return shouldEnqueue, err
}

// (Opcional) utilitário de debug/observabilidade; não é parte do contrato.
func (s *FirestoreStore) ListAll(ctx context.Context) ([]async.Execution, error) {
	it := s.client.Collection(s.collection).Documents(ctx)
	defer it.Stop()

	var out []async.Execution
	for {
		ds, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var f fsExecution
		if err := ds.DataTo(&f); err != nil {
			return nil, err
		}
		out = append(out, fromFS(f))
	}
}

func (s *FirestoreStore) String() string {
	return fmt.Sprintf("FirestoreStore(collection=%s)", s.collection)
}
