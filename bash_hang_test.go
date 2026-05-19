package main

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestGitBashHangPipe(t *testing.T) {
	// Test: env | grep -i rust via Git Bash
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", "env | grep -i rust")
	out, err := cmd.CombinedOutput()
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		t.Fatal("Git Bash pipe command timed out - FOUND THE HANG")
	}
	t.Logf("output len: %d, err: %v", len(out), err)
}

func TestGitBashHangDoublePipe(t *testing.T) {
	// Run two pipe commands concurrently via separate bash processes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd1 := exec.CommandContext(ctx, "bash", "-c", "env | grep -i rust")
	cmd2 := exec.CommandContext(ctx, "bash", "-c", "ls -la ~/.cargo/ 2>/dev/null || echo no")

	var done1, done2 chan struct{}
	done1 = make(chan struct{})
	done2 = make(chan struct{})

	go func() {
		out1, _ := cmd1.CombinedOutput()
		t.Logf("cmd1 output len: %d", len(out1))
		close(done1)
	}()
	go func() {
		out2, _ := cmd2.CombinedOutput()
		t.Logf("cmd2 output len: %d", len(out2))
		close(done2)
	}()

	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("cmd1 timed out - FOUND THE HANG")
	}
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("cmd2 timed out - FOUND THE HANG")
	}
}

func TestGitBashOrOperator(t *testing.T) {
	// Test || operator specifically
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", "ls -la ~/.cargo/ 2>/dev/null || echo no")
	out, err := cmd.CombinedOutput()
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		t.Fatal("Git Bash || command timed out - FOUND THE HANG")
	}
	t.Logf("output len: %d, err: %v", len(out), err)
}
