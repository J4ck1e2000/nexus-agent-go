package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestProxyRequest_ForwardsResponseWithoutAuthorizationHeader(t *testing.T) {
	var forwardedAuthorization string
	proxyClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			forwardedAuthorization = req.Header.Get("Authorization")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
			}, nil
		}),
	}

	r := setupGatewayTestRouter(t, proxyClient)
	token := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodGet, "/api/proxy?url="+url.QueryEscape("http://example.com/metrics"), nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusOK)
	}
	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type mismatch: got=%q want=%q", contentType, "application/json")
	}
	var payload map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if got := payload["status"]; got != "ok" {
		t.Fatalf("status mismatch in body: got=%q want=%q", got, "ok")
	}
	if forwardedAuthorization != "" {
		t.Fatalf("unexpected authorization header forwarded: %q", forwardedAuthorization)
	}
}

func TestProxyRequest_RejectsInvalidURL(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	token := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodGet, "/api/proxy?url="+url.QueryEscape("ftp://example.com"), nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusBadRequest)
	}
	if body := w.Body.String(); body != "{\"error\":\"invalid_url\"}" {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestProxyRequest_MissingURLParameter(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	token := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodGet, "/api/proxy", nil, token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusBadRequest)
	}
	if body := w.Body.String(); body != "{\"error\":\"Missing 'url' parameter\"}" {
		t.Fatalf("body mismatch: got=%q", body)
	}
}
