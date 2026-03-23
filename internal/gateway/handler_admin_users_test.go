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

type adminUserDTO struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func TestAdminUsers_AdminCanGetUserList(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	req := authorizedRequest(http.MethodGet, "/api/admin/users", nil, adminToken)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var users []adminUserDTO
	if err := json.Unmarshal(resp.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(users) < 2 {
		t.Fatalf("user list too short: got=%d body=%s", len(users), resp.Body.String())
	}
	if users[0].ID == 0 {
		t.Fatal("user id should not be empty")
	}
	if strings.Contains(resp.Body.String(), "password_hash") {
		t.Fatalf("response should not include password hash: body=%s", resp.Body.String())
	}

	filterReq := authorizedRequest(http.MethodGet, "/api/admin/users?q=ad", nil, adminToken)
	filterResp := httptest.NewRecorder()
	r.ServeHTTP(filterResp, filterReq)
	if filterResp.Code != http.StatusOK {
		t.Fatalf("filtered list status mismatch: got=%d want=%d body=%s", filterResp.Code, http.StatusOK, filterResp.Body.String())
	}
	var filtered []adminUserDTO
	if err := json.Unmarshal(filterResp.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("unmarshal filtered response failed: %v", err)
	}
	if len(filtered) == 0 {
		t.Fatalf("filtered list should not be empty: body=%s", filterResp.Body.String())
	}
	for _, item := range filtered {
		if !strings.Contains(strings.ToLower(item.Username), "ad") {
			t.Fatalf("filtered user mismatch: username=%q", item.Username)
		}
	}
}

func TestAdminUsers_NormalUserForbidden(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	userToken := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodGet, "/api/admin/users", nil, userToken)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"forbidden"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_UnauthorizedWithoutToken(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusUnauthorized, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"unauthorized"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_AdminCanCreateNormalUser(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	status, body := createAdminUser(t, r, adminToken, `{"username":"alice","password":"alice123","role":"user"}`)
	if status != http.StatusCreated {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", status, http.StatusCreated, body)
	}
	if strings.Contains(body, "password_hash") {
		t.Fatalf("response should not include password hash: body=%s", body)
	}

	var created adminUserDTO
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if created.Username != "alice" {
		t.Fatalf("username mismatch: got=%q want=%q", created.Username, "alice")
	}
	if created.Role != string(RoleUser) {
		t.Fatalf("role mismatch: got=%q want=%q", created.Role, RoleUser)
	}

	aliceToken := loginAndGetToken(t, r, "alice", "alice123")
	meReq := authorizedRequest(http.MethodGet, "/api/me", nil, aliceToken)
	meResp := httptest.NewRecorder()
	r.ServeHTTP(meResp, meReq)
	if meResp.Code != http.StatusOK {
		t.Fatalf("/api/me status mismatch: got=%d want=%d body=%s", meResp.Code, http.StatusOK, meResp.Body.String())
	}
	var me struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(meResp.Body.Bytes(), &me); err != nil {
		t.Fatalf("unmarshal /api/me failed: %v", err)
	}
	if me.Role != string(RoleUser) {
		t.Fatalf("role mismatch: got=%q want=%q", me.Role, RoleUser)
	}
}

func TestAdminUsers_AdminCanCreateAdminUser(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	status, body := createAdminUser(t, r, adminToken, `{"username":"ops-admin","password":"opsadmin123","role":"admin"}`)
	if status != http.StatusCreated {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", status, http.StatusCreated, body)
	}

	var created adminUserDTO
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if created.Role != string(RoleAdmin) {
		t.Fatalf("role mismatch: got=%q want=%q", created.Role, RoleAdmin)
	}

	newAdminToken := loginAndGetToken(t, r, "ops-admin", "opsadmin123")
	meReq := authorizedRequest(http.MethodGet, "/api/me", nil, newAdminToken)
	meResp := httptest.NewRecorder()
	r.ServeHTTP(meResp, meReq)
	if meResp.Code != http.StatusOK {
		t.Fatalf("/api/me status mismatch: got=%d want=%d body=%s", meResp.Code, http.StatusOK, meResp.Body.String())
	}
	var me struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(meResp.Body.Bytes(), &me); err != nil {
		t.Fatalf("unmarshal /api/me failed: %v", err)
	}
	if me.Role != string(RoleAdmin) {
		t.Fatalf("role mismatch: got=%q want=%q", me.Role, RoleAdmin)
	}
}

func TestAdminUsers_CreateUserForbiddenForNormalUser(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	userToken := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodPost, "/api/admin/users", bytes.NewBufferString(`{"username":"u1","password":"pass123","role":"user"}`), userToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"forbidden"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_CreateUserUnauthorizedWithoutToken(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", bytes.NewBufferString(`{"username":"u1","password":"pass123","role":"user"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusUnauthorized, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"unauthorized"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_CreateUserDuplicateUsernameReturnsConflict(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	status, body := createAdminUser(t, r, adminToken, `{"username":"user","password":"user12345","role":"user"}`)
	if status != http.StatusConflict {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", status, http.StatusConflict, body)
	}
	if body != `{"error":"user_already_exists"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_CreateUserInvalidRoleReturnsBadRequest(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	status, body := createAdminUser(t, r, adminToken, `{"username":"u1","password":"pass123","role":"owner"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", status, http.StatusBadRequest, body)
	}
	if body != `{"error":"invalid_role"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_CreateUserShortPasswordReturnsBadRequest(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	status, body := createAdminUser(t, r, adminToken, `{"username":"u-short","password":"12345","role":"user"}`)
	if status != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", status, http.StatusBadRequest, body)
	}
	if body != `{"error":"password_too_short"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_CreateUserResponseDoesNotExposePasswordHash(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	status, body := createAdminUser(t, r, adminToken, `{"username":"safe-user","password":"safeuser123","role":"user"}`)
	if status != http.StatusCreated {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", status, http.StatusCreated, body)
	}
	if strings.Contains(body, "password_hash") {
		t.Fatalf("response should not include password hash: body=%s", body)
	}
}

func TestAdminUsers_AdminCanDeleteNormalUser(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	users := listAdminUsers(t, r, adminToken)
	target := mustFindUserByUsername(t, users, "user")

	deleteReq := authorizedRequest(http.MethodDelete, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10), nil, adminToken)
	deleteResp := httptest.NewRecorder()
	r.ServeHTTP(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", deleteResp.Code, http.StatusOK, deleteResp.Body.String())
	}
	if body := deleteResp.Body.String(); body != `{"status":"success"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}

	after := listAdminUsers(t, r, adminToken)
	for _, item := range after {
		if item.ID == target.ID {
			t.Fatalf("deleted user still exists: %+v", item)
		}
	}

	loginPayload := `{"username":"user","password":"user123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	r.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user login status mismatch: got=%d want=%d body=%s", loginResp.Code, http.StatusUnauthorized, loginResp.Body.String())
	}
}

func TestAdminUsers_AdminCannotDeleteSelf(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	seedUser := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "user")

	promoteUserRole(t, r, adminToken, seedUser.ID, RoleAdmin)

	adminUser := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "admin")
	req := authorizedRequest(http.MethodDelete, "/api/admin/users/"+strconv.FormatUint(uint64(adminUser.ID), 10), nil, adminToken)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"cannot_delete_self"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_AdminCannotDeleteLastAdmin(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	adminUser := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "admin")

	req := authorizedRequest(http.MethodDelete, "/api/admin/users/"+strconv.FormatUint(uint64(adminUser.ID), 10), nil, adminToken)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"cannot_delete_last_admin"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_AdminCanPromoteUserToAdmin(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	target := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "user")

	req := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10)+"/role", bytes.NewBufferString(`{"role":"admin"}`), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var updated adminUserDTO
	if err := json.Unmarshal(resp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if updated.Role != string(RoleAdmin) {
		t.Fatalf("role mismatch: got=%q want=%q", updated.Role, RoleAdmin)
	}

	userToken := loginAndGetToken(t, r, "user", "user123")
	meReq := authorizedRequest(http.MethodGet, "/api/me", nil, userToken)
	meResp := httptest.NewRecorder()
	r.ServeHTTP(meResp, meReq)
	if meResp.Code != http.StatusOK {
		t.Fatalf("/api/me status mismatch: got=%d want=%d body=%s", meResp.Code, http.StatusOK, meResp.Body.String())
	}
	var me struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(meResp.Body.Bytes(), &me); err != nil {
		t.Fatalf("unmarshal me response failed: %v", err)
	}
	if me.Role != string(RoleAdmin) {
		t.Fatalf("role mismatch after promote: got=%q want=%q", me.Role, RoleAdmin)
	}
}

func TestAdminUsers_AdminCannotDowngradeSelf(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	seedUser := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "user")

	promoteUserRole(t, r, adminToken, seedUser.ID, RoleAdmin)
	adminUser := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "admin")

	req := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(adminUser.ID), 10)+"/role", bytes.NewBufferString(`{"role":"user"}`), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"cannot_downgrade_self"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_AdminCannotDowngradeLastAdmin(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	adminUser := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "admin")

	req := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(adminUser.ID), 10)+"/role", bytes.NewBufferString(`{"role":"user"}`), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"cannot_downgrade_last_admin"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_AdminCanResetUserPassword(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	target := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "user")

	resetReq := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10)+"/password", bytes.NewBufferString(`{"password":"newpass123"}`), adminToken)
	resetReq.Header.Set("Content-Type", "application/json")
	resetResp := httptest.NewRecorder()
	r.ServeHTTP(resetResp, resetReq)

	if resetResp.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resetResp.Code, http.StatusOK, resetResp.Body.String())
	}
	if body := resetResp.Body.String(); body != `{"status":"success"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}

	oldLoginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"username":"user","password":"user123"}`))
	oldLoginReq.Header.Set("Content-Type", "application/json")
	oldLoginResp := httptest.NewRecorder()
	r.ServeHTTP(oldLoginResp, oldLoginReq)
	if oldLoginResp.Code != http.StatusUnauthorized {
		t.Fatalf("old password should fail: got=%d want=%d body=%s", oldLoginResp.Code, http.StatusUnauthorized, oldLoginResp.Body.String())
	}

	newLoginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"username":"user","password":"newpass123"}`))
	newLoginReq.Header.Set("Content-Type", "application/json")
	newLoginResp := httptest.NewRecorder()
	r.ServeHTTP(newLoginResp, newLoginReq)
	if newLoginResp.Code != http.StatusOK {
		t.Fatalf("new password should pass: got=%d want=%d body=%s", newLoginResp.Code, http.StatusOK, newLoginResp.Body.String())
	}
}

func TestAdminUsers_ResetPasswordTooShortReturnsBadRequest(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	target := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "user")

	resetReq := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10)+"/password", bytes.NewBufferString(`{"password":"12345"}`), adminToken)
	resetReq.Header.Set("Content-Type", "application/json")
	resetResp := httptest.NewRecorder()
	r.ServeHTTP(resetResp, resetReq)

	if resetResp.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resetResp.Code, http.StatusBadRequest, resetResp.Body.String())
	}
	if body := resetResp.Body.String(); body != `{"error":"password_too_short"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_InvalidRoleReturnsBadRequest(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	target := mustFindUserByUsername(t, listAdminUsers(t, r, adminToken), "user")

	req := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(target.ID), 10)+"/role", bytes.NewBufferString(`{"role":"super-admin"}`), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"invalid_role"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestAdminUsers_NotFoundForDeleteRoleAndPassword(t *testing.T) {
	r := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")
	missingID := "999999"

	deleteReq := authorizedRequest(http.MethodDelete, "/api/admin/users/"+missingID, nil, adminToken)
	deleteResp := httptest.NewRecorder()
	r.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusNotFound {
		t.Fatalf("delete status mismatch: got=%d want=%d body=%s", deleteResp.Code, http.StatusNotFound, deleteResp.Body.String())
	}
	if body := deleteResp.Body.String(); body != `{"error":"user_not_found"}` {
		t.Fatalf("delete body mismatch: got=%q", body)
	}

	roleReq := authorizedRequest(http.MethodPatch, "/api/admin/users/"+missingID+"/role", bytes.NewBufferString(`{"role":"admin"}`), adminToken)
	roleReq.Header.Set("Content-Type", "application/json")
	roleResp := httptest.NewRecorder()
	r.ServeHTTP(roleResp, roleReq)
	if roleResp.Code != http.StatusNotFound {
		t.Fatalf("role status mismatch: got=%d want=%d body=%s", roleResp.Code, http.StatusNotFound, roleResp.Body.String())
	}
	if body := roleResp.Body.String(); body != `{"error":"user_not_found"}` {
		t.Fatalf("role body mismatch: got=%q", body)
	}

	passwordReq := authorizedRequest(http.MethodPatch, "/api/admin/users/"+missingID+"/password", bytes.NewBufferString(`{"password":"newpass123"}`), adminToken)
	passwordReq.Header.Set("Content-Type", "application/json")
	passwordResp := httptest.NewRecorder()
	r.ServeHTTP(passwordResp, passwordReq)
	if passwordResp.Code != http.StatusNotFound {
		t.Fatalf("password status mismatch: got=%d want=%d body=%s", passwordResp.Code, http.StatusNotFound, passwordResp.Body.String())
	}
	if body := passwordResp.Body.String(); body != `{"error":"user_not_found"}` {
		t.Fatalf("password body mismatch: got=%q", body)
	}
}

func listAdminUsers(t *testing.T, r http.Handler, adminToken string) []adminUserDTO {
	t.Helper()

	req := authorizedRequest(http.MethodGet, "/api/admin/users", nil, adminToken)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("list users status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var users []adminUserDTO
	if err := json.Unmarshal(resp.Body.Bytes(), &users); err != nil {
		t.Fatalf("unmarshal list users failed: %v", err)
	}
	return users
}

func mustFindUserByUsername(t *testing.T, users []adminUserDTO, username string) adminUserDTO {
	t.Helper()

	for _, user := range users {
		if user.Username == username {
			return user
		}
	}
	t.Fatalf("user %q not found in %+v", username, users)
	return adminUserDTO{}
}

func promoteUserRole(t *testing.T, r http.Handler, adminToken string, userID uint, targetRole UserRole) {
	t.Helper()

	payload := `{"role":"` + string(targetRole) + `"}`
	req := authorizedRequest(http.MethodPatch, "/api/admin/users/"+strconv.FormatUint(uint64(userID), 10)+"/role", bytes.NewBufferString(payload), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("promote role failed: status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func createAdminUser(t *testing.T, r http.Handler, adminToken, payload string) (int, string) {
	t.Helper()

	req := authorizedRequest(http.MethodPost, "/api/admin/users", bytes.NewBufferString(payload), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp.Code, resp.Body.String()
}
