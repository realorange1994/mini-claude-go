package microlisp

import (
	"strings"
	"testing"
)

func TestGoCallMultiArgs(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(define t (now))
		(go:call t "AddDate" 1 0 0)
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:call multi-arg failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal from AddDate, got %s", typeStr(result))
	}
}

func TestMethodExpression(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define add-date (go:import "time.Time.AddDate"))
		(define now (go:import "time.Now"))
		(define t (now))
		(add-date t 1 0 0)
	`, globalEnv)
	if err != nil {
		t.Fatalf("method expression failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal from method expr call, got %s", typeStr(result))
	}
}

func TestGoCallSingleArg(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-reader (go:import "strings.NewReader"))
		(define r (new-reader "hello"))
		(go:call r "Len")
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:call single arg failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Len, got %s", typeStr(result))
	}
}

func TestGoTypeOfVGoVal(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(go:type-of (now))
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:type-of failed: %v", err)
	}
	if result.typ != VStr || !strings.Contains(result.str, "Time") {
		t.Fatalf("expected 'Time' in type-of result, got: %s", result.str)
	}
}

func TestGoFieldOnFFIReturn(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define generate-key (go:import "crypto/rsa.GenerateKey"))
		(define key (generate-key (go:import "crypto/rand.Reader") 2048))
		(go:type-of key)
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:type-of on FFI return value failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

func TestGoSetFieldNamedInt(t *testing.T) {
	InitGlobalEnv()
	// Test that go:set-field handles named int types like x509.KeyUsage
	result, err := EvalString(`
		(define cert (go:new "crypto/x509.Certificate"))
		(go:set-field cert "KeyUsage" 1)
		(go:field cert "KeyUsage")
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:set-field named int type failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from KeyUsage, got %s", typeStr(result))
	}
}

func TestGoSetFieldPtrStruct(t *testing.T) {
	InitGlobalEnv()
	// Test that go:set-field handles *pkix.Name → pkix.Name conversion
	result, err := EvalString(`
		(define cert (go:new "crypto/x509.Certificate"))
		(define name (go:new "crypto/x509/pkix.Name"))
		(go:set-field name "CommonName" "test")
		(go:set-field cert "Subject" name)
		(go:field (go:field cert "Subject") "CommonName")
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:set-field ptr struct conversion failed: %v", err)
	}
	if result.typ != VStr || result.str != "test" {
		t.Fatalf("expected 'test' from Subject.CommonName, got: %v", result)
	}
}

func TestGoImportConstants(t *testing.T) {
	InitGlobalEnv()
	// Test that Go constants like os.ModePerm are importable
	// reflectToLisp converts os.FileMode (int) to a Lisp number
	result, err := EvalString(`
		(go:import "os.ModePerm")
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:import os.ModePerm failed: %v", err)
	}
	// os.ModePerm is an int-based FileMode constant → converted to number by reflectToLisp
	if !isNumeric(result) {
		t.Fatalf("expected numeric from os.ModePerm, got %s", typeStr(result))
	}
	// ModePerm = 0777 = 511
	if toNum(result) != 511 {
		t.Fatalf("expected 511 (0777) from os.ModePerm, got %v", toNum(result))
	}
}

func TestLispToReflectAutoPtrWrap(t *testing.T) {
	InitGlobalEnv()
	// Test that VGoVal struct values are auto-wrapped in pointers when
	// the target parameter expects *T (e.g. *rsa.PublicKey)
	result, err := EvalString(`
		(define generate-key (go:import "crypto/rsa.GenerateKey"))
		(define key (generate-key (go:import "crypto/rand.Reader") 2048))
		(go:type-of key)
	`, globalEnv)
	if err != nil {
		t.Fatalf("go:field PublicKey failed: %v", err)
	}
	// key should be a *rsa.PrivateKey
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "PrivateKey") {
		t.Fatalf("expected 'PrivateKey' in type-of result, got: %s", result.str)
	}
}

func TestGoInterfaceAutoWrap(t *testing.T) {
	InitGlobalEnv()
	// Test that VGoVal struct values are auto-wrapped to pointers when
	// passed to functions expecting interface types.
	// x509.MarshalPKIXPublicKey takes crypto.PublicKey interface,
	// and internally does type switch on *rsa.PublicKey, *ecdsa.PublicKey, etc.
	// go:field priv-key "PublicKey" returns rsa.PublicKey (value type).
	// lispToReflectSafe should auto-wrap it to *rsa.PublicKey.
	_, err := EvalString(`
		(define generate-key (go:import "crypto/rsa.GenerateKey"))
		(define priv-key (generate-key (go:import "crypto/rand.Reader") 2048))
		; PublicKey field is rsa.PublicKey (value) - should auto-wrap to *rsa.PublicKey
		(define pub-key (go:field priv-key "PublicKey"))
		; MarshalPKIXPublicKey expects crypto.PublicKey interface
		(define marshal-pub (go:import "crypto/x509.MarshalPKIXPublicKey"))
		(marshal-pub pub-key)
	`, globalEnv)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey failed (interface auto-wrap may be missing): %v", err)
	}
}

func TestGoListIncludesTypes(t *testing.T) {
	InitGlobalEnv()
	// go:list should include both functions (from GoPackageRegistry) and
	// types (from GoTypeRegistry) for a given package
	result, err := EvalString(`(go:list "crypto/x509")`, globalEnv)
	if err != nil {
		t.Fatalf("go:list crypto/x509 failed: %v", err)
	}
	if !isList(result) {
		t.Fatalf("expected list from go:list, got %s", typeStr(result))
	}
	// Should contain both functions and types
	output := ToString(result)
	if !strings.Contains(output, "CreateCertificate") {
		t.Fatalf("expected CreateCertificate in go:list output, got: %s", output)
	}
	if !strings.Contains(output, "Certificate") {
		t.Fatalf("expected Certificate type in go:list output, got: %s", output)
	}
	if !strings.Contains(output, "PublicKey") {
		t.Fatalf("expected PublicKey type in go:list output, got: %s", output)
	}
}
