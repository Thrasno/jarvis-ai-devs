package workflowcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIWorkflowContract(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/ci.yml")

	assertContainsAll(t, workflow,
		"name: CI",
		"ubuntu-latest",
		"windows-latest",
		"module: jarvis-cli",
		"module: hive-daemon",
		"module: hive-api",
		"go-version-file: ${{ matrix.module }}/go.mod",
		"cache-dependency-path: ${{ matrix.module }}/go.sum",
		"working-directory: ${{ matrix.module }}",
		"go test ./...",
		"go vet ./...",
		"bash -n",
		"*.sh",
		"System.Management.Automation.Language.Parser",
		"*.ps1",
	)

	assertContainsAll(t, workflow,
		"os: windows-latest\n            module: jarvis-cli",
		"os: ubuntu-latest\n            module: jarvis-cli",
		"os: ubuntu-latest\n            module: hive-daemon",
		"os: ubuntu-latest\n            module: hive-api",
	)
	assertNotContains(t, workflow, "os: windows-latest\n            module: hive-daemon")
	assertNotContains(t, workflow, "os: windows-latest\n            module: hive-api")
	assertNotContains(t, workflow, "continue-on-error")
}

func TestReleaseWorkflowContract(t *testing.T) {
	release := readRepoFile(t, ".github/workflows/release.yml")
	beta := readRepoFile(t, ".github/workflows/beta.yml")
	goreleaser := readRepoFile(t, ".goreleaser.yaml")

	assertContainsAll(t, release,
		"name: Release",
		"tags:",
		"v*.*.*",
		"if: ${{ !contains(github.ref_name, '-') }}",
		"^v[0-9]+\\.[0-9]+\\.[0-9]+$",
		"go-version-file: jarvis-cli/go.mod",
		"jarvis-cli/go.sum",
		"hive-daemon/go.sum",
		"goreleaser/goreleaser-action@v6",
		"args: check",
		"release --clean",
	)

	assertContainsAll(t, beta,
		"name: Beta Release",
		"workflow_dispatch:",
		"ref: master",
		"Verify beta source is remote master",
		"git fetch origin +refs/heads/master:refs/remotes/origin/master --prune",
		"remote_master=\"$(git rev-parse origin/master)\"",
		"current_commit=\"$(git rev-parse HEAD)\"",
		"Beta releases must be published from origin/master.",
		"INTERNAL_BETA_TAG: v0.0.1-beta",
		"PUBLIC_BETA_TAG: beta",
		"contents: write",
		"concurrency:",
		"group: beta-release",
		"git tag -f \"${INTERNAL_BETA_TAG}\"",
		"git tag -f \"${PUBLIC_BETA_TAG}\"",
		"gh release delete \"${INTERNAL_BETA_TAG}\"",
		"gh release delete \"${PUBLIC_BETA_TAG}\"",
		"goreleaser/goreleaser-action@v6",
		"args: check",
		"release --clean",
		"GORELEASER_CURRENT_TAG: v0.0.1-beta",
		"gh release create \"${PUBLIC_BETA_TAG}\"",
		"${filename//$INTERNAL_VERSION/$PUBLIC_VERSION}",
		"public-beta-assets/checksums.txt",
		"sed -i \"s/${INTERNAL_VERSION//./\\\\.}/${PUBLIC_VERSION}/g\" public-beta-assets/checksums.txt",
		"grep -Fq \"${INTERNAL_VERSION}\" public-beta-assets/checksums.txt",
		"checksums.txt still references ${INTERNAL_VERSION}",
		"sha256sum -c checksums.txt",
		"gh release view \"${PUBLIC_BETA_TAG}\"",
		"jarvis_beta_",
		"hive-daemon_beta_",
	)
	assertNotContains(t, beta, "default: beta")
	assertNotContains(t, beta, "inputs:")
	assertNotContains(t, beta, "inputs.ref")
	assertNotContains(t, beta, "Source ref, branch, tag, or SHA")

	assertContainsAll(t, goreleaser,
		"id: jarvis",
		"id: hive-daemon",
		"goos:",
		"linux",
		"darwin",
		"windows",
		"name_template: \"jarvis_{{ .Version }}_{{ .Os }}_{{ .Arch }}\"",
		"name_template: \"hive-daemon_{{ .Version }}_{{ .Os }}_{{ .Arch }}\"",
		"name_template: \"{{ .Tag }}\"",
	)
}

