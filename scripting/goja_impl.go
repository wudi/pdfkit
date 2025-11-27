package scripting

import (
	"context"

	"github.com/dop251/goja"
)

type GojaEngine struct {
	vm *goja.Runtime
}

func NewEngine() *GojaEngine {
	vm := goja.New()
	return &GojaEngine{vm: vm}
}

func (e *GojaEngine) Execute(ctx context.Context, script string) (interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	defer close(done)
	defer e.vm.ClearInterrupt()

	go func() {
		select {
		case <-ctx.Done():
			e.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()

	val, err := e.vm.RunString(script)
	if err != nil {
		if interruptedErr, ok := err.(*goja.InterruptedError); ok {
			if cause := interruptedErr.Unwrap(); cause != nil {
				return nil, cause
			}
			return nil, context.Canceled
		}
		return nil, err
	}
	return val.Export(), nil
}

func (e *GojaEngine) RegisterDOM(dom PDFDOM) error {
	// Expose 'app' object
	appObj := e.vm.NewObject()
	err := appObj.Set("alert", func(call goja.FunctionCall) goja.Value {
		msg := ""
		if len(call.Arguments) > 0 {
			msg = call.Arguments[0].String()
		}
		dom.Alert(msg)
		return goja.Undefined()
	})
	if err != nil {
		return err
	}
	e.vm.Set("app", appObj)

	// Expose Doc methods globally (as if 'this' is the Doc)
	e.vm.Set("getField", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		name := call.Arguments[0].String()
		field, err := dom.GetField(name)
		if err != nil || field == nil {
			return goja.Null()
		}

		// Create JS object for field with 'value' property
		obj := e.vm.NewObject()
		obj.DefineAccessorProperty("value",
			e.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				return e.vm.ToValue(field.GetValue())
			}),
			e.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) > 0 {
					field.SetValue(call.Arguments[0].Export())
				}
				return goja.Undefined()
			}),
			goja.FLAG_TRUE, // Configurable
			goja.FLAG_TRUE, // Enumerable
		)

		return obj
	})

	e.vm.Set("getPage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		idx := int(call.Arguments[0].ToInteger())
		page, err := dom.GetPage(idx)
		if err != nil || page == nil {
			return goja.Null()
		}
		return e.vm.ToValue(&pageProxyWrapper{p: page})
	})

	return nil
}

type pageProxyWrapper struct {
	p PageProxy
}

func (p *pageProxyWrapper) GetIndex() int {
	return p.p.GetIndex()
}
