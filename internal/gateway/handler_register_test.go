package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister_Success(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	payload := `{"username":"new-user","password":"newpass123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusCreated, w.Body.String())
	}

	var body struct {
		User struct {
			ID       uint   `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if body.User.ID == 0 {
		t.Fatal("id should not be empty")
	}
	if body.User.Username != "new-user" {
		t.Fatalf("username mismatch: got=%q want=%q", body.User.Username, "new-user")
	}
	if body.User.Role != string(RoleUser) {
		t.Fatalf("role mismatch: got=%q want=%q", body.User.Role, RoleUser)
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	payload := `{"username":"user","password":"user12345"}`
	req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	if body := w.Body.String(); body != `{"error":"user_already_exists"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestRegister_EmptyUsername(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	payload := `{"username":"   ","password":"valid123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if body := w.Body.String(); body != `{"error":"invalid_username"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	payload := `{"username":"short-pass-user","password":"12345"}`
	req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if body := w.Body.String(); body != `{"error":"password_too_short"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestRegister_InvalidUsernameCharacter(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	payload := `{"username":"bad user","password":"valid123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if body := w.Body.String(); body != `{"error":"invalid_username"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestRegister_UserRoleShouldBeUser(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	registerPayload := `{"username":"role-check-user","password":"rolecheck123","role":"admin"}`
	registerReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(registerPayload))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	r.ServeHTTP(registerResp, registerReq)

	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status mismatch: got=%d want=%d body=%s", registerResp.Code, http.StatusCreated, registerResp.Body.String())
	}

	loginPayload := `{"username":"role-check-user","password":"rolecheck123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	r.ServeHTTP(loginResp, loginReq)

	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status mismatch: got=%d want=%d body=%s", loginResp.Code, http.StatusOK, loginResp.Body.String())
	}

	var loginBody struct {
		User struct {
			Role string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(loginResp.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("unmarshal login response failed: %v", err)
	}
	if loginBody.User.Role != string(RoleUser) {
		t.Fatalf("role mismatch: got=%q want=%q", loginBody.User.Role, RoleUser)
	}
}

func TestRegister_UserCanLoginSuccessfully(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	registerPayload := `{"username":"login-check-user","password":"logincheck123"}`
	registerReq := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewBufferString(registerPayload))
	registerReq.Header.Set("Content-Type", "application/json")
	registerResp := httptest.NewRecorder()
	r.ServeHTTP(registerResp, registerReq)

	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status mismatch: got=%d want=%d body=%s", registerResp.Code, http.StatusCreated, registerResp.Body.String())
	}

	token := loginAndGetToken(t, r, "login-check-user", "logincheck123")
	if token == "" {
		t.Fatal("token should not be empty")
	}
}
