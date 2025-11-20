package contentstream

import (
	"fmt"
	"math"

	"pdflib/coords"
	"pdflib/ir/semantic"
)

// OpBBox represents the bounding box of an operation.
type OpBBox struct {
	OpIndex int
	Rect    semantic.Rectangle
}

// Tracer calculates the bounding boxes of operations in a content stream.
type Tracer struct {
}

func NewTracer() *Tracer {
	return &Tracer{}
}

// Trace executes the operations virtually and returns their bounding boxes.
func (t *Tracer) Trace(ops []semantic.Operation, resources *semantic.Resources) ([]OpBBox, error) {
	bboxes := make([]OpBBox, 0, len(ops))
	gs := &GraphicsState{
		CTM: coords.Identity(),
	}
	ts := &TextState{
		TextMatrix:     coords.Identity(),
		TextLineMatrix: coords.Identity(),
	}

	for i, op := range ops {
		var rect semantic.Rectangle
		var hasRect bool

		switch op.Operator {
		// Graphics State
		case "q":
			gs.Save()
		case "Q":
			if err := gs.Restore(); err != nil {
				return nil, err
			}
		case "cm":
			if len(op.Operands) == 6 {
				m := operandToMatrix(op.Operands)
				gs.CTM = m.Multiply(gs.CTM)
			}

		// Text Objects
		case "BT":
			ts.TextMatrix = coords.Identity()
			ts.TextLineMatrix = coords.Identity()
		case "ET":
			// End text object
		
		// Text State
		case "Tf":
			if len(op.Operands) == 2 {
				if name, ok := op.Operands[0].(semantic.NameOperand); ok {
					if font, ok := resources.Fonts[name.Value]; ok {
						ts.Font = font
					}
				}
				if size, ok := op.Operands[1].(semantic.NumberOperand); ok {
					ts.FontSize = size.Value
				}
			}
		case "Tm":
			if len(op.Operands) == 6 {
				ts.TextLineMatrix = operandToMatrix(op.Operands)
				ts.TextMatrix = ts.TextLineMatrix
			}
		case "Td":
			if len(op.Operands) == 2 {
				tx := operandToFloat(op.Operands[0])
				ty := operandToFloat(op.Operands[1])
				m := coords.Translate(tx, ty)
				ts.TextLineMatrix = m.Multiply(ts.TextLineMatrix)
				ts.TextMatrix = ts.TextLineMatrix
			}
		
		// Text Showing
		case "Tj":
			if len(op.Operands) == 1 {
				if str, ok := op.Operands[0].(semantic.StringOperand); ok {
					rect = calculateTextRect(str.Value, ts, gs)
					hasRect = true
				}
			}
		case "TJ":
			if len(op.Operands) == 1 {
				if arr, ok := op.Operands[0].(semantic.ArrayOperand); ok {
					rect = calculateTJRect(arr.Values, ts, gs)
					hasRect = true
				}
			}

		// Path Construction (Simplified: only 're')
		case "re":
			if len(op.Operands) == 4 {
				x := operandToFloat(op.Operands[0])
				y := operandToFloat(op.Operands[1])
				w := operandToFloat(op.Operands[2])
				h := operandToFloat(op.Operands[3])
				
				// Transform (x,y) and (x+w, y+h) to get bbox
				p1 := gs.CTM.Transform(coords.Point{X: x, Y: y})
				p2 := gs.CTM.Transform(coords.Point{X: x + w, Y: y})
				p3 := gs.CTM.Transform(coords.Point{X: x, Y: y + h})
				p4 := gs.CTM.Transform(coords.Point{X: x + w, Y: y + h})
				
				rect = pointsToRect(p1, p2, p3, p4)
				hasRect = true
			}
		
		// XObjects
		case "Do":
			if len(op.Operands) == 1 {
				if name, ok := op.Operands[0].(semantic.NameOperand); ok {
					if xobj, ok := resources.XObjects[name.Value]; ok {
						// Image/Form XObject is drawn in unit square 0,0 -> 1,1 transformed by CTM
						p1 := gs.CTM.Transform(coords.Point{X: 0, Y: 0})
						p2 := gs.CTM.Transform(coords.Point{X: 1, Y: 0})
						p3 := gs.CTM.Transform(coords.Point{X: 0, Y: 1})
						p4 := gs.CTM.Transform(coords.Point{X: 1, Y: 1})
						
						// If it's a Form XObject with BBox, we should use that instead of 0..1
						if xobj.Subtype == "Form" {
							// TODO: Handle Form XObject BBox and Matrix
						}

						rect = pointsToRect(p1, p2, p3, p4)
						hasRect = true
					}
				}
			}
		}

		if hasRect {
			bboxes = append(bboxes, OpBBox{OpIndex: i, Rect: rect})
		}
	}

	return bboxes, nil
}

