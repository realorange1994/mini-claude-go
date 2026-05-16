package microlisp

import (
	"fmt"
	"strings"
)

// read-delimited-list and delimitedParser for reading forms with custom delimiters

func builtinReadDelimitedList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("read-delimited-list: need a delimiter character")
	}

	// Extract delimiter character
	delimitChar := primaryValue(args[0])
	var delim rune
	if delimitChar.typ == VChar {
		delim = delimitChar.ch
	} else if delimitChar.typ == VStr && len(delimitChar.str) == 1 {
		delim = rune(delimitChar.str[0])
	} else {
		return nil, fmt.Errorf("read-delimited-list: first argument must be a character")
	}

	// Validate: delimiter must be a terminating macro character
	rt := currentReadtable
	if rt == nil {
		return nil, fmt.Errorf("read-delimited-list: no readtable")
	}
	entry, ok := rt.macroFns[delim]
	if !ok || entry == nil {
		return nil, fmt.Errorf("read-delimited-list: %q is not a macro character", string(delim))
	}
	if !entry.terminating {
		return nil, fmt.Errorf("read-delimited-list: %q is not a terminating macro character", string(delim))
	}

	// Determine input stream (default: *standard-input*)
	stream := stdinStream
	if len(args) > 1 {
		stream = primaryValue(args[1])
		if stream.typ != VStream {
			return nil, fmt.Errorf("read-delimited-list: second argument must be a stream")
		}
	}

	// Read all remaining characters from the stream into a buffer
	var buf []rune
	for {
		ch, err := stream.stream.readChar()
		if err != nil {
			break
		}
		buf = append(buf, ch)
	}

	// Parse forms from the buffer using our delimited-list parser
	forms, err := readFormsFromRDP(buf, delim, rt)
	if err != nil {
		return nil, err
	}

	// Build a proper Lisp list from the forms
	var head, tail *Value
	for _, form := range forms {
		pair := cons(form, vnil())
		if head == nil {
			head = pair
			tail = pair
		} else {
			tail.cdr = pair
			tail = pair
		}
	}
	if head == nil {
		return vnil(), nil
	}
	return head, nil
}

// delimitedParser is a parser for read-delimited-list that reads from a rune slice.
type delimitedParser struct {
	src       []rune
	pos       int
	depth     int
	readtable *Readtable
}

// readFormsFromRDP reads all complete forms from the rune slice, stopping
// when the delimiter is encountered at depth 0.
func readFormsFromRDP(src []rune, delim rune, rt *Readtable) ([]*Value, error) {
	p := &delimitedParser{src: src, pos: 0, depth: 0, readtable: rt}
	forms := []*Value{}

	for p.pos < len(p.src) {
		// Skip whitespace
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
			p.pos++
		}
		if p.pos >= len(p.src) {
			break
		}

		// Check delimiter
		if p.src[p.pos] == delim && p.depth == 0 {
			p.pos++ // consume delimiter
			break
		}

		// Read one form
		form, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		if form != nil {
			forms = append(forms, form)
		}
	}

	return forms, nil
}

