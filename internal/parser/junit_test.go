package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestJUnitParserParse(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "junit-mixed.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	parser := NewJUnitParser()
	if !parser.Detect("report.xml", contents) {
		t.Fatal("expected parser to detect junit xml")
	}

	parsed, err := parser.Parse(context.Background(), UploadFile{Name: "report.xml", Contents: contents})
	if err != nil {
		t.Fatalf("parse junit: %v", err)
	}

	if parsed.Format != "junit" {
		t.Fatalf("unexpected format: %s", parsed.Format)
	}
	if len(parsed.TestResults) != 3 {
		t.Fatalf("expected 3 test results, got %d", len(parsed.TestResults))
	}
	if parsed.TestResults[1].Status != "failed" {
		t.Fatalf("expected second test to fail, got %s", parsed.TestResults[1].Status)
	}
	if parsed.TestResults[1].FailureMessage != "boom" {
		t.Fatalf("unexpected failure message: %s", parsed.TestResults[1].FailureMessage)
	}
	if parsed.TestResults[2].Status != "skipped" {
		t.Fatalf("expected skipped test, got %s", parsed.TestResults[2].Status)
	}
}

func TestJUnitParserDoesNotDuplicateSuiteOutputPerTestCase(t *testing.T) {
	t.Parallel()

	contents := []byte(`
<testsuite name="suite">
  <system-out>suite stdout</system-out>
  <system-err>suite stderr</system-err>
  <testcase classname="suite" name="TestOne" time="0.1"></testcase>
  <testcase classname="suite" name="TestTwo" time="0.2">
    <system-out>case stdout</system-out>
  </testcase>
</testsuite>`)

	parser := NewJUnitParser()
	parsed, err := parser.Parse(context.Background(), UploadFile{Name: "report.xml", Contents: contents})
	if err != nil {
		t.Fatalf("parse junit: %v", err)
	}
	if len(parsed.TestResults) != 2 {
		t.Fatalf("expected 2 test results, got %d", len(parsed.TestResults))
	}
	if parsed.TestResults[0].SystemOut != "" || parsed.TestResults[0].SystemErr != "" {
		t.Fatalf("expected suite output to stay off testcase rows, got %+v", parsed.TestResults[0])
	}
	if parsed.TestResults[1].SystemOut != "case stdout" {
		t.Fatalf("expected testcase output to remain, got %+v", parsed.TestResults[1])
	}
}
