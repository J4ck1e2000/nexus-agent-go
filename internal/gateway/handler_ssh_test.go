package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"nexus-agent-go/internal/collector/sshcollector"
)

// ErrWrap 构造带哨兵的错误供错误码映射测试使用。
func ErrWrap(sentinel error, detail string) error {
	return fmt.Errorf("%w: %s", sentinel, detail)
}

type fakeSSHTester struct {
	result sshcollector.TestSSHResult
	err    error
	host   string
	port   int
	user   string
}

func (f *fakeSSHTester) TestSSH(ctx context.Context, host string, port int, user string) (sshcollector.TestSSHResult, error) {
	f.host = host
	f.port = port
	f.user = user
	if f.err != nil {
		return sshcollector.TestSSHResult{}, f.err
	}
	return f.result, nil
}

func setupSSHTestRouter(t *testing.T, tester SSHTester) *gin.Engine {
	t.Helper()
	var testers []SSHTester
	if tester != nil {
		testers = append(testers, tester)
	}
	return setupGatewayTestRouter(t, nil, testers...)
}

func TestTestSSH_Success(t *testing.T) {
	tester := &fakeSSHTester{result: sshcollector.TestSSHResult{
		Hostname: "a6000-d-02",
		GPUCount: 2,
		GPUNames: []string{"NVIDIA RTX A6000", "NVIDIA RTX A6000"},
	}}
	r := setupSSHTestRouter(t, tester)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	req := authorizedRequest(http.MethodPost, "/api/config/test-ssh",
		bytes.NewBufferString(`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status mismatch: got=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		OK       bool     `json:"ok"`
		Hostname string   `json:"hostname"`
		GPUCount int      `json:"gpu_count"`
		GPUNames []string `json:"gpu_names"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !body.OK || body.Hostname != "a6000-d-02" || body.GPUCount != 2 || len(body.GPUNames) != 2 {
		t.Fatalf("success payload mismatch: %+v", body)
	}

	if tester.host != "10.0.0.15" || tester.port != 22 || tester.user != "renhaokun" {
		t.Fatalf("tester received wrong target: %+v", tester)
	}
}

func TestTestSSH_ErrorCodes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{
			name:     "auth failed",
			err:      ErrWrap(sshcollector.ErrSSHAuthFailed, "denied"),
			wantCode: http.StatusBadGateway,
			wantBody: `{"error":"ssh_auth_failed"}`,
		},
		{
			name:     "host key failed",
			err:      ErrWrap(sshcollector.ErrSSHHostKeyFailed, "mismatch"),
			wantCode: http.StatusBadGateway,
			wantBody: `{"error":"ssh_host_key_failed"}`,
		},
		{
			name:     "connect failed",
			err:      ErrWrap(sshcollector.ErrSSHConnectFailed, "refused"),
			wantCode: http.StatusBadGateway,
			wantBody: `{"error":"ssh_connect_failed"}`,
		},
		{
			name:     "command timeout",
			err:      ErrWrap(sshcollector.ErrSSHCommandTimeout, "5s"),
			wantCode: http.StatusBadGateway,
			wantBody: `{"error":"ssh_command_timeout"}`,
		},
		{
			name:     "unknown error maps to metrics failed",
			err:      errors.New("mystery"),
			wantCode: http.StatusBadGateway,
			wantBody: `{"error":"ssh_metrics_failed"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupSSHTestRouter(t, &fakeSSHTester{err: tt.err})
			adminToken := loginAndGetToken(t, r, "admin", "admin123")

			req := authorizedRequest(http.MethodPost, "/api/config/test-ssh",
				bytes.NewBufferString(`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`), adminToken)
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)

			if resp.Code != tt.wantCode {
				t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, tt.wantCode, resp.Body.String())
			}
			if body := resp.Body.String(); body != tt.wantBody {
				t.Fatalf("body mismatch: got=%q want=%q", body, tt.wantBody)
			}
		})
	}
}

func TestTestSSH_Validation(t *testing.T) {
	r := setupSSHTestRouter(t, &fakeSSHTester{})
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	invalidPayloads := []string{
		`{"ssh_port":22,"ssh_user":"renhaokun"}`,                           // missing host
		`{"ssh_host":"","ssh_port":22,"ssh_user":"renhaokun"}`,             // empty host
		`{"ssh_host":"10.0.0.15","ssh_port":0,"ssh_user":"renhaokun"}`,     // port 0
		`{"ssh_host":"10.0.0.15","ssh_port":70000,"ssh_user":"renhaokun"}`, // port overflow
		`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":""}`,             // empty user
		`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"bad user!"}`,    // invalid user chars
		`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun,extra"}`,
	}

	for _, payload := range invalidPayloads {
		req := authorizedRequest(http.MethodPost, "/api/config/test-ssh", bytes.NewBufferString(payload), adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)

		if resp.Code != http.StatusBadRequest {
			t.Fatalf("payload %s: status = %d, want 400 (body=%s)", payload, resp.Code, resp.Body.String())
		}
		if body := resp.Body.String(); body != `{"error":"invalid_payload"}` {
			t.Fatalf("payload %s: body = %q", payload, body)
		}
	}
}

func TestTestSSH_RequiresAdmin(t *testing.T) {
	r := setupSSHTestRouter(t, &fakeSSHTester{})
	userToken := loginAndGetToken(t, r, "user", "user123")

	req := authorizedRequest(http.MethodPost, "/api/config/test-ssh",
		bytes.NewBufferString(`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`), userToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status mismatch: got=%d want=%d body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"forbidden"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}

func TestTestSSH_RequiresAuthentication(t *testing.T) {
	r := setupSSHTestRouter(t, &fakeSSHTester{})

	req := httptest.NewRequest(http.MethodPost, "/api/config/test-ssh",
		bytes.NewBufferString(`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status mismatch: got=%d want=%d", resp.Code, http.StatusUnauthorized)
	}
}

func TestTestSSH_NotConfigured(t *testing.T) {
	// 未注入 tester 时（Gateway 未配置私钥）返回 503。
	r := setupSSHTestRouter(t, nil)
	adminToken := loginAndGetToken(t, r, "admin", "admin123")

	req := authorizedRequest(http.MethodPost, "/api/config/test-ssh",
		bytes.NewBufferString(`{"ssh_host":"10.0.0.15","ssh_port":22,"ssh_user":"renhaokun"}`), adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status mismatch: got=%d body=%s", resp.Code, resp.Body.String())
	}
	if body := resp.Body.String(); body != `{"error":"ssh_not_configured"}` {
		t.Fatalf("body mismatch: got=%q", body)
	}
}
