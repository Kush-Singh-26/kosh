document.addEventListener("DOMContentLoaded", function() {
    // 1. KaTeX Init
    // Checks if the render function exists (loaded from external script)
    if (typeof renderMathInElement === 'function') {
        renderMathInElement(document.body, {
            delimiters: [
                {left: '$$', right: '$$', display: true},
                {left: '$', right: '$', display: false},
                {left: '\\(', right: '\\)', display: false},
                {left: '\\[', right: '\\]', display: true}
            ],
            throwOnError : false
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

    // 3. Copy Logic for Code Blocks
    document.querySelectorAll('pre').forEach(pre => {
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
            } else {
                // Switch to Light
                htmlEl.setAttribute('data-theme', 'light');
                localStorage.setItem('theme', 'light');
            }
        });
    }

});

