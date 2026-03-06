package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
)

// Signature is an embeddable struct that provides HMAC signature support
// with mandatory timestamp for anti-replay protection.
// Embed it in any request struct to make it verifiable via [Verify].
//
//	type RegisterPlanRequest struct {
//	    hmac.Signature
//	    Persistence       string `json:"repo"`
//	    PlanOutput string `json:"plan_output"`
//	}
type Signature struct {
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature" hmac:"-"`
}

// SignatureBytes returns the signature as a byte slice.
func (s Signature) SignatureBytes() []byte {
	return []byte(s.Signature)
}

func (s Signature) SignatureString() string {
	return s.Signature
}

// GetTimestamp returns the request timestamp for anti-replay validation.
func (s Signature) GetTimestamp() int64 {
	return s.Timestamp
}

// Signable is the minimal interface for HMAC-verifiable structs.
// Any struct embedding [Signature] satisfies this automatically.
type Signable interface {
	SignatureString() string
	SignatureBytes() []byte
	GetTimestamp() int64
}

// Verify checks whether the HMAC-SHA256 signature of req is valid for the given signing key.
// Fields tagged with `hmac:"-"` are excluded from the payload computation.
func Verify[T Signable](signingKey []byte, req T) (bool, error) {
	payload, err := BuildPayload(req)
	if err != nil {
		return false, fmt.Errorf("building payload: %w", err)
	}
	mac := computeHMAC256Bytes(signingKey, payload)
	sig, err := hex.DecodeString(req.SignatureString())
	if err != nil {
		return false, fmt.Errorf("decoding signature: %w", err)
	}
	return hmac.Equal(mac, sig), nil
}

// Sign computes the HMAC-SHA256 signature for req using the given signing key
// and returns the raw signature bytes.
func Sign[T Signable](signingKey []byte, req T) (string, error) {
	payload, err := BuildPayload(req)
	if err != nil {
		return "", fmt.Errorf("error building payload: %w", err)
	}
	return computeHMAC256Hex(signingKey, payload), nil
}

// BuildPayload produces a canonical JSON representation of the struct fields,
// excluding any field tagged with `hmac:"-"`.
//
// The canonical form is a JSON object with keys sorted alphabetically,
// produced by json.Marshal over a map[string]any. This guarantees:
//   - Deterministic key ordering (Go sorts map keys in json.Marshal)
//   - Proper escaping of special characters in values
//   - Empty fields are always included (no omitempty)
//   - No ambiguity from custom serialization formats
func BuildPayload(req any) ([]byte, error) {
	m, err := extractFieldsToMap(reflect.ValueOf(req))
	if err != nil {
		return nil, err
	}

	return json.Marshal(m)
}

func extractFieldsToMap(v reflect.Value) (map[string]any, error) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", v.Kind())
	}

	t := v.Type()
	m := make(map[string]any)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields.
		if !field.IsExported() {
			continue
		}

		// Skip fields tagged with hmac:"-".
		if field.Tag.Get("hmac") == "-" {
			continue
		}

		// Flatten embedded (anonymous) structs recursively.
		if field.Anonymous {
			embedded, err := extractFieldsToMap(v.Field(i))
			if err != nil {
				return nil, fmt.Errorf("embedded field %s: %w", field.Name, err)
			}
			for k, val := range embedded {
				m[k] = val
			}
			continue
		}

		m[field.Name] = v.Field(i).Interface()
	}

	return m, nil
}

func computeHMAC256Bytes(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil) // 32 bytes
}

func computeHMAC256Hex(key, data []byte) string {
	return hex.EncodeToString(computeHMAC256Bytes(key, data))
}
