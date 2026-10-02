package runtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Run lifecycle errors.
var (
	ErrRunNotFound        = errors.New("run not found")
	ErrRunAlreadyActive   = errors.New("run already active")
	ErrRunNotActive       = errors.New("run not active")
	ErrRunAlreadyFinished = errors.New("run already finished")
	ErrToolNotAllowed     = errors.New("tool not allowed for run")
)

// Run states. Terminal states are COMPLETED, FAILED, CANCELLED.
const (
	RunStateCreated    = "CREATED"
	RunStateRunning    = "RUNNING"
	RunStateCancelling = "CANCELLING"
	RunStateCompleted  = "COMPLETED"
	RunStateFailed     = "FAILED"
	RunStateCancelled  = "CANCELLED"
)

// RunRecord tracks one run accepted by the gateway.
type RunRecord struct {
	RunID          string
	SessionID      string
	UserID         int64
	Username       string
	Role           string
	AllowedTools   map[string]struct{}
	ToolCallBudget int

	mu         sync.Mutex
	state      string
	finishedAt time.Time
}

// State returns the current state atomically.
func (r *RunRecord) State() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// transition applies one state change; terminal states are immutable.
func (r *RunRecord) transition(target string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.state {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
		return r.state, ErrRunAlreadyFinished
	}
	r.state = target
	if target == RunStateCompleted || target == RunStateFailed || target == RunStateCancelled {
		r.finishedAt = time.Now()
	}
	return r.state, nil
}

// Manager keeps the active-run registry used by the executor (creation,
// lifecycle) and the tool gateway (authorization checks).
type Manager struct {
	mu     sync.RWMutex
	runs   map[string]*RunRecord
	stopGC chan struct{}
	gcOnce sync.Once
}

// NewManager creates the run manager and starts background GC of terminal runs.
func NewManager() *Manager {
	m := &Manager{
		runs:   make(map[string]*RunRecord),
		stopGC: make(chan struct{}),
	}
	go m.gcLoop()
	return m
}

// StartRun registers a new run; duplicate active run IDs are rejected.
func (m *Manager) StartRun(runID, sessionID string, userID int64, username, role string, allowedTools []string, toolBudget int) (*RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run_id required", ErrRunNotActive)
	}
	if toolBudget <= 0 {
		toolBudget = DefaultMaxToolCalls
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.runs[runID]; ok {
		if existing.State() != RunStateCompleted && existing.State() != RunStateFailed && existing.State() != RunStateCancelled {
			return nil, ErrRunAlreadyActive
		}
		// Terminal records linger only for the TTL window; treat as duplicate too.
		return nil, ErrRunAlreadyActive
	}

	tools := make(map[string]struct{}, len(allowedTools))
	for _, name := range allowedTools {
		normalized := strings.TrimSpace(name)
		if normalized != "" {
			tools[normalized] = struct{}{}
		}
	}
	record := &RunRecord{
		RunID:          runID,
		SessionID:      strings.TrimSpace(sessionID),
		UserID:         userID,
		Username:       username,
		Role:           role,
		AllowedTools:   tools,
		ToolCallBudget: toolBudget,
		state:          RunStateCreated,
	}
	m.runs[runID] = record
	return record, nil
}

// MarkRunning transitions CREATED -> RUNNING.
func (m *Manager) MarkRunning(runID string) error {
	record, ok := m.lookup(runID)
	if !ok {
		return ErrRunNotFound
	}
	_, err := record.transition(RunStateRunning)
	return err
}

// MarkTerminal applies a terminal transition; the first writer wins.
func (m *Manager) MarkTerminal(runID, state string) error {
	switch state {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
	default:
		return fmt.Errorf("unsupported terminal state %q", state)
	}
	record, ok := m.lookup(runID)
	if !ok {
		return ErrRunNotFound
	}
	_, err := record.transition(state)
	return err
}

// EnsureTerminal force-finishes a live run: a cancelling run ends as
// CANCELLED, anything else non-terminal takes the fallback state. Terminal
// records are left untouched, so the method is safe as a deferred cleanup.
func (m *Manager) EnsureTerminal(runID, fallback string) error {
	record, ok := m.lookup(runID)
	if !ok {
		return ErrRunNotFound
	}
	switch record.State() {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
		return ErrRunAlreadyFinished
	case RunStateCancelling:
		_, err := record.transition(RunStateCancelled)
		return err
	default:
		_, err := record.transition(fallback)
		return err
	}
}

// BeginCancel transitions a live run into CANCELLING and reports whether the
// caller should notify the runtime. Late cancellation after a terminal state
// returns ErrRunAlreadyFinished.
func (m *Manager) BeginCancel(runID string) error {
	record, ok := m.lookup(runID)
	if !ok {
		return ErrRunNotFound
	}
	switch record.State() {
	case RunStateCreated:
		_, err := record.transition(RunStateCancelled)
		return err
	case RunStateRunning:
		_, err := record.transition(RunStateCancelling)
		return err
	default:
		return ErrRunAlreadyFinished
	}
}

// AuthorizeTool verifies the run is live, the tool is allow-listed and the
// per-run budget still has room. It consumes one budget slot on success.
func (m *Manager) AuthorizeTool(runID, toolName string) error {
	record, ok := m.lookup(runID)
	if !ok {
		return ErrRunNotFound
	}
	record.mu.Lock()
	defer record.mu.Unlock()
	switch record.state {
	case RunStateRunning:
	case RunStateCreated:
		return ErrRunNotActive
	case RunStateCancelling:
		return ErrRunNotActive
	default:
		return ErrRunAlreadyFinished
	}
	if _, allowed := record.AllowedTools[toolName]; !allowed {
		return ErrToolNotAllowed
	}
	if record.ToolCallBudget <= 0 {
		return ErrToolNotAllowed
	}
	record.ToolCallBudget--
	return nil
}

// Close releases the GC goroutine; used by tests.
func (m *Manager) Close() {
	m.gcOnce.Do(func() { close(m.stopGC) })
}

func (m *Manager) lookup(runID string) (*RunRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.runs[runID]
	return record, ok
}

func (m *Manager) gcLoop() {
	ticker := time.NewTicker(managerGCInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopGC:
			return
		case now := <-ticker.C:
			m.gcTerminal(now)
		}
	}
}

func (m *Manager) gcTerminal(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for runID, record := range m.runs {
		switch record.State() {
		case RunStateCompleted, RunStateFailed, RunStateCancelled:
			record.mu.Lock()
			finished := record.finishedAt
			record.mu.Unlock()
			if !finished.IsZero() && now.Sub(finished) > runRecordTTL {
				delete(m.runs, runID)
			}
		}
	}
}
