# Instant API Architect

Instant API Architect is an autonomous agent that dynamically generates, tests, and serves a full-fledged Go REST API directly from any CSV file. By leveraging a "Code as Action" approach, the agent doesn't just output static code; it actively tests the generated Go server code inside a sandbox. If the compilation or automated tests fail, the agent captures the error logs and feeds them back into the LLM (Gemini) in a retry loop to self-correct and fix the code autonomously until it passes.

## How to Build and Run

1. Ensure you have Go installed.
2. Set up your `.env` file with your Gemini API credentials (you can refer to `.env.example`).
3. Run the agent by providing a CSV file:

```bash
go run ./cmd/agent/main.go <path/to/data.csv>
```

Alternatively, you can run `go run ./cmd/agent/main.go` without arguments and the agent will prompt you for the CSV file path. At startup, you will also be prompted to choose whether to generate and run tests during the code generation loop.

## Design Choices

The architecture separates schema inference (Phase 1) from iterative code execution (Phase 2) to maintain structured data models before attempting code generation. By feeding raw compiler and test execution failures directly back to the LLM as actionable feedback, the agent significantly minimizes hallucinations and guarantees a working API.
