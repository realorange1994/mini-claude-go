package microlisp

import "testing"

func TestFFIInterface(t *testing.T) {
	ResetGlobalEnv()
	tests := []string{
		// io.ByteReader
		`(binary-read-uvarint (vector 0 0 0 5))`,
		// *bytes.Buffer
		`(json-compact (vector 123 34 97 34 58 49 125))`,
		// crc32.ChecksumIEEE - just []byte arg
		`(crc32-checksum-ieee (vector 1 2 3))`,
		// time.UTC
		`(time-date 2024 1 15 12 0 0 0 time-utc)`,
		// net.CIDRMask
		`(net-cidr-mask 24 32)`,
		// log.Output
		`(log-output 1 "test")`,
		// subtle-xor-bytes needs 3 args
		`(subtle-xor-bytes (vector 0 0 0) (vector 1 2 3) (vector 4 5 6))`,
		// json-compact needs 2 args: dst, src
		`(json-compact (vector 0 0 0 0 0) (vector 123 125))`,
		// sort-search-ints
		`(sort-search-ints (vector 1 2 3 4 5) 3)`,
		// sort-search-float64s
		`(sort-search-float64s (vector 1.0 2.0 3.0) 2.0)`,
		// sort-strings
		`(sort-search-strings (vector "a" "b" "c") "b")`,
		// cmplx-rect (takes 2 float64, returns complex128)
		`(cmplx-rect 3 4)`,
		// cmplx-inf
		`(cmplx-inf)`,
		// cmplx-na-n
		`(cmplx-na-n)`,
		// reflect-struct-of
		`(reflect-struct-of nil)`,
	}
	for _, expr := range tests {
		out, err := SafeEvalString(expr)
		t.Logf("EXPR: %s", expr)
		if err != nil {
			t.Logf("  ERROR: %v", err)
		} else {
			t.Logf("  OUT: %s", out)
		}
	}
}
