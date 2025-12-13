class WasmSim extends HTMLElement {
    connectedCallback() {
        const simName = this.getAttribute('src');
        let controls = [];
        try { controls = JSON.parse(this.getAttribute('controls') || "[]"); } catch (e) {}

        // 1. Define Unique IDs
        const uniqueSuffix = Math.random().toString(36).substr(2, 9);
        const prefix = `canvas_${simName}_${uniqueSuffix}`; 
        const uiId = `ui_${simName}_${uniqueSuffix}`;

        // 2. Render HTML with those IDs
        this.innerHTML = `
            <div style="background: #161b22; padding: 20px; border-radius: 8px; border: 1px solid #30363d; max-width: 900px; margin: 20px 0;">
                <div style="position: relative; width: 100%; height: 500px; background: #0d1117; border: 1px solid #30363d; margin-bottom: 20px; overflow: hidden;">
                    <canvas id="${prefix}" style="width: 100%; height: 100%; display: block;"></canvas>
                    
                    <div id="${prefix}_label_a" class="sim-label" style="color: white; opacity: 0;">A</div>
                    <div id="${prefix}_label_b" class="sim-label" style="color: #4ade80; opacity: 0;">B</div>
                    <div id="${prefix}_label_c" class="sim-label" style="color: #f87171; opacity: 0;">C</div>
                    <div id="${prefix}_label_proj" class="sim-label" style="color: #facc15; opacity: 0;">Proj</div>
                    <div id="${prefix}_label_i" class="sim-label" style="color: #4ade80; opacity: 0;">î</div>
                    <div id="${prefix}_label_j" class="sim-label" style="color: #f87171; opacity: 0;">ĵ</div>
                    <div id="${prefix}_label_v" class="sim-label" style="color: #facc15; opacity: 0;">v</div>
                </div>
                <div id="${uiId}" style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px;"></div>
            </div>
            <style>
                .sim-label {
                    position: absolute; top: 0; left: 0; 
                    font-family: sans-serif;
                    font-weight: bold; 
                    font-size: 16px; 
                    text-shadow: 0 0 4px black;
                    background: transparent;
                    transform: translate(-50%, -50%);
                    pointer-events: none;
                    transition: opacity 0.1s;
                }
            </style>
        `;

        // 3. Pass the IDs to the function
        this.initWasm(simName, controls, prefix, uiId);
    }

    // FIX: Add 'prefix' and 'uiId' to the arguments here!
    async initWasm(name, controls, prefix, uiId) {
        // Use the specific ID to be safe
        const canvas = this.querySelector(`#${prefix}`);
        
        const script = document.createElement('script');

        const baseUrl = window.siteBaseURL || "";
        script.src = `${baseUrl}/static/wasm/${name}.js`;
        
        script.onload = () => {
            const factory = window[`create_${name}`];
            factory({
                canvas: canvas,
            }).then(module => {
                const sim = new module.Simulation();
                
                const rect = canvas.getBoundingClientRect();
                const dpr = window.devicePixelRatio || 1;
                canvas.width = rect.width * dpr;
                canvas.height = rect.height * dpr;
                
                // NOW 'prefix' IS DEFINED
                sim.init(rect.width, rect.height, "#" + prefix);

                // Find the specific UI container
                const ui = this.querySelector(`#${uiId}`);
                
                controls.forEach(c => {
                    if (sim.hasOwnProperty(c.id) || sim[c.id] !== undefined) {
                        sim[c.id] = c.val;
                    }

                    const wrap = document.createElement('div');
                    wrap.innerHTML = `
                        <div style="color: #8b949e; font-size: 13px; margin-bottom: 6px;">${c.label}: <span id="val_${prefix}_${c.id}">${c.val}</span></div>
                        <input type="range" min="${c.min}" max="${c.max}" step="${c.step}" value="${c.val}" style="width: 100%;">
                    `;
                    
                    wrap.querySelector('input').addEventListener('input', (e) => {
                        const val = parseFloat(e.target.value);
                        if (sim[c.id] !== undefined) sim[c.id] = val; 
                        wrap.querySelector(`#val_${prefix}_${c.id}`).textContent = val.toFixed(2);
                    });
                    ui.appendChild(wrap);
                });

                let lastTime = performance.now();
                const loop = (currentTime) => {
                    if (!document.contains(canvas)) { sim.delete(); return; }
                    
                    const dt = (currentTime - lastTime) / 1000.0;
                    lastTime = currentTime;
                    
                    sim.update(Math.min(dt, 0.1));
                    sim.draw();

                    requestAnimationFrame(loop);
                };
                requestAnimationFrame(loop);
            });
        };
        document.body.appendChild(script);
    }
}
customElements.define('wasm-sim', WasmSim);