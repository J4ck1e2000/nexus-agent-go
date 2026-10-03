package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestAdmin_CannotCreateDuplicateNodeURL(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	firstReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-a","url":"http://127.0.0.1:8005"}`), adminToken)
	firstReq.Header.Set("Content-Type", "application/json")
	firstResp := httptest.NewRecorder()
	r.ServeHTTP(firstResp, firstReq)
	if firstResp.Code != http.StatusCreated {
		t.Fatalf("first create status mismatch: got=%d want=%d body=%s", firstResp.Code, http.StatusCreated, firstResp.Body.String())
	}

	var firstCreated struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(firstResp.Body.Bytes(), &firstCreated); err != nil {
		t.Fatalf("unmarshal first create response failed: %v", err)
	}
	if firstCreated.URL != "http://127.0.0.1:8005" {
		t.Fatalf("first create normalized url mismatch: got=%q want=%q", firstCreated.URL, "http://127.0.0.1:8005")
	}

	duplicateSlashReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-b","url":"http://127.0.0.1:8005/"}`), adminToken)
	duplicateSlashReq.Header.Set("Content-Type", "application/json")
	duplicateSlashResp := httptest.NewRecorder()
	r.ServeHTTP(duplicateSlashResp, duplicateSlashReq)
	if duplicateSlashResp.Code != http.StatusConflict {
		t.Fatalf("duplicate slash create status mismatch: got=%d want=%d body=%s", duplicateSlashResp.Code, http.StatusConflict, duplicateSlashResp.Body.String())
	}
	if body := duplicateSlashResp.Body.String(); body != `{"error":"duplicate_node_url"}` {
		t.Fatalf("duplicate slash create body mismatch: got=%q", body)
	}

	duplicateExactReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-c","url":"http://127.0.0.1:8005"}`), adminToken)
	duplicateExactReq.Header.Set("Content-Type", "application/json")
	duplicateExactResp := httptest.NewRecorder()
	r.ServeHTTP(duplicateExactResp, duplicateExactReq)
	if duplicateExactResp.Code != http.StatusConflict {
		t.Fatalf("duplicate exact create status mismatch: got=%d want=%d body=%s", duplicateExactResp.Code, http.StatusConflict, duplicateExactResp.Body.String())
	}
	if body := duplicateExactResp.Body.String(); body != `{"error":"duplicate_node_url"}` {
		t.Fatalf("duplicate exact create body mismatch: got=%q", body)
	}

	listReq := authorizedRequest(http.MethodGet, "/api/config", nil, adminToken)
	listResp := httptest.NewRecorder()
	r.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status mismatch: got=%d want=%d body=%s", listResp.Code, http.StatusOK, listResp.Body.String())
	}

	var listed []struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 node after duplicate attempts, got=%d nodes=%+v", len(listed), listed)
	}
	if listed[0].URL != "http://127.0.0.1:8005" {
		t.Fatalf("listed normalized url mismatch: got=%q want=%q", listed[0].URL, "http://127.0.0.1:8005")
	}
}

func TestAdmin_CanCreateDifferentNodeURLs(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	createAReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-a","url":"http://127.0.0.1:8005"}`), adminToken)
	createAReq.Header.Set("Content-Type", "application/json")
	createAResp := httptest.NewRecorder()
	r.ServeHTTP(createAResp, createAReq)
	if createAResp.Code != http.StatusCreated {
		t.Fatalf("create node-a status mismatch: got=%d want=%d body=%s", createAResp.Code, http.StatusCreated, createAResp.Body.String())
	}

	createBReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(`{"name":"node-b","url":"http://127.0.0.1:8006"}`), adminToken)
	createBReq.Header.Set("Content-Type", "application/json")
	createBResp := httptest.NewRecorder()
	r.ServeHTTP(createBResp, createBReq)
	if createBResp.Code != http.StatusCreated {
		t.Fatalf("create node-b status mismatch: got=%d want=%d body=%s", createBResp.Code, http.StatusCreated, createBResp.Body.String())
	}

	listReq := authorizedRequest(http.MethodGet, "/api/config", nil, adminToken)
	listResp := httptest.NewRecorder()
	r.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status mismatch: got=%d want=%d body=%s", listResp.Code, http.StatusOK, listResp.Body.String())
	}

	var listed []struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response failed: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 nodes, got=%d nodes=%+v", len(listed), listed)
	}
}

