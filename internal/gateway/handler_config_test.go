package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestLogin_SuccessAndFailure(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	successPayload := `{"username":"admin","password":"admin123"}`
	successReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(successPayload))
	successReq.Header.Set("Content-Type", "application/json")
	successResp := httptest.NewRecorder()
	r.ServeHTTP(successResp, successReq)

	if successResp.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", successResp.Code, http.StatusOK, successResp.Body.String())
	}

	var successBody struct {
		Token string `json:"token"`
		User  struct {
			Role string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(successResp.Body.Bytes(), &successBody); err != nil {
		t.Fatalf("unmarshal success response failed: %v", err)
	}
	if successBody.Token == "" {
		t.Fatal("token should not be empty")
	}
	if successBody.User.Role != string(RoleAdmin) {
		t.Fatalf("role mismatch: got=%q want=%q", successBody.User.Role, RoleAdmin)
	}

	failPayload := `{"username":"admin","password":"wrong"}`
	failReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(failPayload))
	failReq.Header.Set("Content-Type", "application/json")
	failResp := httptest.NewRecorder()
	r.ServeHTTP(failResp, failReq)

	if failResp.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", failResp.Code, http.StatusUnauthorized, failResp.Body.String())
	}
	if body := failResp.Body.String(); body != `{"error":"invalid_credentials"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestMe_ReturnsCurrentUserRole(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	userToken := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodGet, "/api/me", nil, userToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if body.Username != "user" {
		t.Fatalf("username mismatch: got=%q want=%q", body.Username, "user")
	}
	if body.Role != string(RoleUser) {
		t.Fatalf("role mismatch: got=%q want=%q", body.Role, RoleUser)
	}
}

func TestProtectedRoutes_RequireAuthentication(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusUnauthorized, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"unauthorized"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestUser_CannotCreateOrDeleteNode(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	userToken := loginAndGetToken(t, r, "user", "user123")

	createBody := `{"name":"node-admin","url":"http://127.0.0.1:8005"}`
	createReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(createBody), adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("admin create status mismatch: got=%d want=%d body=%s", createResp.Code, http.StatusCreated, createResp.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created response failed: %v", err)
	}

	forbiddenCreateReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-user","url":"http://127.0.0.1:9000"}`), userToken)
	forbiddenCreateReq.Header.Set("Content-Type", "application/json")
	forbiddenCreateResp := httptest.NewRecorder()
	r.ServeHTTP(forbiddenCreateResp, forbiddenCreateReq)

	if forbiddenCreateResp.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", forbiddenCreateResp.Code, http.StatusForbidden, forbiddenCreateResp.Body.String())
	}
	if body := forbiddenCreateResp.Body.String(); body != `{"error":"forbidden"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}

	deleteReq := authorizedRequest(http.MethodDelete, "/api/config/"+strconv.FormatInt(created.ID, 10), nil, userToken)
	deleteResp := httptest.NewRecorder()
	r.ServeHTTP(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", deleteResp.Code, http.StatusForbidden, deleteResp.Body.String())
	}
	if body := deleteResp.Body.String(); body != `{"error":"forbidden"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdmin_CanCreateAndDeleteNode(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	userToken := loginAndGetToken(t, r, "user", "user123")

	createReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-a","url":"http://127.0.0.1:8005"}`), adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", createResp.Code, http.StatusCreated, createResp.Body.String())
	}

	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response failed: %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("invalid created id: %d", created.ID)
	}

	listReq := authorizedRequest(http.MethodGet, "/api/config", nil, userToken)
	listResp := httptest.NewRecorder()
	r.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status mismatch: got=%d want=%d body=%s", listResp.Code, http.StatusOK, listResp.Body.String())
	}

	var listed []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response failed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list mismatch: %+v", listed)
	}

	deleteReq := authorizedRequest(http.MethodDelete, "/api/config/"+strconv.FormatInt(created.ID, 10), nil, adminToken)
	deleteResp := httptest.NewRecorder()
	r.ServeHTTP(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusOK {
		t.Fatalf("delete status mismatch: got=%d want=%d body=%s", deleteResp.Code, http.StatusOK, deleteResp.Body.String())
	}
	if body := deleteResp.Body.String(); body != `{"status":"success"}` {
		t.Fatalf("delete response mismatch: got=%q", body)
	}

	listAfterDeleteReq := authorizedRequest(http.MethodGet, "/api/config", nil, adminToken)
	listAfterDeleteResp := httptest.NewRecorder()
	r.ServeHTTP(listAfterDeleteResp, listAfterDeleteReq)
	if listAfterDeleteResp.Code != http.StatusOK {
		t.Fatalf("list after delete status mismatch: got=%d want=%d", listAfterDeleteResp.Code, http.StatusOK)
	}
	if body := listAfterDeleteResp.Body.String(); body != "[]" {
		t.Fatalf("list after delete mismatch: got=%q", body)
	}
}
