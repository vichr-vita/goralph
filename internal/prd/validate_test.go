package prd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAcceptsValidPRDArray(t *testing.T) {
	data := []byte(`[
		{"category":"cli","description":"validate PRD","steps":["read file","validate fields"],"passes":false},
		{"category":"db","description":"migrate DB","steps":[],"passes":true}
	]`)

	if err := Validate(data); err != nil {
		t.Fatalf("Validate valid PRD: %v", err)
	}
}

func TestValidateRejectsInvalidPRD(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "top-level object", data: `{"category":"cli"}`, want: "array"},
		{name: "unknown field", data: `[{"category":"cli","description":"task","steps":["step"],"passes":false,"extra":1}]`, want: "unknown field"},
		{name: "missing category", data: `[{"description":"task","steps":["step"],"passes":false}]`, want: "category: required"},
		{name: "empty category", data: `[{"category":" ","description":"task","steps":["step"],"passes":false}]`, want: "category: must be non-empty"},
		{name: "missing description", data: `[{"category":"cli","steps":["step"],"passes":false}]`, want: "description: required"},
		{name: "empty description", data: `[{"category":"cli","description":"","steps":["step"],"passes":false}]`, want: "description: must be non-empty"},
		{name: "missing steps", data: `[{"category":"cli","description":"task","passes":false}]`, want: "steps: required"},
		{name: "steps not array", data: `[{"category":"cli","description":"task","steps":"step","passes":false}]`, want: "steps: must be an array"},
		{name: "empty step", data: `[{"category":"cli","description":"task","steps":["step", "  "],"passes":false}]`, want: "steps: item 1 must be non-empty"},
		{name: "missing passes", data: `[{"category":"cli","description":"task","steps":["step"]}]`, want: "passes: required"},
		{name: "passes not bool", data: `[{"category":"cli","description":"task","steps":["step"],"passes":"false"}]`, want: "passes: must be a boolean"},
		{name: "duplicate description", data: `[{"category":"cli","description":"task","steps":["step"],"passes":false},{"category":"cli","description":" task ","steps":["step"],"passes":false}]`, want: "duplicates description"},
		{name: "trailing JSON", data: `[] []`, want: "one top-level JSON array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate([]byte(tt.data))
			if err == nil {
				t.Fatalf("Validate succeeded, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidateFileReadsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prd.json")
	if err := os.WriteFile(path, []byte(`[{"category":"cli","description":"task","steps":["step"],"passes":false}]`), 0o600); err != nil {
		t.Fatalf("write PRD: %v", err)
	}

	if err := ValidateFile(path); err != nil {
		t.Fatalf("ValidateFile: %v", err)
	}
}
