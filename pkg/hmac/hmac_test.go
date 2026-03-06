package hmac

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type simpleRequest struct {
	Signature
	Repo     string `json:"repo"`
	PRNumber int    `json:"pr_number"`
}

type requestWithIgnoredField struct {
	Signature
	Repo   string `json:"repo"`
	Secret string `json:"secret" hmac:"-"`
	Name   string `json:"name"`
}

type requestWithUnexported struct {
	Signature
	Repo    string `json:"repo"`
	private string //nolint:unused
}

func TestBuildPayload_SkipsHMACTaggedFields(t *testing.T) {
	req := simpleRequest{
		Signature: Signature{Signature: "should-be-ignored", Timestamp: 1700000000},
		Repo:      "my-repo",
		PRNumber:  42,
	}

	payload, err := BuildPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// json.Marshal sorts map keys alphabetically
	// Signature string is excluded (hmac:"-"), Timestamp is included
	want := mustJSON(t, map[string]any{
		"PRNumber":  42,
		"Repo":      "my-repo",
		"Timestamp": 1700000000,
	})

	if string(payload) != want {
		t.Errorf("payload = %s, want %s", payload, want)
	}
}

func TestBuildPayload_SkipsMultipleTaggedFields(t *testing.T) {
	req := requestWithIgnoredField{
		Signature: Signature{Signature: "sig", Timestamp: 100},
		Repo:      "repo-a",
		Secret:    "super-secret",
		Name:      "plan-1",
	}

	payload, err := BuildPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := mustJSON(t, map[string]any{
		"Name":      "plan-1",
		"Repo":      "repo-a",
		"Timestamp": 100,
	})

	if string(payload) != want {
		t.Errorf("payload = %s, want %s", payload, want)
	}
}

func TestBuildPayload_SkipsUnexportedFields(t *testing.T) {
	req := requestWithUnexported{
		Signature: Signature{Signature: "sig", Timestamp: 100},
		Repo:      "repo-b",
	}

	payload, err := BuildPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := mustJSON(t, map[string]any{
		"Repo":      "repo-b",
		"Timestamp": 100,
	})

	if string(payload) != want {
		t.Errorf("payload = %s, want %s", payload, want)
	}
}

func TestBuildPayload_Pointer(t *testing.T) {
	req := &simpleRequest{
		Signature: Signature{Signature: "sig", Timestamp: 1234567890},
		Repo:      "ptr-repo",
		PRNumber:  7,
	}

	payload, err := BuildPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := mustJSON(t, map[string]any{
		"PRNumber":  7,
		"Repo":      "ptr-repo",
		"Timestamp": 1234567890,
	})

	if string(payload) != want {
		t.Errorf("payload = %s, want %s", payload, want)
	}
}

func TestBuildPayload_ErrorOnNonStruct(t *testing.T) {
	_, err := BuildPayload("not a struct")
	if err == nil {
		t.Fatal("expected error for non-struct input")
	}
}

func TestBuildPayload_ErrorOnNilPointer(t *testing.T) {
	var req *simpleRequest
	_, err := BuildPayload(req)
	if err == nil {
		t.Fatal("expected error for nil pointer")
	}
}

