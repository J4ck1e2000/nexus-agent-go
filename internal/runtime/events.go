package runtime

import (
	"encoding/json"
	"time"
)

// Runtime event types exchanged with the Pi Runtime.
const (
	EventRunStarted         = "run.started"
	EventMessageDelta       = "message.delta"
	EventMessageCompleted   = "message.completed"
	EventToolStarted        = "tool.started"
	EventToolCompleted      = "tool.completed"
	EventRunCompleted       = "run.completed"
	EventRunFailed          = "run.failed"
	EventRunCancelled       = "run.cancelled"
	EventBudgetExceeded     = "budget_exceeded"
	EventRuntimeUnavailable = "runtime_unavailable"
)

// RunStartedPayload is empty today; kept for forward compatibility.
type RunStartedPayload struct{}

// MessageDeltaPayload carries one incremental text chunk.
type MessageDeltaPayload struct {
	Text string `json:"text"`
}

// MessageCompletedPayload carries the authoritative text of one assistant message.
type MessageCompletedPayload struct {
	Text string `json:"text"`
}

// ToolStartedPayload announces one tool invocation.
type ToolStartedPayload struct {
	ToolCallID string         `json:"tool_call_id"`
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

// ToolResultMeta carries gateway freshness and truncation facts into the run stream.
type ToolResultMeta struct {
	ObservedAtUnix  int64 `json:"observed_at_unix,omitempty"`
	RetrievedAtUnix int64 `json:"retrieved_at_unix,omitempty"`
	Stale           bool  `json:"stale,omitempty"`
	Truncated       bool  `json:"truncated,omitempty"`
}

// ToolCompletedPayload reports one finished tool invocation.
type ToolCompletedPayload struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	OK         bool            `json:"ok"`
	Result     map[string]any  `json:"result,omitempty"`
	Meta       *ToolResultMeta `json:"meta,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// RunCompletedPayload is the final run output.
type RunCompletedPayload struct {
	Text         string `json:"text,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// RunFailedPayload explains a failed run.
type RunFailedPayload struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// RunCancelledPayload explains a cancelled run.
type RunCancelledPayload struct {
	Reason string `json:"reason,omitempty"`
}

// Event is the wire envelope of one runtime event.
type Event struct {
	ProtocolVersion string          `json:"protocol_version"`
	RunID           string          `json:"run_id"`
	Seq             int64           `json:"seq"`
	Type            string          `json:"type"`
	Timestamp       string          `json:"timestamp"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

// NewEvent builds one envelope with a normalized timestamp.
func NewEvent(runID, eventType string, seq int64, payload any) (Event, error) {
	raw := json.RawMessage("null")
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return Event{}, err
		}
		raw = encoded
	}
	return Event{
		ProtocolVersion: protocolVersion,
		RunID:           runID,
		Seq:             seq,
		Type:            eventType,
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		Payload:         raw,
	}, nil
}
