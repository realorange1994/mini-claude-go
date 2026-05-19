package microlisp

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ======== io.Reader adapters ========

// lispStringReader wraps a string as io.Reader, io.ReaderAt, io.Seeker.
type lispStringReader struct {
	s string
	i int64
}

func (r *lispStringReader) Read(p []byte) (int, error) {
	if r.i >= int64(len(r.s)) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += int64(n)
	return n, nil
}

func (r *lispStringReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.s)) {
		return 0, io.EOF
	}
	n := copy(p, r.s[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *lispStringReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case 0:
		abs = offset
	case 1:
		abs = r.i + offset
	case 2:
		abs = int64(len(r.s)) + offset
	default:
		return 0, fmt.Errorf("seek: invalid whence")
	}
	if abs < 0 || abs > int64(len(r.s)) {
		return 0, fmt.Errorf("seek: invalid offset")
	}
	r.i = abs
	return abs, nil
}

// lispBufferReader wraps a byte slice as io.Reader, io.ReaderAt, io.Seeker.
type lispBufferReader struct {
	buf []byte
	i   int64
}

func (r *lispBufferReader) Read(p []byte) (int, error) {
	if r.i >= int64(len(r.buf)) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.i:])
	r.i += int64(n)
	return n, nil
}

func (r *lispBufferReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.buf)) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *lispBufferReader) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case 0:
		abs = offset
	case 1:
		abs = r.i + offset
	case 2:
		abs = int64(len(r.buf)) + offset
	default:
		return 0, fmt.Errorf("seek: invalid whence")
	}
	if abs < 0 || abs > int64(len(r.buf)) {
		return 0, fmt.Errorf("seek: invalid offset")
	}
	r.i = abs
	return abs, nil
}

// lispFileReader wraps an open file as io.Reader + io.ReadCloser.
type lispFileReader struct {
	f *os.File
}

func (r *lispFileReader) Read(p []byte) (int, error) {
	return r.f.Read(p)
}

func (r *lispFileReader) Close() error {
	return r.f.Close()
}

// ======== io.Writer adapters ========

// lispStringWriter accumulates written bytes into a string.
type lispStringWriter struct {
	buf bytes.Buffer
}

func (w *lispStringWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *lispStringWriter) WriteString(s string) (int, error) {
	return w.buf.WriteString(s)
}

func (w *lispStringWriter) String() string {
	return w.buf.String()
}

func (w *lispStringWriter) Bytes() []byte {
	return w.buf.Bytes()
}

func (w *lispStringWriter) Reset() {
	w.buf.Reset()
}

// lispBufferWriter wraps a bytes.Buffer pointer so writes go into it.
type lispBufferWriter struct {
	buf *bytes.Buffer
}

