package version

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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
		slog.Error("Could not load kosh.yaml")
		return
	}

	for _, v := range cfg.Versions {
		if v.Name == versionName {
			slog.Error("Version already exists", "version", versionName)
			return
		}
	}

	latestIdx, latestVersion := findLatestVersion(cfg)
	if latestVersion == nil {
		slog.Error("No current 'latest' version found in config")
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
				slog.Error("Directory already exists and is registered as a version", "dir", frozenDestDir)
				return
			}
			slog.Warn("Directory exists but is not registered. Renaming to backup...", "dir", frozenDestDir)
			backupDir := frozenDestDir + ".backup"
			if err := vfs.Rename(frozenDestDir, backupDir); err != nil {
				slog.Error("Could not rename existing directory", "error", err)
				return
			}
			slog.Info("Backup created", "path", backupDir)
		}
		slog.Info("Freezing version", "version", latestVersion.Name, "path", "content/"+frozenPath+"/")
		if err := snapshotContentFs(vfs, frozenDestDir, frozenSourceDir, cfg); err != nil {
			slog.Error("Error during snapshot", "error", err)
			return
		}
	}

	// Handle new version folder creation (all-in-folders style)
	if newVersionDir != "" {
		if exists, _ := afero.Exists(vfs, newVersionDir); exists {
			slog.Error("Directory already exists", "dir", newVersionDir)
			return
		}
		// Copy from current latest version
		sourceDir := filepath.Join("content", latestVersion.Path)
		slog.Info("Creating new version", "version", versionName, "path", "content/"+versionName+"/")
		if err := snapshotContentFs(vfs, newVersionDir, sourceDir, cfg); err != nil {
			slog.Error("Error creating new version", "error", err)
			return
		}
	}

	slog.Info("Updating version configuration...")

	if err := updateVersionConfigFs(vfs, cfg, latestIdx, versionName, frozenPath, newVersionPath); err != nil {
		slog.Error("Error updating kosh.yaml", "error", err)
		return
	}

	slog.Info("Version transition complete")
	if latestVersion.Path != "" {
		slog.Info("Previous latest frozen", "name", latestVersion.Name, "path", latestVersion.Path)
		slog.Info("New working version", "name", versionName, "path", versionName)
	} else {
		slog.Info("Previous latest frozen", "name", latestVersion.Name, "path", frozenPath)
		slog.Info("New working version", "name", versionName, "path", "root")
	}
	slog.Info("Config updated with proper version ordering")
}

func printVersionInfoFs(fs afero.Fs) {
	cfg := loadConfigFs(fs)
	if cfg == nil {
		slog.Error("Could not load kosh.yaml")
		return
	}

	slog.Info("Version Information")

	if len(cfg.Versions) == 0 {
		slog.Info("No versions configured")
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
		slog.Info(fmt.Sprintf("%s %d. %s - %s", marker, i+1, v.Name, path), "latest", v.IsLatest)
	}

	_, latest := findLatestVersion(cfg)
	if latest != nil {
		slog.Info("Current working version", "name", latest.Name)
		slog.Info("Edit files in content/ to update this version")
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

	var mu sync.Mutex
	return fspkg.ParallelWalk(context.Background(), vfs, sourceDir, 0, func(path string, info fs.FileInfo, err error) error {
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
			mu.Lock()
			skip := sourceDir == "content" && len(parts) > 0 && versionPaths[parts[0]]
			mu.Unlock()
			if skip {
				return filepath.SkipDir
			}
			return vfs.MkdirAll(filepath.Join(destDir, relPath), 0755)
		}

		if strings.HasSuffix(path, ".md") {
			// Skip files in other version directories when source is root
			mu.Lock()
			skip := sourceDir == "content" && len(parts) > 1 && versionPaths[parts[0]]
			mu.Unlock()
			if skip {
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
