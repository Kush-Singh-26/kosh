const canvas = document.getElementById('graphCanvas');
const ctx = canvas.getContext('2d');
const label = document.getElementById('node-label');
const loading = document.getElementById('loading');
const stats = document.getElementById('stats');

let width, height;
let nodes = [];
let links = [];

// Camera State
let transform = { x: 0, y: 0, k: 0.75 }; 

// Interaction State
let draggedNode = null;
let hoveredNode = null;
let isPanning = false;
let startPanX, startPanY;
let dragStartTime = 0;
let dragStartPos = {x:0, y:0};

// Simulation State
let alpha = 1.0; 

// Helper to grab CSS variables
const getStyle = (name) => {
    return getComputedStyle(document.body).getPropertyValue(name).trim();
};

// Config object - Colors are now dynamic placeholders
const CONFIG = {
    repulsion: 3000,
    stiffness: 0.05,
    friction: 0.88,
    nodeRadius: 7,
    linkDistance: 120,
    colors: {
        post: '',       // Will map to --accent-primary
        tag: '',        // Will map to --accent-cool
        link: '',       // Will map to --border
        linkHover: '',  // Will map to --accent-primary
        highlight: '',  // Will map to --h1-color
        text: '',       // Will map to --text-main
        glow: ''        // Will map to --accent-primary
    }
};

// Function to update colors based on current CSS variables
function updateThemeColors() {
    CONFIG.colors.post = getStyle('--accent-primary') || '#2997ff';
    CONFIG.colors.tag = getStyle('--accent-cool') || '#64d2ff';
    CONFIG.colors.link = 'rgba(255,255,255,0.6)';
    CONFIG.colors.linkHover = getStyle('--accent-primary') || '#2997ff';
    CONFIG.colors.highlight = getStyle('--h1-color') || '#ffffff';
    CONFIG.colors.text = getStyle('--text-main') || '#f5f5f7';
    CONFIG.colors.glow = getStyle('--accent-primary') || '#2997ff';
}

async function loadGraph() {
    try {
        // Use the global variable defined in the HTML
        const baseUrl = window.GRAPH_BASE_URL || ''; 
        const response = await fetch(`${baseUrl}/graph.json`);
        const data = await response.json();
        init(data);
    } catch (err) {
        console.error("Could not load graph:", err);
        loading.innerHTML = '<span style="color: var(--accent-primary);">Failed to load graph</span>';
    }
}

function init(data) {
    updateThemeColors(); // Initial color fetch
    resize();
    
    nodes = data.nodes.map(n => ({
        ...n,
        x: (Math.random() - 0.5) * 600, 
        y: (Math.random() - 0.5) * 600,
        vx: 0, vy: 0,
        fx: null, fy: null 
    }));
    
    links = data.links.map(l => ({
        source: nodes.find(n => n.id === l.source),
        target: nodes.find(n => n.id === l.target)
    })).filter(l => l.source && l.target);

    // Update stats
    const postCount = nodes.filter(n => n.group === 1).length;
    const tagCount = nodes.filter(n => n.group === 2).length;
    document.getElementById('postCount').textContent = postCount;
    document.getElementById('tagCount').textContent = tagCount;
    document.getElementById('linkCount').textContent = links.length;
    
    // Hide loading, show stats
    loading.style.display = 'none';
    stats.style.display = 'block';

    requestAnimationFrame(loop);
}

function updatePhysics() {
    if (alpha < 0.001) return;

    // Repulsion
    for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
            const dx = nodes[i].x - nodes[j].x;
            const dy = nodes[i].y - nodes[j].y;
            let distSq = dx * dx + dy * dy;
            if (distSq === 0) { distSq = 1; }
            
            const force = (CONFIG.repulsion / distSq) * alpha;
            const fx = (dx / Math.sqrt(distSq)) * force;
            const fy = (dy / Math.sqrt(distSq)) * force;

            if (!nodes[i].fx) { nodes[i].vx += fx; nodes[i].vy += fy; }
            if (!nodes[j].fx) { nodes[j].vx -= fx; nodes[j].vy -= fy; }
        }
    }

    // Springs
    links.forEach(link => {
        const dx = link.target.x - link.source.x;
        const dy = link.target.y - link.source.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        
        const force = (dist - CONFIG.linkDistance) * CONFIG.stiffness * alpha;
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;

        if (!link.source.fx) { link.source.vx += fx; link.source.vy += fy; }
        if (!link.target.fx) { link.target.vx -= fx; link.target.vy -= fy; }
    });

    // Gravity & Integration
    nodes.forEach(n => {
        if (n.fx !== null) {
            n.x = n.fx;
            n.y = n.fy;
            n.vx = 0; n.vy = 0;
            return;
        }

        n.vx -= n.x * 0.0008 * alpha;
        n.vy -= n.y * 0.0008 * alpha;

        n.x += n.vx;
        n.y += n.vy;

        n.vx *= CONFIG.friction;
        n.vy *= CONFIG.friction;
    });

    alpha *= 0.995;
}

