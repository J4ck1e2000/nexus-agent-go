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

func TestIdleReservationRoutesPersistPerUserAndReturnMatchEdges(t *testing.T) {
	router := setupGatewayTestRouter(t, nil)
	adminToken := loginAndGetToken(t, router, "admin", "admin123")
	userToken := loginAndGetToken(t, router, "user", "user123")

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/idle-reservations", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized list status=%d", unauthorized.Code)
	}

	createBody := `{"name":"large run","filters":{"minFreeVramGb":40,"minFreeSystemMemoryGb":32,"gpuModel":"","processPolicy":"emptyOnly","nodeIds":[],"idleDurationMinutes":0},"notifyMode":"once","expiresInHours":24}`
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, authorizedRequest(http.MethodPost, "/api/idle-reservations", strings.NewReader(createBody), userToken))
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var created IdleReservationDTO
	if err := json.Unmarshal(createResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created reservation: %v", err)
	}
	if created.ID == 0 || created.Status != IdleReservationActive || created.Name != "large run" {
		t.Fatalf("unexpected created reservation: %+v", created)
	}

	evaluateBody, _ := json.Marshal(map[string]any{"matchingKeys": []string{"2:0"}})
	evaluateResp := httptest.NewRecorder()
	reservationPath := "/api/idle-reservations/" + strconv.FormatUint(uint64(created.ID), 10)
	router.ServeHTTP(evaluateResp, authorizedRequest(http.MethodPost, reservationPath+"/evaluate", bytes.NewReader(evaluateBody), userToken))
	if evaluateResp.Code != http.StatusOK {
		t.Fatalf("evaluate status=%d body=%s", evaluateResp.Code, evaluateResp.Body.String())
	}
	var evaluated IdleReservationEvaluation
	if err := json.Unmarshal(evaluateResp.Body.Bytes(), &evaluated); err != nil {
		t.Fatalf("decode evaluated reservation: %v", err)
	}
	if evaluated.Reservation.Status != IdleReservationCompleted || len(evaluated.NewMatchKeys) != 1 || evaluated.NewMatchKeys[0] != "2:0" {
		t.Fatalf("unexpected evaluation result: %+v", evaluated)
	}

	otherUserDelete := httptest.NewRecorder()
	router.ServeHTTP(otherUserDelete, authorizedRequest(http.MethodDelete, reservationPath, nil, adminToken))
	if otherUserDelete.Code != http.StatusNotFound {
		t.Fatalf("other user delete should be hidden as not found, status=%d body=%s", otherUserDelete.Code, otherUserDelete.Body.String())
	}

	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, authorizedRequest(http.MethodGet, "/api/idle-reservations", nil, userToken))
	var items []IdleReservationDTO
	if err := json.Unmarshal(listResp.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode reservation list: %v", err)
	}
	if listResp.Code != http.StatusOK || len(items) != 1 || items[0].Status != IdleReservationCompleted {
		t.Fatalf("user-owned reservation missing: status=%d items=%+v", listResp.Code, items)
	}
	adminList := httptest.NewRecorder()
	router.ServeHTTP(adminList, authorizedRequest(http.MethodGet, "/api/idle-reservations", nil, adminToken))
	var adminItems []IdleReservationDTO
	if err := json.Unmarshal(adminList.Body.Bytes(), &adminItems); err != nil {
		t.Fatalf("decode admin reservation list: %v", err)
	}
	if len(adminItems) != 0 {
		t.Fatalf("reservations should be isolated per user, got %+v", adminItems)
	}
}
