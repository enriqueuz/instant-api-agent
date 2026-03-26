package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WriteFiles
// ---------------------------------------------------------------------------

func TestWriteFiles_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.go": "package main\n",
		"README":  "hello\n",
	}
	if err := WriteFiles(dir, files); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("ReadFile %q: %v", name, err)
			continue
		}
		if string(got) != content {
			t.Errorf("content mismatch for %q: got %q, want %q", name, got, content)
		}
	}
}

func TestWriteFiles_CreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "subdir")
	if err := WriteFiles(dir, map[string]string{"x.txt": "x"}); err != nil {
		t.Fatalf("WriteFiles with nested dir: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("nested dir was not created")
	}
}

// ---------------------------------------------------------------------------
// RunCommand
// ---------------------------------------------------------------------------

func TestRunCommand_CapturesStdout(t *testing.T) {
	ctx := context.Background()
	res, err := RunCommand(ctx, t.TempDir(), "echo", "hello world")
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success, got exit code %d", res.ExitCode)
	}
	if !strings.Contains(res.Output, "hello world") {
		t.Errorf("output %q does not contain 'hello world'", res.Output)
	}
}

func TestRunCommand_CapturesStderr(t *testing.T) {
	ctx := context.Background()
	// 'ls' on a non-existent path writes to stderr and exits non-zero.
	res, err := RunCommand(ctx, t.TempDir(), "ls", "/nonexistent_path_abc123")
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if res.Success {
		t.Error("expected failure exit code, got success")
	}
	if len(res.Output) == 0 {
		t.Error("expected stderr in output, got empty string")
	}
}

func TestRunCommand_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	// sleep 10 seconds – should be cancelled well before that.
	_, err := RunCommand(ctx, t.TempDir(), "sleep", "10")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if elapsed > 4*time.Second {
		t.Errorf("command took too long to cancel: %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// ExtractURL
// ---------------------------------------------------------------------------

func TestExtractURL_Found(t *testing.T) {
	output := "some output\nLISTENING_ON=http://localhost:54321\nmore output\n"
	got := ExtractURL(output)
	if got != "http://localhost:54321" {
		t.Errorf("ExtractURL: got %q, want %q", got, "http://localhost:54321")
	}
}

func TestExtractURL_NotFound(t *testing.T) {
	output := "no sentinel here\n"
	if got := ExtractURL(output); got != "" {
		t.Errorf("ExtractURL: expected empty, got %q", got)
	}
}

func TestExtractURL_TrimmedWhitespace(t *testing.T) {
	output := "  LISTENING_ON=http://localhost:9090  \n"
	got := ExtractURL(output)
	if got != "http://localhost:9090" {
		t.Errorf("ExtractURL with whitespace: got %q", got)
	}
}
