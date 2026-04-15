package tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

func TestE2E_SearchAndGraph(t *testing.T) {
	if os.Getenv("KOSH_E2E") == "" {
		t.Skip("Skipping E2E test; set KOSH_E2E=1 to run")
	}

	// 1. Setup paths
	cwd, _ := os.Getwd()
	repoRoot := filepath.Dir(cwd)
	mockSiteDir := filepath.Join(repoRoot, "tests", "fixtures", "mock-site")

	// Chdir to mock site for the build
	if err := os.Chdir(mockSiteDir); err != nil {
		t.Fatalf("Failed to chdir to mock site: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	// 2. Setup server and config
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := ln.Addr().String()

	t.Log("Building mock site...")
	cfg := config.Load(nil)
	cfg.KoshSourceRoot = repoRoot
	cfg.BaseURL = fmt.Sprintf("http://%s", addr)
	// Fix paths for the test environment (ensure absolute paths)
	cfg.ThemeDir = filepath.Join(repoRoot, "themes")
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
	cfg.StaticDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "static")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.OutputDir = t.TempDir()
	cfg.CacheDir = t.TempDir()

	engine := orchestration.NewEngine(orchestration.WithConfig(cfg))
	if err := engine.Build(context.Background()); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	engine.Close()

	// 3. Start a simple file server
	server := &http.Server{
		Handler: http.FileServer(http.Dir(cfg.OutputDir)),
	}

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Errorf("Server failed: %v", err)
		}
	}()
	defer func() { _ = server.Close() }()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// 4. Browser automation with rod
	t.Log("Starting browser test...")

	// Use a launcher to ensure we can find chrome/chromium
	l := launcher.New().Leakless(false)
	u, err := l.Launch()
	if err != nil {
		t.Skipf("Failed to launch browser (rod): %v. Is Chrome/Chromium installed?", err)
	}

	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("")

	// Console log capture
	go page.EachEvent(func(e *proto.RuntimeConsoleAPICalled) {
		args := make([]string, len(e.Args))
		for i, arg := range e.Args {
			args[i] = fmt.Sprint(arg.Value)
		}
		t.Logf("[Browser Console] %s: %s", e.Type, strings.Join(args, " "))
	})()

	page.MustNavigate(fmt.Sprintf("http://%s/", addr))
	page.MustWaitLoad()

	// --- Test Search ---
	t.Log("Testing Search...")
	// Click search button
	var searchBtn *rod.Element
	err = rod.Try(func() {
		searchBtn = page.Timeout(5 * time.Second).MustElement("#search-btn")
	})
	if err != nil {
		t.Logf("Page source: %s", page.MustHTML())
		t.Fatalf("Could not find search button: %v", err)
	}
	searchBtn.MustClick()

	// Wait for modal and input
	var input *rod.Element
	err = rod.Try(func() {
		input = page.Timeout(5 * time.Second).MustElement("#search-input")
	})
	if err != nil {
		t.Fatalf("Could not find search input: %v", err)
	}
	input.MustWaitVisible()

	// Wait for WASM to be loaded (it might take a moment)
	t.Log("Waiting for WASM to load...")
	err = rod.Try(func() {
		for i := 0; i < 20; i++ {
			if page.MustEval("() => window.koshWasmLoaded").Bool() {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		panic("WASM load timeout")
	})
	if err != nil {
		t.Logf("WASM failed to load. Page source: %s", page.MustHTML())
		t.Fatalf("WASM load failed: %v", err)
	}

	// Type search query
	query := "first"
	input.MustInput(query)

	t.Log("Waiting for search results...")
	var resultsEl *rod.Element
	err = rod.Try(func() {
		resultsEl = page.Timeout(10 * time.Second).MustElement(".search-result-item")
	})

	if err != nil {
		t.Logf("Search failed. Page source: %s", page.MustHTML())
		t.Fatalf("Search failed: %v", err)
	}

	resultText := resultsEl.MustText()
	if !strings.Contains(strings.ToLower(resultText), "first") {
		t.Errorf("Search results missing expected content. Got: %s", resultText)
	}

	// --- Test "Explore in Graph" button ---
	t.Log("Testing 'Explore in Graph' button...")
	graphBtn := page.MustElement(".search-graph-btn")
	graphBtn.MustClick()

	// Wait for navigation to graph.html with focus parameter
	err = rod.Try(func() {
		page.Timeout(5 * time.Second).MustWaitNavigation()
	})
	if err != nil {
		t.Fatalf("Navigation to graph.html failed: %v", err)
	}

	currentURL := page.MustInfo().URL
	if !strings.Contains(currentURL, "/graph.html") || !strings.Contains(currentURL, "focus=") {
		t.Errorf("Expected URL to contain '/graph.html' and 'focus=', got: %s", currentURL)
	}

	// Verify graph is loaded (with retry loop for async fetch)
	canvas := page.MustElement("#graph-canvas")
	canvas.MustWaitVisible()

	err = rod.Try(func() {
		for i := 0; i < 20; i++ {
			if page.MustEval("() => window.graphData && window.graphData.nodes && window.graphData.nodes.length > 0").Bool() {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		panic("Graph data load timeout")
	})

	if err != nil {
		t.Errorf("Knowledge graph data not loaded or empty: %v", err)
	}

	t.Log("E2E tests passed!")
}
