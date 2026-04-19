package server

const devDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Kosh HUD</title>
    <style>
        :root {
            --bg: #1e1e2e;
            --surface: #313244;
            --text: #cdd6f4;
            --mauve: #cba6f7;
            --green: #a6e3a1;
            --red: #f38ba8;
            --blue: #89b4fa;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            background-color: var(--bg);
            color: var(--text);
            margin: 0;
            padding: 2rem;
            line-height: 1.6;
        }
        .container {
            max-width: 900px;
            margin: 0 auto;
        }
        h1 {
            color: var(--mauve);
            border-bottom: 2px solid var(--surface);
            padding-bottom: 0.5rem;
            margin-bottom: 2rem;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }
        .card {
            background-color: var(--surface);
            border-radius: 8px;
            padding: 1.5rem;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
        }
        .card h2 {
            margin-top: 0;
            font-size: 1.2rem;
            color: var(--blue);
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
            padding-bottom: 0.5rem;
        }
        .stat-row {
            display: flex;
            justify-content: space-between;
            margin-bottom: 0.5rem;
            font-size: 0.95rem;
        }
        .stat-label {
            color: #bac2de;
        }
        .stat-value {
            font-weight: 600;
        }
        .status-healthy { color: var(--green); }
        .status-warning { color: #f9e2af; }
        .status-error { color: var(--red); }
        
        pre {
            background: #181825;
            padding: 1rem;
            border-radius: 6px;
            overflow-x: auto;
            font-size: 0.85rem;
            color: #a6adc8;
        }
        #refresh {
            background-color: var(--mauve);
            color: #11111b;
            border: none;
            padding: 0.5rem 1rem;
            border-radius: 4px;
            font-weight: bold;
            cursor: pointer;
            margin-bottom: 1rem;
            transition: opacity 0.2s;
        }
        #refresh:hover {
            opacity: 0.9;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Kosh Development Dashboard</h1>
        <button id="refresh" onclick="fetchHealth()">Refresh Stats (auto updates every 5s)</button>
        <span id="last-updated" style="margin-left: 1rem; font-size: 0.85rem; color: #a6adc8;"></span>
        
        <div id="content">Loading dashboard metrics...</div>
    </div>

    <script>
        function formatBytes(bytes, decimals = 2) {
            if (!+bytes) return '0 Bytes';
            const k = 1024;
            const dm = decimals < 0 ? 0 : decimals;
            const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
        }
        
        function formatTime(ns) {
            if (ns < 1000) return ns + 'ns';
            if (ns < 1000000) return (ns / 1000).toFixed(2) + 'µs';
            if (ns < 1000000000) return (ns / 1000000).toFixed(2) + 'ms';
            return (ns / 1000000000).toFixed(2) + 's';
        }

        async function fetchHealth() {
            try {
                const res = await fetch('/api/health');
                const data = await res.json();
                
                const d = new Date();
                document.getElementById('last-updated').textContent = 'Last updated: ' + d.toLocaleTimeString();

                let html = '<div class="grid">';
                
                // Pipeline Stats
                html += '<div class="card"><h2>Pipeline Health</h2>';
                html += '<div class="stat-row"><span class="stat-label">Cache File</span><span class="stat-value">' + (data.cache_file_exists ? '<span class="status-healthy">OK</span>' : '<span class="status-warning">Missing</span>') + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">Cache Size</span><span class="stat-value">' + formatBytes(data.search_size) + '</span></div>'; // Using search_size as proxy for index
                html += '<div class="stat-row"><span class="stat-label">Total Errors</span><span class="stat-value ' + (data.errors > 0 ? 'status-error' : 'status-healthy') + '">' + (data.errors || 0) + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">Total Warnings</span><span class="stat-value ' + (data.warnings > 0 ? 'status-warning' : 'status-healthy') + '">' + (data.warnings || 0) + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">Asset Conversion</span><span class="stat-value">' + (data.asset_conversion_rate || 0).toFixed(1) + '%</span></div>';
                html += '</div>';

                // Features
                html += '<div class="card"><h2>Features</h2>';
                html += '<div class="stat-row"><span class="stat-label">Drafts</span><span class="stat-value">' + (data.has_drafts ? 'Yes' : 'No') + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">Math Render</span><span class="stat-value">' + (data.has_math ? 'Yes' : 'No') + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">Search Configured</span><span class="stat-value">' + (data.search_configured ? '<span class="status-healthy">Yes</span>' : 'No') + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">Search WASM Sync</span><span class="stat-value">' + (data.search_wasm_sync ? '<span class="status-healthy">OK</span>' : '<span class="status-warning">Out of sync</span>') + '</span></div>';
                html += '<div class="stat-row"><span class="stat-label">A11y Alt Text</span><span class="stat-value">' + (data.a11y_missing_alt_text > 0 ? '<span class="status-warning">'+data.a11y_missing_alt_text+' missing</span>' : '<span class="status-healthy">100% OK</span>') + '</span></div>';
                html += '</div>';

                html += '</div>'; // End grid

                // Messages
                if (data.messages && data.messages.length > 0) {
                    html += '<div class="card" style="margin-bottom: 2rem;"><h2>Recent Logs & Diagnostics</h2><pre>';
                    data.messages.slice(0, 15).forEach(msg => {
                        html += msg + '\n';
                    });
                    html += '</pre></div>';
                }

                document.getElementById('content').innerHTML = html;
            } catch (err) {
                document.getElementById('content').innerHTML = '<div class="card" style="border-left: 4px solid var(--red);">' +
                    '<h2 style="color: var(--red);">Error Connection Failed</h2>' +
                    '<p>Could not reach the Kosh development server. Make sure it is running.</p>' +
                    '<pre>' + err.toString() + '</pre>' +
                '</div>';
            }
        }

        // Initial fetch and set interval
        fetchHealth();
        setInterval(fetchHealth, 5000);
    </script>
</body>
</html>`
