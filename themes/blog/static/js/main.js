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

        // 2. Copy Logic for Code Blocks (skip d2 diagrams)
        document.querySelectorAll('pre').forEach(pre => {
            if (pre.classList.contains('d2') || pre.querySelector('.copy-btn')) return;

            const btn = document.createElement('button');
            btn.className = 'copy-btn';
            btn.textContent = 'Copy';

            btn.addEventListener('click', () => {
                const code = pre.querySelector('code');
                if (!code) return;
                const textToCopy = code.textContent.trimEnd();

                navigator.clipboard.writeText(textToCopy).then(() => {
                    btn.textContent = 'Copied!';
                    btn.classList.add('copied');
                    setTimeout(() => {
                        btn.textContent = 'Copy';
                        btn.classList.remove('copied');
                    }, 2000);
                }).catch(err => {
                    console.error('Failed to copy:', err);
                });
            });

            if (pre.parentElement && pre.parentElement.classList.contains('code-wrapper')) {
                pre.parentElement.appendChild(btn);
            } else {
                pre.appendChild(btn);
            }
        });

        // 4. Reading Progress Bar
        if (!document.getElementById('progress-bar')) {
            const progressBar = document.createElement('div');
            progressBar.id = 'progress-bar';
            document.body.appendChild(progressBar);

            window.addEventListener('scroll', () => {
                const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
                const scrollHeight = document.documentElement.scrollHeight - document.documentElement.clientHeight;
                const scrolled = (scrollTop / scrollHeight) * 100;
                progressBar.style.width = scrolled + "%";
            });
        }

        // 5. Theme Toggle Logic
        const toggleBtn = document.getElementById('theme-toggle');
        const htmlEl = document.documentElement;

        // Use a global flag to prevent race conditions if init() is called multiple times
        if (toggleBtn && !window.__themeToggleInitialized) {
            window.__themeToggleInitialized = true;
            toggleBtn.addEventListener('click', () => {
                const isLight = htmlEl.getAttribute('data-theme') === 'light';
                if (isLight) {
                    htmlEl.removeAttribute('data-theme');
                    localStorage.setItem('theme', 'dark');
                    window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme: 'dark' } }));
                } else {
                    htmlEl.setAttribute('data-theme', 'light');
                    localStorage.setItem('theme', 'light');
                    window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme: 'light' } }));
                }
            });
        }

        // 6. Universal Lightbox (Event Delegation)
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
            lightbox.addEventListener('click', closeLightbox);
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
                    svg.setAttribute('height', '100%');
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

        // Global Click Listener for Zoom (Event Delegation)
        document.body.addEventListener('click', (e) => {
            // Handle Images
            const img = e.target.closest('article img, .content-body img');
            if (img && !img.closest('a') && !img.classList.contains('site-logo')) {
                openLightbox('img', img.src);
                return;
            }

            // Handle D2 Diagrams (click on SVG or its container)
            const d2Container = e.target.closest('.d2-container');
            if (d2Container) {
                const isLight = document.documentElement.getAttribute('data-theme') === 'light';
                const selector = isLight ? '.d2-light svg' : '.d2-dark svg';
                const activeSvg = d2Container.querySelector(selector);

                if (activeSvg) {
                    openLightbox('svg', activeSvg.outerHTML);
                }
            }
        });

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && lightbox && lightbox.classList.contains('active')) {
                closeLightbox();
            }
        });

        // 7. Back to Top Button
        if (!document.getElementById('back-to-top')) {
            const backToTopBtn = document.createElement('button');
            backToTopBtn.id = 'back-to-top';
            backToTopBtn.innerHTML = '↑';
            backToTopBtn.ariaLabel = 'Back to Top';
            document.body.appendChild(backToTopBtn);

            window.addEventListener('scroll', () => {
                if (window.scrollY > 300) {
                    backToTopBtn.classList.add('visible');
                } else {
                    backToTopBtn.classList.remove('visible');
                }
            });

            backToTopBtn.addEventListener('click', () => {
                window.scrollTo({ top: 0, behavior: 'smooth' });
            });
        }

        // 8. Code Block Language Badges
        document.querySelectorAll('.code-wrapper').forEach(wrapper => {
            const lang = wrapper.getAttribute('data-lang');
            if (lang && lang !== 'text' && !wrapper.querySelector('.code-badge')) {
                const badge = document.createElement('span');
                badge.className = 'code-badge';
                badge.textContent = lang.toUpperCase();
                wrapper.appendChild(badge);
                wrapper.classList.add('has-badge');
            }
        });

        // 9. Smart Admonitions (Color-based Blockquotes)
        document.querySelectorAll('blockquote').forEach(bq => {
            if (bq.classList.contains('colored-quote')) return;
            const p = bq.querySelector('p');
            if (!p) return;
            const text = p.innerHTML;
            const match = text.match(/^([a-zA-Z]+):\s/);
            if (match) {
                const colorName = match[1];
                bq.style.setProperty('--quote-color', colorName);
                bq.style.backgroundColor = `color-mix(in srgb, ${colorName} 10%, transparent)`;
                bq.style.borderLeftColor = colorName;
                bq.classList.add('colored-quote');
                p.innerHTML = text.replace(match[0], '');
            }
        });

        // 10. Table of Contents ScrollSpy
        const tocLinks = document.querySelectorAll('.toc-container a');
        const sections = document.querySelectorAll('article h1, article h2, article h3, article h4, article h5, article h6');

        if (tocLinks.length > 0 && sections.length > 0) {
            const observerOptions = {
                root: null,
                rootMargin: '0px 0px -80% 0px',
                threshold: 0
            };

            const observer = new IntersectionObserver((entries) => {
                entries.forEach(entry => {
                    if (entry.isIntersecting) {
                        const id = entry.target.id;
                        if (id) {
                            tocLinks.forEach(link => {
                                link.classList.remove('active');
                                if (link.getAttribute('href') === `#${id}`) {
                                    link.classList.add('active');
                                    const tocNav = link.closest('nav');
                                    if (tocNav) {
                                        const navRect = tocNav.getBoundingClientRect();
                                        const linkRect = link.getBoundingClientRect();
                                        if (linkRect.top < navRect.top || linkRect.bottom > navRect.bottom) {
                                            link.scrollIntoView({ block: 'center', behavior: 'smooth' });
                                        }
                                    }
                                }
                            });
                        }
                    }
                });
            }, observerOptions);

            sections.forEach(section => observer.observe(section));
        }
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", init);
    } else {
        init();
    }
})();
