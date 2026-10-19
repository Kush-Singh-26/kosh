package native

import (
	"log/slog"
	"sync"

	"github.com/fastschema/qjs"
)

// instance represents a single isolated renderer worker
type instance struct {
	rt            *qjs.Runtime
	ctx           *qjs.Context
	katex         *qjs.Value
	renderFn      *qjs.Value
	renderBatchFn *qjs.Value
	opts          *qjs.Value
	pele          *qjs.Value
	peleRenderFn  *qjs.Value
	peleOpts      *qjs.Value
	initOnce      sync.Once
}

var qjsInitMu sync.Mutex

func (i *instance) setupJSContext(rt *qjs.Runtime) (*qjs.Context, error) {
	ctx := rt.Context()
	_, err := ctx.Eval("init.js", qjs.Code(`
		var console = { log: function() {}, warn: function() {}, error: function() {} };
		var document = {
			createElement: function() { return { setAttribute: function() {}, style: {}, getContext: function() { return null; } }; }
		};
		var renderBatch = function(jsonInput) {
			var input = JSON.parse(jsonInput);
			return input.map(function(item) {
				try {
					var html = katex.renderToString(item.l, {
						displayMode: !!item.d,
						throwOnError: true,
						output: 'html'
					});
					return { h: html, e: null };
				} catch (err) {
					return { h: null, e: err.message };
				}
			});
		};
	`))
	return ctx, err
}

func (i *instance) loadKaTeX(ctx *qjs.Context, bytecode []byte) error {
	var err error
	if len(bytecode) > 0 {
		_, err = ctx.Eval("katex.min.js", qjs.Bytecode(bytecode))
	} else {
		_, err = ctx.Eval("katex.min.js", qjs.Code(katexJS))
	}
	return err
}

// loadPele loads the Pele Mermaid renderer into the JS context. The bundle
// declares `var pele` (it expects to run as a classic global script), so the
// source carries an explicit export to guarantee it lands on the global object
// regardless of how the script is evaluated.
func (i *instance) loadPele(ctx *qjs.Context, bytecode []byte) error {
	var err error
	if len(bytecode) > 0 {
		_, err = ctx.Eval("pele.global.js", qjs.Bytecode(bytecode))
	} else {
		_, err = ctx.Eval("pele.global.js", qjs.Code(peleJS+peleExportSuffix))
	}
	return err
}

func (i *instance) bindGlobalFunctions(ctx *qjs.Context) {
	// Note: ctx.Global() is cached and owned by the context; do not Free it here
	// or subsequent lookups would read a released handle.
	global := ctx.Global()

	katex := global.GetPropertyStr("katex")
	if katex == nil || !katex.IsObject() {
		slog.Warn("KaTeX not found in global scope")
		return
	}

	renderToString := katex.GetPropertyStr("renderToString")
	if renderToString == nil || !renderToString.IsFunction() {
		slog.Warn("katex.renderToString is not a function")
		return
	}

	renderBatch := global.GetPropertyStr("renderBatch")
	if renderBatch == nil || !renderBatch.IsFunction() {
		slog.Warn("renderBatch is not a function")
	}

	i.katex = katex
	i.renderFn = renderToString
	i.renderBatchFn = renderBatch
}

// bindPeleFunctions binds the Pele render entry point and its render options.
func (i *instance) bindPeleFunctions(ctx *qjs.Context) {
	// See bindGlobalFunctions: the cached global handle must not be freed.
	global := ctx.Global()

	pele := global.GetPropertyStr("pele")
	if pele == nil || !pele.IsObject() {
		slog.Warn("Pele not found in global scope")
		return
	}

	renderFn := pele.GetPropertyStr("render")
	if renderFn == nil || !renderFn.IsFunction() {
		slog.Warn("pele.render is not a function")
		pele.Free()
		return
	}

	opts := ctx.NewObject()
	opts.SetPropertyStr("fontSize", ctx.NewInt64(mermaidFontSize))
	opts.SetPropertyStr("padding", ctx.NewInt64(mermaidPadding))

	i.pele = pele
	i.peleRenderFn = renderFn
	i.peleOpts = opts
}

// ensureInitialized performs lazy initialization of the JS engine
func (i *instance) ensureInitialized(katexBytecode, peleBytecode []byte) {
	i.initOnce.Do(func() {
		qjsInitMu.Lock()
		defer qjsInitMu.Unlock()

		rt, err := qjs.New(qjs.Option{MaxExecutionTime: maxExecutionTimeMs})
		if err != nil {
			slog.Warn("Failed to create QJS runtime", "error", err)
			return
		}
		ctx, err := i.setupJSContext(rt)
		if err != nil {
			slog.Warn("Failed to initialize JS environment", "error", err)
		}

		if err := i.loadKaTeX(ctx, katexBytecode); err != nil {
			slog.Warn("Failed to load KaTeX", "error", err)
			return
		}

		i.bindGlobalFunctions(ctx)

		if err := i.loadPele(ctx, peleBytecode); err != nil {
			slog.Warn("Failed to load Pele", "error", err)
		} else {
			i.bindPeleFunctions(ctx)
		}

		i.rt = rt
		i.ctx = ctx

		opts := ctx.NewObject()
		opts.SetPropertyStr("throwOnError", ctx.NewBool(false))
		opts.SetPropertyStr("output", ctx.NewString("html"))
		i.opts = opts
	})
}
