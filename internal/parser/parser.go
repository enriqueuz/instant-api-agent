// Package parser reads a CSV file and extracts a DataProfile used to inform
// the LLM during Schema Analysis (Phase 1).
package parser

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// maxSampleRows is the number of data rows to include in the profile.
const maxSampleRows = 5

// DataProfile is the structured representation of a CSV file sent to the LLM
// during the Schema Analysis phase.
type DataProfile struct {
	FilePath   string     `json:"filePath"`
	Headers    []string   `json:"headers"`
	SampleRows [][]string `json:"sampleRows"`
	RowCount   int        `json:"rowCount"` // total data rows (excluding header)
}

// ParseCSV reads a CSV file from disk and returns a DataProfile.
// It reads all rows to produce an accurate total RowCount but only keeps the
// first maxSampleRows for the profile.
func ParseCSV(path string) (*DataProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("parser: open %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true

	// Read header row.
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("parser: read headers: %w", err)
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("parser: CSV has no headers")
	}

	// Trim whitespace from header names.
	for i, h := range headers {
		headers[i] = strings.TrimSpace(h)
	}

	var (
		sampleRows [][]string
		rowCount   int
	)

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parser: read row %d: %w", rowCount+1, err)
		}
		rowCount++
		if len(sampleRows) < maxSampleRows {
			// Trim each cell value.
			trimmed := make([]string, len(row))
			for i, v := range row {
				trimmed[i] = strings.TrimSpace(v)
			}
			sampleRows = append(sampleRows, trimmed)
		}
	}

	if rowCount == 0 {
		return nil, fmt.Errorf("parser: CSV has no data rows")
	}

	return &DataProfile{
		FilePath:   path,
		Headers:    headers,
		SampleRows: sampleRows,
		RowCount:   rowCount,
	}, nil
}

// ToJSON returns a pretty-printed JSON representation of the DataProfile.
func (dp *DataProfile) ToJSON() (string, error) {
	b, err := json.MarshalIndent(dp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("parser: marshal DataProfile: %w", err)
	}
	return string(b), nil
}
