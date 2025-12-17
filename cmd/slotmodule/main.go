package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

const (
	// Module path for version checking
	modulePathForCheck = "git.futuregamestudio.net/be-shared/slot-game-module.git"
	// Go proxy URL (can use proxy.golang.org or private proxy)
	goProxyURL = "https://proxy.golang.org"
	// Set to true to block usage if outdated, false to just warn
	blockIfOutdated = false
)

var (
	version    = getVersion()
	skipUpdate = false // Flag to skip update check
)

// getVersion returns the module version from build info
func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		// Main module version
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev" // fallback for development
}

// getCoreModuleVersion returns a valid Go module version for templates
// If current version is "dev", try to fetch latest or fallback to "latest"
func getCoreModuleVersion() string {
	if version != "dev" {
		return version
	}

	// Try to fetch latest version for dev builds
	if latest, err := fetchLatestVersion(); err == nil && latest != "" {
		return latest
	}

	// Fallback to "latest" which go mod tidy will resolve
	return "latest"
}

// checkForUpdates checks if a newer version is available
func checkForUpdates() error {
	// Skip check in dev mode or if flag is set
	if version == "dev" || skipUpdate {
		return nil
	}

	latestVersion, err := fetchLatestVersion()
	if err != nil {
		// Don't block on network errors, just warn
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not check for updates: %v\n", err)
		return nil
	}

	if latestVersion == "" {
		return nil
	}

	// Compare versions
	cmp := compareVersions(version, latestVersion)
	if cmp < 0 {
		msg := fmt.Sprintf(`
╔════════════════════════════════════════════════════════════════╗
║  🔄 UPDATE AVAILABLE                                           ║
║                                                                ║
║  Current version: %-44s║
║  Latest version:  %-44s║
║                                                                ║
║  To update, run:                                               ║
║  go install %s/cmd/slotmodule@latest  ║
╚════════════════════════════════════════════════════════════════╝
`, version, latestVersion, modulePathForCheck)
		fmt.Fprintln(os.Stderr, msg)

		if blockIfOutdated {
			return fmt.Errorf("please update to the latest version before continuing")
		}
	}

	return nil
}

