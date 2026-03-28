package releaseverify

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type commandResult struct {
	exitCode int
	stdout   string
	stderr   string
}

type commandCase struct {
	name string
	args []string
	cwd  string
	home string
}

func Verify(binaryPath string, version string) error {
	repoRoot, err := repoRoot()
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	binaryAbs, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "tfccli-release-verify-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	referenceBinary := filepath.Join(tempDir, "tfccli-reference"+filepath.Ext(binaryAbs))
	if err := buildReferenceBinary(repoRoot, referenceBinary, version); err != nil {
		return err
	}

	emptyHome := filepath.Join(tempDir, "empty-home")
	if err := os.MkdirAll(emptyHome, 0o755); err != nil {
		return fmt.Errorf("create empty home: %w", err)
	}

	fixtureHome := filepath.Join(tempDir, "fixture-home")
	if err := writeFixtureHome(fixtureHome); err != nil {
		return fmt.Errorf("write fixture home: %w", err)
	}

	cases := []commandCase{
		{name: "version", args: []string{"--version"}, cwd: repoRoot, home: emptyHome},
		{name: "help", args: []string{"--help"}, cwd: repoRoot, home: emptyHome},
		{name: "invalid-usage", args: []string{"not-a-command"}, cwd: repoRoot, home: emptyHome},
		{name: "contexts-list-empty", args: []string{"contexts", "list"}, cwd: repoRoot, home: emptyHome},
		{name: "contexts-list-empty-json", args: []string{"--output-format=json", "contexts", "list"}, cwd: repoRoot, home: emptyHome},
		{name: "contexts-show", args: []string{"contexts", "show"}, cwd: repoRoot, home: fixtureHome},
		{name: "contexts-show-json", args: []string{"--output-format=json", "contexts", "show"}, cwd: repoRoot, home: fixtureHome},
	}

	for _, currentCase := range cases {
		expected, err := runCommand(referenceBinary, currentCase)
		if err != nil {
			return fmt.Errorf("run reference binary for %s: %w", currentCase.name, err)
		}

		actual, err := runCommand(binaryAbs, currentCase)
		if err != nil {
			return fmt.Errorf("run release binary for %s: %w", currentCase.name, err)
		}

		if err := compareResults(currentCase.name, expected, actual); err != nil {
			return err
		}
	}

	return nil
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}

func buildReferenceBinary(repoRoot string, outputPath string, version string) error {
	args := []string{"build", "-trimpath", "-o", outputPath}
	if version != "" {
		args = append(args, "-ldflags", "-X main.version="+version)
	}
	args = append(args, "./cmd/tfc")

	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build reference binary: %w: %s", err, stderr.String())
	}

	return nil
}

func writeFixtureHome(homeDir string) error {
	settingsPath := filepath.Join(homeDir, ".tfccli", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}

	settings := []byte(`{
  "current_context": "default",
  "contexts": {
    "default": {
      "address": "app.terraform.io",
      "default_org": "acme",
      "log_level": "info"
    }
  }
}
`)

	return os.WriteFile(settingsPath, settings, 0o600)
}

func runCommand(binary string, currentCase commandCase) (commandResult, error) {
	cmd := exec.Command(binary, currentCase.args...)
	cmd.Dir = currentCase.cwd
	cmd.Env = envWithHome(currentCase.home)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := commandResult{
		exitCode: 0,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result, nil
	}

	return commandResult{}, err
}

func envWithHome(home string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		if hasEnvKey(item, "HOME") || hasEnvKey(item, "USERPROFILE") || hasEnvKey(item, "HOMEDRIVE") || hasEnvKey(item, "HOMEPATH") {
			continue
		}
		env = append(env, item)
	}

	volume := filepath.VolumeName(home)
	homePath := strings.TrimPrefix(home, volume)
	if homePath == "" {
		homePath = string(os.PathSeparator)
	}

	env = append(env,
		"HOME="+home,
		"USERPROFILE="+home,
		"HOMEDRIVE="+volume,
		"HOMEPATH="+homePath,
	)
	return env
}

func hasEnvKey(item string, key string) bool {
	return len(item) > len(key) && item[:len(key)+1] == key+"="
}

func compareResults(name string, expected commandResult, actual commandResult) error {
	if expected.exitCode != actual.exitCode {
		return fmt.Errorf("%s exit code mismatch: expected %d, got %d", name, expected.exitCode, actual.exitCode)
	}
	if expected.stdout != actual.stdout {
		return fmt.Errorf("%s stdout mismatch\nexpected:\n%s\nactual:\n%s", name, expected.stdout, actual.stdout)
	}
	if expected.stderr != actual.stderr {
		return fmt.Errorf("%s stderr mismatch\nexpected:\n%s\nactual:\n%s", name, expected.stderr, actual.stderr)
	}

	return nil
}
