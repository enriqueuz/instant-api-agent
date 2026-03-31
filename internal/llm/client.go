// Package llm wraps the Gemini generative AI SDK, providing two distinct chat
// sessions:
//
//  1. Schema Analysis session – a stateless, one-shot session used in Phase 1
//     to turn a DataProfile into a SchemaDefinition.
//
//  2. Code Generation session – a stateful session (history preserved) used in
//     Phase 2's CodeAct loop so the model remembers previous attempts on retry.
package llm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// ---- System prompts ---------------------------------------------------------

const schemaSystemPrompt = `You are a data analyst. Given CSV headers and sample rows, return ONLY a JSON code block matching this exact schema (no prose, no explanation):

{
  "resourceName": "<singular PascalCase name for the data entity, e.g., User>",
  "endpointPath": "<plural lowercase endpoint path based on the entity, e.g., /users>",
  "columns": [
    {
      "name":        "<original column header>",
      "goType":      "<Go type: string | int64 | float64 | decimal | bool | time.Time>",
      "isIdentifier": <true | false>,
      "validation":  "<none | email | url | positive | non-empty>",
      "description": "<one-sentence semantic description>"
    }
  ]
}

Rules:
- Choose goType=decimal for any monetary, price, amount, cost, or total column.
- Choose goType=time.Time for any date or datetime column.
- Choose goType=int64 for whole-number identifiers or counts.
- Set isIdentifier=true for exactly ONE column that best serves as the unique ID for the row (prefer columns named "id", "uuid", "code", "email", or similar). All other columns must be false.
- Choose validation=email for any column whose name contains "email".
- Choose validation=positive for decimal or numeric columns that must be > 0.
- Choose validation=non-empty for required string fields.
- Choose validation=none otherwise.
- Return ONLY the JSON code block wrapped in ` + "```json" + ` fences. No other text.`

func getCodeSystemPrompt(generateTests bool) string {
	base := `You are a Senior Go Developer. You generate only production-quality Go code.

Rules:
1. Use ONLY the Go standard library (net/http, encoding/json, encoding/csv, regexp, sort, strconv, strings, fmt, log, os).
2. Proper error handling: never ignore errors; return HTTP 500 with a JSON body {"error":"<message>"}.
3. Clear naming: exported types and handlers, unexported helpers.
4. The server MUST listen on ":0" (OS-assigned port) and print exactly ONE line to stdout:
       LISTENING_ON=http://localhost:<actual_port>
   Use net.Listener to obtain the actual port before starting http.Serve.
5. Read CSV data from the file path given as os.Args[1] at startup. Do NOT embed data. Load it into a slice of structs in memory.
6. Use the SchemaDefinition JSON provided to build the Go struct and endpoints:
   - The base endpoint path MUST be the SchemaDefinition's "endpointPath" (e.g., /users).
   - The unique identifier field is the one marked with "isIdentifier": true.
   - goType=decimal  → parse with strconv.ParseFloat(val, 64), store as float64, marshal with 2 decimal places via fmt.Sprintf("%.2f", v)
   - goType=time.Time → parse with time.Parse("2006-01-02", val) falling back to time.RFC3339
   - goType=int64    → parse with strconv.ParseInt(val, 10, 64)
   - goType=bool     → parse with strconv.ParseBool(val)
   - goType=string   → use as-is
   - validation=email → validate with regexp.MustCompile(` + "`^[^@]+@[^@]+\\.[^@]+$`" + `)
   - validation=positive → return HTTP 400 if value ≤ 0
   - validation=non-empty → return HTTP 400 if value is empty string
7. Required endpoints:
   GET <endpointPath>      → return JSON array of rows. MUST support optional query params:
                             - Pagination: ?page=<page_number>&limit=<items_per_page>
                             - Sorting: ?sort=<columnName>&order=<asc|desc> (default asc)
                             - Filtering: Exact match ONLY using ?<columnName>=<value> (case-insensitive for strings). 
                               CRITICAL: Do NOT use the "reflect" package for filtering or sorting. Implement filtering explicitly/statically for each column defined in the SchemaDefinition. Convert the query string to the correct Go type (strconv, time.Parse, etc.) before comparing. 
                               Simply ignore any unrecognized query parameters (no strict validation).
   GET <endpointPath>/{id} → return single row by matching the {id} value from the URL against the struct field marked as the identifier. 
                             Parse {id} from the URL path manually using strings.TrimPrefix (do not use external routers).
                             Return HTTP 404 if no record matches the given ID.
8. NEVER omit code for brevity. You MUST implement the full filtering, sorting, and validation logic for ALL columns.`

	if generateTests {
		return base + `
9. Write comprehensive tests in server_test.go covering:
   - GET <endpointPath> returns all rows (HTTP 200)
   - GET <endpointPath> with pagination, sorting, and filtering returns correct rows
   - GET <endpointPath>/{id} returns correct row for an existing ID (HTTP 200)
   - GET <endpointPath>/{id} with a non-existent ID returns HTTP 404
   Use httptest.NewRecorder and httptest.NewServer as appropriate.
10. Output EXACTLY two fenced code blocks in this order:
   ` + "```go server.go" + `
   <server code here>
   ` + "```" + `
   ` + "```go server_test.go" + `
   <test code here>
   ` + "```" + `
   No prose, no explanation outside the code blocks.`
	}

	return base + `
9. Output EXACTLY one fenced code block:
   ` + "```go server.go" + `
   <server code here>
   ` + "```" + `
   No prose, no explanation outside the code blocks.`
}

