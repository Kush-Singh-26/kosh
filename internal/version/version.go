package version

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

func Run(args []string) {
	RunFs(afero.NewOsFs(), args)
}

func RunFs(vfs afero.Fs, args []string) {
	if len(args) < 1 {
		printVersionInfoFs(vfs)
		return
	}

	versionName := args[0]

	cfg := loadConfigFs(vfs)
	if cfg == nil {
		fmt.Println("❌ Error: Could not load kosh.yaml")
		return
	}

	for _, v := range cfg.Versions {
		if v.Name == versionName {
			fmt.Printf("❌ Error: Version '%s' already exists in kosh.yaml\n", versionName)
			return
		}
	}

	latestIdx, latestVersion := findLatestVersion(cfg)
	if latestVersion == nil {
		fmt.Println("❌ Error: No current 'latest' version found in config")
		return
	}

	// Build map of existing version paths
	versionPaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			versionPaths[v.Path] = true
		}
	}

	var frozenPath string      // Path for the old latest version after freezing
	var frozenSourceDir string // Where to copy frozen content FROM
	var newVersionPath string  // Path for the new latest version
	var newVersionDir string   // Directory to create for new version

	if latestVersion.Path != "" {
		// All-in-folders style: latest has explicit path (e.g., v7.0)
		// - Old latest stays where it is (already in folder)
		// - New version gets a NEW folder
		frozenPath = latestVersion.Path
		frozenSourceDir = ""         // No need to copy frozen content
		newVersionPath = versionName // e.g., "v8.0"
		newVersionDir = filepath.Join("content", versionName)
	} else {
		// Traditional style: latest at root
		// - Copy root content to frozen folder
		// - New version stays at root
		frozenPath = latestVersion.Name
		frozenSourceDir = "content"
		newVersionPath = "" // New latest at root
		newVersionDir = ""  // No new folder needed
	}

	// Handle frozen content copy (traditional style only)
	if frozenSourceDir != "" {
		frozenDestDir := filepath.Join("content", frozenPath)
		if exists, _ := afero.Exists(vfs, frozenDestDir); exists {
			if versionPaths[frozenPath] {
				fmt.Printf("❌ Error: Directory '%s' already exists and is registered as a version\n", frozenDestDir)
				return
			}
			fmt.Printf("⚠️  Warning: Directory '%s' exists but is not registered. Renaming to backup...\n", frozenDestDir)
			backupDir := frozenDestDir + ".backup"
			if err := vfs.Rename(frozenDestDir, backupDir); err != nil {
				fmt.Printf("❌ Error: Could not rename existing directory: %v\n", err)
				return
			}
			fmt.Printf("   Backup created at: %s\n", backupDir)
		}
		fmt.Printf("📸 Freezing version %s to content/%s/...\n", latestVersion.Name, frozenPath)
		if err := snapshotContentFs(vfs, frozenDestDir, frozenSourceDir, cfg); err != nil {
			fmt.Printf("❌ Error during snapshot: %v\n", err)
			return
		}
	}

	// Handle new version folder creation (all-in-folders style)
	if newVersionDir != "" {
		if exists, _ := afero.Exists(vfs, newVersionDir); exists {
			fmt.Printf("❌ Error: Directory '%s' already exists\n", newVersionDir)
			return
		}
		// Copy from current latest version
		sourceDir := filepath.Join("content", latestVersion.Path)
		fmt.Printf("📸 Creating new version %s at content/%s/...\n", versionName, versionName)
		if err := snapshotContentFs(vfs, newVersionDir, sourceDir, cfg); err != nil {
			fmt.Printf("❌ Error creating new version: %v\n", err)
			return
		}
	}

	fmt.Printf("📝 Updating version configuration...\n")

	if err := updateVersionConfigFs(vfs, cfg, latestIdx, versionName, frozenPath, newVersionPath); err != nil {
		fmt.Printf("❌ Error updating kosh.yaml: %v\n", err)
		return
	}

	fmt.Printf("\n✅ Version transition complete!\n")
	if latestVersion.Path != "" {
		fmt.Printf("   Previous latest: %s (content/%s/) → frozen\n", latestVersion.Name, latestVersion.Path)
		fmt.Printf("   New working version: %s (content/%s/)\n", versionName, versionName)
	} else {
		fmt.Printf("   Previous latest: %s → frozen at content/%s/\n", latestVersion.Name, frozenPath)
		fmt.Printf("   New working version: %s (content/ root)\n", versionName)
	}
	fmt.Printf("   Config updated with proper version ordering\n")
}

func printVersionInfo() {
	printVersionInfoFs(afero.NewOsFs())
}

func printVersionInfoFs(fs afero.Fs) {
	cfg := loadConfigFs(fs)
	if cfg == nil {
		fmt.Println("❌ Error: Could not load kosh.yaml")
		return
	}

	fmt.Println("📚 Version Information")
	fmt.Println("======================")

	if len(cfg.Versions) == 0 {
		fmt.Println("   No versions configured")
		return
	}

	for i, v := range cfg.Versions {
		marker := "  "
		if v.IsLatest {
			marker = "→ " // Arrow for latest
		}
		path := v.Path
		if path == "" {
			path = "(root)"
		}
		fmt.Printf("%s %d. %s - %s", marker, i+1, v.Name, path)
		if v.IsLatest {
			fmt.Printf(" [LATEST]")
		}
		fmt.Println()
	}

	_, latest := findLatestVersion(cfg)
	if latest != nil {
		fmt.Printf("\n💡 Current working version: %s\n", latest.Name)
		fmt.Println("   Edit files in content/ to update this version")
	}
}

