package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/shopspring/decimal"
)

type calculatorTool struct{}

func newCalculatorTool() tool.InvokableTool {
	return &calculatorTool{}
}

type calculatorInput struct {
	Expression string `json:"expression"`
}

func (c *calculatorTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "calculate",
		Desc: "Evaluate a mathematical expression with exact decimal arithmetic. Supports +, -, *, / and parentheses. Use this for any numeric calculation to avoid floating-point errors.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"expression": {
				Type:     schema.String,
				Desc:     `Mathematical expression, e.g. "(1.1 + 2.2) * 3" or "100 / 7".`,
				Required: true,
			},
		}),
	}, nil
}

func (c *calculatorTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input calculatorInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}

	result, err := evalExpr(strings.TrimSpace(input.Expression))
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

// evalExpr is a recursive-descent parser for +, -, *, / and parentheses.
func evalExpr(expr string) (decimal.Decimal, error) {
	p := &parser{input: []rune(expr)}
	result, err := p.parseExpr()
	if err != nil {
		return decimal.Zero, err
	}
	p.skipWS()
	if p.pos < len(p.input) {
		return decimal.Zero, fmt.Errorf("unexpected character %q at position %d", string(p.input[p.pos]), p.pos)
	}
	return result, nil
}

type parser struct {
	input []rune
	pos   int
}

func (p *parser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) parseExpr() (decimal.Decimal, error) {
	return p.parseAddSub()
}

func (p *parser) parseAddSub() (decimal.Decimal, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return decimal.Zero, err
	}
	for {
		p.skipWS()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseMulDiv()
		if err != nil {
			return decimal.Zero, err
		}
		if op == '+' {
			left = left.Add(right)
		} else {
			left = left.Sub(right)
		}
	}
	return left, nil
}

func (p *parser) parseMulDiv() (decimal.Decimal, error) {
	left, err := p.parseUnary()
	if err != nil {
		return decimal.Zero, err
	}
	for {
		p.skipWS()
		if p.pos >= len(p.input) {
			break
		}
		op := p.input[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return decimal.Zero, err
		}
		if op == '*' {
			left = left.Mul(right)
		} else {
			if right.IsZero() {
				return decimal.Zero, fmt.Errorf("division by zero")
			}
			left = left.Div(right)
		}
	}
	return left, nil
}

func (p *parser) parseUnary() (decimal.Decimal, error) {
	p.skipWS()
	if p.pos < len(p.input) && p.input[p.pos] == '-' {
		p.pos++
		v, err := p.parseAtom()
		if err != nil {
			return decimal.Zero, err
		}
		return v.Neg(), nil
	}
	return p.parseAtom()
}

func (p *parser) parseAtom() (decimal.Decimal, error) {
	p.skipWS()
	if p.pos >= len(p.input) {
		return decimal.Zero, fmt.Errorf("unexpected end of expression")
	}

	if p.input[p.pos] == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return decimal.Zero, err
		}
		p.skipWS()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return decimal.Zero, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return v, nil
	}

	return p.parseNumber()
}

func (p *parser) parseNumber() (decimal.Decimal, error) {
	p.skipWS()
	start := p.pos
	for p.pos < len(p.input) && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9' || p.input[p.pos] == '.') {
		p.pos++
	}
	if p.pos == start {
		return decimal.Zero, fmt.Errorf("expected number at position %d", p.pos)
	}
	return decimal.NewFromString(string(p.input[start:p.pos]))
}
