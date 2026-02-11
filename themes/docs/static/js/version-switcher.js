// Version Switcher - Client-side content swapping
(function () {
    'use strict';

    // Mark that version switcher is enabled (prevents fallback inline handler)
    window.versionSwitcherEnabled = true;

    // Store current state
    let isLoading = false;

    // Cache for fetched content
    const contentCache = new Map();

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        const versionSelector = document.getElementById('version-selector');
        if (!versionSelector) return;

        // Intercept version change (use capture to run before inline handler)
        versionSelector.addEventListener('change', handleVersionChange, true);

        // Handle browser back/forward buttons
        window.addEventListener('popstate', handlePopState);
    }

    async function handleVersionChange(e) {
        if (isLoading) return;

        const targetUrl = e.target.value;
        const currentUrl = window.location.href;

        // Don't reload if already on this page
        if (targetUrl === currentUrl) return;

        // Prevent the inline onchange handler from firing
        e.preventDefault();
        e.stopPropagation();
        e.stopImmediatePropagation();

        await switchVersion(targetUrl);
    }

    async function switchVersion(url) {
        if (isLoading) return;
        isLoading = true;

        const versionSelector = document.getElementById('version-selector');
        const mainContent = document.querySelector('.docs-main');
        const tocSidebar = document.querySelector('.docs-toc');

        // Show loading state
        showLoading(mainContent);

        try {
            // Check cache first
            let html;
            if (contentCache.has(url)) {
                html = contentCache.get(url);
            } else {
                // Fetch the page
                const response = await fetch(url, {
                    headers: { 'Accept': 'text/html' }
                });

                if (!response.ok) {
                    if (response.status === 404) {
                        // Page doesn't exist in this version, redirect to version home page
                        const homeUrl = getVersionHomeUrl(url);
                        console.log(`Page not found in this version, redirecting to: ${homeUrl}`);
                        window.location.href = homeUrl;
                        return;
                    }
                    throw new Error(`Failed to load: ${response.status}`);
                }

                html = await response.text();

                // Cache the response (limit cache size)
                if (contentCache.size < 10) {
                    contentCache.set(url, html);
                }
            }

            // Parse the HTML
            const parser = new DOMParser();
            const doc = parser.parseFromString(html, 'text/html');

            // Extract new content
            const newMainContent = doc.querySelector('.docs-main');
            const newTocSidebar = doc.querySelector('.docs-toc');
            const newSidebar = doc.querySelector('.docs-sidebar');
            const newVersionBanner = doc.querySelector('.version-banner');
            const newBreadcrumbs = doc.querySelector('.breadcrumbs');
            const newTitle = doc.querySelector('title');

            const currentSidebar = document.querySelector('.docs-sidebar');

            // Update content if found
            if (newMainContent && mainContent) {
                mainContent.innerHTML = newMainContent.innerHTML;
            }

            if (newTocSidebar && tocSidebar) {
                tocSidebar.innerHTML = newTocSidebar.innerHTML;
            }

            // Update sidebar (version-specific navigation tree)
            if (newSidebar && currentSidebar) {
                currentSidebar.innerHTML = newSidebar.innerHTML;
            }

            // Update version banner
            const currentBanner = document.querySelector('.version-banner');
            if (newVersionBanner) {
                if (currentBanner) {
                    currentBanner.outerHTML = newVersionBanner.outerHTML;
                } else {
                    // Insert after header if it doesn't exist
                    const header = document.querySelector('.docs-header');
                    if (header) {
                        header.insertAdjacentHTML('afterend', newVersionBanner.outerHTML);
                    }
                }
            } else if (currentBanner) {
                // Remove banner if switching to latest
                currentBanner.remove();
            }

            // Update breadcrumbs
            const currentBreadcrumbs = document.querySelector('.breadcrumbs');
            if (newBreadcrumbs) {
                if (currentBreadcrumbs) {
                    currentBreadcrumbs.outerHTML = newBreadcrumbs.outerHTML;
                } else {
                    const banner = document.querySelector('.version-banner') || document.querySelector('.docs-header');
                    if (banner) {
                        banner.insertAdjacentHTML('afterend', newBreadcrumbs.outerHTML);
                    }
                }
            } else if (currentBreadcrumbs) {
                currentBreadcrumbs.remove();
            }

            // Update page title
            if (newTitle) {
                document.title = newTitle.textContent;
            }

            // Update URL without reloading
            history.pushState({ url: url }, '', url);

            // Update version selector to match new page
            updateVersionSelector(doc);

            // Update sidebar active state
            updateSidebarActiveState(url);

            // Scroll to top
            window.scrollTo({ top: 0, behavior: 'smooth' });

        } catch (error) {
            console.error('Failed to switch version:', error);
            // Fall back to full page reload
            window.location.href = url;
        } finally {
            isLoading = false;
        }
    }

    function showLoading(element) {
        if (!element) return;

        const loadingHtml = `
            <div class="version-loading">
                <div class="version-loading-spinner"></div>
                <p>Loading version...</p>
            </div>
        `;

        // Save current content
        element.dataset.originalContent = element.innerHTML;
        element.innerHTML = loadingHtml;
    }

    function updateVersionSelector(doc) {
        const currentSelector = document.getElementById('version-selector');
        const newSelector = doc.getElementById('version-selector');

        if (currentSelector && newSelector) {
            // Update options without changing the select element itself
            // to preserve event listeners
            currentSelector.innerHTML = newSelector.innerHTML;

            // Restore the selected option
            const selectedOption = newSelector.querySelector('option[selected]');
            if (selectedOption) {
                const value = selectedOption.value;
                const option = currentSelector.querySelector(`option[value="${value}"]`);
                if (option) {
                    option.selected = true;
                }
            }
        }
    }

    function updateSidebarActiveState(url) {
        // Remove all active states
        document.querySelectorAll('.tree-link.active').forEach(link => {
            link.classList.remove('active');
        });

        // Find and activate current page link
        try {
            const currentPath = new URL(url, window.location.origin).pathname;
            document.querySelectorAll('.tree-link').forEach(link => {
                const linkPath = new URL(link.href).pathname;
                if (linkPath === currentPath) {
                    link.classList.add('active');
                }
            });
        } catch (e) {
            console.error("Error updating sidebar active state", e);
        }
    }

    function handlePopState(e) {
        if (e.state && e.state.url) {
            switchVersion(e.state.url);
        }
    }

    function getVersionHomeUrl(currentUrl) {
        // Try to extract version from the current URL
        try {
            const pathname = new URL(currentUrl).pathname;

            // Check if URL contains a version path like /v2.0/
            const versionMatch = pathname.match(/^\/v\d+\.\d+\//);
            if (versionMatch) {
                const versionPath = versionMatch[0];
                return window.siteBaseURL + versionPath + 'index.html';
            }

            // Check if URL starts with /vX.X (without trailing slash)
            const versionStartMatch = pathname.match(/^\/v\d+\.\d+/);
            if (versionStartMatch) {
                const versionPath = versionStartMatch[0];
                return window.siteBaseURL + versionPath + '/index.html';
            }
        } catch (e) {
            console.error("Error getting version home URL", e);
        }

        // Fallback to latest version home
        return window.siteBaseURL + '/index.html';
    }
})();