func (w *lispBufferWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func (w *lispBufferWriter) WriteString(s string) (int, error) {
	return w.buf.WriteString(s)
}

func (w *lispBufferWriter) Bytes() []byte {
	return w.buf.Bytes()
}

func (w *lispBufferWriter) Reset() {
	w.buf.Reset()
}

// lispFileWriter wraps an open file as io.Writer + io.WriteCloser.
type lispFileWriter struct {
	f *os.File
}

func (w *lispFileWriter) Write(p []byte) (int, error) {
	return w.f.Write(p)
}

func (w *lispFileWriter) WriteString(s string) (int, error) {
	return w.f.WriteString(s)
}

func (w *lispFileWriter) Close() error {
	return w.f.Close()
}

// ======== Context adapters ========

// lispCancelFunc wraps a context.CancelFunc as a VGoVal.
type lispCancelFunc struct {
	fn context.CancelFunc
}

func (c *lispCancelFunc) Call() {
	c.fn()
}

// ======== Go callback registry ========
// For functions that take Go callbacks like func(int32) bool.

var callbackID atomic.Int64
var callbackStore sync.Map // id -> goCallback

type goCallback struct {
	fn  *Value // Lisp function
	sig string // Signature hint: "int32->bool", "int32->int32", etc.
}

// makeGoCallback creates a Go function of the given type that calls the Lisp function.
func makeGoCallback(lispFn *Value, sig string) (reflect.Value, error) {
	var goFnType reflect.Type

	switch sig {
	case "int32->bool":
		goFnType = reflect.TypeOf(func(int32) bool { return false })
	case "int32->int32":
		goFnType = reflect.TypeOf(func(int32) int32 { return 0 })
	case "int->int":
		goFnType = reflect.TypeOf(func(int) int { return 0 })
	case "int->bool":
		goFnType = reflect.TypeOf(func(int) bool { return false })
	case "int,int->":
		goFnType = reflect.TypeOf(func(int, int) {})
	case "string->string":
		goFnType = reflect.TypeOf(func(string) string { return "" })
	case "()->":
		goFnType = reflect.TypeOf(func() {})
	case "string->error":
		goFnType = reflect.TypeOf(func(string) error { return nil })
	default:
		return reflect.Value{}, fmt.Errorf("go:callback: unknown signature %q", sig)
	}

	id := callbackID.Add(1) - 1
	cb := goCallback{fn: lispFn, sig: sig}
	callbackStore.Store(id, cb)

	return reflect.MakeFunc(goFnType, func(args []reflect.Value) []reflect.Value {
		cbVal, ok := callbackStore.Load(id)
		if !ok {
			panic(fmt.Sprintf("go:callback: stale callback ID %d", id))
		}
		cb := cbVal.(goCallback)

		// Convert Go args to Lisp values
		lispArgs := make([]*Value, len(args))
		for i, arg := range args {
			lispArgs[i] = reflectToLisp(arg)
		}

		// Call the Lisp function via Eval
		result, err := Eval(makeApplyForm(cb.fn, lispArgs), globalEnv)
		if err != nil {
			panic(fmt.Sprintf("go:callback: %v", err))
		}

		// Convert Lisp result back to Go based on return type
		if goFnType.NumOut() == 0 {
			return nil
		}
		retType := goFnType.Out(0)
		rv, convErr := lispToReflectSafe(result, retType)
		if convErr != nil {
			// Fallback: try to use the default zero value
			rv = reflect.Zero(retType)
		}
		return []reflect.Value{rv}
	}), nil
}

// makeApplyForm creates a Lisp form (funcall fn arg1 arg2 ...) for Eval.
func makeApplyForm(fn *Value, args []*Value) *Value {
	// Build (funcall fn arg1 arg2 ...)
	// NOTE: must use vsym, not vstr — Eval dispatches special forms by symbol name
	applyForm := vsym("funcall")
	forms := []*Value{applyForm, fn}
	for _, a := range args {
		forms = append(forms, a)
	}
	return listFromSlice(forms)
}

// ======== Register factories and convenience functions ========

func init() {
	// Register io adapter factory functions
	GoPackageRegistry["microlisp/io"] = map[string]reflect.Value{
		// Reader factories
		"NewStringReader": reflect.ValueOf(newStringReader),
		"NewBufferReader": reflect.ValueOf(newBufferReader),
		"NewFileReader":   reflect.ValueOf(newFileReader),

		// Writer factories
		"NewStringWriter": reflect.ValueOf(newStringWriter),
		"NewBufferWriter": reflect.ValueOf(newBufferWriter),
		"NewFileWriter":   reflect.ValueOf(newFileWriter),

		// Writer getters (to retrieve accumulated content)
		"StringWriterString": reflect.ValueOf(stringWriterString),
		"BufferWriterBytes":  reflect.ValueOf(bufferWriterBytes),
		"BufferWriterReset":  reflect.ValueOf(bufferWriterReset),
		"StringWriterReset":  reflect.ValueOf(stringWriterReset),

		// Reader/Writer close operations
		"FileReaderClose": reflect.ValueOf(fileReaderClose),
		"FileWriterClose": reflect.ValueOf(fileWriterClose),

		// Context operations (concrete types — work via reflect)
		"ContextCancel": reflect.ValueOf(ctxCancel),
		"ContextDone":   reflect.ValueOf(ctxDone),

		// NOTE: Functions that take/return *Value are registered as builtins
		// in builtin_register.go, NOT here. See: reader-read-all,
		// io-copy-to-string, io-copy-to-file, io-limit-string,
		// io-nop-closer, ctx-with-timeout, ctx-with-cancel,
		// go:callback, http-request, http-do
	}

	// fmt-style formatting (variadic ...interface{} — works via CallSlice)
	GoPackageRegistry["microlisp/fmt"] = map[string]reflect.Value{
		"FormatString": reflect.ValueOf(fmtSprintf),
	}

	// encoding/binary wrappers
	GoPackageRegistry["microlisp/binary"] = map[string]reflect.Value{
		"BinaryReadUint32":  reflect.ValueOf(binaryReadUint32),
		"BinaryReadUint64":  reflect.ValueOf(binaryReadUint64),
		"BinaryReadInt32":   reflect.ValueOf(binaryReadInt32),
		"BinaryReadInt64":   reflect.ValueOf(binaryReadInt64),
		"BinaryWriteUint32": reflect.ValueOf(binaryWriteUint32),
		"BinaryWriteUint64": reflect.ValueOf(binaryWriteUint64),
	}

	// JSON convenience (takes interface{} — works via reflect)
	GoPackageRegistry["microlisp/jsonx"] = map[string]reflect.Value{
		"JsonMarshalIndent": reflect.ValueOf(jsonMarshalIndent),
	}
}

// ======== Factory functions for io.Reader ========

func newStringReader(s string) *lispStringReader {
	return &lispStringReader{s: s, i: 0}
}

func newBufferReader(data string) *lispBufferReader {
	return &lispBufferReader{buf: []byte(data), i: 0}
}

func newFileReader(path string) (*lispFileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &lispFileReader{f: f}, nil
}

// ======== Factory functions for io.Writer ========

func newStringWriter() *lispStringWriter {
	return &lispStringWriter{}
}

func newBufferWriter() *lispBufferWriter {
	return &lispBufferWriter{buf: &bytes.Buffer{}}
}

func newFileWriter(path string) (*lispFileWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &lispFileWriter{f: f}, nil
}

// ======== Getter functions ========

func stringWriterString(w *lispStringWriter) string {
	return w.String()
}

func bufferWriterBytes(w *lispBufferWriter) []byte {
	return w.Bytes()
}

func bufferWriterReset(w *lispBufferWriter) {
	w.Reset()
}

func stringWriterReset(w *lispStringWriter) {
	w.Reset()
}

// ======== Reader operations ========

func fileReaderClose(r *lispFileReader) error {
	return r.Close()
}

func fileWriterClose(w *lispFileWriter) error {
	return w.Close()
}

// ======== Context operations ========
// ctxWithTimeout and ctxWithCancel are implemented as builtins (see below).
// ctxCancel and ctxDone work via reflect on concrete types.

func ctxCancel(c *lispCancelFunc) {
	c.fn()
}

func ctxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// ======== Callback factory ========
// makeGoCallback is used by builtinGoCallback (see below).
// The old makeCallbackBuiltin is replaced by builtinGoCallback.

// ======== fmt convenience ========

func fmtSprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// ======== encoding/binary convenience ========

func binaryReadUint32(data string, order string) uint32 {
	if len(data) < 4 {
		return 0
	}
	b := []byte(data)
	if order == "big" {
		return binary.BigEndian.Uint32(b)
	}
	return binary.LittleEndian.Uint32(b)
}

func binaryReadUint64(data string, order string) uint64 {
	if len(data) < 8 {
		return 0
	}
	b := []byte(data)
	if order == "big" {
		return binary.BigEndian.Uint64(b)
	}
	return binary.LittleEndian.Uint64(b)
}

func binaryReadInt32(data string, order string) int32 {
	return int32(binaryReadUint32(data, order))
}

func binaryReadInt64(data string, order string) int64 {
	return int64(binaryReadUint64(data, order))
}

func binaryWriteUint32(n uint32, order string) string {
	b := make([]byte, 4)
	if order == "big" {
		binary.BigEndian.PutUint32(b, n)
	} else {
		binary.LittleEndian.PutUint32(b, n)
	}
	return string(b)
}

func binaryWriteUint64(n uint64, order string) string {
	b := make([]byte, 8)
	if order == "big" {
		binary.BigEndian.PutUint64(b, n)
	} else {
		binary.LittleEndian.PutUint64(b, n)
	}
	return string(b)
}

// ======== JSON convenience ========

func jsonMarshalIndent(v interface{}, prefix, indent string) (string, error) {
	b, err := json.MarshalIndent(v, prefix, indent)
	return string(b), err
}

// ======== Builtin wrappers for *Value-taking functions ========
// These cannot go through GoPackageRegistry because the FFI reflect system
// cannot handle *Value parameters. They are registered as builtins in
// builtin_register.go.

// builtinReaderReadAll reads all data from a VGoVal io.Reader.
func builtinReaderReadAll(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("reader-read-all: need 1 argument (io.Reader)")
	}
	r, err := extractIOReader(args[0])
	if err != nil {
		return nil, fmt.Errorf("reader-read-all: %w", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return vstr(string(data)), nil
}

// builtinIoCopyToString reads all data from a VGoVal io.Reader into a string.
func builtinIoCopyToString(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("io-copy-to-string: need 1 argument (io.Reader)")
	}
	r, err := extractIOReader(args[0])
	if err != nil {
		return nil, fmt.Errorf("io-copy-to-string: %w", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return vstr(string(data)), nil
}

// builtinIoCopyToFile copies all data from a VGoVal io.Reader to a file.
func builtinIoCopyToFile(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("io-copy-to-file: need reader and path")
	}
	r, err := extractIOReader(args[0])
	if err != nil {
		return nil, fmt.Errorf("io-copy-to-file: %w", err)
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("io-copy-to-file: path must be a string")
	}
	f, err := os.Create(args[1].str)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	if err != nil {
		return nil, err
	}
	return vnil(), nil
}

// builtinIoLimitString reads up to n bytes from a VGoVal io.Reader.
func builtinIoLimitString(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("io-limit-string: need reader and byte count")
	}
	r, err := extractIOReader(args[0])
	if err != nil {
		return nil, fmt.Errorf("io-limit-string: %w", err)
	}
	if !isNumeric(args[1]) {
		return nil, fmt.Errorf("io-limit-string: byte count must be a number")
	}
	n := int(toNum(args[1]))
	lr := io.LimitReader(r, int64(n))
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	return vstr(string(data)), nil
}

// builtinIoNopCloser wraps a VGoVal io.Reader as io.ReadCloser.
func builtinIoNopCloser(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("io-nop-closer: need 1 argument (io.Reader)")
	}
	r, err := extractIOReader(args[0])
	if err != nil {
		return nil, fmt.Errorf("io-nop-closer: %w", err)
	}
	closer := io.NopCloser(r)
	return &Value{
		typ:          VGoVal,
		goVal:        closer,
		goValType:    reflect.TypeOf((*io.ReadCloser)(nil)).Elem(),
		goValReflect: reflect.ValueOf(closer),
	}, nil
}

