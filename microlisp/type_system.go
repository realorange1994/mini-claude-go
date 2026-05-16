package microlisp

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func builtinTypeOf(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("type-of: need 1 argument")
	}
	return vsym(typeStr(args[0])), nil
}

func builtinDescribe(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("describe: need 1 argument")
	}
	obj := args[0]
	var sb strings.Builder
	sb.WriteString(ToString(obj))
	sb.WriteString(" is of type ")
	sb.WriteString(typeStr(obj))
	sb.WriteString("\n")
	switch obj.typ {
	case VNil:
		sb.WriteString("It is the canonical false value (nil).\n")
	case VBool:
		if obj == globalEnv.bindings["#t"] {
			sb.WriteString("It is the canonical true value (t).\n")
		} else {
			sb.WriteString("It is the canonical false value (nil).\n")
		}
	case VNum, VRat, VComplex:
		sb.WriteString("Value: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nType: ")
		sb.WriteString(typeStr(obj))
		sb.WriteString("\n")
	case VStr:
		sb.WriteString("Value: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nLength: ")
		sb.WriteString(strconv.Itoa(len(obj.str)))
		sb.WriteString("\n")
	case VChar:
		sb.WriteString("Character: ")
		sb.WriteString(ToString(obj))
		sb.WriteString("\nCode: ")
		sb.WriteString(strconv.Itoa(int(obj.ch)))
		sb.WriteString("\n")
	case VSym:
		sb.WriteString("Name: ")
		sb.WriteString(obj.str)
		sb.WriteString("\n")
		if obj.fn != nil {
			sb.WriteString("It has a function binding.\n")
		}
		val, err := globalEnv.Get(obj.str)
		if err == nil {
			sb.WriteString("Value: ")
			sb.WriteString(ToString(val))
			sb.WriteString("\n")
		}
		if val != nil && val.typ == VMacro {
			sb.WriteString("It is a macro.\n")
		}
	case VPair:
		length := 0
		cur := obj
		for cur != nil && cur.typ == VPair {
			length++
			cur = cur.cdr
		}
		sb.WriteString("It is a cons of length ")
		sb.WriteString(strconv.Itoa(length))
		sb.WriteString(".\nCar: ")
		sb.WriteString(ToString(obj.car))
		sb.WriteString("\nCdr: ")
		sb.WriteString(ToString(obj.cdr))
		sb.WriteString("\n")
	case VArray:
		if len(obj.array.dims) == 1 && obj.array.fillPtr < 0 {
			sb.WriteString("It is a vector.\n")
		} else {
			sb.WriteString("It is an array.\n")
		}
		sb.WriteString("Dimensions: ")
		for i, d := range obj.array.dims {
			if i > 0 {
				sb.WriteString(" x ")
			}
			sb.WriteString(strconv.Itoa(d))
		}
		sb.WriteString("\n")
	case VInstance:
		if obj.instClass != nil {
			sb.WriteString("It is an instance of class ")
			sb.WriteString(obj.instClass.str)
			sb.WriteString(".\nSlots:\n")
			for name, val := range obj.instSlots {
				sb.WriteString("  ")
				sb.WriteString(name)
				sb.WriteString(" = ")
				sb.WriteString(ToString(val))
				sb.WriteString("\n")
			}
		}
	case VClass:
		if obj.str != "" {
			sb.WriteString("It is a class named ")
			sb.WriteString(obj.str)
			sb.WriteString(".\n")
		}
		if obj.classParents != nil {
			parents := make([]string, len(obj.classParents))
			for i, p := range obj.classParents {
				if p != nil && p.str != "" {
					parents[i] = p.str
				} else {
					parents[i] = "(unknown)"
				}
			}
			sb.WriteString("Superclasses: ")
			for i, p := range parents {
				if i > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(p)
			}
			sb.WriteString("\n")
		}
	case VFunc:
		sb.WriteString("It is a function.\n")
		if obj.name != "" {
			sb.WriteString("Name: ")
			sb.WriteString(obj.name)
			sb.WriteString("\n")
		}
		sb.WriteString("Arity: ")
		sb.WriteString(strconv.Itoa(len(obj.params)))
		sb.WriteString("\n")
	case VPrim:
		sb.WriteString("It is a built-in function.\n")
	case VMacro:
		sb.WriteString("It is a macro.\n")
	case VStream:
		sb.WriteString("It is a stream.\n")
		if obj.stream.isInput {
			sb.WriteString("Direction: input\n")
		}
		if obj.stream.isOutput {
			sb.WriteString("Direction: output\n")
		}
	case VVHash:
		sb.WriteString("It is a hash-table.\n")
		sb.WriteString("Size: ")
		sb.WriteString(strconv.Itoa(obj.hashTab.count))
		sb.WriteString("\n")
	}
	return vstr(sb.String()), nil
}

