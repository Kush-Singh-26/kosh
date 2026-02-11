package main

import (
	"fmt"
	"os"
	"time"

	"my-ssg/builder/cache"
)

// handleCacheCommand processes cache-related subcommands
func handleCacheCommand(args []string) {
	if len(args) < 1 {
		printCacheUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "stats":
		cacheStats()
	case "gc":
		dryRun := false
		for _, arg := range subArgs {
			if arg == "--dry-run" || arg == "-n" {
				dryRun = true
			}
		}
		cacheGC(dryRun)
	case "verify":
		cacheVerify()
	case "rebuild":
		cacheRebuild()
	case "clear":
		cacheClear()
	case "inspect":
		if len(subArgs) < 1 {
			fmt.Println("Usage: kosh cache inspect <path>")
			os.Exit(1)
		}
		cacheInspect(subArgs[0])
	default:
		fmt.Printf("Unknown cache subcommand: %s\n", subcommand)
		printCacheUsage()
		os.Exit(1)
	}
}

func printCacheUsage() {
	fmt.Println("Usage: kosh cache <subcommand> [arguments]")
	fmt.Println("\nSubcommands:")
	fmt.Println("  stats          Show cache statistics")
	fmt.Println("  gc             Run garbage collection")
	fmt.Println("  verify         Check cache integrity")
	fmt.Println("  rebuild        Force full cache rebuild")
	fmt.Println("  clear          Delete all cache data")
	fmt.Println("  inspect <path> Show cache entry for a specific file")
	fmt.Println("\nFlags for gc:")
	fmt.Println("  --dry-run, -n  Show what would be deleted without deleting")
}

func openCache() *cache.Manager {
	// Cache commands run in production mode for durability
	cm, err := cache.Open(".kosh-cache", false)
	if err != nil {
		fmt.Printf("❌ Failed to open cache: %v\n", err)
		os.Exit(1)
	}
	return cm
}

func cacheStats() {
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
	fmt.Printf("Store Size:      %.2f MB\n", float64(stats.StoreBytes)/(1024*1024))
	fmt.Printf("Build Count:     %d\n", stats.BuildCount)

	if stats.LastGC > 0 {
		fmt.Printf("Last GC:         %s\n", time.Unix(stats.LastGC, 0).Format(time.RFC3339))
	} else {
		fmt.Printf("Last GC:         never\n")
	}

	// Performance metrics
	fmt.Println("\n⚡ Performance Metrics")
	fmt.Println("────────────────────────────────────────")
	fmt.Printf("Last Read Time:  %v (target: <50ms)\n", stats.LastReadTime)
	fmt.Printf("Last Write Time: %v (target: <100ms)\n", stats.LastWriteTime)
	fmt.Printf("Read Operations: %d\n", stats.ReadCount)
	fmt.Printf("Write Operations: %d\n", stats.WriteCount)
	fmt.Printf("Inline Posts:    %d (%.1f%%)\n", stats.InlinePosts, float64(stats.InlinePosts)*100/float64(stats.TotalPosts))
	fmt.Printf("Hashed Posts:    %d (%.1f%%)\n", stats.HashedPosts, float64(stats.HashedPosts)*100/float64(stats.TotalPosts))
}

func cacheGC(dryRun bool) {
	cm := openCache()
	defer func() { _ = cm.Close() }()

	cfg := cache.DefaultGCConfig()
	cfg.DryRun = dryRun
	cfg.MinBuildsBetweenGC = 0 // Always run when manually invoked

	if dryRun {
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
	fmt.Printf("Deleted:    %d blobs (%.2f MB)\n", result.DeletedBlobs, float64(result.DeletedBytes)/(1024*1024))
	fmt.Printf("Duration:   %v\n", result.Duration)

	if dryRun {
		fmt.Println("\n(No changes made - dry run mode)")
	} else {
		fmt.Println("\n✅ GC complete")
	}
}

func cacheVerify() {
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

func cacheRebuild() {
	cm := openCache()

	fmt.Println("🔄 Clearing cache for rebuild...")

	if err := cm.Rebuild(); err != nil {
		fmt.Printf("❌ Failed to clear cache: %v\n", err)
		os.Exit(1)
	}
	_ = cm.Close()

	fmt.Println("✅ Cache cleared. Run 'kosh build' to rebuild.")
}

func cacheClear() {
	cm := openCache()

	fmt.Println("🗑️  Clearing all cache data...")

	if err := cm.Clear(); err != nil {
		fmt.Printf("❌ Failed to clear cache: %v\n", err)
		os.Exit(1)
	}
	_ = cm.Close()

	fmt.Println("✅ Cache cleared")
}

func cacheInspect(path string) {
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
	fmt.Printf("Tags:         %v\n", post.Tags)
	fmt.Printf("WordCount:    %d\n", post.WordCount)
	fmt.Printf("ReadingTime:  %d min\n", post.ReadingTime)
	fmt.Printf("Draft:        %v\n", post.Draft)
	fmt.Printf("Pinned:       %v\n", post.Pinned)
	fmt.Printf("Version:      %s\n", post.Version)

	if len(post.SSRInputHashes) > 0 {
		fmt.Printf("SSR Hashes:   %d artifacts\n", len(post.SSRInputHashes))
	}
}

func truncateHash(hash string) string {
	if len(hash) > 16 {
		return hash[:8] + "..." + hash[len(hash)-8:]
	}
	return hash
}