// fetchLatestVersion fetches the latest version from Go proxy
func fetchLatestVersion() (string, error) {
	// Try Go proxy first
	url := fmt.Sprintf("%s/%s/@latest", goProxyURL, modulePathForCheck)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Try GitLab API as fallback
		return fetchLatestVersionFromGitLab()
	}

	var result struct {
		Version string `json:"Version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Version, nil
}

// fetchLatestVersionFromGitLab fetches version from GitLab tags API
func fetchLatestVersionFromGitLab() (string, error) {
	// GitLab API for tags (adjust project ID or path as needed)
	url := "https://git.futuregamestudio.net/api/v4/projects/be-shared%2Fslot-game-module/repository/tags?per_page=1"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
	}

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "", err
	}

	if len(tags) > 0 {
		return tags[0].Name, nil
	}

	return "", nil
}

// compareVersions compares two semver versions
// Returns: -1 if v1 < v2, 0 if equal, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < 3; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}

// UpdatableFile represents a file that can be updated
type UpdatableFile struct {
	Name        string // Display name for the file
	Path        string // Path pattern (with placeholders)
	Template    string // Template content
	Description string // Description of what this file does
	Category    string // Category: "config", "build", "ci", "docs"
}

// getUpdatableFiles returns the list of files that can be updated
func getUpdatableFiles() []UpdatableFile {
	return []UpdatableFile{
		{Name: "Dockerfile", Path: "Dockerfile", Template: dockerfileTemplate, Description: "Docker build configuration", Category: "build"},
		{Name: "docker-compose.yml", Path: "docker-compose.yml", Template: dockerComposeTemplate, Description: "Docker Compose configuration", Category: "build"},
		{Name: ".gitlab-ci.yml", Path: ".gitlab-ci.yml", Template: gitlabCITemplate, Description: "GitLab CI/CD pipeline", Category: "ci"},
		{Name: "Makefile", Path: "Makefile", Template: makefileTemplate, Description: "Build tasks and commands", Category: "build"},
		{Name: "README.md", Path: "README.md", Template: readmeTemplate, Description: "Project documentation", Category: "docs"},
		{Name: ".gitignore", Path: ".gitignore", Template: gitignoreTemplate, Description: "Git ignore rules", Category: "config"},
		{Name: "main.go", Path: "main.go", Template: mainTemplate, Description: "Application entry point", Category: "code"},
		{Name: "module.go", Path: "internal/logic/module.go", Template: moduleTemplate, Description: "Game logic module", Category: "code"},
		{Name: "config.go", Path: "internal/logic/config.go", Template: configGoTemplate, Description: "Game-specific config struct", Category: "code"},
		{Name: "config.yaml", Path: "config/config.yaml", Template: configTemplate, Description: "App configuration", Category: "config"},
		{Name: "game_config.yaml", Path: "config/{{.GameCodeSnake}}.yaml", Template: gameSpecificConfigTemplate, Description: "Game-specific configuration", Category: "config"},
		{Name: "go.mod", Path: "go.mod", Template: goModTemplate, Description: "Go module definition", Category: "build"},
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "slotmodule",
		Short: "Slot Game Module CLI - Generate slot game projects",
		Long: `Slot Game Module CLI tool for creating new slot game projects.
		
This CLI generates a complete project structure with:
- Game logic template
- Configuration files
- Docker & Docker Compose
- GitLab CI/CD
- Makefile
- README

Example:
  slotmodule create --name beach-party --port 8081
  slotmodule update ./game-beach-party --files Dockerfile,Makefile
  slotmodule update-all ./games --files Dockerfile`,
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip update check for version and help commands
			if cmd.Name() == "version" || cmd.Name() == "help" {
				return nil
			}
			return checkForUpdates()
		},
	}

	// Global flags
	rootCmd.PersistentFlags().BoolVar(&skipUpdate, "skip-update-check", false, "Skip checking for updates")

	// Create command
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new slot game project",
		Long:  `Create a new slot game project with all necessary files and configurations.`,
		Run:   runCreate,
	}

	createCmd.Flags().StringP("name", "n", "", "Game code/name (required, e.g., beach-party)")
	createCmd.Flags().IntP("port", "p", 8080, "Server port")
	createCmd.Flags().StringP("output", "o", ".", "Output directory")
	createCmd.Flags().StringP("module", "m", "", "Go module path (default: git.futuregamestudio.net/fgs/backend/game-{name})")
	createCmd.Flags().IntP("paylines", "l", 20, "Number of paylines")
	createCmd.Flags().BoolP("with-wire", "w", false, "Include Wire dependency injection")
	createCmd.Flags().Bool("no-setup", false, "Skip running post-setup commands (go mod tidy, swag init)")
	_ = createCmd.MarkFlagRequired("name")

	// Update command - update a single project
	updateCmd := &cobra.Command{
		Use:   "update [project-path]",
		Short: "Update an existing slot game project with latest templates",
		Long: `Update an existing slot game project with the latest template files.

This command reads the project metadata from .slotmodule.json and updates
selected files with the latest templates while preserving project-specific values.

Examples:
  # Update all updatable files (with confirmation)
  slotmodule update ./game-beach-party

  # Update specific files only
  slotmodule update ./game-beach-party --files Dockerfile,Makefile,.gitlab-ci.yml

  # Preview changes without applying (dry-run)
  slotmodule update ./game-beach-party --dry-run

  # Force update without confirmation
  slotmodule update ./game-beach-party --force

  # List available files that can be updated
  slotmodule update --list-files`,
		Args: cobra.MaximumNArgs(1),
		Run:  runUpdate,
	}

	updateCmd.Flags().StringSliceP("files", "f", nil, "Specific files to update (comma-separated)")
	updateCmd.Flags().Bool("dry-run", false, "Preview changes without applying them")
	updateCmd.Flags().Bool("force", false, "Force update without confirmation")
	updateCmd.Flags().Bool("list-files", false, "List all files that can be updated")
	updateCmd.Flags().Bool("backup", true, "Create backup of files before updating")

	// Update-all command - update multiple projects
	updateAllCmd := &cobra.Command{
		Use:   "update-all [directory]",
		Short: "Update all slot game projects in a directory",
		Long: `Scan a directory for slot game projects and update them with latest templates.

This command finds all projects with .slotmodule.json and updates them.

Examples:
  # Update all projects in current directory
  slotmodule update-all .

  # Update all projects with specific files
  slotmodule update-all ./games --files Dockerfile,Makefile

  # Preview changes for all projects
  slotmodule update-all ./games --dry-run`,
		Args: cobra.MaximumNArgs(1),
		Run:  runUpdateAll,
	}

	updateAllCmd.Flags().StringSliceP("files", "f", nil, "Specific files to update (comma-separated)")
	updateAllCmd.Flags().Bool("dry-run", false, "Preview changes without applying them")
	updateAllCmd.Flags().Bool("force", false, "Force update without confirmation")
	updateAllCmd.Flags().Bool("backup", true, "Create backup of files before updating")

	// Init command - initialize metadata for existing projects
	initCmd := &cobra.Command{
		Use:   "init [project-path]",
		Short: "Initialize metadata for an existing project",
		Long: `Initialize .slotmodule.json for an existing project created before the update feature.

This command will scan the project and extract configuration from existing files,
allowing you to use the 'update' command on older projects.

Examples:
  slotmodule init ./game-beach-party
  slotmodule init ./game-beach-party --name beach-party --port 8081`,
		Args: cobra.ExactArgs(1),
		Run:  runInit,
	}

	initCmd.Flags().StringP("name", "n", "", "Game code/name (will try to auto-detect if not provided)")
	initCmd.Flags().IntP("port", "p", 0, "Server port (will try to auto-detect if not provided)")
	initCmd.Flags().StringP("module", "m", "", "Go module path (will try to auto-detect from go.mod)")
	initCmd.Flags().IntP("paylines", "l", 20, "Number of paylines")
	initCmd.Flags().Bool("force", false, "Overwrite existing .slotmodule.json")

	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(updateAllCmd)
	rootCmd.AddCommand(initCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCreate(cmd *cobra.Command, args []string) {
	name, _ := cmd.Flags().GetString("name")
	port, _ := cmd.Flags().GetInt("port")
	output, _ := cmd.Flags().GetString("output")
	modulePath, _ := cmd.Flags().GetString("module")
	paylines, _ := cmd.Flags().GetInt("paylines")
	withWire, _ := cmd.Flags().GetBool("with-wire")

	// Normalize name
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Set default module path
	if modulePath == "" {
		modulePath = fmt.Sprintf("git.futuregamestudio.net/fgs/backend/game-%s", name)
	}

	// Create project directory
	projectDir := filepath.Join(output, fmt.Sprintf("game-%s", name))
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating project directory: %v\n", err)
		os.Exit(1)
	}

	// Template data
	data := TemplateData{
		GameCode:          name,
		GameCodeUpper:     toUpperCamel(name),
		GameCodeSnake:     strings.ReplaceAll(name, "-", "_"),
		ModulePath:        modulePath,
		Port:              port,
		PayLines:          paylines,
		WithWire:          withWire,
		CoreModulePath:    "git.futuregamestudio.net/be-shared/slot-game-module.git",
		CoreModuleVersion: getCoreModuleVersion(),
	}

	// Generate files
	files := []struct {
		path     string
		template string
	}{
		{"go.mod", goModTemplate},
		{"main.go", mainTemplate},
		{"docs/docs.go", docsPlaceholderTemplate},
		{"internal/logic/config.go", configGoTemplate},
		{"internal/logic/module.go", moduleTemplate},
		{"config/config.yaml", configTemplate},
		{"config/module-base.yml", moduleBaseConfigTemplate},
		{fmt.Sprintf("config/%s.yaml", data.GameCodeSnake), gameSpecificConfigTemplate},
		{"Dockerfile", dockerfileTemplate},
		{"docker-compose.yml", dockerComposeTemplate},
		{".gitlab-ci.yml", gitlabCITemplate},
		{"Makefile", makefileTemplate},
		{"README.md", readmeTemplate},
		{".gitignore", gitignoreTemplate},
	}

	for _, f := range files {
		filePath := filepath.Join(projectDir, f.path)

		// Create directory if needed
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			continue
		}

		// Generate file
		if err := generateFile(filePath, f.template, data); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating %s: %v\n", f.path, err)
			continue
		}

		fmt.Printf("✓ Created %s\n", f.path)
	}

	// Save project metadata for future updates
	if err := saveProjectMetadata(projectDir, data); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not save project metadata: %v\n", err)
	} else {
		fmt.Printf("✓ Created %s\n", metadataFileName)
	}

	fmt.Printf("\n🎉 Project created successfully at: %s\n", projectDir)

	// Run post-setup commands
	noSetup, _ := cmd.Flags().GetBool("no-setup")
	if !noSetup {
		fmt.Println("\n📦 Running post-setup commands...")
		runPostSetup(projectDir)
	} else {
		fmt.Printf("\nNext steps:\n")
		fmt.Printf("  cd %s\n", projectDir)
		fmt.Printf("  go mod tidy\n")
		fmt.Printf("  make swagger  # Generate swagger docs\n")
		fmt.Printf("  make run\n")
	}
}

// runUpdate handles the update command
func runUpdate(cmd *cobra.Command, args []string) {
	listFiles, _ := cmd.Flags().GetBool("list-files")

	// List available files
	if listFiles {
		printUpdatableFiles()
		return
	}

	// Require project path
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: project path is required")
		fmt.Fprintln(os.Stderr, "Usage: slotmodule update [project-path]")
		fmt.Fprintln(os.Stderr, "\nUse --list-files to see available files")
		os.Exit(1)
	}

	projectDir := args[0]
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	backup, _ := cmd.Flags().GetBool("backup")
	files, _ := cmd.Flags().GetStringSlice("files")

	if err := updateProject(projectDir, files, dryRun, force, backup); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runUpdateAll handles the update-all command
func runUpdateAll(cmd *cobra.Command, args []string) {
	searchDir := "."
	if len(args) > 0 {
		searchDir = args[0]
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	backup, _ := cmd.Flags().GetBool("backup")
	files, _ := cmd.Flags().GetStringSlice("files")

	// Find all projects
	projects, err := findProjects(searchDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning directory: %v\n", err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Println("No slot game projects found in", searchDir)
		fmt.Println("Projects must have a .slotmodule.json file to be detected.")
		return
	}

	fmt.Printf("Found %d project(s):\n", len(projects))
	for _, p := range projects {
		fmt.Printf("  • %s\n", p)
	}
	fmt.Println()

	// Confirm if not forced
	if !force && !dryRun {
		fmt.Print("Do you want to update all projects? [y/N]: ")
		var response string
		fmt.Scanln(&response) //nolint:errcheck
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	// Update each project
	successCount := 0
	failCount := 0
	for _, projectDir := range projects {
		fmt.Printf("\n━━━ Updating %s ━━━\n", projectDir)
		if err := updateProject(projectDir, files, dryRun, true, backup); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed: %v\n", err)
			failCount++
		} else {
			successCount++
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("═", 50))
	fmt.Printf("Summary: %d succeeded, %d failed\n", successCount, failCount)
}

// findProjects finds all slot game projects in a directory
func findProjects(searchDir string) ([]string, error) {
	var projects []string

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
		}

		// Skip hidden directories and vendor
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
		}

		// Check for .slotmodule.json
		if info.Name() == metadataFileName {
			projectDir := filepath.Dir(path)
			projects = append(projects, projectDir)
			return filepath.SkipDir // Don't recurse into this project
		}

		return nil
	})

	return projects, err
}

// updateProject updates a single project
func updateProject(projectDir string, selectedFiles []string, dryRun, force, backup bool) error {
	// Load project metadata
	metadata, err := loadProjectMetadata(projectDir)
	if err != nil {
		return err
	}

	// Convert to template data
	data := metadataToTemplateData(metadata)

	// Get files to update
	updatableFiles := getUpdatableFiles()
	filesToUpdate := filterFiles(updatableFiles, selectedFiles)

	if len(filesToUpdate) == 0 {
		return fmt.Errorf("no matching files to update")
	}

	fmt.Printf("Project: %s (game: %s, CLI version: %s → %s)\n",
		projectDir, metadata.GameCode, metadata.CLIVersion, version)

	if dryRun {
		fmt.Println("\n🔍 DRY RUN - Changes that would be made:")
	}

	updatedCount := 0
	for _, f := range filesToUpdate {
		// Resolve file path (replace placeholders)
		filePath := resolveFilePath(f.Path, data)
		fullPath := filepath.Join(projectDir, filePath)

		// Generate new content
		newContent, err := generateContent(f.Template, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠️  Error generating %s: %v\n", f.Name, err)
			continue
		}

		// Check if file exists and compare
		existingContent, err := os.ReadFile(fullPath)
		fileExists := err == nil

		if fileExists && string(existingContent) == newContent {
			fmt.Printf("  ○ %s (no changes)\n", filePath)
			continue
		}

		// Show diff preview
		if dryRun {
			if fileExists {
				fmt.Printf("\n  📝 %s - WOULD UPDATE:\n", filePath)
				showSimpleDiff(string(existingContent), newContent)
			} else {
				fmt.Printf("\n  📝 %s - WOULD CREATE (new file)\n", filePath)
			}
			updatedCount++
			continue
		}

		// Confirm update if not forced
		if !force && fileExists {
			fmt.Printf("\n  Update %s? [y/N/d(diff)]: ", filePath)
			var response string
			fmt.Scanln(&response) //nolint:errcheck

			if strings.ToLower(response) == "d" {
				showSimpleDiff(string(existingContent), newContent)
				fmt.Printf("  Update %s? [y/N]: ", filePath)
				fmt.Scanln(&response) //nolint:errcheck
			}

			if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
				fmt.Printf("  ○ %s (skipped)\n", filePath)
				continue
			}
		}

		// Create backup if requested
		if backup && fileExists {
			backupPath := fullPath + ".bak"
			if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "  ⚠️  Warning: Could not create backup: %v\n", err)
			}
		}

		// Create directory if needed
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "  ❌ Error creating directory %s: %v\n", dir, err)
			continue
		}

		// Write new content
		if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  ❌ Error writing %s: %v\n", filePath, err)
			continue
		}

		if fileExists {
			fmt.Printf("  ✓ %s (updated)\n", filePath)
		} else {
			fmt.Printf("  ✓ %s (created)\n", filePath)
		}
		updatedCount++
	}

	// Update metadata
	if !dryRun && updatedCount > 0 {
		metadata.UpdatedAt = time.Now()
		metadata.CLIVersion = version
		metadata.CoreModuleVersion = getCoreModuleVersion()
		if err := writeMetadata(projectDir, *metadata); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠️  Warning: Could not update metadata: %v\n", err)
		}
	}

	if dryRun {
		fmt.Printf("\n📋 %d file(s) would be updated\n", updatedCount)
	} else {
		fmt.Printf("\n✅ %d file(s) updated\n", updatedCount)
	}

	return nil
}

// filterFiles filters the updatable files based on selection
func filterFiles(allFiles []UpdatableFile, selected []string) []UpdatableFile {
	if len(selected) == 0 {
		return allFiles
	}

	var filtered []UpdatableFile
	selectedMap := make(map[string]bool)
	for _, s := range selected {
		selectedMap[strings.ToLower(s)] = true
	}

	for _, f := range allFiles {
		if selectedMap[strings.ToLower(f.Name)] || selectedMap[strings.ToLower(f.Path)] {
			filtered = append(filtered, f)
		}
	}

	return filtered
}

// resolveFilePath replaces placeholders in file path
func resolveFilePath(pathTemplate string, data TemplateData) string {
	t, err := template.New("path").Parse(pathTemplate)
	if err != nil {
		return pathTemplate
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return pathTemplate
	}

	return buf.String()
}

// generateContent generates content from template
func generateContent(tmpl string, data TemplateData) (string, error) {
	t, err := template.New("content").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// printUpdatableFiles prints the list of files that can be updated
func printUpdatableFiles() {
	files := getUpdatableFiles()

	fmt.Println("Files that can be updated:")
	fmt.Println()

	categories := map[string][]UpdatableFile{}
	for _, f := range files {
		categories[f.Category] = append(categories[f.Category], f)
	}

	categoryOrder := []string{"build", "ci", "config", "code", "docs"}
	categoryNames := map[string]string{
		"build":  "🔧 Build & Deployment",
		"ci":     "🚀 CI/CD",
		"config": "⚙️  Configuration",
		"code":   "📝 Code Templates",
		"docs":   "📚 Documentation",
	}

	for _, cat := range categoryOrder {
		if files, ok := categories[cat]; ok {
			fmt.Printf("%s\n", categoryNames[cat])
			for _, f := range files {
				fmt.Printf("  %-20s %s\n", f.Name, f.Description)
			}
			fmt.Println()
		}
	}

	fmt.Println("Usage examples:")
	fmt.Println("  slotmodule update ./game-xyz --files Dockerfile,Makefile")
	fmt.Println("  slotmodule update ./game-xyz --files .gitlab-ci.yml --dry-run")
	fmt.Println("  slotmodule update-all ./games --files Dockerfile")
}

// showSimpleDiff shows a simple diff between old and new content
func showSimpleDiff(oldContent, newContent string) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Simple line-by-line comparison
	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	diffFound := false
	for i := 0; i < maxLines; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if !diffFound {
				fmt.Println("    ───────────────────────────────────")
				diffFound = true
			}
			if oldLine != "" && (i >= len(newLines) || oldLine != newLine) {
				fmt.Printf("    \033[31m- %s\033[0m\n", truncateLine(oldLine, 70))
			}
			if newLine != "" && (i >= len(oldLines) || oldLine != newLine) {
				fmt.Printf("    \033[32m+ %s\033[0m\n", truncateLine(newLine, 70))
			}
		}
	}

	if diffFound {
		fmt.Println("    ───────────────────────────────────")
	}
}

// truncateLine truncates a line to maxLen characters
func truncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen-3] + "..."
}

// runInit handles the init command for existing projects
func runInit(cmd *cobra.Command, args []string) {
	projectDir := args[0]
	force, _ := cmd.Flags().GetBool("force")
	name, _ := cmd.Flags().GetString("name")
	port, _ := cmd.Flags().GetInt("port")
	modulePath, _ := cmd.Flags().GetString("module")
	paylines, _ := cmd.Flags().GetInt("paylines")

	// Check if .slotmodule.json already exists
	metadataPath := filepath.Join(projectDir, metadataFileName)
	if _, err := os.Stat(metadataPath); err == nil && !force {
		fmt.Fprintf(os.Stderr, "Error: %s already exists. Use --force to overwrite.\n", metadataFileName)
		os.Exit(1)
	}

	// Try to auto-detect values from existing files
	if name == "" {
		name = detectGameCode(projectDir)
		if name == "" {
			fmt.Fprintln(os.Stderr, "Error: Could not auto-detect game code. Please provide --name flag.")
			os.Exit(1)
		}
		fmt.Printf("🔍 Auto-detected game code: %s\n", name)
	}

	if modulePath == "" {
		modulePath = detectModulePath(projectDir)
		if modulePath != "" {
			fmt.Printf("🔍 Auto-detected module path: %s\n", modulePath)
		} else {
			modulePath = fmt.Sprintf("git.futuregamestudio.net/fgs/backend/game-%s", name)
			fmt.Printf("📝 Using default module path: %s\n", modulePath)
		}
	}

	if port == 0 {
		port = detectPort(projectDir)
		if port != 0 {
			fmt.Printf("🔍 Auto-detected port: %d\n", port)
		} else {
			port = 8080
			fmt.Printf("📝 Using default port: %d\n", port)
		}
	}

	// Create template data
	data := TemplateData{
		GameCode:          name,
		GameCodeUpper:     toUpperCamel(name),
		GameCodeSnake:     strings.ReplaceAll(name, "-", "_"),
		ModulePath:        modulePath,
		Port:              port,
		PayLines:          paylines,
		WithWire:          false,
		CoreModulePath:    "git.futuregamestudio.net/be-shared/slot-game-module.git",
		CoreModuleVersion: getCoreModuleVersion(),
	}

	// Save metadata
	if err := saveProjectMetadata(projectDir, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not save metadata: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Initialized %s in %s\n", metadataFileName, projectDir)
	fmt.Println("\nYou can now use:")
	fmt.Printf("  slotmodule update %s\n", projectDir)
	fmt.Printf("  slotmodule update %s --files Dockerfile,Makefile\n", projectDir)
}

// detectGameCode tries to detect game code from project directory or files
func detectGameCode(projectDir string) string {
	// Try from directory name (e.g., "game-beach-party" -> "beach-party")
	dirName := filepath.Base(projectDir)
	if strings.HasPrefix(dirName, "game-") {
		return strings.TrimPrefix(dirName, "game-")
	}

	// Try from config file names
	configDir := filepath.Join(projectDir, "config")
	files, err := os.ReadDir(configDir)
	if err == nil {
		for _, f := range files {
			name := f.Name()
			if name != "config.yaml" && strings.HasSuffix(name, ".yaml") {
				// e.g., "beach_party.yaml" -> "beach-party"
				gameName := strings.TrimSuffix(name, ".yaml")
				return strings.ReplaceAll(gameName, "_", "-")
			}
		}
	}

	// Try from go.mod
	goModPath := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err == nil {
		lines := strings.Split(string(content), "\n")
		if len(lines) > 0 && strings.HasPrefix(lines[0], "module ") {
			modulePath := strings.TrimPrefix(lines[0], "module ")
			modulePath = strings.TrimSpace(modulePath)
			// e.g., "git.futuregamestudio.net/fgs/backend/game-beach-party" -> "beach-party"
			parts := strings.Split(modulePath, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				if strings.HasPrefix(lastPart, "game-") {
					return strings.TrimPrefix(lastPart, "game-")
				}
			}
		}
	}

	return ""
}

// detectModulePath tries to detect module path from go.mod
func detectModulePath(projectDir string) string {
	goModPath := filepath.Join(projectDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "module ") {
		return strings.TrimSpace(strings.TrimPrefix(lines[0], "module "))
	}

	return ""
}

// detectPort tries to detect port from config files
func detectPort(projectDir string) int {
	configPath := filepath.Join(projectDir, "config", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}

	// Simple parsing - look for "port:" line
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "port:") {
			portStr := strings.TrimSpace(strings.TrimPrefix(line, "port:"))
			if port, err := strconv.Atoi(portStr); err == nil {
				return port
			}
		}
	}

	return 0
}

// runPostSetup runs post-setup commands in the project directory
func runPostSetup(projectDir string) {
	// 1. Run go mod tidy
	fmt.Println("\n→ Running go mod tidy...")
	if err := runCommand(projectDir, "go", "mod", "tidy"); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  Warning: go mod tidy failed: %v\n", err)
	} else {
		fmt.Println("  ✓ go mod tidy completed")
	}

	// 2. Vendor dependencies
	fmt.Println("\n→ Vendoring dependencies...")
	if err := runCommand(projectDir, "go", "mod", "vendor"); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  Warning: go mod vendor failed: %v\n", err)
	} else {
		fmt.Println("  ✓ Dependencies vendored")
	}

	// 3. Generate swagger docs
	fmt.Println("\n→ Generating Swagger docs...")
	if err := runCommand(projectDir, "swag", "init", "-g", "main.go", "-o", "docs", "--parseVendor", "--parseDependency"); err != nil {
		// Try to install swag and retry
		fmt.Println("  Installing swag...")
		_ = runCommand(projectDir, "go", "install", "github.com/swaggo/swag/cmd/swag@latest")
		if err := runCommand(projectDir, "swag", "init", "-g", "main.go", "-o", "docs", "--parseVendor", "--parseDependency"); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠️  Warning: swag init failed: %v\n", err)
			fmt.Println("  Run manually: make swagger")
		} else {
			fmt.Println("  ✓ Swagger docs generated")
		}
	} else {
		fmt.Println("  ✓ Swagger docs generated")
	}

	// 4. Run go mod tidy again to update go.sum
	fmt.Println("\n→ Running go mod tidy (cleanup)...")
	_ = runCommand(projectDir, "go", "mod", "tidy")
	fmt.Println("  ✓ Done")

	fmt.Println("\n✅ Setup complete!")
	fmt.Printf("\nTo run the project:\n")
	fmt.Printf("  cd %s\n", projectDir)
	fmt.Printf("  make run\n")
	fmt.Printf("\nSwagger UI: http://localhost:<port>/swagger/index.html\n")
}

// runCommand runs a command in the specified directory
func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isCommandAvailable checks if a command is available in PATH
type TemplateData struct {
	GameCode          string
	GameCodeUpper     string
	GameCodeSnake     string
	ModulePath        string
	Port              int
	PayLines          int
	WithWire          bool
	CoreModulePath    string
	CoreModuleVersion string
}

// ProjectMetadata stores information about a generated project
type ProjectMetadata struct {
	GameCode          string    `json:"game_code"`
	GameCodeUpper     string    `json:"game_code_upper"`
	GameCodeSnake     string    `json:"game_code_snake"`
	ModulePath        string    `json:"module_path"`
	Port              int       `json:"port"`
	PayLines          int       `json:"pay_lines"`
	WithWire          bool      `json:"with_wire"`
	CoreModulePath    string    `json:"core_module_path"`
	CoreModuleVersion string    `json:"core_module_version"`
	CLIVersion        string    `json:"cli_version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

const metadataFileName = ".slotmodule.json"

// saveProjectMetadata saves project metadata to .slotmodule.json
func saveProjectMetadata(projectDir string, data TemplateData) error {
	metadata := ProjectMetadata{
		GameCode:          data.GameCode,
		GameCodeUpper:     data.GameCodeUpper,
		GameCodeSnake:     data.GameCodeSnake,
		ModulePath:        data.ModulePath,
		Port:              data.Port,
		PayLines:          data.PayLines,
		WithWire:          data.WithWire,
		CoreModulePath:    data.CoreModulePath,
		CoreModuleVersion: data.CoreModuleVersion,
		CLIVersion:        version,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	return writeMetadata(projectDir, metadata)
}

// writeMetadata writes metadata to file
func writeMetadata(projectDir string, metadata ProjectMetadata) error {
	metadataPath := filepath.Join(projectDir, metadataFileName)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath, data, 0644)
}

// loadProjectMetadata loads project metadata from .slotmodule.json
func loadProjectMetadata(projectDir string) (*ProjectMetadata, error) {
	metadataPath := filepath.Join(projectDir, metadataFileName)
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("project metadata not found: %w (run 'slotmodule create' or create .slotmodule.json manually)", err)
	}

	var metadata ProjectMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("invalid project metadata: %w", err)
	}

	return &metadata, nil
}

