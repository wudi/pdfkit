package contentstream

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"pdflib/coords"
	"pdflib/ir/semantic"
)

type Processor interface {
	Process(ctx Context, stream []byte, state *GraphicsState) error
	RegisterHandler(op string, h OperatorHandler)
}

type OperatorHandler interface {
	Handle(ctx *ExecutionContext, operands []semantic.Operand) error
}

type ExecutionContext struct {
	GraphicsState *GraphicsState
	TextState     *TextState
	Resources     *semantic.Resources
}

type GraphicsState struct {
	CTM       coords.Matrix
	LineWidth float64
	stack     []*GraphicsState
}

func (gs *GraphicsState) Save() { clone := *gs; gs.stack = append(gs.stack, &clone) }
func (gs *GraphicsState) Restore() error {
	n := len(gs.stack)
	if n == 0 {
		return errors.New("state stack empty")
	}
	*gs = *gs.stack[n-1]
	gs.stack = gs.stack[:n-1]
	return nil
}

type TextState struct {
	Font           *semantic.Font
	FontSize       float64
	TextMatrix     coords.Matrix
	TextLineMatrix coords.Matrix
}

type simpleProcessor struct{ handlers map[string]OperatorHandler }

func NewProcessor() Processor                                           { return &simpleProcessor{handlers: make(map[string]OperatorHandler)} }
func (p *simpleProcessor) RegisterHandler(op string, h OperatorHandler) { p.handlers[op] = h }
func (p *simpleProcessor) Process(ctx Context, stream []byte, state *GraphicsState) error {
	tokens := tokenize(string(stream))
	ec := &ExecutionContext{GraphicsState: state, TextState: &TextState{}}
	opStack := []semantic.Operand{}

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if h, ok := p.handlers[tok]; ok {
			if err := h.Handle(ec, opStack); err != nil {
				return err
			}
			opStack = opStack[:0]
			continue
		}
		// treat as operand
		if num, err := strconv.ParseFloat(tok, 64); err == nil {
			opStack = append(opStack, semantic.NumberOperand{Value: num})
			continue
		}
		if strings.HasPrefix(tok, "/") {
			opStack = append(opStack, semantic.NameOperand{Value: strings.TrimPrefix(tok, "/")})
			continue
		}
		if strings.HasPrefix(tok, "(") && strings.HasSuffix(tok, ")") {
			opStack = append(opStack, semantic.StringOperand{Value: []byte(tok[1 : len(tok)-1])})
			continue
		}
		// ignore unknown tokens
	}

	if len(opStack) > 0 {
		return fmt.Errorf("dangling operands: %d", len(opStack))
	}
	return nil
}

type Context interface{ Done() <-chan struct{} }
