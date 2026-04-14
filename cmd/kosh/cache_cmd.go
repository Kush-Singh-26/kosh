package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
)

const (
	bytesPerMiB               = 1024 * 1024
	percentScale              = 100
	defaultMinBuildsBetweenGC = 0
	hashPreviewLen            = 16
	hashEdgeLen               = 8
)

var (
	cacheDryRun bool
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Cache management commands",
	Long:  `Manage the Kosh cache. Includes stats, garbage collection, verification, and inspection.`,
}

var cacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	Run:   runCacheStats,
}

var cacheGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Run garbage collection on cache",
	Run:   runCacheGC,
}

var cacheVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Check cache integrity",
	Run:   runCacheVerify,
}

var cacheRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Clear cache for full rebuild",
	Run:   runCacheRebuild,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete all cache data",
	Run:   runCacheClear,
}

var cacheInspectCmd = &cobra.Command{
	Use:   "inspect <path>",
	Short: "Show cache entry for a file",
	Args:  cobra.ExactArgs(1),
	Run:   runCacheInspect,
}

func init() {
	rootCmd.AddCommand(cacheCmd)

	cacheCmd.AddCommand(cacheStatsCmd)
	cacheCmd.AddCommand(cacheGCCmd)
	cacheCmd.AddCommand(cacheVerifyCmd)
	cacheCmd.AddCommand(cacheRebuildCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cacheInspectCmd)

	cacheGCCmd.Flags().BoolVarP(&cacheDryRun, "dry-run", "n", false, "Show what would be deleted without deleting")
}

func openCache() *cache.Manager {
	cfg := config.Load([]string{})
	cm, err := cache.Open(cfg.CacheDir, false)
	if err != nil {
		fmt.Printf("❌ Failed to open cache: %v\n", err)
		os.Exit(1)
	}
	return cm
}

func runCacheStats(cmd *cobra.Command, args []string) {
	cm := openCache()
	defer func() { _ = cm.Close() }()

	stats, err := cm.Stats()
	if err != nil {
		fmt.Printf("❌ Failed to get stats: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("📊 Cache Statistics")
	fmt.Println("════════════════════════════════════════")
	fmt.Printf("Schema Version:  %d\n", stats.SchemaVersion)
	fmt.Printf("Total Posts:     %d\n", stats.TotalPosts)
	fmt.Printf("Total SSR:       %d artifacts\n", stats.TotalSSR)
	fmt.Printf("Store Size:      %.2f MB\n", float64(stats.StoreBytes)/bytesPerMiB)
	fmt.Printf("Build Count:     %d\n", stats.BuildCount)

	if stats.LastGC > 0 {
		fmt.Printf("Last GC:         %s\n", time.Unix(stats.LastGC, 0).Format(time.RFC3339))
	} else {
		fmt.Printf("Last GC:         never\n")
	}

	fmt.Println("\n📦 Storage Metrics")
	fmt.Println("────────────────────────────────────────")
	fmt.Printf("Inline Posts:    %d (%.1f%%)\n", stats.InlinePosts, float64(stats.InlinePosts)*percentScale/float64(stats.TotalPosts))
	fmt.Printf("Hashed Posts:    %d (%.1f%%)\n", stats.HashedPosts, float64(stats.HashedPosts)*percentScale/float64(stats.TotalPosts))
}

func runCacheGC(cmd *cobra.Command, args []string) {
	cm := openCache()
	defer func() { _ = cm.Close() }()

	cfg := cache.DefaultGCConfig()
	cfg.DryRun = cacheDryRun
	cfg.MinBuildsBetweenGC = defaultMinBuildsBetweenGC

	if cacheDryRun {
		fmt.Println("🗑️  Running GC (dry run)...")
	} else {
		fmt.Println("🗑️  Running garbage collection...")
	}

	result, err := cm.RunGC(cfg)
	if err != nil {
		fmt.Printf("❌ GC failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("════════════════════════════════════════")
	fmt.Printf("Scanned:    %d blobs\n", result.ScannedBlobs)
	fmt.Printf("Live:       %d blobs\n", result.LiveBlobs)
	fmt.Printf("Deleted:    %d blobs (%.2f MB)\n", result.DeletedBlobs, float64(result.DeletedBytes)/bytesPerMiB)
	fmt.Printf("Duration:   %v\n", result.Duration)

	if cacheDryRun {
		fmt.Println("\n(No changes made - dry run mode)")
	} else {
		fmt.Println("\n✅ GC complete")
	}
}

func runCacheVerify(cmd *cobra.Command, args []string) {
	cm := openCache()
	defer func() { _ = cm.Close() }()

	fmt.Println("🔍 Verifying cache integrity...")

	errors, err := cm.Verify()
	if err != nil {
		fmt.Printf("❌ Verification failed: %v\n", err)
		os.Exit(1)
	}

	if len(errors) == 0 {
		fmt.Println("✅ Cache is healthy - no issues found")
	} else {
		fmt.Printf("⚠️  Found %d issues:\n", len(errors))
		for i, e := range errors {
			fmt.Printf("  %d. %s\n", i+1, e)
		}
	}
}

func runCacheRebuild(cmd *cobra.Command, args []string) {
	cm := openCache()
	defer func() { _ = cm.Close() }()

	fmt.Println("🔄 Clearing cache for rebuild...")

	if err := cm.Rebuild(); err != nil {
		fmt.Printf("❌ Failed to clear cache: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Cache cleared. Run 'kosh build' to rebuild.")
}

func runCacheClear(cmd *cobra.Command, args []string) {
	cm := openCache()
	defer func() { _ = cm.Close() }()

	fmt.Println("🗑️  Clearing all cache data...")

	if err := cm.Clear(); err != nil {
		fmt.Printf("❌ Failed to clear cache: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Cache cleared")
}

func runCacheInspect(cmd *cobra.Command, args []string) {
	path := args[0]
	cm := openCache()
	defer func() { _ = cm.Close() }()

	post, err := cm.GetPostByPath(path)
	if err != nil {
		fmt.Printf("❌ Error looking up path: %v\n", err)
		os.Exit(1)
	}

	if post == nil {
		fmt.Printf("❌ No cache entry found for: %s\n", path)
		os.Exit(1)
	}

	fmt.Println("📄 Cache Entry")
	fmt.Println("════════════════════════════════════════")
	fmt.Printf("PostID:       %s\n", post.PostID)
	fmt.Printf("Path:         %s\n", post.Path)
	fmt.Printf("Title:        %s\n", post.Title)
	fmt.Printf("ModTime:      %s\n", time.Unix(post.ModTime, 0).Format(time.RFC3339))
	fmt.Printf("ContentHash:  %s\n", truncateHash(post.ContentHash))
	fmt.Printf("HTMLHash:     %s\n", truncateHash(post.HTMLHash))
	fmt.Printf("Date:         %s\n", post.Date.Format("2006-01-02"))
	fmt.Printf("Taxonomies:   %v\n", post.Taxonomies)
	fmt.Printf("WordCount:    %d\n", post.WordCount)
	fmt.Printf("Reading Time:  %v min\n", post.ReadingTime)
	fmt.Printf("Is Draft:      %v\n", post.IsDraft)
	fmt.Printf("Is Pinned:     %v\n", post.IsPinned)

	if len(post.SSRInputHashes) > 0 {
		fmt.Printf("SSR Hashes:   %d artifacts\n", len(post.SSRInputHashes))
	}
}

func truncateHash(hash string) string {
	if len(hash) > hashPreviewLen {
		return hash[:hashEdgeLen] + "..." + hash[len(hash)-hashEdgeLen:]
	}
	return hash
}
