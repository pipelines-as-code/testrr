package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGoTestJSONParserParse(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "go-test-sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	parser := NewGoTestJSONParser()
	if !parser.Detect("go-test.json", contents) {
		t.Fatal("expected parser to detect go test json")
	}

	parsed, err := parser.Parse(context.Background(), UploadFile{Name: "go-test.json", Contents: contents})
	if err != nil {
		t.Fatalf("parse go test json: %v", err)
	}

	if parsed.Format != "gotestjson" {
		t.Fatalf("unexpected format: %s", parsed.Format)
	}
	if len(parsed.TestResults) != 3 {
		t.Fatalf("expected 3 test results, got %d", len(parsed.TestResults))
	}
	if parsed.TestResults[0].Status != "passed" {
		t.Fatalf("expected first test to pass, got %s", parsed.TestResults[0].Status)
	}
	if parsed.TestResults[1].Status != "failed" {
		t.Fatalf("expected second test to fail, got %s", parsed.TestResults[1].Status)
	}
	if parsed.TestResults[1].FailureMessage != "go test failed" {
		t.Fatalf("unexpected failure message: %s", parsed.TestResults[1].FailureMessage)
	}
	if parsed.TestResults[2].TestName != "package" || parsed.TestResults[2].Status != "failed" {
		t.Fatalf("expected package failure result, got %+v", parsed.TestResults[2])
	}
}
