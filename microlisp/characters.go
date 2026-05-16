package microlisp

import (
	"fmt"
	"strings"
	"unicode"
)

func builtinChar(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char: need string and index")
	}
	if args[0].typ != VStr {
		return nil, fmt.Errorf("char: expected a string")
	}
	if args[1].typ != VNum {
		return nil, fmt.Errorf("char: expected an integer")
	}
	idx := int(args[1].num)
	s := args[0].str
	runes := []rune(s)
	if idx < 0 || idx >= len(runes) {
		return nil, fmt.Errorf("char: index %d out of range", idx)
	}
	return vchar(runes[idx]), nil
}

func builtinCharSetf(args []*Value) (*Value, error) {
	// (char-setf newchar string index)
	if len(args) < 3 {
		return nil, fmt.Errorf("char-setf: need newchar, string, and index")
	}
	newChar := args[0]
	s := args[1]
	idx := args[2]
	if newChar.typ != VChar {
		return nil, fmt.Errorf("char-setf: expected a character for new value")
	}
	if s.typ != VStr {
		return nil, fmt.Errorf("char-setf: expected a string")
	}
	if idx.typ != VNum {
		return nil, fmt.Errorf("char-setf: expected an integer index")
	}
	i := int(idx.num)
	runes := []rune(s.str)
	if i < 0 || i >= len(runes) {
		return nil, fmt.Errorf("char-setf: index %d out of range", i)
	}
	runes[i] = newChar.ch
	s.str = string(runes)
	return newChar, nil
}

func charCompare(op string, args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char%s: expected at least 2 characters", op)
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char%s: expected a character", op)
		}
		runes[i] = a.ch
	}
	for i := 0; i < len(runes)-1; i++ {
		a, b := runes[i], runes[i+1]
		switch op {
		case "=":
			if a != b {
				return vbool(false), nil
			}
		case "<":
			if !(a < b) {
				return vbool(false), nil
			}
		case ">":
			if !(a > b) {
				return vbool(false), nil
			}
		case "<=":
			if !(a <= b) {
				return vbool(false), nil
			}
		case ">=":
			if !(a >= b) {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinCharEq(args []*Value) (*Value, error) { return charCompare("=", args) }

func builtinCharLt(args []*Value) (*Value, error) { return charCompare("<", args) }

func builtinCharGt(args []*Value) (*Value, error) { return charCompare(">", args) }

func builtinCharLe(args []*Value) (*Value, error) { return charCompare("<=", args) }

func builtinCharGe(args []*Value) (*Value, error) { return charCompare(">=", args) }

func builtinCodeChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("code-char: need an integer")
	}
	if args[0].typ != VNum {
		return nil, fmt.Errorf("code-char: expected an integer")
	}
	n := int(args[0].num)
	if n < 0 || n > 0x10FFFF {
		return vnil(), nil
	}
	return vchar(rune(n)), nil
}

func builtinCharCode(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-code: need a character")
	}
	if args[0].typ != VChar {
		return nil, fmt.Errorf("char-code: expected a character")
	}
	return vnum(float64(args[0].ch)), nil
}

func builtinBitAref(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("bit: need bit-vector and index")
	}
	arr := args[0]
	if arr.typ != VArray {
		return nil, fmt.Errorf("bit: not a bit-vector")
	}
	if !isBitVector(arr.array) {
		return nil, fmt.Errorf("bit: not a bit-vector")
	}
	idx := int(toNum(args[1]))
	if idx < 0 || idx >= arr.array.dims[0] {
		return nil, fmt.Errorf("bit: index %d out of range", idx)
	}
	return arr.array.elements[idx], nil
}

func builtinSbitAref(args []*Value) (*Value, error) {
	// sbit is like bit but for simple bit-vectors (same as bit in microlisp)
	return builtinBitAref(args)
}

func builtinCharInt(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-int: need a character")
	}
	if args[0].typ != VChar {
		return nil, fmt.Errorf("char-int: expected a character")
	}
	// In most implementations, char-int is the same as char-code
	return vnum(float64(args[0].ch)), nil
}

func builtinNameChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("name-char: need a string")
	}
	name := args[0]
	var nameStr string
	switch name.typ {
	case VStr:
		nameStr = name.str
	case VSym:
		nameStr = name.str
	case VChar:
		// If already a char, return it
		return name, nil
	default:
		return vnil(), nil
	}
	nameStr = strings.ToLower(nameStr)
	if ch, ok := charNameMap[nameStr]; ok {
		return vchar(ch), nil
	}
	return vnil(), nil
}

