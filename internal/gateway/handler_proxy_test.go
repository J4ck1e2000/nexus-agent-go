package gateway

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupProxyTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	h := NewHandler(NewConfigStore(filepath.Join(t.TempDir(), "config.json")), VersionInfo{})
	h.RegisterAPIRoutes(r, func(c *gin.Context) { c.Next() })
	return r
}

func TestProxyRequest_ForwardsTargetAuthorization(t *testing.T) {
	r := setupProxyTestRouter(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("Authorization"); got != "Bearer secret-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/proxy?url="+url.QueryEscape(target.URL), nil)
	req.Header.Set("X-Target-Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "ok" {
		t.Fatalf("body mismatch: got=%q want=%q", body, "ok")
	}
}

func TestProxyRequest_RejectsInvalidTargetAuthorization(t *testing.T) {
	r := setupProxyTestRouter(t)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/proxy?url="+url.QueryEscape(target.URL), nil)
	req.Header.Set("X-Target-Authorization", "Basic abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusBadRequest)
	}
	if body := w.Body.String(); body != "{\"error\":\"invalid_target_authorization\"}" {
		t.Fatalf("body mismatch: got=%q", body)
	}
}
