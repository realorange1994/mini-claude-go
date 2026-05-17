package microlisp

import (
	"math"
	"strconv"
	"strings"
)

func princToString(v *Value) string {
	switch v.typ {
	case VBigInt:
		return v.bigInt.String()
	case VStr:
		return v.str
	case VChar:
		return string(v.ch)
	case VNil:
		return "nil"
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "#t"
		}
		return "#f"
	case VPair, VArray:
		// Use writeToString for circular reference detection
		return writeToString(v)
	case VInstance:
		// Check for defstruct :print-function / :print-object first
		if v.instClass != nil {
			if printFn, ok := structPrintFns[strings.ToUpper(v.instClass.str)]; ok && printFn != nil {
				// Create a string output stream and call the print function
				outStream := newStringOutput()
				_, callErr := callFnOnSeq(printFn, []*Value{v, outStream, vnum(0)}, globalEnv)
				if callErr == nil {
					return outStream.stream.strBuf.String()
				}
				// If print function fails, fall through to default
			}
		}
		// Check if this is a condition instance with format-control/format-arguments
		if v.instClass != nil && classHasAncestor(v.instClass, "condition") {
			fc := instanceSlotWithInheritance(v, "format-control")
			fa := instanceSlotWithInheritance(v, "format-arguments")
			if fc != nil && fc.typ == VStr && len(fc.str) > 0 {
				var args []*Value
				if fa != nil && fa.typ != VNil {
					cur := fa
					for cur.typ == VPair {
						args = append(args, cur.car)
						cur = cur.cdr
					}
				}
				return formatMessage(fc.str, args)
			}
			// Fallback: check for MESSAGE slot directly
			msg := instanceSlotWithInheritance(v, "message")
			if msg != nil && msg.typ == VStr {
				return msg.str
			}
		}
		return writeToString(v)
	default:
		return ToString(v)
	}
}

// -------- Printer --------
func ToString(v *Value) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VNum:
		f := v.num
		if v.isFloat {
			// explicitly a float: always print with decimal point
			s := strconv.FormatFloat(f, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eE") {
				s += ".0"
			}
			return s
		}
		if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	case VStr:
		return `"` + strings.ReplaceAll(v.str, `"`, `\"`) + `"`
	case VSym:
		return v.str
	case VBool:
		if v == globalEnv.bindings["#t"] {
			return "#t"
		}
		return "#f"
	case VPair:
		return listToString(v)
	case VPrim:
		return "#<primitive>"
	case VFunc:
		return "#<procedure>"
	case VMacro:
		return "#<macro>"
	case VRat:
		return strconv.FormatInt(v.irat, 10) + "/" + strconv.FormatInt(v.iden, 10)
	case VComplex:
		r := formatComplexPart(v.num, v.isFloat)
		i := formatComplexPart(v.imag, v.isFloat)
		return "#c(" + r + " " + i + ")"
	case VBigInt:
		return v.bigInt.String()
	case VPathname:
		return "#P\"" + pathnameToString(v.pathname) + "\""
	case VPackage:
		if v.pkg != nil {
			return "#<PACKAGE " + v.pkg.name + ">"
		}
		return "#<PACKAGE>"
	case VReadtable:
		return "#<READTABLE>"
	case VClass:
		return "#<class " + v.str + ">"
	case VGeneric:
		return "#<generic " + v.str + ">"
	case VInstance:
		if v.instClass != nil {
			if printFn, ok := structPrintFns[strings.ToUpper(v.instClass.str)]; ok && printFn != nil {
				outStream := newStringOutput()
				_, callErr := callFnOnSeq(printFn, []*Value{v, outStream, vnum(0)}, globalEnv)
				if callErr == nil {
					return outStream.stream.strBuf.String()
				}
			}
			return "#<instance " + v.instClass.str + ">"
		}
		return "#<instance>"
	case VVHash:
		return "#<hash-table " + strconv.Itoa(v.hashTab.count) + ">"
	case VThread:
		return "#<thread " + strconv.FormatInt(int64(v.num), 10) + ">"
	case VLock:
		return "#<lock " + strconv.FormatInt(int64(v.num), 10) + ">"
	case VChar:
		switch v.ch {
		case ' ':
			return "#\\space"
		case '\n':
			return "#\\newline"
		case '\t':
			return "#\\tab"
		case '\r':
			return "#\\return"
		case '\x08':
			return "#\\backspace"
		case '\x7f':
			return "#\\rubout"
		case '\f':
			return "#\\page"
		case '\x00':
			return "#\\null"
		case '\x07':
			return "#\\bell"
		case '\x1b':
			return "#\\escape"
		case '\x01':
			return "#\\soh"
		case '\x02':
			return "#\\stx"
		case '\x03':
			return "#\\etx"
		case '\x04':
			return "#\\eot"
		case '\x05':
			return "#\\enq"
		case '\x06':
			return "#\\ack"
		case '\x15':
			return "#\\nak"
		case '\x16':
			return "#\\syn"
		case '\x17':
			return "#\\etb"
		case '\x18':
			return "#\\can"
		case '\x19':
			return "#\\em"
		case '\x1a':
			return "#\\sub"
		case '\x1c':
			return "#\\fs"
		case '\x1d':
			return "#\\gs"
		case '\x1e':
			return "#\\rs"
		case '\x1f':
			return "#\\us"
		case '\x11':
			return "#\\xon"
		case '\x13':
			return "#\\xoff"
		default:
			return "#\\" + string(v.ch)
		}
	case VStream:
		return "#<stream>"
	case VArray:
		return arrayToString(v)
	case VMultiVal:
		return listToString(cons(v.car, v.cdr))
	case VRandomState:
		return "#<random-state>"
	case VMethod:
		return "#<method " + v.str + ">"
	case VRestart:
		return "#<restart " + v.str + ">"
	default:
		return "#<unknown>"
	}
}

