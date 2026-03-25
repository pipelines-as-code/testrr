package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTRXParserParse(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "trx-sample.trx"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewTRXParser()
	if !p.Detect("report.trx", contents) {
		t.Fatal("expected parser to detect trx by extension")
	}
	if !p.Detect("report.xml", contents) {
		t.Fatal("expected parser to detect trx xml by root element")
	}

	parsed, err := p.Parse(context.Background(), UploadFile{Name: "report.trx", Contents: contents})
	if err != nil {
		t.Fatalf("parse trx: %v", err)
	}

	if parsed.Format != "trx" {
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
	if parsed.TestResults[1].FailureMessage != "Expected 1 but was 2" {
		t.Fatalf("unexpected failure message: %s", parsed.TestResults[1].FailureMessage)
	}
	if parsed.TestResults[2].Status != "skipped" {
		t.Fatalf("expected third test to be skipped, got %s", parsed.TestResults[2].Status)
	}
	if parsed.TestResults[0].DurationMillis != 123 {
		t.Fatalf("expected 123ms duration, got %d", parsed.TestResults[0].DurationMillis)
	}
}
