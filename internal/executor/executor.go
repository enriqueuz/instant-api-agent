// Package executor handles file I/O to the sandbox, running shell commands,
// streaming their output, and graceful process termination.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WriteFiles writes a map of filename → content into dir, creating any
// necessary parent directories.
func WriteFiles(dir string, files map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("executor: mkdir %q: %w", dir, err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("executor: write %q: %w", path, err)
		}
	}
	return nil
}

// RunResult holds the combined output and exit status of a command.
type RunResult struct {
	Output   string // combined stdout + stderr
	ExitCode int
	Success  bool
}

// RunCommand executes name with args inside dir.  It streams output to both a
// live writer (for the user to see progress) and an internal buffer (returned
// in RunResult).  The command is cancelled when ctx is done; a SIGKILL is
// issued after a 3-second grace period if the process does not exit cleanly.
func RunCommand(ctx context.Context, dir, name string, args ...string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var buf bytes.Buffer
	// Tee to both the capture buffer and os.Stdout so the user sees live output.
	mw := io.MultiWriter(&buf, os.Stdout)
	cmd.Stdout = mw
	cmd.Stderr = mw

	if err := cmd.Start(); err != nil {
		return RunResult{}, fmt.Errorf("executor: start %q: %w", name, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-done:
		// Command finished normally.
	case <-ctx.Done():
		// Context cancelled – attempt graceful termination.
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
			// Process exited after interrupt.
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		waitErr = ctx.Err()
	}

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Context error or other non-exit error.
			return RunResult{Output: buf.String()}, fmt.Errorf("executor: wait %q: %w", name, waitErr)
		}
	}

	return RunResult{
		Output:   buf.String(),
		ExitCode: exitCode,
		Success:  exitCode == 0,
	}, nil
}

// ExtractURL scans the combined command output for the sentinel line emitted
// by the generated server:
//
//	LISTENING_ON=http://localhost:<port>
//
// It returns the URL string, or an empty string if the sentinel is not found.
func ExtractURL(output string) string {
	const prefix = "LISTENING_ON="
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

// EnsureSandboxModule initialises a go.mod in dir if one does not already
// exist.  This allows the generated files to compile as a standalone module.
func EnsureSandboxModule(ctx context.Context, dir, moduleName string) error {
	modPath := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(modPath); err == nil {
		return nil // already exists
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("executor: mkdir sandbox: %w", err)
	}
	res, err := RunCommand(ctx, dir, "go", "mod", "init", moduleName)
	if err != nil {
		return fmt.Errorf("executor: go mod init: %w", err)
	}
	if !res.Success {
		return fmt.Errorf("executor: go mod init failed:\n%s", res.Output)
	}
	return nil
}
