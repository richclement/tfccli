package homebrewtap

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	defaultDescription = "Terraform Cloud CLI tool"
	defaultLicense     = "MIT"
)

type AssetChecksum struct {
	Name   string
	SHA256 string
}

type FormulaInput struct {
	FormulaName string
	ClassName   string
	Description string
	Homepage    string
	Version     string
	License     string
	SourceRepo  string

	DarwinAMD64 AssetChecksum
	DarwinARM64 AssetChecksum
	LinuxAMD64  AssetChecksum
	LinuxARM64  AssetChecksum
}

type RenderResult struct {
	Formula []byte
	PRBody  []byte
}

type WriteOptions struct {
	TapDir     string
	PRBodyPath string
}

var formulaTemplate = template.Must(template.New("formula").Parse(`class {{ .ClassName }} < Formula
  desc "{{ .Description }}"
  homepage "{{ .Homepage }}"
  version "{{ .Version }}"
  license "{{ .License }}"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/{{ .SourceRepo }}/releases/download/v#{version}/{{ .FormulaName }}_#{version}_darwin_arm64.tar.gz"
      sha256 "{{ .DarwinARM64.SHA256 }}"
    else
      url "https://github.com/{{ .SourceRepo }}/releases/download/v#{version}/{{ .FormulaName }}_#{version}_darwin_amd64.tar.gz"
      sha256 "{{ .DarwinAMD64.SHA256 }}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/{{ .SourceRepo }}/releases/download/v#{version}/{{ .FormulaName }}_#{version}_linux_arm64.tar.gz"
      sha256 "{{ .LinuxARM64.SHA256 }}"
    else
      url "https://github.com/{{ .SourceRepo }}/releases/download/v#{version}/{{ .FormulaName }}_#{version}_linux_amd64.tar.gz"
      sha256 "{{ .LinuxAMD64.SHA256 }}"
    end
  end

  def install
    bin.install "{{ .FormulaName }}"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/{{ .FormulaName }} --version")
  end
end
`))

var prBodyTemplate = template.Must(template.New("pr-body").Parse(`## Homebrew update for {{ .FormulaName }}

- Release: [v{{ .Version }}](https://github.com/{{ .SourceRepo }}/releases/tag/v{{ .Version }})
- Formula: ` + "`Formula/{{ .FormulaName }}.rb`" + `
- Version bump: ` + "`{{ .Version }}`" + `

| Platform | Asset | SHA256 |
| --- | --- | --- |
| macOS amd64 | ` + "`{{ .DarwinAMD64.Name }}`" + ` | ` + "`{{ .DarwinAMD64.SHA256 }}`" + ` |
| macOS arm64 | ` + "`{{ .DarwinARM64.Name }}`" + ` | ` + "`{{ .DarwinARM64.SHA256 }}`" + ` |
| Linux amd64 | ` + "`{{ .LinuxAMD64.Name }}`" + ` | ` + "`{{ .LinuxAMD64.SHA256 }}`" + ` |
| Linux arm64 | ` + "`{{ .LinuxARM64.Name }}`" + ` | ` + "`{{ .LinuxARM64.SHA256 }}`" + ` |
`))

func Generate(version string, sourceRepo string, formulaName string, checksums []byte) (RenderResult, error) {
	assets, err := ParseChecksums(checksums)
	if err != nil {
		return RenderResult{}, err
	}

	input, err := BuildFormulaInput(version, sourceRepo, formulaName, assets)
	if err != nil {
		return RenderResult{}, err
	}

	formula, err := RenderFormula(input)
	if err != nil {
		return RenderResult{}, err
	}

	prBody, err := RenderPRBody(input)
	if err != nil {
		return RenderResult{}, err
	}

	return RenderResult{
		Formula: formula,
		PRBody:  prBody,
	}, nil
}

func ParseChecksums(checksums []byte) (map[string]AssetChecksum, error) {
	scanner := bufio.NewScanner(bytes.NewReader(checksums))
	assets := make(map[string]AssetChecksum)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("parse checksums line %d: expected 2 fields, got %d", lineNumber, len(fields))
		}

		asset := AssetChecksum{
			SHA256: fields[0],
			Name:   fields[1],
		}

		if _, exists := assets[asset.Name]; exists {
			return nil, fmt.Errorf("parse checksums line %d: duplicate asset %q", lineNumber, asset.Name)
		}

		assets[asset.Name] = asset
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan checksums: %w", err)
	}

	return assets, nil
}

