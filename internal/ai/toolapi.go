package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Tool gateway error codes exposed to the Pi runtime.
const (
	ToolErrInvalidArguments  = "INVALID_ARGUMENTS"
	ToolErrNodeNotFound      = "NODE_NOT_FOUND"
	ToolErrNodeNameRequired  = "NODE_NAME_REQUIRED"
	ToolErrNoNodesAvailable  = "NO_NODES_AVAILABLE"
	ToolErrProviderMissing   = "DATA_PROVIDER_MISSING"
	ToolErrKnowledgeDisabled = "KNOWLEDGE_DISABLED"
	ToolErrInternal          = "TOOL_INTERNAL"
	ToolErrOutputTooLarge    = "TOOL_OUTPUT_TOO_LARGE"
)

const (
	// ToolOutputMaxBytes caps the marshalled tool result returned to the runtime.
	ToolOutputMaxBytes = 256 * 1024
	// ToolMaxProcesses caps process lists inside tool results.
	ToolMaxProcesses = 64
	// ToolMaxCandidates caps ranking lists inside tool results.
	ToolMaxCandidates = 32
)

// ToolCallRequest is the body of one internal tool invocation.
type ToolCallRequest struct {
	RunID      string         `json:"run_id"`
	ToolCallID string         `json:"tool_call_id"`
	Arguments  map[string]any `json:"arguments"`
}

// ToolCallMeta reports freshness and truncation facts about the result.
type ToolCallMeta struct {
	ObservedAtUnix  int64 `json:"observed_at_unix,omitempty"`
	RetrievedAtUnix int64 `json:"retrieved_at_unix"`
	Stale           bool  `json:"stale,omitempty"`
	Truncated       bool  `json:"truncated,omitempty"`
}

// ToolCallError is the structured failure payload.
type ToolCallError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// ToolCallResponse is the structured result envelope.
type ToolCallResponse struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data,omitempty"`
	Meta  *ToolCallMeta  `json:"meta,omitempty"`
	Error *ToolCallError `json:"error,omitempty"`
}

// ToolDispatcher validates model-supplied arguments and executes the read-only
// Toolbox tools on behalf of the Pi runtime.
type ToolDispatcher struct {
	toolbox   *Toolbox
	toolNames []string
}

// NewToolDispatcher builds the dispatcher over the shared toolbox.
func NewToolDispatcher(toolbox *Toolbox) *ToolDispatcher {
	names := []string{
		"get_node_metrics",
		"list_idle_nodes",
		"get_gpu_processes",
		"get_node_summary",
		"get_alert_history",
		"recommend_nodes_for_job",
		"explain_node_anomaly",
		"search_knowledge_base",
	}
	return &ToolDispatcher{toolbox: toolbox, toolNames: names}
}

// ToolNames returns the supported tool names in stable order.
func (d *ToolDispatcher) ToolNames() []string {
	return append([]string(nil), d.toolNames...)
}

// HasTool reports whether the tool name is dispatchable.
func (d *ToolDispatcher) HasTool(name string) bool {
	for _, supported := range d.toolNames {
		if supported == name {
			return true
		}
	}
	return false
}

// Dispatch validates arguments, runs the tool and shapes the response.
// Model-supplied arguments are treated as untrusted input.
func (d *ToolDispatcher) Dispatch(ctx context.Context, name string, args map[string]any) ToolCallResponse {
	started := time.Now()
	response := d.dispatch(ctx, name, args)
	if response.Meta == nil {
		response.Meta = &ToolCallMeta{RetrievedAtUnix: started.Unix()}
	} else if response.Meta.RetrievedAtUnix == 0 {
		response.Meta.RetrievedAtUnix = started.Unix()
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return ToolCallResponse{
			OK:   false,
			Meta: response.Meta,
			Error: &ToolCallError{
				Code:      ToolErrInternal,
				Message:   "tool result could not be encoded",
				Retryable: false,
			},
		}
	}
	if len(encoded) > ToolOutputMaxBytes {
		response.Meta.Truncated = true
		return ToolCallResponse{
			OK:   false,
			Meta: response.Meta,
			Error: &ToolCallError{
				Code:      ToolErrOutputTooLarge,
				Message:   fmt.Sprintf("tool result exceeds the %d-byte response limit", ToolOutputMaxBytes),
				Retryable: false,
			},
		}
	}
	return response
}

