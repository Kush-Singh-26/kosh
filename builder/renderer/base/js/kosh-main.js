(function () {
    function init() {
        // 1. Image Path Fix for GitHub Pages (or sub-directory hosting)
        if (typeof siteBaseURL !== 'undefined' && siteBaseURL && siteBaseURL !== "") {
            document.querySelectorAll('img').forEach(img => {
                const src = img.getAttribute('src');
                if (src && src.startsWith('/') && !src.startsWith(siteBaseURL)) {
                    img.src = siteBaseURL + src;
                }
            });
        }

        // 2. Code Block Setup
        // Handled by SSG
        
        // 3. Scroll UI Elements (Integrated Progress & Back to Top)
        let backToTopBtn = document.getElementById('back-to-top');

        const floatingTocBtn = document.getElementById('floating-toc-btn');
        const tocModal = document.getElementById('toc-modal');
        const tocModalClose = document.getElementById('toc-modal-close');

        // Consolidated Scroll Listener with requestAnimationFrame for Performance
        if (!window.__scrollInitialized) {
            window.__scrollInitialized = true;
            let ticking = false;
            const header = document.querySelector('header');
            
            window.addEventListener('scroll', () => {
                if (!ticking) {
                    window.requestAnimationFrame(() => {
                        const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
                        const scrollHeight = document.documentElement.scrollHeight - document.documentElement.clientHeight;
                        
                        // Update Integrated Reading Progress Indicator
                        if (scrollHeight > 0) {
                            const progress = (scrollTop / scrollHeight) * 100;
                            if (header) {
                                 header.style.setProperty('--scroll-percent', progress + "%");
                            }
                        }
                        
                        // Update Back to Top Visibility
                        if (backToTopBtn) {
                            if (scrollTop > 300) {
                                backToTopBtn.classList.add('visible');
                            } else {
                                backToTopBtn.classList.remove('visible');
                            }
                        }

                        // Update Floating TOC Visibility
                        if (floatingTocBtn) {
                            if (scrollTop > 500) {
                                floatingTocBtn.classList.add('visible');
                            } else {
                                floatingTocBtn.classList.remove('visible');
                            }
                        }
                        
                        ticking = false;
                    });
                    ticking = true;
                }
            }, { passive: true });
        }

        // 4. Floating TOC Modal Logic
        function closeTocModal() {
            if (tocModal) {
                tocModal.classList.remove('active');
                setTimeout(() => {
                    if (!tocModal.classList.contains('active')) {
                        tocModal.style.display = 'none';
                    }
                }, 300);
            }
            document.body.style.overflow = '';
        }

        function openTocModal() {
            if (tocModal) {
                tocModal.style.display = 'flex';
                // Trigger reflow for transition
                tocModal.offsetHeight;
                tocModal.classList.add('active');
            }
            document.body.style.overflow = 'hidden';
        }

        if (floatingTocBtn) {
            floatingTocBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                openTocModal();
            });
        }

        if (tocModalClose) tocModalClose.addEventListener('click', closeTocModal);
        if (tocModal) {
            tocModal.addEventListener('click', (e) => {
                if (e.target === tocModal) closeTocModal();
            });
        }

        // 4. Theme Toggle Logic
        const toggleBtn = document.getElementById('theme-toggle');
        const mobileToggleBtn = document.getElementById('theme-toggle-mobile');
        const htmlEl = document.documentElement;

        function toggleTheme() {
            const isDark = htmlEl.getAttribute('data-theme') === 'dark';
            if (isDark) {
                htmlEl.setAttribute('data-theme', 'light');
                localStorage.setItem('theme', 'light');
                window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme: 'light' } }));
            } else {
                htmlEl.setAttribute('data-theme', 'dark');
                localStorage.setItem('theme', 'dark');
                window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme: 'dark' } }));
            }
        }

        if (!window.__themeToggleInitialized) {
            window.__themeToggleInitialized = true;
            if (toggleBtn) toggleBtn.addEventListener('click', toggleTheme);
            if (mobileToggleBtn) mobileToggleBtn.addEventListener('click', toggleTheme);
        }

        // 5. Mobile Menu Logic
        const menuToggle = document.getElementById('menu-toggle');
        const menuClose = document.getElementById('menu-close');
        const mobileMenu = document.getElementById('mobile-menu');
        const mobileMenuBackdrop = document.getElementById('mobile-menu-backdrop');

        function openMobileMenu() {
            if (!mobileMenu || !mobileMenuBackdrop) return;
            mobileMenu.classList.add('open');
            mobileMenuBackdrop.classList.add('open');
            mobileMenu.setAttribute('aria-hidden', 'false');
            menuToggle?.setAttribute('aria-expanded', 'true');
            document.body.classList.add('menu-open');
        }

        function closeMobileMenu() {
            if (!mobileMenu || !mobileMenuBackdrop) return;
            mobileMenu.classList.remove('open');
            mobileMenuBackdrop.classList.remove('open');
            mobileMenu.setAttribute('aria-hidden', 'true');
            menuToggle?.setAttribute('aria-expanded', 'false');
            document.body.classList.remove('menu-open');
        }

        if (menuToggle) {
            menuToggle.addEventListener('click', () => {
                mobileMenu.classList.contains('open') ? closeMobileMenu() : openMobileMenu();
            });
        }
        if (menuClose) menuClose.addEventListener('click', closeMobileMenu);
        if (mobileMenuBackdrop) mobileMenuBackdrop.addEventListener('click', closeMobileMenu);

        const mobileSearchBtn = document.getElementById('search-btn-mobile');
        if (mobileSearchBtn) {
            mobileSearchBtn.addEventListener('click', () => {
                const searchModal = document.getElementById('search-modal');
                const searchInput = document.getElementById('search-input');
                if (searchModal) {
                    searchModal.style.display = 'flex';
                    if (searchInput) searchInput.focus();
                }
            });
        }

        // 6. Universal Lightbox
        let lightbox = document.getElementById('lightbox');
        if (!lightbox) {
            lightbox = document.createElement('div');
            lightbox.id = 'lightbox';
            lightbox.innerHTML = `
                <div class="lightbox-content">
                    <img src="" alt="Expanded image" style="display: none;">
                    <div class="lightbox-svg-container" style="display: none;"></div>
                </div>
            `;
            document.body.appendChild(lightbox);
        }

        const lightboxImg = lightbox.querySelector('img');
        const lightboxSvgContainer = lightbox.querySelector('.lightbox-svg-container');

        function openLightbox(type, content) {
            if (type === 'img') {
                lightboxImg.src = content;
                lightboxImg.style.display = 'block';
                lightboxSvgContainer.style.display = 'none';
            } else if (type === 'svg') {
                lightboxSvgContainer.innerHTML = content;
                lightboxImg.style.display = 'none';
                lightboxSvgContainer.style.display = 'block';
                
                const svg = lightboxSvgContainer.querySelector('svg');
                if (svg) {
                    svg.removeAttribute('width');
                    svg.removeAttribute('height');
                    svg.setAttribute('width', '100%');
                    svg.style.display = 'block';
                }
            }
            lightbox.classList.add('active');
            document.body.style.overflow = 'hidden';
        }

        function closeLightbox() {
            lightbox.classList.remove('active');
            document.body.style.overflow = '';
            setTimeout(() => {
                if (!lightbox.classList.contains('active')) {
                    lightboxImg.src = '';
                    lightboxSvgContainer.innerHTML = '';
                }
            }, 300);
        }

        // 7. Global Event Delegation (Clicks & Keys)
        if (!window.__globalEventsInitialized) {
            window.__globalEventsInitialized = true;
            
            document.body.addEventListener('click', (e) => {
                // Handle Lightbox BG Click
                if (e.target.id === 'lightbox' || e.target.classList.contains('lightbox-content')) {
                    closeLightbox();
                    return;
                }

                // Handle Image Click
                const img = e.target.closest('article img, .content-body img');
                if (img && !img.closest('a') && !img.classList.contains('site-logo')) {
                    openLightbox('img', img.src);
                    return;
                }

                // Handle D2 SVG Click
                const d2Container = e.target.closest('.d2-container');
                if (d2Container) {
                    const isLight = document.documentElement.getAttribute('data-theme') === 'light';
                    const selector = isLight ? '.d2-light svg' : '.d2-dark svg';
                    const activeSvg = d2Container.querySelector(selector);
                    if (activeSvg) openLightbox('svg', activeSvg.outerHTML);
                    return;
                }

                // Handle Copy Button Click (Unified)
                const copyBtn = e.target.closest('.copy-btn, .copy-btn-explicit, .katex-copy-btn');
                if (copyBtn) {
                    let textToCopy = '';
                    const container = copyBtn.closest('.code-block-container, [data-latex]');
                    
                    if (copyBtn.classList.contains('katex-copy-btn')) {
                        textToCopy = container ? container.getAttribute('data-latex') : '';
                    } else {
                        const code = container ? container.querySelector('code') : null;
                        textToCopy = code ? code.textContent.trimEnd() : '';
                    }

                    if (textToCopy) {
                        navigator.clipboard.writeText(textToCopy).then(() => {
                            const textEl = copyBtn.querySelector('.copy-text') || copyBtn;
                            const originalText = textEl.textContent;
                            
                            textEl.textContent = 'Copied!';
                            copyBtn.classList.add('copied');
                            setTimeout(() => {
                                textEl.textContent = originalText;
                                copyBtn.classList.remove('copied');
                            }, 2000);
                        }).catch(err => console.error('Failed to copy:', err));
                    }
                    return;
                }

                // Handle Mobile Nav Link Click
                if (e.target.closest('.mobile-nav-link')) {
                    closeMobileMenu();
                }

                // Handle Back to Top Click
                if (e.target.closest('#back-to-top')) {
                    window.scrollTo({ top: 0, behavior: 'smooth' });
                }
            });

            document.addEventListener('keydown', (e) => {
                if (e.key === 'Escape') {
                    if (lightbox && lightbox.classList.contains('active')) closeLightbox();
                    if (mobileMenu && mobileMenu.classList.contains('open')) closeMobileMenu();
                }
            });
        }

        // 8. Table of Contents ScrollSpy
        const tocLinks = document.querySelectorAll('.toc-container a, .toc-modal-link');
        const sections = Array.from(document.querySelectorAll('article h1, article h2, article h3, article h4, article h5, article h6'))
            .filter(section => section.id);

        if (tocLinks.length > 0 && sections.length > 0) {
            const observerOptions = {
                root: null,
                rootMargin: '-100px 0px -70% 0px',
                threshold: 0
            };

            const observer = new IntersectionObserver((entries) => {
                // Find the entry that is intersecting and closest to the top
                const intersectingEntries = entries
                    .filter(entry => entry.isIntersecting)
                    .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);

                if (intersectingEntries.length > 0) {
                    const id = intersectingEntries[0].target.id;
                    updateActiveTOC(id);
                }
            }, observerOptions);

            function updateActiveTOC(id) {
                tocLinks.forEach(link => {
                    const isActive = link.getAttribute('href') === `#${id}`;
                    link.classList.toggle('active', isActive);
                    
                    if (isActive) {
                        const tocNav = link.closest('nav, .toc-modal-nav');
                        const tocContainer = link.closest('.toc-container');
                        const isModal = link.closest('.toc-modal-nav');
                        
                        // Only scroll the TOC itself if it's the floating sidebar or in the modal
                        // This prevents the page from jumping when using the inline TOC at the top of a post
                        const isFloatingSidebar = tocContainer && window.getComputedStyle(tocContainer).position === 'fixed';
                        
                        if (tocNav && (isFloatingSidebar || isModal)) {
                            const navRect = tocNav.getBoundingClientRect();
                            const linkRect = link.getBoundingClientRect();
                            if (linkRect.top < navRect.top || linkRect.bottom > navRect.bottom) {
                                link.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
                            }
                        }
                    }
                });
            }

            sections.forEach(section => observer.observe(section));
            
            // Initial check in case we're already scrolled down
            const initialSection = Array.from(sections).reverse().find(section => {
                const rect = section.getBoundingClientRect();
                return rect.top < window.innerHeight * 0.3;
            });
            if (initialSection) updateActiveTOC(initialSection.id);
        }

        // 9. Smooth Scroll for TOC Links
        tocLinks.forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const id = link.getAttribute('href').substring(1);
                const target = document.getElementById(id);
                if (target) {
                    if (link.classList.contains('toc-modal-link')) {
                        closeTocModal();
                    }
                    const headerHeight = document.querySelector('header')?.offsetHeight || 0;
                    const targetPosition = target.getBoundingClientRect().top + window.pageYOffset - headerHeight - 20;
                    window.scrollTo({
                        top: targetPosition,
                        behavior: 'smooth'
                    });
                }
            });
        });

        // 10. Search Highlight on Target Page
        (function initSearchHighlight() {
            const params = new URLSearchParams(window.location.search);
            const highlightTerm = params.get('highlight');
            if (!highlightTerm) return;

            // Clean the URL without reload
            params.delete('highlight');
            const cleanSearch = params.toString();
            const cleanURL = cleanSearch
                ? window.location.pathname + '?' + cleanSearch + window.location.hash
                : window.location.pathname + window.location.hash;
            history.replaceState({}, '', cleanURL);

            const SKIP_TAGS = new Set(['CODE', 'PRE', 'SCRIPT', 'STYLE', 'MARK']);

            function findFirstMatch(root, term) {
                const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
                    acceptNode(node) {
                        const parent = node.parentElement;
                        if (!parent) return NodeFilter.FILTER_REJECT;
                        if (SKIP_TAGS.has(parent.tagName)) return NodeFilter.FILTER_REJECT;
                        if (parent.classList && parent.classList.contains('search-highlight')) return NodeFilter.FILTER_REJECT;
                        return NodeFilter.FILTER_ACCEPT;
                    }
                });

                const lowerTerm = term.toLowerCase();
                let node;
                while ((node = walker.nextNode())) {
                    const text = node.textContent;
                    const lowerText = text.toLowerCase();
                    const idx = lowerText.indexOf(lowerTerm);
                    if (idx === -1) continue;

                    return { node, idx, length: term.length };
                }
                return null;
            }

            function highlightAndScroll(term) {
                const container = document.querySelector('article') || document.querySelector('.content-body') || document.body;
                const match = findFirstMatch(container, term);
                if (!match) return;

                const { node, idx, length } = match;
                const range = document.createRange();
                range.setStart(node, idx);
                range.setEnd(node, idx + length);

                const mark = document.createElement('mark');
                mark.className = 'search-highlight';
                range.surroundContents(mark);

                mark.scrollIntoView({ behavior: 'smooth', block: 'center' });

                setTimeout(() => {
                    mark.classList.add('fade-out');
                    setTimeout(() => {
                        const parent = mark.parentNode;
                        if (parent) {
                            while (mark.firstChild) parent.insertBefore(mark.firstChild, mark);
                            parent.removeChild(mark);
                        }
                    }, 500);
                }, 3000);
            }

            highlightAndScroll(highlightTerm);
        })();
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", init);
    } else {
        init();
    }
})();