func builtinRoom(args []*Value) (*Value, error) {
	var verbose bool
	if len(args) > 0 {
		verbose = !isNil(args[0])
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(os.Stderr, "Dynamic Space Usage: %d bytes allocated, %d bytes total\n", m.Alloc, m.TotalAlloc)
	if verbose {
		fmt.Fprintf(os.Stderr, "  HeapSys: %d bytes\n", m.HeapSys)
		fmt.Fprintf(os.Stderr, "  HeapAlloc: %d bytes\n", m.HeapAlloc)
		fmt.Fprintf(os.Stderr, "  HeapIdle: %d bytes\n", m.HeapIdle)
		fmt.Fprintf(os.Stderr, "  HeapReleased: %d bytes\n", m.HeapReleased)
		fmt.Fprintf(os.Stderr, "  NumGC: %d\n", m.NumGC)
	}
	return vnil(), nil
}

func builtinMakeLoadForm(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-load-form: need an object")
	}
	// Default: return (make-load-form-saving-slots object) if the object is a structure
	if args[0].typ == VInstance {
		return vsym("MAKE-LOAD-FORM-SAVING-SLOTS"), nil
	}
	return vnil(), fmt.Errorf("make-load-form: cannot make load form for %s", typeStr(args[0]))
}

func builtinMakeLoadFormSavingSlots(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("make-load-form-saving-slots: need an object")
	}
	if args[0].typ != VInstance {
		return nil, fmt.Errorf("make-load-form-saving-slots: expected a structure")
	}
	inst := args[0]
	slotNames := listFromSlice([]*Value{})
	if inst.instClass != nil {
		// Try to get slot names from the class
		class := globalEnv.bindings[inst.instClass.str]
		if class != nil && class.typ == VClass {
			for _, sn := range class.classSlots {
				slotNames = cons(vstr(sn), slotNames)
			}
		}
	}
	// Return: (make-load-form-saving-slots-helper object slot-names)
	// For simplicity, return a list form
	return listFromSlice([]*Value{vsym("MAKE-LOAD-FORM-SAVING-SLOTS-HELPERS"), inst, slotNames}), nil
}

var docstrings = make(map[string]string)

func builtinDocumentation(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("documentation: need symbol and doc-type")
	}
	sym := args[0]
	if sym.typ != VSym {
		return vnil(), nil
	}
	docType := ""
	if args[1].typ == VSym {
		docType = args[1].str
	}
	key := sym.str + "_" + docType
	if doc, ok := docstrings[key]; ok {
		return vstr(doc), nil
	}
	return vnil(), nil
}

func builtinApropos(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("apropos: need a string")
	}
	searchStr := ""
	if args[0].typ == VStr {
		searchStr = args[0].str
	} else if args[0].typ == VSym {
		searchStr = args[0].str
	} else {
		searchStr = ToString(args[0])
	}
	var results []*Value
	for name := range globalEnv.bindings {
		if strings.Contains(strings.ToLower(name), strings.ToLower(searchStr)) {
			results = append(results, vsym(name))
		}
	}
	// Sort results
	sort.Slice(results, func(i, j int) bool {
		return results[i].str < results[j].str
	})
	return listFromSlice(results), nil
}

func builtinAproposList(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("apropos-list: need a string")
	}
	searchStr := ""
	if args[0].typ == VStr {
		searchStr = args[0].str
	} else if args[0].typ == VSym {
		searchStr = args[0].str
	} else {
		searchStr = ToString(args[0])
	}
	var results []*Value
	for name := range globalEnv.bindings {
		if strings.Contains(strings.ToLower(name), strings.ToLower(searchStr)) {
			results = append(results, vsym(name))
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].str < results[j].str
	})
	return listFromSlice(results), nil
}