func TestSignAndVerify(t *testing.T) {
	key := []byte("test-secret-key")

	req := simpleRequest{
		Signature: Signature{Timestamp: 1700000000},
		Repo:      "infra-repo",
		PRNumber:  10,
	}

	sig, err := Sign(key, &req)
	fmt.Print(sig)
	if err != nil {
		t.Fatalf("Sign error: %v", err)
	}

	req.Signature.Signature = sig

	ok, err := Verify(key, &req)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !ok {
		t.Error("Verify returned false, want true")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	key := []byte("correct-key")

	req := simpleRequest{
		Signature: Signature{Timestamp: 1700000000},
		Repo:      "infra-repo",
		PRNumber:  10,
	}

	sig, err := Sign(key, &req)
	if err != nil {
		t.Fatalf("Sign error: %v", err)
	}

	req.Signature.Signature = string(sig)

	ok, err := Verify([]byte("wrong-key"), &req)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if ok {
		t.Error("Verify returned true with wrong key, want false")
	}
}

func TestVerify_TamperedField(t *testing.T) {
	key := []byte("test-key")

	req := simpleRequest{
		Signature: Signature{Timestamp: 1700000000},
		Repo:      "original-repo",
		PRNumber:  10,
	}

	sig, err := Sign(key, &req)
	if err != nil {
		t.Fatalf("Sign error: %v", err)
	}

	req.Signature.Signature = string(sig)
	req.Repo = "tampered-repo"

	ok, err := Verify(key, &req)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if ok {
		t.Error("Verify returned true after tampering, want false")
	}
}

func TestSignatureBytes(t *testing.T) {
	s := Signature{Signature: "abc123", Timestamp: 100}
	got := s.SignatureBytes()
	want := []byte("abc123")

	if string(got) != string(want) {
		t.Errorf("SignatureBytes() = %q, want %q", got, want)
	}
}

func TestGetTimestamp(t *testing.T) {
	s := Signature{Timestamp: 1700000000}
	if got := s.GetTimestamp(); got != 1700000000 {
		t.Errorf("GetTimestamp() = %d, want %d", got, 1700000000)
	}
}

// These two structs have the same fields in different declaration order.
// Canonical payload must be identical for both.
type orderA struct {
	Signature
	Zebra string `json:"zebra"`
	Alpha string `json:"alpha"`
}

type orderB struct {
	Signature
	Alpha string `json:"alpha"`
	Zebra string `json:"zebra"`
}

func TestBuildPayload_CanonicalizationOrder(t *testing.T) {
	a := orderA{Zebra: "z-val", Alpha: "a-val"}
	b := orderB{Alpha: "a-val", Zebra: "z-val"}

	payloadA, err := BuildPayload(a)
	if err != nil {
		t.Fatalf("BuildPayload(orderA): %v", err)
	}

	payloadB, err := BuildPayload(b)
	if err != nil {
		t.Fatalf("BuildPayload(orderB): %v", err)
	}

	if string(payloadA) != string(payloadB) {
		t.Errorf("payloads differ:\n  orderA: %s\n  orderB: %s", payloadA, payloadB)
	}
}

type emptyFieldRequest struct {
	Signature
	Name string `json:"name"`
	Repo string `json:"repo"`
}

func TestBuildPayload_EmptyFieldsIncluded(t *testing.T) {
	req := emptyFieldRequest{
		Name: "",
		Repo: "my-repo",
	}

	payload, err := BuildPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := mustJSON(t, map[string]any{
		"Name":      "",
		"Repo":      "my-repo",
		"Timestamp": 0,
	})

	if string(payload) != want {
		t.Errorf("payload = %s, want %s", payload, want)
	}
}

func TestBuildPayload_EmptyFieldsProduceDifferentSignatures(t *testing.T) {
	req1 := emptyFieldRequest{Name: "abc", Repo: ""}
	req2 := emptyFieldRequest{Name: "", Repo: "abc"}

	p1, _ := BuildPayload(req1)
	p2, _ := BuildPayload(req2)

	if string(p1) == string(p2) {
		t.Errorf("different field values produced same payload: %s", p1)
	}
}

type specialCharsRequest struct {
	Signature
	Value string `json:"value"`
}

func TestBuildPayload_SpecialCharactersEscaped(t *testing.T) {
	req := specialCharsRequest{
		Value: "line1\nline2|pipe\"quote",
	}

	payload, err := BuildPayload(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// json.Marshal handles escaping of \n, ", etc.
	want := mustJSON(t, map[string]any{
		"Value":     "line1\nline2|pipe\"quote",
		"Timestamp": 0,
	})

	if string(payload) != want {
		t.Errorf("payload = %s, want %s", payload, want)
	}
}

// mustJSON is a test helper that marshals v to JSON or fails the test.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return string(b)
}

type RegisterPlanRequest struct {
	Signature
	Repo       string
	PlanOutput string
}

type WithIgnoredField struct {
	Signature
	Repo string

	// Deve ser ignorado no payload
	Transient string `hmac:"-"`
}

type WithWeirdChars struct {
	Signature
	Repo       string
	PlanOutput string
	Note       string
}

type BadSignatureReq struct {
	Signature
	Repo string
}

// força SignatureString() a retornar algo não-hex (para cobrir erro no Verify)
func (b BadSignatureReq) SignatureString() string { return "zzzz-not-hex" }

type NotAStruct int

func TestSignThenVerify_OK_ValueReceiver(t *testing.T) {
	key := []byte("super-secret-key")

	req := RegisterPlanRequest{
		Signature: Signature{
			Timestamp: 1700000000,
		},
		Repo:       "org/repo",
		PlanOutput: "no changes",
	}

	sig, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	// seta a assinatura no request e verifica
	req.Signature.Signature = sig

	ok, err := Verify(key, req)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected signature to verify, got ok=false")
	}

	// sanity: assinatura é hex válida e com 32 bytes
	raw, err := hex.DecodeString(sig)
	if err != nil {
		t.Fatalf("signature is not valid hex: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("expected 32-byte HMAC, got %d", len(raw))
	}
}

func TestSignThenVerify_OK_PointerReceiver(t *testing.T) {
	key := []byte("super-secret-key")

	req := &RegisterPlanRequest{
		Signature:  Signature{Timestamp: 1700000001},
		Repo:       "org/repo",
		PlanOutput: "apply me",
	}

	sig, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}
	req.Signature.Signature = sig

	ok, err := Verify(key, req)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
}

