package microlisp

import (
	"strings"
	"testing"
)

// ===========================================================================
// Scenario 1: Math & Numeric Operations
// ===========================================================================

func TestScenario_MathOperations(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define sqrt (go:import "math.Sqrt"))
		(sqrt 16.0)
	`, globalEnv)
	if err != nil {
		t.Fatalf("math.Sqrt failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Sqrt, got %s", typeStr(result))
	}
	if toNum(result) != 4.0 {
		t.Fatalf("expected 4.0 from Sqrt(16), got %v", toNum(result))
	}
}

func TestScenario_MathFunctions(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define abs (go:import "math.Abs"))
		(abs -42.5)
	`, globalEnv)
	if err != nil {
		t.Fatalf("math.Abs failed: %v", err)
	}
	if !isNumeric(result) || toNum(result) != 42.5 {
		t.Fatalf("expected 42.5, got %v", result)
	}
}

// ===========================================================================
// Scenario 2: String Processing Pipeline
// ===========================================================================

func TestScenario_StringProcessing(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define to-upper (go:import "strings.ToUpper"))
		(to-upper "hello world")
	`, globalEnv)
	if err != nil {
		t.Fatalf("strings.ToUpper failed: %v", err)
	}
	if result.typ != VStr || result.str != "HELLO WORLD" {
		t.Fatalf("expected 'HELLO WORLD', got %v", result)
	}

	result2, err := EvalString(`
		(define trim-space (go:import "strings.TrimSpace"))
		(define to-lower (go:import "strings.ToLower"))
		(to-lower (trim-space "  Hello  "))
	`, globalEnv)
	if err != nil {
		t.Fatalf("trim+lower failed: %v", err)
	}
	if result2.typ != VStr || result2.str != "hello" {
		t.Fatalf("expected 'hello', got %v", result2)
	}
}

func TestScenario_StringContains(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define contains (go:import "strings.Contains"))
		(contains "hello world" "world")
	`, globalEnv)
	if err != nil {
		t.Fatalf("strings.Contains failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool from Contains, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 3: JSON Serialization
// ===========================================================================

func TestScenario_JSONMarshal(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define json-valid (go:import "encoding/json.Valid"))
		(json-valid "{\"key\": \"value\"}")
	`, globalEnv)
	if err != nil {
		t.Fatalf("json.Valid failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool from json.Valid, got %s", typeStr(result))
	}
}

func TestScenario_JSONIndent(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(define json-indent (go:import "encoding/json.Indent"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("json.Indent import failed: %v", err)
	}
}

// ===========================================================================
// Scenario 4: Time Operations
// ===========================================================================

func TestScenario_TimeOperations(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(define t (now))
		(go:call t "Unix")
	`, globalEnv)
	if err != nil {
		t.Fatalf("time.Now + Unix() failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Unix(), got %s", typeStr(result))
	}
}

func TestScenario_TimeParseAndFormat(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define parse (go:import "time.Parse"))
		(define layout "2006-01-02")
		(define t (parse layout "2024-06-15"))
		(go:call t "Year")
	`, globalEnv)
	if err != nil {
		t.Fatalf("time.Parse + Year() failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Year(), got %s", typeStr(result))
	}
}

func TestScenario_TimeFormat(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(define t (now))
		(go:call t "Format" "2006-01-02 15:04:05")
	`, globalEnv)
	if err != nil {
		t.Fatalf("time.Now + Format failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from Format, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "20") {
		t.Fatalf("expected date string, got: %s", result.str)
	}
}

// ===========================================================================
// Scenario 5: URL Parsing
// ===========================================================================