func builtinCompile(args []*Value) (*Value, error) {
	// (compile name &optional definition)
	// In SBCL, compile returns (values compiled-fn warnings-p failure-p)
	// For MicroLisp, we return the function as-is (it's already interpreted)
	var name *Value
	var def *Value
	if len(args) >= 1 {
		name = args[0]
	}
	if len(args) >= 2 {
		def = args[1]
	}
	if name != nil && name.typ == VNil {
		name = nil
	}
	if name != nil && name.typ == VSym && def != nil {
		// Compile a lambda definition
		if def.typ == VPair && def.car != nil && def.car.typ == VSym && def.car.str == "LAMBDA" {
			// It's a lambda, eval it and return
			result, err := Eval(def, globalEnv)
			if err != nil {
				return vnil(), nil
			}
			// Set the function binding
			if name.typ == VSym {
				globalEnv.Set(name.str, result)
			}
			// Return (values result nil nil)
			return multiVal(result, vnil(), vnil()), nil
		}
	}
	if name != nil && name.typ == VSym {
		// Look up existing function
		val, err := globalEnv.Get(name.str)
		if err == nil {
			// Return (values val nil nil)
			return multiVal(val, vnil(), vnil()), nil
		}
	}
	if def != nil && def.typ == VPair && def.car != nil && def.car.typ == VSym && def.car.str == "LAMBDA" {
		result, err := Eval(def, globalEnv)
		if err != nil {
			return vnil(), nil
		}
		return multiVal(result, vnil(), vnil()), nil
	}
	return vnil(), nil
}

func builtinDisassemble(args []*Value) (*Value, error) {
	// (disassemble fn) -- returns a string describing the function
	if len(args) < 1 {
		return nil, fmt.Errorf("disassemble: need a function")
	}
	fn := primaryValue(args[0])
	var sb strings.Builder
	switch fn.typ {
	case VNil:
		sb.WriteString("#<NULL>\n")
	case VPrim:
		sb.WriteString("; This is a built-in function.\n")
		sb.WriteString("; Disassembly not available for primitives.\n")
	case VFunc:
		sb.WriteString("; Lambda Expression:\n")
		if fn.name != "" {
			sb.WriteString("; Name: ")
			sb.WriteString(fn.name)
			sb.WriteString("\n")
		}
		sb.WriteString("; Parameters: (")
		for i, p := range fn.params {
			if i > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(p)
		}
		if fn.rest != "" {
			if len(fn.params) > 0 {
				sb.WriteString(" &rest ")
			} else {
				sb.WriteString("&rest ")
			}
			sb.WriteString(fn.rest)
		}
		sb.WriteString(")\n")
		sb.WriteString("; Environment: (closure)\n")
		sb.WriteString("; Compiled: No (interpreted)\n")
	case VMacro:
		sb.WriteString("; This is a macro.\n")
		sb.WriteString("; Name: ")
		sb.WriteString(fn.name)
		sb.WriteString("\n")
	case VInstance:
		if fn.instClass != nil {
			sb.WriteString("; Instance of class: ")
			sb.WriteString(fn.instClass.str)
			sb.WriteString("\n")
		}
	default:
		sb.WriteString("; Not a function.\n")
	}
	fmt.Print(sb.String())
	return vnil(), nil
}

func builtinReplace(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("replace: need at least two sequences")
	}
	target := args[0]
	source := args[1]
	start1 := 0
	end1 := -1
	start2 := 0
	end2 := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START1":
				start1 = int(primaryValue(args[i+1]).num)
			case ":END1":
				end1 = int(primaryValue(args[i+1]).num)
			case ":START2":
				start2 = int(primaryValue(args[i+1]).num)
			case ":END2":
				end2 = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch target.typ {
	case VStr:
		ts := []rune(target.str)
		tLen := len(ts)
		if end1 < 0 || end1 > tLen {
			end1 = tLen
		}
		if start1 < 0 {
			start1 = 0
		}
		ss := []rune(source.str)
		sLen := len(ss)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			ts[i] = ss[j]
			j++
		}
		target.str = string(ts)
		return target, nil
	case VArray:
		te := target.array.elements
		tLen := len(te)
		if end1 < 0 || end1 > tLen {
			end1 = tLen
		}
		if start1 < 0 {
			start1 = 0
		}
		se := seqToList(source)
		sLen := len(se)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		j := start2
		for i := start1; i < end1 && j < end2; i++ {
			te[i] = se[j]
			j++
		}
		return target, nil
	case VPair:
		// For lists, rebuild with replaced portion
		if end1 < 0 {
			end1 = 999999
		}
		result := vnil()
		var tail **Value = &result
		idx := 0
		srcList := seqToList(source)
		sLen := len(srcList)
		if end2 < 0 || end2 > sLen {
			end2 = sLen
		}
		if start2 < 0 {
			start2 = 0
		}
		for i := 0; i < start2; i++ {
			// Copy source elements before replacement
		}
		cur := target
		idx = 0
		for !isNil(cur) && idx < start1 {
			*tail = cons(cur.car, vnil())
			tail = &((*tail).cdr)
			cur = cur.cdr
			idx++
		}
		// Skip target elements in range
		for !isNil(cur) && idx < end1 {
			cur = cur.cdr
			idx++
		}
		// Insert source elements
		for j := start2; j < end2 && j < sLen; j++ {
			*tail = cons(srcList[j], vnil())
			tail = &((*tail).cdr)
		}
		// Append remaining target
		for !isNil(cur) {
			*tail = cons(cur.car, vnil())
			tail = &((*tail).cdr)
			cur = cur.cdr
		}
		*tail = vnil()
		return result, nil
	default:
		return nil, fmt.Errorf("replace: not a sequence")
	}
}

