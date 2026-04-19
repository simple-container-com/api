package cmd_image

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/simple-container-com/api/pkg/security"
	"github.com/simple-container-com/api/pkg/security/scan"
)

func TestToolNames(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantErr bool
	}{
		{"grype", 1, false},
		{"trivy", 1, false},
		{"all", 2, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			names, err := toolNames(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("toolNames(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && len(names) != tt.wantLen {
				t.Errorf("toolNames(%q) returned %d names, want %d", tt.input, len(names), tt.wantLen)
			}
		})
	}
}

func TestBuildScanSecurityConfig_Basic(t *testing.T) {
	t.Run("grype with fail-on high", func(t *testing.T) {
		opts := &scanOptions{
			image:  "myimage:latest",
			tool:   "grype",
			failOn: "high",
			warnOn: "medium",
		}
		cfg, err := buildScanSecurityConfig(opts)
		if err != nil {
			t.Fatalf("buildScanSecurityConfig() error = %v", err)
		}
		if cfg.Scan == nil {
			t.Fatal("expected non-nil scan config")
		}
		if len(cfg.Scan.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(cfg.Scan.Tools))
		}
		if cfg.Scan.Tools[0].Name != "grype" {
			t.Errorf("tool name = %q, want grype", cfg.Scan.Tools[0].Name)
		}
		if cfg.Scan.FailOn != security.Severity("high") {
			t.Errorf("FailOn = %q, want high", cfg.Scan.FailOn)
		}
	})

	t.Run("all tools returns two entries", func(t *testing.T) {
		opts := &scanOptions{
			image: "myimage:latest",
			tool:  "all",
		}
		cfg, err := buildScanSecurityConfig(opts)
		if err != nil {
			t.Fatalf("buildScanSecurityConfig() error = %v", err)
		}
		if len(cfg.Scan.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(cfg.Scan.Tools))
		}
	})

	t.Run("invalid tool name returns error", func(t *testing.T) {
		opts := &scanOptions{
			image: "myimage:latest",
			tool:  "unknown",
		}
		if _, err := buildScanSecurityConfig(opts); err == nil {
			t.Fatal("expected error for invalid tool name")
		}
	})
}

func TestBuildScanSecurityConfig_DefectDojo(t *testing.T) {
	t.Run("upload-defectdojo requires url and key", func(t *testing.T) {
		opts := &scanOptions{
			image:            "myimage:latest",
			tool:             "grype",
			uploadDefectDojo: true,
			defectDojoURL:    "",
			defectDojoAPIKey: "",
		}
		if _, err := buildScanSecurityConfig(opts); err == nil {
			t.Fatal("expected error when defectdojo URL/key are missing")
		}
	})
}

// TestLoadMergedScanResult_DigestValidation verifies that a scan result whose
// content has been tampered with after the digest was recorded is rejected.
func TestLoadMergedScanResult_DigestValidation(t *testing.T) {
	// Build a valid scan result with a correct digest.
	original := scan.NewScanResult("sha256:deadbeef", scan.ScanToolGrype, []scan.Vulnerability{
		{ID: "CVE-2024-1234", Severity: scan.SeverityHigh, Package: "curl", Version: "7.80.0"},
	})

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	t.Run("valid result passes", func(t *testing.T) {
		f := writeTempJSON(t, data)
		result, err := loadMergedScanResult([]string{f})
		if err != nil {
			t.Fatalf("loadMergedScanResult() unexpected error = %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil result")
		}
	})

	t.Run("tampered result is rejected", func(t *testing.T) {
		// Modify the digest field to simulate tampering.
		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		raw["digest"] = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("attacker-controlled")))
		tampered, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal tampered: %v", err)
		}

		f := writeTempJSON(t, tampered)
		if _, err := loadMergedScanResult([]string{f}); err == nil {
			t.Fatal("expected error for tampered scan result, got nil")
		}
	})
}

// writeTempJSON writes content to a temp file and returns its path.
func writeTempJSON(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writeTempJSON: %v", err)
	}
	return path
}
