package microlisp

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// skipFuncsFromGen is copied from ffi_gen/lisp_gen.go — functions the generator
// explicitly skips (they have hand-written wrappers).
var skipFuncsFromGen = map[string]bool{
	// Internal microlisp adapter functions — for go:import, not lazy-loaded
	"microlisp/io.NewStringReader":    true,
	"microlisp/io.NewBufferReader":    true,
	"microlisp/io.NewFileReader":      true,
	"microlisp/io.NewStringWriter":    true,
	"microlisp/io.NewBufferWriter":    true,
	"microlisp/io.NewFileWriter":      true,
	"microlisp/io.StringWriterString": true,
	"microlisp/io.BufferWriterBytes":  true,
	"microlisp/io.BufferWriterReset":  true,
	"microlisp/io.StringWriterReset":  true,
	"microlisp/io.FileReaderClose":    true,
	"microlisp/io.FileWriterClose":    true,
	"microlisp/io.ContextCancel":      true,
	"microlisp/io.ContextDone":        true,
	"microlisp/fmt.FormatString":      true,
	"microlisp/binary.BinaryReadUint32":  true,
	"microlisp/binary.BinaryReadUint64":  true,
	"microlisp/binary.BinaryReadInt32":   true,
	"microlisp/binary.BinaryReadInt64":   true,
	"microlisp/binary.BinaryWriteUint32": true,
	"microlisp/binary.BinaryWriteUint64": true,
	"microlisp/jsonx.JsonMarshalIndent": true,
	"strings.Contains":       true,
	"strings.HasPrefix":      true,
	"strings.HasSuffix":      true,
	"strings.Split":          true,
	"strings.SplitN":         true,
	"strings.Join":           true,
	"strings.Replace":        true,
	"strings.ReplaceAll":     true,
	"strings.TrimSpace":      true,
	"strings.Trim":           true,
	"strings.TrimLeft":       true,
	"strings.TrimRight":      true,
	"strings.ToUpper":        true,
	"strings.ToLower":        true,
	"strings.Repeat":         true,
	"os.ReadFile":            true,
	"os.WriteFile":           true,
	"os.OpenFile":            true,
	"os.Stat":                true,
	"os.Remove":              true,
	"os.Rename":              true,
	"os.ReadDir":             true,
	"os.Mkdir":               true,
	"os.MkdirAll":            true,
	"os.CreateTemp":          true,
	"os.Getenv":              true,
	"os.Setenv":              true,
	"os.Unsetenv":            true,
	"os.Environ":             true,
	"os.Getwd":               true,
	"os.Chdir":               true,
	"os.Getpid":              true,
	"os.Hostname":            true,
	"os.ExpandEnv":           true,
	"net/http.Get":           true,
	"net/http.Post":          true,
	"net/http.StatusText":    true,
	"path/filepath.Abs":      true,
	"path/filepath.Base":     true,
	"path/filepath.Dir":      true,
	"path/filepath.Ext":      true,
	"path/filepath.Join":     true,
	"path/filepath.Clean":    true,
	"path/filepath.IsAbs":    true,
	"regexp.MatchString":     true,
	"regexp.Compile":         true,
	"io.ReadAll":             true,
	"time.Now":               true,
	"time.Unix":              true,
	"time.Parse":             true,
	"encoding/json.Valid":    true,
	"encoding/json.Marshal":  true,
	"net/url.QueryEscape":    true,
	"net/url.QueryUnescape":  true,
	"net/url.Parse":          true,
	"os/exec.Command":        true,
	"os/exec.LookPath":       true,
	"runtime.Version":        true,
	"runtime.NumCPU":         true,
	"os.Exit":                true,
	"time.Sleep":             true,
	"flag.Usage":             true, // variable to assign, not a function to call
}

// incompatibleParamPatterns is copied from ffi_gen/lisp_gen.go.
// A parameter whose type string contains any of these is considered incompatible.
var incompatibleParamPatterns = []string{
	"context.Context",
	"io.Writer",
	"io.Reader",
	"io.ReadWriter",
	"io.WriteCloser",
	"io.ReadCloser",
	"io.ReadSeeker",
	"io.Seeker",
	"io.ReaderAt",
	"io.WriterAt",
	"http.ResponseWriter",
	"http.Handler",
	"http.Flusher",
	"http.Hijacker",
	"http.Pusher",
	"func(",
	"chan ",
	"reflect.Type",
	"reflect.Value",
	"testing.T",
	"testing.B",
	"testing.F",
}