// builtinCtxWithTimeout creates a context with timeout.
func builtinCtxWithTimeout(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("ctx-with-timeout: need timeout in seconds")
	}
	var parentCtx context.Context = context.Background()
	if len(args) >= 2 && args[0].typ == VGoVal {
		if ctx, ok := args[0].goVal.(context.Context); ok {
			parentCtx = ctx
		}
	}
	secondsIdx := 0
	if len(args) >= 2 {
		secondsIdx = 1
	}
	if !isNumeric(args[secondsIdx]) {
		return nil, fmt.Errorf("ctx-with-timeout: seconds must be a number")
	}
	d := time.Duration(toNum(args[secondsIdx]) * float64(time.Second))
	ctx, cancel := context.WithTimeout(parentCtx, d)

	ctxVal := &Value{
		typ:       VGoVal,
		goVal:     ctx,
		goValType: reflect.TypeOf((*context.Context)(nil)).Elem(),
	}
	cancelVal := &Value{
		typ:       VGoVal,
		goVal:     &lispCancelFunc{fn: cancel},
		goValType: reflect.TypeOf(&lispCancelFunc{}),
	}
	return listFromSlice([]*Value{ctxVal, cancelVal}), nil
}

// builtinCtxWithCancel creates a context with cancel function.
func builtinCtxWithCancel(args []*Value) (*Value, error) {
	var parentCtx context.Context = context.Background()
	if len(args) >= 1 {
		if args[0].typ == VGoVal {
			if ctx, ok := args[0].goVal.(context.Context); ok {
				parentCtx = ctx
			}
		} else if args[0].typ != VNil {
			return nil, fmt.Errorf("ctx-with-cancel: parent must be context or nil")
		}
	}

	ctx, cancel := context.WithCancel(parentCtx)
	ctxVal := &Value{
		typ:       VGoVal,
		goVal:     ctx,
		goValType: reflect.TypeOf((*context.Context)(nil)).Elem(),
	}
	cancelVal := &Value{
		typ:       VGoVal,
		goVal:     &lispCancelFunc{fn: cancel},
		goValType: reflect.TypeOf(&lispCancelFunc{}),
	}
	return listFromSlice([]*Value{ctxVal, cancelVal}), nil
}