func BuildFormulaInput(version string, sourceRepo string, formulaName string, assets map[string]AssetChecksum) (FormulaInput, error) {
	if version == "" {
		return FormulaInput{}, errors.New("version is required")
	}
	if sourceRepo == "" {
		return FormulaInput{}, errors.New("source repo is required")
	}
	if formulaName == "" {
		return FormulaInput{}, errors.New("formula name is required")
	}

	required := map[string]string{
		"darwin_amd64": fmt.Sprintf("%s_%s_darwin_amd64.tar.gz", formulaName, version),
		"darwin_arm64": fmt.Sprintf("%s_%s_darwin_arm64.tar.gz", formulaName, version),
		"linux_amd64":  fmt.Sprintf("%s_%s_linux_amd64.tar.gz", formulaName, version),
		"linux_arm64":  fmt.Sprintf("%s_%s_linux_arm64.tar.gz", formulaName, version),
	}

	resolve := func(key string) (AssetChecksum, error) {
		name := required[key]
		asset, ok := assets[name]
		if !ok {
			return AssetChecksum{}, fmt.Errorf("required asset %q missing from checksums", name)
		}
		return asset, nil
	}

	darwinAMD64, err := resolve("darwin_amd64")
	if err != nil {
		return FormulaInput{}, err
	}
	darwinARM64, err := resolve("darwin_arm64")
	if err != nil {
		return FormulaInput{}, err
	}
	linuxAMD64, err := resolve("linux_amd64")
	if err != nil {
		return FormulaInput{}, err
	}
	linuxARM64, err := resolve("linux_arm64")
	if err != nil {
		return FormulaInput{}, err
	}

	return FormulaInput{
		FormulaName: formulaName,
		ClassName:   formulaClassName(formulaName),
		Description: defaultDescription,
		Homepage:    "https://github.com/" + sourceRepo,
		Version:     version,
		License:     defaultLicense,
		SourceRepo:  sourceRepo,
		DarwinAMD64: darwinAMD64,
		DarwinARM64: darwinARM64,
		LinuxAMD64:  linuxAMD64,
		LinuxARM64:  linuxARM64,
	}, nil
}

func RenderFormula(input FormulaInput) ([]byte, error) {
	var out bytes.Buffer
	if err := formulaTemplate.Execute(&out, input); err != nil {
		return nil, fmt.Errorf("render formula: %w", err)
	}
	return out.Bytes(), nil
}

func RenderPRBody(input FormulaInput) ([]byte, error) {
	var out bytes.Buffer
	if err := prBodyTemplate.Execute(&out, input); err != nil {
		return nil, fmt.Errorf("render pr body: %w", err)
	}
	return out.Bytes(), nil
}

func WriteFiles(options WriteOptions, formulaName string, result RenderResult) error {
	if options.TapDir == "" {
		return errors.New("tap dir is required")
	}
	if options.PRBodyPath == "" {
		return errors.New("pr body path is required")
	}
	if formulaName == "" {
		return errors.New("formula name is required")
	}

	formulaPath := filepath.Join(options.TapDir, "Formula", formulaName+".rb")
	if err := os.MkdirAll(filepath.Dir(formulaPath), 0o755); err != nil {
		return fmt.Errorf("create formula directory: %w", err)
	}
	if err := os.WriteFile(formulaPath, result.Formula, 0o644); err != nil {
		return fmt.Errorf("write formula: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(options.PRBodyPath), 0o755); err != nil {
		return fmt.Errorf("create pr body directory: %w", err)
	}
	if err := os.WriteFile(options.PRBodyPath, result.PRBody, 0o644); err != nil {
		return fmt.Errorf("write pr body: %w", err)
	}

	return nil
}

func formulaClassName(formulaName string) string {
	parts := strings.FieldsFunc(formulaName, func(r rune) bool {
		return r == '-' || r == '_'
	})

	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			builder.WriteString(part[1:])
		}
	}

	return builder.String()
}