func builtinCharName(args []*Value) (*Value, error) {
	if len(args) < 1 || args[0].typ != VChar {
		return nil, fmt.Errorf("char-name: expected a character")
	}
	ch := args[0].ch
	// CL spec: code 127 is named "Rubout" (not "Del" which is an implementation-dependent synonym)
	if ch == 127 {
		return vstr("Rubout"), nil
	}
	// Check named characters (return capitalized name per CL spec)
	for name, r := range charNameMap {
		if r == ch {
			// Capitalize first letter
			return vstr(strings.ToUpper(name[:1]) + name[1:]), nil
		}
	}
	// For control characters (C0: 0-31) that don't have specific names
	if ch < 32 && ch >= 0 {
		// Check if already in charNameMap (specific name like Newline)
		if _, ok := charNameMapRev[ch]; ok {
			// handled above
		} else {
			return vstr(fmt.Sprintf("C%d", ch)), nil
		}
	}
	// C1 control characters (128-159): return "C128", "C129", etc.
	if ch >= 128 && ch < 160 {
		return vstr(fmt.Sprintf("C%d", ch)), nil
	}
	// Other non-printing characters (e.g. NBSP, soft hyphen)
	if !unicode.IsPrint(ch) {
		return vstr("control"), nil
	}
	if ch == 127 {
		return vstr("rubout"), nil
	}
	return vnil(), nil
}
func builtinDigitChar(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("digit-char: need weight")
	}
	weight := int(toNum(args[0]))
	radix := 10
	if len(args) >= 2 {
		radix = int(toNum(args[1]))
	}
	if weight < 0 || weight >= radix {
		return vnil(), nil
	}
	if weight < 10 {
		return vchar(rune('0' + weight)), nil
	}
	return vchar(rune('A' + weight - 10)), nil
}

// -------- digit-char-p --------

// -------- alphanumericp --------
func builtinAlphanumericp(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("alphanumericp: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch) || unicode.IsDigit(ch)), nil
}

// -------- alpha-char-p --------
func builtinAlphaCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("alpha-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch)), nil
}

// -------- graphic-char-p --------
func builtinGraphicCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("graphic-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	// Graphic chars are printable characters EXCLUDING whitespace (space, tab, newline, etc.)
	// Per ANSI CL: space, newline, tab, page, return, backspace are NOT graphic characters
	if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\b' {
		return vbool(false), nil
	}
	return vbool(unicode.IsPrint(ch) && !unicode.IsSpace(ch)), nil
}

// -------- standard-char-p --------
// Standard chars are: space, newline, tab, page, return, backspace,
// and all 95 printable ASCII characters (codes 32-126)
// isStandardChar checks if a character is a standard-char
func isStandardChar(ch rune) bool {
	return (ch >= 32 && ch <= 126) || ch == 10 || ch == 9 || ch == 12 || ch == 13 || ch == 8
}

func builtinStandardCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("standard-char-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	// Standard chars are space (32), newline (10), tab (9), page (12), return (13),
	// backspace (8), and all 95 printable ASCII chars (32-126)
	return vbool(isStandardChar(ch)), nil
}

// -------- upper-case-p / lower-case-p / both-case-p --------
func builtinUpperCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("upper-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsUpper(ch) && unicode.IsLetter(ch)), nil
}

func builtinLowerCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("lower-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLower(ch) && unicode.IsLetter(ch)), nil
}

func builtinBothCaseP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("both-case-p: need a character")
	}
	var ch rune
	if args[0].typ == VChar {
		ch = args[0].ch
	} else if args[0].typ == VStr && len(args[0].str) > 0 {
		ch = []rune(args[0].str)[0]
	} else {
		return vbool(false), nil
	}
	return vbool(unicode.IsLetter(ch)), nil
}

// -------- char-upcase / char-downcase --------

// -------- char-equal (case-insensitive) / char-not-equal --------
func builtinCharEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char-equal: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char-equal: expected a character")
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinCharNotEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char-not-equal: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char-not-equal: expected a character")
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	// All pairs must be distinct (not just adjacent)
	for i := 0; i < len(runes); i++ {
		for j := i + 1; j < len(runes); j++ {
			if runes[i] == runes[j] {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func charVal(v *Value) rune {
	if v.typ == VChar {
		return v.ch
	}
	if v.typ == VStr && len(v.str) > 0 {
		return []rune(v.str)[0]
	}
	return 0
}

// -------- char-lessp / char-greaterp / char-not-lessp / char-not-greaterp (case-insensitive, multi-arg) --------
func charCompareCI(op string, args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char%s: expected at least 2 characters", op)
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char%s: expected a character", op)
		}
		runes[i] = unicode.ToLower(a.ch)
	}
	for i := 0; i < len(runes)-1; i++ {
		a, b := runes[i], runes[i+1]
		switch op {
		case "lessp":
			if !(a < b) {
				return vbool(false), nil
			}
		case "greaterp":
			if !(a > b) {
				return vbool(false), nil
			}
		case "not-lessp":
			if !(a >= b) {
				return vbool(false), nil
			}
		case "not-greaterp":
			if !(a <= b) {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}

func builtinCharLessp(args []*Value) (*Value, error)    { return charCompareCI("lessp", args) }
func builtinCharGreaterp(args []*Value) (*Value, error) { return charCompareCI("greaterp", args) }
func builtinCharNotLessp(args []*Value) (*Value, error) { return charCompareCI("not-lessp", args) }
func builtinCharNotGreaterp(args []*Value) (*Value, error) {
	return charCompareCI("not-greaterp", args)
}

// builtinCharNotEq - ANSI CL char/=: all pairs must be distinct (case-sensitive)
func builtinCharNotEq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("char/=: need at least two characters")
	}
	runes := make([]rune, len(args))
	for i, a := range args {
		if a.typ != VChar {
			return nil, fmt.Errorf("char/=: expected a character")
		}
		runes[i] = a.ch
	}
	// All pairs must be distinct (not just adjacent)
	for i := 0; i < len(runes); i++ {
		for j := i + 1; j < len(runes); j++ {
			if runes[i] == runes[j] {
				return vbool(false), nil
			}
		}
	}
	return vbool(true), nil
}
