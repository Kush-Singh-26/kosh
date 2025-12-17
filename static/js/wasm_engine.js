class WasmSim extends HTMLElement {
    connectedCallback() {
        const simName = this.getAttribute('src');
        let controls = [];
        try { controls = JSON.parse(this.getAttribute('controls') || "[]"); } catch (e) {}

        const prefix = `canvas_${simName}`;

        this.innerHTML = `
            <div style="background: #161b22; padding: 20px; border-radius: 8px; border: 1px solid #30363d; max-width: 900px; margin: 20px 0;">
                <div style="position: relative; width: 100%; height: 500px; background: #0d1117; border: 1px solid #30363d; margin-bottom: 20px; overflow: hidden;">
                    <canvas id="${prefix}" oncontextmenu="event.preventDefault()" style="width: 100%; height: 100%; display: block;"></canvas>
                    
                    <div id="${prefix}_label_a" class="sim-label" style="color: white; opacity: 0;">A</div>
                    <div id="${prefix}_label_b" class="sim-label" style="color: #4ade80; opacity: 0;">B</div>
                    <div id="${prefix}_label_c" class="sim-label" style="color: #f87171; opacity: 0;">C</div>
                    <div id="${prefix}_label_proj" class="sim-label" style="color: #facc15; opacity: 0;">Proj</div>
                    <div id="${prefix}_label_i" class="sim-label" style="color: #4ade80; opacity: 0;">î</div>
                    <div id="${prefix}_label_j" class="sim-label" style="color: #f87171; opacity: 0;">ĵ</div>
                    <div id="${prefix}_label_v" class="sim-label" style="color: #facc15; opacity: 0;">v</div>

                    <div id="${prefix}_target" class="sim-label" style="color: #facc15; opacity: 0;">Target</div>
                    <div id="${prefix}_result" class="sim-label" style="color: #4ade80; opacity: 0;">Result</div>
                    <div id="${prefix}_v" class="sim-label" style="color: #f87171; opacity: 0;">v</div>
                    <div id="${prefix}_w" class="sim-label" style="color: #38bdf8; opacity: 0;">w</div>
                </div>
                <div id="ui_${simName}" style="display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px;"></div>
            </div>
            <style>
                .sim-label {
                    position: absolute; top: 0; left: 0; 
                    font-family: sans-serif; font-weight: bold; font-size: 16px; 
                    text-shadow: 0 0 4px black; pointer-events: none;
                    transition: opacity 0.1s;
                    z-index: 10;
                }
            </style>
        `;

        this.initWasm(simName, controls);
    }

    async initWasm(name, controls) {
        const canvas = this.querySelector('canvas');
        
        if (!document.getElementById(`script_${name}`)) {
            const script = document.createElement('script');
            script.id = `script_${name}`;
            script.src = `static/wasm/${name}.js`;
            script.onload = () => { this.startSim(name, canvas, controls); };
            document.body.appendChild(script);
        } else {
            setTimeout(() => this.startSim(name, canvas, controls), 100);
        }
    }

    startSim(name, canvas, controls) {
        const factory = window[`create_${name}`];
        if (!factory) { console.error(`Factory create_${name} not found`); return; }

        // [FIX 1] Capture the correct title before the sim runs
        // We use a static property or fallback to ensure we don't capture "DotProduct" if a previous sim ran.
        const correctTitle = window.originalPageTitle || document.title;
        if (!window.originalPageTitle) window.originalPageTitle = correctTitle;

        const config = {
            canvas: canvas,
            print: (text) => console.log(name + ": " + text),
            printErr: (text) => console.error(name + ": " + text),
            
            // [FIX 2] Intercept Raylib's request to change the title
            setWindowTitle: (text) => {
                // console.log(`Blocked ${name} from changing title to: ${text}`);
            }
        };

        factory(config).then(module => {
            // [FIX 3] Force restore the title just in case Fix 2 failed
            if (document.title !== correctTitle) {
                document.title = correctTitle;
            }

            let simInstance = module.getInstance ? module.getInstance() : null;

            if (simInstance && simInstance.initHelper) {
                const rect = canvas.getBoundingClientRect();
                const dpr = window.devicePixelRatio || 1;
                simInstance.initHelper(rect.width * dpr, rect.height * dpr, "#" + canvas.id);
            }

            const ui = this.querySelector(`#ui_${name}`);
            const updateSimValue = (id, val) => {
                if (simInstance && simInstance[id] !== undefined) simInstance[id] = val;
            };

            controls.forEach(c => {
                updateSimValue(c.id, c.val);
                const wrap = document.createElement('div');
                wrap.innerHTML = `
                    <div style="color: #8b949e; font-size: 13px; margin-bottom: 6px;">${c.label}: <span id="val_${name}_${c.id}">${c.val}</span></div>
                    <input type="range" min="${c.min}" max="${c.max}" step="${c.step}" value="${c.val}" style="width: 100%;">
                `;
                wrap.querySelector('input').addEventListener('input', (e) => {
                    const val = parseFloat(e.target.value);
                    updateSimValue(c.id, val);
                    wrap.querySelector(`#val_${name}_${c.id}`).textContent = val.toFixed(2);
                });
                ui.appendChild(wrap);
            });
        });
    }
}
customElements.define('wasm-sim', WasmSim);