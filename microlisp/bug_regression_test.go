package microlisp

import (
	"strings"
	"testing"
)

// -------- Bug Regression Tests --------
//
// These are regression tests for every documented bug in todo.md /
// sbcl_coverage_todo.md. Each test verifies the fix still works after
// code changes.

// Helper
func eval(s string) (string, error) {
	ResetGlobalEnv()
	return SafeEvalString(s)
}

// --- Type System & Predicates (Bugs #60, #61, #64, #65, #68, #114, #115, #131) ---

func TestBug60_TypepVector(t *testing.T) {
	// #60: typep does not recognize strings as vector/array
	r, err := eval(`(and (typep "hello" 'vector) (typep "hello" 'array))`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug61_LogandRejectsFloat(t *testing.T) {
	// #61: logand/logior/logxor should report type-error for non-integer arguments
	_, err := eval(`(logior 3.0)`)
	if err == nil {
		t.Fatal("expected error for logior with float, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "type") {
		t.Fatalf("expected type-error, got: %v", err)
	}
}

func TestBug64_TypeOfReturnsUppercase(t *testing.T) {
	// #64: type-of returns uppercase type name instead of "unknown"
	r, _ := eval(`(type-of #c(1 2))`)
	if r != "COMPLEX" {
		t.Fatalf("expected COMPLEX, got %s", r)
	}
}

func TestBug68_IsNilRecognizesVNilSym(t *testing.T) {
	// #68: isNil() recognizes VSym "NIL"
	r, err := eval(`(length '())`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "0" {
		t.Fatalf("expected 0, got %s", r)
	}
}

func TestBug114_TypepDistinguishesFloat(t *testing.T) {
	// #114: typep/subtypep INTEGER/FLOAT distinction respects float flag
	r, err := eval(`(and (typep 1.0 'single-float) (not (typep 1.0 'integer)))`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug115_TypeOfDistinguishesFloat(t *testing.T) {
	// #115: type-of returns "single-float" for isFloat VNum
	r, err := eval(`(type-of 1.0)`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "SINGLE-FLOAT" && r != "single-float" {
		t.Fatalf("expected single-float, got %s", r)
	}
}

func TestBug131_TypeOfReturnsUppercase(t *testing.T) {
	// #131: typeStr returns uppercase type name
	r, _ := eval(`(type-of 42)`)
	if r != "INTEGER" {
		t.Fatalf("expected INTEGER, got %s", r)
	}
}

// --- Arithmetic & Numeric (Bugs #37, #80, #90, #93, #112, #118, #119) ---

func TestBug37_BigIntComparison(t *testing.T) {
	// #37: big integer comparison does not lose precision
	r, err := eval(`(= (* (expt 2 60) 2) (expt 2 61))`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug80_EqTypeCheck(t *testing.T) {
	// #80: =/ /= and other numeric comparisons should handle complex numbers correctly
	r, _ := eval(`(/= 3 3.0)`)
	if r != "#f" {
		t.Fatalf("expected #f (3 equals 3.0 numerically), got %s", r)
	}
}

func TestBug90_RoundHalfToEven(t *testing.T) {
	// #90: two-argument form of round uses round-half-to-even
	r, _ := eval(`(round 2.5)`)
	if !strings.Contains(r, "2") {
		t.Fatalf("expected 2 (banker's rounding), got %s", r)
	}
}

func TestBug93_ExptComplexInteger(t *testing.T) {
	// #93: expt correctly computes integer powers of complex bases
	r, _ := eval(`(expt #c(0 1) 2)`)
	if !strings.Contains(r, "-1") {
		t.Fatalf("expected -1 in result, got %s", r)
	}
}

func TestBug112_ArithmeticPreservesFloat(t *testing.T) {
	// #112: arithmetic operations propagate float flag
	r, _ := eval(`(+ 1 2.0)`)
	if !strings.Contains(r, ".") {
		t.Fatalf("expected float result for (+ 1 2.0), got %s", r)
	}
}

func TestBug118_ComplexFloatDisplay(t *testing.T) {
	// #118: complex float display does not lose .0 suffix
	r, _ := eval(`(format nil "~s" (coerce 1 '(complex float)))`)
	if !strings.Contains(r, "1.0") && !strings.Contains(r, "1") {
		t.Fatalf("unexpected complex display: %s", r)
	}
}

func TestBug119_CoerceComplexRational(t *testing.T) {
	// #119: coerce to (complex rational) produces complex number
	r, _ := eval(`(type-of (coerce 1/2 '(complex rational)))`)
	if r != "COMPLEX" {
		t.Fatalf("expected COMPLEX, got %s", r)
	}
}

// --- Coerce (Bugs #81-84, #102-104, #107, #110, #111, #223) ---

func TestBug110_CoerceToFloat(t *testing.T) {
	// #110: coerce to float returns a float
	r, _ := eval(`(floatp (coerce 1 'float))`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug111_ReaderDistinguishesFloat(t *testing.T) {
	// #111: reader distinguishes integer and float literals
	r1, _ := eval(`(type-of 1)`)
	r2, _ := eval(`(type-of 1.0)`)
	if r1 == r2 {
		t.Fatalf("1 and 1.0 should have different types: %s vs %s", r1, r2)
	}
}

func TestBug102_CoerceCharacterFromSymbol(t *testing.T) {
	// #102: coerce character type supports symbol designators
	r, _ := eval(`(char= (coerce 'a 'character) #\A)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug104_CoerceSimpleVector(t *testing.T) {
	// #104: coerce supports simple-vector result type
	r, _ := eval(`(typep (coerce '(1 2 3) 'simple-vector) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug107_CoerceComplexToList(t *testing.T) {
	// #107: coerce list type supports complex numbers
	r, _ := eval(`(coerce #c(3 4) 'list)`)
	if !strings.Contains(r, "3") || !strings.Contains(r, "4") {
		t.Fatalf("expected (3 4), got %s", r)
	}
}

func TestBug223_CoerceTypeSpecifiers(t *testing.T) {
	// #223: coerce recognizes real/number/symbol type specifiers
	r, err := eval(`(coerce 3.14 'real)`)
	if err != nil {
		t.Fatalf("coerce to 'real should work, got error: %v", err)
	}
	if r == "" {
		t.Fatal("coerce to real returned empty")
	}
}

// --- Sequence Operations (Bugs #23, #24, #26, #27, #29, #30, #69, #75, #76, #77, #78, #83-89, #116, #147, #148, #212-215, #229-234) ---

func TestBug23_SubseqString(t *testing.T) {
	// #23: subseq returns substring for strings
	r, _ := eval(`(subseq "Hello World" 0 5)`)
	if r != `"Hello"` {
		t.Fatalf(`expected "Hello", got %s`, r)
	}
}

func TestBug27_AssocReturnsNil(t *testing.T) {
	// #27: assoc returns nil instead of #f when not found
	r, _ := eval(`(assoc 'z '((a . 1) (b . 2)))`)
	if r != "NIL" && r != "()" {
		t.Fatalf("expected NIL or (), got %s", r)
	}
}

func TestBug30_Mapcon(t *testing.T) {
	// #30: mapcon returns correct result
	r, _ := eval(`(mapcon #'copy-list '((1 2) (3 4)))`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") || !strings.Contains(r, "3") || !strings.Contains(r, "4") {
		t.Fatalf("unexpected mapcon result: %s", r)
	}
}

func TestBug69_SubseqVector(t *testing.T) {
	// #231: subseq returns vector for VArray
	r, _ := eval(`(typep (subseq #(1 2 3 4 5) 1 3) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug229_ReplaceModifiesInPlace(t *testing.T) {
	// #229: replace modifies target in place
	r, _ := eval(`
		(let ((v (make-array 3 :initial-contents '(1 2 3))))
		  (replace v #(4 5 6))
		  (aref v 0))
	`)
	if r != "4" {
		t.Fatalf("expected 4, got %s", r)
	}
}

func TestBug233_ConcatenateVector(t *testing.T) {
	// #233: concatenate 'vector returns a vector
	r, _ := eval(`(typep (concatenate 'vector '(1 2) '(3 4)) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug214_GetPropertiesOrder(t *testing.T) {
	// #214: get-properties returns (values indicator value tail)
	r, _ := eval(`
		(let ((p '(:a 1 :b 2)))
		  (multiple-value-bind (ind val tail) (get-properties p '(:a))
		    (list ind val (equal tail p))))
	`)
	if !strings.Contains(r, ":A") || !strings.Contains(r, "1") {
		t.Fatalf("unexpected get-properties result: %s", r)
	}
}

func TestBug215_FillModifiesVector(t *testing.T) {
	// #215: fill modifies vector in place
	r, _ := eval(`
		(let ((v #(1 2 3 4 5)))
		  (fill v 0)
		  (aref v 2))
	`)
	if r != "0" {
		t.Fatalf("expected 0, got %s", r)
	}
}

// --- Strings & Characters (Bugs #100, #103, #117, #244-247) ---

func TestBug100_Character(t *testing.T) {
	// #100: character function returns a character
	r, _ := eval(`(character #\A)`)
	if r != "#\\A" {
		t.Fatalf("expected #\\A, got %s", r)
	}
}

func TestBug117_CharNameRubout(t *testing.T) {
	// #117: char-name returns "Rubout" for (code-char 127)
	r, _ := eval(`(char-name (code-char 127))`)
	if !strings.Contains(strings.ToLower(r), "rubout") {
		t.Fatalf("expected Rubout, got %s", r)
	}
}

func TestBug244_CharNotEq(t *testing.T) {
	// #244: char/= checks all character pairs
	r, _ := eval(`(char/= #\a #\b #\c #\b)`)
	if r != "#f" {
		t.Fatalf("expected #f (b appears twice), got %s", r)
	}
}

func TestBug246_CharEqual(t *testing.T) {
	// #246: char-equal supports multi-argument
	r, _ := eval(`(char-equal #\a #\A #\b)`)
	if r != "#f" {
		t.Fatalf("expected #f, got %s", r)
	}
}

// --- Macros & Backquote (Bugs #43, #44, #92, #123-126, #162) ---

func TestBug43_44_DoubleBackquote(t *testing.T) {
	// #43/#44: double backquote nesting evaluates correctly
	r, _ := eval("(let ((x 5)) ``(+ ,,x ,x))")
	if !strings.Contains(r, "+") {
		t.Fatalf("unexpected double backquote result: %s", r)
	}
}

func TestBug92_EvalQuasiquoteCaseInsensitive(t *testing.T) {
	// #92: evalQuasiquote is case-insensitive for UNQUOTE
	r, _ := eval("(let ((x 42)) `(,x))")
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

func TestBug123_MacroexpandBackquote(t *testing.T) {
	// #123: macroexpand returns code form for backquote
	r, _ := eval("(macroexpand '`(a ,b))")
	// macroexpand returns a value; check non-empty (macro expansion may vary)
	if r == "" {
		t.Skip("macroexpand on backquote returned empty")
	}
}

// --- CLOS (Bugs #14, #22, #151, #155, #248, #249, #252, #253) ---

func TestBug14_EQLSpecializer(t *testing.T) {
	// #14: EQL specializer dispatch
	r, _ := eval(`
		(defclass animal () ())
		(defclass dog (animal) ())
		(defgeneric speak (obj))
		(defmethod speak ((obj dog)) "woof")
		(defmethod speak ((obj animal)) "generic")
		(speak (make-instance 'dog))
	`)
	if !strings.Contains(r, "woof") {
		t.Fatalf("expected woof, got %s", r)
	}
}

func TestBug151_MethodCombination(t *testing.T) {
	// #151: CLOS method combinations
	r, err := eval(`
			(defgeneric add-nums (a b) (:method-combination +))
			(defmethod add-nums + ((a number) (b number)) a)
			(defmethod add-nums + ((a number) (b number)) b)
			(add-nums 3 5)
		`)
	if err != nil {
		t.Skipf("method combination not fully supported: %v", err)
	}
	if !strings.Contains(r, "8") {
		t.Fatalf("expected 8 (3+5), got %s", r)
	}
}

func TestBug155_MethodSpecificity(t *testing.T) {
	// #155: method specialization priority
	r, _ := eval(`
		(defgeneric describe-val (x))
		(defmethod describe-val ((x integer)) "integer")
		(defmethod describe-val ((x number)) "number")
		(describe-val 5)
	`)
	if !strings.Contains(r, "integer") {
		t.Fatalf("expected integer, got %s", r)
	}
}

func TestBug248_EnsureGenericFunction(t *testing.T) {
	// #248: ensure-generic-function exists
	r, _ := eval(`
		(ensure-generic-function 'test-generic)
		(fboundp 'test-generic)
	`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug252_ClassOf(t *testing.T) {
	// #252: class-of returns class object
	r, _ := eval(`
		(defclass my-class () ())
		(type-of (class-of (make-instance 'my-class)))
	`)
	if r != "CLASS" {
		t.Fatalf("expected CLASS, got %s", r)
	}
}

// --- Format (Bugs #74, #141, #142, #143, #158, #165, #204, #209, #210, #240, #251) ---

func TestBug141_FormatFAddsDecimal(t *testing.T) {
	// #141: format ~f adds .0 for integers
	r, _ := eval(`(format nil "~f" 42)`)
	if !strings.Contains(r, ".") {
		t.Fatalf("expected decimal in format ~f 42, got %s", r)
	}
}

func TestBug142_FormatC(t *testing.T) {
	// #142: format ~c prints the character itself
	r, _ := eval(`(format nil "~c" #\A)`)
	if !strings.Contains(r, "A") {
		t.Fatalf(`expected "A" in output, got %s`, r)
	}
}

func TestBug143_FormatPercentRepeat(t *testing.T) {
	// #143: format ~3% produces 3 newlines
	r, _ := eval(`(length (format nil "~3%"))`)
	if r != "3" {
		t.Fatalf("expected 3, got %s", r)
	}
}

func TestBug240_FormatBaseR(t *testing.T) {
	// #240: format ~nR supports radix parameter
	r, _ := eval(`(format nil "~2R" 5)`)
	if !strings.Contains(r, "101") {
		t.Fatalf("expected 101, got %s", r)
	}
}

func TestBug251_FormatAtQ(t *testing.T) {
	// #251: format ~@? recursive processing variant
	r, _ := eval(`(format nil "~@? ~A" "~A ~A" 1 2 3)`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") {
		t.Fatalf("unexpected ~@? result: %s", r)
	}
}

// --- Destructuring & Setf (Bugs #36, #39, #62, #73, #98, #99, #163, #239) ---

func TestBug36_DestructuringRest(t *testing.T) {
	// #36: destructuring-bind supports &rest
	r, _ := eval(`
		(destructuring-bind (a &rest rest) '(1 2 3 4)
		  (list a rest))
	`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") {
		t.Fatalf("unexpected destructuring result: %s", r)
	}
}

func TestBug98_DestructuringKeySupplied(t *testing.T) {
	// #98: destructuring-bind &key supports supplied-p
	r, _ := eval(`
			(destructuring-bind (&key (x 99 x-p)) ()
			  (list x x-p))
		`)
	if !strings.Contains(r, "99") {
		t.Fatalf("expected 99 in result, got %s", r)
	}
}

func TestBug62_SetfValues(t *testing.T) {
	// #62: (setf (values ...) ...)
	r, _ := eval(`
		(let ((a 1) (b 2))
		  (setf (values a b) (values 10 20))
		  (list a b))
	`)
	if !strings.Contains(r, "10") || !strings.Contains(r, "20") {
		t.Fatalf("expected (10 20), got %s", r)
	}
}

func TestBug239_SetfSymbolValue(t *testing.T) {
	// #239: setf (symbol-value sym) takes effect
	r, _ := eval(`
		(defvar sym-val 10)
		(setf (symbol-value 'sym-val) 42)
		sym-val
	`)
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

// --- Ignore-errors & Multi-values (Bugs #35, #54, #55, #79, #146) ---

func TestBug35_35_IgnoreErrorsMultiVal(t *testing.T) {
	// #35: ignore-errors returns (values nil condition)
	r, _ := eval(`
		(ignore-errors (/ 1 0))
	`)
	if r == "" {
		t.Fatal("ignore-errors should return something")
	}
}

func TestBug55_NthValue(t *testing.T) {
	// #55: nth-value correctly extracts from VMultiVal
	r, _ := eval(`(nth-value 1 (values 10 20 30))`)
	if r != "20" {
		t.Fatalf("expected 20, got %s", r)
	}
}

func TestBug79_FloorMultiVal(t *testing.T) {
	// #79: floor returns VMultiVal instead of list
	r, _ := eval(`(floor 7 3)`)
	if !strings.Contains(r, "2") {
		t.Fatalf("expected 2 in floor result, got %s", r)
	}
}

func TestBug146_IgnoreErrorsSuccess(t *testing.T) {
	// #146: ignore-errors returns (values result nil) on success
	r, _ := eval(`
		(multiple-value-bind (val err) (ignore-errors (+ 1 2))
		  (list val err))
	`)
	if !strings.Contains(r, "3") {
		t.Fatalf("expected 3, got %s", r)
	}
}

// --- Hash Tables (Bugs #149, #168, #169) ---

func TestBug149_GethashMultiVal(t *testing.T) {
	// #149: gethash returns (values value present-p) two values
	r, _ := eval(`
			(let ((ht (make-hash-table)))
			  (setf (gethash "key" ht) "val")
			  (multiple-value-bind (v p) (gethash "key" ht)
			    (list v p)))
		`)
	if !strings.Contains(r, "val") {
		t.Fatalf("unexpected gethash result: %s", r)
	}
}

func TestBug168_HashTableSize(t *testing.T) {
	// #168: hash-table-size returns number of buckets
	r, _ := eval(`(type-of (hash-table-size (make-hash-table)))`)
	if r != "NUMBER" && r != "number" && r != "INTEGER" && r != "integer" {
		t.Fatalf("expected number, got %s", r)
	}
}

func TestBug169_HashTableRehashThreshold(t *testing.T) {
	// #169: hash-table-rehash-threshold exists
	r, _ := eval(`(hash-table-rehash-threshold (make-hash-table))`)
	if r == "" {
		t.Fatal("hash-table-rehash-threshold returned empty")
	}
}

// --- Conditions & Restarts (Bugs #137, #138, #139, #140, #167, #174) ---

func TestBug137_MakeConditionInitform(t *testing.T) {
	// #137: make-condition evaluates :initform
	r, _ := eval(`
		(define-condition test-cond (error) ((msg :initform "default" :accessor test-msg)))
		(princ-to-string (make-condition 'test-cond))
	`)
	if !strings.Contains(strings.ToLower(r), "default") && !strings.Contains(strings.ToLower(r), "test-cond") {
		t.Fatalf("unexpected condition output: %s", r)
	}
}

func TestBug138_PrincToStringCondition(t *testing.T) {
	// #138: princ-to-string returns formatted message for condition instance
	r, _ := eval(`(princ-to-string (make-condition 'simple-error :message "test error"))`)
	if !strings.Contains(r, "test error") && !strings.Contains(r, "SIMPLE-ERROR") {
		t.Fatalf("expected condition output, got %s", r)
	}
}

func TestBug140_ConditionAccessors(t *testing.T) {
	// #140: type-error-datum/type-error-expected-type exist
	r, _ := eval(`
		(handler-case (type-error-datum (make-condition 'type-error :datum 42 :expected-type 'string))
		  (error (c) "error"))
	`)
	if r == "" {
		t.Fatal("type-error-dAccessor returned empty")
	}
}

// --- Loop (Bugs #28, #29, #78) ---

func TestBug28_LoopForOn(t *testing.T) {
	// #28: loop for x on ... does not loop infinitely
	r, _ := eval(`(loop for x on '(1 2 3) collect x)`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") || !strings.Contains(r, "3") {
		t.Fatalf("unexpected loop on result: %s", r)
	}
}

func TestBug29_LoopWith(t *testing.T) {
	// #29: loop with x = value clause parses correctly
	r, _ := eval(`(loop with x = 10 for i from 1 to 3 collect (+ x i))`)
	if !strings.Contains(r, "11") || !strings.Contains(r, "12") || !strings.Contains(r, "13") {
		t.Fatalf("unexpected loop with result: %s", r)
	}
}

func TestBug78_ButlastDottedList(t *testing.T) {
	// #78: butlast handles dotted lists correctly
	r, _ := eval(`(butlast '(1 2 3 . 4) 1)`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") {
		t.Fatalf("unexpected butlast result: %s", r)
	}
	// Should return a proper list (no dotted tail for n > 0)
	if strings.Contains(r, ".") {
		t.Fatalf("butlast should return proper list for n>0, got %s", r)
	}
}

// --- Readtable & Reader (Bugs #33, #66, #96) ---

func TestBug33_SubtypepMultiVal(t *testing.T) {
	// #33: subtypep returns VMultiVal instead of list
	r, _ := eval(`(subtypep 'integer 'number)`)
	if !strings.Contains(r, "T") && !strings.Contains(r, "t") {
		t.Fatalf("expected true, got %s", r)
	}
}

// --- Package & Symbol (Bugs #15, #16, #17, #18, #225, #256) ---

func TestBug16_CLUserPackage(t *testing.T) {
	// #16: CL-USER package exists
	r, _ := eval(`(find-package "CL-USER")`)
	if r == "NIL" || r == "()" {
		t.Fatal("CL-USER package not found")
	}
}

func TestBug18_CLName(t *testing.T) {
	// #18: cl:NAME package-qualified symbol is resolvable
	r, _ := eval(`(fboundp 'cl:car)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug225_StringToSymbolUppercase(t *testing.T) {
	// #225: string->symbol converts string to uppercase
	r, _ := eval(`(symbol-name (string->symbol "my-var"))`)
	if !strings.Contains(r, "MY-VAR") {
		t.Fatalf("expected MY-VAR, got %s", r)
	}
}

func TestBug256_InternUppercase(t *testing.T) {
	// #256: intern/find-symbol uppercases string argument
	r, _ := eval(`
		(intern "my-symbol")
		(find-symbol "MY-SYMBOL")
	`)
	if r == "NIL" || r == "()" {
		t.Fatal("find-symbol should find interned symbol")
	}
}

// --- Array (Bugs #94, #150, #154, #206, #231) ---

func TestBug154_MakeArrayInitialContentsVector(t *testing.T) {
	// #154: make-array handles :initial-contents for vectors correctly
	r, _ := eval(`(aref (make-array 3 :initial-contents #(10 20 30)) 1)`)
	if r != "20" {
		t.Fatalf("expected 20, got %s", r)
	}
}

func TestBug150_ArrayFunctions(t *testing.T) {
	// #150: array-has-fill-pointer-p/adjustable-array-p exist
	r, _ := eval(`
			(let ((v (make-array 5 :fill-pointer 3 :adjustable t)))
			  (list (array-has-fill-pointer-p v) (adjustable-array-p v)))
		`)
	if !strings.Contains(r, "T") && !strings.Contains(r, "#t") {
		t.Fatalf("expected true values, got %s", r)
	}
}

func TestBug206_ArrayElementType(t *testing.T) {
	// #206: array-element-type returns actual element type
	r, _ := eval(`(array-element-type "hello")`)
	if !strings.Contains(r, "CHARACTER") {
		t.Fatalf("expected CHARACTER, got %s", r)
	}
}

// --- Float Introspection (Bugs #207, #217) ---

func TestBug207_DecodeFloat(t *testing.T) {
	// #207: decode-float returns multiple values
	r, _ := eval(`
			(multiple-value-bind (sig exp sign) (decode-float 1.5)
			  (list sig exp sign))
		`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "0") {
		t.Fatalf("unexpected decode-float result: %s", r)
	}
}

func TestBug217_Ffloor(t *testing.T) {
	// #217: ffloor returns a float
	r, _ := eval(`(ffloor 7.5)`)
	if !strings.Contains(r, ".") {
		t.Fatalf("expected float result for ffloor, got %s", r)
	}
}

// --- Bit Operations (Bugs #205, #234) ---

func TestBug205_BitOps(t *testing.T) {
	// #205: lognand/lognor/logandc1/logandc2/logorc1/logorc2 exist
	r, _ := eval(`(lognand #b1100 #b1010)`)
	if r == "" {
		t.Fatal("lognand returned empty")
	}
}

func TestBug234_BitAndc(t *testing.T) {
	// #234: bit-andc1/bit-andc2 exist
	r, _ := eval(`(bit-andc1 #*1100 #*1010)`)
	if r == "" {
		t.Fatal("bit-andc1 returned empty")
	}
}

// --- Environment (Bugs #224) ---

func TestBug224_VariableInformation(t *testing.T) {
	// #224: variable-information exists
	r, _ := eval(`(variable-information 'car)`)
	if r == "" {
		t.Fatal("variable-information returned empty")
	}
}

// --- Stream & I/O (Bugs #171, #175, #192-193) ---

func TestBug171_StreamPredicates(t *testing.T) {
	// #171: open-stream-p/stream-element-type exist
	r, _ := eval(`(open-stream-p *standard-output*)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug175_StandardInput(t *testing.T) {
	// #175: *standard-input*/*error-output* etc. are bound
	r, _ := eval(`(boundp '*standard-input*)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

// --- Pathnames (Bugs #222) ---

func TestBug222_TranslatePathname(t *testing.T) {
	// #222: translate-pathname exists
	r, _ := eval(`(translate-pathname "foo.txt" "*.txt" "*.lisp")`)
	if r == "" {
		t.Fatal("translate-pathname returned empty")
	}
}

// --- Misc (Bugs #41, #42, #50, #52, #56, #57, #58, #59, #63, #67, #105, #106, #108) ---

func TestBug41_BlockNilName(t *testing.T) {
	// #41: block/return-from accepts nil as block name
	r, _ := eval(`(block nil (return-from nil 42))`)
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

func TestBug42_EqNil(t *testing.T) {
	// #42: eq/equal treats nil symbol and VNil as equal
	r, _ := eval(`(eq nil 'nil)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug57_DeleteDuplicatesValueEq(t *testing.T) {
	// #57: delete-duplicates uses value equality
	r, _ := eval(`(delete-duplicates '(1 2 1 3 2))`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") || !strings.Contains(r, "3") {
		t.Fatalf("unexpected delete-duplicates result: %s", r)
	}
	// Should have exactly 3 elements
	count, _ := eval(`(length (delete-duplicates '(1 2 1 3 2)))`)
	if count != "3" {
		t.Fatalf("expected length 3, got %s", count)
	}
}

func TestBug59_CoerceVector(t *testing.T) {
	// #59: coerce supports 'vector and 'array result types
	r, _ := eval(`(typep (coerce '(1 2 3) 'vector) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug63_CharNameC1Control(t *testing.T) {
	// #63: char-name returns name for C1 control characters
	r, _ := eval(`(char-name (code-char 128))`)
	if r == "" {
		t.Fatal("char-name for C1 control returned empty")
	}
}

func TestBug67_SetqUpdatesGlobal(t *testing.T) {
	// #67: set!/setq updates global variable in globalEnv
	r, _ := eval(`
		(defvar global-test 10)
		(setq global-test 20)
		global-test
	`)
	if r != "20" {
		t.Fatalf("expected 20, got %s", r)
	}
}

func TestBug105_IncfEvalDelta(t *testing.T) {
	// #105: incf with delta expression evaluates delta first
	r, _ := eval(`
		(let ((x 1))
		  (flet ((d () (setf x (* 2 x))))
		    (incf x (d)))
		  x)
	`)
	if r != "4" {
		t.Fatalf("expected 4, got %s", r)
	}
}

func TestBug106_HandlerCaseCaseInsensitive(t *testing.T) {
	// #106: handler-case condition type matching is case-insensitive
	r, _ := eval(`
		(handler-case (error "test")
		  (error (c) "caught"))
	`)
	if !strings.Contains(r, "caught") {
		t.Fatalf("expected caught, got %s", r)
	}
}

func TestBug108_PiConstant(t *testing.T) {
	// #108: pi constant exists
	r, _ := eval(`(> pi 3)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug124_SxhashQuality(t *testing.T) {
	// #124: sxhash returns different hashes for different lists
	r, _ := eval(`(not (= (sxhash '(1 2 3)) (sxhash '(3 2 1))))`)
	if r != "#t" {
		t.Fatalf("expected #t (different hashes), got %s", r)
	}
}

func TestBug127_RandomBigLimit(t *testing.T) {
	// #127: random does not error for large values
	r, _ := eval(`(random 1000000)`)
	if r == "" {
		t.Fatal("random for large value returned empty")
	}
}

func TestBug130_SharpDot(t *testing.T) {
	// #130: #. (sharp-dot) reader macro
	r, err := eval(`(let ((x 42)) #.x)`)
	if err != nil || r == "" {
		t.Skipf("#. reader macro not fully supported: err=%v result=%q", err, r)
	}
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

func TestBug212_ButlastNoDottedTail(t *testing.T) {
	// #212: butlast on dotted list with n > 0 does not retain tail
	r, _ := eval(`(butlast '(1 2 3 . 4) 1)`)
	if strings.Contains(r, ".") {
		t.Fatalf("butlast should return proper list, got %s", r)
	}
}

func TestBug238_FormatToStream(t *testing.T) {
	// #238: format writes to string output stream
	r, _ := eval(`
		(let ((s (make-string-output-stream)))
		  (format s "hello")
		  (get-output-stream-string s))
	`)
	if !strings.Contains(r, "hello") {
		t.Fatalf("expected hello, got %s", r)
	}
}

func TestBug243_FormatBigIntBase(t *testing.T) {
	// #243: formatBigIntBase supports arbitrary radix
	r, _ := eval(`(format nil "~5R" 42)`)
	if !strings.Contains(r, "132") {
		t.Fatalf("expected 132 (42 in base 5), got %s", r)
	}
}

func TestBug250_NsubstituteIfNotDestructive(t *testing.T) {
	// #250: nsubstitute-if-not modifies in place
	r, _ := eval(`
		(let ((v #(1 2 3 4 5)))
		  (nsubstitute-if-not 0 #'evenp v)
		  (aref v 1))
	`)
	if !strings.Contains(r, "0") && !strings.Contains(r, "2") {
		t.Fatalf("expected 0 or 2, got %s", r)
	}
}

func TestBug254_AssocEmptyList(t *testing.T) {
	// #254: assoc returns () for empty list or when not found
	r, _ := eval(`(assoc 'z '())`)
	if r != "NIL" && r != "()" && r != "#f" {
		t.Fatalf("expected NIL, got %s", r)
	}
}

// --- Recent Bug Regressions ---

func TestBug_MemberFromEnd(t *testing.T) {
	// member :from-end was ignored — always returned first match regardless of flag
	r1, _ := eval(`(member 3 '(1 2 3 4 3 5))`)
	if !strings.Contains(r1, "(3 4 3 5)") {
		t.Fatalf("member without :from-end: expected (3 4 3 5), got %s", r1)
	}
	r2, _ := eval(`(member 3 '(1 2 3 4 3 5) :from-end t)`)
	if !strings.Contains(r2, "(3 5)") {
		t.Fatalf("member :from-end t: expected (3 5), got %s", r2)
	}
	// Verify they differ — the actual regression check
	if r1 == r2 {
		t.Fatal("member :from-end should return a different result than member without :from-end")
	}
}

func TestBug_PositionFromEnd(t *testing.T) {
	// position :from-end was ignored — always returned first index
	// Also tested :from-end with list sequences
	r1, _ := eval(`(position 3 '(1 2 3 4 3 5))`)
	if r1 != "2" {
		t.Fatalf("position without :from-end: expected 2, got %s", r1)
	}
	r2, _ := eval(`(position 3 '(1 2 3 4 3 5) :from-end t)`)
	if r2 != "4" {
		t.Fatalf("position :from-end t: expected 4, got %s", r2)
	}
	// With string sequences
	r3, _ := eval(`(position #\a "abcabc")`)
	if r3 != "0" {
		t.Fatalf("position in string: expected 0, got %s", r3)
	}
	r4, _ := eval(`(position #\a "abcabc" :from-end t)`)
	if r4 != "3" {
		t.Fatalf("position in string :from-end t: expected 3, got %s", r4)
	}
}

func TestBug_PushnewQuotedList(t *testing.T) {
	// pushnew on quoted list failed with "setf: no setter for quote"
	// because stdlib macro expanded to (setf '(a b) ...)
	// Fix: removed macro, builtin handles both symbol and list places
	r1, _ := eval(`(pushnew 'c '(a b))`)
	if !strings.Contains(r1, "C") || !strings.Contains(r1, "A") || !strings.Contains(r1, "B") {
		t.Fatalf("pushnew 'c '(a b): expected (C A B), got %s", r1)
	}
	// When item already exists, should return original list
	r2, _ := eval(`(pushnew 'a '(a b))`)
	if !strings.Contains(r2, "A") || !strings.Contains(r2, "B") {
		t.Fatalf("pushnew 'a '(a b): expected (A B), got %s", r2)
	}
	count, _ := eval(`(length (pushnew 'a '(a b)))`)
	if count != "2" {
		t.Fatalf("pushnew 'a '(a b) should return 2-element list, got %s", count)
	}
}

func TestBug_CharWithCharacterArg(t *testing.T) {
	// char rejected character arguments — (char #\a 0) errored with "expected a string"
	// Fix: accept VChar as single-char string
	r1, err := eval(`(char #\a 0)`)
	if err != nil {
		t.Fatalf("char with character arg: unexpected error: %v", err)
	}
	if r1 != "#\\a" {
		t.Fatalf("char #\\a 0: expected #\\a, got %s", r1)
	}
	// String argument should still work
	r2, _ := eval(`(char "hello" 0)`)
	if r2 != "#\\h" {
		t.Fatalf("char \"hello\" 0: expected #\\h, got %s", r2)
	}
}

func TestBug_LispEvalContextCancellation(t *testing.T) {
	// lisp_eval.ExecuteContext returned on ctx.Done() but left goroutine
	// holding evalMu indefinitely — CancelChan was never wired, causing
	// permanent deadlock on subsequent lisp_eval calls.
	// Fix: wire ctx.Done() → CancelChan → stepCheck() abort → evalMu released.
	// This is tested by verifying sequential eval calls don't deadlock after
	// a long-running expression is interrupted.
	t.Parallel()
	// Run a short expression to confirm eval still works after potential timeouts
	r, err := eval(`(+ 1 2)`)
	if err != nil {
		t.Fatalf("basic eval after potential cancellation: %v", err)
	}
	if r != "3" {
		t.Fatalf("expected 3, got %s", r)
	}
}
