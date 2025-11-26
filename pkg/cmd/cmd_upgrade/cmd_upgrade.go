package cmd_upgrade

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
)

type upgradeCmd struct {
	Root    *root_cmd.RootCmd
	Version string
}

func NewUpgradeCmd(rootCmd *root_cmd.RootCmd) *cobra.Command {
	_ = upgradeCmd{
		Root: rootCmd,
	}
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrades simple-container-com CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade()
		},
	}

	return cmd
}

// runUpgrade downloads and executes the sc.sh script with proper output streaming and cross-platform support
func runUpgrade() error {
	fmt.Println("🚀 Upgrading Simple Container CLI...")

	// Download the upgrade script
	fmt.Println("📦 Downloading upgrade script...")
	resp, err := http.Get("https://dist.simple-container.com/sc.sh")
	if err != nil {
		return fmt.Errorf("failed to download upgrade script: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download upgrade script: HTTP %d", resp.StatusCode)
	}

	// Create temporary file for the script
	tmpFile, err := os.CreateTemp("", "sc-upgrade-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write script to temporary file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write upgrade script: %w", err)
	}
	tmpFile.Close()

	// Make script executable
	err = os.Chmod(tmpFile.Name(), 0o755)
	if err != nil {
		return fmt.Errorf("failed to make script executable: %w", err)
	}

	// Determine shell to use based on platform
	shell := getShell()
	if shell == "" {
		return fmt.Errorf("no compatible shell found (tried: bash, sh, cmd, powershell)")
	}

	fmt.Printf("🔧 Executing upgrade using %s...\n", shell)

	// Execute the upgrade script with streaming output
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if shell == "powershell" {
			cmd = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", tmpFile.Name(), "--version")
		} else {
			cmd = exec.Command("cmd", "/C", tmpFile.Name(), "--version")
		}
	} else {
		cmd = exec.Command(shell, tmpFile.Name(), "--version")
	}

	// Stream output in real-time
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	fmt.Println("✅ Simple Container CLI upgraded successfully!")
	return nil
}

// getShell returns the best available shell for the current platform
func getShell() string {
	// Try shells in order of preference
	var shells []string

	if runtime.GOOS == "windows" {
		shells = []string{"powershell", "cmd"}
	} else {
		shells = []string{"bash", "sh"}
	}

	for _, shell := range shells {
		if _, err := exec.LookPath(shell); err == nil {
			return shell
		}
	}

	return ""
}
