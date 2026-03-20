package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{
			name:   "empty header",
			header: "",
			ok:     false,
		},
		{
			name:   "missing bearer prefix",
			header: "Basic abc",
			ok:     false,
		},
		{
			name:   "bearer without token",
			header: "Bearer",
			ok:     false,
		},
		{
			name:   "valid bearer token",
			header: "Bearer test-token",
			want:   "test-token",
			ok:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractBearerToken(tt.header)
			if ok != tt.ok {
				t.Fatalf("ok mismatch: got=%v want=%v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("token mismatch: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestBearerTokenMiddleware_DisabledWhenTokenEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BearerTokenMiddleware(""))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusOK)
	}
}

func TestBearerTokenMiddleware_MissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BearerTokenMiddleware("secret-token"))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusUnauthorized)
	}
	if body := w.Body.String(); body != "{\"error\":\"unauthorized\"}" {
		t.Fatalf("body mismatch: got=%s", body)
	}
}

func TestBearerTokenMiddleware_WrongToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BearerTokenMiddleware("secret-token"))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusUnauthorized)
	}
	if body := w.Body.String(); body != "{\"error\":\"unauthorized\"}" {
		t.Fatalf("body mismatch: got=%s", body)
	}
}

func TestBearerTokenMiddleware_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BearerTokenMiddleware("secret-token"))

	handlerCalled := false
	r.GET("/protected", func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d", w.Code, http.StatusOK)
	}
	if !handlerCalled {
		t.Fatalf("handler should be called")
	}
}
