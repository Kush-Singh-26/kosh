class WasmSim extends HTMLElement {
    connectedCallback() {
        const simName = this.getAttribute('src');
        let controls = [];
        try { controls = JSON.parse(this.getAttribute('controls') || "[]"); } catch (e) {}

        const prefix = `canvas_${simName}`;

        // FIX: Prevent title changes by C++ (GLFW) or Emscripten
        // We capture the title once and use a MutationObserver to revert unwanted changes.
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

        this.innerHTML = `
            <div style="background: #161b22; padding: 20px; border-radius: 8px; border: 1px solid #30363d; max-width: 900px; margin: 20px 0;">
                <div style="position: relative; width: 100%; height: 500px; background: #0d1117; border: 1px solid #30363d; margin-bottom: 20px; overflow: hidden;">
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

        factory({
            canvas: canvas,
            print: (text) => console.log(name + ": " + text),
            printErr: (text) => console.error(name + ": " + text),
            // We also provide an empty setStatus to prevent the "Running..." message
            setStatus: (text) => {},
        }).then(module => {
            let simInstance = module.getInstance ? module.getInstance() : null;

            if (simInstance && simInstance.initHelper) {
                const rect = canvas.getBoundingClientRect();
                const dpr = window.devicePixelRatio || 1;
                simInstance.initHelper(rect.width * dpr, rect.height * dpr, "#" + canvas.id);
            }

            const ui = this.querySelector(`#ui_${name}`);
            const setSimProp = (id, val) => {
                if (simInstance && simInstance[id] !== undefined) simInstance[id] = val;
            };

            controls.forEach(c => {
                const type = c.type || 'slider';
                const wrapper = document.createElement('div');
                
                if (type === 'button') {
                    wrapper.innerHTML = `<button class="sim-btn">${c.label}</button>`;
                    wrapper.querySelector('button').onclick = () => {
                        if (simInstance && typeof simInstance[c.id] === 'function') {
                            simInstance[c.id]();
                        }
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