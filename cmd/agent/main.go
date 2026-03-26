// cmd/agent/main.go – Instant API Architect
//
// Usage:
//
//	go run ./cmd/agent/main.go <path/to/data.csv>
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/joho/godotenv"

	"instant-api-agent/internal/executor"
	"instant-api-agent/internal/llm"
	"instant-api-agent/internal/parser"
	"instant-api-agent/internal/schema"
)

const (
	maxRetries = 3
	sandboxDir = "sandbox"
	sandboxMod = "sandbox"
)

func main() {
	// ── 0. Load environment ──────────────────────────────────────────────
	if err := godotenv.Load(); err != nil {
		log.Println("⚠  .env not found – using existing environment variables")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── 1. Resolve CSV path ──────────────────────────────────────────────
	csvPath, err := resolveCSVPath()
	if err != nil {
		log.Fatalf("❌ %v", err)
	}
	absCSV, err := filepath.Abs(csvPath)
	if err != nil {
		log.Fatalf("❌ abs path: %v", err)
	}

	printBanner("Instant API Architect")

	// ── 2. Parse CSV → DataProfile ───────────────────────────────────────
	fmt.Println("📂 Parsing CSV…")
	dp, err := parser.ParseCSV(absCSV)
	if err != nil {
		log.Fatalf("❌ %v", err)
	}
	fmt.Printf("   ✓ %d rows, %d columns detected\n\n", dp.RowCount, len(dp.Headers))

	dpJSON, err := dp.ToJSON()
	if err != nil {
		log.Fatalf("❌ %v", err)
	}

	// ── 3. Initialise LLM client ─────────────────────────────────────────
	fmt.Println("🤖 Connecting to Gemini…")
	client, err := llm.NewClient(ctx)
	if err != nil {
		log.Fatalf("❌ %v", err)
	}
	defer client.Close()
	fmt.Println("   ✓ Connected")

	// ═════════════════════════════════════════════════════════════════════
	// PHASE 1 – Schema Analysis
	// ═════════════════════════════════════════════════════════════════════
	printSection("Phase 1 – Schema Analysis")
	fmt.Println("📊 Analysing CSV schema with Gemini…")

	rawSchema, err := client.AnalyzeSchema(ctx, dpJSON)
	if err != nil {
		log.Fatalf("❌ Schema analysis failed: %v", err)
	}

	sd, err := schema.ParseSchemaFromLLM(rawSchema)
	if err != nil {
		log.Fatalf("❌ Could not parse schema response: %v\n\nRaw LLM response:\n%s", err, rawSchema)
	}

	sdJSON, err := sd.ToJSON()
	if err != nil {
		log.Fatalf("❌ %v", err)
	}

	fmt.Printf("\n   Resource : %s\n", sd.ResourceName)
	fmt.Printf("   Endpoint : %s\n", sd.EndpointPath)
	fmt.Println("   Columns  :")
	for _, col := range sd.Columns {
		idMarker := ""
		if col.IsIdentifier {
			idMarker = " (Identifier)"
		}
		fmt.Printf("     • %-22s  type=%-12s  validation=%-10s%s\n",
			col.Name, col.GoType, col.Validation, idMarker)
		if col.Description != "" {
			fmt.Printf("       %s\n", col.Description)
		}
	}
	fmt.Println()

	// ═════════════════════════════════════════════════════════════════════
	// PHASE 2 – CodeAct Loop
	// ═════════════════════════════════════════════════════════════════════
	printSection("Phase 2 – Code Generation & Test Loop")

	// Ensure the sandbox has a go.mod.
	if err := executor.EnsureSandboxModule(ctx, sandboxDir, sandboxMod); err != nil {
		log.Fatalf("❌ sandbox init: %v", err)
	}

	var (
		serverURL   string
		lastFailure string
	)

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("🔄 Attempt %d/%d – generating Go code…\n", attempt, maxRetries)

		var prompt string
		if attempt == 1 {
			prompt = buildCodeGenPrompt(sdJSON, absCSV, sd.ResourceName)
		} else {
			prompt = buildRetryPrompt(sdJSON, lastFailure)
		}

		rawCode, err := client.GenerateCode(ctx, prompt)
		if err != nil {
			log.Fatalf("❌ LLM error: %v", err)
		}

		serverGo, ok1 := extractCodeBlock(rawCode, "server.go")
		serverTestGo, ok2 := extractCodeBlock(rawCode, "server_test.go")
		if !ok1 || !ok2 {
			lastFailure = "The response did not contain the required ```go server.go and " +
				"```go server_test.go code blocks. You MUST output exactly those two fenced blocks."
			fmt.Printf("   ⚠  Missing code blocks in LLM response (attempt %d)\n", attempt)
			continue
		}

		if err := executor.WriteFiles(sandboxDir, map[string]string{
			"server.go":      serverGo,
			"server_test.go": serverTestGo,
		}); err != nil {
			log.Fatalf("❌ Write sandbox files: %v", err)
		}

		fmt.Println("🧪 Running go test ./…")
		res, err := executor.RunCommand(ctx, sandboxDir, "go", "test", "-v", "-count=1", "./...")
		if err != nil {
			log.Fatalf("❌ go test exec error: %v", err)
		}

		if res.Success {
			fmt.Println("\n✅ All tests passed!")
			serverURL, err = streamServerUntilReady(ctx, sandboxDir, absCSV)
			if err != nil {
				log.Fatalf("❌ Start server: %v", err)
			}
			break
		}

		lastFailure = res.Output
		fmt.Printf("\n   ⚠  Tests failed (attempt %d/%d). Sending errors to Gemini…\n\n", attempt, maxRetries)

		if attempt == maxRetries {
			fmt.Println("❌ Exhausted all retries. See test output above.")
			os.Exit(1)
		}
	}

	if serverURL != "" {
		printSection("Server Ready")
		fmt.Printf("🌐  %s\n\n", serverURL)
		fmt.Printf("Try it:\n  curl %s%s\n  curl %s%s/123\n  curl \"%s%s?sort=<col>&filter=<col>:<val>\"\n\n",
			serverURL, sd.EndpointPath, serverURL, sd.EndpointPath, serverURL, sd.EndpointPath)
		fmt.Println("Press Ctrl+C to stop.")
		<-ctx.Done()
		fmt.Println("\n👋 Shutting down.")
	}
}