// builtinGoCallback creates a Go callback from a Lisp function.
func builtinGoCallback(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("go:callback: need function and signature string")
	}
	if args[1].typ != VStr {
		return nil, fmt.Errorf("go:callback: second arg must be a signature string")
	}
	goFn, err := makeGoCallback(args[0], args[1].str)
	if err != nil {
		return nil, err
	}
	return &Value{
		typ:          VGoVal,
		goVal:        goFn.Interface(),
		goValType:    goFn.Type(),
		goValReflect: goFn,
	}, nil
}

// builtinHttpRequest creates and optionally executes an HTTP request.
// (http-request method url &optional body) -> VGoVal *http.Request
func builtinHttpRequest(args []*Value) (*Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("http-request: need method and url")
	}
	if args[0].typ != VStr || args[1].typ != VStr {
		return nil, fmt.Errorf("http-request: method and url must be strings")
	}
	method := args[0].str
	url := args[1].str
	var bodyReader io.Reader
	if len(args) >= 3 && args[2].typ == VStr && args[2].str != "" {
		bodyReader = strings.NewReader(args[2].str)
	} else if len(args) >= 3 && args[2].typ == VGoVal {
		if r, ok := args[2].goVal.(io.Reader); ok {
			bodyReader = r
		}
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	return &Value{
		typ:          VGoVal,
		goVal:        req,
		goValType:    reflect.TypeOf(req),
		goValReflect: reflect.ValueOf(req),
	}, nil
}

// builtinHttpDo executes an HTTP request and returns (body-string status-code).
func builtinHttpDo(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("http-do: need *http.Request")
	}
	var req *http.Request
	if args[0].typ == VGoVal {
		if httpReq, ok := args[0].goVal.(*http.Request); ok {
			req = httpReq
		} else {
			return nil, fmt.Errorf("http-do: need *http.Request")
		}
	} else {
		return nil, fmt.Errorf("http-do: need VGoVal *http.Request")
	}

	c := http.DefaultClient
	if len(args) >= 2 && args[1].typ == VGoVal {
		if hc, ok := args[1].goVal.(*http.Client); ok {
			c = hc
		}
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return listFromSlice([]*Value{vstr(string(data)), vnum(float64(resp.StatusCode))}), nil
}

// extractIOReader extracts an io.Reader from a VGoVal Value.
func extractIOReader(v *Value) (io.Reader, error) {
	if v.typ != VGoVal {
		return nil, fmt.Errorf("need VGoVal io.Reader")
	}
	reader, ok := v.goVal.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("value is not an io.Reader (got %T)", v.goVal)
	}
	return reader, nil
}