function draw() {
    ctx.clearRect(0, 0, width, height);
    ctx.save();
    
    ctx.translate(width / 2 + transform.x, height / 2 + transform.y);
    ctx.scale(transform.k, transform.k);

    // Draw Links
    links.forEach(l => {
        const isConnected = (l.source === hoveredNode || l.target === hoveredNode || 
                             l.source === draggedNode || l.target === draggedNode);
        
        ctx.strokeStyle = isConnected ? CONFIG.colors.linkHover : CONFIG.colors.link;
        ctx.lineWidth = (isConnected ? 2 : 1) / transform.k;
        ctx.globalAlpha = isConnected ? 0.8 : 0.6; // Increased base opacity slightly for visibility
        
        ctx.beginPath();
        ctx.moveTo(l.source.x, l.source.y);
        ctx.lineTo(l.target.x, l.target.y);
        ctx.stroke();
    });
    ctx.globalAlpha = 1;

    // Draw Nodes
    nodes.forEach(n => {
        const isActive = n === hoveredNode || n.fx !== null;
        // Group 1: Posts (Primary), Group 2: Tags (Secondary/Cool)
        let color = n.group === 2 ? CONFIG.colors.tag : CONFIG.colors.post;
        const r = (n.val || CONFIG.nodeRadius) * (isActive ? 1.3 : 1);

        // Glow effect for active nodes
        if (isActive) {
            ctx.shadowBlur = 20;
            ctx.shadowColor = CONFIG.colors.glow;
        }

        // Node circle
        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.arc(n.x, n.y, r, 0, Math.PI * 2);
        ctx.fill();

        // Inner highlight
        if (isActive) {
            ctx.fillStyle = CONFIG.colors.highlight;
            ctx.beginPath();
            ctx.arc(n.x, n.y, r * 0.5, 0, Math.PI * 2);
            ctx.fill();
        }

        ctx.shadowBlur = 0;

        // Labels
        if (transform.k > 1.0 || isActive) {
            ctx.fillStyle = CONFIG.colors.text;
            ctx.font = `${isActive ? 'bold ' : ''}13px Inter, sans-serif`;
            
            // Subtle shadow for text legibility
            ctx.shadowBlur = 4;
            ctx.shadowColor = getStyle('--bg-body'); 
            
            const textWidth = ctx.measureText(n.label).width;
            ctx.fillText(n.label, n.x - textWidth / 2, n.y - r - 8);
            ctx.shadowBlur = 0;
        }
    });

    ctx.restore();
}

function toWorld(sx, sy) {
    const rect = canvas.getBoundingClientRect();
    return {
        x: (sx - rect.left - width / 2 - transform.x) / transform.k,
        y: (sy - rect.top - height / 2 - transform.y) / transform.k
    };
}

canvas.addEventListener('pointerdown', e => {
    const wPos = toWorld(e.clientX, e.clientY);
    
    const hitNode = nodes.find(n => {
        const r = (n.val || CONFIG.nodeRadius) * 1.5;
        return Math.hypot(n.x - wPos.x, n.y - wPos.y) < r;
    });

    if (hitNode) {
        draggedNode = hitNode;
        draggedNode.fx = draggedNode.x;
        draggedNode.fy = draggedNode.y;
        alpha = 1.0; 
        requestAnimationFrame(loop);
        
        dragStartTime = Date.now();
        dragStartPos = { x: e.clientX, y: e.clientY };
        canvas.style.cursor = 'grabbing';
    } else {
        isPanning = true;
        startPanX = e.clientX;
        startPanY = e.clientY;
        canvas.style.cursor = 'move';
    }
});

window.addEventListener('pointermove', e => {
    const wPos = toWorld(e.clientX, e.clientY);

    if (draggedNode) {
        draggedNode.fx = wPos.x;
        draggedNode.fy = wPos.y;
        alpha = 0.5;
        return;
    }

    if (isPanning) {
        transform.x += e.clientX - startPanX;
        transform.y += e.clientY - startPanY;
        startPanX = e.clientX;
        startPanY = e.clientY;
        return;
    }

    const prevHover = hoveredNode;
    hoveredNode = nodes.find(n => {
        const r = (n.val || CONFIG.nodeRadius) * 1.5;
        return Math.hypot(n.x - wPos.x, n.y - wPos.y) < r;
    });

    if (hoveredNode !== prevHover) {
        canvas.style.cursor = hoveredNode ? 'pointer' : 'grab';
    }

    if (hoveredNode) {
        label.style.display = 'block';
        label.style.left = e.clientX + 'px';
        label.style.top = e.clientY + 'px';
        label.innerText = hoveredNode.label;
    } else {
        label.style.display = 'none';
    }
});

window.addEventListener('pointerup', e => {
    if (draggedNode) {
        const dist = Math.hypot(e.clientX - dragStartPos.x, e.clientY - dragStartPos.y);
        const timeDiff = Date.now() - dragStartTime;

        if (dist < 5 && timeDiff < 300 && draggedNode.url) {
            window.location.href = draggedNode.url;
        }

        draggedNode.fx = null;
        draggedNode.fy = null;
        draggedNode = null;
        alpha = 0.5;
    }

    isPanning = false;
    canvas.style.cursor = 'grab';
});

canvas.addEventListener('wheel', e => {
    e.preventDefault();
    const zoomIntensity = 0.001;
    const delta = 1 - e.deltaY * zoomIntensity;
    const newK = Math.max(0.2, Math.min(4, transform.k * delta));
    transform.k = newK;
}, { passive: false });

function resize() {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
    width = canvas.width;
    height = canvas.height;
    updateThemeColors(); // Update colors on resize (helps with layout shifts)
}

window.addEventListener('resize', resize);

// MutationObserver to detect Theme Switch (Light/Dark mode)
// Watches the <body> for class or attribute changes
const observer = new MutationObserver((mutations) => {
    mutations.forEach((mutation) => {
        if (mutation.type === 'attributes' && 
           (mutation.attributeName === 'data-theme' || mutation.attributeName === 'class')) {
            updateThemeColors();
            // Wake up simulation briefly to redraw with new colors
            if (alpha < 0.1) alpha = 0.1; 
            requestAnimationFrame(loop);
        }
    });
});
observer.observe(document.body, { attributes: true });

function loop() {
    updatePhysics();
    draw();
    if (alpha > 0.01 || draggedNode || isPanning) {
        requestAnimationFrame(loop);
    }
}

loadGraph();