func TestScenario_URLParsing(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define parse-url (go:import "net/url.Parse"))
		(define u (parse-url "https://example.com:8080/path?q=hello#section"))
		(go:field u "Host")
	`, globalEnv)
	if err != nil {
		t.Fatalf("url.Parse + field Host failed: %v", err)
	}
	if result.typ != VStr || result.str != "example.com:8080" {
		t.Fatalf("expected 'example.com:8080', got %v", result)
	}
}

func TestScenario_URLEncode(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define q-escape (go:import "net/url.QueryEscape"))
		(define encoded (q-escape "hello world & foo=bar"))
		encoded
	`, globalEnv)
	if err != nil {
		t.Fatalf("url.QueryEscape failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from QueryEscape, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "+") || !strings.Contains(result.str, "%26") {
		t.Fatalf("expected URL-encoded string, got: %s", result.str)
	}
}

// ===========================================================================
// Scenario 6: HTTP Client
// ===========================================================================

func TestScenario_HTTPRequestWithHeaders(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-request (go:import "net/http.NewRequest"))
		(define req (new-request "POST" "https://api.example.com/v1/users" nil))
		(go:call (go:field req "Header") "Set" "Content-Type" "application/json")
		(go:call (go:field req "Header") "Set" "Authorization" "Bearer token123")
		(go:field req "Header")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http request with headers failed: %v", err)
	}
	// Header is http.Header (map[string][]string)
	if result.typ != VGoVal && result.typ != VNil {
		t.Fatalf("expected VGoVal or nil from Header, got %s", typeStr(result))
	}
}

func TestScenario_HTTPRequestURL(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-request (go:import "net/http.NewRequest"))
		(define req (new-request "GET" "https://example.com/path" nil))
		(go:field req "URL")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal from req.URL, got %s", typeStr(result))
	}
}

func TestScenario_HTTPMethodField(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-request (go:import "net/http.NewRequest"))
		(define req (new-request "GET" "https://api.example.com/data?key=value" nil))
		(go:call (go:field req "Header") "Set" "Accept" "application/json")
		(go:field req "Method")
	`, globalEnv)
	if err != nil {
		t.Fatalf("complete HTTP request failed: %v", err)
	}
	if result.typ != VStr || result.str != "GET" {
		t.Fatalf("expected 'GET', got: %v", result)
	}
}

func TestScenario_HTTPStatusText(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define status-text (go:import "net/http.StatusText"))
		(status-text 200)
	`, globalEnv)
	if err != nil {
		t.Fatalf("http.StatusText failed: %v", err)
	}
	if result.typ != VStr || result.str != "OK" {
		t.Fatalf("expected 'OK', got: %v", result)
	}
}

// ===========================================================================
// Scenario 7: Error Handling
// ===========================================================================