// -------- Circular structure printing (*print-circle*) --------

// circleState tracks visited values for circular reference detection
type circleState struct {
	seen    map[*Value]int // value -> label number
	counter int
}

// writeToString returns a string representation respecting *print-circle*
func writeToString(v *Value) string {
	pc, _ := globalEnv.Get("*print-circle*")
	useCircle := pc != nil && !isNil(pc)
	// Always detect circular structures to prevent infinite loops
	cs := &circleState{seen: make(map[*Value]int), counter: 1}
	findShared(v, cs, make(map[*Value]bool))
	if useCircle && len(cs.seen) > 0 {
		// Print with #n= and #n# labels
		return toStringCircle(v, cs)
	}
	if len(cs.seen) > 0 {
		// Circular but *print-circle* is nil — truncate with ...
		return toStringSafe(v, cs)
	}
	return ToString(v)
}

// toStringSafe prints with ... for circular references (when *print-circle* is nil)
func toStringSafe(v *Value, cs *circleState) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if _, ok := cs.seen[v]; ok {
			return "..."
		}
		return listToStringSafe(v, cs)
	case VArray:
		if _, ok := cs.seen[v]; ok {
			return "..."
		}
		return arrayToStringSafe(v, cs)
	default:
		return ToString(v)
	}
}

func listToStringSafe(v *Value, cs *circleState) string {
	var b strings.Builder
	b.WriteString("(")
	first := true
	localSeen := make(map[*Value]bool)
	for !isNil(v) {
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringSafe(v, cs))
			break
		}
		if _, ok := cs.seen[v]; ok && !first {
			b.WriteString("...")
			break
		}
		if localSeen[v] {
			b.WriteString("...")
			break
		}
		localSeen[v] = true
		if !first {
			b.WriteString(" ")
		}
		first = false
		b.WriteString(toStringSafe(v.car, cs))
		v = v.cdr
	}
	b.WriteString(")")
	return b.String()
}

func arrayToStringSafe(v *Value, cs *circleState) string {
	if v.array == nil {
		return "#()"
	}
	var b strings.Builder
	b.WriteString("#(")
	for i, elem := range v.array.elements {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(toStringSafe(elem, cs))
	}
	b.WriteString(")")
	return b.String()
}

