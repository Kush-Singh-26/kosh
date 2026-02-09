const loading = document.getElementById('loading');
const stats = document.getElementById('stats');

// Helper to grab CSS variables
const getStyle = (name) => {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
};

// 1. SAFETY CHECK
if (typeof ForceGraph === 'undefined') {
    loading.innerHTML = '<span style="color: red; padding: 20px; display: block;">Error: force-graph.js not found.<br>Check static/js/ folder.</span>';
    throw new Error("ForceGraph library is missing. Did you download it?");
}

// State for colors
let colors = {
    post: getStyle('--accent-primary') || '#2997ff',
    tag: getStyle('--accent-cool') || '#64d2ff',
    text: getStyle('--text-main') || '#f5f5f7'
};

// Initialize ForceGraph
const Graph = ForceGraph()
    (document.getElementById('graph-container'))
    .backgroundColor('rgba(0,0,0,0)') 
    .width(window.innerWidth)
    .height(window.innerHeight)
    
    // DATA MAPPING
    .nodeId('id')           
    .nodeLabel('label')     
    .nodeVal('val')         
    .linkSource('source')   
    .linkTarget('target')   
    
    // VISUALS
    .nodeColor(node => node.group === 1 ? colors.post : colors.tag)
    .nodeRelSize(1)         
    .linkColor(() => 'rgba(150, 150, 150, 0.2)')
    .linkWidth(1)

    // --- NEW: Custom Label Rendering on Zoom ---
    .nodeCanvasObjectMode(() => 'after') // Draw labels ON TOP of nodes
    .nodeCanvasObject((node, ctx, globalScale) => {
        // 1. Only show labels if we are zoomed in (Threshold: 1.5x)
        if (globalScale < 1.5) return; 

        // 2. Configure Font
        // "4px" is the World Size. It will look like 4px at zoom 1x, 
        // but 12px at zoom 3x. This mimics the "growing text" of the old version.
        const label = node.label;
        ctx.font = '4px Sans-Serif'; 
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = colors.text; // Matches your theme (Light/Dark)

        // 3. Draw Text (Slightly below the node)
        // Node radius is ~10px, so y + 12 puts it just underneath.
        ctx.fillText(label, node.x, node.y + 12);
    })
    // -------------------------------------------
    
    // PERFORMANCE TUNING
    .warmupTicks(10)      
    .cooldownTicks(100)   
    
    // INTERACTION
    .onNodeClick(node => {
        if (node.url) {
            window.location.href = node.url;
        }
    });

// Load Data
async function loadGraph() {
    try {
        const baseUrl = window.GRAPH_BASE_URL || '';
        const response = await fetch(`${baseUrl}/graph.json`);
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const data = await response.json();

        Graph.graphData(data);

        // Hide Loading IMMEDIATELY
        loading.style.display = 'none';
        stats.style.display = 'flex';

        // Update Stats
        if (data.nodes) {
            const postCount = data.nodes.filter(n => n.group === 1).length;
            const tagCount = data.nodes.filter(n => n.group === 2).length;
            document.getElementById('postCount').innerText = postCount;
            document.getElementById('tagCount').innerText = tagCount;
            document.getElementById('linkCount').innerText = data.links.length;
        }

    } catch (err) {
        console.error("Could not load graph:", err);
        loading.innerHTML = `<span style="color: var(--accent-primary);">Failed to load data: ${err.message}</span>`;
    }
}

// Handle Window Resize
window.addEventListener('resize', () => {
    Graph.width(window.innerWidth);
    Graph.height(window.innerHeight);
});

// Handle Theme Switch
const observer = new MutationObserver((mutations) => {
    mutations.forEach((mutation) => {
        if (mutation.type === 'attributes' && 
           (mutation.attributeName === 'data-theme' || mutation.attributeName === 'class')) {
            colors.post = getStyle('--accent-primary');
            colors.tag = getStyle('--accent-cool');
            colors.text = getStyle('--text-main');
            Graph.nodeColor(node => node.group === 1 ? colors.post : colors.tag);
        }
    });
});
observer.observe(document.documentElement, { attributes: true });

loadGraph();