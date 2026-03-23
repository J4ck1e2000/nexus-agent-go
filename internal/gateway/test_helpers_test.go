package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGatewayTestRouter(t *testing.T, proxyClient *http.Client) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "gateway-test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	seedTestUser(t, db, "admin", "admin123", RoleAdmin)
	seedTestUser(t, db, "user", "user123", RoleUser)

	r := gin.New()
	h := NewHandler(NewConfigStore(db), NewAuthService(db, "test-secret"), VersionInfo{})
	if proxyClient != nil {
		h.proxyClient = proxyClient
	}
	h.RegisterAPIRoutes(r)
	return r
}

func seedTestUser(t *testing.T, db *gorm.DB, username, password string, role UserRole) {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	user := User{Username: username, PasswordHash: string(hashed), Role: role}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
}

func loginAndGetToken(t *testing.T, r *gin.Engine, username, password string) string {
	t.Helper()
	payload := map[string]string{"username": username, "password": password}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal login payload failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: status=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse login response failed: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("token should not be empty")
	}
	return resp.Token
}

func authorizedRequest(method, path string, body io.Reader, token string) *http.Request {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}
