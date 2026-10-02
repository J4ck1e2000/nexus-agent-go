package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStartRunParsesNDJSONAcrossChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nexus-Internal-Token") != "shared-secret" {
			t.Errorf("missing internal token header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if credential := r.Header.Get("Authorization"); credential != "Bearer cred-token" {
			t.Errorf("missing run credential header, got %q", credential)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// One event split across writes, plus a malformed line that must be skipped.
		event1, _ := NewEvent("run-1", EventRunStarted, 1, RunStartedPayload{})
		raw1, _ := json.Marshal(event1)
		half := len(raw1) / 2
		_, _ = w.Write(raw1[:half])
		flusher.Flush()
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write(raw1[half:])
		_, _ = w.Write([]byte("\nnot-json\n"))
		flusher.Flush()

		delta, _ := NewEvent("run-1", EventMessageDelta, 2, MessageDeltaPayload{Text: "你好，世界"})
		raw2, _ := json.Marshal(delta)
		_, _ = w.Write(raw2)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()

		completed, _ := NewEvent("run-1", EventRunCompleted, 3, RunCompletedPayload{Text: "done"})
		raw3, _ := json.Marshal(completed)
		_, _ = w.Write(raw3)
		_, _ = w.Write([]byte("\n"))
		flusher.Flush()
	}))
	defer server.Close()

	client := NewClient(server.URL, "shared-secret")
	events, closeStream, err := client.StartRun(context.Background(), StartRunRequest{RunID: "run-1"}, "cred-token")
	if err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	defer closeStream()

	var seen []Event
	for event := range events {
		seen = append(seen, event)
		if Terminal(event.Type) {
			return
		}
	}

	if len(seen) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(seen), seen)
	}
	if seen[0].Type != EventRunStarted || seen[1].Type != EventMessageDelta || seen[2].Type != EventRunCompleted {
		t.Fatalf("unexpected event sequence: %s -> %s -> %s", seen[0].Type, seen[1].Type, seen[2].Type)
	}
	var delta MessageDeltaPayload
	if err := json.Unmarshal(seen[1].Payload, &delta); err != nil || delta.Text != "你好，世界" {
		t.Fatalf("multi-byte delta payload corrupted: %q err=%v", delta.Text, err)
	}
}

func TestStartRunDuplicateReturnsConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	_, _, err := client.StartRun(context.Background(), StartRunRequest{RunID: "run-1"}, "")
	if !errors.Is(err, ErrRunDuplicate) {
		t.Fatalf("expected duplicate run error, got %v", err)
	}
}

func TestHealthReportsUnavailableOnBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	if err := client.Health(context.Background()); !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected runtime unavailable, got %v", err)
	}
}

func TestCancelRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-1/cancel" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body CancelRunRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Reason != "client_disconnected" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	if err := client.CancelRun(context.Background(), "run-1", "client_disconnected"); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if err := client.CancelRun(context.Background(), "run-missing", "x"); err == nil {
		t.Fatal("expected cancel failure for missing run")
	}
}

func TestEventEnvelopeShape(t *testing.T) {
	event, err := NewEvent("run-1", EventToolStarted, 4, ToolStartedPayload{ToolCallID: "call-1", ToolName: "get_node_metrics"})
	if err != nil {
		t.Fatalf("new event failed: %v", err)
	}
	if event.ProtocolVersion != ProtocolVersion() || event.Seq != 4 || event.RunID != "run-1" {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	if _, err := time.Parse(time.RFC3339Nano, event.Timestamp); err != nil {
		t.Fatalf("timestamp not RFC3339Nano: %v", err)
	}
	raw, _ := json.Marshal(event)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	for _, key := range []string{"protocol_version", "run_id", "seq", "type", "timestamp"} {
		if _, ok := decoded[key]; !ok {
			fmt.Printf("missing key %s in %s\n", key, raw)
			t.Fatalf("envelope missing key %s", key)
		}
	}
}
