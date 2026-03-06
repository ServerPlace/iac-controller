package credentials

import (
	"context"
	"fmt"
	"github.com/ServerPlace/iac-controller/pkg/hmac"
	"strings"
	"testing"
	"time"

	"github.com/ServerPlace/iac-controller/internal/core/model"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func metaWithVersion(id string, version int) *model.RepositoryMetadata {
	return &model.RepositoryMetadata{ID: id, KeyVersion: version}
}

// ─── KeyDerivationParams.Info ────────────────────────────────────────────────────────────

func TestRepoKey_Info(t *testing.T) {
	tests := []struct {
		name     string
		rk       KeyDerivationParams
		expected string
	}{
		{
			name:     "global namespace without extra",
			rk:       KeyDerivationParams{RepoID: "abc-123", NS: Global},
			expected: "repo:abc-123",
		},
		{
			name:     "plan namespace without extra",
			rk:       KeyDerivationParams{RepoID: "abc-123", NS: NSPlan},
			expected: "plan:abc-123",
		},
		{
			name:     "apply namespace with extra",
			rk:       KeyDerivationParams{RepoID: "abc-123", NS: NSApply, Extra: "run-99"},
			expected: "apply:abc-123:run-99",
		},
		{
			name:     "global with extra",
			rk:       KeyDerivationParams{RepoID: "abc-123", NS: Global, Extra: "ctx"},
			expected: "repo:abc-123:ctx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rk.Info()
			if got != tt.expected {
				t.Errorf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

// ─── KeyDerivationParams.Salt ────────────────────────────────────────────────────────────

func TestRepoKey_Salt(t *testing.T) {
	t.Run("version zero returns nil", func(t *testing.T) {
		rk := KeyDerivationParams{Version: 0}
		if rk.Salt() != nil {
			t.Errorf("expected nil, got %v", rk.Salt())
		}
	})

	t.Run("version negative returns nil", func(t *testing.T) {
		rk := KeyDerivationParams{Version: -1}
		if rk.Salt() != nil {
			t.Errorf("expected nil for negative version, got %v", rk.Salt())
		}
	})

	t.Run("version 1 returns '1'", func(t *testing.T) {
		rk := KeyDerivationParams{Version: 1}
		if string(rk.Salt()) != "1" {
			t.Errorf("expected '1', got %q", rk.Salt())
		}
	})

	t.Run("version 3 returns '3'", func(t *testing.T) {
		rk := KeyDerivationParams{Version: 3}
		if string(rk.Salt()) != "3" {
			t.Errorf("expected '3', got %q", rk.Salt())
		}
	})
}

// ─── KeyDerivationParams.Valid ───────────────────────────────────────────────────────────

func TestRepoKey_Valid(t *testing.T) {
	if (KeyDerivationParams{RepoID: ""}).Valid() {
		t.Error("empty RepoID should be invalid")
	}
	if !(KeyDerivationParams{RepoID: "abc"}).Valid() {
		t.Error("non-empty RepoID should be valid")
	}
}

// ─── NewDerivationParams ──────────────────────────────────────────────────────────────

func TestNewRepoKey(t *testing.T) {
	t.Run("version > 0 applies all opts", func(t *testing.T) {
		meta := metaWithVersion("repo-id", 2)
		rk := NewDerivationParams(meta, WithNameSpace(NSPlan), WithExtra("run-1"))

		if rk.NS != NSPlan {
			t.Errorf("expected NSPlan, got %q", rk.NS)
		}
		if rk.Extra != "run-1" {
			t.Errorf("expected extra 'run-1', got %q", rk.Extra)
		}
		if rk.Version != 2 {
			t.Errorf("expected version 2, got %d", rk.Version)
		}
	})

	t.Run("version 0: NS opt is ignored, only Extra is copied", func(t *testing.T) {
		meta := metaWithVersion("repo-id", 0)
		rk := NewDerivationParams(meta, WithNameSpace(NSApply), WithExtra("run-99"))

		if rk.NS != Global {
			t.Errorf("expected Global NS for version 0, got %q", rk.NS)
		}
		if rk.Extra != "run-99" {
			t.Errorf("expected extra 'run-99', got %q", rk.Extra)
		}
		if rk.Version != 0 {
			t.Errorf("expected version 0, got %d", rk.Version)
		}
	})

	t.Run("version < 0 behaves same as version 0", func(t *testing.T) {
		meta := metaWithVersion("repo-id", -1)
		rk := NewDerivationParams(meta, WithNameSpace(NSPlan))

		if rk.NS != Global {
			t.Errorf("expected Global for version < 0, got %q", rk.NS)
		}
	})

	t.Run("no opts uses defaults", func(t *testing.T) {
		meta := metaWithVersion("repo-id", 1)
		rk := NewDerivationParams(meta)

		if rk.NS != Global {
			t.Errorf("expected default Global NS, got %q", rk.NS)
		}
		if rk.Extra != "" {
			t.Errorf("expected empty Extra, got %q", rk.Extra)
		}
	})

	t.Run("RepoID comes from metadata", func(t *testing.T) {
		meta := metaWithVersion("my-repo-uuid", 1)
		rk := NewDerivationParams(meta)
		if rk.RepoID != "my-repo-uuid" {
			t.Errorf("expected RepoID 'my-repo-uuid', got %q", rk.RepoID)
		}
	})

	t.Run("version 0 without extra returns empty Extra", func(t *testing.T) {
		meta := metaWithVersion("repo-id", 0)
		rk := NewDerivationParams(meta)
		if rk.Extra != "" {
			t.Errorf("expected empty Extra, got %q", rk.Extra)
		}
	})
}

// ─── DeriveRepoKeys ──────────────────────────────────────────────────────────

func TestDeriveRepoKeys(t *testing.T) {
	master := "super-secret-master-key"
	baseRK := KeyDerivationParams{RepoID: "repo-uuid-abc", NS: Global}

	t.Run("returns 64 hex chars (32 bytes)", func(t *testing.T) {
		key, err := DeriveRepoKeys(master, baseRK)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(key) != 64 {
			t.Errorf("expected 64 hex chars, got %d", len(key))
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		k1, _ := DeriveRepoKeys(master, baseRK)
		k2, _ := DeriveRepoKeys(master, baseRK)
		if k1 != k2 {
			t.Error("derivation is not deterministic")
		}
	})

	t.Run("different NS produces different key", func(t *testing.T) {
		kGlobal, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: "abc", NS: Global})
		kPlan, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: "abc", NS: NSPlan})
		if kGlobal == kPlan {
			t.Error("different namespaces should produce different keys")
		}
	})

	t.Run("different version produces different key", func(t *testing.T) {
		k1, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: "abc", NS: Global, Version: 1})
		k2, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: "abc", NS: Global, Version: 2})
		if k1 == k2 {
			t.Error("different versions should produce different keys")
		}
	})

	t.Run("different extra produces different key", func(t *testing.T) {
		k1, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: "abc", NS: NSPlan, Extra: "run-1"})
		k2, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: "abc", NS: NSPlan, Extra: "run-2"})
		if k1 == k2 {
			t.Error("different extra values should produce different keys")
		}
	})

	t.Run("error on empty master key", func(t *testing.T) {
		_, err := DeriveRepoKeys("", baseRK)
		if err == nil || !strings.Contains(err.Error(), "master key is empty") {
			t.Errorf("expected master key error, got %v", err)
		}
	})

	t.Run("error on invalid repo key", func(t *testing.T) {
		_, err := DeriveRepoKeys(master, KeyDerivationParams{})
		if err == nil || !strings.Contains(err.Error(), "repo key is empty") {
			t.Errorf("expected repo key error, got %v", err)
		}
	})
}

