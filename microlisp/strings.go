package microlisp

import (
	"fmt"
	"strings"
	"unicode"
)

type strCmpKw struct {
	start1 int
	end1   int
	start2 int
	end2   int
}

func builtinStrUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-upcase: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-upcase: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-upcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-upcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	for i := start; i < end; i++ {
		runes[i] = unicode.ToUpper(runes[i])
	}
	return vstr(string(runes)), nil
}

func builtinStrDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-downcase: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-downcase: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-downcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-downcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	for i := start; i < end; i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return vstr(string(runes)), nil
}

func builtinStringTrim(args []*Value) (*Value, error) {
	// CL: (string-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	start := 0
	for _, r := range runes {
		if charSet[r] {
			start++
		} else {
			break
		}
	}
	end := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if charSet[runes[i]] {
			end = i
		} else {
			break
		}
	}
	if start >= end {
		return vstr(""), nil
	}
	return vstr(string(runes[start:end])), nil
}

func builtinStringLeftTrim(args []*Value) (*Value, error) {
	// CL: (string-left-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-left-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-left-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	start := 0
	for _, r := range runes {
		if charSet[r] {
			start++
		} else {
			break
		}
	}
	return vstr(string(runes[start:])), nil
}

func builtinStringRightTrim(args []*Value) (*Value, error) {
	// CL: (string-right-trim char-bag string)
	if len(args) < 2 {
		return nil, fmt.Errorf("string-right-trim: need char-bag and string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("string-right-trim: second argument must be a string")
	}
	s := args[1].str
	cSeq := seqToList(args[0])
	var cb strings.Builder
	for _, v := range cSeq {
		if v.typ == VStr {
			cb.WriteString(v.str)
		} else if v.typ == VChar {
			cb.WriteRune(v.ch)
		}
	}
	chars := cb.String()
	if len(chars) == 0 {
		chars = " \t\n\r"
	}
	charSet := make(map[rune]bool)
	for _, r := range chars {
		charSet[r] = true
	}
	runes := []rune(s)
	end := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		if charSet[runes[i]] {
			end = i
		} else {
			break
		}
	}
	return vstr(string(runes[:end])), nil
}

func builtinStrCapitalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("string-capitalize: expected a string designator")
	}
	s, err := coerceStringDesignator(args[0])
	if err != nil {
		return nil, fmt.Errorf("string-capitalize: %v", err)
	}
	runes := []rune(s)
	start, end := 0, len(runes)
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-capitalize :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "string-capitalize :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	var result strings.Builder
	prevAlpha := false
	for i, r := range runes {
		if i >= start && i < end {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				if !prevAlpha {
					result.WriteRune(unicode.ToUpper(r))
				} else {
					result.WriteRune(unicode.ToLower(r))
				}
				prevAlpha = true
			} else {
				result.WriteRune(r)
				prevAlpha = false
			}
		} else {
			result.WriteRune(r)
		}
	}
	return vstr(result.String()), nil
}

func checkStringArg(v *Value, funcName string) (string, error) {
	v = primaryValue(v)
	s, err := coerceStringDesignator(v)
	if err != nil {
		return "", fmt.Errorf("%s: %v", funcName, err)
	}
	return s, nil
}

func runesSlice(rs []rune, start, end int) []rune {
	if start < 0 {
		start = 0
	}
	if end > len(rs) {
		end = len(rs)
	}
	if start >= end {
		return []rune{}
	}
	return rs[start:end]
}

func parseStrCmpKwArgs(args []*Value, s1len, s2len int) strCmpKw {
	p := strCmpKw{start1: 0, end1: s1len, start2: 0, end2: s2len}
	for i := 2; i < len(args); i++ {
		if args[i].typ != VSym {
			continue
		}
		switch args[i].str {
		case ":START1":
			if i+1 < len(args) {
				i++
				p.start1 = int(toNum(args[i]))
			}
		case ":END1":
			if i+1 < len(args) {
				i++
				p.end1 = int(toNum(args[i]))
			}
		case ":START2":
			if i+1 < len(args) {
				i++
				p.start2 = int(toNum(args[i]))
			}
		case ":END2":
			if i+1 < len(args) {
				i++
				p.end2 = int(toNum(args[i]))
			}
		}
	}
	if p.start1 < 0 {
		p.start1 = 0
	}
	if p.start1 > s1len {
		p.start1 = s1len
	}
	if p.end1 < 0 || p.end1 > s1len {
		p.end1 = s1len
	}
	if p.start2 < 0 {
		p.start2 = 0
	}
	if p.start2 > s2len {
		p.start2 = s2len
	}
	if p.end2 < 0 || p.end2 > s2len {
		p.end2 = s2len
	}
	return p
}

func builtinStrEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	sub1 := runesSlice(r1, p.start1, p.end1)
	sub2 := runesSlice(r2, p.start2, p.end2)
	return vbool(string(sub1) == string(sub2)), nil
}

func builtinStrLess(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string<: need two strings")
	}
	s1, err := checkStringArg(args[0], "string<")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string<")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] < runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] > runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinMakeString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-string: need size")
	}
	size := int(toNum(args[0]))
	initChar := ' '
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym && args[i].str == ":INITIAL-ELEMENT" {
			if i+1 < len(args) {
				i++
				c := args[i]
				if c.typ == VChar {
					initChar = c.ch
				} else if c.typ == VStr && len(c.str) > 0 {
					initChar = []rune(c.str)[0]
				}
			}
		}
	}
	if size <= 0 {
		return vstr(""), nil
	}
	runes := make([]rune, size)
	for i := range runes {
		runes[i] = initChar
	}
	return vstr(string(runes)), nil
}

func vecToString(v *Value) string {
	arr := v.array
	end := len(arr.elements)
	if arr.fillPtr >= 0 && arr.fillPtr <= len(arr.elements) {
		end = arr.fillPtr
	}
	var sb strings.Builder
	for i := 0; i < end; i++ {
		elem := arr.elements[i]
		if elem != nil {
			if elem.typ == VChar {
				sb.WriteRune(elem.ch)
			} else if elem.typ == VStr && len(elem.str) == 1 {
				sb.WriteRune(rune(elem.str[0]))
			}
		}
	}
	return sb.String()
}

func builtinNStringUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-upcase: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-upcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-upcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			runes[i] = unicode.ToUpper(runes[i])
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				arr.elements[i] = vchar(unicode.ToUpper(elem.ch))
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-upcase: need a string")
	}
}

func builtinNStringDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-downcase: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-downcase :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-downcase :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				arr.elements[i] = vchar(unicode.ToLower(elem.ch))
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-downcase: need a string")
	}
}

func builtinNStringCapitalize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("nstring-capitalize: need a string")
	}
	v := primaryValue(args[0])
	start, end := 0, -1
	for i := 1; i < len(args); i++ {
		if args[i].typ == VSym {
			switch args[i].str {
			case ":START":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-capitalize :start")
					if err != nil {
						return nil, err
					}
					start = int(n)
				}
			case ":END":
				if i+1 < len(args) {
					i++
					n, err := safeToNum(args[i], "nstring-capitalize :end")
					if err != nil {
						return nil, err
					}
					end = int(n)
				}
			}
		}
	}
	switch v.typ {
	case VStr:
		runes := []rune(v.str)
		e := len(runes)
		if end >= 0 && end < e {
			e = end
		}
		prevAlpha := false
		for i := start; i < e; i++ {
			if (runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= 'A' && runes[i] <= 'Z') {
				if !prevAlpha {
					runes[i] = unicode.ToUpper(runes[i])
				} else {
					runes[i] = unicode.ToLower(runes[i])
				}
				prevAlpha = true
			} else {
				prevAlpha = false
			}
		}
		v.str = string(runes)
		return v, nil
	case VArray:
		arr := v.array
		e := len(arr.elements)
		if arr.fillPtr >= 0 && arr.fillPtr < e {
			e = arr.fillPtr
		}
		if end >= 0 && end < e {
			e = end
		}
		prevAlpha := false
		for i := start; i < e; i++ {
			elem := arr.elements[i]
			if elem != nil && elem.typ == VChar {
				r := elem.ch
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
					if !prevAlpha {
						arr.elements[i] = vchar(unicode.ToUpper(r))
					} else {
						arr.elements[i] = vchar(unicode.ToLower(r))
					}
					prevAlpha = true
				} else {
					prevAlpha = false
				}
			} else {
				prevAlpha = false
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("nstring-capitalize: need a string")
	}
}