func builtinParseInteger(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("parse-integer: need a string")
	}
	strVal := primaryValue(args[0])
	var str string
	if strVal.typ == VStr {
		str = strVal.str
	} else {
		str = ToString(strVal)
	}
	start := 0
	end := -1
	radix := 10
	junkAllowed := false
	for i := 1; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			case ":RADIX":
				radix = int(primaryValue(args[i+1]).num)
			case ":JUNK-ALLOWED":
				junkAllowed = !isNil(args[i+1])
			}
		}
	}
	if end < 0 {
		end = len(str)
	}
	s := strings.TrimSpace(str[start:end])
	s = strings.ReplaceAll(s, "_", "")
	if s == "" || s == "-" || s == "+" {
		if junkAllowed {
			return vnil(), nil
		}
		return nil, fmt.Errorf("parse-integer: no integer at position %d", start)
	}
	// Position: count from start, skip whitespace then count sign+digits
	pos := start
	for pos < len(str) && (str[pos] == ' ' || str[pos] == '\t' || str[pos] == '\n' || str[pos] == '\r') {
		pos++
	}
	// s contains sign+digits (or just digits); len(s) counts all
	pos += len(s)

	n, err := strconv.ParseInt(s, radix, 64)
	if err != nil {
		// Try to parse as much as possible for junk-allowed
		if junkAllowed {
			// Find longest valid prefix of s
			for l := len(s); l > 0; l-- {
				partial, e2 := strconv.ParseInt(s[:l], radix, 64)
				if e2 == nil {
					p := start
					for p < len(str) && (str[p] == ' ' || str[p] == '\t' || str[p] == '\n' || str[p] == '\r') {
						p++
					}
					p += l
					return multiVal(vnum(float64(partial)), vnum(float64(p))), nil
				}
			}
			return vnil(), nil
		}
		return nil, fmt.Errorf("parse-integer: not an integer")
	}
	return multiVal(vnum(float64(n)), vnum(float64(pos))), nil
}

func builtinDigitCharP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vnil(), nil
	}
	radix := 10
	if len(args) > 1 {
		radix = int(toNum(args[1]))
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return vnil(), nil
	}
	d := unicode.ToLower(c.ch)
	baseDigit := '0'
	radixChar := rune('a' + radix - 10)
	if d >= baseDigit && d < baseDigit+rune(radix) {
		return vnum(float64(int(d - baseDigit))), nil
	}
	if d >= 'a' && d < radixChar {
		return vnum(float64(int(d - 'a' + 10))), nil
	}
	return vnil(), nil
}

func builtinAlphanumericP(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return vbool(false), nil
	}
	c := primaryValue(args[0])
	if c.typ == VChar {
		return vbool(unicode.IsLetter(c.ch) || unicode.IsDigit(c.ch)), nil
	}
	return vbool(false), nil
}

func builtinCharUpcase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-upcase: need a character")
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return nil, fmt.Errorf("char-upcase: not a character")
	}
	return vchar(unicode.ToUpper(c.ch)), nil
}

func builtinCharDowncase(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("char-downcase: need a character")
	}
	c := primaryValue(args[0])
	if c.typ != VChar {
		return nil, fmt.Errorf("char-downcase: not a character")
	}
	return vchar(unicode.ToLower(c.ch)), nil
}

