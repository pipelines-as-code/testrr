package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type TRXParser struct{}

func NewTRXParser() TRXParser {
	return TRXParser{}
}

func (TRXParser) Name() string {
	return "trx"
}

func (TRXParser) Detect(name string, contents []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".trx" {
		return true
	}
	if ext != ".xml" {
		return false
	}
	decoder := xml.NewDecoder(bytes.NewReader(contents))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if el, ok := token.(xml.StartElement); ok {
			return el.Name.Local == "TestRun"
		}
	}
}

func (TRXParser) Parse(_ context.Context, file UploadFile) (ParsedFile, error) {
	// Strip the default namespace so encoding/xml matches element names without namespace.
	cleaned := bytes.ReplaceAll(file.Contents,
		[]byte(`xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010"`),
		[]byte(""))

	var run trxRun
	if err := xml.Unmarshal(cleaned, &run); err != nil {
		return ParsedFile{}, fmt.Errorf("parse trx %s: %w", file.Name, err)
	}

	classNames := make(map[string]string, len(run.TestDefinitions.UnitTests))
	for _, ut := range run.TestDefinitions.UnitTests {
		classNames[ut.ID] = ut.TestMethod.ClassName
	}

	results := make([]ParsedTestResult, 0, len(run.Results.UnitTestResults))
	for _, r := range run.Results.UnitTestResults {
		className := classNames[r.TestID]
		packageName := trxPackageName(className)
		results = append(results, ParsedTestResult{
			SuiteName:      className,
			PackageName:    packageName,
			ClassName:      className,
			TestName:       r.TestName,
			TestKey:        deriveTestKey(packageName, className, r.TestName),
			Status:         trxOutcome(r.Outcome),
			DurationMillis: parseTRXDuration(r.Duration),
			FailureMessage: strings.TrimSpace(r.Output.ErrorInfo.Message),
			FailureOutput:  strings.TrimSpace(r.Output.ErrorInfo.StackTrace),
			SystemOut:      strings.TrimSpace(r.Output.StdOut),
		})
	}

	return ParsedFile{Format: "trx", TestResults: results}, nil
}

func trxPackageName(className string) string {
	if i := strings.LastIndex(className, "."); i >= 0 {
		return className[:i]
	}
	return className
}

func trxOutcome(outcome string) string {
	switch outcome {
	case "Passed":
		return "passed"
	case "Failed":
		return "failed"
	default:
		return "skipped"
	}
}

func parseTRXDuration(raw string) int64 {
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) != 3 {
		return 0
	}
	hours, _ := strconv.ParseFloat(parts[0], 64)
	mins, _ := strconv.ParseFloat(parts[1], 64)
	secs, _ := strconv.ParseFloat(parts[2], 64)
	return int64((hours*3600+mins*60+secs) * 1000)
}

type trxRun struct {
	XMLName         xml.Name   `xml:"TestRun"`
	TestDefinitions trxDefs    `xml:"TestDefinitions"`
	Results         trxResults `xml:"Results"`
}

type trxDefs struct {
	UnitTests []trxUnitTest `xml:"UnitTest"`
}

type trxUnitTest struct {
	ID         string        `xml:"id,attr"`
	Name       string        `xml:"name,attr"`
	TestMethod trxTestMethod `xml:"TestMethod"`
}

type trxTestMethod struct {
	ClassName string `xml:"className,attr"`
	Name      string `xml:"name,attr"`
}

type trxResults struct {
	UnitTestResults []trxUnitTestResult `xml:"UnitTestResult"`
}

type trxUnitTestResult struct {
	TestID   string    `xml:"testId,attr"`
	TestName string    `xml:"testName,attr"`
	Outcome  string    `xml:"outcome,attr"`
	Duration string    `xml:"duration,attr"`
	Output   trxOutput `xml:"Output"`
}

type trxOutput struct {
	StdOut    string       `xml:"StdOut"`
	ErrorInfo trxErrorInfo `xml:"ErrorInfo"`
}

type trxErrorInfo struct {
	Message    string `xml:"Message"`
	StackTrace string `xml:"StackTrace"`
}
