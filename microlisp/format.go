package microlisp

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// -------- Advanced format --------

type fmtState struct {
	ctrl      string
	pos       int
	args      []*Value
	argIdx    int
	buf       strings.Builder
	escaped   bool
	remaining int // items remaining in current ~{ iteration (-1 = not in iteration)
}

func (fs *fmtState) done() bool { return fs.pos >= len(fs.ctrl) }
func (fs *fmtState) peek() byte {
	if fs.done() {
		return 0
	}
	return fs.ctrl[fs.pos]
}
func (fs *fmtState) next() byte {
	c := fs.ctrl[fs.pos]
	fs.pos++
	return c
}

func (fs *fmtState) popArg() *Value {
	if fs.argIdx < len(fs.args) {
		v := fs.args[fs.argIdx]
		fs.argIdx++
		return v
	}
	return vnil()
}

// formatBigIntBase formats a big.Int in the given base (2, 8, 16).
func formatBigIntBase(n *big.Int, base int) string {
	if base >= 2 && base <= 36 {
		return new(big.Int).Set(n).Text(base)
	}
	return new(big.Int).Set(n).Text(10)
}

func formatRoman(n int) string {
	if n <= 0 || n >= 4000 {
		return ""
	}
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	romans := []string{"m", "cm", "d", "cd", "c", "xc", "l", "xl", "x", "ix", "v", "iv", "i"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(romans[i])
			n -= v
		}
	}
	return b.String()
}

func formatRomanUpper(n int) string {
	return strings.ToUpper(formatRoman(n))
}

func formatOldRoman(n int) string {
	// Old-style Roman: like regular Roman but uses simpler additive notation
	if n <= 0 || n >= 4000 {
		return ""
	}
	vals := []int{1000, 500, 100, 50, 10, 5, 1}
	romans := []string{"M", "D", "C", "L", "X", "V", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(romans[i])
			n -= v
		}
	}
	return b.String()
}

// formatCardinal converts a number to English cardinal word form.
// 0 -> "zero", 1 -> "one", 21 -> "twenty-one", 123 -> "one hundred twenty-three"
func formatCardinal(n int) string {
	if n == 0 {
		return "zero"
	}
	if n < 0 {
		return "minus " + formatCardinalPositive(-n)
	}
	return formatCardinalPositive(n)
}

