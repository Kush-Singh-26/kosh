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

var qjsInitMu sync.Mutex

func (i *instance) setupJSContext(rt *qjs.Runtime) (*qjs.Context, error) {
	ctx := rt.Context()
	_, err := ctx.Eval("init.js", qjs.Code(`
		var console = { log: function() {}, warn: function() {}, error: function() {} };
		var document = {
			createElement: function() { return { setAttribute: function() {} }; }
		};
		var renderBatch = function(jsonInput) {
			var input = JSON.parse(jsonInput);
			return input.map(function(item) {
				try {
					return katex.renderToString(item.l, {
						displayMode: !!item.d,
						throwOnError: false,
						output: 'html'
					});
				} catch (err) {
					return "error: " + err.message;
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

func (i *instance) bindGlobalFunctions(ctx *qjs.Context) {
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

	i.katex = katex
	i.renderFn = renderToString
	i.renderBatchFn = renderBatch
}

// ensureInitialized performs lazy initialization of the JS engine
func (i *instance) ensureInitialized(bytecode []byte) {
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

		if err := i.loadKaTeX(ctx, bytecode); err != nil {
			slog.Warn("Failed to load KaTeX", "error", err)
			return
		}

		i.bindGlobalFunctions(ctx)
		i.rt = rt
		i.ctx = ctx

		opts := ctx.NewObject()
		opts.SetPropertyStr("throwOnError", ctx.NewBool(false))
		opts.SetPropertyStr("output", ctx.NewString("html"))
		i.opts = opts
	})
}
