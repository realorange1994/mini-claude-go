package microlisp

import "fmt"

func EvalString(s string, env *Env) (*Value, error) {
	// Use lazy parsing: parse and evaluate one expression at a time.
	// This ensures that set-macro-character calls take effect for
	// subsequent forms in the same source string.
	l := lex(s)
	p := &Parser{l: l, ptoks: make([]Tok, 0, 64), readtable: currentReadtable, env: globalEnv}
	p.advance()
	var result *Value = vnil()
	for p.tok.typ != TEOF {
		// Update readtable reference before each expression
		p.readtable = currentReadtable
		p.env = globalEnv
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		// readExpr returning nil value without error means an unmatched
		// close parenthesis was encountered — report as syntax error.
		if v == nil {
			return nil, fmt.Errorf("syntax error: unmatched close parenthesis")
		}
		result, err = Eval(v, env)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