// readExpr reads one expression from the delimited parser.
func (p *delimitedParser) readExpr() (*Value, error) {
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected end of input")
	}

	ch := p.src[p.pos]

	// Skip whitespace
	for (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') && p.pos < len(p.src) {
		p.pos++
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unexpected end of input")
		}
		ch = p.src[p.pos]
	}

	switch ch {
	case '(':
		return p.readList('(', ')')
	case '[':
		return p.readList('[', ']')
	case '{':
		return p.readList('{', '}')
	case '\'':
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("quote"), cons(v, vnil())), nil
	case '`':
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("QUASIQUOTE"), cons(v, vnil())), nil
	case ',':
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '@' {
			p.pos += 2
			v, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			return cons(vsym("UNQUOTE-SPLICING"), cons(v, vnil())), nil
		}
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("UNQUOTE"), cons(v, vnil())), nil
	case '#':
		return p.readDispatch()
	case '"':
		return p.readString()
	case '|':
		return p.readBarSym()
	case ';':
		// Comment — skip to end of line
		p.pos++
		for p.pos < len(p.src) && p.src[p.pos] != '\n' {
			p.pos++
		}
		// After comment, try to read next expression
		return p.readExpr()
	}

	// Number or symbol
	start := p.pos
	if (ch == '-' || ch == '+') && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1]) {
		p.pos++
		for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
			p.pos++
		}
		numStr := string(p.src[start:p.pos])
		if v, err := parseExpr(numStr); err == nil {
			return v, nil
		}
	}
	if isDigit(ch) || (ch == '-' && p.pos+1 < len(p.src) && isDigit(p.src[p.pos+1])) {
		p.pos++
		for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
			p.pos++
		}
		// Check for rational
		if p.pos < len(p.src) && p.src[p.pos] == '/' {
			p.pos++
			for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
				p.pos++
			}
		}
		// Check for decimal
		if p.pos < len(p.src) && p.src[p.pos] == '.' {
			p.pos++
			for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
				p.pos++
			}
		}
		// Check for exponent
		if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E' || p.src[p.pos] == 'd' || p.src[p.pos] == 'D') {
			p.pos++
			if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
				p.pos++
			}
			for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
				p.pos++
			}
		}
		numStr := string(p.src[start:p.pos])
		if v, err := parseExpr(numStr); err == nil {
			return v, nil
		}
	}

	// Symbol
	if isSymbolChar(ch) {
		p.pos++
		for p.pos < len(p.src) && isSymbolChar(p.src[p.pos]) {
			p.pos++
		}
		symStr := string(p.src[start:p.pos])
		return internOrVsym(symStr), nil
	}

	// Unknown char — skip and try next
	p.pos++
	return p.readExpr()
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isSymbolChar(ch rune) bool {
	if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '=' || ch == '<' || ch == '>' || ch == '_' || ch == '@' || ch == '#' || ch == '$' || ch == '%' || ch == '^' || ch == '&' || ch == '~' || ch == '.' || ch == '?' || ch == '!' || ch == ':'
}

func (p *delimitedParser) readList(open, close rune) (*Value, error) {
	p.pos++ // skip open
	p.depth++
	var head, tail *Value
	for p.pos < len(p.src) {
		// Skip whitespace
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
			p.pos++
		}
		if p.pos >= len(p.src) {
			break
		}
		if p.src[p.pos] == close {
			p.pos++ // skip close
			p.depth--
			if head == nil {
				return vnil(), nil
			}
			return head, nil
		}
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		if v != nil {
			pair := cons(v, vnil())
			if head == nil {
				head = pair
				tail = pair
			} else {
				tail.cdr = pair
				tail = pair
			}
		}
	}
	return nil, fmt.Errorf("unclosed list")
}

func (p *delimitedParser) readString() (*Value, error) {
	p.pos++ // skip opening "
	var b strings.Builder
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			p.pos++ // skip backslash
			esc := p.src[p.pos]
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteRune(esc)
			}
			p.pos++ // skip escaped char
		} else if ch == '"' {
			p.pos++ // skip closing "
			return vstr(b.String()), nil
		} else {
			b.WriteRune(ch)
			p.pos++
		}
	}
	return nil, fmt.Errorf("unclosed string")
}

func (p *delimitedParser) readBarSym() (*Value, error) {
	p.pos++ // skip opening |
	var b strings.Builder
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch == '\\' && p.pos+1 < len(p.src) {
			// Escape sequence: skip backslash and next char
			p.pos += 2
			b.WriteRune(p.src[p.pos-1])
		} else if ch == '|' {
			p.pos++ // skip closing |
			return vsym(b.String()), nil
		} else {
			b.WriteRune(ch)
			p.pos++
		}
	}
	return nil, fmt.Errorf("unclosed bar symbol")
}

