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

    // Load WASM when needed
    async function loadWasm() {
        if (wasmLoaded) return;
        if (wasmPromise) return wasmPromise;

        console.log("Initializing Search WASM...");
        wasmPromise = (async () => {
            try {
                if (typeof Go === 'undefined') {
                    throw new Error("wasm_exec.js not loaded");
                }
                const go = new Go();
                
                // Construct paths safely (avoiding double slashes except after protocol)
                const joinPath = (base, path) => {
                    const cleanBase = base.endsWith('/') ? base.slice(0, -1) : base;
                    const cleanPath = path.startsWith('/') ? path.slice(1) : path;
                    return cleanBase + '/' + cleanPath;
                };

                const wasmPath = joinPath(baseURL, '/static/wasm/search.wasm');
                const response = await fetch(wasmPath);
                if (!response.ok) throw new Error(`Failed to fetch WASM: ${response.statusText}`);
                
                const result = await WebAssembly.instantiateStreaming(response, go.importObject);
                go.run(result.instance);
                
                // Initialize with the data index
                const binPath = joinPath(baseURL, '/search.bin');
                await window.initSearch(binPath);
                wasmLoaded = true;
                console.log("Search WASM Loaded and Initialized");
            } catch (err) {
                console.error("Search initialization failed:", err);
                if (searchResults) {
                    searchResults.innerHTML = `<div style="padding: 2rem; color: #f85149;">Search initialization failed: ${err.message}</div>`;
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

        const joinPath = (base, path) => {
            const cleanBase = base.endsWith('/') ? base.slice(0, -1) : base;
            const cleanPath = path.startsWith('/') ? path.slice(1) : path;
            return cleanBase + '/' + cleanPath;
        };

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
