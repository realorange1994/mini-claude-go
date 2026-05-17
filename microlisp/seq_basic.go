package microlisp

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

var charNameMap = map[string]rune{
	"space":     ' ',
	"newline":   '\n',
	"tab":       '\t',
	"return":    '\r',
	"backspace": '\x08',
	"bell":      '\x07',
	"page":      '\f',
	"escape":    '\x1b',
	"rubout":    '\x7f',
	"null":      '\x00',
	// Additional standard ASCII control character names
	"soh": '\x01',
	"stx": '\x02',
	"etx": '\x03',
	"eot": '\x04',
	"enq": '\x05',
	"ack": '\x06',
	"nak": '\x15',
	"syn": '\x16',
	"etb": '\x17',
	"can": '\x18',
	"em":  '\x19',
	"sub": '\x1a',
	"fs":  '\x1c',
	"gs":  '\x1d',
	"rs":  '\x1e',
	"us":  '\x1f',
	"del": '\x7f',
	// Common abbreviations
	"xoff": '\x13',
	"xon":  '\x11',
}

var charNameMapRev = map[rune]string{
	' ':    "space",
	'\n':   "newline",
	'\t':   "tab",
	'\r':   "return",
	'\x08': "backspace",
	'\x7f': "rubout",
	'\f':   "page",
	'\x00': "null",
	'\x07': "bell",
	'\x1b': "escape",
	'\x01': "soh",
	'\x02': "stx",
	'\x03': "etx",
	'\x04': "eot",
	'\x05': "enq",
	'\x06': "ack",
	'\x15': "nak",
	'\x16': "syn",
	'\x17': "etb",
	'\x18': "can",
	'\x19': "em",
	'\x1a': "sub",
	'\x1c': "fs",
	'\x1d': "gs",
	'\x1e': "rs",
	'\x1f': "us",
	'\x13': "xoff",
	'\x11': "xon",
}

func builtinLength(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("length: need argument")
	}
	v := primaryValue(args[0])
	// In CL, nil is a valid sequence (empty list), length = 0
	if isNil(v) {
		return vnum(0), nil
	}
	if v.typ != VPair && v.typ != VStr && v.typ != VArray {
		return nil, fmt.Errorf("length: %s is not a sequence", ToString(v))
	}
	return vnum(float64(lengthSafe(v))), nil
}

func lengthSafe(v *Value) int64 {
	if v.typ == VStr {
		return int64(utf8.RuneCountInString(v.str))
	}
	if v.typ == VArray && v.array != nil {
		if v.array.fillPtr >= 0 {
			return int64(v.array.fillPtr)
		}
		return int64(len(v.array.elements))
	}
	n := int64(0)
	visited := make(map[*Value]bool)
	for !isNil(v) && v.typ == VPair {
		if visited[v] {
			break // circular list
		}
		visited[v] = true
		n++
		v = v.cdr
	}
	return n
}

func builtinDisplay(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("display: need 1 argument")
	}
	fmt.Print(ToString(args[0]))
	return args[0], nil
}

func builtinNewline(args []*Value) (*Value, error) {
	fmt.Println()
	return vnil(), nil
}

func builtinWriteToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-to-string: need argument")
	}
	return vstr(writeToString(primaryValue(args[0]))), nil
}

func builtinRead(args []*Value) (*Value, error) {
	if len(args) > 0 {
		// stream argument - return nil for now
		return vnil(), nil
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return vnil(), nil
	}
	return parseExpr(strings.TrimSpace(line))
}

func builtinStr(args []*Value) (*Value, error) {
	var b strings.Builder
	for _, a := range args {
		if a.typ == VStr {
			b.WriteString(a.str)
		} else {
			b.WriteString(ToString(a))
		}
	}
	return vstr(b.String()), nil
}

func builtinStrAppend(args []*Value) (*Value, error) {
	var b strings.Builder
	for _, a := range args {
		if a.typ != VStr {
			return nil, fmt.Errorf("string-append: expected string")
		}
		b.WriteString(a.str)
	}
	return vstr(b.String()), nil
}

func builtinStrLen(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-length: need string")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string-length: expected string")
	}
	return vnum(float64(utf8.RuneCountInString(args[0].str))), nil
}