func TestVerify_Fails_WhenFieldChanges(t *testing.T) {
	key := []byte("super-secret-key")

	req := RegisterPlanRequest{
		Signature:  Signature{Timestamp: 1700000002},
		Repo:       "org/repo",
		PlanOutput: "original",
	}

	sig, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}
	req.Signature.Signature = sig

	// muda um campo que entra no payload
	req.PlanOutput = "tampered"

	ok, err := Verify(key, req)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false after tampering")
	}
}

func TestVerify_Fails_WhenSigningKeyChanges(t *testing.T) {
	key := []byte("super-secret-key")
	otherKey := []byte("other-key")

	req := RegisterPlanRequest{
		Signature:  Signature{Timestamp: 1700000003},
		Repo:       "org/repo",
		PlanOutput: "stable",
	}

	sig, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}
	req.Signature.Signature = sig

	ok, err := Verify(otherKey, req)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false with different key")
	}
}

func TestSign_Stable_WhenIgnoredFieldChanges(t *testing.T) {
	key := []byte("super-secret-key")

	req := WithIgnoredField{
		Signature: Signature{Timestamp: 1700000004},
		Repo:      "org/repo",
		Transient: "temp-1",
	}

	sig1, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	// muda apenas o campo ignorado (hmac:"-")
	req.Transient = "temp-2"

	sig2, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}

	if sig1 != sig2 {
		t.Fatalf("expected signatures to match when only ignored field changes; sig1=%s sig2=%s", sig1, sig2)
	}

	// E Verify com sig1 deve passar mesmo com Transient alterado
	req.Signature.Signature = sig1
	ok, err := Verify(key, req)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true with ignored field tamper")
	}
}

func TestVerify_ReturnsError_WhenSignatureIsNotHex(t *testing.T) {
	key := []byte("super-secret-key")

	req := BadSignatureReq{
		Signature: Signature{Timestamp: 1700000005, Signature: "zzzz-not-hex"},
		Repo:      "org/repo",
	}

	ok, err := Verify(key, req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if ok {
		t.Fatalf("expected ok=false on error")
	}
	if !strings.Contains(err.Error(), "decoding signature") {
		t.Fatalf("expected decoding signature error, got: %v", err)
	}
}

func TestSign_ReturnsError_WhenNilPointer(t *testing.T) {
	key := []byte("super-secret-key")

	var req *RegisterPlanRequest = nil
	_, err := Sign(key, req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	// O erro deve vir do BuildPayload -> extractFieldsToMap(nil pointer)
	if !strings.Contains(err.Error(), "building payload") {
		t.Fatalf("expected building payload in error, got: %v", err)
	}
}

func TestVerify_ReturnsError_WhenNilPointer(t *testing.T) {
	key := []byte("super-secret-key")

	var req *RegisterPlanRequest = nil
	ok, err := Verify(key, req)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if ok {
		t.Fatalf("expected ok=false on error")
	}
	if !strings.Contains(err.Error(), "building payload") {
		t.Fatalf("expected building payload in error, got: %v", err)
	}
}

func TestBuildPayload_ReturnsError_WhenNotStruct(t *testing.T) {
	_, err := BuildPayload(NotAStruct(10))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "expected struct") {
		t.Fatalf("expected 'expected struct' error, got: %v", err)
	}
}

func TestSignThenVerify_OK_WithSpecialCharacters(t *testing.T) {
	key := []byte("super-secret-key")

	req := WithWeirdChars{
		Signature:  Signature{Timestamp: 1700000006},
		Repo:       `org/"repo"\n`,
		PlanOutput: "linha1\nlinha2\t☃",
		Note:       `{"json":"ish","x":1}`,
	}

	sig, err := Sign(key, req)
	if err != nil {
		t.Fatalf("Sign returned error: %v", err)
	}
	req.Signature.Signature = sig

	ok, err := Verify(key, req)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
}