// ── streamServerUntilReady ────────────────────────────────────────────────────
// Starts `go run . <csvPath>` in dir and reads stdout line-by-line until the
// LISTENING_ON sentinel appears.  The child process keeps running after this
// function returns (it's managed by the caller's context cancel on Ctrl+C).
func streamServerUntilReady(ctx context.Context, dir, csvPath string) (string, error) {
	fmt.Println("\n🚀 Starting generated API server…")

	cmd := exec.CommandContext(ctx, "go", "run", ".", csvPath)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("go run: %w", err)
	}

	// Read lines; the goroutine below keeps draining after we return.
	lineCh := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lineCh <- sc.Text()
		}
		close(lineCh)
	}()

	for line := range lineCh {
		fmt.Println("  [server]", line)
		if url := executor.ExtractURL(line); url != "" {
			// Keep draining stdout in the background so the pipe doesn't block.
			go func() {
				for range lineCh {
				}
			}()
			return url, nil
		}
	}

	// Channel closed without sentinel – process exited early.
	_ = cmd.Wait()
	return "", fmt.Errorf("server process exited before emitting LISTENING_ON sentinel")
}

// ── Prompt builders ───────────────────────────────────────────────────────────

func buildCodeGenPrompt(schemaJSON, csvPath, resourceName string) string {
	return fmt.Sprintf(`Generate a complete Go API server and its test file for the following dataset.

Resource name  : %s
CSV file path  : %s  (the server must accept this as os.Args[1])

SchemaDefinition JSON – use this EXACTLY to define your Go struct and handlers:
%s

Follow all rules in your system prompt strictly.
Output ONLY the two fenced code blocks (server.go then server_test.go).`, resourceName, csvPath, schemaJSON)
}

func buildRetryPrompt(schemaJSON, failureOutput string) string {
	const maxLen = 4000
	out := failureOutput
	if len(out) > maxLen {
		out = out[:maxLen] + "\n...(truncated)"
	}
	return fmt.Sprintf(`The previous code failed. Analyse the errors and regenerate BOTH files.

--- FAILURE OUTPUT ---
%s
--- END ---

The SchemaDefinition is unchanged:
%s

Fix ALL errors. Output ONLY the two fenced code blocks.`, out, schemaJSON)
}

// ── Code block extraction ─────────────────────────────────────────────────────

// extractCodeBlock finds a ```go <filename>\n...\n``` fence in raw and returns
// its content trimmed of surrounding whitespace.
func extractCodeBlock(raw, filename string) (string, bool) {
	re := regexp.MustCompile("(?i)```go[ \t]+" + regexp.QuoteMeta(filename) + `[ \t]*\r?\n`)
	loc := re.FindStringIndex(raw)
	if loc == nil {
		return "", false
	}
	rest := raw[loc[1]:]
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:end]), true
}

// ── Display helpers ───────────────────────────────────────────────────────────

func printBanner(title string) {
	line := strings.Repeat("═", len(title)+4)
	fmt.Printf("\n╔%s╗\n║  %s  ║\n╚%s╝\n\n", line, title, line)
}

func printSection(title string) {
	fmt.Printf("\n── %s %s\n", title, strings.Repeat("─", max(0, 50-len(title))))
}

func resolveCSVPath() (string, error) {
	if len(os.Args) > 1 {
		return os.Args[1], nil
	}
	fmt.Print("Enter path to CSV file: ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return "", fmt.Errorf("no CSV path provided")
	}
	p := strings.TrimSpace(sc.Text())
	if p == "" {
		return "", fmt.Errorf("CSV path cannot be empty")
	}
	return p, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