func (d *ToolDispatcher) dispatch(ctx context.Context, name string, args map[string]any) ToolCallResponse {
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case "get_node_metrics":
		nodeName, errResp := requireStringArg(args, "node_name")
		if errResp != nil {
			return *errResp
		}
		snapshot, err := d.toolbox.GetNodeMetrics(ctx, nodeName)
		if err != nil {
			return errorResponse(err)
		}
		data, truncated := capSnapshot(snapshot)
		return successResponse(map[string]any{"node": data}, snapshotMeta(snapshot), truncated)

	case "get_node_summary":
		nodeName, errResp := requireStringArg(args, "node_name")
		if errResp != nil {
			return *errResp
		}
		summary, err := d.toolbox.GetNodeSummary(ctx, nodeName)
		if err != nil {
			return errorResponse(err)
		}
		return successResponse(map[string]any{"summary": summary}, snapshotMetaFromAge(summary.DataAgeSec), false)

	case "get_gpu_processes":
		nodeName, errResp := requireStringArg(args, "node_name")
		if errResp != nil {
			return *errResp
		}
		snapshot, err := d.toolbox.GetNodeMetrics(ctx, nodeName)
		if err != nil {
			return errorResponse(err)
		}
		processes, err := d.toolbox.GetGPUProcesses(ctx, nodeName)
		if err != nil {
			return errorResponse(err)
		}
		if len(processes) > ToolMaxProcesses {
			processes = processes[:ToolMaxProcesses]
		}
		return successResponse(map[string]any{"processes": processes}, snapshotMeta(snapshot), false)

	case "list_idle_nodes":
		minFree, hasMin := asFloatPtr(args["min_free_vram_gb"])
		limit := asInt(args["limit"], 3)
		if limit <= 0 || limit > ToolMaxCandidates {
			limit = ToolMaxCandidates
		}
		var minFreePtr *float64
		if hasMin {
			minFreePtr = &minFree
		}
		candidates, err := d.toolbox.ListIdleNodes(ctx, minFreePtr, limit)
		if err != nil {
			return errorResponse(err)
		}
		return successResponse(map[string]any{"candidates": candidates}, nil, false)

	case "get_alert_history":
		window := asStringWithDefault(args["window"], "30m")
		if window != "30m" && window != "1h" {
			return invalidArgumentsResponse("window must be 30m or 1h")
		}
		nodeName := asString(args["node_name"])
		var nodeNamePtr *string
		if nodeName != "" {
			nodeNamePtr = &nodeName
		}
		result, err := d.toolbox.GetAlertHistory(ctx, nodeNamePtr, window)
		if err != nil {
			return errorResponse(err)
		}
		return successResponse(map[string]any{"history": result}, nil, false)

	case "recommend_nodes_for_job":
		requirement := parseJobRequirementArgs(args["requirements"])
		limit := asInt(args["limit"], 3)
		if limit <= 0 || limit > ToolMaxCandidates {
			limit = ToolMaxCandidates
		}
		candidates, err := d.toolbox.RecommendNodesForJob(ctx, requirement, limit)
		if err != nil {
			return errorResponse(err)
		}
		return successResponse(map[string]any{"candidates": candidates}, nil, false)

	case "explain_node_anomaly":
		nodeName, errResp := requireStringArg(args, "node_name")
		if errResp != nil {
			return *errResp
		}
		explanation, err := d.toolbox.ExplainNodeAnomaly(ctx, nodeName)
		if err != nil {
			return errorResponse(err)
		}
		return successResponse(map[string]any{"explanation": explanation}, nil, false)

	case "search_knowledge_base":
		query := asString(args["query"])
		if query == "" {
			return invalidArgumentsResponse("query is required")
		}
		if !d.toolbox.HasKnowledge() {
			return ToolCallResponse{
				OK:    false,
				Error: &ToolCallError{Code: ToolErrKnowledgeDisabled, Message: "knowledge retrieval is not enabled on the gateway", Retryable: false},
			}
		}
		limit := asInt(args["limit"], defaultKnowledgeTopK)
		if limit <= 0 || limit > 20 {
			limit = 20
		}
		hits, meta, err := d.toolbox.SearchKnowledge(ctx, query, limit)
		if err != nil {
			return errorResponse(err)
		}
		return successResponse(map[string]any{
			"hits":      summarizeKnowledgeHits(hits),
			"retrieval": meta,
		}, nil, false)

	default:
		return ToolCallResponse{
			OK:    false,
			Error: &ToolCallError{Code: ToolErrInvalidArguments, Message: fmt.Sprintf("unsupported tool: %s", name), Retryable: false},
		}
	}
}