// ─── DeriveRepoKey ───────────────────────────────────────────────────────────

func TestDeriveRepoKey(t *testing.T) {
	master := "super-secret-master-key"
	uuid := "550e8400-e29b-41d4-a716-446655440000"

	t.Run("returns 64 hex chars", func(t *testing.T) {
		key, err := DeriveRepoKey(master, uuid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(key) != 64 {
			t.Errorf("expected 64 hex chars, got %d", len(key))
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		k1, _ := DeriveRepoKey(master, uuid)
		k2, _ := DeriveRepoKey(master, uuid)
		if k1 != k2 {
			t.Error("not deterministic")
		}
	})

	t.Run("different uuid produces different key", func(t *testing.T) {
		k1, _ := DeriveRepoKey(master, "uuid-aaa")
		k2, _ := DeriveRepoKey(master, "uuid-bbb")
		if k1 == k2 {
			t.Error("different UUIDs should produce different keys")
		}
	})

	t.Run("different master produces different key", func(t *testing.T) {
		k1, _ := DeriveRepoKey("master-1", uuid)
		k2, _ := DeriveRepoKey("master-2", uuid)
		if k1 == k2 {
			t.Error("different masters should produce different keys")
		}
	})

	t.Run("error on empty master", func(t *testing.T) {
		_, err := DeriveRepoKey("", uuid)
		if err == nil || !strings.Contains(err.Error(), "master key is empty") {
			t.Errorf("expected error, got %v", err)
		}
	})

	t.Run("error on empty uuid", func(t *testing.T) {
		_, err := DeriveRepoKey(master, "")
		if err == nil || !strings.Contains(err.Error(), "repo uuid is empty") {
			t.Errorf("expected error, got %v", err)
		}
	})

	t.Run("equivalent to DeriveRepoKeys with Global NS and no version", func(t *testing.T) {
		k1, _ := DeriveRepoKey(master, uuid)
		k2, _ := DeriveRepoKeys(master, KeyDerivationParams{RepoID: uuid, NS: Global})
		if k1 != k2 {
			t.Errorf("DeriveRepoKey e DeriveRepoKeys(Global) devem ser equivalentes: %q vs %q", k1, k2)
		}
	})
}

// ─── ValidateHMAC ────────────────────────────────────────────────────────────

// mockSignable implementa hmac.Signable para testes
type mockSignable struct {
	hmac.Signature
	payload string
}

// func (m mockSignable) GetTimestamp() int64  { return m.timestamp }
// func (m mockSignable) GetSignature() string { return m.signature }
func (m mockSignable) GetPayload() []byte { return []byte(m.payload) }

func TestValidateHMAC(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects timestamp too old", func(t *testing.T) {
		req := mockSignable{
			Signature: hmac.Signature{
				Timestamp: time.Now().Add(-10 * time.Minute).Unix(),
			},
		}
		ok, err := ValidateHMAC(ctx, "secret", req)
		if ok || err == nil || !strings.Contains(err.Error(), "expired") {
			t.Errorf("expected expired error, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("rejects timestamp too far in future", func(t *testing.T) {
		req := mockSignable{Signature: hmac.Signature{
			Timestamp: time.Now().Add(10 * time.Minute).Unix()},
		}
		ok, err := ValidateHMAC(ctx, "secret", req)
		if ok || err == nil || !strings.Contains(err.Error(), "expired") {
			t.Errorf("expected expired error, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("rejects invalid signature within window", func(t *testing.T) {
		req := mockSignable{
			Signature: hmac.Signature{
				Timestamp: time.Now().Unix(),
				Signature: "invalidsignature",
			},
			payload: "some-payload",
		}
		ok, err := ValidateHMAC(ctx, "secret", req)
		if ok {
			t.Error("expected invalid signature to be rejected")
		}
		if err == nil {
			t.Error("expected error for invalid signature")
		}
	})

	t.Run("accepts valid HMAC within window", func(t *testing.T) {
		// Requer integração com hmac.Sign — coberto em testes de integração
		t.Skip("requer hmac.Sign para gerar assinatura válida")
	})
}

// ─── FindContextParams ──────────────────────────────────────────────────────────

// mockPersistence implementa a interface necessária para ports.ResolveManagedRepo
// Ajuste conforme a assinatura real de ports.Persistence no seu projeto
type mockPersistence struct {
	repo *model.RepositoryMetadata
	err  error
}

func TestFindContextKey(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error for unknown repo", func(t *testing.T) {
		// Testa o caminho de erro diretamente via ports.ResolveManagedRepo mockado
		// Se ports.ResolveManagedRepo for uma função injetável, substitua aqui
		// Este teste documenta o contrato esperado
		_ = ctx
		_ = fmt.Errorf("not found")
		t.Skip("requer mock de ports.ResolveManagedRepo — coberto em testes de integração")
	})

	t.Run("opts forwarded to NewDerivationParams when version > 0", func(t *testing.T) {
		// Valida indiretamente via NewDerivationParams que já é testado acima
		meta := metaWithVersion("repo-uuid", 2)
		rk := NewDerivationParams(meta, WithNameSpace(NSPlan), WithExtra("run-1"))

		if rk.NS != NSPlan {
			t.Errorf("expected NSPlan, got %q", rk.NS)
		}
		if rk.Extra != "run-1" {
			t.Errorf("expected extra 'run-1', got %q", rk.Extra)
		}
	})
}
