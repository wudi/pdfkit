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
	// TODO: Handle context cancellation using Interrupt
	val, err := e.vm.RunString(script)
	if err != nil {
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
		return e.vm.ToValue(&fieldProxyWrapper{p: field})
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

type fieldProxyWrapper struct {
	p FormFieldProxy
}

// Goja will map "Value" field to "value" property if we configure it,
// but here we use methods or we need to use a custom object with getters/setters.
// For simplicity, we expose GetValue/SetValue as methods first,
// or we can try to simulate properties.
// Acrobat JS uses `f.value`.
// To support `f.value`, we can use `DefineDataProperty` or similar if we create an Object.
// But mapping a Go struct to have getters/setters for fields is tricky in goja without using `DefineProperty` in JS.

// Let's just expose methods for now to satisfy the "Engine" requirement,
// and maybe add a TODO for full Acrobat API compliance.
func (f *fieldProxyWrapper) GetValue() interface{} {
	return f.p.GetValue()
}

func (f *fieldProxyWrapper) SetValue(v interface{}) {
	f.p.SetValue(v)
}

type pageProxyWrapper struct {
	p PageProxy
}

func (p *pageProxyWrapper) GetIndex() int {
	return p.p.GetIndex()
}