func requireStringArg(args map[string]any, key string) (string, *ToolCallResponse) {
	value := asString(args[key])
	if value == "" {
		response := invalidArgumentsResponse(fmt.Sprintf("%s is required", key))
		return "", &response
	}
	return value, nil
}

func invalidArgumentsResponse(message string) ToolCallResponse {
	return ToolCallResponse{
		OK:    false,
		Error: &ToolCallError{Code: ToolErrInvalidArguments, Message: message, Retryable: false},
	}
}

func errorResponse(err error) ToolCallResponse {
	code := ToolErrInternal
	message := err.Error()
	switch {
	case errors.Is(err, ErrNodeNotFound):
		code = ToolErrNodeNotFound
	case errors.Is(err, ErrNodeNameRequired):
		code = ToolErrNodeNameRequired
	case errors.Is(err, ErrNoNodesAvailable):
		code = ToolErrNoNodesAvailable
	case errors.Is(err, ErrClusterDataProviderNil):
		code = ToolErrProviderMissing
	}
	return ToolCallResponse{
		OK:    false,
		Error: &ToolCallError{Code: code, Message: message, Retryable: false},
	}
}

func successResponse(data map[string]any, meta *ToolCallMeta, truncated bool) ToolCallResponse {
	if meta == nil {
		meta = &ToolCallMeta{RetrievedAtUnix: time.Now().Unix()}
	}
	meta.Truncated = meta.Truncated || truncated
	return ToolCallResponse{OK: true, Data: data, Meta: meta}
}

func snapshotMeta(snapshot NodeSnapshot) *ToolCallMeta {
	observed := snapshot.CollectedAtUnix
	if observed == 0 {
		observed = snapshot.LastPolledAtUnix
	}
	stale := false
	if snapshot.DataAgeSec != nil {
		stale = *snapshot.DataAgeSec > stalePenaltyThresholdSec
	}
	return &ToolCallMeta{
		ObservedAtUnix:  observed,
		RetrievedAtUnix: time.Now().Unix(),
		Stale:           stale,
	}
}

func snapshotMetaFromAge(dataAgeSec *float64) *ToolCallMeta {
	meta := &ToolCallMeta{RetrievedAtUnix: time.Now().Unix()}
	if dataAgeSec != nil {
		meta.Stale = *dataAgeSec > stalePenaltyThresholdSec
	}
	return meta
}

// capSnapshot bounds oversized snapshot sections and reports truncation.
func capSnapshot(snapshot NodeSnapshot) (map[string]any, bool) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return map[string]any{"error": err.Error()}, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{"error": err.Error()}, false
	}
	truncated := false
	if processes, ok := decoded["processes"].([]any); ok && len(processes) > ToolMaxProcesses {
		decoded["processes"] = processes[:ToolMaxProcesses]
		truncated = true
	}
	if gpus, ok := decoded["gpus"].([]any); ok && len(gpus) > ToolMaxCandidates {
		decoded["gpus"] = gpus[:ToolMaxCandidates]
		truncated = true
	}
	return decoded, truncated
}

// ToolNames returns the dispatcher's supported names (package-level helper).
func ToolNames(dispatcher *ToolDispatcher) []string {
	if dispatcher == nil {
		return nil
	}
	names := dispatcher.ToolNames()
	sort.Strings(names)
	return names
}