func findLatestVersion(cfg *config.Config) (int, *config.Version) {
	for i := range cfg.Versions {
		if cfg.Versions[i].IsLatest {
			return i, &cfg.Versions[i]
		}
	}
	return -1, nil
}

func snapshotContent(destDir string, sourceDir string, cfg *config.Config) error {
	return snapshotContentFs(afero.NewOsFs(), destDir, sourceDir, cfg)
}

func snapshotContentFs(vfs afero.Fs, destDir string, sourceDir string, cfg *config.Config) error {
	if err := vfs.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	versionPaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			versionPaths[v.Path] = true
		}
	}

	// Add destination to versionPaths so we don't try to copy into it
	rel, _ := filepath.Rel("content", destDir)
	if rel != "." && rel != "" {
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) > 0 {
			versionPaths[parts[0]] = true
		}
	}

	return afero.Afero{Fs: vfs}.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == sourceDir {
			return nil
		}

		relPath, _ := filepath.Rel(sourceDir, path)
		parts := strings.Split(relPath, string(os.PathSeparator))

		if info.IsDir() {
			// Skip if this is another version's directory (when source is root)
			if sourceDir == "content" && len(parts) > 0 && versionPaths[parts[0]] {
				return filepath.SkipDir
			}
			return vfs.MkdirAll(filepath.Join(destDir, relPath), 0755)
		}

		if strings.HasSuffix(path, ".md") {
			// Skip files in other version directories when source is root
			if sourceDir == "content" && len(parts) > 1 && versionPaths[parts[0]] {
				return nil
			}

			destPath := filepath.Join(destDir, relPath)
			if err := copyFileFs(vfs, path, destPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", path, err)
			}
		}

		return nil
	})
}

func copyFile(src, dst string) error {
	return copyFileFs(afero.NewOsFs(), src, dst)
}

func copyFileFs(vfs afero.Fs, src, dst string) error {
	source, err := vfs.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := source.Close(); cerr != nil {
			slog.Warn("Failed to close source file", "path", src, "error", cerr)
		}
	}()

	destination, err := vfs.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := destination.Close(); cerr != nil {
			slog.Warn("Failed to close destination file", "path", dst, "error", cerr)
		}
	}()

	_, err = io.Copy(destination, source)
	return err
}

func loadConfig() *config.Config {
	return loadConfigFs(afero.NewOsFs())
}

func loadConfigFs(vfs afero.Fs) *config.Config {
	data, err := afero.ReadFile(vfs, "kosh.yaml")
	if err != nil {
		return nil
	}
	cfg := &config.Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil
	}
	return cfg
}

func updateVersionConfig(cfg *config.Config, oldLatestIdx int, newVersionName, frozenPath string, newLatestPath string) error {
	return updateVersionConfigFs(afero.NewOsFs(), cfg, oldLatestIdx, newVersionName, frozenPath, newLatestPath)
}

func updateVersionConfigFs(vfs afero.Fs, cfg *config.Config, oldLatestIdx int, newVersionName, frozenPath string, newLatestPath string) error {
	data, err := afero.ReadFile(vfs, "kosh.yaml")
	if err != nil {
		return err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("invalid yaml structure")
	}

	var versionsNode *yaml.Node
	mapping := root.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "versions" {
			versionsNode = mapping.Content[i+1]
			break
		}
	}

	if versionsNode == nil || versionsNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("versions not found or not a sequence")
	}

	newLatestNode := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "name"},
			{Kind: yaml.ScalarNode, Value: newVersionName},
			{Kind: yaml.ScalarNode, Value: "path"},
			{Kind: yaml.ScalarNode, Value: newLatestPath},
			{Kind: yaml.ScalarNode, Value: "isLatest"},
			{Kind: yaml.ScalarNode, Value: "true"},
		},
	}

	for _, versionNode := range versionsNode.Content {
		if versionNode.Kind != yaml.MappingNode {
			continue
		}
		var isCurrentlyLatest bool
		var currentPath string
		for i := 0; i < len(versionNode.Content); i += 2 {
			key := versionNode.Content[i].Value
			if key == "isLatest" && versionNode.Content[i+1].Value == "true" {
				isCurrentlyLatest = true
			}
			if key == "path" {
				currentPath = versionNode.Content[i+1].Value
			}
		}
		for i := 0; i < len(versionNode.Content); i += 2 {
			key := versionNode.Content[i].Value
			if key == "isLatest" {
				versionNode.Content[i+1].Value = "false"
			}
			if key == "path" {
				// If old latest had empty path (root), set it to frozenPath
				// If old latest had explicit path, keep it (content is already there)
				if isCurrentlyLatest && currentPath == "" {
					versionNode.Content[i+1] = &yaml.Node{
						Kind:  yaml.ScalarNode,
						Value: frozenPath,
						Tag:   "",
					}
				}
			}
		}
	}

	newVersions := []*yaml.Node{newLatestNode}
	newVersions = append(newVersions, versionsNode.Content...)
	versionsNode.Content = newVersions

	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}

	return afero.WriteFile(vfs, "kosh.yaml", out, 0644)
}