func builtinReadSequence(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("read-sequence: need sequence and stream")
	}
	seq := args[0]
	stream := args[1]
	if stream.typ != VStream {
		return nil, fmt.Errorf("read-sequence: second argument must be a stream")
	}
	start := 0
	end := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch seq.typ {
	case VStr:
		s := seq.str
		runes := []rune(s)
		total := len(runes)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		count := 0
		for i := start; i < end; i++ {
			r, err := stream.stream.readChar()
			if err != nil {
				break
			}
			runes[i] = r
			count++
		}
		seq.str = string(runes)
		return vnum(float64(start + count)), nil
	case VArray:
		arr := seq.array
		total := len(arr.elements)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		count := 0
		for i := start; i < end; i++ {
			r, err := stream.stream.readChar()
			if err != nil {
				break
			}
			arr.elements[i] = vstr(string(r))
			count++
		}
		return vnum(float64(start + count)), nil
	case VPair:
		lst := seq
		seen := make(map[*Value]bool)
		idx := 0
		count := 0
		for !isNil(lst) {
			if seen[lst] {
				break
			}
			seen[lst] = true
			if idx >= start && (end < 0 || idx < end) {
				r, err := stream.stream.readChar()
				if err != nil {
					break
				}
				lst.car = vstr(string(r))
				count++
			}
			lst = lst.cdr
			idx++
		}
		return vnum(float64(start + count)), nil
	default:
		return nil, fmt.Errorf("read-sequence: not a sequence")
	}
}

func builtinWriteSequence(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("write-sequence: need a sequence")
	}
	seq := args[0]
	stream := stdoutStream
	if len(args) > 1 {
		stream = args[1]
		if stream.typ != VStream {
			return nil, fmt.Errorf("write-sequence: not a stream")
		}
	}
	start := 0
	end := -1
	for i := 2; i+1 < len(args); i += 2 {
		key := primaryValue(args[i])
		if key.typ == VSym {
			switch key.str {
			case ":START":
				start = int(primaryValue(args[i+1]).num)
			case ":END":
				end = int(primaryValue(args[i+1]).num)
			}
		}
	}
	switch seq.typ {
	case VStr:
		s := seq.str
		if end < 0 || end > len(s) {
			end = len(s)
		}
		if start < 0 {
			start = 0
		}
		if err := stream.stream.writeString(s[start:end]); err != nil {
			return nil, err
		}
		stream.stream.flush()
		return seq, nil
	case VArray:
		arr := seq.array
		total := len(arr.elements)
		if end < 0 || end > total {
			end = total
		}
		if start < 0 {
			start = 0
		}
		for i := start; i < end; i++ {
			if err := stream.stream.writeString(ToString(primaryValue(arr.elements[i]))); err != nil {
				return nil, err
			}
		}
		stream.stream.flush()
		return seq, nil
	case VPair:
		lst := seq
		seen := make(map[*Value]bool)
		idx := 0
		for !isNil(lst) {
			if seen[lst] {
				break
			}
			seen[lst] = true
			if idx >= start && (end < 0 || idx < end) {
				if err := stream.stream.writeString(ToString(primaryValue(lst.car))); err != nil {
					return nil, err
				}
			}
			lst = lst.cdr
			idx++
		}
		stream.stream.flush()
		return seq, nil
	default:
		return nil, fmt.Errorf("write-sequence: not a sequence")
	}
}