// hasIncompatibleParam returns the first incompatible parameter type string found,
// or "" if all parameters are compatible.
func hasIncompatibleParam(t reflect.Type) string {
	if t.Kind() != reflect.Func {
		return "not-a-function"
	}
	for i := 0; i < t.NumIn(); i++ {
		paramType := t.In(i).String()
		for _, pattern := range incompatibleParamPatterns {
			if strings.Contains(paramType, pattern) {
				return paramType
			}
		}
	}
	// Also check variadic element type
	if t.IsVariadic() && t.NumIn() > 0 {
		variadicType := t.In(t.NumIn() - 1).String()
		for _, pattern := range incompatibleParamPatterns {
			if strings.Contains(variadicType, pattern) {
				return variadicType
			}
		}
	}
	return ""
}

// funcSignature returns a human-readable signature string.
func funcSignature(t reflect.Type) string {
	if t.Kind() != reflect.Func {
		return t.String()
	}
	var b strings.Builder
	b.WriteString("func(")
	for i := 0; i < t.NumIn(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		if t.IsVariadic() && i == t.NumIn()-1 {
			b.WriteString("...")
			b.WriteString(t.In(i).Elem().String())
		} else {
			b.WriteString(t.In(i).String())
		}
	}
	b.WriteString(")")
	switch t.NumOut() {
	case 0:
	case 1:
		b.WriteString(" " + t.Out(0).String())
	default:
		b.WriteString(" (")
		for i := 0; i < t.NumOut(); i++ {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(t.Out(i).String())
		}
		b.WriteString(")")
	}
	return b.String()
}

type auditEntry struct {
	goPath     string // e.g. "math.Sin"
	incompat   string // "" if compatible, otherwise the offending param type
	signature  string
	numParams  int
	numReturns int
}

func TestAuditMissingFromLazyTable(t *testing.T) {
	InitGlobalEnv()

	// Build set of Go import paths that ARE in the lazy table.
	// GoFFILazyTable values are "pkg.Func" strings.
	lazyGoPaths := map[string]bool{}
	for _, goPath := range GoFFILazyTable {
		lazyGoPaths[goPath] = true
	}

	// Scan GoPackageRegistry for functions that are:
	//  1) NOT in GoFFILazyTable (by their pkg.Func path)
	//  2) NOT in skipFuncsFromGen
	//  3) Are actually functions (reflect.Func)
	var missing []auditEntry

	for pkg, symbols := range GoPackageRegistry {
		for name, val := range symbols {
			if val.Kind() != reflect.Func {
				continue // only audit functions
			}
			goPath := pkg + "." + name
			if lazyGoPaths[goPath] {
				continue // already in lazy table
			}
			if skipFuncsFromGen[goPath] {
				continue // explicitly skipped in generator
			}
			rt := val.Type()
			incompat := hasIncompatibleParam(rt)
			missing = append(missing, auditEntry{
				goPath:     goPath,
				incompat:   incompat,
				signature:  funcSignature(rt),
				numParams:  rt.NumIn(),
				numReturns: rt.NumOut(),
			})
		}
	}

	// Sort for deterministic output
	sort.Slice(missing, func(i, j int) bool { return missing[i].goPath < missing[j].goPath })

	// Classify
	var incompatible, compatible []auditEntry
	for _, e := range missing {
		if e.incompat != "" {
			incompatible = append(incompatible, e)
		} else {
			compatible = append(compatible, e)
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("=== FFI Audit: Functions in GoPackageRegistry but NOT in GoFFILazyTable ===")
	fmt.Println("============================================================")
	fmt.Printf("\nTotal missing functions: %d\n", len(missing))
	fmt.Printf("  - incompatible_params: %d\n", len(incompatible))
	fmt.Printf("  - compatible (SHOULD be in lazy table): %d\n", len(compatible))

	// Detail: incompatible
	if len(incompatible) > 0 {
		fmt.Println("\n--- Incompatible parameter types ---")
		// Group by incompat reason
		byReason := map[string][]auditEntry{}
		for _, e := range incompatible {
			byReason[e.incompat] = append(byReason[e.incompat], e)
		}
		reasons := make([]string, 0, len(byReason))
		for r := range byReason {
			reasons = append(reasons, r)
		}
		sort.Strings(reasons)
		for _, reason := range reasons {
			items := byReason[reason]
			fmt.Printf("\n  Reason: %s (%d functions)\n", reason, len(items))
			for _, e := range items {
				fmt.Printf("    %s %s\n", e.goPath, e.signature)
			}
		}
	}

	// Detail: compatible — these are the ones that SHOULD be in the lazy table
	if len(compatible) > 0 {
		fmt.Println("\n--- Compatible (SHOULD be in lazy table) ---")
		for _, e := range compatible {
			fmt.Printf("  %s %s\n", e.goPath, e.signature)
		}
	}

	fmt.Println("\n============================================================")

	// Fail the test if there are compatible functions that should be in the lazy table
	if len(compatible) > 0 {
		t.Errorf("%d compatible function(s) are missing from GoFFILazyTable — see output above", len(compatible))
	}
}
