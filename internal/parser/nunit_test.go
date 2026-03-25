package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNUnitParserParse(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "nunit-sample.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewNUnitParser()
	if !p.Detect("report.xml", contents) {
		t.Fatal("expected parser to detect nunit xml")
	}

	parsed, err := p.Parse(context.Background(), UploadFile{Name: "report.xml", Contents: contents})
	if err != nil {
		t.Fatalf("parse nunit: %v", err)
	}

	if parsed.Format != "nunit" {
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
	if parsed.TestResults[1].FailureMessage != "Expected 1 but got 2" {
		t.Fatalf("unexpected failure message: %s", parsed.TestResults[1].FailureMessage)
	}
	if parsed.TestResults[2].Status != "skipped" {
		t.Fatalf("expected third test to be skipped, got %s", parsed.TestResults[2].Status)
	}
}