// metadataToTemplateData converts ProjectMetadata to TemplateData
func metadataToTemplateData(m *ProjectMetadata) TemplateData {
	return TemplateData{
		GameCode:          m.GameCode,
		GameCodeUpper:     m.GameCodeUpper,
		GameCodeSnake:     m.GameCodeSnake,
		ModulePath:        m.ModulePath,
		Port:              m.Port,
		PayLines:          m.PayLines,
		WithWire:          m.WithWire,
		CoreModulePath:    m.CoreModulePath,
		CoreModuleVersion: getCoreModuleVersion(), // Always use latest version
	}
}

func generateFile(path, tmpl string, data TemplateData) error {
	t, err := template.New("file").Parse(tmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return t.Execute(f, data)
}

func toUpperCamel(s string) string {
	parts := strings.Split(s, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// Templates
var goModTemplate = `module {{.ModulePath}}

go 1.25.4

require {{.CoreModulePath}} {{.CoreModuleVersion}}
`

var mainTemplate = `package main

import (
	"log"

	"{{.CoreModulePath}}/config"
	"{{.CoreModulePath}}/db/redis"
	"{{.CoreModulePath}}/events/kafka"
	"{{.CoreModulePath}}/logging"
	"{{.CoreModulePath}}/provider"
	"{{.CoreModulePath}}/server"
	"{{.CoreModulePath}}/pkg/jackpot"

	"{{.ModulePath}}/internal/logic"

	_ "{{.ModulePath}}/docs" // Swagger docs (generated by swag init)
	"{{.ModulePath}}/docs"   // Swagger docs for runtime host update

)

// @title           {{.GameCodeUpper}} Game API
// @version         1.0
// @description     Slot game service API for {{.GameCodeUpper}}

// @contact.name   FGS Backend Team
// @contact.url    https://futuregamestudio.net

// @host     
// @BasePath  /api

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

func main() {
	// 1. Load config & logger
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	logger := logging.New(cfg.Logging)

	// 2. Initialize dependencies
	redisClient, err := redis.New(cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	kafkaProducer, _ := kafka.NewProducer(cfg.Kafka.Brokers)

	// 3. Create app & set providers
	app := server.New(server.Options{Config: cfg, Logger: logger})
	app.SetStateProvider(provider.NewStateProvider(redisClient, logger))
	app.SetWalletProvider(provider.NewWalletProvider(cfg, logger))
	app.SetRewardProvider(provider.NewRewardProvider(cfg, logger))
	app.SetLogProvider(provider.NewLogProvider(cfg, kafkaProducer, logger))

	// 4. Register game module
	// Load from config directory (merges module-base.yml and {{.GameCodeSnake}}.yaml)
	// Can also use single file: "config/{{.GameCodeSnake}}.yaml"
	gameModule, err := logic.New{{.GameCodeUpper}}Module("config")
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize game module")
	}
	
	// Set jackpot service to register pools from config (if game module supports it)
	if app.GetJackpotService() != nil {
		if moduleWithJackpot, ok := any(gameModule).(interface{ SetJackpotService(*jackpot.Service) }); ok {
			moduleWithJackpot.SetJackpotService(app.GetJackpotService())
		}
	}
	
	app.RegisterGame(gameModule)

	// 5. Setup routes & features
	app.UseCommonMiddlewares()
	app.RegisterHealthCheck()
	app.RegisterCommonGameRoutes()
	app.RegisterSwagger(server.SwaggerInfo{Title: "{{.GameCodeUpper}} API", Version: "1.0"}, func(host string) {
		docs.SwaggerInfo.Host = host
	})


	// 6. Attach jackpot feed from Kafka (topic: jackpot-updates or config override)
	jackpotFeed := make(chan jackpot.Update, 256)
	app.AttachJackpotUpdateFeed(jackpotFeed)

	if len(cfg.Kafka.Brokers) > 0 {
		topic := "jackpot-updates"
		if cfg.Kafka.Topics != nil {
			if t, ok := cfg.Kafka.Topics["jackpot_updates"]; ok {
				topic = t
			}
		}
		consumer := kafka.NewConsumer(kafka.ConsumerConfig{
			Brokers:       cfg.Kafka.Brokers,
			Topic:         topic,
			ConsumerGroup: cfg.Kafka.ConsumerGroup + "-jackpot",
			Logger:        logger,
		}, kafka.NewPoolCache(logger))
		if err := consumer.Start(); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start jackpot Kafka consumer")
		}
		sub := consumer.SubscribeAll()
		go func() {
			for evt := range sub.Channel {
				jackpotFeed <- jackpot.Update{
					PoolID:    evt.PoolID,
					Amount:    evt.Amount,
					Timestamp: evt.UpdatedAt,
				}
			}
		}()
		app.OnShutdown(func() {
			consumer.Unsubscribe(sub)
			_ = consumer.Stop()
		})
	}

	// 7. Cleanup & run
	app.OnShutdown(func() {
		if kafkaProducer != nil {
			kafkaProducer.Close()
		}
		redisClient.Close()
	})

	logger.Info().Int("port", cfg.Server.Port).Msg("Starting {{.GameCodeUpper}} service")
	if err := app.Run(); err != nil {
		logger.Fatal().Err(err).Msg("Server error")
	}
}
`

var configGoTemplate = `package logic

import "{{.CoreModulePath}}/game"

// {{.GameCodeUpper}}Config holds custom game-specific configuration
// Embeds game.Config to inherit all base config fields (PayLine, ReelRows, etc.)
// Add your custom config fields here and they will be automatically loaded from YAML files
//
// Example usage in YAML ({{.GameCodeSnake}}.yaml):
//   custom_field: "value"
//   jackpot_config:
//     mini:
//       init: "3"
//       prog: "0.003"
type {{.GameCodeUpper}}Config struct {
	game.Config `mapstructure:",squash"` // Embed game.Config to get all base config fields (PayLine, ReelRows, ReelCols, etc.)

	// Add your custom config fields here
	// Example:
	// CustomField string ` + "`mapstructure:\"custom_field\"`" + `
	
	// Example: Jackpot configuration (uncomment and customize as needed)
	// JackpotConfig map[string]JackpotTierConfig ` + "`mapstructure:\"jackpot_config\"`" + `
}

// Normalize overrides game.Config.Normalize() to customize the response format
// This method is called when the game config is returned via API (e.g., GET /games/{game_code}/config)
// You can add custom fields to the response or modify existing ones
func (c *{{.GameCodeUpper}}Config) Normalize() map[string]interface{} {
	// Call base Normalize() to get base fields
	base := c.Config.Normalize()
	
	// Add or override custom fields
	// Example:
	// base["customField"] = c.CustomField
	// base["jackpotConfig"] = c.JackpotConfig
	
	return base
}

// GetConfig returns a pointer to the embedded Config (implements game.ConfigNormalizer interface).
// This allows extracting the base Config from custom config structs.
func (c *{{.GameCodeUpper}}Config) GetConfig() *game.Config {
	return &c.Config
}

// Example: JackpotTierConfig (uncomment if using jackpot)
// type JackpotTierConfig struct {
// 	Init string ` + "`mapstructure:\"init\"`" + ` // Initial value multiplier
// 	Prog string ` + "`mapstructure:\"prog\"`" + ` // Progressive contribution rate
// }
`

var moduleTemplate = `package logic

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/shopspring/decimal"
	"{{.CoreModulePath}}/game"
)

// {{.GameCodeUpper}}Module implements the game.Module interface
//
// Flow: gameRoutes -> gameHandler -> gameService -> gameModule
//
// This module can:
// - Embed game.BaseModule for common functionality
// - Implement game.JackpotHandler for custom jackpot logic (optional)
// - Access ModuleContext via game.MustFromContext(ctx) to get logger, providers, etc.
//
// Example:
//   type MyGameModule struct {
//       game.BaseModule              // Embed for GetConfig(), GetGameCode()
//       game.JackpotHandler          // Embed interface to implement custom jackpot
//       rng    *rand.Rand
//   }
type {{.GameCodeUpper}}Module struct {
	game.BaseModule              // Embed for GetConfig(), GetGameCode() - config is in BaseModule.Config
	// game.JackpotHandler        // Uncomment to implement custom jackpot logic
	rng        *rand.Rand
	gameConfig *{{.GameCodeUpper}}Config // Store full config to access fields directly
}

// New{{.GameCodeUpper}}Module creates a new game module
// configPath can be a single file or a directory containing multiple config files
// If directory: loads and merges all YAML files (e.g., module-base.yml, {{.GameCodeSnake}}.yaml)
// If file: loads single config file
// Automatically loads both base game config and custom game-specific config
func New{{.GameCodeUpper}}Module(configPath string) (*{{.GameCodeUpper}}Module, error) {
	// Load config with custom fields ({{.GameCodeUpper}}Config embeds game.Config)
	gameConfig, err := game.LoadGameConfig[{{.GameCodeUpper}}Config](configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load game config: %w", err)
	}

	// Create base module with loaded config
	// Store the full custom config (not just embedded Config) so Normalize() method works correctly
	base := &game.BaseModule{
		GameCode: "{{.GameCode}}",
		Config:   gameConfig, // Store full custom config to preserve Normalize() method
	}

	// TODO: Use gameConfig custom fields here if needed
	// Example: customField := gameConfig.CustomField

	return &{{.GameCodeUpper}}Module{
		BaseModule: *base,  // Embed BaseModule with config loaded
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		gameConfig: gameConfig, // Store full config to access fields (PayLine, ReelRows, etc.)
	}, nil
}

// GetConfig returns the game configuration
// Override to return the full custom config (with custom Normalize() method)
func (m *{{.GameCodeUpper}}Module) GetConfig(ctx context.Context) (game.ConfigNormalizer, error) {
	// Return the full custom config so Normalize() includes custom fields (e.g., jackpotConfig)
	return m.gameConfig, nil
}

// PlayNormalSpin executes a normal spin
//
// Flow: HTTP Request -> gameRoutes -> gameHandler -> gameService -> PlayNormalSpin (here)
//
// ModuleContext is ALWAYS available - use game.MustFromContext(ctx) to get it:
//
//	ctx := game.MustFromContext(ctx)
//	userID := ctx.User().ID()
//	username := ctx.User().Username()
//	currencyID := ctx.User().CurrencyID()
//	ctx.Logger.Info().Str("user_id", userID).Msg("Playing spin")
func (m *{{.GameCodeUpper}}Module) PlayNormalSpin(ctx context.Context, betMultiplier float32, cheatPayout interface{}) (*game.SpinResult, error) {
	// ModuleContext is set by middleware - get user info, logger, providers
	mc := game.MustFromContext(ctx)
	
	// User may be nil if no auth middleware - always check
	if user := mc.User(); user != nil {
		userID := user.ID()
		username := user.Username()
		currencyID := user.CurrencyID()
		
		// Use logger with user info
		mc.Logger.Info().
			Str("user_id", userID).
			Str("username", username).
			Str("currency", currencyID).
			Float32("bet_multiplier", betMultiplier).
			Msg("Playing normal spin")
	} else {
		// No user info available (no auth middleware)
		mc.Logger.Info().
			Float32("bet_multiplier", betMultiplier).
			Msg("Playing normal spin (no user)")
	}

	// TODO: Implement your game logic here
	
	// Generate reels
	reels := m.generateReels()

	// Use gameConfig directly (embeds game.Config, so we can access all fields)
	totalBet := decimal.NewFromFloat32(betMultiplier).Mul(decimal.NewFromInt(int64(m.gameConfig.PayLine)))

	// Calculate winlines
	winlines := m.calculateWinlines(reels, betMultiplier)

	// Calculate total win
	totalWin := decimal.Zero
	for _, wl := range winlines {
		totalWin = totalWin.Add(wl.WinAmount)
	}

	isGetFreeSpin := false
	isGetJackpot := false

	return &game.SpinResult{
		Reels:         reels,
		Winlines:      winlines,
		TotalWin:      totalWin,
		TotalBet:      totalBet,
		Multiplier:    1,
		IsGetFreeSpin: &isGetFreeSpin,
		IsGetJackpot:  &isGetJackpot,
		WinTitle:      m.determineWinTitle(totalWin.InexactFloat64(), totalBet.InexactFloat64()),
		SpinType:      0,
	}, nil
}

// PlayFreeSpin executes a free spin
func (m *{{.GameCodeUpper}}Module) PlayFreeSpin(ctx context.Context, betMultiplier float32) (*game.SpinResult, error) {
	// TODO: Implement free spin logic
	return m.PlayNormalSpin(ctx, betMultiplier, nil)
}

// GenerateFreeSpins pre-generates all free spin results
func (m *{{.GameCodeUpper}}Module) GenerateFreeSpins(ctx context.Context, betMultiplier float32, count int) ([]*game.SpinResult, error) {
	results := make([]*game.SpinResult, count)
	for i := 0; i < count; i++ {
		result, err := m.PlayFreeSpin(ctx, betMultiplier)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// generateReels generates the reel matrix
func (m *{{.GameCodeUpper}}Module) generateReels() [][]game.Symbol {
	// TODO: Implement reel generation based on your reel strips
	// Access config fields directly from gameConfig (embeds game.Config)
	rows := m.gameConfig.ReelRows
	if rows == 0 {
		rows = 3
	}
	cols := m.gameConfig.ReelCols
	if cols == 0 {
		cols = 5
	}
	
	matrix := make([][]game.Symbol, rows)
	for i := range matrix {
		matrix[i] = make([]game.Symbol, cols)
		for j := range matrix[i] {
			symbolID := m.rng.Intn(7) // Random symbol 0-6
			matrix[i][j] = game.Symbol{
				Symbol: symbolID,
				Value:  symbolID,
				Type:   0,
			}
		}
	}
	return matrix
}

// calculateWinlines calculates winning lines
func (m *{{.GameCodeUpper}}Module) calculateWinlines(reels [][]game.Symbol, betMultiplier float32) []game.Winline {
	// TODO: Implement winline calculation
	return []game.Winline{}
}

// determineWinTitle determines the win title based on win ratio
func (m *{{.GameCodeUpper}}Module) determineWinTitle(totalWin, totalBet float64) string {
	if totalWin == 0 || totalBet == 0 {
		return ""
	}
	ratio := totalWin / totalBet
	if ratio >= 50 {
		return "Mega Win"
	} else if ratio >= 20 {
		return "Super Win"
	} else if ratio >= 10 {
		return "Big Win"
	}
	return ""
}

// ============================================================================
// Optional: Implement game.JackpotHandler interface for custom jackpot logic
// ============================================================================
// To enable custom jackpot logic:
// 1. Uncomment "game.JackpotHandler" in the struct definition above
// 2. Uncomment and implement the methods below
// 3. Otherwise, base module will use default logic (3 pools: mini, minor, grand)
//
// Flow: gameService -> GetContributions/GetWin (here) -> rewardProvider

/*
// GetContributions returns jackpot contributions for a spin
// Called by gameService when IsGetJackpot is false
// You can access ModuleContext via game.FromContext(ctx) if needed
func (m *{{.GameCodeUpper}}Module) GetContributions(ctx context.Context, spinResult *game.SpinResult, totalBet decimal.Decimal, gameConfig *game.Config) ([]game.JackpotContribution, error) {
	// Example: Custom contribution logic
	// You can implement any logic here, e.g.:
	// - Different number of pools
	// - Different contribution rates
	// - Conditional contributions based on spin result
	
	gameCode := m.GetGameCode()
	contributions := []game.JackpotContribution{}
	
	// Example: Contribute to a single pool
	// contribution := decimal.NewFromString("0.01").Mul(totalBet) // 1% of bet
	// contributions = append(contributions, game.JackpotContribution{
	// 	PoolID: fmt.Sprintf("%s:main", gameCode),
	// 	Amount: contribution,
	// })
	
	return contributions, nil
}

// GetWin returns jackpot win information for a spin
// Called by gameService when IsGetJackpot is true
// You can access ModuleContext via game.FromContext(ctx) if needed
func (m *{{.GameCodeUpper}}Module) GetWin(ctx context.Context, spinResult *game.SpinResult, totalBet decimal.Decimal, gameConfig *game.Config) (*game.JackpotWin, error) {
	// Example: Custom win detection logic
	// You can implement any logic here, e.g.:
	// - Detect tier based on symbol positions
	// - Detect tier based on win amount
	// - Multiple pools with different rules
	
	gameCode := m.GetGameCode()
	
	// Example: Detect tier from spin result
	// tier := m.detectTierFromSpin(spinResult)
	// if tier == "" {
	// 	return nil, nil // No jackpot win
	// }
	
	// Example: Calculate init value
	// initValue := decimal.NewFromString("100").Mul(totalBet)
	
	// return &game.JackpotWin{
	// 	PoolID:    fmt.Sprintf("%s:%s", gameCode, tier),
	// 	Tier:      tier,
	// 	InitValue: initValue,
	// }, nil
	
	return nil, nil
}

// GetPoolID returns pool IDs for SSE updates
// This is used for jackpot SSE streaming
func (m *{{.GameCodeUpper}}Module) GetPoolID(ctx context.Context, gameCode string, betMultiplier float32, gameConfig *game.Config) ([]string, error) {
	// Example: Return pool IDs to stream
	// Can return multiple pools if your game has multiple jackpot pools
	
	// Example: Single pool
	// return []string{fmt.Sprintf("jackpot:%s", gameCode)}, nil
	
	// Example: Multiple pools
	// return []string{
	// 	fmt.Sprintf("%s:mini", gameCode),
	// 	fmt.Sprintf("%s:major", gameCode),
	// 	fmt.Sprintf("%s:grand", gameCode),
	// }, nil
	
	// Default: single pool
	return []string{fmt.Sprintf("jackpot:%s", gameCode)}, nil
}

// GetInitialPoolValue returns initial pool value for SSE
func (m *{{.GameCodeUpper}}Module) GetInitialPoolValue(ctx context.Context, poolID string, betMultiplier float32, gameConfig *game.Config) (decimal.Decimal, error) {
	// Example: Calculate initial value based on bet multiplier
	// This is used when connecting to SSE to show initial jackpot value
	
	betMult := decimal.NewFromFloat32(betMultiplier)
	baseBet := decimal.NewFromInt(int64(gameConfig.PayLine))
	jackpotMultiplier := decimal.NewFromInt(int64(gameConfig.JackpotMultiplier))
	return betMult.Mul(baseBet).Mul(jackpotMultiplier), nil
}
*/
`

var configTemplate = `environment: development

server:
  port: {{.Port}}
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
  enable_cors: true

redis:
  addr: redis-server:6379
  username: fgs
  password: K9mPx4Zq
  db: 0
  pool_size: 10
  min_idle_conns: 5

kafka:
  brokers:
    - kafka:9092
  consumer_group: {{.GameCode}}
  topics:
    audit: audit-events
    jackpot_updates: jackpot-updates

jwt:
  secret: your-secret-key-here
  expiration: 24h

logging:
  level: debug
  format: console
  output: stdout

external_services:
  wallet_service:
    base_url: http://wallet-service:8081
    timeout: 5s
  reward_service:
    base_url: http://reward-service:8387
    timeout: 5s
  log_service:
    base_url: http://log-service:8388
    timeout: 5s
`

var moduleBaseConfigTemplate = `# Base module configuration (shared across games)
# This file contains common configuration that can be reused

# Game base settings
game_code: "{{.GameCode}}"
pay_line: {{.PayLines}}
jackpot_multiplier: 5000
reel_size: [3, 3, 3, 3, 3]

# Symbol mapping
# 0-3: Low pay symbols
# 4-6: High pay symbols
# 7-10: Wild symbols
# 11: Jackpot symbol

valid_symbols: [0, 1, 2, 3, 4, 5, 6]
wild_symbols: [7, 8, 9, 10]
scatter_symbol: -1
jackpot_symbol: 11

# Winlines ({{.PayLines}} lines)
winlines:
  - [0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0]
  - [1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0]
  - [0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1]
  # TODO: Add more winlines

# Pay table
pay_table:
  - [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
  - [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
  - [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
  - [2, 3, 4, 6, 10, 12, 20, 0, 0, 0, 0, 0]
  - [4, 6, 8, 12, 20, 24, 50, 0, 0, 0, 0, 0]
  - [6, 9, 12, 22, 45, 55, 100, 0, 0, 0, 0, 0]

# Reel strips
reel_strip:
  - 
    - [7, 6, 5, 4, 3, 2, 2, 0, 0, 0, 0, 1]
    - [9, 10, 5, 4, 3, 2, 2, 2, 2, 0, 0, 1]
    - [2, 2, 3, 3, 4, 5, 4, 1, 1, 1, 1, 1]
    - [2, 2, 3, 5, 6, 4, 4, 1, 1, 1, 1, 1]
    - [7, 7, 6, 5, 6, 7, 8, 1, 1, 1, 1, 1]
`

var gameSpecificConfigTemplate = `# {{.GameCodeUpper}} Game-Specific Configuration
# This file contains game-specific customizations that override module-base.yml
# Add your custom configuration fields here

# Example: Jackpot configuration (map structure for easy extension)
# Format: map[tier]{init, prog}
# init: initial value multiplier (e.g., "3" means 3×TotalBet)
# prog: progressive contribution rate (e.g., "0.003" means 0.3% of TotalBet)
# TODO: Customize jackpot_config for your game
jackpot_config:
  mini:
    init: "3"
    prog: "0.003"
  minor:
    init: "75"
    prog: "0.007"
  grand:
    init: "1500"
    prog: "0.02"

# TODO: Add more custom fields here as needed
# Example:
# custom_field:
#   setting1: value1
#   setting2: value2
`

var dockerfileTemplate = `# syntax=docker/dockerfile:1.4
# Build stage
FROM golang:1.25.4-alpine AS builder

# Install git for downloading dependencies
RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./

# Download dependencies
# Remove replace directives with relative paths (they don't work in Docker builds)
RUN sed -i '/^replace.*=> \.\./d' go.mod && \
    go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/{{.GameCode}} main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bin/{{.GameCode}} .
COPY --from=builder /app/config ./config

ENV TZ=Asia/Ho_Chi_Minh

EXPOSE {{.Port}}

CMD ["./{{.GameCode}}"]
`

var dockerComposeTemplate = `version: '3.8'

services:
  {{.GameCode}}:
    build:
      context: .
    ports:
      - "{{.Port}}:{{.Port}}"
    environment:
      - ENV=development
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - KAFKA_BROKERS=kafka:9092
      - JWT_SECRET=toi-dai-dot-123
    depends_on:
      - redis
    networks:
      - game-network

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - game-network

networks:
  game-network:
    driver: bridge
`

var gitlabCITemplate = `stages:
  - build
  - deploy
  - notify

workflow:
  rules:
    - if: '$CI_COMMIT_BRANCH == "main"'
    - when: always

include:
  - project: "fgs/devops/cicd"
    ref: feat/template-service-cicd
    file:
      - templates/service.yml

variables:
  CONTAINER_NAME: game-service-{{.GameCode}}
  SERVICE_NAME: game-service-{{.GameCode}}
  PORTS: "{{.Port}}:{{.Port}}"
  NETWORKS: "net"
  GAME_CODE: "{{.GameCode}}"
  GAME_SLUG: "{{.GameCode}}"
  GAME_PATH: "{{.GameCode}}"
  GAME_PORT: "{{.Port}}"

build-{{.GameCode}}:
  extends: .build
  stage: build
  tags:
    - fgs-dind-runner-vn

deploy-{{.GameCode}}:
  extends: .deploy
  stage: deploy
  needs:
    - build-{{.GameCode}}
  tags:
    - fgs-dind-runner-vn

update-nginx-{{.GameCode}}:
  extends:
    - .service
    - .update-nginx
  needs:
    - deploy-{{.GameCode}}
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
      when: on_success
      allow_failure: false
  tags:
    - fgs-dind-runner-vn

notify-{{.GameCode}}:
  extends: .notify
  stage: notify
  needs:
    - update-nginx-{{.GameCode}}
  tags:
    - fgs-dind-runner-vn
`

var makefileTemplate = `.PHONY: run build test clean docker-build docker-run swagger vendor

# Variables
APP_NAME = {{.GameCode}}
PORT = {{.Port}}

# Run the application
run:
	go run main.go

# Build the application  
build:
	go build -o bin/$(APP_NAME) main.go

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf docs/
	rm -rf vendor/

# Download dependencies
deps:
	go mod tidy
	go mod download

# Vendor dependencies (required for swagger to parse module annotations)
vendor:
	go mod vendor

# Generate swagger docs
# Uses --parseVendor to scan module code in vendor/
swagger: vendor
	@which swag > /dev/null || (echo "Installing swag..." && go install github.com/swaggo/swag/cmd/swag@latest)
	swag init -g main.go -o docs --parseVendor --parseDependency

# Docker build
docker-build:
	docker build -t $(APP_NAME):latest .

# Docker run
docker-run:
	docker run -p $(PORT):$(PORT) $(APP_NAME):latest

# Docker compose up
up:
	docker-compose up -d

# Docker compose down
down:
	docker-compose down

# Install swag CLI (one-time setup)
install-swag:
	go install github.com/swaggo/swag/cmd/swag@latest

# Lint
lint:
	golangci-lint run ./...

# Full build with swagger
release: swagger build
`

var readmeTemplate = `# {{.GameCodeUpper}} Game Service

A slot game service built using the [Slot Game Module](https://git.futuregamestudio.net/be-shared/slot-game-module.git).

## Quick Start

### Prerequisites

- Go 1.25.4+
- Redis
- Docker (optional)
- swag CLI (for swagger docs)

### Running Locally

` + "```bash" + `
# Install dependencies
make deps

# Generate swagger docs (first time or after API changes)
make swagger

# Run the service
make run
` + "```" + `

### Using Docker

` + "```bash" + `
# Build and run with docker-compose
make up

# Or build and run manually
make docker-build
make docker-run
` + "```" + `

## API Documentation

Swagger UI is available at: ` + "`http://localhost:{{.Port}}/swagger/index.html`" + `

### Generate Swagger Docs

` + "```bash" + `
# Install swag CLI (one-time)
make install-swag

# Generate docs
make swagger
` + "```" + `

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | ` + "`/health`" + ` | Health check |
| GET | ` + "`/swagger/*`" + ` | Swagger UI |
| POST | ` + "`/api/games/{{.GameCode}}/authorize-game`" + ` | Authorize game |
| POST | ` + "`/api/games/{{.GameCode}}/spin`" + ` | Execute spin |
| GET | ` + "`/api/games/{{.GameCode}}/config`" + ` | Get config |
| GET | ` + "`/api/games/{{.GameCode}}/get-player-state`" + ` | Get player state |
| POST | ` + "`/api/games/{{.GameCode}}/jackpot/updates`" + ` | SSE jackpot updates |
| GET | ` + "`/api/games/{{.GameCode}}/jackpot-info`" + ` | Get jackpot info |

## Example Requests

### Spin

` + "```bash" + `
curl -X POST http://localhost:{{.Port}}/api/games/{{.GameCode}}/spin \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"betMultiplier": 1}'
` + "```" + `

### SSE Jackpot Updates

` + "```bash" + `
curl -N -X POST http://localhost:{{.Port}}/api/games/{{.GameCode}}/jackpot/updates \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"betMultiplier": 1}'
` + "```" + `

## Configuration

Edit ` + "`config/config.yaml`" + ` for app configuration and ` + "`config/{{.GameCodeSnake}}.yaml`" + ` for game-specific configuration.

## Project Structure

` + "```" + `
{{.GameCode}}/
├── config/
│   ├── config.yaml           # App configuration
│   └── {{.GameCodeSnake}}.yaml  # Game configuration
├── docs/                     # Swagger generated docs (generated)
├── internal/
│   └── logic/
│       └── module.go         # Game logic (only this!)
├── main.go                   # Entry point
├── Dockerfile
├── docker-compose.yml
├── .gitlab-ci.yml
└── Makefile
` + "```" + `

> **Note**: Providers are included in core - you don't need to write them!

## License

Proprietary - Future Game Studio
`

var gitignoreTemplate = `# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary
*.test

# Output of the go coverage tool
*.out

# Dependency directories
vendor/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Logs
*.log

# Environment
.env
.env.local

# Build
dist/
`

var docsPlaceholderTemplate = `// Package docs - Swagger documentation
// Run 'make swagger' to regenerate.
package docs

import "github.com/swaggo/swag"

func init() {
	swag.Register(swag.Name, &swag.Spec{
		InfoInstanceName: "swagger",
		SwaggerTemplate:  ` + "`" + `{"swagger":"2.0","info":{"title":"{{.GameCodeUpper}} API","version":"1.0"}}` + "`" + `,
	})
}
`
