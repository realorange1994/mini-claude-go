//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	bashPath := `C:\Program Files\Git\bin\bash.exe`
	
	// Test 1: simple command
	cmd := exec.Command(bashPath, "-c", "echo hello")
	out, err := cmd.CombinedOutput()
	fmt.Printf("Test1: out=%q err=%v\n", string(out), err)
	
	// Test 2: with working dir
	wd, _ := os.Getwd()
	fmt.Printf("CWD: %s\n", wd)
	
	cmd2 := exec.Command(bashPath, "-c", "echo hello")
	cmd2.Dir = wd
	out2, err := cmd2.CombinedOutput()
	fmt.Printf("Test2 (with Dir): out=%q err=%v\n", string(out2), err)
	
	// Test 3: with POSIX working dir
	cmd3 := exec.Command(bashPath, "-c", "echo hello")
	cmd3.Dir = "/e/Git/miniClaudeCode-go-github/tools"
	out3, err := cmd3.CombinedOutput()
	fmt.Printf("Test3 (POSIX Dir): out=%q err=%v\n", string(out3), err)
}
