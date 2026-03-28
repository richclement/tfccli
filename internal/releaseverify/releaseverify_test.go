package releaseverify

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyBuiltBinary(t *testing.T) {
	repoRoot, err := repoRoot()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	binaryPath := filepath.Join(t.TempDir(), "tfccli-test-binary")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.version=v0.0.0-test", "-o", binaryPath, "./cmd/tfc")
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build test binary: %v: %s", err, stderr.String())
	}

	if err := Verify(binaryPath, "v0.0.0-test"); err != nil {
		t.Fatalf("verify built binary: %v", err)
	}
}

func TestCompareResultsMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr string
		left    commandResult
		right   commandResult
	}{
		{
			name:    "exit code mismatch",
			wantErr: "exit code mismatch",
			left:    commandResult{exitCode: 0, stdout: "ok", stderr: ""},
			right:   commandResult{exitCode: 1, stdout: "ok", stderr: ""},
		},
		{
			name:    "stdout mismatch",
			wantErr: "stdout mismatch",
			left:    commandResult{exitCode: 0, stdout: "expected", stderr: ""},
			right:   commandResult{exitCode: 0, stdout: "actual", stderr: ""},
		},
		{
			name:    "stderr mismatch",
			wantErr: "stderr mismatch",
			left:    commandResult{exitCode: 0, stdout: "ok", stderr: "expected"},
			right:   commandResult{exitCode: 0, stdout: "ok", stderr: "actual"},
		},
	}

	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			err := compareResults("test-case", current.left, current.right)
			if err == nil {
				t.Fatal("expected compareResults to report a mismatch")
			}
			if !strings.Contains(err.Error(), current.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", current.wantErr, err.Error())
			}
		})
	}
}
