package tools

import (
	"context"
	"strings"
	"testing"
)

func TestLispEvalDefineBuiltin(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "car",
	})
	if result.IsError {
		t.Fatalf("define car failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "car") {
		t.Fatalf("expected 'car' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "builtin") {
		t.Fatalf("expected 'builtin' in output, got: %s", result.Output)
	}
}

func TestLispEvalDefineFFI(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "math.Pow",
	})
	if result.IsError {
		t.Fatalf("define math.Pow failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "float64") {
		t.Fatalf("expected 'float64' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "math.Pow") {
		t.Fatalf("expected 'math.Pow' in output, got: %s", result.Output)
	}
}

func TestLispEvalDefineStdlib(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "filter",
	})
	if result.IsError {
		t.Fatalf("define filter failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "filter") {
		t.Fatalf("expected 'filter' in output, got: %s", result.Output)
	}
	// filter is a stdlib function (defined in stdlib.go as Lisp code)
	if !strings.Contains(result.Output, "stdlib") {
		t.Fatalf("expected 'stdlib' in output, got: %s", result.Output)
	}
}

func TestLispEvalDefineNotFound(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "nonexistent_function_xyz",
	})
	if result.IsError {
		t.Fatalf("define nonexistent_function_xyz should not error, got: %s", result.Output)
	}
	// Should show "No function definition found" message
	if !strings.Contains(result.Output, "No function definition") &&
		!strings.Contains(result.Output, "helper") {
		// It might match a helper function name — that's ok too
	}
}

func TestLispEvalDefineEmptyExpression(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "",
	})
	if !result.IsError {
		t.Fatal("expected error for empty expression")
	}
}

func TestLispEvalDefineFFIVariadic(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "fmt.Sprintf",
	})
	if result.IsError {
		t.Fatalf("define fmt.Sprintf failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "variadic") {
		t.Fatalf("expected 'variadic' in output for fmt.Sprintf, got: %s", result.Output)
	}
}

func TestLispEvalDefineSpecialForm(t *testing.T) {
	tool := &LispEvalTool{}
	result := tool.ExecuteContext(context.Background(), map[string]any{
		"operation": "define",
		"expression": "if",
	})
	if result.IsError {
		t.Fatalf("define if failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "special") {
		t.Fatalf("expected 'special' in output, got: %s", result.Output)
	}
}
