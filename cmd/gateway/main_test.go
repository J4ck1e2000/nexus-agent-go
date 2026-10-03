package main

import (
	"path/filepath"
	"testing"
)

func TestBuildSSHCollector_InvalidKeyDisablesOnlySSHCollector(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("SSH_PRIVATE_KEY_PATH", tempDir)
	t.Setenv("SSH_KNOWN_HOSTS_PATH", filepath.Join(tempDir, "known_hosts"))

	if collector := buildSSHCollector(); collector != nil {
		t.Fatal("expected invalid optional SSH configuration to disable only the SSH collector")
	}
}
