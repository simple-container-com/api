package utils

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simple-container-com/api/pkg/api/logger/color"
)

// CheckAndWarnExistingSimpleContainerProject checks if project is already using Simple Container and warns user
// This is a standalone utility function that can be used by different components
// Set interactive=false for chat contexts where stdin is not available
func CheckAndWarnExistingSimpleContainerProject(projectPath string, forceOverwrite, skipConfirmation, interactive bool) error {
	if projectPath == "" {
		projectPath = "."
	}

	var existingFiles []string
	var foundInSubdir string

	// Check for client.yaml in project root
	clientYAMLPath := filepath.Join(projectPath, "client.yaml")
	if _, err := os.Stat(clientYAMLPath); err == nil {
		existingFiles = append(existingFiles, "client.yaml")
	}

	// Check for client.yaml in .sc/stacks subdirectories
	pattern := filepath.Join(projectPath, ".sc/stacks/*/client.yaml")
	if entries, err := filepath.Glob(pattern); err == nil && len(entries) > 0 {
		for _, entry := range entries {
			relPath, _ := filepath.Rel(projectPath, entry)
			existingFiles = append(existingFiles, relPath)
			if foundInSubdir == "" {
				foundInSubdir = filepath.Dir(entry)
			}
		}
	}

	// Check for other Simple Container specific files
	// Note: Dockerfile alone is NOT a Simple Container indicator - it's a standard Docker file
	otherFiles := []string{
		"server.yaml",  // Simple Container server configuration
		"secrets.yaml", // Simple Container secrets file
		".sc/stacks",   // Simple Container stacks directory
	}

	for _, file := range otherFiles {
		filePath := filepath.Join(projectPath, file)
		if _, err := os.Stat(filePath); err == nil {
			if file == ".sc/stacks" {
				existingFiles = append(existingFiles, ".sc/stacks/")
			} else {
				existingFiles = append(existingFiles, file)
			}
		}
	}

	// If any Simple Container files found and not forcing overwrite, warn the user
	if len(existingFiles) > 0 && !forceOverwrite && !skipConfirmation {
		fmt.Printf("\n%s WARNING: This project appears to already be using Simple Container!\n", color.YellowString("⚠️"))
		fmt.Println("   Found existing files:")
		for _, file := range existingFiles {
			fmt.Printf("   • %s\n", color.CyanString(file))
		}

		if foundInSubdir != "" {
			fmt.Printf("   Configuration found in: %s\n", color.CyanString(foundInSubdir))
		}

		fmt.Println("\n   Running setup will potentially overwrite or modify your existing configuration.")
		fmt.Printf("   %s\n", color.RedString("This action cannot be undone!"))

		if interactive {
			// Interactive mode: prompt for confirmation
			fmt.Print("\n   Do you want to continue and potentially overwrite existing files? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response != "y" && response != "yes" {
				fmt.Printf("\n%s Setup cancelled by user. Your existing configuration is safe.\n", color.GreenString("✅"))
				fmt.Printf("   💡 Tip: Use 'sc config' to view your current Simple Container configuration\n")
				fmt.Printf("   💡 Tip: Use 'sc assistant dev analyze' to view your current project structure\n")
				return fmt.Errorf("setup cancelled by user")
			}
			fmt.Printf("\n%s Proceeding with setup...\n", color.YellowString("⚠️"))
		} else {
			// Non-interactive mode: provide guidance but block execution for safety
			fmt.Printf("\n%s Setup cancelled for safety. Your existing configuration is protected.\n", color.GreenString("✅"))
			fmt.Printf("   💡 To proceed with setup on an existing project:\n")
			fmt.Printf("   • Use CLI: 'sc assistant dev setup --force-overwrite' to override\n")
			fmt.Printf("   • Or run '/analyze' to view current project structure\n")
			fmt.Printf("   • Or run '/config' to view current configuration\n")
			return fmt.Errorf("setup cancelled - existing Simple Container project detected")
		}
	}

	return nil
}
