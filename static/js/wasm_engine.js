class WasmSim extends HTMLElement {
    connectedCallback() {
        // Use setTimeout to ensure child elements (like the config script) are fully parsed by the browser
        setTimeout(() => this.init(), 0);
    }

    init() {
        const simName = this.getAttribute('src');
        if (!simName) return;

        let controls = [];

        // STRATEGY 1: Try reading from child script tag (Recommended/Robust)
        const scriptEl = this.querySelector('script[type="application/json"]');
        if (scriptEl) {
            try {
                controls = JSON.parse(scriptEl.textContent);
            } catch (e) {
                console.error(`WasmSim [${simName}]: Error parsing JSON from script tag:`, e);
            }
        }

        // STRATEGY 2: Fallback to attribute (Legacy)
        if (controls.length === 0) {
            const attrControls = this.getAttribute('controls');
            if (attrControls) {
                try {
                    controls = JSON.parse(attrControls);
                } catch (e) {
                    console.error(`WasmSim [${simName}]: Error parsing 'controls' attribute. Check quotes and escaping.`, e);
                }
            }
        }

        const prefix = `canvas_${simName}`;

        // FIX: Prevent title changes by C++ (GLFW) or Emscripten
        if (!window.fixedTitle) {
            window.fixedTitle = document.title;
            const titleEl = document.querySelector('title');
            if (titleEl) {
                new MutationObserver(() => {
                    if (document.title !== window.fixedTitle) {
                        document.title = window.fixedTitle;
                    }
                }).observe(titleEl, { childList: true, subtree: true, characterData: true });
            }
        }

        // [PRESERVED] Fixed 800x600 size to avoid centering issues
        this.innerHTML = `
            <div style="background: #161b22; padding: 20px; border-radius: 8px; border: 1px solid #30363d; display: inline-block; margin: 20px 0;">
                <div style="position: relative; width: 800px; height: 600px; background: #0d1117; border: 1px solid #30363d; margin-bottom: 20px; overflow: hidden;">
                    <canvas id="${prefix}" oncontextmenu="event.preventDefault()" style="width: 100%; height: 100%; display: block;"></canvas>
                    
                    <div id="${prefix}_label_a" class="sim-label" style="color: white; opacity: 0;">A</div>
                    <div id="${prefix}_label_b" class="sim-label" style="color: #4ade80; opacity: 0;">B</div>
                    <div id="${prefix}_label_c" class="sim-label" style="color: #f87171; opacity: 0;">C</div>
                </div>
                <div id="ui_${simName}" style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; align-items: end;"></div>
            </div>
            <style>
                .sim-label {
                    position: absolute; top: 0; left: 0; 
                    font-family: sans-serif; font-weight: bold; font-size: 16px; 
                    text-shadow: 0 0 4px black; pointer-events: none;
                    transition: opacity 0.1s;
                    z-index: 10;
                }
                .sim-btn {
                    padding: 8px 16px; background: #238636; color: white; border: none; border-radius: 6px; cursor: pointer; width: 100%; font-weight: 600;
                }
                .sim-btn:hover { background: #2ea043; }
                .sim-btn:active { background: #238636; opacity: 0.8; }
            </style>
        `;

        // GLOBAL HELPER: Allow C++ to update UI
        window.updateSimControl = (simName, id, val) => {
            // 1. Update the Slider (Input)
            const input = document.getElementById(`input_${simName}_${id}`);
            if (input) input.value = val;

            // 2. Update the Text Label
            const label = document.getElementById(`val_${simName}_${id}`);
            if (label) label.textContent = val.toFixed(2);
        };

        this.initWasm(simName, controls);
    }

    async initWasm(name, controls) {
        const canvas = this.querySelector('canvas');
        // We only need to load the engine script ONCE.
        if (!document.getElementById(`script_engine`)) {
            const script = document.createElement('script');
            script.id = `script_engine`;
            script.src = `static/wasm/engine.js`;
            script.onload = () => { this.waitForEngine(name, canvas, controls); };
            document.body.appendChild(script);
        } else {
            this.waitForEngine(name, canvas, controls);
        }
    }

    waitForEngine(name, canvas, controls) {
        // Poll for the factory function
        if (window[`create_engine`]) {
            this.startSim(name, canvas, controls);
        } else {
            setTimeout(() => this.waitForEngine(name, canvas, controls), 50);
        }
    }

    startSim(name, canvas, controls) {
        // [PRESERVED] Use unified engine factory
        const factory = window[`create_engine`];
        if (!factory) { console.error(`Factory create_engine not found`); return; }

        factory({
            canvas: canvas,
            print: (text) => console.log(name + ": " + text),
            printErr: (text) => console.error(name + ": " + text),
            setStatus: (text) => { },
        }).then(module => {
            // [PRESERVED] Load the specific simulation via unified API
            const success = module.loadSim(name);
            if (!success) {
                console.error(`Failed to load simulation: ${name}`);
                return;
            }

            // [PRESERVED] Init Helper with fixed resolution
            const simInstance = module.getCurrentSim();
            if (simInstance && simInstance.initHelper) {
                // FORCE 800x600 to match original fixed behavior and avoid centering issues
                simInstance.initHelper(800, 600, "#" + canvas.id);
            }

            const ui = this.querySelector(`#ui_${name}`);

            // [PRESERVED] Generic setter helper using Module exports
            const setSimProp = (id, val) => {
                if (typeof val === 'boolean') {
                    module.setSimBool(id, val);
                } else if (typeof val === 'number') {
                    module.setSimFloat(id, val);
                }
            };

            // [PRESERVED] Action helper
            const callSimAction = (id) => {
                module.callSimAction(id);
            };

            controls.forEach(c => {
                const type = c.type || 'slider';
                const wrapper = document.createElement('div');

                if (type === 'button') {
                    wrapper.innerHTML = `<button class="sim-btn">${c.label}</button>`;
                    wrapper.querySelector('button').onclick = () => {
                        callSimAction(c.id);
                    };
                } else if (type === 'checkbox') {
                    setSimProp(c.id, !!c.val);
                    wrapper.style.display = "flex";
                    wrapper.style.alignItems = "center";
                    wrapper.style.height = "100%";
                    wrapper.innerHTML = `
                        <input type="checkbox" id="chk_${name}_${c.id}" ${c.val ? 'checked' : ''} style="margin-right: 10px; transform: scale(1.2);">
                        <label for="chk_${name}_${c.id}" style="color: #c9d1d9; cursor: pointer;">${c.label}</label>
                    `;
                    wrapper.querySelector('input').onchange = (e) => setSimProp(c.id, e.target.checked);
                } else {
                    setSimProp(c.id, c.val);
                    wrapper.innerHTML = `
                        <div style="color: #8b949e; font-size: 13px; margin-bottom: 6px;">${c.label}: <span id="val_${name}_${c.id}">${c.val}</span></div>
                        <input id="input_${name}_${c.id}" type="range" min="${c.min}" max="${c.max}" step="${c.step}" value="${c.val}" style="width: 100%;">
                    `;
                    wrapper.querySelector('input').addEventListener('input', (e) => {
                        const val = parseFloat(e.target.value);
                        setSimProp(c.id, val);
                        wrapper.querySelector(`#val_${name}_${c.id}`).textContent = val.toFixed(2);
                    });
                }
                ui.appendChild(wrapper);
            });
        });
    }
}
customElements.define('wasm-sim', WasmSim);