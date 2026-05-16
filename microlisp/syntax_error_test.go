package microlisp

import (
	"strings"
	"testing"
	"time"
)

type badSyntaxCase struct {
	name        string
	input       string
	expectErr   bool
	errContains string
}

func runBadSyntaxTest(t *testing.T, tc badSyntaxCase) {
	t.Helper()
	ResetGlobalEnv()

	limits := ResourceLimits{MaxSteps: 10000, MaxTimeMs: 3000}
	done := make(chan struct{})
	var evalErr error

	go func() {
		_, evalErr = SafeEvalWithLimits(tc.input, limits)
		close(done)
	}()

	select {
	case <-done:
		if tc.expectErr {
			if evalErr == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if tc.errContains != "" {
				if !strings.Contains(evalErr.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q for %q, got: %v", tc.errContains, tc.input, evalErr)
				}
			}
		} else {
			if evalErr != nil {
				t.Fatalf("expected no error for %q, got: %v", tc.input, evalErr)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("SafeEvalWithLimits hung on %q (deadlock or infinite loop)", tc.input)
	}
}

// ======== Unmatched / Extra Close Parens ========

func TestBadSyntaxCloseParen(t *testing.T) {
	runBadSyntaxTest(t, badSyntaxCase{
		name:      "leading close paren",
		input:     "> 3 2)",
		expectErr: true,
	})
}

func TestBadSyntaxMultipleCloseParens(t *testing.T) {
	cases := []badSyntaxCase{
		{"only close parens", "))))", true, ""},
		{"spaced close parens", ") ) )", true, ""},
		{"valid then extra close", "(+ 1 2))", true, "unmatched"},
		{"close paren after valid expr", "(+ 1 2) )", true, ""},
		{"close paren mid-expression", "(+ 1 ) 2)", true, ""},
		{"nested extra close", "((+ 1 2)))", true, ""},
		{"extra close at start", ") (+ 1 2)", true, ""},
		{"two extra close", "(+ 1 2)))", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Unclosed Lists ========

func TestBadSyntaxUnclosedList(t *testing.T) {
	cases := []badSyntaxCase{
		{"simple unclosed", "(1 2 3", true, "unclosed"},
		{"nested unclosed", "((1 2", true, "unclosed"},
		{"many opens", "((((x", true, "unclosed"},
		{"operator unclosed", "(+ 1 2", true, "unclosed"},
		{"define unclosed", "(define x 42", true, "unclosed"},
		{"let unclosed", "(let ((x 1)", true, "unclosed"},
		{"lambda unclosed", "(lambda (x)", true, "unclosed"},
		{"quote unclosed list", "'(1 2 3", true, "unclosed"},
		{"list func unclosed", "(list 1 2 3", true, "unclosed"},
		{"if unclosed", "(if t 1", true, "unclosed"},
		{"cond unclosed", "(cond ((t 1)", true, "unclosed"},
		{"funcall unclosed", "(funcall #'+ 1 2", true, "unclosed"},
		{"progn unclosed", "(progn 1 2 3", true, "unclosed"},
		{"when unclosed", "(when t 1 2", true, "unclosed"},
		{"unless unclosed", "(unless nil 1 2", true, "unclosed"},
		{"or unclosed", "(or t nil", true, "unclosed"},
		{"and unclosed", "(and t t", true, "unclosed"},
		{"not unclosed", "(not t", true, "unclosed"},
		{"setq unclosed", "(setq x 42", true, "unclosed"},
		{"setf unclosed", "(setf x 1", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Dot Notation Errors ========

func TestBadSyntaxDotErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"dot outside list", ". 5", true, ""},
		{"double dot", "(1 . 2 . 3)", true, ""},
		{"dot only", "(.)", true, ""},
		{"dot space close", "(. )", true, ""},
		{"dot without value", "(1 . )", true, ""},
		{"dot at start of list", "(. 1 2)", true, ""},
		{"multiple dots", "(1 . 2 .)", true, ""},
		{"dot in nested list", "(1 (2 . 3 . 4))", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Unclosed / Malformed Strings ========

func TestBadSyntaxStringErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"unclosed string", "\"hello", true, "unclosed"},
		{"unclosed empty string", "\"", true, "unclosed"},
		{"unclosed with spaces", "\"   unclosed", true, "unclosed"},
		{"escaped quote then unclosed", "\"hello \\\" world", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Reader Macro Errors ========

func TestBadSyntaxReaderMacroErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"bare sharp", "#", true, ""},
		{"sharp open paren unclosed", "#(", true, "unclosed"},
		{"sharp dot no form", "#.", true, ""},
		{"sharp backslash no char", "#\\", true, ""},
		{"sharp invalid char name", "#\\invalidCharName123", true, ""},
		{"sharp b no bits", "#b", true, ""},
		{"sharp x no hex", "#x", true, ""},
		{"sharp o no octal", "#o", true, ""},
		{"sharp c no complex", "#c", true, ""},
		{"sharp a no array", "#a", true, ""},
		{"sharp p no pathname", "#p", true, ""},
		{"sharp s no structure", "#s", true, ""},
		{"sharp colon no package", "#:", true, ""},
		{"sharp double quote no string", "#\"", true, ""},
		{"sharp vertical bar no pipe", "#|", true, ""},
		{"sharp vector unclosed", "#(1 2 3", true, "unclosed"},
		{"sharp b invalid bits", "#b2", true, ""},
		{"sharp x invalid hex", "#xG", true, ""},
		{"sharp with underscore", "#_", true, ""},
		{"sharp with newline", "#\n", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Quote / Quasiquote Errors ========

func TestBadSyntaxQuoteErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"quote then close paren", "')", true, ""},
		{"quasiquote then close paren", "`)", true, ""},
		{"unquote then close paren", ",)", true, ""},
		{"unquote-splicing then close paren", ",@)", true, ""},
		{"double quote then close", "'' )", true, ""},
		{"quote unclosed list", "'(1 2 3", true, "unclosed"},
		{"backtick unclosed", "`(1 2 3", true, "unclosed"},
		{"quote followed by nothing", "'", true, ""},
		{"backtick alone", "`", true, ""},
		{"unquote alone", ",", true, ""},
		{"unquote-splicing alone", ",@", true, ""},
		{"comma inside list no form", "(,)", true, ""},
		{"comma-at inside list no form", "(,@)", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Imbalanced Parens ========

func TestBadSyntaxImbalancedParens(t *testing.T) {
	cases := []badSyntaxCase{
		{"extra close after valid list", "((+ 1 2))", true, ""},
		{"more opens than closes", "(((+ 1 2)", true, "unclosed"},
		{"close before open", ") (+ 1 2)", true, ""},
		{"interleaved mismatch", "(+ 1)) 2)", true, ""},
		{"deep close imbalance", "(((1)))", true, ""},
		{"two lists one extra close", "(1) (2) )", true, ""},
		{"closing before any open", "))) (1)", true, ""},
		{"mixed valid and invalid", "(+ 1 2) )) (3 4)", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Empty / Whitespace (Valid, no error) ========

func TestBadSyntaxEmptyOrWhitespace(t *testing.T) {
	cases := []badSyntaxCase{
		{"empty string", "", false, ""},
		{"spaces only", "   ", false, ""},
		{"tab newline", "\t\n", false, ""},
		{"mixed whitespace", "  \t  \n  ", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Malformed Numbers ========

func TestBadSyntaxNumberErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"dot as number", ".", true, ""},
		{"double dot number", "..5", true, ""},
		{"float no integer part", ".5", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Comment Edge Cases ========

func TestBadSyntaxCommentErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"only comment valid no-op", "; this is a comment", false, ""},
		{"comment then close paren", "; hi\n)", true, ""},
		{"comment unclosed list", "; hi\n(1 2 3", true, "unclosed"},
		{"inline comment eof mid-list", "(1 ; comment\n", true, "unclosed"},
		{"multiple comments then error", "; a\n; b\n)", true, ""},
		{"comment between parens", "( ; hi\n1", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Nested / Complex Errors ========

func TestBadSyntaxNestedErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"nested list unclosed", "(list (1 2)", true, "unclosed"},
		{"nested define unclosed", "(define (f x) (if (> x 0)", true, "unclosed"},
		{"cond unclosed clause", "(cond ((> x 0)", true, "unclosed"},
		{"let unclosed body", "(let ((x 1)) (+ x", true, "unclosed"},
		{"lambda unclosed body", "(lambda () (1 2", true, "unclosed"},
		{"multiple lists one unclosed", "(+ 1 2) (list 3 4", true, "unclosed"},
		{"deep nesting missing close", "((((((1)))))", true, "unclosed"},
		{"nested cond unclosed", "(cond ((t (if t 1)))", true, "unclosed"},
		{"nested let unclosed", "(let ((a 1) (b (let ((c 2))", true, "unclosed"},
		{"nested quote unclosed", "'(1 '(2 3)", true, "unclosed"},
		{"unclosed function def", "(defun foo (x) (let ((y x))", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Character Literal Edge Cases ========

func TestBadSyntaxCharLiteralErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"sharp backslash only", "#\\", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Boolean / Nil Edge Cases ========

func TestBadSyntaxBooleanEdgeCases(t *testing.T) {
	cases := []badSyntaxCase{
		{"nil then extra close", "nil)", true, ""},
		{"t then extra close", "t)", true, ""},
		{"#t then extra close", "#t)", true, ""},
		{"#f then extra close", "#f)", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Format String Errors ========

func TestBadSyntaxFormatErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"format unclosed string", "(format t \"hello", true, "unclosed"},
		{"format no closing paren", "(format t \"~a\" 1", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Destructuring / Setf Errors ========

func TestBadSyntaxDestructuringErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"setf no value", "(setf x)", true, ""},
		{"setf unclosed", "(setf x 1", true, "unclosed"},
		{"psetq unclosed", "(psetq a 1 b 2", true, "unclosed"},
		{"multiple-value-setq unclosed", "(multiple-value-setq (a b) (values 1 2)", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Loop Errors ========

func TestBadSyntaxLoopErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"loop unclosed", "(loop for i from 1 to 10", true, "unclosed"},
		{"loop for unclosed range", "(loop for i from 1 to", true, "unclosed"},
		{"dotimes unclosed", "(dotimes (i 10)", true, "unclosed"},
		{"dolist unclosed", "(dolist (x lst)", true, "unclosed"},
		{"do unclosed", "(do ((i 0 (1+ i)))", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Array / Vector Errors ========

func TestBadSyntaxArrayErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"make-array unclosed", "(make-array 10", true, "unclosed"},
		{"aref unclosed", "(aref arr 0", true, "unclosed"},
		{"vector unclosed", "(vector 1 2 3", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Hash Table Errors ========

func TestBadSyntaxHashErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"make-hash-table unclosed", "(make-hash-table", true, "unclosed"},
		{"gethash unclosed", "(gethash key ht", true, "unclosed"},
		{"setf gethash unclosed", "(setf (gethash key ht) val", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== CLOS Errors ========

func TestBadSyntaxCLOSErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"defclass unclosed", "(defclass foo ()", true, "unclosed"},
		{"defmethod unclosed", "(defmethod bar ((x t))", true, "unclosed"},
		{"defgeneric unclosed", "(defgeneric foo", true, "unclosed"},
		{"make-instance unclosed", "(make-instance 'foo", true, "unclosed"},
		{"with-slots unclosed", "(with-slots (x) obj", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Condition / Restart Errors ========

func TestBadSyntaxConditionErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"handler-case unclosed", "(handler-case (foo)", true, "unclosed"},
		{"restart-case unclosed", "(restart-case (foo)", true, "unclosed"},
		{"signal unclosed", "(signal 'error", true, "unclosed"},
		{"error unclosed", "(error \"msg\"", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Package / Symbol Errors ========

func TestBadSyntaxPackageErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"in-package unclosed", "(in-package :cl", true, "unclosed"},
		{"defpackage unclosed", "(defpackage :foo", true, "unclosed"},
		{"symbol double colon no name", "foo::", true, ""},
		{"symbol single colon no name", "foo:", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Lambda / Destructuring Errors ========

func TestBadSyntaxLambdaErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"lambda no body", "(lambda (x))", false, ""},
		{"lambda unclosed args", "(lambda (x", true, "unclosed"},
		{"lambda empty args unclosed", "(lambda ()", true, ""},
		{"lambda &rest no var", "(lambda (&rest)", true, ""},
		{"lambda &key unclosed", "(lambda (&key", true, "unclosed"},
		{"lambda &optional unclosed", "(lambda (&optional x", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Block / Return / Catch Errors ========

func TestBadSyntaxBlockErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"block unclosed", "(block nil 1 2", true, "unclosed"},
		{"return-from unclosed", "(return-from foo", true, "unclosed"},
		{"catch unclosed", "(catch 'tag", true, "unclosed"},
		{"throw unclosed", "(throw 'tag", true, "unclosed"},
		{"unwind-protect unclosed", "(unwind-protect (foo)", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Macro Definition Errors ========

func TestBadSyntaxMacroErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"define-macro unclosed", "(define-macro (foo)", true, "unclosed"},
		{"defmacro unclosed", "(defmacro foo (x)", true, "unclosed"},
		{"macrolet unclosed", "(macrolet ((foo () body))", true, "unclosed"},
		{"symbol-macrolet unclosed", "(symbol-macrolet ((x y))", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Multiple Expression Mixed Errors ========

func TestBadSyntaxMultipleExprErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"valid then unclosed", "(+ 1 2) (3 4", true, "unclosed"},
		{"valid then extra close", "(+ 1 2) (3 4))", true, ""},
		{"two unclosed", "(1 (2 (3", true, "unclosed"},
		{"valid then dot error", "(+ 1 2) . 3", true, ""},
		{"valid then sharp error", "(+ 1 2) #\\", true, ""},
		{"valid then unclosed string", "(+ 1 2) \"unclosed", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Special Characters ========

func TestBadSyntaxSpecialChars(t *testing.T) {
	cases := []badSyntaxCase{
		{"bell character", "\x07", true, ""},
		{"form feed", "\x0c", true, ""},
		{"vertical tab", "\x0b", true, ""},
		{"null byte", "\x00", true, ""},
		{"escape char", "\x1b", true, ""},
		{"backspace", "\x08", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Deeply Nested ========

func TestBadSyntaxDeepNesting(t *testing.T) {
	cases := []badSyntaxCase{
		{"deep valid nesting extra close", "((((((((((10))))))))))", true, ""},
		{"deep almost balanced", "((((((((((10)))))))))", true, "unclosed"},
		{"deep with unclosed mid", "(((((1 2 3", true, "unclosed"},
		{"deep dot unclosed", "((1 . ", true, ""},
		{"deep quote unclosed", "'''(1 2 3", true, "unclosed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Bit Vector / Number Base Errors ========

func TestBadSyntaxBitVectorErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"sharp b alone", "#b", true, ""},
		{"sharp b invalid digit", "#b2", true, ""},
		{"sharp b with spaces", "#b 1 0 1", true, ""},
		{"sharp x alone", "#x", true, ""},
		{"sharp x invalid digit", "#xG", true, ""},
		{"sharp o alone", "#o", true, ""},
		{"sharp o invalid digit", "#o8", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Complex Number Errors ========

func TestBadSyntaxComplexErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"sharp c alone", "#c", true, ""},
		{"sharp c unclosed paren", "#c(1 2", true, ""},
		{"sharp c no elements", "#c()", true, ""},
		{"sharp c one element", "#c(1)", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Pathname Errors ========

func TestBadSyntaxPathnameErrors(t *testing.T) {
	cases := []badSyntaxCase{
		{"sharp p no string", "#p", true, ""},
		{"sharp p no string", "#p", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Whitespace-Only with Error Tokens ========

func TestBadSyntaxWhitespaceEdgeCases(t *testing.T) {
	cases := []badSyntaxCase{
		{"close paren with lots of space", "  \n\n  )  ", true, ""},
		{"dot with lots of space", "  \n  .  ", true, ""},
		{"sharp with trailing space", "#   ", true, ""},
		{"comma with leading space", "   ,", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}

// ======== Comprehensive All-Cases Test ========

func TestBadSyntaxAllCases(t *testing.T) {
	allCases := []badSyntaxCase{
		{"single close paren", ")", true, ""},
		{"four close parens", "))))", true, ""},
		{"spaced close parens", ") ) )", true, ""},
		{"unclosed simple", "(1 2 3", true, "unclosed"},
		{"unclosed nested", "((1 2", true, "unclosed"},
		{"many opens", "((((x", true, "unclosed"},
		{"dot outside", ". 5", true, ""},
		{"double dot", "(1 . 2 . 3)", true, ""},
		{"dot only", "(.)", true, ""},
		{"dot no value", "(1 . )", true, ""},
		{"unclosed string", "\"hello", true, "unclosed"},
		{"bare quote", "\"", true, "unclosed"},
		{"bare sharp", "#", true, ""},
		{"sharp paren unclosed", "#(", true, "unclosed"},
		{"sharp dot no form", "#.", true, ""},
		{"sharp backslash no char", "#\\", true, ""},
		{"sharp vector unclosed", "#(1 2", true, "unclosed"},
		{"quote then close", "')", true, ""},
		{"backtick then close", "`)", true, ""},
		{"unquote then close", ",)", true, ""},
		{"unquote-splicing then close", ",@)", true, ""},
		{"extra close after list", "((+ 1 2))", true, ""},
		{"close before open", ") (+ 1 2)", true, ""},
		{"bare dot", ".", true, ""},
		{"nested unclosed", "(list (1 2)", true, "unclosed"},
		{"deep imbalance", "((((((1)))))", true, "unclosed"},
	}

	for _, tc := range allCases {
		t.Run(tc.name, func(t *testing.T) { runBadSyntaxTest(t, tc) })
	}
}
