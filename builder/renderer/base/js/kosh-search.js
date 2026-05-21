(function() {
    let wasmLoaded = false;
    let wasmPromise = null;
    let wasmLoadedVersion = null;

    // Elements
    const searchBtn = document.getElementById('search-btn');
    const heroSearchTrigger = document.getElementById('hero-search-trigger');
    const searchModal = document.getElementById('search-modal');
    const closeSearch = document.querySelector('.close-search');
    const searchInput = document.getElementById('search-input');
    const searchResults = document.getElementById('search-results');
    const searchSuggestions = document.getElementById('search-suggestions');
    
    let selectedIndex = -1;
    let selectedSuggestionIndex = -1;
    let suggestionsActive = false;
    let currentSuggestions = [];

    // Ensure siteBaseURL is set
    const baseURL = window.siteBaseURL || '';
    const versionQuery = window.buildVersion ? `?v=${window.buildVersion}` : '';

    const graphEnabled = () => window.KOSH_GRAPH_ENABLED !== false;

    const joinPath = (base, path) => {
        if (!base) return path;
        const needsSlash = !base.endsWith('/') && !path.startsWith('/');
        const hasDoubleSlash = base.endsWith('/') && path.startsWith('/');
        if (hasDoubleSlash) return base + path.slice(1);
        if (needsSlash) return base + '/' + path;
        return base + path;
    };

    const slugifyTopic = (value) => {
        return String(value || '')
            .normalize('NFKD')
            .replace(/[^\w\s-]/g, '')
            .trim()
            .replace(/[\s_-]+/g, '-')
            .replace(/^-+|-+$/g, '')
            .toLowerCase();
    };

    async function loadWasm() {
        const currentVersion = window.buildVersion || '';
        if (wasmLoaded && wasmLoadedVersion === currentVersion) return;
        if (wasmLoaded) { wasmLoaded = false; wasmPromise = null; }
        if (wasmPromise) return wasmPromise;

        wasmPromise = (async () => {
            try {
                if (typeof Go === 'undefined') {
                    const script = document.createElement('script');
                    script.src = joinPath(baseURL, '/static/js/wasm_exec.js') + versionQuery;
                    await new Promise((resolve, reject) => {
                        script.onload = resolve;
                        script.onerror = reject;
                        document.head.appendChild(script);
                    });
                }
                const go = new Go();
                const wasmPath = joinPath(baseURL, '/static/wasm/search.wasm') + versionQuery;
                const response = await fetch(wasmPath);
                const result = await WebAssembly.instantiateStreaming(response, go.importObject);
                go.run(result.instance);

                const searchEndpoints = window.searchEndpoints;
                const endpoints = (Array.isArray(searchEndpoints) && searchEndpoints.length > 0) 
                    ? searchEndpoints 
                    : [joinPath(baseURL, '/search.bin')];
                
                await Promise.all(endpoints.map(async ep => {
                    const epUrl = ep + versionQuery;
                    try {
                        const response = await fetch(epUrl);
                        if (!response.ok) throw new Error(`HTTP ${response.status}`);
                        
                        const data = await response.arrayBuffer();
                        const uint8Array = new Uint8Array(data);
                        const result = window.initSearch(uint8Array);
                        if (typeof result === 'string' && result.startsWith('Error')) {
                            console.warn("Failed to load search index:", ep, result);
                        }
                    } catch (e) {
                        console.warn("Failed to load federated search:", ep, e);
                    }
                }));

                wasmLoaded = true;
                window.koshWasmLoaded = true;
                wasmLoadedVersion = currentVersion;
            } catch (err) {
                console.error("Search initialization failed:", err);
                if (searchResults) searchResults.innerHTML = `<div style="padding: 2rem; color: var(--color-error);">Search failed to load.</div>`;
            }
        })();
        return wasmPromise;
    }

    function renderDiscovery(noResults = false) {
        if (!searchResults) return;
        
        let tagsHTML = '';
        const taxonomies = window.siteTaxonomies || {};
        const blogTags = Array.isArray(window.blogTags) ? window.blogTags : [];
        const taxonomyBaseURL = window.taxonomyBaseURL || '';
        
        try {
            const taxonomyList = Object.values(taxonomies);
            if (taxonomyList.length > 0) {
                const targetTax = taxonomies["tags"] || taxonomyList.find(t => t.Terms && t.Terms.length > 0);
                if (targetTax && targetTax.Terms && targetTax.Terms.length > 0) {
                    const sorted = targetTax.Terms.slice().sort((a, b) => (b.Count || 0) - (a.Count || 0)).slice(0, 4);
                    tagsHTML = sorted.map(term => `<a href="${joinPath(baseURL, term.Link)}" class="discovery-tag">#${term.Name}</a>`).join('');
                }
            }

            if (!tagsHTML && blogTags.length > 0) {
                const sorted = blogTags.slice().sort((a, b) => (b.Count || 0) - (a.Count || 0)).slice(0, 4);
                const tagPrefix = taxonomyBaseURL ? `/${taxonomyBaseURL}/tags/` : '/blogs/tags/';
                tagsHTML = sorted.map(term => {
                    const slug = slugifyTopic(term.Name);
                    const link = `${tagPrefix}${slug}.html`;
                    return `<a href="${joinPath(baseURL, link)}" class="discovery-tag">#${term.Name}</a>`;
                }).join('');
            }

            if (!tagsHTML && wasmLoaded && typeof window.getFeaturedTopics === 'function') {
                const featuredTopics = window.getFeaturedTopics();
                if (Array.isArray(featuredTopics) && featuredTopics.length > 0) {
                    const tagPrefix = taxonomyBaseURL ? `/${taxonomyBaseURL}/tags/` : '/blogs/tags/';
                    tagsHTML = featuredTopics.slice(0, 4).map(term => {
                        const slug = slugifyTopic(term.name);
                        const link = `${tagPrefix}${slug}.html`;
                        return `<a href="${joinPath(baseURL, link)}" class="discovery-tag">#${term.name}</a>`;
                    }).join('');
                }
            }
        } catch (e) {
            console.error("Discovery rendering failed:", e);
        }

        if (!tagsHTML) {
            // Only if we truly have no data from the backend
            tagsHTML = '<div class="discovery-tag">No topics found</div>';
        }
        
        const noResultsHTML = noResults ? `<div class="search-no-results">No results found for "${searchInput.value}".</div>` : '';
        const allTopicsPath = taxonomyBaseURL ? '/' + taxonomyBaseURL + '/tags/' : '/blogs/tags/';

        searchResults.innerHTML = `
            <div class="search-empty-state">
                ${noResultsHTML}
                <div class="discovery-section">
                    <div class="discovery-title">Featured Topics</div>
                    <div class="discovery-tags">${tagsHTML}</div>
                </div>
                <div class="discovery-section discovery-recent">
                    <div class="discovery-title">Quick Actions</div>
                    ${graphEnabled() ? '<a href="' + joinPath(baseURL, '/graph.html') + '" class="recent-search-item">' +
                        '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"></circle><path d="M12 2v3m0 14v3M2 12h3m14 0h3M4.9 4.9l2.1 2.1m10 10l2.1 2.1M4.9 19.1l2.1-2.1m10-10l2.1-2.1"></path></svg>' +
                        'Explore Knowledge Graph' +
                    '</a>' : ''}
                    <a href="${joinPath(baseURL, allTopicsPath)}" class="recent-search-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"></path><line x1="7" y1="7" x2="7.01" y2="7"></line></svg>
                        Browse All Topics
                    </a>
                </div>
            </div>
        `;
    }

    function openModal() {
        if (!searchModal) return;
        searchModal.style.display = 'flex';
        document.body.style.overflow = 'hidden';
        renderDiscovery();
        loadWasm().then(() => {
            if (searchInput) {
                searchInput.value = '';
                searchInput.focus();
            }
            if (searchResults && (!searchInput || !searchInput.value)) {
                renderDiscovery();
            }
        }).catch(() => {});
    }

    function closeModal() {
        if (!searchModal) return;
        searchModal.style.display = 'none';
        document.body.style.overflow = '';
        if (searchInput) searchInput.value = '';
        if (searchResults) searchResults.innerHTML = '';
        if (searchSuggestions) { searchSuggestions.innerHTML = ''; searchSuggestions.style.display = 'none'; }
        selectedIndex = -1;
        selectedSuggestionIndex = -1;
        suggestionsActive = false;
    }

    if (searchBtn) searchBtn.addEventListener('click', openModal);
    if (heroSearchTrigger) heroSearchTrigger.addEventListener('click', openModal);
    if (closeSearch) closeSearch.addEventListener('click', closeModal);
    window.addEventListener('click', (e) => { if (e.target == searchModal) closeModal(); });

    window.addEventListener('keydown', (e) => {
        const isSearchOpen = searchModal && searchModal.style.display === 'flex';
        if (!isSearchOpen && (e.key === '/' || ((e.ctrlKey || e.metaKey) && e.key === 'k'))) {
            if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) return;
            e.preventDefault();
            openModal();
            return;
        }
        if (isSearchOpen) {
            if (e.key === 'Escape') { closeModal(); return; }

            const items = searchResults ? searchResults.querySelectorAll('.search-result-item') : [];
            const sugItems = searchSuggestions ? searchSuggestions.querySelectorAll('.suggestion-item') : [];

            if (e.key === 'ArrowDown') {
                e.preventDefault();
                if (suggestionsActive && sugItems.length > 0) {
                    if (selectedSuggestionIndex < sugItems.length - 1) {
                        selectedSuggestionIndex = Math.min(selectedSuggestionIndex + 1, sugItems.length - 1);
                        updateSuggestionSelection(sugItems);
                    } else if (items.length > 0) {
                        suggestionsActive = false;
                        selectedSuggestionIndex = -1;
                        selectedIndex = 0;
                        updateSuggestionSelection(sugItems);
                        updateSelection(items);
                    }
                } else if (items.length > 0) {
                    selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
                    updateSelection(items);
                }
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                if (suggestionsActive && sugItems.length > 0) {
                    if (selectedSuggestionIndex > 0) {
                        selectedSuggestionIndex = Math.max(selectedSuggestionIndex - 1, 0);
                        updateSuggestionSelection(sugItems);
                    } else if (items.length > 0) {
                        suggestionsActive = false;
                        selectedSuggestionIndex = -1;
                        selectedIndex = items.length - 1;
                        updateSuggestionSelection(sugItems);
                        updateSelection(items);
                    }
                } else if (items.length > 0) {
                    if (selectedIndex > 0) {
                        selectedIndex = Math.max(selectedIndex - 1, 0);
                        updateSelection(items);
                    } else if (sugItems.length > 0) {
                        suggestionsActive = true;
                        selectedSuggestionIndex = sugItems.length - 1;
                        selectedIndex = -1;
                        updateSuggestionSelection(sugItems);
                        updateSelection(items);
                    }
                }
            } else if (e.key === 'Enter') {
                if (suggestionsActive && selectedSuggestionIndex >= 0 && sugItems[selectedSuggestionIndex]) {
                    e.preventDefault();
                    sugItems[selectedSuggestionIndex].click();
                } else if (selectedIndex >= 0 && items[selectedIndex]) {
                    e.preventDefault();
                    items[selectedIndex].click();
                }
            } else if (e.key === 'Tab') {
                if (suggestionsActive && sugItems.length > 0) {
                    e.preventDefault();
                    selectedSuggestionIndex = (selectedSuggestionIndex + 1) % sugItems.length;
                    updateSuggestionSelection(sugItems);
                }
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

    function updateSuggestionSelection(items) {
        items.forEach((item, i) => {
            if (i === selectedSuggestionIndex) { 
                item.classList.add('selected'); 
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
            
            // Real-time suggestions
            if (wasmLoaded && query.trim().length > 1) {
                fetchSuggestions(query);
            } else {
                if (searchSuggestions) {
                    searchSuggestions.style.display = 'none';
                    searchSuggestions.innerHTML = '';
                    suggestionsActive = false;
                }
            }

            debounceTimer = setTimeout(() => { performSearch(query); }, 100);
        });
    }

    function fetchSuggestions(query) {
        try {
            const suggestions = window.getSuggestions(query);
            if (suggestions && suggestions.length > 0) {
                renderSuggestions(suggestions);
            } else {
                if (searchSuggestions) {
                    searchSuggestions.style.display = 'none';
                    suggestionsActive = false;
                }
            }
        } catch (e) {
            console.error("Suggestion fetch failed:", e);
        }
    }

    function renderSuggestions(suggestions) {
        if (!searchSuggestions) return;
        searchSuggestions.innerHTML = '';
        selectedSuggestionIndex = -1;
        suggestionsActive = true;
        
        suggestions.slice(0, 5).forEach(s => {
            const div = document.createElement('div');
            div.className = 'suggestion-item';
            div.textContent = s;
            div.onclick = () => {
                searchInput.value = s;
                performSearch(s);
                searchSuggestions.style.display = 'none';
                suggestionsActive = false;
            };
            searchSuggestions.appendChild(div);
        });
        searchSuggestions.style.display = 'flex';
    }

    function performSearch(query) {
        if (!wasmLoaded || !query || !query.trim()) { renderDiscovery(); return; }
        try {
            const results = window.searchItems(query, "all");
            if (!results || results.length === 0) {
                renderDiscovery(true);
            } else {
                renderResults(results);
            }
        } catch (err) { console.error("Search failed:", err); }
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
        results.slice(0, 20).forEach(res => {
            const item = document.createElement('a');
            // res.link is already absolute (e.g., /blogs/maths-matrices.html), use it directly
            item.href = res.link;
            item.className = 'search-result-item';
            item.innerHTML = `
                <div class="search-result-title">${res.title}</div>
                <div class="search-result-snippet">${res.snippet}</div>
            `;
            if (graphEnabled()) {
                const gBtn = document.createElement('div');
                gBtn.className = 'search-graph-btn';
                gBtn.textContent = 'Explore in Graph';
                gBtn.onclick = (e) => {
                    e.preventDefault(); e.stopPropagation();
                    // res.link is already absolute
                    const absLink = res.link;
                    if (document.getElementById('graph-canvas')) {
                        window.dispatchEvent(new CustomEvent('kosh:graph-focus', { detail: { link: absLink } }));
                    } else {
                        window.location.href = `${joinPath(baseURL, '/graph.html')}?focus=${encodeURIComponent(absLink)}`;
                    }
                    closeModal();
                };
                item.appendChild(gBtn);
            }
            fragment.appendChild(item);
        });
        searchResults.appendChild(fragment);
    }
})();
