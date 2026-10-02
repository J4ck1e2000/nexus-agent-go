package runtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// signPayload computes the HMAC-SHA256 authentication tag for run credentials.
func signPayload(payload []byte, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return mac.Sum(nil)
}

// Run credential errors.
var (
	ErrRunTokenInvalid = errors.New("run token invalid")
	ErrRunTokenExpired = errors.New("run token expired")
)

// RunCredential is the signed binding between a run and its owning user.
// The runtime presents it on every tool callback; the tool gateway verifies it
// and re-derives authorization facts instead of trusting runtime-claimed roles.
type RunCredential struct {
	RunID    string `json:"run_id"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

// SignRunCredential returns "<payload>.<hmac>" with a tamper-proof expiry.
func SignRunCredential(cred RunCredential, secret string) (string, error) {
	if strings.TrimSpace(cred.RunID) == "" {
		return "", fmt.Errorf("%w: run_id required", ErrRunTokenInvalid)
	}
	if cred.Exp == 0 {
		cred.Exp = time.Now().Add(DefaultRunTimeout + runRecordTTL).Unix()
	}
	payload, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}
	mac := signPayload(payload, secret)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	encodedSig := base64.RawURLEncoding.EncodeToString(mac)
	return encodedPayload + "." + encodedSig, nil
}

// VerifyRunCredential validates signature and expiry.
func VerifyRunCredential(token, secret string) (RunCredential, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RunCredential{}, ErrRunTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return RunCredential{}, ErrRunTokenInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return RunCredential{}, ErrRunTokenInvalid
	}
	if !hmac.Equal(signature, signPayload(payload, secret)) {
		return RunCredential{}, ErrRunTokenInvalid
	}
	var cred RunCredential
	if err := json.Unmarshal(payload, &cred); err != nil {
		return RunCredential{}, ErrRunTokenInvalid
	}
	if strings.TrimSpace(cred.RunID) == "" {
		return RunCredential{}, ErrRunTokenInvalid
	}
	if cred.Exp > 0 && time.Now().Unix() > cred.Exp {
		return RunCredential{}, ErrRunTokenExpired
	}
	return cred, nil
}