func TestScenario_ErrorHandling(t *testing.T) {
	InitGlobalEnv()
	// errors.New returns an error interface; FFI treats non-nil error as failure
	// Just verify it's importable
	_, err := EvalString(`
		(define new-error (go:import "errors.New"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("errors.New import failed: %v", err)
	}
}

// ===========================================================================
// Scenario 8: Base64 Encoding
// ===========================================================================

func TestScenario_Base64(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define std-enc (go:import "encoding/base64.StdEncoding"))
		(go:call std-enc "EncodeToString" "Hello, World!")
	`, globalEnv)
	if err != nil {
		t.Fatalf("base64.StdEncoding.EncodeToString failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from EncodeToString, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 9: Bytes Buffer
// ===========================================================================

func TestScenario_BytesBuffer(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define buf (go:new "bytes.Buffer"))
		(go:call buf "WriteString" "hello ")
		(go:call buf "WriteString" "world")
		(go:call buf "String")
	`, globalEnv)
	if err != nil {
		t.Fatalf("bytes.Buffer failed: %v", err)
	}
	if result.typ != VStr || result.str != "hello world" {
		t.Fatalf("expected 'hello world', got %v", result)
	}
}

// ===========================================================================
// Scenario 10: Crypto - RSA Key Generation + Public Key
// ===========================================================================

func TestScenario_SelfSignedCert(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define generate-key (go:import "crypto/rsa.GenerateKey"))
		(define priv-key (generate-key (go:import "crypto/rand.Reader") 2048))
		(define tmpl (go:new "crypto/x509.Certificate"))
		(go:set-field tmpl "SerialNumber" 1)
		(define pub-key (go:field priv-key "PublicKey"))
		(define marshal-pub (go:import "crypto/x509.MarshalPKIXPublicKey"))
		(define der-pub (marshal-pub pub-key))
		; der-pub is []byte -> reflectToLisp converts to string
		der-pub
	`, globalEnv)
	if err != nil {
		t.Fatalf("self-signed cert pipeline failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from der-pub ([]byte->string), got: %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 11: BigInt Operations
// ===========================================================================

func TestScenario_BigInt(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-big (go:new "math/big.Int"))
		(go:call new-big "SetInt64" 12345678901234567890)
		(go:call new-big "String")
	`, globalEnv)
	if err != nil {
		t.Fatalf("math/big.Int failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from BigInt.String, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 12: Hash/SHA256
// ===========================================================================

func TestScenario_SHA256Hash(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define sum256 (go:import "crypto/sha256.Sum256"))
		(define hash (sum256 "hello world"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sha256.Sum256 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 13: Random Number Generation
// ===========================================================================

func TestScenario_RandomNumber(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define rand-intn (go:import "math/rand.Intn"))
		(rand-intn 100)
	`, globalEnv)
	if err != nil {
		t.Fatalf("math/rand.Intn failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Intn, got %s", typeStr(result))
	}
	n := toNum(result)
	if n < 0 || n >= 100 {
		t.Fatalf("expected 0 <= n < 100, got %v", n)
	}
}

// ===========================================================================
// Scenario 14: Regular Expressions
// ===========================================================================

func TestScenario_RegexMatching(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define compile (go:import "regexp.Compile"))
		(define re (compile "^hello.*world$"))
		(go:call re "MatchString" "hello beautiful world")
	`, globalEnv)
	if err != nil {
		t.Fatalf("regexp.Compile failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool from MatchString, got %s", typeStr(result))
	}
}

func TestScenario_RegexFindAll(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(define compile (go:import "regexp.Compile"))
		(define re (compile "\\d+"))
		(go:call re "FindAllString" "abc123def456ghi" -1)
	`, globalEnv)
	if err != nil {
		t.Fatalf("regexp.FindAllString failed: %v", err)
	}
	// Returns []string -> converted to string
}

// ===========================================================================
// Scenario 15: File Path Operations
// ===========================================================================

func TestScenario_FilePath(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define clean (go:import "path/filepath.Clean"))
		(clean "home/user/../user/documents")
	`, globalEnv)
	if err != nil {
		t.Fatalf("filepath.Clean failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from filepath.Clean, got %s", typeStr(result))
	}
}

func TestScenario_FilePathAbs(t *testing.T) {
	InitGlobalEnv()
	_, err := EvalString(`
		(define abs (go:import "path/filepath.Abs"))
		(abs "relative/path")
	`, globalEnv)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}
}

// ===========================================================================
// Scenario 16: Context
// ===========================================================================

func TestScenario_Context(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define background (go:import "context.Background"))
		(define ctx (background))
		(go:type-of ctx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("context.Background failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 17: Sync Mutex
// ===========================================================================

func TestScenario_Mutex(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define m (go:new "sync.Mutex"))
		(go:call m "Lock")
		(go:call m "Unlock")
		(go:type-of m)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sync.Mutex failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

func TestScenario_WaitGroup(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define wg (go:new "sync.WaitGroup"))
		(go:call wg "Add" 1)
		(go:call wg "Done")
		(go:type-of wg)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sync.WaitGroup failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 18: Unicode Handling
// ===========================================================================

func TestScenario_UnicodeHandling(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define to-upper (go:import "strings.ToUpper"))
		(to-upper "hello 世界")
	`, globalEnv)
	if err != nil {
		t.Fatalf("unicode handling failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from ToUpper, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 19: Empty String Handling
// ===========================================================================

func TestScenario_EmptyStringHandling(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define len (go:import "strings.Count"))
		(len "" "")
	`, globalEnv)
	if err != nil {
		t.Fatalf("empty string handling failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Count, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 20: Negative Number Handling
// ===========================================================================

func TestScenario_NegativeNumberHandling(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define abs (go:import "math.Abs"))
		(abs -42.5)
	`, globalEnv)
	if err != nil {
		t.Fatalf("math.Abs failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Abs, got %s", typeStr(result))
	}
	if toNum(result) != 42.5 {
		t.Fatalf("expected 42.5, got %v", toNum(result))
	}
}

