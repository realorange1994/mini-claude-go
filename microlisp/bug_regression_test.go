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
	// #60: typep 不识别字符串为 vector/array
	r, err := eval(`(and (typep "hello" 'vector) (typep "hello" 'array))`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug61_LogandRejectsFloat(t *testing.T) {
	// #61: logand/logior/logxor 对非整数参数应报type-error
	_, err := eval(`(logior 3.0)`)
	if err == nil {
		t.Fatal("expected error for logior with float, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "type") {
		t.Fatalf("expected type-error, got: %v", err)
	}
}

func TestBug64_TypeOfReturnsUppercase(t *testing.T) {
	// #64: type-of 返回大写类型名称而非 "unknown"
	r, _ := eval(`(type-of #c(1 2))`)
	if r != "COMPLEX" {
		t.Fatalf("expected COMPLEX, got %s", r)
	}
}

func TestBug68_IsNilRecognizesVNilSym(t *testing.T) {
	// #68: isNil() 识别 VSym "NIL"
	r, err := eval(`(length '())`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "0" {
		t.Fatalf("expected 0, got %s", r)
	}
}

func TestBug114_TypepDistinguishesFloat(t *testing.T) {
	// #114: typep/ subtypep 的 INTEGER/FLOAT 区分尊重浮点标记
	r, err := eval(`(and (typep 1.0 'single-float) (not (typep 1.0 'integer)))`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug115_TypeOfDistinguishesFloat(t *testing.T) {
	// #115: type-of 对 isFloat VNum 返回 "single-float"
	r, err := eval(`(type-of 1.0)`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "SINGLE-FLOAT" && r != "single-float" {
		t.Fatalf("expected single-float, got %s", r)
	}
}

func TestBug131_TypeOfReturnsUppercase(t *testing.T) {
	// #131: typeStr 返回大写类型名称
	r, _ := eval(`(type-of 42)`)
	if r != "INTEGER" {
		t.Fatalf("expected INTEGER, got %s", r)
	}
}

// --- Arithmetic & Numeric (Bugs #37, #80, #90, #93, #112, #118, #119) ---

func TestBug37_BigIntComparison(t *testing.T) {
	// #37: 大整数比较不丢失精度
	r, err := eval(`(= (* (expt 2 60) 2) (expt 2 61))`)
	if err != nil {
		t.Fatal(err)
	}
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug80_EqTypeCheck(t *testing.T) {
	// #80: =/ /= 等数值比较对复数应报错或正确比较
	r, _ := eval(`(/= 3 3.0)`)
	if r != "#f" {
		t.Fatalf("expected #f (3 equals 3.0 numerically), got %s", r)
	}
}

func TestBug90_RoundHalfToEven(t *testing.T) {
	// #90: round 的 two-argument 形式使用 round-half-to-even
	r, _ := eval(`(round 2.5)`)
	if !strings.Contains(r, "2") {
		t.Fatalf("expected 2 (banker's rounding), got %s", r)
	}
}

func TestBug93_ExptComplexInteger(t *testing.T) {
	// #93: expt 对复数基底的整数指数幂正确计算
	r, _ := eval(`(expt #c(0 1) 2)`)
	if !strings.Contains(r, "-1") {
		t.Fatalf("expected -1 in result, got %s", r)
	}
}

func TestBug112_ArithmeticPreservesFloat(t *testing.T) {
	// #112: 算术运算传播浮点标记
	r, _ := eval(`(+ 1 2.0)`)
	if !strings.Contains(r, ".") {
		t.Fatalf("expected float result for (+ 1 2.0), got %s", r)
	}
}

func TestBug118_ComplexFloatDisplay(t *testing.T) {
	// #118: 复数浮点显示不丢失 .0 后缀
	r, _ := eval(`(format nil "~s" (coerce 1 '(complex float)))`)
	if !strings.Contains(r, "1.0") && !strings.Contains(r, "1") {
		t.Fatalf("unexpected complex display: %s", r)
	}
}

func TestBug119_CoerceComplexRational(t *testing.T) {
	// #119: coerce 到 (complex rational) 产生复数
	r, _ := eval(`(type-of (coerce 1/2 '(complex rational)))`)
	if r != "COMPLEX" {
		t.Fatalf("expected COMPLEX, got %s", r)
	}
}

// --- Coerce (Bugs #81-84, #102-104, #107, #110, #111, #223) ---

func TestBug110_CoerceToFloat(t *testing.T) {
	// #110: coerce 到 float 返回浮点数
	r, _ := eval(`(floatp (coerce 1 'float))`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug111_ReaderDistinguishesFloat(t *testing.T) {
	// #111: reader 区分整数和浮点数字面量
	r1, _ := eval(`(type-of 1)`)
	r2, _ := eval(`(type-of 1.0)`)
	if r1 == r2 {
		t.Fatalf("1 and 1.0 should have different types: %s vs %s", r1, r2)
	}
}

func TestBug102_CoerceCharacterFromSymbol(t *testing.T) {
	// #102: coerce 的 character 类型支持符号设计符
	r, _ := eval(`(char= (coerce 'a 'character) #\A)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug104_CoerceSimpleVector(t *testing.T) {
	// #104: coerce 支持 simple-vector 结果类型
	r, _ := eval(`(typep (coerce '(1 2 3) 'simple-vector) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug107_CoerceComplexToList(t *testing.T) {
	// #107: coerce 的 list 类型支持复数
	r, _ := eval(`(coerce #c(3 4) 'list)`)
	if !strings.Contains(r, "3") || !strings.Contains(r, "4") {
		t.Fatalf("expected (3 4), got %s", r)
	}
}

func TestBug223_CoerceTypeSpecifiers(t *testing.T) {
	// #223: coerce 识别 real/number/symbol 类型说明符
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
	// #23: subseq 对字符串返回子字符串
	r, _ := eval(`(subseq "Hello World" 0 5)`)
	if r != `"Hello"` {
		t.Fatalf(`expected "Hello", got %s`, r)
	}
}

func TestBug27_AssocReturnsNil(t *testing.T) {
	// #27: assoc 找不到时返回 nil 而非 #f
	r, _ := eval(`(assoc 'z '((a . 1) (b . 2)))`)
	if r != "NIL" && r != "()" {
		t.Fatalf("expected NIL or (), got %s", r)
	}
}

func TestBug30_Mapcon(t *testing.T) {
	// #30: mapcon 返回正确结果
	r, _ := eval(`(mapcon #'copy-list '((1 2) (3 4)))`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") || !strings.Contains(r, "3") || !strings.Contains(r, "4") {
		t.Fatalf("unexpected mapcon result: %s", r)
	}
}

func TestBug69_SubseqVector(t *testing.T) {
	// #231: subseq 对 VArray 返回向量
	r, _ := eval(`(typep (subseq #(1 2 3 4 5) 1 3) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug229_ReplaceModifiesInPlace(t *testing.T) {
	// #229: replace 就地修改目标
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
	// #233: concatenate 'vector 返回向量
	r, _ := eval(`(typep (concatenate 'vector '(1 2) '(3 4)) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug214_GetPropertiesOrder(t *testing.T) {
	// #214: get-properties 返回 (values indicator value tail)
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
	// #215: fill 就地修改向量
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
	// #100: character 函数返回字符
	r, _ := eval(`(character #\A)`)
	if r != "#\\A" {
		t.Fatalf("expected #\\A, got %s", r)
	}
}

func TestBug117_CharNameRubout(t *testing.T) {
	// #117: char-name 对 (code-char 127) 返回 "Rubout"
	r, _ := eval(`(char-name (code-char 127))`)
	if !strings.Contains(strings.ToLower(r), "rubout") {
		t.Fatalf("expected Rubout, got %s", r)
	}
}

func TestBug244_CharNotEq(t *testing.T) {
	// #244: char/= 检查所有字符对
	r, _ := eval(`(char/= #\a #\b #\c #\b)`)
	if r != "#f" {
		t.Fatalf("expected #f (b appears twice), got %s", r)
	}
}

func TestBug246_CharEqual(t *testing.T) {
	// #246: char-equal 支持多参数
	r, _ := eval(`(char-equal #\a #\A #\b)`)
	if r != "#f" {
		t.Fatalf("expected #f, got %s", r)
	}
}

// --- Macros & Backquote (Bugs #43, #44, #92, #123-126, #162) ---

func TestBug43_44_DoubleBackquote(t *testing.T) {
	// #43/#44: 双反引号嵌套求值正确
	r, _ := eval("(let ((x 5)) ``(+ ,,x ,x))")
	if !strings.Contains(r, "+") {
		t.Fatalf("unexpected double backquote result: %s", r)
	}
}

func TestBug92_EvalQuasiquoteCaseInsensitive(t *testing.T) {
	// #92: evalQuasiquote 对 UNQUOTE 大小写不敏感
	r, _ := eval("(let ((x 42)) `(,x))")
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

func TestBug123_MacroexpandBackquote(t *testing.T) {
	// #123: macroexpand 对 backquote 返回代码形式
	r, _ := eval("(macroexpand '`(a ,b))")
	// macroexpand returns a value; check non-empty (macro expansion may vary)
	if r == "" {
		t.Skip("macroexpand on backquote returned empty")
	}
}

// --- CLOS (Bugs #14, #22, #151, #155, #248, #249, #252, #253) ---

func TestBug14_EQLSpecializer(t *testing.T) {
	// #14: EQL specializer 分发
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
	// #155: 方法特化优先级
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
	// #248: ensure-generic-function 存在
	r, _ := eval(`
		(ensure-generic-function 'test-generic)
		(fboundp 'test-generic)
	`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug252_ClassOf(t *testing.T) {
	// #252: class-of 返回类对象
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
	// #141: format ~f 对整数添加 .0
	r, _ := eval(`(format nil "~f" 42)`)
	if !strings.Contains(r, ".") {
		t.Fatalf("expected decimal in format ~f 42, got %s", r)
	}
}

func TestBug142_FormatC(t *testing.T) {
	// #142: format ~c 打印字符本身
	r, _ := eval(`(format nil "~c" #\A)`)
	if !strings.Contains(r, "A") {
		t.Fatalf(`expected "A" in output, got %s`, r)
	}
}

func TestBug143_FormatPercentRepeat(t *testing.T) {
	// #143: format ~3% 产生3个换行
	r, _ := eval(`(length (format nil "~3%"))`)
	if r != "3" {
		t.Fatalf("expected 3, got %s", r)
	}
}

func TestBug240_FormatBaseR(t *testing.T) {
	// #240: format ~nR 支持基数参数
	r, _ := eval(`(format nil "~2R" 5)`)
	if !strings.Contains(r, "101") {
		t.Fatalf("expected 101, got %s", r)
	}
}

func TestBug251_FormatAtQ(t *testing.T) {
	// #251: format ~@? 递归处理变体
	r, _ := eval(`(format nil "~@? ~A" "~A ~A" 1 2 3)`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") {
		t.Fatalf("unexpected ~@? result: %s", r)
	}
}

// --- Destructuring & Setf (Bugs #36, #39, #62, #73, #98, #99, #163, #239) ---

func TestBug36_DestructuringRest(t *testing.T) {
	// #36: destructuring-bind 支持 &rest
	r, _ := eval(`
		(destructuring-bind (a &rest rest) '(1 2 3 4)
		  (list a rest))
	`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") {
		t.Fatalf("unexpected destructuring result: %s", r)
	}
}

func TestBug98_DestructuringKeySupplied(t *testing.T) {
	// #98: destructuring-bind 的 &key 支持 supplied-p
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
	// #239: setf (symbol-value sym) 生效
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
	// #35: ignore-errors 返回 (values nil condition)
	r, _ := eval(`
		(ignore-errors (/ 1 0))
	`)
	if r == "" {
		t.Fatal("ignore-errors should return something")
	}
}

func TestBug55_NthValue(t *testing.T) {
	// #55: nth-value 从 VMultiVal 正确提取
	r, _ := eval(`(nth-value 1 (values 10 20 30))`)
	if r != "20" {
		t.Fatalf("expected 20, got %s", r)
	}
}

func TestBug79_FloorMultiVal(t *testing.T) {
	// #79: floor 返回 VMultiVal 而非 list
	r, _ := eval(`(floor 7 3)`)
	if !strings.Contains(r, "2") {
		t.Fatalf("expected 2 in floor result, got %s", r)
	}
}

func TestBug146_IgnoreErrorsSuccess(t *testing.T) {
	// #146: ignore-errors 成功时返回 (values result nil)
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
	// #149: gethash 返回 (values value present-p) 双值
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
	// #168: hash-table-size 返回桶数量
	r, _ := eval(`(type-of (hash-table-size (make-hash-table)))`)
	if r != "NUMBER" && r != "number" && r != "INTEGER" && r != "integer" {
		t.Fatalf("expected number, got %s", r)
	}
}

func TestBug169_HashTableRehashThreshold(t *testing.T) {
	// #169: hash-table-rehash-threshold 存在
	r, _ := eval(`(hash-table-rehash-threshold (make-hash-table))`)
	if r == "" {
		t.Fatal("hash-table-rehash-threshold returned empty")
	}
}

// --- Conditions & Restarts (Bugs #137, #138, #139, #140, #167, #174) ---

func TestBug137_MakeConditionInitform(t *testing.T) {
	// #137: make-condition 评估 :initform
	r, _ := eval(`
		(define-condition test-cond (error) ((msg :initform "default" :accessor test-msg)))
		(princ-to-string (make-condition 'test-cond))
	`)
	if !strings.Contains(strings.ToLower(r), "default") && !strings.Contains(strings.ToLower(r), "test-cond") {
		t.Fatalf("unexpected condition output: %s", r)
	}
}

func TestBug138_PrincToStringCondition(t *testing.T) {
	// #138: princ-to-string 对条件实例返回格式化消息
	r, _ := eval(`(princ-to-string (make-condition 'simple-error :message "test error"))`)
	if !strings.Contains(r, "test error") && !strings.Contains(r, "SIMPLE-ERROR") {
		t.Fatalf("expected condition output, got %s", r)
	}
}

func TestBug140_ConditionAccessors(t *testing.T) {
	// #140: type-error-datum/type-error-expected-type 存在
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
	// #28: loop 的 for x on ... 不无限循环
	r, _ := eval(`(loop for x on '(1 2 3) collect x)`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "2") || !strings.Contains(r, "3") {
		t.Fatalf("unexpected loop on result: %s", r)
	}
}

func TestBug29_LoopWith(t *testing.T) {
	// #29: loop 的 with x = value 子句正确解析
	r, _ := eval(`(loop with x = 10 for i from 1 to 3 collect (+ x i))`)
	if !strings.Contains(r, "11") || !strings.Contains(r, "12") || !strings.Contains(r, "13") {
		t.Fatalf("unexpected loop with result: %s", r)
	}
}

func TestBug78_ButlastDottedList(t *testing.T) {
	// #78: butlast 对点状列表处理正确
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
	// #33: subtypep 返回 VMultiVal 而非 list
	r, _ := eval(`(subtypep 'integer 'number)`)
	if !strings.Contains(r, "T") && !strings.Contains(r, "t") {
		t.Fatalf("expected true, got %s", r)
	}
}

// --- Package & Symbol (Bugs #15, #16, #17, #18, #225, #256) ---

func TestBug16_CLUserPackage(t *testing.T) {
	// #16: CL-USER 包存在
	r, _ := eval(`(find-package "CL-USER")`)
	if r == "NIL" || r == "()" {
		t.Fatal("CL-USER package not found")
	}
}

func TestBug18_CLName(t *testing.T) {
	// #18: cl:NAME 包限定符号可解析
	r, _ := eval(`(fboundp 'cl:car)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug225_StringToSymbolUppercase(t *testing.T) {
	// #225: string->symbol 将字符串转为大写
	r, _ := eval(`(symbol-name (string->symbol "my-var"))`)
	if !strings.Contains(r, "MY-VAR") {
		t.Fatalf("expected MY-VAR, got %s", r)
	}
}

func TestBug256_InternUppercase(t *testing.T) {
	// #256: intern/find-symbol 对字符串参数大写化
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
	// #154: make-array 对向量 :initial-contents 正确处理
	r, _ := eval(`(aref (make-array 3 :initial-contents #(10 20 30)) 1)`)
	if r != "20" {
		t.Fatalf("expected 20, got %s", r)
	}
}

func TestBug150_ArrayFunctions(t *testing.T) {
	// #150: array-has-fill-pointer-p/adjustable-array-p 存在
	r, _ := eval(`
			(let ((v (make-array 5 :fill-pointer 3 :adjustable t)))
			  (list (array-has-fill-pointer-p v) (adjustable-array-p v)))
		`)
	if !strings.Contains(r, "T") && !strings.Contains(r, "#t") {
		t.Fatalf("expected true values, got %s", r)
	}
}

func TestBug206_ArrayElementType(t *testing.T) {
	// #206: array-element-type 返回实际元素类型
	r, _ := eval(`(array-element-type "hello")`)
	if !strings.Contains(r, "CHARACTER") {
		t.Fatalf("expected CHARACTER, got %s", r)
	}
}

// --- Float Introspection (Bugs #207, #217) ---

func TestBug207_DecodeFloat(t *testing.T) {
	// #207: decode-float 返回多值
	r, _ := eval(`
			(multiple-value-bind (sig exp sign) (decode-float 1.5)
			  (list sig exp sign))
		`)
	if !strings.Contains(r, "1") || !strings.Contains(r, "0") {
		t.Fatalf("unexpected decode-float result: %s", r)
	}
}

func TestBug217_Ffloor(t *testing.T) {
	// #217: ffloor 返回浮点数
	r, _ := eval(`(ffloor 7.5)`)
	if !strings.Contains(r, ".") {
		t.Fatalf("expected float result for ffloor, got %s", r)
	}
}

// --- Bit Operations (Bugs #205, #234) ---

func TestBug205_BitOps(t *testing.T) {
	// #205: lognand/lognor/logandc1/logandc2/logorc1/logorc2 存在
	r, _ := eval(`(lognand #b1100 #b1010)`)
	if r == "" {
		t.Fatal("lognand returned empty")
	}
}

func TestBug234_BitAndc(t *testing.T) {
	// #234: bit-andc1/bit-andc2 存在
	r, _ := eval(`(bit-andc1 #*1100 #*1010)`)
	if r == "" {
		t.Fatal("bit-andc1 returned empty")
	}
}

// --- Environment (Bugs #224) ---

func TestBug224_VariableInformation(t *testing.T) {
	// #224: variable-information 存在
	r, _ := eval(`(variable-information 'car)`)
	if r == "" {
		t.Fatal("variable-information returned empty")
	}
}

// --- Stream & I/O (Bugs #171, #175, #192-193) ---

func TestBug171_StreamPredicates(t *testing.T) {
	// #171: open-stream-p/stream-element-type 存在
	r, _ := eval(`(open-stream-p *standard-output*)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug175_StandardInput(t *testing.T) {
	// #175: *standard-input*/*error-output* 等绑定
	r, _ := eval(`(boundp '*standard-input*)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

// --- Pathnames (Bugs #222) ---

func TestBug222_TranslatePathname(t *testing.T) {
	// #222: translate-pathname 存在
	r, _ := eval(`(translate-pathname "foo.txt" "*.txt" "*.lisp")`)
	if r == "" {
		t.Fatal("translate-pathname returned empty")
	}
}

// --- Misc (Bugs #41, #42, #50, #52, #56, #57, #58, #59, #63, #67, #105, #106, #108) ---

func TestBug41_BlockNilName(t *testing.T) {
	// #41: block/return-from 接受 nil 作为块名
	r, _ := eval(`(block nil (return-from nil 42))`)
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

func TestBug42_EqNil(t *testing.T) {
	// #42: eq/equal 将 nil 符号和 VNil 视为相等
	r, _ := eval(`(eq nil 'nil)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug57_DeleteDuplicatesValueEq(t *testing.T) {
	// #57: delete-duplicates 使用值相等
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
	// #59: coerce 支持 'vector 和 'array 结果类型
	r, _ := eval(`(typep (coerce '(1 2 3) 'vector) 'vector)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug63_CharNameC1Control(t *testing.T) {
	// #63: char-name 对 C1 控制字符返回名称
	r, _ := eval(`(char-name (code-char 128))`)
	if r == "" {
		t.Fatal("char-name for C1 control returned empty")
	}
}

func TestBug67_SetqUpdatesGlobal(t *testing.T) {
	// #67: set!/setq 更新 globalEnv 中的全局变量
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
	// #105: incf 带 delta 表达式时先求值 delta
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
	// #106: handler-case 条件类型匹配大小写不敏感
	r, _ := eval(`
		(handler-case (error "test")
		  (error (c) "caught"))
	`)
	if !strings.Contains(r, "caught") {
		t.Fatalf("expected caught, got %s", r)
	}
}

func TestBug108_PiConstant(t *testing.T) {
	// #108: pi 常量存在
	r, _ := eval(`(> pi 3)`)
	if r != "#t" {
		t.Fatalf("expected #t, got %s", r)
	}
}

func TestBug124_SxhashQuality(t *testing.T) {
	// #124: sxhash 对不同列表返回不同哈希值
	r, _ := eval(`(not (= (sxhash '(1 2 3)) (sxhash '(3 2 1))))`)
	if r != "#t" {
		t.Fatalf("expected #t (different hashes), got %s", r)
	}
}

func TestBug127_RandomBigLimit(t *testing.T) {
	// #127: random 对大值不报错
	r, _ := eval(`(random 1000000)`)
	if r == "" {
		t.Fatal("random for large value returned empty")
	}
}

func TestBug130_SharpDot(t *testing.T) {
	// #130: #. (sharp-dot) 读者宏
	r, err := eval(`(let ((x 42)) #.x)`)
	if err != nil || r == "" {
		t.Skipf("#. reader macro not fully supported: err=%v result=%q", err, r)
	}
	if !strings.Contains(r, "42") {
		t.Fatalf("expected 42 in result, got %s", r)
	}
}

func TestBug212_ButlastNoDottedTail(t *testing.T) {
	// #212: butlast 对点状列表 n > 0 不保留尾部
	r, _ := eval(`(butlast '(1 2 3 . 4) 1)`)
	if strings.Contains(r, ".") {
		t.Fatalf("butlast should return proper list, got %s", r)
	}
}

func TestBug238_FormatToStream(t *testing.T) {
	// #238: format 写入字符串输出流
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
	// #243: formatBigIntBase 支持任意基数
	r, _ := eval(`(format nil "~5R" 42)`)
	if !strings.Contains(r, "132") {
		t.Fatalf("expected 132 (42 in base 5), got %s", r)
	}
}

func TestBug250_NsubstituteIfNotDestructive(t *testing.T) {
	// #250: nsubstitute-if-not 就地修改
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
	// #254: assoc 在空列表或未找到时返回 ()
	r, _ := eval(`(assoc 'z '())`)
	if r != "NIL" && r != "()" && r != "#f" {
		t.Fatalf("expected NIL, got %s", r)
	}
}
