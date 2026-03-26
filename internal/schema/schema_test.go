package schema

import (
	"strings"
	"testing"
)

const validJSON = `{
  "resourceName": "Product",
  "columns": [
    {"name": "id",    "goType": "int64",   "validation": "none",     "description": "Unique identifier"},
    {"name": "email", "goType": "string",  "validation": "email",    "description": "User email"},
    {"name": "price", "goType": "decimal", "validation": "positive", "description": "Item price"}
  ]
}`

func TestParseSchemaFromLLM_PlainJSON(t *testing.T) {
	sd, err := ParseSchemaFromLLM(validJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sd.ResourceName != "Product" {
		t.Errorf("resourceName: got %q, want %q", sd.ResourceName, "Product")
	}
	if len(sd.Columns) != 3 {
		t.Errorf("columns: got %d, want 3", len(sd.Columns))
	}
}

func TestParseSchemaFromLLM_FencedBlock(t *testing.T) {
	fenced := "Sure! Here you go:\n```json\n" + validJSON + "\n```\n"
	sd, err := ParseSchemaFromLLM(fenced)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sd.ResourceName != "Product" {
		t.Errorf("resourceName: got %q, want %q", sd.ResourceName, "Product")
	}
}

func TestParseSchemaFromLLM_InvalidJSON(t *testing.T) {
	_, err := ParseSchemaFromLLM("not json at all")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseSchemaFromLLM_MissingResourceName(t *testing.T) {
	bad := `{"columns":[{"name":"x","goType":"string","validation":"none","description":"x"}]}`
	_, err := ParseSchemaFromLLM(bad)
	if err == nil {
		t.Fatal("expected error for missing resourceName")
	}
}

func TestParseSchemaFromLLM_NoColumns(t *testing.T) {
	bad := `{"resourceName":"Foo","columns":[]}`
	_, err := ParseSchemaFromLLM(bad)
	if err == nil {
		t.Fatal("expected error for empty columns")
	}
}

func TestToJSON(t *testing.T) {
	sd, _ := ParseSchemaFromLLM(validJSON)
	j, err := sd.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if !strings.Contains(j, "Product") {
		t.Error("JSON output missing resourceName")
	}
}

func TestStripFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		lang  string
		want  string
	}{
		{
			name:  "json fence",
			input: "```json\n{\"a\":1}\n```",
			lang:  "json",
			want:  `{"a":1}`,
		},
		{
			name:  "bare fence",
			input: "```\nhello\n```",
			lang:  "json",
			want:  "hello",
		},
		{
			name:  "no fence",
			input: "raw text",
			lang:  "json",
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripFences(tc.input, tc.lang)
			if got != tc.want {
				t.Errorf("stripFences: got %q, want %q", got, tc.want)
			}
		})
	}
}