func formatCardinalPositive(n int) string {
	if n == 0 {
		return ""
	}
	if n >= 1000000000 {
		return formatCardinalHelper(n/1000000000, "billion", formatCardinalPositive(n%1000000000))
	}
	if n >= 1000000 {
		return formatCardinalHelper(n/1000000, "million", formatCardinalPositive(n%1000000))
	}
	if n >= 1000 {
		return formatCardinalHelper(n/1000, "thousand", formatCardinalPositive(n%1000))
	}
	if n >= 100 {
		return formatCardinalHelper(n/100, "hundred", formatCardinalPositive(n%100))
	}
	tens := []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	teens := []string{"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	units := []string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
	switch {
	case n >= 20:
		s := tens[n/10]
		if n%10 != 0 {
			s += "-" + units[n%10]
		}
		return s
	case n >= 10:
		return teens[n-10]
	default:
		return units[n]
	}
}

func formatCardinalHelper(val int, name, rest string) string {
	s := formatCardinalPositive(val) + " " + name
	if rest != "" {
		s += " " + rest
	}
	return s
}

// formatOrdinal converts a number to English ordinal word form.
// 0 -> "zeroth", 1 -> "first", 21 -> "twenty-first", 123 -> "one hundred twenty-third"
func formatOrdinal(n int) string {
	if n == 0 {
		return "zeroth"
	}
	if n < 0 {
		return "minus " + formatOrdinalPositive(-n)
	}
	return formatOrdinalPositive(n)
}

func formatOrdinalPositive(n int) string {
	ordinals := []string{
		"zeroth", "first", "second", "third", "fourth", "fifth",
		"sixth", "seventh", "eighth", "ninth", "tenth",
		"eleventh", "twelfth", "thirteenth", "fourteenth", "fifteenth",
		"sixteenth", "seventeenth", "eighteenth", "nineteenth",
		"twentieth", "twenty-first", "twenty-second", "twenty-third",
		"twenty-fourth", "twenty-fifth", "twenty-sixth", "twenty-seventh",
		"twenty-eighth", "twenty-ninth", "thirtieth", "thirty-first",
	}
	if n < len(ordinals) {
		return ordinals[n]
	}
	// For larger numbers, use cardinal form with ordinal ending on last word
	cardinal := formatCardinalPositive(n)
	words := strings.Split(cardinal, " ")
	last := words[len(words)-1]
	ordLast := lastOrdinal(last)
	if ordLast == last {
		// Fallback: just append "th"
		ordLast = last + "th"
	}
	words[len(words)-1] = ordLast
	return strings.Join(words, " ")
}

func lastOrdinal(cardinal string) string {
	ordinals := map[string]string{
		"zero": "zeroth", "one": "first", "two": "second", "three": "third",
		"four": "fourth", "five": "fifth", "six": "sixth", "seven": "seventh",
		"eight": "eighth", "nine": "ninth", "ten": "tenth",
		"eleven": "eleventh", "twelve": "twelfth", "thirteen": "thirteenth",
		"fourteen": "fourteenth", "fifteen": "fifteenth", "sixteen": "sixteenth",
		"seventeen": "seventeenth", "eighteen": "eighteenth", "nineteen": "nineteenth",
		"twenty": "twentieth", "thirty": "thirtieth", "forty": "fortieth",
		"fifty": "fiftieth", "sixty": "sixtieth", "seventy": "seventieth",
		"eighty": "eightieth", "ninety": "ninetieth",
	}
	// Check if cardinal ends with hyphenated word like "twenty-one"
	if idx := strings.LastIndex(cardinal, "-"); idx != -1 {
		lastWord := cardinal[idx+1:]
		if ord, ok := ordinals[lastWord]; ok {
			return cardinal[:idx+1] + ord
		}
	}
	if ord, ok := ordinals[cardinal]; ok {
		return ord
	}
	return cardinal
}

func (fs *fmtState) parseFmtDirective() (params []interface{}, colon, at bool, cmd byte) {
	colon = false
	at = false
	params = nil
	gotValue := false
	for !fs.done() {
		c := fs.peek()
		if c == ':' {
			colon = true
			fs.next()
		} else if c == '@' {
			at = true
			fs.next()
		} else if c == '\'' {
			fs.next()
			if !fs.done() {
				params = append(params, fs.next())
			}
			gotValue = true
		} else if c == 'V' || c == 'v' {
			fs.next()
			params = append(params, 'V')
			gotValue = true
		} else if c == '#' {
			fs.next()
			params = append(params, '#')
			gotValue = true
		} else if c >= '0' && c <= '9' {
			n := 0
			for !fs.done() && fs.peek() >= '0' && fs.peek() <= '9' {
				n = n*10 + int(fs.next()-'0')
			}
			params = append(params, n)
			gotValue = true
		} else if c == ',' {
			fs.next()
			if !gotValue {
				params = append(params, -1)
			}
			gotValue = false
		} else {
			break
		}
	}
	if !fs.done() {
		cmd = fs.next()
	}
	return
}

func (fs *fmtState) getParam(params []interface{}, idx, defaultVal int) int {
	if idx < len(params) {
		if v, ok := params[idx].(int); ok {
			return v
		}
	}
	// Check for V param
	if idx < len(params) {
		if _, ok := params[idx].(byte); ok {
			arg := fs.popArg()
			if arg.typ == VNum {
				return int(toNum(arg))
			}
			return defaultVal
		}
	}
	return defaultVal
}

func (fs *fmtState) getCharParam(params []interface{}, idx int, defaultVal byte) byte {
	if idx < len(params) {
		switch v := params[idx].(type) {
		case byte:
			return v
		case int:
			return byte(v)
		}
	}
	return defaultVal
}

func builtinFormat(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("format: need stream and control-string")
	}
	stream := args[0]
	ctrl := args[1]
	if ctrl.typ != VStr {
		return nil, fmt.Errorf("format: control-string must be a string")
	}
	fs := &fmtState{
		ctrl: ctrl.str,
		args: args[2:],
	}
	formatRun(fs)
	result := fs.buf.String()
	if stream == globalEnv.bindings["#t"] {
		fmt.Print(result)
		return vnil(), nil
	}
	// If stream is a real stream (not nil), write to it
	if stream.typ == VStream && stream.stream != nil && stream.stream.isOutput {
		if stream.stream.isString && stream.stream.strBuf != nil {
			stream.stream.strBuf.WriteString(result)
		}
		return vnil(), nil
	}
	return vstr(result), nil
}

func newFmtState(ctrl string, args []*Value, argIdx int) *fmtState {
	return &fmtState{ctrl: ctrl, args: args, argIdx: argIdx, remaining: -1}
}

func formatRun(fs *fmtState) {
	for !fs.done() && !fs.escaped {
		c := fs.peek()
		if c == '~' {
			fs.next()
			if fs.done() {
				fs.buf.WriteByte('~')
				return
			}
			formatDispatch(fs)
		} else {
			fs.buf.WriteByte(fs.next())
		}
	}
}

