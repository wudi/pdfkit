package xfa

import (
	"context"
	"pdflib/ir/semantic"
)

// LayoutEngine renders an XFA Form into PDF pages.
type LayoutEngine interface {
	Render(ctx context.Context, form *Form) ([]*semantic.Page, error)
}

type LayoutEngineImpl struct{}

func NewLayoutEngine() *LayoutEngineImpl {
	return &LayoutEngineImpl{}
}

type LayoutContext struct {
	X, Y float64
	Page *semantic.Page
}

func (e *LayoutEngineImpl) Render(ctx context.Context, form *Form) ([]*semantic.Page, error) {
	// 1. Bind Data
	binder := NewBinder(form)
	binder.Bind()

	// 2. Layout
	pages := []*semantic.Page{}
	
	// Create Resources with a default font
	resources := &semantic.Resources{
		Fonts: map[string]*semantic.Font{
			"F1": {
				Subtype:  "Type1",
				BaseFont: "Helvetica",
			},
		},
	}

	page := &semantic.Page{
		MediaBox:  semantic.Rectangle{LLX: 0, LLY: 0, URX: 612, URY: 792},
		Resources: resources,
		Contents:  []semantic.ContentStream{{}},
	}
	pages = append(pages, page)

	lCtx := &LayoutContext{
		X:    0,
		Y:    792, // Start at top
		Page: page,
	}

	if form.Template != nil && form.Template.Subform != nil {
		e.renderSubform(lCtx, form.Template.Subform)
	}

	return pages, nil
}

func (e *LayoutEngineImpl) renderSubform(ctx *LayoutContext, subform *Subform) {
	// Handle positioning
	x := ParseUnit(subform.X)
	y := ParseUnit(subform.Y)
	// w := ParseUnit(subform.W)
	h := ParseUnit(subform.H)

	// Save context state if needed (e.g. for relative positioning)
	originalX := ctx.X
	originalY := ctx.Y

	// If absolute positioning is used, adjust context
	if subform.X != "" {
		ctx.X += x
	}
	if subform.Y != "" {
		ctx.Y -= y // Y is down in XFA, so we subtract from PDF Y
	}

	// Render Draws
	for _, draw := range subform.Draws {
		e.renderDraw(ctx, &draw)
	}

	// Render Fields
	for _, field := range subform.Fields {
		e.renderField(ctx, &field)
	}

	// Render Child Subforms
	for _, child := range subform.Subforms {
		e.renderSubform(ctx, &child)
	}
	
	// Restore context or update flow
	if subform.Layout == "tb" {
		// If flow, we move Y down by height
		// If height is auto, we'd need to calculate it. 
		// For now, use explicit height or default.
		if h > 0 {
			ctx.Y = originalY - h
		} else {
			// Simple flow assumption: move down by some amount if no height
			// This is very naive.
		}
		ctx.X = originalX
	} else {
		// If positioned, restore context
		ctx.X = originalX
		ctx.Y = originalY
	}
}

func (e *LayoutEngineImpl) renderDraw(ctx *LayoutContext, draw *Draw) {
	x := ParseUnit(draw.X)
	y := ParseUnit(draw.Y)

	pdfX := ctx.X + x
	pdfY := ctx.Y - y

	if draw.Value != nil && draw.Value.Text != "" {
		e.addText(ctx.Page, pdfX, pdfY, draw.Value.Text)
	}
}

func (e *LayoutEngineImpl) renderField(ctx *LayoutContext, field *Field) {
	x := ParseUnit(field.X)
	y := ParseUnit(field.Y)

	pdfX := ctx.X + x
	pdfY := ctx.Y - y

	// Render Caption
	if field.Caption != nil && field.Caption.Value != nil && field.Caption.Value.Text != "" {
		e.addText(ctx.Page, pdfX, pdfY, field.Caption.Value.Text)
		// Offset value?
		pdfX += 50 // Arbitrary offset for value
	}

	if field.Value != nil && field.Value.Text != "" {
		e.addText(ctx.Page, pdfX, pdfY, field.Value.Text)
	}
}

func (e *LayoutEngineImpl) addText(page *semantic.Page, x, y float64, text string) {
	ops := []semantic.Operation{
		{Operator: "BT", Operands: nil},
		{Operator: "Tf", Operands: []semantic.Operand{
			semantic.NameOperand{Value: "F1"},
			semantic.NumberOperand{Value: 12},
		}},
		{Operator: "Td", Operands: []semantic.Operand{
			semantic.NumberOperand{Value: x},
			semantic.NumberOperand{Value: y},
		}},
		{Operator: "Tj", Operands: []semantic.Operand{
			semantic.StringOperand{Value: []byte(text)},
		}},
		{Operator: "ET", Operands: nil},
	}
	
	// Append to the last content stream
	if len(page.Contents) > 0 {
		page.Contents[len(page.Contents)-1].Operations = append(page.Contents[len(page.Contents)-1].Operations, ops...)
	} else {
		page.Contents = append(page.Contents, semantic.ContentStream{Operations: ops})
	}
}
