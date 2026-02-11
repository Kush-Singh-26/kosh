package version

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"my-ssg/builder/config"
)

// Run takes a snapshot of current content and creates a new version
func Run(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: kosh version <vX.X>")
		return
	}

	versionName := args[0]
	versionPath := versionName // Use name as path for now

	// 1. Load current config to check for existing versions and version folders
	cfg := loadConfig()
	if cfg == nil {
		fmt.Println("❌ Error: Could not load kosh.yaml")
		return
	}

	// 2. Validate version name
	for _, v := range cfg.Versions {
		if v.Name == versionName || v.Path == versionPath {
			fmt.Printf("❌ Error: Version '%s' or path '%s' already exists in kosh.yaml\n", versionName, versionPath)
			return
		}
	}

	destDir := filepath.Join("content", versionPath)
	if _, err := os.Stat(destDir); err == nil {
		fmt.Printf("❌ Error: Directory '%s' already exists\n", destDir)
		return
	}

	// 3. Snapshot content
	fmt.Printf("📸 Taking snapshot for version %s...\n", versionName)

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Printf("❌ Error: Failed to create directory: %v\n", err)
		return
	}

	// Collect version paths to ignore
	versionPaths := make(map[string]bool)
	for _, v := range cfg.Versions {
		if v.Path != "" {
			versionPaths[v.Path] = true
		}
	}
	// Also ignore the one we are creating
	versionPaths[versionPath] = true

	err := filepath.WalkDir("content", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip "content" itself
		if path == "content" {
			return nil
		}

		relPath, _ := filepath.Rel("content", path)
		parts := strings.Split(relPath, string(os.PathSeparator))

		// If it's a directory and starts with a version path we know, skip it
		if d.IsDir() {
			if versionPaths[parts[0]] {
				return filepath.SkipDir
			}
			// Create directory in destination
			return os.MkdirAll(filepath.Join(destDir, relPath), 0755)
		}

		// Copy .md files
		if strings.HasSuffix(path, ".md") {
			// Ensure it's not in a version subdirectory (extra safety)
			if len(parts) > 1 && versionPaths[parts[0]] {
				return nil
			}

			destPath := filepath.Join(destDir, relPath)
			if err := copyFile(path, destPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("❌ Error during snapshot: %v\n", err)
		return
	}

	// 4. Update kosh.yaml
	if err := updateConfig(versionName, versionPath); err != nil {
		fmt.Printf("❌ Error updating kosh.yaml: %v\n", err)
		return
	}

	fmt.Printf("✅ Version %s created successfully as a snapshot!\n", versionName)
	fmt.Println("   New content at:", destDir)
	fmt.Println("   Updated kosh.yaml with strategy: snapshot")
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func loadConfig() *config.Config {
	data, err := os.ReadFile("kosh.yaml")
	if err != nil {
		return nil
	}
	cfg := &config.Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil
	}
	return cfg
}

func updateConfig(name, path string) error {
	data, err := os.ReadFile("kosh.yaml")
	if err != nil {
		return err
	}

	// Use yaml.Node to preserve comments
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	// Find the 'versions' node
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

	newVersion := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "name"},
			{Kind: yaml.ScalarNode, Value: name},
			{Kind: yaml.ScalarNode, Value: "path"},
			{Kind: yaml.ScalarNode, Value: path},
			{Kind: yaml.ScalarNode, Value: "strategy"},
			{Kind: yaml.ScalarNode, Value: "snapshot"},
		},
	}

	if versionsNode == nil {
		// Create 'versions' key if it doesn't exist
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "versions"}
		valNode := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{newVersion}}
		mapping.Content = append(mapping.Content, keyNode, valNode)
	} else {
		// Append to existing versions
		// We want to insert it AFTER the latest version (which is usually first)
		// but before older ones? Or just at the end?
		// Docusaurus puts newer ones at the top, but Latest is always root.

		// Let's just append for now, or insert after Latest.
		// For now, append is simplest.
		versionsNode.Content = append(versionsNode.Content, newVersion)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}

	return os.WriteFile("kosh.yaml", out, 0644)
}