const docSystemPrompt = `You are an expert Technical Writer. Given a JSON Schema and a final, working Go REST API code (server.go), generate a comprehensive usage guide (the usage.md thats currently being generated in GenerateCode).

Rules:
- Provide comprehensive curl examples showcasing ALL features generated, including:
  - Pagination (limit and page)
  - Sorting (asc and desc)
  - Filtering for EVERY column (show exact matches, and for numeric/dates show range queries min_XX/max_XX).
- Make sure to use the actual endpoints and concrete values from the dataset.
- Use $SERVER_URL as a placeholder for the base URL.
- Output ONLY ONE fenced markdown block: ` + "```markdown usage.md" + `
- CRITICAL rule: DO NOT use triple backticks (` + "```" + `) anywhere INSIDE the usage.md text, as it will break our markdown parser. Use single backticks or 4-space indentation for code snippets instead.
- No prose, no explanation outside the code block.`

// ---- Client -----------------------------------------------------------------

// Client wraps the Gemini SDK with three independent chat sessions.
type Client struct {
	genaiClient   *genai.Client
	schemaSession *genai.ChatSession
	codeSession   *genai.ChatSession
	docSession    *genai.ChatSession
}

// NewClient creates a Client, loading GEMINI_API_KEY and GEMINI_MODEL from the
// environment.  Call this after loading .env with godotenv.
func NewClient(ctx context.Context, generateTests bool) (*Client, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("llm: GEMINI_API_KEY is not set")
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		return nil, fmt.Errorf("llm: GEMINI_MODEL is not set")
	}

	gc, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("llm: create genai client: %w", err)
	}

	// Schema session – low temperature for deterministic JSON output.
	schemaModel := gc.GenerativeModel(model)
	schemaModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(schemaSystemPrompt)},
	}
	schemaModel.SetTemperature(0.1)

	// Code session – slightly higher temperature for more creative code.
	codeModel := gc.GenerativeModel(model)
	codeModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(getCodeSystemPrompt(generateTests))},
	}
	codeModel.SetTemperature(0.2)

	// Doc session – low temperature for deterministic markdown output.
	docModel := gc.GenerativeModel(model)
	docModel.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(docSystemPrompt)},
	}
	docModel.SetTemperature(0.1)

	return &Client{
		genaiClient:   gc,
		schemaSession: schemaModel.StartChat(),
		codeSession:   codeModel.StartChat(),
		docSession:    docModel.StartChat(),
	}, nil
}

// Close releases resources held by the underlying Gemini client.
func (c *Client) Close() error {
	return c.genaiClient.Close()
}

// AnalyzeSchema sends a DataProfile JSON to the Schema Analysis session and
// returns the raw LLM text (a ```json fenced SchemaDefinition).
func (c *Client) AnalyzeSchema(ctx context.Context, dataProfileJSON string) (string, error) {
	prompt := "Here is the CSV data profile. Analyse it and return the SchemaDefinition JSON:\n\n" + dataProfileJSON
	return c.sendMessage(ctx, c.schemaSession, prompt)
}

// GenerateCode sends a prompt to the Code Generation session (history
// preserved) and returns the raw LLM text containing server.go and
// server_test.go code blocks.
func (c *Client) GenerateCode(ctx context.Context, prompt string) (string, error) {
	return c.sendMessage(ctx, c.codeSession, prompt)
}

// GenerateDocs sends a prompt to the Document Generation session and returns
// the raw LLM text containing the usage.md code block.
func (c *Client) GenerateDocs(ctx context.Context, schemaJSON string, finalServerCode string) (string, error) {
	prompt := fmt.Sprintf("Schema:\n%s\n\nFinal Server Code:\n%s", schemaJSON, finalServerCode)
	return c.sendMessage(ctx, c.docSession, prompt)
}

// sendMessage sends a single user message to a ChatSession and returns the
// full text of the first response candidate.
func (c *Client) sendMessage(ctx context.Context, session *genai.ChatSession, prompt string) (string, error) {
	resp, err := session.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("llm: send message: %w", err)
	}
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("llm: empty response from model")
	}
	return extractText(resp.Candidates[0]), nil
}

// extractText concatenates all text Parts from a Candidate into a single string.
func extractText(c *genai.Candidate) string {
	if c.Content == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range c.Content.Parts {
		if t, ok := part.(genai.Text); ok {
			sb.WriteString(string(t))
		}
	}
	return sb.String()
}
