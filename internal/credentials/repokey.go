package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ServerPlace/iac-controller/internal/core/model"
	"github.com/ServerPlace/iac-controller/internal/core/ports"
	"github.com/ServerPlace/iac-controller/pkg/log"
	"golang.org/x/crypto/hkdf"
	"io"
	"strconv"
)

type KeyDerivationParams struct {
	RepoID  string
	NS      KeyNamespace
	Version int
	Extra   string
}
type KeyNamespace string

const (
	Global  KeyNamespace = "repo"
	NSPlan  KeyNamespace = "plan"
	NSApply KeyNamespace = "apply"
)

func (k KeyDerivationParams) Info() string {
	if k.Extra != "" {
		return fmt.Sprintf("%s:%s:%s", k.NS, k.RepoID, k.Extra)
	}
	return fmt.Sprintf("%s:%s", k.NS, k.RepoID)
}

func (k KeyDerivationParams) Salt() []byte {
	if k.Version > 0 {
		return []byte(strconv.Itoa(k.Version))
	}
	return nil
}

func (k KeyDerivationParams) Valid() bool {
	return k.RepoID != ""
}

type KeyDerivationOptions func(*KeyDerivationParams)

func defaultKeyDerivationParams(metadata *model.RepositoryMetadata) *KeyDerivationParams {
	return &KeyDerivationParams{
		RepoID:  metadata.ID,
		Version: metadata.KeyVersion,
		NS:      Global,
	}
}

func WithNameSpace(ns KeyNamespace) KeyDerivationOptions {
	return func(c *KeyDerivationParams) {
		c.NS = ns
	}
}

func WithExtra(extra string) KeyDerivationOptions {
	return func(c *KeyDerivationParams) {
		c.Extra = extra
	}
}
func NewDerivationParams(metadata *model.RepositoryMetadata, opts ...KeyDerivationOptions) *KeyDerivationParams {
	rk := defaultKeyDerivationParams(metadata)
	for _, opt := range opts {
		opt(rk)
	}
	// TODO: remover branch quando todos os repos tiverem KeyVersion > 0
	if metadata.KeyVersion < 1 {
		rkOld := defaultKeyDerivationParams(metadata)
		rkOld.Extra = rk.Extra
		return rkOld
	}

	return rk
}

func DeriveRepoKeys(masterKey string, rk KeyDerivationParams) (string, error) {
	if masterKey == "" {
		return "", fmt.Errorf("master key is empty")
	}
	if !rk.Valid() {
		return "", fmt.Errorf("repo key is empty")
	}

	// O "info" garante que a chave derivada seja vinculada a este contexto específico
	info := []byte(rk.Info())

	// HKDF reader
	hkdfStream := hkdf.New(sha256.New, []byte(masterKey), rk.Salt(), info)

	// Lemos 32 bytes (256 bits) para a chave
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfStream, key); err != nil {
		return "", fmt.Errorf("failed to derive key: %w", err)
	}

	return hex.EncodeToString(key), nil
}

// DeriveRepoKey (K_r)
// Deriva uma chave única para o repositório baseada na Master Key (do Config) e no UUID do Persistence.
// K_r = HKDF(hash=SHA256, secret=MasterKey, salt=nil, info="repo:"+RepoUUID)
func DeriveRepoKey(masterKey, repoUUID string) (string, error) {
	if masterKey == "" {
		return "", fmt.Errorf("master key is empty")
	}
	if repoUUID == "" {
		return "", fmt.Errorf("repo uuid is empty")
	}

	// O "info" garante que a chave derivada seja vinculada a este contexto específico
	info := []byte("repo:" + repoUUID)

	// HKDF reader
	hkdfStream := hkdf.New(sha256.New, []byte(masterKey), nil, info)

	// Lemos 32 bytes (256 bits) para a chave
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfStream, key); err != nil {
		return "", fmt.Errorf("failed to derive key: %w", err)
	}

	return hex.EncodeToString(key), nil
}

func FindContextParams(ctx context.Context, persistence ports.Persistence, managedRepoIdentifier string, opts ...KeyDerivationOptions) (*KeyDerivationParams, error) {
	logger := log.FromContext(ctx)
	managedRepoMetadata, err := ports.ResolveManagedRepo(ctx, persistence, managedRepoIdentifier)
	if err != nil {
		logger.Warn().Str("repo_identifier", managedRepoIdentifier).Msg("Persistence not found")
		return nil, fmt.Errorf("unauthorized repository")
	}

	// 3. Deriva K_r baseado no ID nativo (não no nome)
	return NewDerivationParams(managedRepoMetadata, opts...), nil
}
