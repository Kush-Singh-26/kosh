document.addEventListener("DOMContentLoaded", function () {
    // 1. KaTeX Init
    // Checks if the render function exists (loaded from external script)
    if (typeof renderMathInElement === 'function') {
        const mathLayer = document.querySelector('.content-body') || document.body;
        requestAnimationFrame(() => {
            renderMathInElement(mathLayer, {
                delimiters: [
                    { left: '$$', right: '$$', display: true },
                    { left: '$', right: '$', display: false },
                    { left: '\\(', right: '\\)', display: false },
                    { left: '\\[', right: '\\]', display: true }
                ],
                throwOnError: false
            });
        });
    }

    // 2. Image Path Fix for GitHub Pages (or sub-directory hosting)
    // 'siteBaseURL' is defined in layout.html
    if (typeof siteBaseURL !== 'undefined' && siteBaseURL && siteBaseURL !== "") {
        document.querySelectorAll('img').forEach(img => {
            const src = img.getAttribute('src');
            // If src is absolute to root (starts with /) but doesn't already have base URL
            if (src && src.startsWith('/') && !src.startsWith(siteBaseURL)) {
                img.src = siteBaseURL + src;
            }
        });
    }

    // 3. Copy Logic for Code Blocks (skip mermaid diagrams)
    document.querySelectorAll('pre').forEach(pre => {
        // Skip mermaid diagram blocks
        if (pre.classList.contains('mermaid')) return;

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

        pre.appendChild(btn);
    });

    // 4. Reading Progress Bar
    const progressBar = document.createElement('div');
    progressBar.id = 'progress-bar';
    document.body.appendChild(progressBar);

    window.addEventListener('scroll', () => {
        const scrollTop = document.documentElement.scrollTop || document.body.scrollTop;
        const scrollHeight = document.documentElement.scrollHeight - document.documentElement.clientHeight;
        const scrolled = (scrollTop / scrollHeight) * 100;
        progressBar.style.width = scrolled + "%";
    });


    // 5. Theme Toggle Logic
    const toggleBtn = document.getElementById('theme-toggle');
    const htmlEl = document.documentElement;

    if (toggleBtn) {
        toggleBtn.addEventListener('click', () => {
            // Check if currently light
            const isLight = htmlEl.getAttribute('data-theme') === 'light';

            if (isLight) {
                // Switch to Dark (Default)
                htmlEl.removeAttribute('data-theme');
                localStorage.setItem('theme', 'dark');
                window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme: 'dark' } }));
            } else {
                // Switch to Light
                htmlEl.setAttribute('data-theme', 'light');
                localStorage.setItem('theme', 'light');
                window.dispatchEvent(new CustomEvent('themeChanged', { detail: { theme: 'light' } }));
            }
        });
    }

    // 6. Image Lightbox
    const lightbox = document.createElement('div');
    lightbox.id = 'lightbox';
    lightbox.innerHTML = '<img src="" alt="Expanded image">';
    document.body.appendChild(lightbox);

    const lightboxImg = lightbox.querySelector('img');

    // Only apply to article images
    document.querySelectorAll('article img, .content-body img').forEach(img => {
        // Skip small icons/logos
        if (img.width < 100 || img.classList.contains('site-logo')) return;

        img.style.cursor = 'zoom-in';
        img.addEventListener('click', () => {
            lightboxImg.src = img.src;
            lightbox.classList.add('active');
            document.body.style.overflow = 'hidden';
        });
    });

    lightbox.addEventListener('click', () => {
        lightbox.classList.remove('active');
        document.body.style.overflow = '';
    });

    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && lightbox.classList.contains('active')) {
            lightbox.classList.remove('active');
            document.body.style.overflow = '';
        }
    });

    // 7. Back to Top Button
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

    // 8. Code Block Language Badges
    document.querySelectorAll('.code-wrapper').forEach(wrapper => {
        const lang = wrapper.getAttribute('data-lang');
        if (lang && lang !== 'text') {
             const langName = lang.toUpperCase();
             if (!wrapper.querySelector('.code-badge')) {
                 const badge = document.createElement('span');
                 badge.className = 'code-badge';
                 badge.textContent = langName;
                 wrapper.appendChild(badge);
             }
        }
    });

    // 9. Smart Admonitions (Color-based Blockquotes)
    // Format: "ColorName: Text..."
    document.querySelectorAll('blockquote').forEach(bq => {
        // Find the first text node or element
        const p = bq.querySelector('p');
        if (!p) return;

        const text = p.innerHTML;
        // Regex to match "Word:" at the start
        const match = text.match(/^([a-zA-Z]+):\s/);

        if (match) {
            const colorName = match[1];
            // Basic validation to ensure it looks like a color (not just any random word)
            // You can restrict this to a specific list if preferred.
            
            // Apply styles
            bq.style.backgroundColor = `color-mix(in srgb, ${colorName} 10%, transparent)`; // Modern CSS tint
            bq.style.borderLeftColor = colorName;
            
            // Fallback for older browsers if color-mix fails (simple opacity approach via rgba is harder with named colors without canvas helper)
            // simpler approach: assign a css variable locally
            bq.style.setProperty('--quote-color', colorName);
            bq.classList.add('colored-quote');

            // Remove the prefix
            p.innerHTML = text.replace(match[0], '');
        }
    });

    // 10. Table of Contents ScrollSpy
    const tocLinks = document.querySelectorAll('.toc-container a');
    const sections = document.querySelectorAll('article h1, article h2, article h3, article h4, article h5, article h6');

    if (tocLinks.length > 0 && sections.length > 0) {
        const observerOptions = {
            root: null,
            rootMargin: '0px 0px -80% 0px', // Trigger when section is near top
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
                                // Optional: Auto-scroll TOC container to keep active link in view
                                const tocNav = link.closest('nav');
                                if(tocNav) {
                                     // Simple check to see if link is out of view
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

        sections.forEach(section => {
            observer.observe(section);
        });
    }

});
