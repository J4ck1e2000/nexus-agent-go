package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client errors.
var (
	ErrRuntimeUnavailable = errors.New("pi runtime unavailable")
	ErrRunDuplicate       = errors.New("run already accepted by runtime")
	ErrRunBusy            = errors.New("session busy with another run")
	ErrStreamClosed       = errors.New("runtime stream closed before terminal event")
)

// StartRunRequest is the Go -> Pi run bootstrap payload (design §3.1).
type StartRunRequest struct {
	ProtocolVersion string        `json:"protocol_version"`
	RunID           string        `json:"run_id"`
	SessionID       string        `json:"session_id"`
	Input           StartRunInput `json:"input"`
	Context         RunContext    `json:"context"`
	Policy          RunPolicy     `json:"policy"`
	ModelProfile    ModelProfile  `json:"model_profile"`
}

// StartRunInput carries the raw user question.
type StartRunInput struct {
	Message string `json:"message"`
}

// RunContext carries bounded recovery data for the runtime.
type RunContext struct {
	KnownNodes    []string          `json:"known_nodes,omitempty"`
	LocaleHint    string            `json:"locale_hint,omitempty"`
	RecentEntries []RunHistoryEntry `json:"recent_entries,omitempty"`
}

// RunHistoryEntry is one bounded prior conversation item (lossy first-version recovery).
type RunHistoryEntry struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RunPolicy constrains what the runtime may do.
type RunPolicy struct {
	EnabledToolNames []string `json:"enabled_tool_names"`
	DeadlineAt       string   `json:"deadline_at,omitempty"`
}

// ModelProfile selects the model on the runtime side.
type ModelProfile struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
}

// CancelRunRequest is the body for run cancellation.
type CancelRunRequest struct {
	Reason string `json:"reason,omitempty"`
}

// Client talks to the Pi Runtime over HTTP + NDJSON.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient builds a runtime client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			// No global timeout: the NDJSON stream lives as long as the run.
			// Cancellation is enforced via request context and run deadline.
			Timeout: 0,
		},
	}
}

// Health probes the runtime process.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Nexus-Internal-Token", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: health status %d", ErrRuntimeUnavailable, resp.StatusCode)
	}
	return nil
}

// StartRun opens the NDJSON event stream for one run.
// credential is the gateway-signed run credential the runtime must present on
// tool callbacks; it travels as a header, never inside the JSON body.
// The returned readCloser must be closed by the caller; events are parsed with
// a scanner that handles chunks spanning multiple reads and 1MB lines.
func (c *Client) StartRun(ctx context.Context, request StartRunRequest, credential string) (<-chan Event, func(), error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/runs", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("X-Nexus-Internal-Token", c.token)
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrRuntimeUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusConflict:
			return nil, nil, ErrRunDuplicate
		case http.StatusTooManyRequests:
			return nil, nil, ErrRunBusy
		default:
			return nil, nil, fmt.Errorf("%w: start run status %d", ErrRuntimeUnavailable, resp.StatusCode)
		}
	}

	events := make(chan Event, 32)
	closeStream := func() { _ = resp.Body.Close() }

	go func() {
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, defaultScannerBuffer), maxScannerBuffer)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var event Event
			if err := json.Unmarshal(line, &event); err != nil {
				// A malformed line must not kill the stream; log via error event path.
				continue
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, closeStream, nil
}

// Terminal reports whether the event ends a run.
func Terminal(eventType string) bool {
	switch eventType {
	case EventRunCompleted, EventRunFailed, EventRunCancelled:
		return true
	}
	return false
}

// CancelRun aborts one run on the runtime side.
func (c *Client) CancelRun(ctx context.Context, runID, reason string) error {
	body, err := json.Marshal(CancelRunRequest{Reason: reason})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/runs/"+runID+"/cancel", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nexus-Internal-Token", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cancel run status %d", resp.StatusCode)
	}
	return nil
}
