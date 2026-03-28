package homebrewtap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGoldenFiles(t *testing.T) {
	t.Parallel()

	result, err := Generate("0.12.0", "richclement/tfccli", "tfccli", []byte(sampleChecksums))
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if got, want := string(result.Formula), string(readGoldenFile(t, "formula.golden")); got != want {
		t.Fatalf("formula golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}

	if got, want := string(result.PRBody), string(readGoldenFile(t, "pr-body.golden")); got != want {
		t.Fatalf("pr body golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestGenerateMissingAsset(t *testing.T) {
	t.Parallel()

	_, err := Generate("0.12.0", "richclement/tfccli", "tfccli", []byte(strings.ReplaceAll(sampleChecksums, "tfccli_0.12.0_linux_arm64.tar.gz", "tfccli_0.12.0_linux_arm64-missing.tar.gz")))
	if err == nil {
		t.Fatal("expected error for missing asset")
	}
	if !strings.Contains(err.Error(), `required asset "tfccli_0.12.0_linux_arm64.tar.gz" missing`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseChecksumsMalformedLine(t *testing.T) {
	t.Parallel()

	_, err := ParseChecksums([]byte("abc only-one-field extra\n"))
	if err == nil {
		t.Fatal("expected malformed line error")
	}
	if !strings.Contains(err.Error(), "expected 2 fields") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseChecksumsDuplicateAsset(t *testing.T) {
	t.Parallel()

	_, err := ParseChecksums([]byte(strings.Join([]string{
		"111 tfccli_0.12.0_darwin_amd64.tar.gz",
		"222 tfccli_0.12.0_darwin_amd64.tar.gz",
	}, "\n")))
	if err == nil {
		t.Fatal("expected duplicate asset error")
	}
	if !strings.Contains(err.Error(), `duplicate asset "tfccli_0.12.0_darwin_amd64.tar.gz"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildFormulaInputIgnoresWindowsAssets(t *testing.T) {
	t.Parallel()

	assets, err := ParseChecksums([]byte(sampleChecksums))
	if err != nil {
		t.Fatalf("ParseChecksums returned error: %v", err)
	}

	input, err := BuildFormulaInput("0.12.0", "richclement/tfccli", "tfccli", assets)
	if err != nil {
		t.Fatalf("BuildFormulaInput returned error: %v", err)
	}

	if input.ClassName != "Tfccli" {
		t.Fatalf("unexpected class name: %q", input.ClassName)
	}
	if input.DarwinAMD64.Name != "tfccli_0.12.0_darwin_amd64.tar.gz" {
		t.Fatalf("unexpected darwin amd64 asset: %+v", input.DarwinAMD64)
	}
	if input.LinuxARM64.Name != "tfccli_0.12.0_linux_arm64.tar.gz" {
		t.Fatalf("unexpected linux arm64 asset: %+v", input.LinuxARM64)
	}
}

func TestWriteFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	result, err := Generate("0.12.0", "richclement/tfccli", "tfccli", []byte(sampleChecksums))
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	prBodyPath := filepath.Join(dir, "artifacts", "tfccli-homebrew-pr-body.md")
	if err := WriteFiles(WriteOptions{
		TapDir:     filepath.Join(dir, "tap"),
		PRBodyPath: prBodyPath,
	}, "tfccli", result); err != nil {
		t.Fatalf("WriteFiles returned error: %v", err)
	}

	formulaPath := filepath.Join(dir, "tap", "Formula", "tfccli.rb")
	formula, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatalf("read formula: %v", err)
	}
	if string(formula) != string(result.Formula) {
		t.Fatalf("formula mismatch\nwant:\n%s\ngot:\n%s", result.Formula, formula)
	}

	prBody, err := os.ReadFile(prBodyPath)
	if err != nil {
		t.Fatalf("read pr body: %v", err)
	}
	if string(prBody) != string(result.PRBody) {
		t.Fatalf("pr body mismatch\nwant:\n%s\ngot:\n%s", result.PRBody, prBody)
	}
}

func readGoldenFile(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %s: %v", name, err)
	}

	return contents
}

const sampleChecksums = `1111111111111111111111111111111111111111111111111111111111111111 tfccli_0.12.0_darwin_amd64.tar.gz
2222222222222222222222222222222222222222222222222222222222222222 tfccli_0.12.0_darwin_arm64.tar.gz
3333333333333333333333333333333333333333333333333333333333333333 tfccli_0.12.0_linux_amd64.tar.gz
4444444444444444444444444444444444444444444444444444444444444444 tfccli_0.12.0_linux_arm64.tar.gz
5555555555555555555555555555555555555555555555555555555555555555 tfccli_0.12.0_windows_amd64.zip
6666666666666666666666666666666666666666666666666666666666666666 tfccli_0.12.0_windows_arm64.zip`
