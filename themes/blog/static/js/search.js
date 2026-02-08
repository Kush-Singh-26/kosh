(function() {
    let wasmLoaded = false;
    let wasmPromise = null;

    // Elements
    const searchBtn = document.getElementById('search-btn');
    const heroSearchTrigger = document.getElementById('hero-search-trigger');
    const searchModal = document.getElementById('search-modal');
    const closeSearch = document.querySelector('.close-search');
    const searchInput = document.getElementById('search-input');
    const searchResults = document.getElementById('search-results');
    
    let selectedIndex = -1;

    // Ensure siteBaseURL is set
    const baseURL = window.siteBaseURL || '';

    // Construct paths safely (avoiding double slashes except after protocol)
    // Optimized to reduce intermediate string allocations
    const joinPath = (base, path) => {
        if (!base) return path;
        const needsSlash = !base.endsWith('/') && !path.startsWith('/');
        const hasDoubleSlash = base.endsWith('/') && path.startsWith('/');
        
        if (hasDoubleSlash) return base + path.slice(1);
        if (needsSlash) return base + '/' + path;
        return base + path;
    };

    // Load WASM when needed
    async function loadWasm() {
        if (wasmLoaded) return;
        if (wasmPromise) return wasmPromise;

        console.log("Initializing Search WASM...");
        wasmPromise = (async () => {
            try {
                // 1. Load wasm_exec.js if Go is not defined
                if (typeof Go === 'undefined') {
                    console.log("Loading wasm_exec.js dynamically...");
                    const script = document.createElement('script');
                    script.src = joinPath(baseURL, '/static/js/wasm_exec.js');
                    console.log("Loading wasm_exec.js from:", script.src);
                    const scriptPromise = new Promise((resolve, reject) => {
                        script.onload = resolve;
                        script.onerror = (e) => reject(new Error(`Failed to load wasm_exec.js: ${e}`));
                    });
                    document.head.appendChild(script);
                    await scriptPromise;
                    console.log("wasm_exec.js loaded successfully");
                }

                if (typeof Go === 'undefined') {
                    throw new Error("wasm_exec.js failed to load - Go is still undefined");
                }
                console.log("Creating Go instance...");
                const go = new Go();
                
                const wasmPath = joinPath(baseURL, '/static/wasm/search.wasm');
                console.log("Fetching WASM from:", wasmPath);
                const response = await fetch(wasmPath);
                if (!response.ok) throw new Error(`Failed to fetch WASM: ${response.status} ${response.statusText}`);
                console.log("WASM fetched successfully");
                
                console.log("Instantiating WASM...");
                const result = await WebAssembly.instantiateStreaming(response, go.importObject);
                console.log("WASM instantiated, running Go...");
                go.run(result.instance);
                console.log("Go runtime started");
                
                // Check if initSearch is available
                if (typeof window.initSearch !== 'function') {
                    throw new Error("window.initSearch is not available after WASM load");
                }
                
                // Initialize with the data index
                const binPath = joinPath(baseURL, '/search.bin');
                console.log("Initializing search with index from:", binPath);
                
                try {
                    const initResult = await window.initSearch(binPath);
                    console.log("Search initialized successfully with", initResult, "posts");
                    wasmLoaded = true;
                    console.log("Search WASM Loaded and Initialized");
                } catch (initErr) {
                    console.error("initSearch failed:", initErr);
                    throw new Error(`Failed to initialize search index: ${initErr}`);
                }
            } catch (err) {
                console.error("Search initialization failed:", err);
                if (searchResults) {
                    const errorMessage = err && err.message ? err.message : String(err);
                    searchResults.innerHTML = `<div style="padding: 2rem; color: #f85149;">Search initialization failed: ${errorMessage}</div>`;
                }
                throw err;
            }
        })();

        return wasmPromise;
    }

    function openModal() {
        if (!searchModal) return;
        searchModal.style.display = 'block';
        document.body.style.overflow = 'hidden'; // Prevent scrolling
        
        loadWasm().then(() => {
            if (searchInput) searchInput.focus();
        }).catch(() => {});
    }

    function closeModal() {
        if (!searchModal) return;
        searchModal.style.display = 'none';
        document.body.style.overflow = '';
        if (searchInput) searchInput.value = '';
        if (searchResults) searchResults.innerHTML = '';
        selectedIndex = -1;
    }

    // Event Listeners
    if (searchBtn) searchBtn.addEventListener('click', openModal);
    if (heroSearchTrigger) heroSearchTrigger.addEventListener('click', openModal);
    if (closeSearch) closeSearch.addEventListener('click', closeModal);

    window.addEventListener('click', (e) => {
        if (e.target == searchModal) closeModal();
    });

    window.addEventListener('keydown', (e) => {
        // Open with / or Ctrl+K
        const isSearchOpen = searchModal && searchModal.style.display === 'block';
        
        if (!isSearchOpen && (e.key === '/' || (e.ctrlKey && e.key === 'k'))) {
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) {
                return;
            }
            e.preventDefault();
            openModal();
        }
        
        if (isSearchOpen) {
            if (e.key === 'Escape') {
                closeModal();
            } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                const items = searchResults.querySelectorAll('.search-result-item');
                selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
                updateSelection(items);
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                const items = searchResults.querySelectorAll('.search-result-item');
                selectedIndex = Math.max(selectedIndex - 1, 0);
                updateSelection(items);
            } else if (e.key === 'Enter' && selectedIndex >= 0) {
                e.preventDefault();
                const items = searchResults.querySelectorAll('.search-result-item');
                if (items[selectedIndex]) items[selectedIndex].click();
            }
        }
    });

    function updateSelection(items) {
        items.forEach((item, i) => {
            if (i === selectedIndex) {
                item.classList.add('selected');
                item.scrollIntoView({ block: 'nearest' });
            } else {
                item.classList.remove('selected');
            }
        });
    }

    let debounceTimer;
    if (searchInput) {
        searchInput.addEventListener('input', (e) => {
            clearTimeout(debounceTimer);
            const query = e.target.value;
            debounceTimer = setTimeout(() => {
                performSearch(query);
            }, 100);
        });
    }

    function performSearch(query) {
        if (!wasmLoaded || !query || !query.trim()) {
            if (searchResults) searchResults.innerHTML = '';
            return;
        }

        try {
            const results = window.searchPosts(query);
            renderResults(results);
        } catch (err) {
            console.error("Search failed:", err);
        }
    }

    function renderResults(results) {
        if (!searchResults) return;
        searchResults.innerHTML = '';
        selectedIndex = -1;

        if (!results || results.length === 0) {
            searchResults.innerHTML = '<div style="padding: 2rem; text-align: center; color: var(--text-muted);">No results found.</div>';
            return;
        }

        const fragment = document.createDocumentFragment();
        results.forEach(res => {
            const item = document.createElement('a');
            const link = joinPath(baseURL, res.link);
            item.href = link;
            item.className = 'search-result-item';
            item.innerHTML = `
                <div class="search-result-title">${res.title}</div>
                <div class="search-result-snippet">${res.snippet}</div>
            `;
            fragment.appendChild(item);
        });
        searchResults.appendChild(fragment);
    }
})();
