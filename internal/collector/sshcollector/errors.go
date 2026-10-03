package sshcollector

import (
	"errors"
)

// SSH 采集链路的错误哨兵。错误码字符串即 API 对外暴露的 error 值。
var (
	// ErrSSHNotConfigured 表示 Gateway 未配置 SSH 私钥，SSH 采集不可用。
	ErrSSHNotConfigured = errors.New("ssh_not_configured")
	// ErrSSHConnectFailed 表示 TCP 连接或 SSH 握手建立失败。
	ErrSSHConnectFailed = errors.New("ssh_connect_failed")
	// ErrSSHAuthFailed 表示 SSH 认证被拒绝。
	ErrSSHAuthFailed = errors.New("ssh_auth_failed")
	// ErrSSHHostKeyFailed 表示 known_hosts 校验失败（未知主机或指纹变化）。
	ErrSSHHostKeyFailed = errors.New("ssh_host_key_failed")
	// ErrSSHCommandTimeout 表示远程脚本执行超时。
	ErrSSHCommandTimeout = errors.New("ssh_command_timeout")
	// ErrSSHCommandFailed 表示远程命令执行失败（非零退出或脚本发送失败）。
	ErrSSHCommandFailed = errors.New("ssh_command_failed")
	// ErrSSHOutputTooLarge 表示远程脚本输出超出上限。
	ErrSSHOutputTooLarge = errors.New("ssh_output_too_large")
	// ErrSSHSnapshotInvalid 表示远程输出缺少结束标记或必需段落无法解析。
	ErrSSHSnapshotInvalid = errors.New("ssh_snapshot_invalid")
	// ErrEncryptedPrivateKey 表示私钥带 passphrase，第一阶段不支持。
	ErrEncryptedPrivateKey = errors.New("encrypted_private_key_not_supported")
)

// ErrorCode 将错误归约为稳定的错误码字符串，供 API / 日志使用。
// 未匹配到已知哨兵时返回 ssh_metrics_failed。
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrSSHNotConfigured):
		return ErrSSHNotConfigured.Error()
	case errors.Is(err, ErrSSHConnectFailed):
		return ErrSSHConnectFailed.Error()
	case errors.Is(err, ErrSSHAuthFailed):
		return ErrSSHAuthFailed.Error()
	case errors.Is(err, ErrSSHHostKeyFailed):
		return ErrSSHHostKeyFailed.Error()
	case errors.Is(err, ErrSSHCommandTimeout):
		return ErrSSHCommandTimeout.Error()
	case errors.Is(err, ErrSSHCommandFailed):
		return ErrSSHCommandFailed.Error()
	case errors.Is(err, ErrSSHOutputTooLarge):
		return ErrSSHOutputTooLarge.Error()
	case errors.Is(err, ErrSSHSnapshotInvalid):
		return ErrSSHSnapshotInvalid.Error()
	case errors.Is(err, ErrEncryptedPrivateKey):
		return ErrEncryptedPrivateKey.Error()
	default:
		return "ssh_metrics_failed"
	}
}
