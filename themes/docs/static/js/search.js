(function () {
    'use strict';

    function init() {
        const searchBtn = document.getElementById('search-btn');
        const searchModal = document.getElementById('search-modal');
        const closeSearch = document.querySelector('.close-search');
        const searchInput = document.getElementById('search-input');
        const searchResults = document.getElementById('search-results');
        const searchAllVersions = document.getElementById('search-all-versions');

        if (!searchBtn || !searchModal) return;

        let wasmLoaded = false;
        let wasmPromise = null;
        let selectedIndex = -1;
        const baseURL = window.siteBaseURL || '';

        // Safely extract current version from URL
        function getCurrentVersion() {
            const path = window.location.pathname.replace(baseURL, '');
            const parts = path.split('/').filter(p => p);
            if (parts.length > 0 && parts[0].match(/^v\d+/)) {
                return parts[0];
            }
            return ""; // latest
        }

        const joinPath = (base, path) => {
            if (!base) return path;
            const cleanBase = base.endsWith('/') ? base.slice(0, -1) : base;
            const cleanPath = path.startsWith('/') ? path.slice(1) : path;
            return cleanBase + '/' + cleanPath;
        };

        async function loadWasm() {
            if (wasmLoaded) return;
            if (wasmPromise) return wasmPromise;

            wasmPromise = (async () => {
                try {
                    if (typeof Go === 'undefined') {
                        throw new Error("Go is undefined. wasm_exec.js missing?");
                    }

                    const go = new Go();
                    const wasmPath = joinPath(baseURL, '/static/wasm/search.wasm');
                    const response = await fetch(wasmPath);
                    if (!response.ok) throw new Error(`Fetch failed: ${response.status}`);

                    const result = await WebAssembly.instantiateStreaming(response, go.importObject);
                    go.run(result.instance);

                    const binPath = joinPath(baseURL, '/search.bin');
                    await window.initSearch(binPath);

                    wasmLoaded = true;
                } catch (err) {
                    console.error("Search initialization failed:", err);
                    if (searchResults) {
                        searchResults.innerHTML = `<div style="padding:1rem;color:var(--primary-color)">Error loading search engine.</div>`;
                    }
                }
            })();
            return wasmPromise;
        }

        function openModal() {
            searchModal.style.display = 'block';
            document.body.style.overflow = 'hidden';
            if (searchInput) searchInput.focus();
            loadWasm();
        }

        function closeModal() {
            searchModal.style.display = 'none';
            document.body.style.overflow = '';
            selectedIndex = -1;
        }

        function updateSelection(items) {
            items.forEach((item, i) => {
                item.classList.toggle('selected', i === selectedIndex);
                if (i === selectedIndex) item.scrollIntoView({ block: 'nearest' });
            });
        }

        function performSearch() {
            if (!wasmLoaded || !searchInput) return;
            const query = searchInput.value.trim();
            if (!query) {
                searchResults.innerHTML = '';
                return;
            }

            const versionFilter = searchAllVersions && searchAllVersions.checked ? "all" : getCurrentVersion();
            
            try {
                const results = window.searchPosts(query, versionFilter);
                renderResults(results);
            } catch (err) {
                console.error("Search execution failed:", err);
            }
        }

        function renderResults(results) {
            if (!searchResults) return;
            searchResults.innerHTML = '';
            selectedIndex = -1;

            if (!results || results.length === 0) {
                searchResults.innerHTML = '<div style="padding:2rem;text-align:center;color:var(--text-muted)">No results found.</div>';
                return;
            }

            const fragment = document.createDocumentFragment();
            results.forEach((res, i) => {
                const item = document.createElement('a');
                // Construct full link
                const link = res.link.startsWith('http') ? res.link : joinPath(baseURL, res.link);
                item.href = link;
                item.className = 'search-result-item';
                
                const versionTag = res.version ? `<span class="search-result-version">${res.version}</span>` : '';
                
                item.innerHTML = `
                    <div class="search-result-header">
                        <div class="search-result-title">${res.title}</div>
                        ${versionTag}
                    </div>
                    <div class="search-result-snippet">${res.snippet}</div>
                `;
                
                fragment.appendChild(item);
            });
            searchResults.appendChild(fragment);
        }

        // Event Listeners
        searchBtn.addEventListener('click', (e) => { e.preventDefault(); openModal(); });
        if (closeSearch) closeSearch.addEventListener('click', closeModal);
        window.addEventListener('click', (e) => { if (e.target === searchModal) closeModal(); });

        let debounceTimer;
        searchInput.addEventListener('input', () => {
            clearTimeout(debounceTimer);
            debounceTimer = setTimeout(performSearch, 150);
        });

        if (searchAllVersions) {
            searchAllVersions.addEventListener('change', performSearch);
        }

        window.addEventListener('keydown', (e) => {
            const isModalOpen = searchModal.style.display === 'block';

            if (!isModalOpen && (e.key === '/' || (e.ctrlKey && e.key === 'k'))) {
                if (['INPUT', 'TEXTAREA'].includes(document.activeElement.tagName) || document.activeElement.isContentEditable) return;
                e.preventDefault();
                openModal();
            } else if (isModalOpen) {
                if (e.key === 'Escape') {
                    closeModal();
                } else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                    e.preventDefault();
                    const items = searchResults.querySelectorAll('.search-result-item');
                    if (items.length === 0) return;
                    
                    if (e.key === 'ArrowDown') {
                        selectedIndex = (selectedIndex + 1) % items.length;
                    } else {
                        selectedIndex = (selectedIndex - 1 + items.length) % items.length;
                    }
                    updateSelection(items);
                } else if (e.key === 'Enter' && selectedIndex >= 0) {
                    const items = searchResults.querySelectorAll('.search-result-item');
                    if (items[selectedIndex]) {
                        e.preventDefault();
                        items[selectedIndex].click();
                    }
                }
            }
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
