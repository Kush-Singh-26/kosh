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
	initOnce      sync.Once
}

// ensureInitialized performs lazy initialization of the JS engine
func (i *instance) ensureInitialized(bytecode []byte) {
	i.initOnce.Do(func() {
		rt, err := qjs.New(qjs.Option{
			MaxExecutionTime: 2000, // 2s safety timeout per task
		})
		if err != nil {
			slog.Warn("Failed to create QJS runtime", "error", err)
			return
		}
		ctx := rt.Context()

		// Provide minimal browser environment for KaTeX
		_, err = ctx.Eval("init.js", qjs.Code(`
			var console = {
				log: function() {},
				warn: function() {},
				error: function() {}
			};
			var document = {
				createElement: function() {
					return {
						setAttribute: function() {}
					};
				}
			};
			var renderBatch = function(latexs, modes) {
				return latexs.map(function(latex, i) {
					try {
						return katex.renderToString(latex, {
							displayMode: !!modes[i],
							throwOnError: false,
							output: 'html'
						});
					} catch (err) {
						return "error: " + err.message;
					}
				});
			};
		`))
		if err != nil {
			slog.Warn("Failed to initialize JS environment", "error", err)
		}

		// Load KaTeX (Use bytecode if available, fallback to eval)
		if len(bytecode) > 0 {
			_, err = ctx.Eval("katex.min.js", qjs.Bytecode(bytecode))
		} else {
			_, err = ctx.Eval("katex.min.js", qjs.Code(katexJS))
		}

		if err != nil {
			slog.Warn("Failed to load KaTeX", "error", err)
			return
		}

		global := ctx.Global()
		defer global.Free()

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

		i.rt = rt
		i.ctx = ctx
		i.katex = katex
		i.renderFn = renderToString
		i.renderBatchFn = renderBatch

		// Pre-create options object
		opts := ctx.NewObject()
		opts.SetPropertyStr("throwOnError", ctx.NewBool(false))
		opts.SetPropertyStr("output", ctx.NewString("html"))
		i.opts = opts
	})
}