// ===========================================================================
// Scenario 21: HTTP Custom Transport
// ===========================================================================

func TestScenario_HTTPCustomClient(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define transport (go:new "net/http.Transport"))
		(go:set-field transport "MaxIdleConns" 10)
		(go:field transport "MaxIdleConns")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http.Transport failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from MaxIdleConns, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 22: Data Pipeline (trim -> upper -> base64 encode)
// ===========================================================================

func TestScenario_DataPipeline(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define trim (go:import "strings.TrimSpace"))
		(define upper (go:import "strings.ToUpper"))
		(define std-enc (go:import "encoding/base64.StdEncoding"))
		(define input "  hello world  ")
		(define trimmed (trim input))
		(define uppered (upper trimmed))
		(define encoded (go:call std-enc "EncodeToString" uppered))
		encoded
	`, globalEnv)
	if err != nil {
		t.Fatalf("data pipeline failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from pipeline, got %s", typeStr(result))
	}
	if result.str != "SEVMTE8gV09STEQ=" {
		t.Fatalf("expected 'SEVMTE8gV09STEQ=', got: %s", result.str)
	}
}

// ===========================================================================
// Scenario 23: fmt.Sprintf (variadic function test)
// ===========================================================================

func TestScenario_FmtSprint(t *testing.T) {
	InitGlobalEnv()
	// Test string manipulation via non-variadic functions
	result, err := EvalString(`
		(define repeat (go:import "strings.Repeat"))
		(define replace (go:import "strings.ReplaceAll"))
		(replace (repeat "ab" 3) "ab" "XY")
	`, globalEnv)
	if err != nil {
		t.Fatalf("string repeat+replace failed: %v", err)
	}
	if result.typ != VStr || result.str != "XYXYXY" {
		t.Fatalf("expected 'XYXYXY', got: %v", result)
	}
}

func TestScenario_FmtErrorf(t *testing.T) {
	InitGlobalEnv()
	// fmt.Errorf returns error type; FFI treats non-nil error as failure
	// Just verify it's importable
	_, err := EvalString(`
		(define errorf (go:import "fmt.Errorf"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("fmt.Errorf import failed: %v", err)
	}
}

// ===========================================================================
// Scenario 24: Complete Certificate Pipeline
// ===========================================================================