func builtinStrFind(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-find: need char/string and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-find: second arg must be a string")
	}
	var searchStr string
	if args[0].typ == VChar {
		searchStr = string(args[0].ch)
	} else if args[0].typ == VStr {
		searchStr = args[0].str
	} else {
		return nil, fmt.Errorf("string-find: first arg must be string or character")
	}
	idx := strings.Index(args[1].str, searchStr)
	if idx < 0 {
		return vnil(), nil
	}
	return vnum(float64(utf8.RuneCountInString(args[1].str[:idx]))), nil
}

func builtinSubstring(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("substring: need string and start index")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("substring: expected string")
	}
	s := args[0].str
	runes := []rune(s)
	start, err := safeToNum(args[1], "substring")
	if err != nil {
		return nil, err
	}
	startInt := int(start)
	var endInt int
	if len(args) >= 3 {
		en, err := safeToNum(args[2], "substring")
		if err != nil {
			return nil, err
		}
		endInt = int(en)
	} else {
		endInt = len(runes)
	}
	// Bounds checking
	if startInt < 0 {
		return nil, fmt.Errorf("substring: start index %d out of range [0..%d]", startInt, len(runes))
	}
	if endInt < 0 {
		return nil, fmt.Errorf("substring: end index %d out of range [0..%d]", endInt, len(runes))
	}
	if endInt > len(runes) {
		return nil, fmt.Errorf("substring: end index %d out of range [0..%d]", endInt, len(runes))
	}
	if startInt > endInt {
		return nil, fmt.Errorf("substring: start index %d greater than end index %d", startInt, endInt)
	}
	return vstr(string(runes[startInt:endInt])), nil
}

func builtinNumStr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("number->string: need a number")
	}
	s := strconv.FormatFloat(toNum(args[0]), 'g', -1, 64)
	return vstr(s), nil
}

func coerceStringDesignator(v *Value) (string, error) {
	switch v.typ {
	case VStr:
		return v.str, nil
	case VSym:
		return v.str, nil
	case VChar:
		return string(v.ch), nil
	case VArray:
		return vecToString(v), nil
	default:
		return "", fmt.Errorf("expected a string designator (string, symbol, or character)")
	}
}

func builtinStrNum(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string->number: need a string argument")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string->number: expected a string")
	}
	f, err := strconv.ParseFloat(args[0].str, 64)
	if err != nil {
		return vnil(), nil
	}
	return vnum(f), nil
}

func builtinSymStr(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("symbol->string: need a symbol")
	}
	if args[0].typ != VSym {
		return nil, fmt.Errorf("symbol->string: expected symbol")
	}
	return vstr(args[0].str), nil
}

func builtinStrSym(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string->symbol: need a string")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("string->symbol: expected string")
	}
	return vsym(strings.ToUpper(args[0].str)), nil
}

func builtinErr(args []*Value) (*Value, error) {
	var msg string
	for _, a := range args {
		msg += ToString(a)
	}
	return nil, fmt.Errorf("%s", msg)
}

func builtinExit(args []*Value) (*Value, error) {
	code := 0
	if len(args) > 0 {
		code = int(toNum(args[0]))
	}
	os.Exit(code)
	return vnil(), nil
}

var gensymCounter int64 = 0

func builtinGensym(args []*Value) (*Value, error) {
	gensymCounter++
	prefix := "G"
	if len(args) > 0 {
		prefix = args[0].str
	}
	return vsym(fmt.Sprintf("%s_%d", prefix, gensymCounter)), nil
}

func builtinGentemp(args []*Value) (*Value, error) {
	gensymCounter++
	prefix := "T"
	if len(args) > 0 {
		prefix = args[0].str
	}
	name := fmt.Sprintf("%s%d", prefix, gensymCounter)
	return internSymbol(name, currentPackage), nil
}

func builtinLoopCheck(args []*Value) (*Value, error) {
	loopIterationCount++
	if loopIterationCount > maxLoopIterations {
		loopIterationCount = 0
		return nil, fmt.Errorf("loop iteration limit exceeded (%d)", maxLoopIterations)
	}
	return vnil(), nil
}
