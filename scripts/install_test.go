// Copyright 2026 Nestor G Pestelos Jr and contributors. Licensed under Apache-2.0. See LICENSE.

package scripts

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceFallbackKeepsDiagnosticsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("installer targets POSIX systems")
	}
	original, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		status string
	}{{"success", "0"}, {"failure", "1"}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			binDir := filepath.Join(root, "bin")
			tmpDir := filepath.Join(root, "tmp")
			for _, dir := range []string{binDir, tmpDir} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			sentinel := filepath.Join(root, "sentinel")
			shared := filepath.Join(root, "shared.err")
			if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(sentinel, shared); err != nil {
				t.Fatal(err)
			}
			// Relocate only the legacy fixed path into the fixture. The test
			// must never touch the host's real shared temporary pathname.
			script := strings.ReplaceAll(string(original), "/tmp/pse-edge-install.err", `"$TEST_SHARED_LOG"`)
			scriptPath := filepath.Join(root, "install.sh")
			if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			uname := "#!/bin/sh\nif [ \"$1\" = -m ]; then echo unsupported; else echo Darwin; fi\n"
			goStub := "#!/bin/sh\nprintf 'fixture diagnostic\\n' >&2\nif [ \"$TEST_GO_STATUS\" != 0 ]; then exit 1; fi\ncase \"$*\" in *pse-edge-pp-mcp@latest*) target=pse-edge-pp-mcp;; *) target=pse-edge-pp-cli;; esac\nprintf '#!/bin/sh\\nexit 0\\n' > \"$GOBIN/$target\"\nchmod +x \"$GOBIN/$target\"\n"
			for name, body := range map[string]string{"uname": uname, "go": goStub} {
				if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", scriptPath)
			cmd.Env = append(os.Environ(), "HOME="+root, "HERMES_CONFIG="+root, "GOBIN="+binDir, "TMPDIR="+tmpDir, "TEST_SHARED_LOG="+shared, "TEST_GO_STATUS="+tc.status, "PSE_EDGE_REQUIRE_COSIGN=0", "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, runErr := cmd.CombinedOutput()
			if (runErr == nil) != (tc.status == "0") {
				t.Fatalf("unexpected installer result: %v\n%s", runErr, output)
			}
			data, err := os.ReadFile(sentinel)
			if err != nil || string(data) != "unchanged" {
				t.Fatalf("sentinel changed: %q, error=%v", data, err)
			}
			entries, err := os.ReadDir(tmpDir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("private temporary files remain: %v, error=%v", entries, err)
			}
		})
	}
}

func TestReleaseGateRejectsMissingArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release verifier runs in the Linux release workflow")
	}
	cmd := exec.Command("bash", "verify-release.sh", t.TempDir(), "1.2.3", strings.Repeat("a", 40), "1.27.3")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "missing release archive") {
		t.Fatalf("missing archive must block publication: error=%v output=%s", err, output)
	}
}

func TestReleaseGateChecksArtifactMetadataAndSignatures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release verifier runs in the Linux release workflow")
	}
	for _, tc := range []struct {
		name, badMetadata, cosignStatus, wantError string
	}{
		{"valid", "", "0", ""},
		{"wrong architecture", "wrong-arch", "0", "missing build setting GOARCH=arm64"},
		{"wrong revision", "wrong-revision", "0", "missing build setting vcs.revision="},
		{"wrong version", "wrong-version", "0", "wrong embedded version"},
		{"bad checksum", "bad-checksum", "0", "FAILED"},
		{"missing binary", "missing-binary", "0", "missing or duplicate binary"},
		{"invalid signature", "", "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			binDir := filepath.Join(root, "bin")
			if err := os.Mkdir(binDir, 0o700); err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{
				"go":     "#!/bin/sh\ncat \"$3\"\n",
				"cosign": "#!/bin/sh\nexit \"$TEST_COSIGN_STATUS\"\n",
			} {
				if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			sha := strings.Repeat("a", 40)
			writeReleaseFixture(t, root, sha, tc.badMetadata)
			cmd := exec.Command("bash", "verify-release.sh", root, "1.2.3", sha, "1.27.3")
			cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_COSIGN_STATUS="+tc.cosignStatus)
			output, err := cmd.CombinedOutput()
			if tc.name == "valid" && err != nil {
				t.Fatalf("valid fixture rejected: %v\n%s", err, output)
			}
			if tc.name != "valid" && err == nil {
				t.Fatalf("invalid fixture accepted: %s", output)
			}
			if tc.wantError != "" && !strings.Contains(string(output), tc.wantError) {
				t.Fatalf("wrong failure: %v\n%s", err, output)
			}
		})
	}
}

func writeReleaseFixture(t *testing.T, root, sha, badMetadata string) {
	t.Helper()
	var checksums strings.Builder
	for _, osName := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			ext, suffix := "tar.gz", ""
			if osName == "windows" {
				ext, suffix = "zip", ".exe"
			}
			name := fmt.Sprintf("pse-edge-pp-cli_1.2.3_%s_%s.%s", osName, arch, ext)
			path := filepath.Join(root, name)
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			actualArch := arch
			if badMetadata == "wrong-arch" && osName == "darwin" && arch == "arm64" {
				actualArch = "amd64"
			}
			actualSHA, actualVersion := sha, "1.2.3"
			if badMetadata == "wrong-revision" && osName == "darwin" && arch == "arm64" {
				actualSHA = strings.Repeat("b", 40)
			}
			if badMetadata == "wrong-version" && osName == "darwin" && arch == "arm64" {
				actualVersion = "1.2.30"
			}
			metadata := fmt.Sprintf("fixture: go1.27.3\n\tbuild\t-ldflags=\"-s -w -X github.com/ph-commons/pse-edge-pp-cli/internal/cli.version=%s\"\n\tbuild\tvcs.revision=%s\n\tbuild\tvcs.modified=false\n\tbuild\tGOOS=%s\n\tbuild\tGOARCH=%s\n", actualVersion, actualSHA, osName, actualArch)
			members := []string{"pse-edge-pp-cli" + suffix, "pse-edge-pp-mcp" + suffix}
			if badMetadata == "missing-binary" && osName == "darwin" && arch == "arm64" {
				members = members[:1]
			}
			if ext == "zip" {
				zw := zip.NewWriter(file)
				for _, member := range members {
					out, err := zw.Create(member)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := out.Write([]byte(metadata)); err != nil {
						t.Fatal(err)
					}
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				gz := gzip.NewWriter(file)
				tw := tar.NewWriter(gz)
				for _, member := range members {
					if err := tw.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(metadata))}); err != nil {
						t.Fatal(err)
					}
					if _, err := tw.Write([]byte(metadata)); err != nil {
						t.Fatal(err)
					}
				}
				if err := tw.Close(); err != nil {
					t.Fatal(err)
				}
				if err := gz.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			sbom := name + ".sbom.json"
			if err := os.WriteFile(filepath.Join(root, sbom), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, asset := range []string{name, sbom} {
				data, err := os.ReadFile(filepath.Join(root, asset))
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(data)
				fmt.Fprintf(&checksums, "%x  %s\n", sum, asset)
				if err := os.WriteFile(filepath.Join(root, asset+".sigstore.json"), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := os.WriteFile(filepath.Join(root, "checksums.txt"), []byte(checksums.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if badMetadata == "bad-checksum" {
		if err := os.WriteFile(filepath.Join(root, "pse-edge-pp-cli_1.2.3_darwin_arm64.tar.gz.sbom.json"), []byte("changed"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
