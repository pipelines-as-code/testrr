package parser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

type GoTestJSONParser struct{}

func NewGoTestJSONParser() GoTestJSONParser {
	return GoTestJSONParser{}
}

func (GoTestJSONParser) Name() string {
	return "gotestjson"
}

func (GoTestJSONParser) Detect(name string, contents []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != "" && ext != ".json" && ext != ".jsonl" && ext != ".txt" {
		return false
	}

	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Buffer(make([]byte, 0, 1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}

		var event goTestJSONEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Action != "" && event.Package != "" {
			return true
		}
	}
	return false
}

func (GoTestJSONParser) Parse(_ context.Context, file UploadFile) (ParsedFile, error) {
	scanner := bufio.NewScanner(bytes.NewReader(file.Contents))
	scanner.Buffer(make([]byte, 0, 1024), 4*1024*1024)

	suitesByPackage := map[string]*goTestJSONSuite{}
	var order []string

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}

		var event goTestJSONEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Package == "" {
			continue
		}

		suite := suitesByPackage[event.Package]
		if suite == nil {
			suite = &goTestJSONSuite{
				PackageName: event.Package,
				Cases:       map[string]*goTestJSONCase{},
			}
			suitesByPackage[event.Package] = suite
			order = append(order, event.Package)
		}

		if event.Test == "" {
			switch event.Action {
			case "output":
				suite.Output.WriteString(event.Output)
			case "fail":
				suite.PackageErr = true
				suite.Elapsed = event.Elapsed
			case "pass":
				suite.Elapsed = event.Elapsed
			}
			continue
		}

		testCase := suite.Cases[event.Test]
		if testCase == nil {
			testCase = &goTestJSONCase{Name: event.Test}
			suite.Cases[event.Test] = testCase
			suite.CaseOrder = append(suite.CaseOrder, event.Test)
		}

		switch event.Action {
		case "output":
			testCase.Output.WriteString(event.Output)
		case "pass", "fail", "skip":
			testCase.Status = event.Action
			testCase.Elapsed = event.Elapsed
		}
	}
	if err := scanner.Err(); err != nil {
		return ParsedFile{}, fmt.Errorf("scan go test json %s: %w", file.Name, err)
	}

	results := make([]ParsedTestResult, 0)
	for _, packageName := range order {
		suite := suitesByPackage[packageName]
		for _, testName := range suite.CaseOrder {
			testCase := suite.Cases[testName]
			status := "passed"
			switch testCase.Status {
			case "fail":
				status = "failed"
			case "skip":
				status = "skipped"
			}

			output := strings.TrimSpace(testCase.Output.String())
			results = append(results, ParsedTestResult{
				SuiteName:      suite.PackageName,
				PackageName:    suite.PackageName,
				ClassName:      suite.PackageName,
				TestName:       testCase.Name,
				TestKey:        deriveTestKey(suite.PackageName, suite.PackageName, testCase.Name),
				Status:         status,
				DurationMillis: goTestJSONDurationMillis(testCase.Elapsed),
				FailureMessage: goTestJSONFailureMessage(status),
				FailureOutput:  goTestJSONFailureOutput(status, output),
				SystemOut:      output,
			})
		}

		if len(suite.CaseOrder) == 0 && suite.PackageErr {
			output := strings.TrimSpace(suite.Output.String())
			results = append(results, ParsedTestResult{
				SuiteName:      suite.PackageName,
				PackageName:    suite.PackageName,
				ClassName:      suite.PackageName,
				TestName:       "package",
				TestKey:        deriveTestKey(suite.PackageName, suite.PackageName, "package"),
				Status:         "failed",
				DurationMillis: goTestJSONDurationMillis(suite.Elapsed),
				FailureMessage: "package setup failed",
				FailureOutput:  output,
				SystemOut:      output,
			})
		}
	}

	return ParsedFile{Format: "gotestjson", TestResults: results}, nil
}

type goTestJSONEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

type goTestJSONSuite struct {
	PackageName string
	Cases       map[string]*goTestJSONCase
	CaseOrder   []string
	Output      strings.Builder
	PackageErr  bool
	Elapsed     float64
}

type goTestJSONCase struct {
	Name    string
	Output  strings.Builder
	Status  string
	Elapsed float64
}

func goTestJSONDurationMillis(seconds float64) int64 {
	return int64(math.Round(seconds * 1000))
}

func goTestJSONFailureMessage(status string) string {
	if status != "failed" {
		return ""
	}
	return "go test failed"
}

func goTestJSONFailureOutput(status, output string) string {
	if status != "failed" {
		return ""
	}
	return output
}