func TestAdmin_SaveConfigArrayRejectsDuplicateURLs(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	duplicateArrayPayload := `[
		{"name":"node-a","url":"http://127.0.0.1:8005/"},
		{"name":"node-b","url":"http://127.0.0.1:8005"}
	]`
	saveReq := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(duplicateArrayPayload), adminToken)
	saveReq.Header.Set("Content-Type", "application/json")
	saveResp := httptest.NewRecorder()
	r.ServeHTTP(saveResp, saveReq)

	if saveResp.Code != http.StatusConflict {
		t.Fatalf("save array status mismatch: got=%d want=%d body=%s", saveResp.Code, http.StatusConflict, saveResp.Body.String())
	}
	if body := saveResp.Body.String(); body != `{"error":"duplicate_node_url"}` {
		t.Fatalf("save array response mismatch: got=%q", body)
	}

	listReq := authorizedRequest(http.MethodGet, "/api/config", nil, adminToken)
	listResp := httptest.NewRecorder()
	r.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status mismatch: got=%d want=%d body=%s", listResp.Code, http.StatusOK, listResp.Body.String())
	}
	if body := listResp.Body.String(); body != "[]" {
		t.Fatalf("expected empty list after rejected save, got=%q", body)
	}
}

func TestAdmin_CanCreateAndListSSHNode(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	createReq := authorizedRequest(http.MethodPost, "/api/config",
		bytes.NewBufferString(`{"name":"A6000-01","collector_type":"ssh","ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun","ssh_auth_type":"key"}`), adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	r.ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("create ssh node status mismatch: got=%d want=%d body=%s", createResp.Code, http.StatusCreated, createResp.Body.String())
	}

	var created struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		CollectorType string `json:"collector_type"`
		SSHHost       string `json:"ssh_host"`
		SSHPort       int    `json:"ssh_port"`
		SSHUser       string `json:"ssh_user"`
		SSHAuthType   string `json:"ssh_auth_type"`
		URL           string `json:"url"`
		HasURL        bool   `json:"-"`
	}
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response failed: %v", err)
	}
	created.HasURL = strings.Contains(createResp.Body.String(), `"url"`)
	if created.CollectorType != "ssh" || created.SSHHost != "10.0.0.15" || created.SSHPort != 22 || created.SSHUser != "renhaokun" || created.SSHAuthType != "key" {
		t.Fatalf("created ssh node mismatch: %+v", created)
	}
	if created.HasURL {
		t.Fatalf("ssh node response should omit url: %s", createResp.Body.String())
	}

	listReq := authorizedRequest(http.MethodGet, "/api/config", nil, adminToken)
	listResp := httptest.NewRecorder()
	r.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status mismatch: got=%d want=%d", listResp.Code, http.StatusOK)
	}

	var listed []struct {
		ID            int64  `json:"id"`
		CollectorType string `json:"collector_type"`
		SSHHost       string `json:"ssh_host"`
		URL           string `json:"url"`
		HasURL        bool   `json:"-"`
	}
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list failed: %v", err)
	}
	listed[0].HasURL = strings.Contains(listResp.Body.String(), `"url"`)
	if len(listed) != 1 || listed[0].CollectorType != "ssh" || listed[0].SSHHost != "10.0.0.15" {
		t.Fatalf("listed ssh nodes mismatch: %+v", listed)
	}
	if listed[0].HasURL {
		t.Fatalf("listed ssh node should omit url: %s", listResp.Body.String())
	}
}

func TestAdmin_SSHNodeValidationAndDuplicateEndpoint(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	postNode := func(body string) *httptest.ResponseRecorder {
		req := authorizedRequest(http.MethodPost, "/api/config", bytes.NewBufferString(body), adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
		return resp
	}

	// 非法 payload(缺 host)返回 invalid_payload。
	if resp := postNode(`{"name":"bad-ssh","collector_type":"ssh","ssh_port":22,"ssh_user":"ops"}`); resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid ssh payload status mismatch: got=%d body=%s", resp.Code, resp.Body.String())
	}
	if resp := postNode(`{"name":"bad-port","collector_type":"ssh","ssh_host":"10.0.0.1","ssh_port":0,"ssh_user":"ops"}`); resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid ssh port status mismatch: got=%d body=%s", resp.Code, resp.Body.String())
	}

	first := postNode(`{"name":"A6000-01","collector_type":"ssh","ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first ssh create status mismatch: got=%d body=%s", first.Code, first.Body.String())
	}

	dup := postNode(`{"name":"A6000-02","collector_type":"ssh","ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate ssh endpoint status mismatch: got=%d body=%s", dup.Code, dup.Body.String())
	}
	if body := dup.Body.String(); body != `{"error":"duplicate_node_endpoint"}` {
		t.Fatalf("duplicate ssh endpoint body mismatch: got=%q", body)
	}
}

func TestUser_CannotCreateSSHNode(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	userToken := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodPost, "/api/config",
		bytes.NewBufferString(`{"name":"A6000-01","collector_type":"ssh","ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`), userToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
}