func formatDispatch(fs *fmtState) {
	params, colon, at, cmd := fs.parseFmtDirective()
	// Format directives are case-insensitive
	if cmd >= 'a' && cmd <= 'z' {
		cmd = cmd - 'a' + 'A'
	}
	switch cmd {
	case 'A':
		arg := fs.popArg()
		mincol := fs.getParam(params, 0, 0)
		colinc := fs.getParam(params, 1, 1)
		minpad := fs.getParam(params, 2, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		s := princToString(arg)
		if isNil(arg) && colon {
			s = "()"
		}
		padlen := 0
		if int(mincol) > len(s) {
			padlen = int(mincol) - len(s)
			if int(minpad) > padlen {
				padlen = int(minpad)
			}
			if int(colinc) > 1 {
				for padlen < int(mincol)-len(s) || (padlen-int(minpad))%int(colinc) != 0 {
					padlen++
				}
			}
		}
		if at {
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			fs.buf.WriteString(s)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
		}
	case 'S':
		arg := fs.popArg()
		mincol := fs.getParam(params, 0, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		s := writeToString(arg)
		padlen := 0
		if int(mincol) > len(s) {
			padlen = int(mincol) - len(s)
		}
		if at {
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			fs.buf.WriteString(s)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteByte(byte(padchar))
			}
		}
	case 'D':
		width := fs.getParam(params, 0, 0)
		padchar := fs.getCharParam(params, 1, ' ')
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = arg.bigInt.String()
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 10)
		}
		n := 0
		if arg.typ == VBigInt {
			n = arg.bigInt.Sign()
		} else {
			n = int(toNum(arg))
		}
		if n >= 0 && at {
			s = "+" + s
		}
		for len(s) < width {
			s = string(padchar) + s
		}
		if colon && !at {
			var b strings.Builder
			for i, c := range s {
				if i > 0 && (len(s)-i)%3 == 0 && c != '-' {
					b.WriteByte(',')
				}
				b.WriteRune(c)
			}
			s = b.String()
		}
		fs.buf.WriteString(s)
	case 'B':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 2)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 2)
		}
		if at {
			fs.buf.WriteString("#b" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'O':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 8)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 8)
		}
		if at {
			fs.buf.WriteString("#o" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'X':
		arg := fs.popArg()
		var s string
		if arg.typ == VBigInt {
			s = formatBigIntBase(arg.bigInt, 16)
		} else {
			s = strconv.FormatInt(int64(toNum(arg)), 16)
		}
		if at {
			fs.buf.WriteString("#x" + s)
		} else {
			fs.buf.WriteString(s)
		}
	case 'F':
		decimals := fs.getParam(params, 1, -1)
		arg := fs.popArg()
		f := toNum(arg)
		var s string
		if decimals >= 0 {
			s = strconv.FormatFloat(f, 'f', decimals, 64)
		} else {
			s = strconv.FormatFloat(f, 'f', -1, 64)
		}
		// ~F must always produce a float: ensure decimal point is present
		if !strings.Contains(s, ".") {
			s = s + ".0"
		}
		fs.buf.WriteString(s)
	case '$':
		// ~$, ~:$, ~@$ - dollar float formatting
		// Params: d n w padchar
		// d = digits after decimal (default 2)
		// n = minimum digits before decimal point (default 1)
		// w = minimum field width (default 0)
		// padchar = padding character (default space)
		d := fs.getParam(params, 0, 2)
		n := fs.getParam(params, 1, 1)
		w := fs.getParam(params, 2, 0)
		padchar := fs.getCharParam(params, 3, ' ')
		arg := fs.popArg()
		f := toNum(arg)
		s := strconv.FormatFloat(f, 'f', int(d), 64)
		// Ensure minimum digits before decimal point
		if idx := strings.Index(s, "."); idx >= 0 {
			beforeDot := s[:idx]
			sign := ""
			if len(beforeDot) > 0 && (beforeDot[0] == '-' || beforeDot[0] == '+') {
				sign = string(beforeDot[0])
				beforeDot = beforeDot[1:]
			}
			for len(beforeDot) < int(n) {
				beforeDot = "0" + beforeDot
			}
			s = sign + beforeDot + s[idx:]
		}
		// Handle @ modifier: always print sign
		if at && f >= 0 {
			s = "+" + s
		}
		// Handle colon modifier: sign appears before padding
		if colon {
			// ~:$ sign before padding
			padlen := int(w) - len(s)
			if padlen < 0 {
				padlen = 0
			}
			signPart := ""
			if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
				signPart = string(s[0])
				s = s[1:]
			}
			fs.buf.WriteString(signPart)
			for i := 0; i < padlen; i++ {
				fs.buf.WriteRune(rune(padchar))
			}
			fs.buf.WriteString(s)
		} else {
			padlen := int(w) - len(s)
			if padlen < 0 {
				padlen = 0
			}
			for i := 0; i < padlen; i++ {
				fs.buf.WriteRune(rune(padchar))
			}
			fs.buf.WriteString(s)
		}
	case 'E':
		// ~E: scientific notation
		decimals := fs.getParam(params, 1, -1)
		eMinDigits := fs.getParam(params, 2, 1)
		arg := fs.popArg()
		f := toNum(arg)
		prec := -1
		if decimals >= 0 {
			prec = int(decimals) + 1
		}
		s := strconv.FormatFloat(f, 'E', prec, 64)
		idx := strings.Index(s, "E")
		if idx < 0 {
			fs.buf.WriteString(s)
			break
		}
		mantissa := s[:idx]
		expStr := s[idx+1:]
		// Ensure mantissa has a decimal point
		if !strings.Contains(mantissa, ".") {
			mantissa += ".0"
		}
		// Normalize exponent: strip leading zeros
		expSign := ""
		if len(expStr) > 0 && (expStr[0] == '+' || expStr[0] == '-') {
			expSign = string(expStr[0])
			expStr = expStr[1:]
		}
		expDigits := strings.TrimLeft(expStr, "0")
		if expDigits == "" {
			expDigits = "0"
		}
		// Pad to minimum digit count
		for len(expDigits) < int(eMinDigits) {
			expDigits = "0" + expDigits
		}
		fs.buf.WriteString(mantissa + "E" + expSign + expDigits)
	case 'R':
		arg := fs.popArg()
		n := int(toNum(arg))
		// ~nR with radix parameter: print in base n (ANSI CL)
		if len(params) >= 1 && !colon && !at {
			radix := int(fs.getParam(params, 0, 10))
			if radix < 2 || radix > 36 {
				fs.buf.WriteString(formatCardinal(n))
			} else {
				fs.buf.WriteString(formatBigIntBase(big.NewInt(int64(n)), radix))
			}
			break
		}
		switch {
		case at && colon:
			// ~:@R: old-style Roman numerals
			fs.buf.WriteString(formatOldRoman(n))
		case at:
			// ~@R: Roman numerals (uppercase)
			fs.buf.WriteString(formatRomanUpper(n))
		case colon:
			// ~:R: ordinal
			fs.buf.WriteString(formatOrdinal(n))
		default:
			// ~R: cardinal
			fs.buf.WriteString(formatCardinal(n))
		}
	case 'C':
		arg := fs.popArg()
		if arg.typ == VChar {
			if at {
				// ~@C: print with escape syntax like prin1
				fs.buf.WriteString(ToString(arg))
			} else if colon {
				// ~:C: spell out the character name for non-printing chars
				ch := arg.ch
				switch ch {
				case ' ':
					fs.buf.WriteString("Space")
				case '\n':
					fs.buf.WriteString("Newline")
				case '\t':
					fs.buf.WriteString("Tab")
				case '\r':
					fs.buf.WriteString("Return")
				case '\x08':
					fs.buf.WriteString("Backspace")
				case '\x7f':
					fs.buf.WriteString("Rubout")
				case '\f':
					fs.buf.WriteString("Page")
				default:
					if ch < ' ' || ch == '\x7f' {
						// Non-printing: use #\name form
						fs.buf.WriteString(ToString(arg))
					} else {
						fs.buf.WriteRune(ch)
					}
				}
			} else {
				// ~C: print just the character itself (like princ)
				fs.buf.WriteRune(arg.ch)
			}
		} else {
			fs.buf.WriteString(princToString(arg))
		}
	case '%':
		n := int(fs.getParam(params, 0, 1))
		for i := 0; i < n; i++ {
			fs.buf.WriteString("\n")
		}
	case '&':
		// fresh-line: output newline unless already at start of line
		if fs.buf.Len() > 0 {
			n := int(fs.getParam(params, 0, 1))
			for i := 0; i < n; i++ {
				fs.buf.WriteString("\n")
			}
		}
	case '|':
		fs.buf.WriteString("\f")
	case '~':
		fs.buf.WriteString("~")
	case 'T':
		colnum := fs.getParam(params, 0, 1)
		colinc := fs.getParam(params, 1, 1)
		current := fs.buf.Len()
		if at {
			// ~@T: output colnum spaces, then enough to reach next multiple of colinc
			for i := 0; i < int(colnum); i++ {
				fs.buf.WriteByte(' ')
			}
			current = fs.buf.Len()
			if int(colinc) > 0 {
				rem := current % int(colinc)
				if rem != 0 {
					for i := 0; i < int(colinc)-rem; i++ {
						fs.buf.WriteByte(' ')
					}
				}
			}
		} else {
			// ~T: advance to column colnum, or if already past, advance by colinc
			if current < int(colnum) {
				for i := current; i < int(colnum); i++ {
					fs.buf.WriteByte(' ')
				}
			} else if int(colinc) > 0 {
				target := current
				if target%int(colinc) != 0 {
					target = ((target / int(colinc)) + 1) * int(colinc)
				} else {
					target = current + int(colinc)
				}
				for i := current; i < target; i++ {
					fs.buf.WriteByte(' ')
				}
			}
		}
	case '*':
		n := fs.getParam(params, 0, 1)
		if at {
			// ~@* with no param: go to arg 0 (first argument)
			if len(params) == 0 {
				n = 0
			}
			fs.argIdx = n
		} else {
			fs.argIdx += n
		}
	case '?':
		if at {
			// ~@?: pop control string from args, use remaining args for recursive format
			newCtrl := fs.popArg()
			if newCtrl.typ == VStr {
				subFs := &fmtState{ctrl: newCtrl.str, args: fs.args, argIdx: fs.argIdx}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
				fs.argIdx = subFs.argIdx
			}
		} else {
			// ~?: pop control string and argument list
			newCtrl := fs.popArg()
			newArgs := fs.popArg()
			if newCtrl.typ == VStr {
				subFs := &fmtState{ctrl: newCtrl.str, args: seqToList(newArgs)}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		}
	case '(':
		// ~( ... ~) - case conversion
		depth := 1
		bodyStart := fs.pos
		for !fs.done() && depth > 0 {
			c := fs.next()
			if c == '~' && !fs.done() {
				nc := fs.next()
				if nc == '(' {
					depth++
				} else if nc == ')' {
					depth--
				}
			}
		}
		bodyEnd := fs.pos - 2
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := fs.ctrl[bodyStart:bodyEnd]
		subFs := newFmtState(body, fs.args, fs.argIdx)
		formatRun(subFs)
		fs.argIdx = subFs.argIdx
		s := subFs.buf.String()
		if colon && at {
			s = titleCaseString(strings.ToLower(s))
		} else if colon {
			s = strings.ToUpper(s)
		} else if at {
			if len(s) > 0 {
				s = strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
			}
		} else {
			s = strings.ToLower(s)
		}
		fs.buf.WriteString(s)
	case ')':
	case '[':
		sections := formatCollectSections(fs, ']')
		selector := fs.popArg()
		sel := int(toNum(selector))
		if sel < 0 {
			sel = 0
		}
		if sel >= len(sections) {
			sel = len(sections) - 1
		}
		if sel >= 0 && sel < len(sections) {
			subFs := &fmtState{ctrl: sections[sel], args: fs.args, argIdx: fs.argIdx}
			formatRun(subFs)
			fs.buf.WriteString(subFs.buf.String())
			fs.argIdx = subFs.argIdx
		}
	case ';', ']':
	case '{':
		body := formatCollectBody(fs, '}')
		limit := fs.getParam(params, 0, -1) // -1 means no limit
		if colon {
			listArg := fs.popArg()
			elements := seqToList(listArg)
			for i, el := range elements {
				if limit >= 0 && i >= limit {
					break
				}
				rem := len(elements) - i - 1
				subFs := &fmtState{ctrl: body, args: seqToList(el), remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		} else if at {
			count := 0
			prevArgIdx := fs.argIdx
			iterCount := 0
			for fs.argIdx < len(fs.args) {
				if limit >= 0 && count >= limit {
					break
				}
				if iterCount > len(fs.args)+10 {
					break
				}
				rem := len(fs.args) - fs.argIdx - 1
				subFs := &fmtState{ctrl: body, args: fs.args, argIdx: fs.argIdx, remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
				if subFs.argIdx <= fs.argIdx {
					break
				}
				fs.argIdx = subFs.argIdx
				if fs.argIdx == prevArgIdx {
					break
				}
				count++
				iterCount++
			}
		} else {
			listArg := fs.popArg()
			elements := seqToList(listArg)
			for i, el := range elements {
				if limit >= 0 && i >= limit {
					break
				}
				rem := len(elements) - i - 1
				subFs := &fmtState{ctrl: body, args: []*Value{el}, remaining: rem}
				formatRun(subFs)
				fs.buf.WriteString(subFs.buf.String())
			}
		}
	case '}':
	case '^':
		if fs.remaining >= 0 {
			// Inside ~{ iteration: check if more items remain
			if fs.remaining == 0 {
				fs.escaped = true
			}
		} else if fs.argIdx >= len(fs.args) {
			fs.escaped = true
		}
	case '<':
		// ~mincol,colinc,minpad,padchar<text~> — Justification
		// ~<seg1~;seg2~;seg3~> — segmented: distribute segments across width
		// Extract body between ~< and ~>
		depth := 1
		bodyStart := fs.pos
		for !fs.done() && depth > 0 {
			c := fs.next()
			if c == '~' && !fs.done() {
				nc := fs.next()
				if nc == '<' {
					depth++
				} else if nc == '>' {
					depth--
				}
			}
		}
		bodyEnd := fs.pos - 2
		if bodyEnd < bodyStart {
			bodyEnd = bodyStart
		}
		body := fs.ctrl[bodyStart:bodyEnd]

		// Check for segments separated by ~; and detect fill separator (~:;)
		segments, fillSegIdx := formatCollectJustifySegments(body)

		if !colon && !at && len(segments) <= 1 {
			// ~<~> without modifiers, no segments: process body and output
			subFs := newFmtState(body, fs.args, fs.argIdx)
			formatRun(subFs)
			fs.argIdx = subFs.argIdx
			fs.buf.WriteString(subFs.buf.String())
		} else if len(segments) > 1 {
			// Segmented justification: process each segment and distribute
			// padding between them to fill mincol
			mincol := fs.getParam(params, 0, 0)
			colinc := fs.getParam(params, 1, 1)
			padchar := fs.getCharParam(params, 3, ' ')

			// Process each segment
			processedSegs := make([]string, 0, len(segments))
			for _, seg := range segments {
				subFs := newFmtState(seg, fs.args, fs.argIdx)
				formatRun(subFs)
				fs.argIdx = subFs.argIdx
				processedSegs = append(processedSegs, subFs.buf.String())
			}

			totalContentLen := 0
			for _, s := range processedSegs {
				totalContentLen += len(s)
			}

			// Calculate total padding needed
			numGaps := len(processedSegs) - 1
			if numGaps <= 0 {
				numGaps = 1
			}
			totalPadLen := 0
			if int(mincol) > totalContentLen {
				totalPadLen = int(mincol) - totalContentLen
				if int(colinc) > 1 && numGaps > 0 {
					// Round up totalPadLen to a multiple of colinc
					for totalPadLen%int(colinc) != 0 && totalPadLen+totalContentLen < int(mincol)+int(colinc) {
						totalPadLen++
					}
				}
			}

			// Distribute padding between gaps
			gapPad := make([]int, numGaps)
			remaining := totalPadLen
			if fillSegIdx >= 0 && fillSegIdx < numGaps {
				// Fill section: all extra padding goes to the gap after fill separator
				for i := 0; i < numGaps; i++ {
					if i == fillSegIdx {
						gapPad[i] = remaining
					} else {
						gapPad[i] = 0
					}
				}
			} else {
				// Distribute evenly
				base := 0
				if numGaps > 0 {
					base = remaining / numGaps
				}
				extra := remaining - base*numGaps
				for i := 0; i < numGaps; i++ {
					gapPad[i] = base
					if i < extra {
						gapPad[i]++
					}
				}
			}

			// Output segments with padding
			for i, s := range processedSegs {
				fs.buf.WriteString(s)
				if i < numGaps {
					for j := 0; j < gapPad[i]; j++ {
						fs.buf.WriteByte(byte(padchar))
					}
				}
			}
		} else {
			// ~:@<~> or ~@<~> or ~:<~>: single-segment justification
			mincol := fs.getParam(params, 0, 0)
			colinc := fs.getParam(params, 1, 1)
			minpad := fs.getParam(params, 2, 0)
			padchar := fs.getCharParam(params, 3, ' ')

			subFs := newFmtState(body, fs.args, fs.argIdx)
			formatRun(subFs)
			fs.argIdx = subFs.argIdx
			s := subFs.buf.String()

			// Calculate padding to reach mincol
			padlen := 0
			if int(mincol) > len(s) {
				padlen = int(mincol) - len(s)
				if int(minpad) > padlen {
					padlen = int(minpad)
				}
				if int(colinc) > 1 {
					for padlen < int(mincol)-len(s) || (padlen-int(minpad))%int(colinc) != 0 {
						padlen++
					}
				}
			}

			if colon && at {
				// ~:@< : center (pad on both sides equally)
				leftPad := padlen / 2
				rightPad := padlen - leftPad
				for i := 0; i < leftPad; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
				fs.buf.WriteString(s)
				for i := 0; i < rightPad; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
			} else if at {
				// ~@< : right-justify (pad on left)
				for i := 0; i < padlen; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
				fs.buf.WriteString(s)
			} else {
				// ~:< : left-justify (pad on right)
				fs.buf.WriteString(s)
				for i := 0; i < padlen; i++ {
					fs.buf.WriteByte(byte(padchar))
				}
			}
		}
	case '>':
	case 'P':
		n := int(toNum(fs.popArg()))
		if at {
			if n != 1 {
				fs.buf.WriteString("ies")
			} else {
				fs.buf.WriteString("y")
			}
		} else if colon {
			if n != 1 {
				fs.buf.WriteString("es")
			}
		} else {
			if n != 1 {
				fs.buf.WriteString("s")
			}
		}
	case 'G':
		// General format: scientific for very large/small, fixed otherwise
		digits := fs.getParam(params, 0, 6)
		if digits < 1 {
			digits = 1
		}
		arg := fs.popArg()
		f := toNum(arg)
		if f == 0 {
			fs.buf.WriteString("0.0")
		} else {
			absF := math.Abs(f)
			// Calculate exponent for determining format
			exp := int(math.Floor(math.Log10(absF)))
			// CL ~g: use fixed-point when -4 <= exp < digits
			if absF >= 1e16 || (absF < 1e-3 && absF > 0) || exp >= digits || exp < -4 {
				// Use exponential format with ~E-like logic
				// Format as mantissa + E + exponent, stripping trailing zeros
				prec := digits - 1
				if prec < 0 {
					prec = 0
				}
				s := strconv.FormatFloat(f, 'E', prec, 64)
				idx := strings.Index(s, "E")
				if idx < 0 {
					fs.buf.WriteString(s)
				} else {
					mantissa := s[:idx]
					expStr := s[idx+1:]
					// Strip trailing zeros from mantissa
					mantissa = strings.TrimRight(mantissa, "0")
					// If mantissa ends with '.', remove it
					mantissa = strings.TrimSuffix(mantissa, ".")
					// If mantissa is empty, use "0"
					if mantissa == "" {
						mantissa = "0"
					}
					// Ensure mantissa has a decimal point
					if !strings.Contains(mantissa, ".") {
						mantissa += ".0"
					}
					// Normalize exponent: strip leading zeros
					expSign := ""
					if len(expStr) > 0 && (expStr[0] == '+' || expStr[0] == '-') {
						expSign = string(expStr[0])
						expStr = expStr[1:]
					}
					expDigits := strings.TrimLeft(expStr, "0")
					if expDigits == "" {
						expDigits = "0"
					}
					fs.buf.WriteString(mantissa + "E" + expSign + expDigits)
				}
			} else {
				// Use fixed-point format with appropriate decimal places
				decPlaces := digits - 1 - exp
				if decPlaces < 0 {
					decPlaces = 0
				}
				s := strconv.FormatFloat(f, 'f', decPlaces, 64)
				// Strip unnecessary trailing zeros, but keep at least one decimal digit
				if dotIdx := strings.Index(s, "."); dotIdx >= 0 {
					// Remove trailing zeros after decimal point
					s = strings.TrimRight(s, "0")
					// If only '.' remains, add one '0' to keep "42."
					s = strings.TrimSuffix(s, ".")
					if s == "" || !strings.Contains(s, ".") {
						s += ".0"
					}
				} else {
					// No decimal point - add ".0" for floating-point clarity
					s += ".0"
				}
				fs.buf.WriteString(s)
			}
		}
	case 'W':
		// ~W: write format - use writeToString for canonical output with escape
		// ~:W uses princ output (no escape), ~@W uses print (with newline), ~:@W uses princ + newline
		arg := fs.popArg()
		if colon && at {
			fs.buf.WriteString(princToString(arg))
			fs.buf.WriteByte('\n')
		} else if colon {
			fs.buf.WriteString(princToString(arg))
		} else if at {
			fs.buf.WriteString(writeToString(arg))
			fs.buf.WriteByte('\n')
		} else {
			fs.buf.WriteString(writeToString(arg))
		}
	case '_':
		// Conditional newline: output a newline (simplified)
		fs.buf.WriteByte('\n')
	case 'I':
		// Indent: output spaces for pretty-printing
		n := fs.getParam(params, 0, 0)
		for i := 0; i < n; i++ {
			fs.buf.WriteByte(' ')
		}
	case '/':
		// ~/function-name/ — call a user-defined function to format
		var fnName strings.Builder
		for !fs.done() {
			c := fs.next()
			if c == '/' {
				break
			}
			fnName.WriteByte(c)
		}
		name := fnName.String()
		fnVal, fnErr := globalEnv.Get(strings.ToUpper(name))
		if fnErr != nil || fnVal == nil || (fnVal.typ != VPrim && fnVal.typ != VFunc && fnVal.typ != VGeneric) {
			fs.buf.WriteString("?")
		} else {
			arg := fs.popArg()
			colVal := vbool(colon)
			atVal := vbool(at)
			callArgs := []*Value{arg, colVal, atVal}
			for _, p := range params {
				if f64, ok := p.(float64); ok {
					callArgs = append(callArgs, vnum(f64))
				} else {
					callArgs = append(callArgs, vnum(0))
				}
			}
			result, _ := callFnOnSeq(fnVal, callArgs, nil)
			if result != nil {
				fs.buf.WriteString(princToString(result))
			}
		}
	}
}

// formatCollectJustifySegments splits a ~<...~> body by ~; separators,
// respecting nested directives. Returns segments and the index of the
// gap that follows the ~:; fill separator (-1 if none).
func formatCollectJustifySegments(body string) ([]string, int) {
	var segments []string
	fillGapIdx := -1
	depth := 0
	start := 0
	pos := 0
	for pos < len(body) {
		if body[pos] == '~' && pos+1 < len(body) {
			nc := body[pos+1]
			if nc == '<' {
				depth++
				pos += 2
			} else if nc == '>' {
				depth--
				pos += 2
			} else if nc == ':' && pos+2 < len(body) && body[pos+2] == ';' && depth == 0 {
				// ~:; fill separator
				segments = append(segments, body[start:pos])
				fillGapIdx = len(segments) // gap after this segment gets fill padding
				pos += 3
				start = pos
			} else if nc == ';' && depth == 0 {
				// ~; regular separator
				segments = append(segments, body[start:pos])
				pos += 2
				start = pos
			} else {
				pos += 2
			}
		} else {
			pos++
		}
	}
	if start < len(body) {
		segments = append(segments, body[start:])
	}
	return segments, fillGapIdx
}

func formatCollectSections(fs *fmtState, endChar byte) []string {
	var sections []string
	depth := 0
	start := fs.pos
	for !fs.done() {
		if fs.peek() == '~' {
			fs.next()
			if fs.done() {
				break
			}
			nc := fs.peek()
			if nc == '[' {
				depth++
				fs.next()
			} else if nc == ']' {
				if depth == 0 {
					sections = append(sections, fs.ctrl[start:fs.pos-1])
					fs.next()
					return sections
				}
				depth--
				fs.next()
			} else if nc == ':' {
				// Check for ~:; (default clause marker)
				pos := fs.pos
				if pos+1 < len(fs.ctrl) && fs.ctrl[pos+1] == ';' && depth == 0 {
					// ~:; — mark everything before this as section,
					// everything after as default, skip the next ~;
					sections = append(sections, fs.ctrl[start:fs.pos-1])
					fs.next() // consume ':'
					fs.next() // consume ';'
					start = fs.pos
					// Now continue collecting, but when we see the next ~; at depth 0,
					// skip it (it's the paired ~; with ~:)
					for !fs.done() {
						if fs.peek() == '~' {
							fs.next()
							if fs.done() {
								break
							}
							nc2 := fs.peek()
							if nc2 == '[' {
								depth++
								fs.next()
							} else if nc2 == ']' {
								if depth == 0 {
									sections = append(sections, fs.ctrl[start:fs.pos-1])
									fs.next()
									return sections
								}
								depth--
								fs.next()
							} else if nc2 == '{' {
								depth++
								fs.next()
							} else if nc2 == '}' {
								depth--
								fs.next()
							} else if nc2 == ';' && depth == 0 {
								// Skip the paired ~; — this is the second half of ~:;
								fs.next()
								start = fs.pos
							} else {
								fs.next()
							}
						} else {
							fs.next()
						}
					}
					sections = append(sections, fs.ctrl[start:fs.pos])
					return sections
				}
				// Not ~:; just consume
				fs.next()
			} else if nc == ';' && depth == 0 {
				sections = append(sections, fs.ctrl[start:fs.pos-1])
				fs.next()
				start = fs.pos
			} else if nc == '{' {
				depth++
				fs.next()
			} else if nc == '}' {
				depth--
				fs.next()
			} else {
				fs.next()
			}
		} else {
			fs.next()
		}
	}
	sections = append(sections, fs.ctrl[start:fs.pos])
	return sections
}

func formatCollectBody(fs *fmtState, endChar byte) string {
	depth := 0
	start := fs.pos
	for !fs.done() {
		if fs.peek() == '~' {
			fs.next()
			if fs.done() {
				break
			}
			nc := fs.next()
			if nc == '{' {
				depth++
			} else if nc == '}' {
				if depth == 0 {
					return fs.ctrl[start : fs.pos-2]
				}
				depth--
			} else if nc == '[' {
				depth++
			} else if nc == ']' {
				depth--
			}
		} else {
			fs.next()
		}
	}
	return fs.ctrl[start:fs.pos]
}