func TestWorkflowGovernanceExclusionContract(t *testing.T) {
	for _, path := range []string{
		".github/workflows/ci.yml",
		".github/workflows/release.yml",
		".github/workflows/beta.yml",
	} {
		content := strings.ToLower(readRepoFile(t, path))
		for _, forbidden := range []string{"pull_request_review", "approval", "required reviewer", "branch protection", "issue approval"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain PR governance term %q", path, forbidden)
			}
		}
	}
}

func TestInstallerVersionOverrideContract(t *testing.T) {
	linux := readRepoFile(t, "scripts/install.sh")
	windows := readRepoFile(t, "scripts/install.ps1")

	assertContainsAll(t, linux,
		"VERSION_OVERRIDE=\"${JARVIS_INSTALL_VERSION:-}\"",
		"if [ -n \"$VERSION_OVERRIDE\" ]; then",
		"VERSION=\"$VERSION_OVERRIDE\"",
		"releases/latest",
		"releases/download/${VERSION}/${name}_${VERSION#v}_${OS}_${ARCH}.tar.gz",
	)
	assertContainsAll(t, windows,
		"$VERSION_OVERRIDE = $env:JARVIS_INSTALL_VERSION",
		"if ($VERSION_OVERRIDE) {",
		"return $VERSION_OVERRIDE",
		"releases/latest",
		"$versionNumber = $Version.TrimStart(\"v\")",
		"releases/download/$Version/${Name}_${versionNumber}_windows_${Arch}.zip",
	)

	for path, content := range map[string]string{
		"scripts/install.sh":  linux,
		"scripts/install.ps1": windows,
	} {
		assertNotContains(t, content, "select beta")
		assertNotContains(t, content, "Read-Host")
		assertNotContains(t, strings.ToLower(content), "prompt")
		if strings.Contains(content, "JARVIS_INSTALL_VERSION=beta curl") {
			t.Fatalf("%s must not scope JARVIS_INSTALL_VERSION only to the downloader process", path)
		}
	}
}

func assertNotContains(t *testing.T, content string, forbidden string) {
	t.Helper()
	if strings.Contains(content, forbidden) {
		t.Fatalf("expected content not to contain %q", forbidden)
	}
}

func TestReleaseDocumentationContract(t *testing.T) {
	runbook := readRepoFile(t, "docs/release-runbook.md")
	readme := readRepoFile(t, "README.md")
	installation := readRepoFile(t, "docs/installation.md")

	assertContainsAll(t, runbook,
		"## Beta release: mutable beta channel",
		"gh workflow run \"Beta Release\"",
		"gh release view beta",
		"export JARVIS_INSTALL_VERSION=beta",
		"v0.0.1-beta compatibility fallback",
		"## Production release: immutable semantic version",
		"git tag vX.Y.Z HEAD",
		"macOS artifacts are best effort",
	)

	assertContainsAll(t, readme,
		"export JARVIS_INSTALL_VERSION=beta",
		"$env:JARVIS_INSTALL_VERSION = \"beta\"",
		"latest production release",
		"macOS artifacts are best effort",
	)

	assertContainsAll(t, installation,
		"## Latest production",
		"## Beta channel",
		"export JARVIS_INSTALL_VERSION=beta",
		"$env:JARVIS_INSTALL_VERSION = \"beta\"",
		"macOS artifacts are best effort",
	)

	for path, content := range map[string]string{
		"README.md":               readme,
		"docs/installation.md":    installation,
		"docs/release-runbook.md": runbook,
	} {
		assertNotContains(t, content, "JARVIS_INSTALL_VERSION=beta curl")
		assertNotContains(t, content, "JARVIS_INSTALL_VERSION=beta irm")
		if strings.Contains(content, "JARVIS_INSTALL_VERSION=beta") && !strings.Contains(content, "export JARVIS_INSTALL_VERSION=beta") {
			t.Fatalf("%s must set JARVIS_INSTALL_VERSION before running the existing installer command", path)
		}
	}
}

func assertContainsAll(t *testing.T, content string, required ...string) {
	t.Helper()
	for _, needle := range required {
		if !strings.Contains(content, needle) {
			t.Fatalf("expected content to contain %q", needle)
		}
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", path))
	if err != nil {
		t.Fatalf("read repo file %s: %v", path, err)
	}
	return string(content)
}
