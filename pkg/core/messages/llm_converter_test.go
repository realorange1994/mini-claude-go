package messages

import (
	"strings"
	"testing"
)

func TestBashExecutionText(t *testing.T) {
	got := BashExecutionText("ls -la", "file1\nfile2", 0, false, false, "")
	if !strings.Contains(got, "Ran `ls -la`") {
		t.Error("missing command")
	}
	if !strings.Contains(got, "file1") {
		t.Error("missing output")
	}
}

func TestBashExecutionText_ExitCode(t *testing.T) {
	got := BashExecutionText("false", "", 1, false, false, "")
	if !strings.Contains(got, "Command exited with code 1") {
		t.Error("missing exit code")
	}
}

func TestBashExecutionText_Cancelled(t *testing.T) {
	got := BashExecutionText("sleep 10", "", 0, true, false, "")
	if !strings.Contains(got, "(command cancelled)") {
		t.Error("missing cancelled")
	}
}

func TestBashExecutionText_Truncated(t *testing.T) {
	got := BashExecutionText("big", "out", 0, false, true, "/tmp/full.txt")
	if !strings.Contains(got, "Output truncated") {
		t.Error("missing truncation notice")
	}
	if !strings.Contains(got, "/tmp/full.txt") {
		t.Error("missing full output path")
	}
}

func TestCompactionSummaryPrefix(t *testing.T) {
	if !strings.Contains(CompactionSummaryPrefix, "compacted") {
		t.Error("prefix should mention compacted")
	}
	if !strings.Contains(CompactionSummarySuffix, "</summary>") {
		t.Error("suffix should close summary tag")
	}
}

func TestBranchSummaryPrefix(t *testing.T) {
	if !strings.Contains(BranchSummaryPrefix, "branch") {
		t.Error("prefix should mention branch")
	}
}
