package runtime

import (
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyRunCredential(t *testing.T) {
	credential := RunCredential{
		RunID:    "run-abc",
		UserID:   7,
		Username: "admin",
		Role:     "admin",
		Exp:      time.Now().Add(time.Minute).Unix(),
	}
	token, err := SignRunCredential(credential, "secret")
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	verified, err := VerifyRunCredential(token, "secret")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if verified.RunID != credential.RunID || verified.UserID != credential.UserID || verified.Role != credential.Role {
		t.Fatalf("credential mismatch: %+v", verified)
	}
}

func TestVerifyRunCredentialRejectsTampering(t *testing.T) {
	credential := RunCredential{RunID: "run-abc", Exp: time.Now().Add(time.Minute).Unix()}
	token, err := SignRunCredential(credential, "secret")
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	if _, err := VerifyRunCredential(token+"x", "secret"); err != ErrRunTokenInvalid {
		t.Fatalf("expected invalid token error, got %v", err)
	}
	if _, err := VerifyRunCredential(token, "other-secret"); err != ErrRunTokenInvalid {
		t.Fatalf("expected invalid token error for wrong secret, got %v", err)
	}
}

func TestVerifyRunCredentialRejectsExpired(t *testing.T) {
	credential := RunCredential{RunID: "run-abc", Exp: time.Now().Add(-time.Minute).Unix()}
	token, err := SignRunCredential(credential, "secret")
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if _, err := VerifyRunCredential(token, "secret"); err != ErrRunTokenExpired {
		t.Fatalf("expected expired token error, got %v", err)
	}
}

func TestSignRunCredentialRequiresRunID(t *testing.T) {
	if _, err := SignRunCredential(RunCredential{}, "secret"); err == nil {
		t.Fatal("expected error for missing run id")
	}
}

func TestVerifyRunCredentialRejectsMalformed(t *testing.T) {
	inputs := []string{"", "no-dot", "a.b.c", strings.Repeat("x", 10) + "." + strings.Repeat("y", 10)}
	for _, input := range inputs {
		if _, err := VerifyRunCredential(input, "secret"); err != ErrRunTokenInvalid {
			t.Fatalf("expected invalid token for %q, got %v", input, err)
		}
	}
}
