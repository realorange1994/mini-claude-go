package microlisp

import "fmt"

// Byte operations: byte, byte-size, byte-position, ldb, dpb, ldb-test,
// mask-field, deposit-field for Common Lisp byte manipulation
func builtinByte(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("byte: need size and position")
	}
	size := int(toNum(args[0]))
	position := int(toNum(args[1]))
	return list(vnum(float64(size)), vnum(float64(position))), nil
}

func builtinByteSize(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("byte-size: need a byte specifier")
	}
	bs := seqToList(args[0])
	if len(bs) >= 1 {
		return bs[0], nil
	}
	return vnum(0), nil
}

func builtinBytePosition(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("byte-position: need a byte specifier")
	}
	bs := seqToList(args[0])
	if len(bs) >= 2 {
		return bs[1], nil
	}
	return vnum(0), nil
}

func builtinLdb(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldb: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("ldb: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 {
		return vnum(0), nil
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n >> uint(pos)) & mask)), nil
}

func builtinDpb(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("dpb: need newbyte, byte specifier, and integer")
	}
	newByte := int64(toNum(args[0]))
	bs := seqToList(args[1])
	if len(bs) < 2 {
		return nil, fmt.Errorf("dpb: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[2]))
	if size <= 0 {
		return args[2], nil // zero-width field: no change
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n & ^(mask << uint(pos))) | ((newByte & mask) << uint(pos)))), nil
}

// -------- Byte manipulation helpers --------
func builtinLdbTest(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ldb-test: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("ldb-test: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 || size > 63 {
		return vbool(false), nil
	}
	mask := int64((1 << uint(size)) - 1)
	return vbool(((n >> uint(pos)) & mask) != 0), nil
}

func builtinMaskField(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("mask-field: need byte specifier and integer")
	}
	bs := seqToList(args[0])
	if len(bs) < 2 {
		return nil, fmt.Errorf("mask-field: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[1]))
	if size <= 0 {
		return vnum(0), nil
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n >> uint(pos)) & mask)), nil
}

func builtinDepositField(args []*Value) (*Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("deposit-field: need newbyte, byte specifier, and integer")
	}
	newByte := int64(toNum(args[0]))
	bs := seqToList(args[1])
	if len(bs) < 2 {
		return nil, fmt.Errorf("deposit-field: invalid byte specifier")
	}
	size := int(toNum(bs[0]))
	pos := int(toNum(bs[1]))
	n := int64(toNum(args[2]))
	if size <= 0 {
		return args[2], nil // zero-width field: no change
	}
	if size > 63 {
		size = 63
	}
	mask := int64((1 << uint(size)) - 1)
	return vnum(float64((n & ^(mask << uint(pos))) | ((newByte & mask) << uint(pos)))), nil
}
