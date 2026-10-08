package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNodeHistoryEndpointMergesRecentAndLongTermSamples(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seedTestUser(t, db, "user", "user123", RoleUser)
	store := NewConfigStore(db)
	if err := db.Create(&AgentNode{ID: 1, Name: "gpu-a", CollectorType: "ssh", SSHHost: "10.0.0.1", SSHPort: 22, SSHUser: "collector", CreatedBy: 1}).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(redisServer.Close)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	now := time.Now().UTC().Truncate(time.Second)
	stateStore := NewNodeStateStore(redisClient, "test", 10*time.Minute)
	metricHistory := NewNodeMetricHistoryStore(db)
	old := historyTestState(1, now.Add(-48*time.Hour).Unix(), NodeStatusOnline)
	if err := metricHistory.RecordNodeState(context.Background(), old); err != nil {
		t.Fatalf("write mysql history: %v", err)
	}
	recent := historyTestState(1, now.Add(-10*time.Minute).Unix(), NodeStatusOnline)
	if err := stateStore.SaveNodeState(context.Background(), recent); err != nil {
		t.Fatalf("write redis history: %v", err)
	}
	service := NewNodeStateService(store, stateStore, NodeStateServiceOptions{MetricHistoryStore: metricHistory})
	handler := NewHandler(store, NewAuthService(db, "history-test-secret"), VersionInfo{}, service)
	router := gin.New()
	handler.RegisterAPIRoutes(router)
	token := loginAndGetToken(t, router, "user", "user123")

	request := authorizedRequest(http.MethodGet, "/api/nodes/1/history?from="+strconv.FormatInt(now.Add(-72*time.Hour).Unix(), 10)+"&step_seconds=15&aggregation=peak", nil, token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", response.Code, response.Body.String())
	}
	var result NodeHistoryResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if result.RetentionDays != defaultMetricHistoryRetentionDays || result.StepSeconds != 15 || result.NodeID != 1 || len(result.Samples) != 2 {
		t.Fatalf("unexpected history response: %+v", result)
	}
	if result.Samples[0].TimestampUnix != old.LastPolledAtUnix || result.Samples[1].TimestampUnix != recent.LastPolledAtUnix {
		t.Fatalf("history samples are not merged in time order: %+v", result.Samples)
	}
}