func subtypepChecks(v1, v2 *Value) (bool, bool) {
	typeName := func(v *Value) string {
		if v.typ == VSym {
			return v.str
		}
		if v.typ == VPair && v.car != nil && v.car.typ == VSym {
			return v.car.str
		}
		return ""
	}

	simpleSubtype := func(t1, t2 string) bool {
		if t1 == t2 {
			return true
		}
		types := map[string][]string{"INTEGER": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FLOAT": {"REAL", "NUMBER", "ATOM", "T"}, "RATIONAL": {"REAL", "NUMBER", "ATOM", "T"}, "COMPLEX": {"NUMBER", "ATOM", "T"}, "REAL": {"NUMBER", "ATOM", "T"}, "RATIO": {"RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "FIXNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIGNUM": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "BIT": {"INTEGER", "RATIONAL", "REAL", "NUMBER", "ATOM", "T"}, "SHORT-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "SINGLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "DOUBLE-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "LONG-FLOAT": {"FLOAT", "REAL", "NUMBER", "ATOM", "T"}, "STRING": {"ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "SIMPLE-STRING": {"STRING", "ARRAY", "VECTOR", "SEQUENCE", "ATOM", "T"}, "CHARACTER": {"ATOM", "T"}, "BASE-CHAR": {"CHARACTER", "ATOM", "T"}, "STANDARD-CHAR": {"BASE-CHAR", "CHARACTER", "ATOM", "T"}, "EXTENDED-CHAR": {"CHARACTER", "ATOM", "T"}, "SYMBOL": {"ATOM", "T"}, "KEYWORD": {"SYMBOL", "ATOM", "T"}, "NULL": {"SYMBOL", "LIST", "SEQUENCE", "ATOM", "T"}, "CONS": {"LIST", "SEQUENCE", "T"}, "PAIR": {"CONS", "LIST", "SEQUENCE", "T"}, "LIST": {"SEQUENCE", "T"}, "SEQUENCE": {"T"}, "VECTOR": {"ARRAY", "SEQUENCE", "T"}, "SIMPLE-VECTOR": {"VECTOR", "ARRAY", "SEQUENCE", "T"}, "ARRAY": {"T"}, "FUNCTION": {"T"}, "COMPILED-FUNCTION": {"FUNCTION", "T"}, "HASH-TABLE": {"T"}, "STREAM": {"T"}, "PACKAGE": {"T"}, "PATHNAME": {"T"}, "RANDOM-STATE": {"T"}, "READTABLE": {"T"}, "INSTANCE": {"T"}, "STRUCTURE": {"INSTANCE", "T"}, "METHOD": {"T"}, "BOOLEAN": {"ATOM", "T"}}
		if subtypes, ok := types[t1]; ok {
			for _, s := range subtypes {
				if s == t2 {
					return true
				}
			}
		}
		return false
	}

	n1 := strings.ToUpper(typeName(v1))
	n2 := strings.ToUpper(typeName(v2))

	// If both are simple type names
	if v1.typ == VSym && v2.typ == VSym {
		if n1 == n2 {
			return true, true
		}
		// * is universal type
		if n1 == "*" {
			if n2 == "*" || n2 == "T" {
				return true, true
			}
			return true, false
		}
		if n2 == "*" || n2 == "T" {
			return true, true
		}
		n1u := strings.ToUpper(n1)
		n2u := strings.ToUpper(n2)
		if simpleSubtype(n1u, n2u) {
			return true, true
		}
		cls1 := findClass(n1)
		cls2 := findClass(n2)
		if cls1 != nil && cls2 != nil {
			if classHasAncestor(cls1, n2) {
				return true, true
			}
		}
		return true, false
	}

	// Handle compound (cons ...) types
	if n1 == "CONS" || n2 == "CONS" {
		extractConsParts := func(v *Value) (carType *Value, cdrType *Value) {
			cdrType = vsym("*")
			if v.typ == VSym {
				carType = vsym("*")
				return
			}
			if v.typ == VPair && v.car != nil && v.car.typ == VSym && v.car.str == "CONS" {
				rest := v.cdr
				if rest != nil && !isNil(rest) && rest.typ == VPair {
					carType = rest.car
					rest = rest.cdr
					if rest != nil && !isNil(rest) && rest.typ == VPair {
						cdrType = rest.car
					}
				} else {
					carType = vsym("*")
				}
			}
			return
		}

		ct1, cd1 := extractConsParts(v1)
		ct2, cd2 := extractConsParts(v2)

		if ct1 != nil && ct2 != nil {
			isCarKnown, isCarSub := subtypepChecks(ct1, ct2)
			isCdrKnown, isCdrSub := subtypepChecks(cd1, cd2)
			if !isCarKnown || !isCdrKnown {
				return false, false
			}
			if isCarSub && isCdrSub {
				return true, true
			}
			return true, false
		}
		return false, false
	}

	// If t2 is t or *, anything is a subtype
	if n2 == "T" || n2 == "*" {
		return true, true
	}

	return false, false
}

func builtinSubtypep(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return multiVal(vbool(false), vbool(false)), nil
	}
	t1v := primaryValue(args[0])
	t2v := primaryValue(args[1])

	isSub, result := subtypepChecks(t1v, t2v)
	if isSub && result {
		return multiVal(vbool(true), vbool(true)), nil
	}
	if isSub && !result {
		return multiVal(vbool(false), vbool(true)), nil
	}
	return multiVal(vbool(false), vbool(false)), nil
}