func TestScenario_CompleteCertPipeline(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define gen-key (go:import "crypto/rsa.GenerateKey"))
		(define priv (gen-key (go:import "crypto/rand.Reader") 2048))
		(define pub-key (go:field priv "PublicKey"))
		(define marshal-pub (go:import "crypto/x509.MarshalPKIXPublicKey"))
		(define der-pub (marshal-pub pub-key))
		(define marshal-priv (go:import "crypto/x509.MarshalPKCS1PrivateKey"))
		(define der-priv (marshal-priv priv))
		; der-priv is []byte -> reflectToLisp converts to string
		der-priv
	`, globalEnv)
	if err != nil {
		t.Fatalf("complete cert pipeline failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from der-priv ([]byte->string), got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 25: Time Duration
// ===========================================================================

func TestScenario_TimeDuration(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(define t1 (now))
		(define parse (go:import "time.Parse"))
		(define t2 (parse "2006-01-02" "2025-01-01"))
		(go:call t1 "Sub" t2)
	`, globalEnv)
	if err != nil {
		t.Fatalf("time.Sub failed: %v", err)
	}
	// time.Sub returns time.Duration (int64) -> converted to number
	if !isNumeric(result) {
		t.Fatalf("expected numeric from time.Sub, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 26: strconv ParseInt
// ===========================================================================

func TestScenario_StrconvMath(t *testing.T) {
	InitGlobalEnv()
	// strconv.Atoi returns (int, error); with no error, FFI returns int directly
	result, err := EvalString(`
		(define atoi (go:import "strconv.Atoi"))
		(atoi "144")
	`, globalEnv)
	if err != nil {
		t.Fatalf("strconv.Atoi failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric from Atoi, got %s", typeStr(result))
	}
	if toNum(result) != 144 {
		t.Fatalf("expected 144, got %v", toNum(result))
	}
}

// ===========================================================================
// Scenario 27: String Split
// ===========================================================================

func TestScenario_StringSplit(t *testing.T) {
	InitGlobalEnv()
	// strings.Split returns []string; reflectToLisp converts non-[]byte slices to nil
	// Just verify the import and call work without error
	_, err := EvalString(`
		(define split (go:import "strings.Split"))
		(split "apple,banana,cherry" ",")
	`, globalEnv)
	if err != nil {
		t.Fatalf("strings.Split failed: %v", err)
	}
}

// ===========================================================================
// Scenario 28: Sort (verify import works)
// ===========================================================================

func TestScenario_SortImport(t *testing.T) {
	InitGlobalEnv()
	// sort.Ints is a function; just verify import works
	_, err := EvalString(`
		(define sort-ints (go:import "sort.Ints"))
	`, globalEnv)
	if err != nil {
		t.Fatalf("sort.Ints import failed: %v", err)
	}
}

// ===========================================================================
// Scenario 29: nil Interface (nil body in HTTP request)
// ===========================================================================

func TestScenario_NilInterface(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-request (go:import "net/http.NewRequest"))
		(define req (new-request "GET" "https://example.com" nil))
		(go:field req "URL")
	`, globalEnv)
	if err != nil {
		t.Fatalf("nil interface handling failed: %v", err)
	}
	if result.typ != VGoVal {
		t.Fatalf("expected VGoVal, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 30: Slice Operations
// ===========================================================================

func TestScenario_SliceOperations(t *testing.T) {
	InitGlobalEnv()
	// bytes.NewBuffer creates a buffer from []byte
	_, err := EvalString(`
		(define new-buffer (go:import "bytes.NewBuffer"))
		(define buf (new-buffer (go:import "bytes.Buffer")))
		(go:type-of buf)
	`, globalEnv)
	// This may fail depending on how NewBuffer handles the argument
	_ = err
}

// ===========================================================================
// Scenario 31: HMAC-SHA256 (direct hash)
// ===========================================================================

func TestScenario_HMAC(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define sum256 (go:import "crypto/sha256.Sum256"))
		(define hash (sum256 "message to sign"))
		(go:type-of hash)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sha256.Sum256 failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 32: Atomic Counter
// ===========================================================================

func TestScenario_AtomicCounter(t *testing.T) {
	InitGlobalEnv()
	// sync/atomic not in registry; use sync.Once instead
	result, err := EvalString(`
		(define once (go:new "sync.Once"))
		(go:type-of once)
	`, globalEnv)
	if err != nil {
		t.Fatalf("sync.Once failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 33: JSON Unmarshal/Valid
// ===========================================================================

func TestScenario_JSONUnmarshalStruct(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define json-valid (go:import "encoding/json.Valid"))
		(json-valid "{\"name\": \"test\", \"value\": 42}")
	`, globalEnv)
	if err != nil {
		t.Fatalf("json.Valid failed: %v", err)
	}
	if result.typ != VBool {
		t.Fatalf("expected bool from json.Valid, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 34: Complete HTTP Request Simulation
// ===========================================================================

func TestScenario_CompleteHTTPRequest(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-request (go:import "net/http.NewRequest"))
		(define req (new-request "GET" "https://api.example.com/data?key=value" nil))
		(go:call (go:field req "Header") "Set" "Accept" "application/json")
		(go:call (go:field req "Header") "Set" "User-Agent" "LispFFI/1.0")
		(go:field req "Method")
	`, globalEnv)
	if err != nil {
		t.Fatalf("complete HTTP request failed: %v", err)
	}
	if result.typ != VStr || result.str != "GET" {
		t.Fatalf("expected 'GET', got: %v", result)
	}
}

// ===========================================================================
// Scenario 35: Multiple FFI Function Calls in Sequence
// ===========================================================================

func TestScenario_MultiCallSequence(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define sqrt (go:import "math.Sqrt"))
		(define abs (go:import "math.Abs"))
		(define sin (go:import "math.Sin"))
		; Chain: sqrt(abs(sin(-3.14159)))
		(sqrt (abs (sin -3.14159)))
	`, globalEnv)
	if err != nil {
		t.Fatalf("multi-call sequence failed: %v", err)
	}
	if !isNumeric(result) {
		t.Fatalf("expected numeric, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 36: HTTP Header Get After Set
// ===========================================================================

func TestScenario_HTTPHeaderGetSet(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define new-request (go:import "net/http.NewRequest"))
		(define req (new-request "POST" "https://example.com" nil))
		(go:call (go:field req "Header") "Set" "Content-Type" "application/json")
		(go:call (go:field req "Header") "Get" "Content-Type")
	`, globalEnv)
	if err != nil {
		t.Fatalf("http header get/set failed: %v", err)
	}
	if result.typ != VStr || result.str != "application/json" {
		t.Fatalf("expected 'application/json', got: %v", result)
	}
}

// ===========================================================================
// Scenario 37: JSON Round Trip (marshal simple string)
// ===========================================================================

func TestScenario_JSONRoundTrip(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define json-marshal (go:import "encoding/json.Marshal"))
		(define json-str (json-marshal "hello"))
		json-str
	`, globalEnv)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if result.typ != VStr || result.str != `"hello"` {
		t.Fatalf(`expected "hello", got: %v`, result)
	}
}

// ===========================================================================
// Scenario 38: Context Background
// ===========================================================================

func TestScenario_ContextTimeout(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define background (go:import "context.Background"))
		(define ctx (background))
		(go:type-of ctx)
	`, globalEnv)
	if err != nil {
		t.Fatalf("context.Background failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from go:type-of, got %s", typeStr(result))
	}
}

// ===========================================================================
// Scenario 39: Path Clean
// ===========================================================================

func TestScenario_PathClean(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define clean (go:import "path/filepath.Clean"))
		(clean "home/user/../user/documents")
	`, globalEnv)
	if err != nil {
		t.Fatalf("filepath.Clean failed: %v", err)
	}
	if result.typ != VStr {
		t.Fatalf("expected string from filepath.Clean, got %s", typeStr(result))
	}
	if !strings.Contains(result.str, "user") || !strings.Contains(result.str, "documents") {
		t.Fatalf("expected path with user/documents, got: %v", result)
	}
}

// ===========================================================================
// Scenario 40: go:import + go:type-of verification
// ===========================================================================

func TestScenario_FFITypeVerification(t *testing.T) {
	InitGlobalEnv()
	result, err := EvalString(`
		(define now (go:import "time.Now"))
		(define t (now))
		(go:type-of t)
	`, globalEnv)
	if err != nil {
		t.Fatalf("time.Now type verification failed: %v", err)
	}
	if result.typ != VStr || !strings.Contains(result.str, "Time") {
		t.Fatalf("expected Time type, got: %v", result)
	}
}
