package gateway

import (
	"errors"
	"testing"
)

func TestNormalizeAgentURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:  "trim and lowercase scheme host and remove root slash",
			input: "  HTTP://LOCALHOST:8005/  ",
			want:  "http://localhost:8005",
		},
		{
			name:  "preserve path and query semantics",
			input: "https://Example.COM:8443/api/v1/?q=abc",
			want:  "https://example.com:8443/api/v1/?q=abc",
		},
		{
			name:    "reject empty",
			input:   "   ",
			wantErr: ErrInvalidNodeURL,
		},
		{
			name:    "reject relative url",
			input:   "/metrics",
			wantErr: ErrInvalidNodeURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAgentURL(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error mismatch: got=%v want=%v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("normalizeAgentURL failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeAgentURL mismatch: got=%q want=%q", got, tt.want)
			}
		})
	}
}
