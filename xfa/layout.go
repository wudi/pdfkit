package xfa

import (
	"context"
	"math"
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
	X, Y   float64
	Page   *semantic.Page
	Pages  *[]*semantic.Page
	Margin float64
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
		X:      36,
		Y:      792 - 36, // Start at top with margin
		Page:   page,
		Pages:  &pages,
		Margin: 36,
	}

	if form.Template != nil && form.Template.Subform != nil {
		e.renderSubform(lCtx, form.Template.Subform)
	}

	return pages, nil
}

func (e *LayoutEngineImpl) checkPageBreak(ctx *LayoutContext, height float64) {
	if ctx.Y-height < ctx.Margin {
		// New Page
		page := &semantic.Page{
			MediaBox:  semantic.Rectangle{LLX: 0, LLY: 0, URX: 612, URY: 792},
			Resources: ctx.Page.Resources, // Share resources
			Contents:  []semantic.ContentStream{{}},
		}
		*ctx.Pages = append(*ctx.Pages, page)
		ctx.Page = page
		ctx.Y = 792 - ctx.Margin
		// X is usually reset too, but depends on layout. Assuming flow layout resets X.
		// If we are in a positioned subform, this might be tricky.
		// For now, reset to margin.
		// ctx.X = ctx.Margin // Don't reset X blindly, caller handles X.
	}
}

func (e *LayoutEngineImpl) renderSubform(ctx *LayoutContext, subform *Subform) {
	// Handle positioning
	x := ParseUnit(subform.X)
	y := ParseUnit(subform.Y)
	w := ParseUnit(subform.W)
	h := ParseUnit(subform.H)

	// Save context state
	originalX := ctx.X
	originalY := ctx.Y

	// If absolute positioning is used, adjust context
	if subform.X != "" {
		ctx.X += x
	}
	if subform.Y != "" {
		ctx.Y -= y
	}

	// Render Draws
	for _, draw := range subform.Draws {
		drawH := e.measureDraw(&draw, w)
		if subform.Layout == "tb" {
			e.checkPageBreak(ctx, drawH)
		}
		e.renderDraw(ctx, &draw)
		if subform.Layout == "tb" {
			ctx.Y -= drawH
		}
	}

	// Render Fields
	for _, field := range subform.Fields {
		fieldH := e.measureField(&field, w)
		if subform.Layout == "tb" {
			e.checkPageBreak(ctx, fieldH)
		}
		e.renderField(ctx, &field)
		if subform.Layout == "tb" {
			ctx.Y -= fieldH
		}
	}

	// Render Child Subforms
	for _, child := range subform.Subforms {
		// Recursive call
		// If child is flow, it will consume space.
		// We need to know how much space it consumed?
		// The child modifies ctx.Y directly if it flows.
		e.renderSubform(ctx, &child)
	}
	
	// Restore context if not flow
	if subform.Layout != "tb" {
		ctx.X = originalX
		ctx.Y = originalY
	} else {
		// If flow, we keep the new Y (moved down)
		// But we restore X
		ctx.X = originalX
		
		// If explicit height was set, enforce it?
		if h > 0 {
			ctx.Y = originalY - h
		}
	}
}

func (e *LayoutEngineImpl) measureDraw(draw *Draw, containerWidth float64) float64 {
	h := ParseUnit(draw.H)
	if h > 0 {
		return h
	}
	if draw.Value != nil && draw.Value.Text != "" {
		_, textH := e.measureText(draw.Value.Text, ParseUnit(draw.W))
		return textH
	}
	return 14 // Default line height
}

func (e *LayoutEngineImpl) measureField(field *Field, containerWidth float64) float64 {
	h := ParseUnit(field.H)
	if h > 0 {
		return h
	}
	// Caption + Value
	// Assuming side-by-side or stacked?
	// Simple assumption: 1 line
	return 14
}

func (e *LayoutEngineImpl) measureText(text string, width float64) (float64, float64) {
	// Very rough estimation
	// fontSize := 12.0
	lineHeight := 14.0
	charWidth := 7.0 // Average char width for Helvetica 12

	if width <= 0 {
		width = float64(len(text)) * charWidth
		return width, lineHeight
	}

	charsPerLine := width / charWidth
	if charsPerLine < 1 {
		charsPerLine = 1
	}
	numLines := float64(len(text)) / charsPerLine
	if numLines < 1 {
		numLines = 1
	}
	return width, math.Ceil(numLines) * lineHeight
}

func (e *LayoutEngineImpl) renderDraw(ctx *LayoutContext, draw *Draw) {
	x := ParseUnit(draw.X)
	y := ParseUnit(draw.Y)

	// If flow, x/y might be 0 or relative.
	// If positioned, they are offsets.
	
	pdfX := ctx.X + x
	pdfY := ctx.Y - y // Y is current cursor

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
