(function() {
    const nmap = {};
    let nodes = [];
    const tags = [];
    let links = [];

    const canvas = document.getElementById('graph-canvas');
    if (!canvas) { console.error('Graph canvas not found'); return; }
    const ctx = canvas.getContext('2d');

    let dotsDirty = true;
    let W, H;
    function resize() {
        const dpr = window.devicePixelRatio || 1;
        const rect = canvas.parentElement.getBoundingClientRect();
        W = rect.width;
        H = rect.height;
        canvas.width = W * dpr;
        canvas.height = H * dpr;
        ctx.scale(dpr, dpr);
        dotsDirty = true;
    }
    window.addEventListener('resize', resize);
    resize();

    let camX = 0, camY = 0, zoom = 0.05;
    let tCamX = 0, tCamY = 0, tZoom = 0.14;
    let startTick = Date.now();
    const animDuration = 1200;

    function toScreen(wx, wy) {
        return [wx * zoom + W / 2 + camX, wy * zoom + H / 2 + camY];
    }
    function toWorld(sx, sy) {
        return [(sx - W / 2 - camX) / zoom, (sy - H / 2 - camY) / zoom];
    }

    // --- Cached CSS vars (perf #1) ---
    let cssVars = null;
    function readCSSVars() {
        const s = getComputedStyle(document.documentElement);
        cssVars = {
            accentPrimary: s.getPropertyValue('--accent-primary').trim() || '#8c948c',
            accentLink: s.getPropertyValue('--accent-link').trim() || '#6c7e8c',
            accentSecondary: s.getPropertyValue('--accent-secondary').trim() || '#a38c85',
            accentSage: s.getPropertyValue('--accent-sage').trim() || '#8c948c',
            accentClay: s.getPropertyValue('--accent-clay').trim() || '#a38c85',
            accentIndigo: s.getPropertyValue('--accent-indigo').trim() || '#6c7e8c',
            accentGold: s.getPropertyValue('--accent-gold').trim() || '#b4a896',
            accentWarm: s.getPropertyValue('--accent-warm').trim() || '#a38c85',
            accentTerracotta: s.getPropertyValue('--accent-terracotta').trim() || '#c4704a',
            textMuted: s.getPropertyValue('--text-muted').trim() || '#787470',
            cardBg: s.getPropertyValue('--bg-card').trim() || '#23221e'
        };
        recalcAllColors();
    }

    // --- Cached node colors (perf #2) ---
    function computeNodeColor(n) {
        if (!n || typeof n !== 'object') return cssVars.accentSage;
        if (n.group === 0) return cssVars.accentGold;
        if (n.group === 2) return cssVars.accentIndigo;
        const id = n.id || '';
        const hash = id.split('').reduce((acc, ch) => ch.charCodeAt(0) + ((acc << 5) - acc), 0);
        return hash % 2 === 0 ? cssVars.accentTerracotta : cssVars.accentSage;
    }

    function recalcAllColors() {
        for (let i = 0; i < nodes.length; i++) {
            nodes[i].color = computeNodeColor(nodes[i]);
        }
    }

    // --- Offscreen dot grid (perf #7) ---
    let dotCanvas = null, dotZoom = 0;

    function renderDotGrid() {
        const sp = Math.max(20, 60 * zoom);
        const neededW = W + sp * 2;
        const neededH = H + sp * 2;
        if (!dotCanvas) dotCanvas = document.createElement('canvas');
        if (dotCanvas.width < neededW || dotCanvas.height < neededH) {
            dotCanvas.width = Math.ceil(neededW);
            dotCanvas.height = Math.ceil(neededH);
        }
        const dctx = dotCanvas.getContext('2d');
        dctx.clearRect(0, 0, dotCanvas.width, dotCanvas.height);
        dctx.fillStyle = cssVars.accentSecondary;
        dctx.globalAlpha = 0.02;
        const ox = (W / 2 + camX) % sp;
        const oy = (H / 2 + camY) % sp;
        for (let x = ox - sp; x < W + sp; x += sp)
            for (let y = oy - sp; y < H + sp; y += sp) {
                dctx.beginPath();
                dctx.arc(x, y, 1, 0, Math.PI * 2);
                dctx.fill();
            }
        dotZoom = zoom;
        dotsDirty = false;
    }

    // Re-cache CSS vars on theme toggle
    new MutationObserver(() => { readCSSVars(); dotsDirty = true; wakeLoop(); })
        .observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'data-theme'] });

    function handleGraphFocus(link) {
        if (!link) return;

        const getPath = (u) => {
            if (!u) return '';
            if (u.includes('://')) {
                try {
                    const url = new URL(u);
                    return url.pathname;
                } catch {
                    return u.replace(/^https?:\/\/[^/]+/, '');
                }
            }
            return u;
        };

        const focusPath = getPath(link).replace(/\/$/, '').replace(/\/index\.html$/, '');

        const found = nodes.find(n => {
            const nodePath = getPath(n.url || n.id).replace(/\/$/, '').replace(/\/index\.html$/, '');

            if (nodePath === focusPath) return true;

            const focusSeg = focusPath.split('/').pop();
            const nodeSeg = nodePath.split('/').pop();
            if (focusSeg && nodeSeg && focusSeg === nodeSeg) return true;

            return false;
        });

        if (found) {
            openPanel(found);
            tZoom = 1.1;
            focusOn(found);
            pingNode = found;
            pingStart = Date.now();
        }
    }

    const baseURL = window.siteBaseURL || '';
    const joinPath = (base, path) => {
        if (!base) return path;
        const needsSlash = !base.endsWith('/') && !path.startsWith('/');
        const hasDoubleSlash = base.endsWith('/') && path.startsWith('/');
        if (hasDoubleSlash) return base + path.slice(1);
        if (needsSlash) return base + '/' + path;
        return base + path;
    };

    fetch(joinPath(baseURL, '/graph.json'))
        .then(res => { if (!res.ok) throw new Error('Failed to load graph'); return res.json(); })
        .then(data => {
            window.graphData = data;
            initNodes(data);
            readCSSVars();

            const urlParams = new URLSearchParams(window.location.search);
            const focusLink = urlParams.get('focus');
            if (focusLink) {
                handleGraphFocus(focusLink);
            }

            loop();
        })
        .catch(err => { console.error('Graph load error:', err); document.getElementById('graph-count').textContent = 'Error'; });

    function initNodes(data) {
        data.nodes.forEach(n => {
            const isRoot = n.group === 0;
            const isTag = n.group === 2;
            nmap[n.id] = {
                ...n,
                x: isRoot ? 0 : (Math.random() - 0.5) * 200,
                y: isRoot ? 0 : (Math.random() - 0.5) * 200,
                vx: 0, vy: 0,
                r: isRoot ? 22 : (isTag ? 10 : 7),
                conns: [],
                isRoot,
                isTag,
                entranceDelay: Math.random() * 400,
                entranceStart: Date.now(),
                color: null,
                lastBounce: 0
            };
            if (isTag) tags.push(nmap[n.id]);
        });

        data.links.forEach(l => {
            const a = nmap[l.source], b = nmap[l.target];
            if (!a || !b) return;
            const isTag = a.group === 2 || b.group === 2;
            links.push({ a, b, isTag, weight: l.weight || 1 });
            if (!a.conns.includes(b)) a.conns.push(b);
            if (!b.conns.includes(a)) b.conns.push(a);
        });

        nodes = Object.values(nmap);
        tagNodes = nodes.filter(n => n.isTag);
        articleNodes = nodes.filter(n => n.group === 1);
        rootNode = nodes.find(n => n.isRoot);
        nodes.forEach(n => { n.r += Math.min(Math.sqrt(n.conns.length) * 0.8, 5); });
        document.getElementById('graph-count').textContent = nodes.length;
    }

    let tick = 0;
    const CIRCLE_R = 380;
    let running = true;
    let rootNode = null;

    // --- Cached filtered arrays for physics (perf #5) ---
    let tagNodes = [], articleNodes = [];

    function physics() {
        if (nodes.length === 0) return;
        if (!rootNode) return;
        tick++;
        if (tick > 550) return;

        let totalV = 0;
        for (let i = 0; i < nodes.length; i++) {
            for (let j = i + 1; j < nodes.length; j++) {
                const a = nodes[i], b = nodes[j];
                if (a.isRoot || b.isRoot) continue;
                const dx = b.x - a.x, dy = b.y - a.y;
                const d2 = dx * dx + dy * dy + 60;
                const f = 3000 / d2;
                a.vx -= dx * f; a.vy -= dy * f;
                b.vx += dx * f; b.vy += dy * f;
            }
        }

        for (let i = 0; i < tagNodes.length; i++) {
            const t = tagNodes[i];
            const dx = t.x - rootNode.x, dy = t.y - rootNode.y;
            const dist = Math.sqrt(dx * dx + dy * dy) || 1;
            const target = 140;
            const f = (dist - target) * 0.04;
            t.vx -= (dx / dist) * f;
            t.vy -= (dy / dist) * f;
        }

        for (let i = 0; i < articleNodes.length; i++) {
            const a = articleNodes[i];
            const parentTag = a.conns.find(c => c.isTag);
            if (parentTag) {
                const dx = a.x - parentTag.x, dy = a.y - parentTag.y;
                const dist = Math.sqrt(dx * dx + dy * dy) || 1;
                const target = 85;
                const f = (dist - target) * 0.035;
                a.vx -= (dx / dist) * f;
                a.vy -= (dy / dist) * f;
            }
            const dx = a.x - rootNode.x, dy = a.y - rootNode.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist > CIRCLE_R) {
                const f = (dist - CIRCLE_R) * 0.08;
                a.vx -= (dx / dist) * f;
                a.vy -= (dy / dist) * f;
            }
        }

        for (let i = 0; i < tagNodes.length; i++) {
            const t = tagNodes[i];
            const dx = t.x - rootNode.x, dy = t.y - rootNode.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist > CIRCLE_R - 40) {
                const f = (dist - (CIRCLE_R - 40)) * 0.06;
                t.vx -= (dx / dist) * f;
                t.vy -= (dy / dist) * f;
            }
        }

        for (let i = 0; i < links.length; i++) {
            const { a, b } = links[i];
            if (a.isRoot || b.isRoot) continue;
            const dx = b.x - a.x, dy = b.y - a.y;
            const d = Math.sqrt(dx * dx + dy * dy) || 1;
            const target = links[i].isTag ? 120 : 80;
            const str = a.isTag || b.isTag ? 0.03 : 0.02;
            const f = (d - target) * str;
            const fx = dx / d * f, fy = dy / d * f;
            a.vx += fx; a.vy += fy;
            b.vx -= fx; b.vy -= fy;
        }

        for (let i = 0; i < nodes.length; i++) {
            const n = nodes[i];
            if (n.isRoot) continue;
            n.vx *= 0.74; n.vy *= 0.74;
            n.vx = Math.max(-10, Math.min(10, n.vx));
            n.vy = Math.max(-10, Math.min(10, n.vy));
            n.x += n.vx; n.y += n.vy;
            totalV += Math.abs(n.vx) + Math.abs(n.vy);
        }

        if (totalV < 0.5) tick = 99999999;
    }

    let hover = null, active = null, searchMatch = null;
    let pingNode = null, pingStart = 0;
    let following = null;

    function draw() {
        const { accentPrimary, accentSecondary, textMuted, cardBg } = cssVars;

        ctx.clearRect(0, 0, W, H);

        if (following && !dragging && !touchDragging) {
            tCamX = -following.x * tZoom;
            tCamY = -following.y * tZoom;
        }

        camX += (tCamX - camX) * 0.12;
        camY += (tCamY - camY) * 0.12;
        zoom += (tZoom - zoom) * 0.12;

        const focus = active || searchMatch || hover;
        const focusSet = new Set();
        if (focus) { focusSet.add(focus); focus.conns.forEach(n => focusSet.add(n)); }

        // --- Offscreen dot grid (perf #7) ---
        if (dotsDirty || Math.abs(zoom - dotZoom) / (dotZoom || 1) > 0.15) renderDotGrid();
        if (dotCanvas) ctx.drawImage(dotCanvas, 0, 0, W, H);

        links.forEach(l => {
            const [ax, ay] = toScreen(l.a.x, l.a.y);
            const [bx, by] = toScreen(l.b.x, l.b.y);
            const focused = focus && (l.a === focus || l.b === focus);
            const faded = focus && !focused;

            const mx = (ax + bx) / 2, my = (ay + by) / 2;
            const dx = bx - ax, dy = by - ay;
            const len = Math.sqrt(dx * dx + dy * dy) || 1;
            const perpX = -dy / len * len * 0.15, perpY = dx / len * len * 0.15;

            ctx.beginPath();
            ctx.moveTo(ax, ay);
            ctx.quadraticCurveTo(mx + perpX, my + perpY, bx, by);

            if (focused) {
                const grad = ctx.createLinearGradient(ax, ay, bx, by);
                grad.addColorStop(0, l.a.color);
                grad.addColorStop(1, l.b.color);
                ctx.strokeStyle = grad;
                ctx.globalAlpha = 1;
                ctx.lineWidth = 3 * zoom;
            }
            else if (l.a.isRoot || l.b.isRoot) {
                return;
            }
            else {
                ctx.strokeStyle = textMuted;
                ctx.globalAlpha = faded ? 0.04 : 0.35;
                ctx.lineWidth = 1 * zoom;

                if (!focus) {
                    const pulse = Math.sin(Date.now() / 1500 + l.weight) * 0.1;
                    ctx.globalAlpha += pulse;
                }
            }
            ctx.stroke();
        });

        for (let i = 0; i < nodes.length; i++) {
            const n = nodes[i];
            if (n.isRoot) continue;
            const [sx, sy] = toScreen(n.x, n.y);

            const bounceTgt = n === hover ? 1 : 0;
            n.lastBounce += (bounceTgt - n.lastBounce) * 0.25;
            const bounce = 1 + n.lastBounce * 0.22;

            const entranceMs = 800;
            let entranceElapsed = Date.now() - n.entranceStart - n.entranceDelay;
            let entranceScale = 1;
            let entranceAlpha = 1;
            if (entranceElapsed < entranceMs) {
                const t = Math.max(0, entranceElapsed / entranceMs);
                entranceScale = 1 - Math.pow(1 - t, 3);
                entranceAlpha = t;
            }
            const r = n.r * zoom * entranceScale * bounce;

            if (sx + r < 0 || sx - r > W || sy + r < 0 || sy - r > H) continue;
            const isTag = n.group === 2;
            const color = n.color;
            const isFocus = focus && focusSet.has(n);
            const isFaded = focus && !focusSet.has(n);
            const isActive = n === active || n === searchMatch;

            // --- Conditional shadow blur (perf #6) ---
            ctx.globalAlpha = (isFaded ? 0.08 : 1) * entranceAlpha;
            if (!isFaded && zoom >= 0.3) {
                ctx.shadowColor = color;
                ctx.shadowBlur = isActive ? 32 : (isFocus ? 22 : (zoom > 0.8 ? 12 : 6));
            } else {
                ctx.shadowBlur = 0;
            }

            ctx.beginPath();
            if (isTag) {
                const s = r * 0.85;
                ctx.roundRect(sx - s, sy - s, s * 2, s * 2, 6);
            }
            else ctx.arc(sx, sy, r, 0, Math.PI * 2);

            ctx.fillStyle = cardBg;
            ctx.fill();

            if (n === hover && entranceAlpha > 0.92) {
                ctx.save();
                ctx.shadowBlur = 30;
                ctx.shadowColor = color;
                ctx.beginPath();
                if (isTag) {
                    const s = r * 0.92;
                    ctx.roundRect(sx - s, sy - s, s * 2, s * 2, 8);
                } else {
                    ctx.arc(sx, sy, r * 1.04, 0, Math.PI * 2);
                }
                ctx.lineWidth = 5.2;
                ctx.strokeStyle = color;
                ctx.globalAlpha = 0.68 * entranceAlpha;
                ctx.stroke();
                ctx.globalAlpha = 1;
                ctx.restore();
            }

            ctx.lineWidth = isActive ? 3 : (isFocus ? 2 : 1.5);
            ctx.strokeStyle = color;
            ctx.stroke();

            if (!isFaded && zoom > 0.4) {
                ctx.beginPath();
                ctx.arc(sx, sy, r * 0.25, 0, Math.PI * 2);
                ctx.fillStyle = color;
                ctx.globalAlpha = isActive ? 1 : 0.6;
                ctx.fill();
                ctx.globalAlpha = 1;
            }

            ctx.shadowBlur = 0;

            const showLabel = n === active || n === searchMatch || n === hover || n.isRoot ||
                (!isFaded && zoom > 1.1) || (!isFaded && n.conns.length > 3 && zoom > 0.7);
            if (showLabel) {
                const fs = Math.max(10, 11 * zoom);
                const weight = n.isRoot ? '700' : '500';
                const fontStack = zoom > 1.2 ? 'var(--font-serif)' : 'var(--font-main)';
                ctx.font = `${weight} ${fs}px ${fontStack}`;
                ctx.fillStyle = color;
                ctx.textAlign = 'center';
                ctx.globalAlpha = (n === hover) ? 1 : (isFaded ? 0.12 : 1);
                const lbl = n.label.length > 35 ? n.label.slice(0, 34) + '\u2026' : n.label;
                ctx.fillText(lbl, sx, sy + r + fs + 8);
            }
        }

        // Root node drawn last (on top)
        if (rootNode) {
            const [rx, ry] = toScreen(rootNode.x, rootNode.y);
            const cr = CIRCLE_R * zoom;
            ctx.beginPath();
            ctx.arc(rx, ry, cr, 0, Math.PI * 2);
            ctx.strokeStyle = accentPrimary;
            ctx.globalAlpha = 0.15;
            ctx.lineWidth = 1;
            ctx.stroke();
            ctx.globalAlpha = 1;

            const r = rootNode.r * zoom;
            ctx.shadowColor = rootNode.color;
            ctx.shadowBlur = 10;
            ctx.beginPath();
            ctx.arc(rx, ry, r, 0, Math.PI * 2);
            ctx.fillStyle = cardBg;
            ctx.fill();
            ctx.lineWidth = 1.5;
            ctx.strokeStyle = rootNode.color;
            ctx.stroke();
            ctx.shadowBlur = 0;

            if (zoom > 0.4) {
                ctx.beginPath();
                ctx.arc(rx, ry, r * 0.25, 0, Math.PI * 2);
                ctx.fillStyle = rootNode.color;
                ctx.globalAlpha = 0.6;
                ctx.fill();
                ctx.globalAlpha = 1;
            }

            const fs = Math.max(10, 11 * zoom);
            ctx.font = `700 ${fs}px ${zoom > 1.2 ? 'var(--font-serif)' : 'var(--font-main)'}`;
            ctx.fillStyle = rootNode.color;
            ctx.textAlign = 'center';
            ctx.globalAlpha = 1;
            const lbl = rootNode.label.length > 35 ? rootNode.label.slice(0, 34) + '\u2026' : rootNode.label;
            ctx.fillText(lbl, rx, ry + r + fs + 8);
        }

        if (pingNode) {
            const elapsed = Date.now() - pingStart;
            if (elapsed > 1000) {
                pingNode = null;
            } else {
                const [sx, sy] = toScreen(pingNode.x, pingNode.y);
                const t = elapsed / 1000;
                const r = (pingNode.r + (40 * t)) * zoom;
                ctx.beginPath();
                ctx.arc(sx, sy, r, 0, Math.PI * 2);
                ctx.strokeStyle = pingNode.color;
                ctx.globalAlpha = (1 - t) * 0.8;
                ctx.lineWidth = 3;
                ctx.stroke();
                ctx.globalAlpha = 1;
            }
        }

        ctx.globalAlpha = 1; ctx.shadowBlur = 0;
    }

    function needsAnimation() {
        if (tick < 999999) return true;
        if (following && !dragging && !touchDragging) return true;
        const dx = Math.abs(tCamX - camX), dy = Math.abs(tCamY - camY), dz = Math.abs(tZoom - zoom);
        if (dx > 0.01 || dy > 0.01 || dz > 0.001) return true;
        if (pingNode) return true;
        return false;
    }

    function wakeLoop() {
        if (!running) { running = true; loop(); }
    }

    function loop() {
        physics();
        draw();
        if (needsAnimation()) {
            requestAnimationFrame(loop);
        } else {
            running = false;
        }
    }

    function nodeAt(mx, my) {
        const [wx, wy] = toWorld(mx, my);
        for (let i = nodes.length - 1; i >= 0; i--) {
            const n = nodes[i];
            const dx = n.x - wx, dy = n.y - wy;
            const hit = n.r / zoom + 10;
            if (dx * dx + dy * dy < hit * hit) return n;
        }
        return null;
    }

    const panel = document.getElementById('graph-panel');
    function openPanel(n) {
        active = n;
        searchMatch = null;
        wakeLoop();
        const typeEl = document.getElementById('graph-p-type');
        typeEl.textContent = n.group === 0 ? 'Hub' : (n.group === 2 ? 'Tag' : 'Article');
        typeEl.style.color = n.color;
        document.getElementById('graph-p-title').textContent = n.label;

        const dateEl = document.getElementById('graph-p-date');
        if (n.date) {
            dateEl.textContent = n.date;
            dateEl.style.display = 'block';
        } else {
            dateEl.style.display = 'none';
        }

        const descEl = document.getElementById('graph-p-desc');
        if (n.excerpt) {
            descEl.textContent = n.excerpt;
            descEl.classList.add('has-content');
        } else {
            descEl.textContent = '';
            descEl.classList.remove('has-content');
        }

        const urlEl = document.getElementById('graph-p-url');
        if (n.url) { urlEl.innerHTML = `<a href="${n.url}" target="_blank">${n.url}</a>`; }
        else { urlEl.textContent = ''; }
        const pills = document.getElementById('graph-pills');
        pills.innerHTML = '';
        n.conns.forEach(c => {
            const p = document.createElement('div');
            p.className = 'graph-pill'; p.textContent = c.label;
            p.onclick = () => { focusOn(c); openPanel(c); };
            pills.appendChild(p);
        });
        panel.classList.add('open');
    }
    function closePanel() {
        active = null; searchMatch = null; panel.classList.remove('open');
        tZoom = 0.14; tCamX = 0; tCamY = 0;
        wakeLoop();
    }
    document.getElementById('graph-panel-close').onclick = closePanel;

    function focusOn(n) {
        following = n;
        tCamX = -n.x * tZoom; tCamY = -n.y * tZoom;
    }

    let dragging = false, lastMX = 0, lastMY = 0, downMX = 0, downMY = 0;
    let clickTimeout = null;

    canvas.addEventListener('mousedown', e => {
        dragging = true; lastMX = e.clientX; lastMY = e.clientY;
        downMX = e.clientX; downMY = e.clientY;
        wakeLoop();
    });

    canvas.addEventListener('mousemove', e => {
        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left, my = e.clientY - rect.top;
        if (dragging) {
            following = null;
            tCamX += e.clientX - lastMX; tCamY += e.clientY - lastMY;
            lastMX = e.clientX; lastMY = e.clientY;
            canvas.className = 'dragging';
            wakeLoop();
        } else {
            hover = nodeAt(mx, my);
            canvas.className = hover ? 'pointer' : '';
            wakeLoop();

            let tip = document.getElementById('graph-tooltip');
            if (!tip) {
                tip = document.createElement('div');
                tip.id = 'graph-tooltip';
                tip.style.position = 'fixed';
                tip.style.pointerEvents = 'none';
                tip.style.zIndex = '1000';
                tip.style.background = 'rgba(34,31,23,0.98)';
                tip.style.color = '#fff';
                tip.style.font = '600 14px var(--font-main, sans-serif)';
                tip.style.boxShadow = '0 4px 24px 0 rgba(0,0,0,0.34)';
                tip.style.borderRadius = '7px';
                tip.style.padding = '8px 13px 6px 13px';
                tip.style.maxWidth = '320px';
                tip.style.display = 'none';
                tip.style.whiteSpace = 'pre-line';
                document.body.appendChild(tip);
            }
            if (hover && hover.entranceStart && (Date.now() - hover.entranceStart - (hover.entranceDelay||0)) > 750) {
                tip.textContent = hover.label || '';
                if (hover.excerpt) {
                    tip.innerHTML = `<strong>${hover.label}</strong><br><span style='font-weight:400;opacity:0.86;'>${hover.excerpt.replace(/</g,'&lt;')}</span>`;
                }
                tip.style.left = (e.clientX + 15) + 'px';
                tip.style.top = (e.clientY + 14) + 'px';
                tip.style.display = 'block';
            } else {
                tip.style.display = 'none';
            }
        }
    });

    window.addEventListener('mouseup', e => {
        if (!dragging) return;
        dragging = false;
        canvas.className = '';
        wakeLoop();
        const moved = Math.abs(e.clientX - downMX) + Math.abs(e.clientY - downMY);
        if (moved < 5) {
            const rect = canvas.getBoundingClientRect();
            const n = nodeAt(e.clientX - rect.left, e.clientY - rect.top);
            if (n) {
                if (clickTimeout) {
                    clearTimeout(clickTimeout);
                    clickTimeout = null;
                    if (n.url) window.location.href = n.url;
                } else {
                    clickTimeout = setTimeout(() => {
                        clickTimeout = null;
                        openPanel(n);
                        tZoom = Math.max(tZoom, 0.8);
                        focusOn(n);
                        wakeLoop();
                    }, 250);
                }
            } else {
                closePanel();
            }
        }
    });

    let lastTouchDist = 0, touchDragging = false, lastTX = 0, lastTY = 0, downTX = 0, downTY = 0;
    canvas.addEventListener('touchstart', e => {
        e.preventDefault();
        wakeLoop();
        if (e.touches.length === 1) {
            touchDragging = true;
            lastTX = e.touches[0].clientX; lastTY = e.touches[0].clientY;
            downTX = lastTX; downTY = lastTY;
        } else if (e.touches.length === 2) {
            const dx = e.touches[0].clientX - e.touches[1].clientX;
            const dy = e.touches[0].clientY - e.touches[1].clientY;
            lastTouchDist = Math.sqrt(dx * dx + dy * dy);
        }
    }, { passive: false });

    canvas.addEventListener('touchmove', e => {
        e.preventDefault();
        wakeLoop();
        if (e.touches.length === 1 && touchDragging) {
            following = null;
            tCamX += e.touches[0].clientX - lastTX; tCamY += e.touches[0].clientY - lastTY;
            lastTX = e.touches[0].clientX; lastTY = e.touches[0].clientY;
        } else if (e.touches.length === 2) {
            following = null;
            const dx = e.touches[0].clientX - e.touches[1].clientX;
            const dy = e.touches[0].clientY - e.touches[1].clientY;
            const dist = Math.sqrt(dx * dx + dy * dy);
            tZoom = Math.max(0.1, Math.min(4, tZoom * (dist / lastTouchDist)));
            lastTouchDist = dist;
        }
    }, { passive: false });

    canvas.addEventListener('touchend', e => {
        wakeLoop();
        if (e.changedTouches.length === 1 && touchDragging) {
            touchDragging = false;
            const moved = Math.abs(e.changedTouches[0].clientX - downTX) + Math.abs(e.changedTouches[0].clientY - downTY);
            if (moved < 10) {
                const rect = canvas.getBoundingClientRect();
                const n = nodeAt(e.changedTouches[0].clientX - rect.left, e.changedTouches[0].clientY - rect.top);
                if (n) { openPanel(n); tZoom = Math.max(tZoom, 0.8); focusOn(n); wakeLoop(); }
                else closePanel();
            }
        }
    });

    canvas.addEventListener('wheel', e => {
        e.preventDefault();
        following = null;
        wakeLoop();
        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left, my = e.clientY - rect.top;
        const factor = e.deltaY > 0 ? 0.87 : 1.15;
        const newZoom = Math.max(0.1, Math.min(4, tZoom * factor));
        const [wx, wy] = toWorld(mx, my);
        tCamX = mx - W / 2 - wx * newZoom; tCamY = my - H / 2 - wy * newZoom;
        tZoom = newZoom;
    }, { passive: false });

    document.getElementById('graph-zi').onclick = () => { following = null; tZoom = Math.min(4, tZoom * 1.3); wakeLoop(); };
    document.getElementById('graph-zo').onclick = () => { following = null; tZoom = Math.max(0.1, tZoom * 0.77); wakeLoop(); };
    document.getElementById('graph-re').onclick = () => { following = null; tZoom = 0.25; tCamX = 0; tCamY = 0; closePanel(); wakeLoop(); };

    window.addEventListener('kosh:graph-focus', (e) => {
        handleGraphFocus(e.detail.link);
        wakeLoop();
    });

    window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            closePanel();
            return;
        }

        if (!e.key.startsWith('Arrow')) return;

        const root = nodes.find(n => n.group === 0);

        if (!active) {
            if (root) {
                e.preventDefault();
                openPanel(root);
                focusOn(root);
            }
            return;
        }

        e.preventDefault();
        const allTags = nodes.filter(n => n.group === 2).sort((a, b) => a.label.localeCompare(b.label));

        let next = null;

        if (active.group === 0) {
            if (e.key === 'ArrowUp' && allTags.length > 0) next = allTags[0];
        } else if (active.group === 2) {
            if (e.key === 'ArrowDown') {
                next = root;
            } else if (e.key === 'ArrowUp') {
                const items = active.conns.filter(c => c.group === 1);
                if (items.length > 0) next = items[0];
            } else {
                const idx = allTags.indexOf(active);
                if (e.key === 'ArrowRight') next = allTags[(idx + 1) % allTags.length];
                if (e.key === 'ArrowLeft') next = allTags[(idx - 1 + allTags.length) % allTags.length];
            }
        } else if (active.group === 1) {
            const parentTags = active.conns.filter(c => c.group === 2);
            const parentTag = parentTags[0];
            if (e.key === 'ArrowDown') {
                next = parentTag || root;
            } else if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
                if (parentTag) {
                    const siblings = parentTag.conns.filter(c => c.group === 1);
                    const idx = siblings.indexOf(active);
                    if (e.key === 'ArrowRight') next = siblings[(idx + 1) % siblings.length];
                    if (e.key === 'ArrowLeft') next = siblings[(idx - 1 + siblings.length) % siblings.length];
                }
            }
        }

        if (next) {
            openPanel(next);
            tZoom = Math.max(tZoom, 0.8);
            focusOn(next);
            wakeLoop();
        }
    });

})();