func (p *delimitedParser) readDispatch() (*Value, error) {
	if p.pos+1 >= len(p.src) {
		p.pos++
		return internOrVsym("#"), nil
	}
	p.pos++ // skip #
	ch := p.src[p.pos]

	if ch == '\\' {
		// Character literal #\x or #\Space
		p.pos++
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != ' ' && p.src[p.pos] != '\t' && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' && p.src[p.pos] != '(' && p.src[p.pos] != ')' && p.src[p.pos] != '"' && p.src[p.pos] != '[' && p.src[p.pos] != ']' {
			p.pos++
		}
		name := string(p.src[start:p.pos])
		// Named character lookup (case-insensitive)
		switch strings.ToLower(name) {
		case "space":
			return vchar(' '), nil
		case "newline":
			return vchar('\n'), nil
		case "tab":
			return vchar('\t'), nil
		case "return":
			return vchar('\r'), nil
		case "backspace":
			return vchar('\x08'), nil
		case "rubout", "del":
			return vchar('\x7f'), nil
		}
		if len(name) == 1 {
			return vchar(rune(name[0])), nil
		}
		// Unknown multi-character name — treat as symbol
		return vsym("#" + "\\" + name), nil
	}

	if ch == '(' {
		// Vector literal #(...)
		p.pos++
		var elements []*Value
		for p.pos < len(p.src) {
			for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
				p.pos++
			}
			if p.pos >= len(p.src) {
				break
			}
			if p.src[p.pos] == ')' {
				p.pos++
				break
			}
			v, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			if v != nil {
				elements = append(elements, v)
			}
		}
		arr := &LispArray{dims: []int{len(elements)}, elements: make([]*Value, len(elements)), fillPtr: -1}
		copy(arr.elements, elements)
		v := gcv()
		v.typ = VArray
		v.array = arr
		return v, nil
	}

	if ch == '|' {
		// Block comment #| ... |#
		p.pos++
		depth := 1
		for p.pos+1 < len(p.src) && depth > 0 {
			if p.src[p.pos] == '|' && p.src[p.pos+1] == '#' {
				p.pos += 2
				depth--
			} else if p.src[p.pos] == '#' && p.src[p.pos+1] == '|' {
				p.pos += 2
				depth++
			} else {
				p.pos++
			}
		}
		// After block comment, read next expression
		return p.readExpr()
	}

	if ch == '+' || ch == '-' {
		// Feature conditional #+ or #-
		inc := (ch == '+')
		p.pos++
		// Read feature name
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != ' ' && p.src[p.pos] != '\t' && p.src[p.pos] != '\n' && p.src[p.pos] != '\r' && p.src[p.pos] != '(' && p.src[p.pos] != ')' {
			p.pos++
		}
		featName := string(p.src[start:p.pos])
		// Read the conditional form
		for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
			p.pos++
		}
		form, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		satisfied := featureSatisfied(internOrVsym(":" + featName))
		if inc == satisfied {
			return form, nil
		}
		return p.readExpr()
	}

	if ch == 'C' || ch == 'c' {
		// Complex number #C(real imag)
		p.pos++
		if p.pos < len(p.src) && p.src[p.pos] == '(' {
			p.pos++
			realPart, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			imagPart, err := p.readExpr()
			if err != nil {
				return nil, err
			}
			// Expect ')'
			for p.pos < len(p.src) && p.src[p.pos] != ')' {
				p.pos++
			}
			if p.pos < len(p.src) {
				p.pos++
			}
			r := 0.0
			if realPart != nil && realPart.typ == VNum {
				r = realPart.num
			}
			i := 0.0
			if imagPart != nil && imagPart.typ == VNum {
				i = imagPart.num
			}
			return vcomplex(r, i), nil
		}
	}

	if ch == 'P' || ch == 'p' {
		// Pathname #P"..."
		p.pos++
		if p.pos < len(p.src) && p.src[p.pos] == '"' {
			p.pos++
			var b strings.Builder
			for p.pos < len(p.src) && p.src[p.pos] != '"' {
				if p.src[p.pos] == '\\' && p.pos+1 < len(p.src) {
					p.pos++
				}
				b.WriteRune(p.src[p.pos])
				p.pos++
			}
			if p.pos < len(p.src) {
				p.pos++
			}
			return vpathname(parsePathnameString(b.String())), nil
		}
	}

	if ch == '\'' {
		// Function shorthand #'expr
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		return cons(vsym("function"), cons(v, vnil())), nil
	}

	if ch == '.' {
		// Sharp-dot #.expr — read and evaluate expr immediately
		p.pos++
		v, err := p.readExpr()
		if err != nil {
			return nil, err
		}
		result, err := Eval(v, globalEnv)
		if err != nil {
			return nil, fmt.Errorf("#. read-time evaluation error: %v", err)
		}
		return result, nil
	}

	p.pos++
	return internOrVsym("#" + string(ch)), nil
}

// buildListFromForms converts a slice of forms to a proper Lisp list.
func buildListFromForms(forms []*Value) *Value {
	var head, tail *Value
	for _, form := range forms {
		pair := cons(form, vnil())
		if head == nil {
			head = pair
			tail = pair
		} else {
			tail.cdr = pair
			tail = pair
		}
	}
	if head == nil {
		return vnil()
	}
	return head
}