func operandToMatrix(ops []semantic.Operand) coords.Matrix {
	return coords.Matrix{
		operandToFloat(ops[0]),
		operandToFloat(ops[1]),
		operandToFloat(ops[2]),
		operandToFloat(ops[3]),
		operandToFloat(ops[4]),
		operandToFloat(ops[5]),
	}
}

func operandToFloat(op semantic.Operand) float64 {
	if n, ok := op.(semantic.NumberOperand); ok {
		return n.Value
	}
	return 0
}

func calculateTextRect(text []byte, ts *TextState, gs *GraphicsState) semantic.Rectangle {
	if ts.Font == nil {
		return semantic.Rectangle{}
	}

	width := 0.0
	// Simple width calculation (ignoring encoding issues for now, assuming 1 byte = 1 char)
	// In reality, we need to use Font.Encoding and Font.Widths properly.
	for _, b := range text {
		if w, ok := ts.Font.Widths[int(b)]; ok {
			width += float64(w)
		} else {
			// Default width?
			width += 500 // Assume 500/1000 em
		}
	}
	
	// Convert width from glyph space (1000 units) to text space
	width = width / 1000.0 * ts.FontSize

	// Height is font size
	height := ts.FontSize

	// Text Matrix transforms text space to user space
	// Current point is (0,0) in text space (relative to Tm)
	// But wait, Td/Tm updates TextMatrix.
	// We need to apply TextMatrix then CTM.
	
	// Text space rect: (0, 0) to (width, height) (simplified, ignoring descent)
	// Actually, origin is usually baseline. So (0, descent) to (width, ascent).
	// Let's assume (0, 0) to (width, height) for simplicity of "covering" the text.
	
	// Combined Matrix: TextMatrix * CTM
	m := ts.TextMatrix.Multiply(gs.CTM)
	
	p1 := m.Transform(coords.Point{X: 0, Y: 0})
	p2 := m.Transform(coords.Point{X: width, Y: 0})
	p3 := m.Transform(coords.Point{X: 0, Y: height})
	p4 := m.Transform(coords.Point{X: width, Y: height})
	
	// Update TextMatrix for next glyphs (advance width)
	// T_m = T_m * Translate(width, 0) ? No, T_m is global.
	// The text matrix is updated by drawing text? No, only Td/Tm updates it.
	// But subsequent Tj calls? Usually they are separate.
	// If multiple strings in TJ, we handle them.
	
	return pointsToRect(p1, p2, p3, p4)
}

func calculateTJRect(ops []semantic.Operand, ts *TextState, gs *GraphicsState) semantic.Rectangle {
	// Similar to calculateTextRect but handles array of strings and numbers (kerning)
	// For now, simplified: just sum up widths.
	totalWidth := 0.0
	if ts.Font == nil {
		return semantic.Rectangle{}
	}

	for _, op := range ops {
		if str, ok := op.(semantic.StringOperand); ok {
			for _, b := range str.Value {
				if w, ok := ts.Font.Widths[int(b)]; ok {
					totalWidth += float64(w)
				} else {
					totalWidth += 500
				}
			}
		} else if num, ok := op.(semantic.NumberOperand); ok {
			// Kerning is in thousands of em, subtracted
			totalWidth -= num.Value
		}
	}
	
	width := totalWidth / 1000.0 * ts.FontSize
	height := ts.FontSize
	
	m := ts.TextMatrix.Multiply(gs.CTM)
	
	p1 := m.Transform(coords.Point{X: 0, Y: 0})
	p2 := m.Transform(coords.Point{X: width, Y: 0})
	p3 := m.Transform(coords.Point{X: 0, Y: height})
	p4 := m.Transform(coords.Point{X: width, Y: height})
	
	return pointsToRect(p1, p2, p3, p4)
}

func pointsToRect(points ...coords.Point) semantic.Rectangle {
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	
	for _, p := range points {
		if p.X < minX { minX = p.X }
		if p.Y < minY { minY = p.Y }
		if p.X > maxX { maxX = p.X }
		if p.Y > maxY { maxY = p.Y }
	}
	
	return semantic.Rectangle{LLX: minX, LLY: minY, URX: maxX, URY: maxY}
}
