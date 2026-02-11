// Docs Theme - Enhanced Features
(function () {
    'use strict';

    // Initialize all features when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    function init() {
        // 1. Reading Progress Bar
        initProgressBar();

        // 2. Copy Code Buttons
        initCopyButtons();

        // 3. Heading Anchor Links
        initHeadingAnchors();

        // 4. TOC Scroll Highlighting
        initTocScrollHighlight();

        // 5. Mobile Navigation
        initMobileNav();

        // 6. Sidebar Collapsible
        initSidebar();

        // 7. Theme Toggle Enhancement
        initThemeToggle();
    }

    // 1. Reading Progress Bar
    function initProgressBar() {
        if (document.getElementById('progress-bar')) return;

        const progressBar = document.createElement('div');
        progressBar.id = 'progress-bar';
        document.body.appendChild(progressBar);

        window.addEventListener('scroll', () => {
            const scrollTop = window.scrollY || document.documentElement.scrollTop;
            const docHeight = document.documentElement.scrollHeight - document.documentElement.clientHeight;
            const scrolled = docHeight > 0 ? (scrollTop / docHeight) * 100 : 0;
            progressBar.style.width = scrolled + '%';
        });
    }

    // 2. Copy Code Buttons
    function initCopyButtons() {
        document.querySelectorAll('pre').forEach(pre => {
            // Skip if already has copy button
            if (pre.querySelector('.copy-btn')) return;

            // Skip D2 diagrams
            if (pre.classList.contains('d2')) return;

            const btn = document.createElement('button');
            btn.className = 'copy-btn';
            btn.textContent = 'Copy';
            btn.setAttribute('aria-label', 'Copy code to clipboard');

            btn.addEventListener('click', async () => {
                const code = pre.querySelector('code');
                if (!code) return;

                const text = code.textContent.trimEnd();

                try {
                    await navigator.clipboard.writeText(text);
                    btn.textContent = 'Copied!';
                    btn.classList.add('copied');

                    setTimeout(() => {
                        btn.textContent = 'Copy';
                        btn.classList.remove('copied');
                    }, 2000);
                } catch (err) {
                    console.error('Failed to copy:', err);
                    btn.textContent = 'Failed';
                    setTimeout(() => {
                        btn.textContent = 'Copy';
                        btn.classList.remove('copied');
                    }, 2000);
                };
            });

            // Append to code-wrapper or pre
            const wrapper = pre.closest('.code-wrapper');
            if (wrapper) {
                wrapper.appendChild(btn);
            } else {
                pre.appendChild(btn);
            }
        });
    }

    // 3. Heading Anchor Links
    function initHeadingAnchors() {
        const content = document.querySelector('.content');
        if (!content) return;

        content.querySelectorAll('h1, h2, h3, h4, h5, h6').forEach(heading => {
            // Skip if already has anchor
            if (heading.querySelector('.heading-anchor')) return;

            const id = heading.id;
            if (!id) return;

            const anchor = document.createElement('a');
            anchor.href = '#' + id;
            anchor.className = 'heading-anchor';
            anchor.textContent = '#';
            anchor.setAttribute('aria-label', 'Link to this heading');

            heading.appendChild(anchor);
        });
    }

    // 4. TOC Scroll Highlighting
    function initTocScrollHighlight() {
        const tocLinks = document.querySelectorAll('.toc-link');
        if (tocLinks.length === 0) return;

        const headings = [];
        tocLinks.forEach(link => {
            const href = link.getAttribute('href');
            if (href && href.startsWith('#')) {
                const heading = document.getElementById(href.slice(1));
                if (heading) {
                    headings.push({ link, heading });
                }
            }
        });

        if (headings.length === 0) return;

        // Use IntersectionObserver for better performance
        const observerOptions = {
            root: null,
            rootMargin: '-80px 0px -80% 0px',
            threshold: 0
        };

        const observer = new IntersectionObserver((entries) => {
            entries.forEach(entry => {
                if (entry.isIntersecting) {
                    // Remove active from all
                    tocLinks.forEach(link => link.classList.remove('active'));

                    // Add active to current
                    const match = headings.find(h => h.heading === entry.target);
                    if (match) {
                        match.link.classList.add('active');
                    }
                }
            });
        }, observerOptions);

        headings.forEach(h => observer.observe(h.heading));
    }

    // 5. Mobile Navigation
    function initMobileNav() {
        // Create mobile menu toggle button
        const header = document.querySelector('.docs-header nav');
        if (!header || document.getElementById('mobile-menu-toggle')) return;

        const toggleBtn = document.createElement('button');
        toggleBtn.id = 'mobile-menu-toggle';
        toggleBtn.className = 'mobile-menu-toggle';
        toggleBtn.innerHTML = '☰';
        toggleBtn.setAttribute('aria-label', 'Toggle navigation menu');

        header.insertBefore(toggleBtn, header.firstChild);

        // Create overlay
        const overlay = document.createElement('div');
        overlay.className = 'docs-sidebar-overlay';
        document.body.appendChild(overlay);

        const sidebar = document.querySelector('.docs-sidebar');

        function toggleMenu() {
            sidebar.classList.toggle('open');
            overlay.classList.toggle('open');
            toggleBtn.innerHTML = sidebar.classList.contains('open') ? '✕' : '☰';
        }

        toggleBtn.addEventListener('click', toggleMenu);
        overlay.addEventListener('click', toggleMenu);

        // Close menu when clicking a link (using delegation to support dynamic updates)
        sidebar.addEventListener('click', (e) => {
            if (e.target.tagName === 'A' || e.target.closest('a')) {
                if (window.innerWidth <= 768) {
                    toggleMenu();
                }
            }
        });
    }

    // 6. Sidebar Collapsible
    function initSidebar() {
        const sidebar = document.querySelector('.docs-sidebar');
        if (!sidebar) return;

        // Auto-expand parents of active link and highlight section
        const activeLink = sidebar.querySelector('.tree-link.active');
        if (activeLink) {
            // Find the immediate parent tree-item to mark as section-active
            const immediateParent = activeLink.closest('.tree-item');
            if (immediateParent) {
                immediateParent.classList.add('section-active');
            }

            let el = activeLink;
            while (el && el !== sidebar) {
                if (el.classList.contains('tree-group')) {
                    el.classList.remove('collapsed');
                }
                if (el.classList.contains('tree-item')) {
                    el.classList.add('expanded');
                }
                el = el.parentElement;
            }
        }

        sidebar.addEventListener('click', (e) => {
            const toggle = e.target.closest('.tree-toggle');
            const label = e.target.closest('.tree-label');

            if (toggle || label) {
                const item = (toggle || label).closest('.tree-item');
                const group = item.querySelector('.tree-group');

                if (group) {
                    const isCollapsed = group.classList.toggle('collapsed');
                    item.classList.toggle('expanded', !isCollapsed);
                }
            }
        });
    }

    // 7. Theme Toggle Enhancement
    function initThemeToggle() {
        const toggle = document.getElementById('theme-toggle');
        if (!toggle) return;

        // Load saved theme
        const savedTheme = localStorage.getItem('theme') || 'dark';
        document.documentElement.setAttribute('data-theme', savedTheme);

        toggle.addEventListener('click', () => {
            const currentTheme = document.documentElement.getAttribute('data-theme') || 'dark';
            const nextTheme = currentTheme === 'dark' ? 'light' : 'dark';
            
            document.documentElement.setAttribute('data-theme', nextTheme);
            localStorage.setItem('theme', nextTheme);
        });
    }
})();
