package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"miniclaudecode-go/microlisp"
	"miniclaudecode-go/tools"
)

func main() {
	microlisp.InitGlobalEnv()
	microlisp.ResetGlobalEnv()

	// Create test file
	testFile := "/tmp/test_lisp_tools.txt"
	os.WriteFile(testFile, []byte("test content line1\ntest content line2\ntest content line3\n"), 0644)

	tool := &tools.LispToolsTool{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Testing lisp read operation...")
	result := tool.ExecuteContext(ctx, map[string]any{
		"operation": "read",
		"file_path": testFile,
	})
	fmt.Printf("Read result (IsError=%v): %s\n", result.IsError, result.Output)

	fmt.Println("\nTesting lisp write operation...")
	result = tool.ExecuteContext(ctx, map[string]any{
		"operation": "write",
		"file_path": testFile,
		"content":   "new content\n",
	})
	fmt.Printf("Write result (IsError=%v): %s\n", result.IsError, result.Output)

	fmt.Println("\nAll tests done.")
}