func titleCaseString(s string) string {
	var result []rune
	capitalize := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			capitalize = true
		}
		if capitalize && unicode.IsLetter(r) {
			result = append(result, unicode.ToTitle(r))
			capitalize = false
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func builtinStrEqualCI(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-equal: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-equal")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-equal")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	sub1 := runesSlice(r1, p.start1, p.end1)
	sub2 := runesSlice(r2, p.start2, p.end2)
	if len(sub1) != len(sub2) {
		return vbool(false), nil
	}
	for i := range sub1 {
		if unicode.ToLower(sub1[i]) != unicode.ToLower(sub2[i]) {
			return vbool(false), nil
		}
	}
	return vbool(true), nil
}

func builtinStringNotEqual(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-equal: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-equal")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-equal")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	if len(runes1) != len(runes2) {
		return vnum(float64(0)), nil
	}
	for i := 0; i < len(runes1); i++ {
		if unicode.ToLower(runes1[i]) != unicode.ToLower(runes2[i]) {
			return vnum(float64(i)), nil
		}
	}
	return vnil(), nil
}

func builtinStringGreaterp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-greaterp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-greaterp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-greaterp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 > c2 {
			return vnum(float64(i)), nil
		}
		if c1 < c2 {
			return vnil(), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

func builtinStringLessp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-lessp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-lessp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-lessp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 < c2 {
			return vnum(float64(i)), nil
		}
		if c1 > c2 {
			return vnil(), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinStringNotGreaterp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-greaterp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-greaterp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-greaterp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 > c2 {
			return vnil(), nil
		}
		if c1 < c2 {
			return vnum(float64(i)), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnil(), nil
	}
	return vnum(float64(len(runes1))), nil
}

func builtinStringNotLessp(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-not-lessp: need two strings")
	}
	s1, err := checkStringArg(args[0], "string-not-lessp")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string-not-lessp")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		c1 := unicode.ToLower(runes1[i])
		c2 := unicode.ToLower(runes2[i])
		if c1 < c2 {
			return vnil(), nil
		}
		if c1 > c2 {
			return vnum(float64(i)), nil
		}
	}
	if len(runes1) < len(runes2) {
		return vnil(), nil
	}
	return vnum(float64(len(runes2))), nil
}

func builtinPrin1ToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("prin1-to-string: need an object")
	}
	return vstr(ToString(primaryValue(args[0]))), nil
}

func builtinPrincToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("princ-to-string: need an object")
	}
	return vstr(princToString(primaryValue(args[0]))), nil
}

func builtinStringElt(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string-elt: need string and index")
	}
	s, err := checkStringArg(args[0], "string-elt")
	if err != nil {
		return nil, err
	}
	idx := int(toNum(args[1]))
	runes := []rune(s)
	if idx < 0 || idx >= len(runes) {
		return nil, fmt.Errorf("string-elt: index %d out of bounds", idx)
	}
	return vchar(runes[idx]), nil
}

func builtinStringNotEq(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string/=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string/=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string/=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	if string(runes1) == string(runes2) {
		return vnil(), nil
	}
	minLen := len(runes1)
	if len(runes2) < minLen {
		minLen = len(runes2)
	}
	for i := 0; i < minLen; i++ {
		if runes1[i] != runes2[i] {
			return vnum(float64(i)), nil
		}
	}
	return vnum(float64(minLen)), nil
}

func builtinStringGreater(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string>: need two strings")
	}
	s1, err := checkStringArg(args[0], "string>")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string>")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] > runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] < runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) > len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}

func builtinStringLe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string<=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string<=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string<=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] < runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] > runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) <= len(runes2) {
		return vnum(float64(len(runes1))), nil
	}
	return vnil(), nil
}

func builtinStringGe(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("string>=: need two strings")
	}
	s1, err := checkStringArg(args[0], "string>=")
	if err != nil {
		return nil, err
	}
	s2, err := checkStringArg(args[1], "string>=")
	if err != nil {
		return nil, err
	}
	r1 := []rune(s1)
	r2 := []rune(s2)
	p := parseStrCmpKwArgs(args, len(r1), len(r2))
	runes1 := runesSlice(r1, p.start1, p.end1)
	runes2 := runesSlice(r2, p.start2, p.end2)
	for i := 0; i < len(runes1) && i < len(runes2); i++ {
		if runes1[i] > runes2[i] {
			return vnum(float64(i)), nil
		}
		if runes1[i] < runes2[i] {
			return vnil(), nil
		}
	}
	if len(runes1) >= len(runes2) {
		return vnum(float64(len(runes2))), nil
	}
	return vnil(), nil
}
