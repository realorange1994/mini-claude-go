package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir, _ := os.MkdirTemp("", "test_direct_*")
	defer os.RemoveAll(dir)

	// Use the exact same path format as the Lisp code produces
	fwdPath := filepath.ToSlash(filepath.Join(dir, "test_write.txt"))
	fmt.Printf("Testing path: %s\n", fwdPath)

	// Try to create the file directly
	f, err := os.OpenFile(fwdPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Direct Go OpenFile error: %v\n", err)
	} else {
		f.WriteString("hello")
		f.Close()
		fmt.Println("Direct Go OpenFile: SUCCESS")
		data, _ := os.ReadFile(fwdPath)
		fmt.Printf("Content: %s\n", string(data))
	}
}
