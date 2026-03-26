package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCSV_Normal(t *testing.T) {
	content := "id,name,email,price\n1,Alice,alice@example.com,19.99\n2,Bob,bob@example.com,29.50\n"
	path := writeTempCSV(t, content)

	dp, err := ParseCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := len(dp.Headers), 4; got != want {
		t.Errorf("headers count: got %d, want %d", got, want)
	}
	if got, want := dp.Headers[0], "id"; got != want {
		t.Errorf("first header: got %q, want %q", got, want)
	}
	if got, want := dp.RowCount, 2; got != want {
		t.Errorf("row count: got %d, want %d", got, want)
	}
	if got, want := len(dp.SampleRows), 2; got != want {
		t.Errorf("sample rows: got %d, want %d", got, want)
	}
	if dp.FilePath != path {
		t.Errorf("file path: got %q, want %q", dp.FilePath, path)
	}
}

func TestParseCSV_MoreThanFiveRows(t *testing.T) {
	content := "a,b\n1,x\n2,x\n3,x\n4,x\n5,x\n6,x\n7,x\n"
	path := writeTempCSV(t, content)

	dp, err := ParseCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dp.RowCount != 7 {
		t.Errorf("row count: got %d, want 7", dp.RowCount)
	}
	if len(dp.SampleRows) != maxSampleRows {
		t.Errorf("sample rows capped at %d, got %d", maxSampleRows, len(dp.SampleRows))
	}
}

func TestParseCSV_EmptyFile(t *testing.T) {
	path := writeTempCSV(t, "")
	_, err := ParseCSV(path)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

func TestParseCSV_HeaderOnly(t *testing.T) {
	path := writeTempCSV(t, "id,name\n")
	_, err := ParseCSV(path)
	if err == nil {
		t.Fatal("expected error for header-only CSV, got nil")
	}
}

func TestParseCSV_TrimsWhitespace(t *testing.T) {
	content := " id , name \n 1 , Alice \n"
	path := writeTempCSV(t, content)

	dp, err := ParseCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dp.Headers[0] != "id" {
		t.Errorf("expected trimmed header 'id', got %q", dp.Headers[0])
	}
	if dp.SampleRows[0][1] != "Alice" {
		t.Errorf("expected trimmed cell 'Alice', got %q", dp.SampleRows[0][1])
	}
}

func TestToJSON(t *testing.T) {
	content := "x,y\n1,2\n"
	path := writeTempCSV(t, content)

	dp, _ := ParseCSV(path)
	j, err := dp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if len(j) == 0 {
		t.Error("ToJSON returned empty string")
	}
}

// writeTempCSV creates a temporary CSV file with the given content and
// registers a cleanup to remove it when the test finishes.
func writeTempCSV(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempCSV: %v", err)
	}
	return path
}
