package collector

import (
	"context"
	"testing"

	"nexus-agent-go/internal/model"
)

type stubCollector struct {
	name string
}

func (s stubCollector) Collect(ctx context.Context, node model.AgentConfig) (model.SystemMetrics, int64, error) {
	return model.SystemMetrics{}, 0, nil
}

type pointerStubCollector struct{}

func (*pointerStubCollector) Collect(context.Context, model.AgentConfig) (model.SystemMetrics, int64, error) {
	return model.SystemMetrics{}, 0, nil
}
func TestCollectorRouter_CollectorFor(t *testing.T) {
	agent := stubCollector{name: "agent"}
	ssh := stubCollector{name: "ssh"}

	router := NewCollectorRouter(agent, ssh)

	tests := []struct {
		name      string
		collector string
		want      stubCollector
		wantErr   bool
	}{
		{
			name:      "empty collector type defaults to agent",
			collector: "",
			want:      agent,
		},
		{
			name:      "explicit agent",
			collector: model.CollectorTypeAgent,
			want:      agent,
		},
		{
			name:      "ssh",
			collector: model.CollectorTypeSSH,
			want:      ssh,
		},
		{
			name:      "unknown type rejected",
			collector: "snmp",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := router.CollectorFor(model.AgentConfig{CollectorType: tt.collector})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for collector type %q", tt.collector)
				}
				return
			}
			if err != nil {
				t.Fatalf("CollectorFor failed: %v", err)
			}
			if got != NodeMetricsCollector(tt.want) {
				t.Fatalf("collector mismatch: got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestCollectorRouter_TypedNilCollectorsRejected(t *testing.T) {
	var nilCollector *pointerStubCollector
	router := NewCollectorRouter(nilCollector, nilCollector)

	for _, collectorType := range []string{model.CollectorTypeAgent, model.CollectorTypeSSH} {
		got, err := router.CollectorFor(model.AgentConfig{CollectorType: collectorType})
		if err == nil {
			t.Fatalf("expected missing collector error for %q", collectorType)
		}
		if got != nil {
			t.Fatalf("expected nil collector for %q, got %T", collectorType, got)
		}
	}
}
func TestCollectorRouter_MissingCollectorRejected(t *testing.T) {
	agentOnly := NewCollectorRouter(stubCollector{name: "agent"}, nil)
	if _, err := agentOnly.CollectorFor(model.AgentConfig{CollectorType: model.CollectorTypeSSH}); err == nil {
		t.Fatal("expected error when ssh collector is not configured")
	}

	sshOnly := NewCollectorRouter(nil, stubCollector{name: "ssh"})
	if _, err := sshOnly.CollectorFor(model.AgentConfig{CollectorType: model.CollectorTypeAgent}); err == nil {
		t.Fatal("expected error when agent collector is not configured")
	}

	var nilRouter *CollectorRouter
	if _, err := nilRouter.CollectorFor(model.AgentConfig{}); err == nil {
		t.Fatal("expected error on nil router")
	}
}