// findShared walks the structure and marks values referenced more than once (or circularly)
func findShared(v *Value, cs *circleState, visited map[*Value]bool) {
	if v == nil || v.typ == VNil {
		return
	}
	if v.typ == VPair {
		if visited[v] {
			// Already visited — mark as shared/circular
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return // Don't recurse into already-visited
		}
		visited[v] = true
		findShared(v.car, cs, visited)
		findShared(v.cdr, cs, visited)
	} else if v.typ == VArray && v.array != nil {
		if visited[v] {
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return
		}
		visited[v] = true
		for _, elem := range v.array.elements {
			findShared(elem, cs, visited)
		}
	} else if v.typ == VInstance {
		if visited[v] {
			if _, exists := cs.seen[v]; !exists {
				cs.seen[v] = cs.counter
				cs.counter++
			}
			return
		}
		visited[v] = true
		for _, slot := range v.instSlots {
			findShared(slot, cs, visited)
		}
	}
}

// toStringCircle prints with #n= and #n# syntax for circular/shared references
func toStringCircle(v *Value, cs *circleState) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if label, ok := cs.seen[v]; ok {
			// Check if already printed (replace the entry with negative to mark)
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label // mark as printed
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + listToStringCircle(v, cs)
		}
		return listToStringCircle(v, cs)
	case VArray:
		if label, ok := cs.seen[v]; ok {
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + arrayToStringCircle(v, cs)
		}
		return arrayToStringCircle(v, cs)
	case VInstance:
		if label, ok := cs.seen[v]; ok {
			if label < 0 {
				return "#" + strconv.Itoa(-label) + "#"
			}
			cs.seen[v] = -label
			prefix := "#" + strconv.Itoa(label) + "="
			return prefix + ToString(v)
		}
		return ToString(v)
	default:
		return ToString(v)
	}
}

func listToStringCircle(v *Value, cs *circleState) string {
	var b strings.Builder
	b.WriteString("(")
	for !isNil(v) {
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringCircle(v, cs))
			break
		}
		// Check cdr for circular reference before recursing
		if !isNil(v.cdr) {
			if label, ok := cs.seen[v.cdr]; ok {
				// cdr is shared — print (a b . #n#) style
				b.WriteString(toStringCircle(v.car, cs))
				b.WriteString(" . ")
				cs.seen[v.cdr] = -absInt(label) // ensure negative for #n#
				b.WriteString(toStringCircle(v.cdr, cs))
				b.WriteString(")")
				return b.String()
			}
		}
		b.WriteString(toStringCircle(v.car, cs))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

func arrayToStringCircle(v *Value, cs *circleState) string {
	if v.array == nil {
		return "#()"
	}
	var b strings.Builder
	b.WriteString("#(")
	for i, elem := range v.array.elements {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(toStringCircle(elem, cs))
	}
	b.WriteString(")")
	return b.String()
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func listToString(v *Value) string {
	if v == nil || v.typ != VPair {
		return "()"
	}
	var b strings.Builder
	b.WriteString("(")
	seen := make(map[*Value]bool)
	for !isNil(v) {
		if seen[v] {
			b.WriteString("...")
			break
		}
		seen[v] = true
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringWithSeen(v, seen))
			break
		}
		b.WriteString(toStringWithSeen(v.car, seen))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}

// toStringWithSeen is like toString but shares a seen map for cycle detection
func toStringWithSeen(v *Value, seen map[*Value]bool) string {
	if v == nil {
		return "()"
	}
	switch v.typ {
	case VNil:
		return "()"
	case VPair:
		if seen[v] {
			return "..."
		}
		// Don't add v to seen here; listToString will do it
		return listToStringShared(v, seen)
	default:
		return ToString(v)
	}
}

func listToStringShared(v *Value, seen map[*Value]bool) string {
	if v == nil || v.typ != VPair {
		return "()"
	}
	var b strings.Builder
	b.WriteString("(")
	for !isNil(v) {
		if seen[v] {
			b.WriteString("...")
			break
		}
		seen[v] = true
		if v.typ != VPair {
			b.WriteString(" . ")
			b.WriteString(toStringWithSeen(v, seen))
			break
		}
		b.WriteString(toStringWithSeen(v.car, seen))
		v = v.cdr
		if !isNil(v) {
			b.WriteString(" ")
		}
	}
	b.WriteString(")")
	return b.String()
}
