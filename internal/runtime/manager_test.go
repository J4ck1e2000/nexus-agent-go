package runtime

import (
	"errors"
	"sync"
	"testing"
)

func TestRunLifecycle(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	if _, err := manager.StartRun("run-1", "run-1", 1, "alice", "user", []string{"get_node_metrics"}, 2); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	if err := manager.MarkRunning("run-1"); err != nil {
		t.Fatalf("mark running failed: %v", err)
	}

	if err := manager.AuthorizeTool("run-1", "get_node_metrics"); err != nil {
		t.Fatalf("authorize tool failed: %v", err)
	}
	if err := manager.AuthorizeTool("run-1", "unknown_tool"); !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("expected tool not allowed, got %v", err)
	}

	// Budget of 2: one consumed above, one more allowed, then exhausted.
	if err := manager.AuthorizeTool("run-1", "get_node_metrics"); err != nil {
		t.Fatalf("second authorize failed: %v", err)
	}
	if err := manager.AuthorizeTool("run-1", "get_node_metrics"); !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("expected budget exhaustion, got %v", err)
	}

	if err := manager.MarkTerminal("run-1", RunStateCompleted); err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}
	if err := manager.MarkTerminal("run-1", RunStateCancelled); !errors.Is(err, ErrRunAlreadyFinished) {
		t.Fatalf("late cancel should be rejected, got %v", err)
	}
}

func TestDuplicateRunRejected(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	if _, err := manager.StartRun("run-1", "run-1", 1, "alice", "user", nil, 1); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	if _, err := manager.StartRun("run-1", "run-1", 1, "alice", "user", nil, 1); !errors.Is(err, ErrRunAlreadyActive) {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestBeginCancelTransitions(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	if _, err := manager.StartRun("run-1", "run-1", 1, "alice", "user", nil, 1); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	// Cancelling a CREATED run terminates it immediately.
	if err := manager.BeginCancel("run-1"); err != nil {
		t.Fatalf("begin cancel failed: %v", err)
	}
	if err := manager.AuthorizeTool("run-1", "get_node_metrics"); err == nil {
		t.Fatal("tool calls on cancelled run must be rejected")
	}

	if _, err := manager.StartRun("run-2", "run-2", 1, "alice", "user", nil, 1); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	_ = manager.MarkRunning("run-2")
	if err := manager.BeginCancel("run-2"); err != nil {
		t.Fatalf("begin cancel running failed: %v", err)
	}
	// CANCELLING rejects new tool calls and completes as CANCELLED.
	if err := manager.AuthorizeTool("run-2", "get_node_metrics"); !errors.Is(err, ErrRunNotActive) {
		t.Fatalf("expected run not active, got %v", err)
	}
	if err := manager.EnsureTerminal("run-2", RunStateFailed); err != nil {
		t.Fatalf("ensure terminal failed: %v", err)
	}
	if state := manager.runs["run-2"].State(); state != RunStateCancelled {
		t.Fatalf("cancelling run should end cancelled, got %s", state)
	}
}

func TestEnsureTerminalIsIdempotent(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	if _, err := manager.StartRun("run-1", "run-1", 1, "alice", "user", nil, 1); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	_ = manager.MarkRunning("run-1")
	if err := manager.MarkTerminal("run-1", RunStateCompleted); err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}
	// Deferred cleanup must not overwrite the terminal state.
	_ = manager.EnsureTerminal("run-1", RunStateFailed)
	if state := manager.runs["run-1"].State(); state != RunStateCompleted {
		t.Fatalf("terminal state was overwritten: %s", state)
	}
}

func TestConcurrentTerminalTransitions(t *testing.T) {
	manager := NewManager()
	defer manager.Close()

	if _, err := manager.StartRun("run-1", "run-1", 1, "alice", "user", nil, 100); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	_ = manager.MarkRunning("run-1")

	const workers = 16
	var wg sync.WaitGroup
	completions := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			switch i % 2 {
			case 0:
				err = manager.MarkTerminal("run-1", RunStateCompleted)
			default:
				err = manager.BeginCancel("run-1")
			}
			if err == nil {
				completions <- "won"
			}
		}(i)
	}
	wg.Wait()
	close(completions)

	wins := 0
	for range completions {
		wins++
	}
	if wins == 0 {
		t.Fatal("expected completion or cancellation to win")
	}
	if state := manager.runs["run-1"].State(); state != RunStateCompleted {
		t.Fatalf("completion should be terminal after concurrent cancellation, got %s", state)
	}
}
