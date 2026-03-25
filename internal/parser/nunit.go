package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

type NUnitParser struct{}

func NewNUnitParser() NUnitParser {
	return NUnitParser{}
}

func (NUnitParser) Name() string {
	return "nunit"
}

func (NUnitParser) Detect(name string, contents []byte) bool {
	if ext := strings.ToLower(filepath.Ext(name)); ext != ".xml" {
		return false
	}
	decoder := xml.NewDecoder(bytes.NewReader(contents))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if el, ok := token.(xml.StartElement); ok {
			return el.Name.Local == "test-run" || el.Name.Local == "test-results"
		}
	}
}

func (NUnitParser) Parse(_ context.Context, file UploadFile) (ParsedFile, error) {
	root := struct {
		XMLName xml.Name
	}{}
	if err := xml.Unmarshal(file.Contents, &root); err != nil {
		return ParsedFile{}, fmt.Errorf("parse nunit root %s: %w", file.Name, err)
	}

	results := make([]ParsedTestResult, 0)
	switch root.XMLName.Local {
	case "test-run":
		var run nunitRun
		if err := xml.Unmarshal(file.Contents, &run); err != nil {
			return ParsedFile{}, fmt.Errorf("parse nunit run %s: %w", file.Name, err)
		}
		for _, suite := range run.Suites {
			walkNUnitSuite(&results, suite)
		}
	case "test-results":
		var run nunitV2Results
		if err := xml.Unmarshal(file.Contents, &run); err != nil {
			return ParsedFile{}, fmt.Errorf("parse nunit results %s: %w", file.Name, err)
		}
		for _, suite := range run.Suites {
			walkNUnitSuite(&results, suite)
		}
	default:
		return ParsedFile{}, fmt.Errorf("unsupported nunit root %q in %s", root.XMLName.Local, file.Name)
	}

	return ParsedFile{Format: "nunit", TestResults: results}, nil
}

func walkNUnitSuite(results *[]ParsedTestResult, suite nunitSuite) {
	// Collect cases from direct children and NUnit 2 nested <results> element.
	cases := append(suite.Cases, suite.Nested.Cases...)
	for _, tc := range cases {
		fullName := firstNonEmpty(tc.FullName, tc.Name)
		packageName, className := nunitSplitFullName(fullName)
		status, message, output := nunitCaseStatus(tc)
		*results = append(*results, ParsedTestResult{
			SuiteName:      firstNonEmpty(suite.FullName, suite.Name),
			PackageName:    packageName,
			ClassName:      className,
			TestName:       tc.Name,
			TestKey:        deriveTestKey(packageName, className, tc.Name),
			Status:         status,
			DurationMillis: nunitDurationMillis(tc),
			FailureMessage: message,
			FailureOutput:  output,
		})
	}
	for _, child := range append(suite.Suites, suite.Nested.Suites...) {
		walkNUnitSuite(results, child)
	}
}

func nunitSplitFullName(fullName string) (packageName, className string) {
	// fullName = "Namespace.Sub.ClassName.TestName" — drop the last segment.
	i := strings.LastIndex(fullName, ".")
	if i < 0 {
		return fullName, fullName
	}
	withoutTest := fullName[:i]
	j := strings.LastIndex(withoutTest, ".")
	if j < 0 {
		return withoutTest, withoutTest
	}
	return withoutTest[:j], withoutTest
}

func nunitCaseStatus(tc nunitCase) (status, message, output string) {
	switch tc.Result {
	case "Passed", "Success":
		return "passed", "", ""
	case "Failed", "Failure":
		return "failed",
			strings.TrimSpace(tc.Failure.Message),
			strings.TrimSpace(tc.Failure.StackTrace)
	default:
		return "skipped", strings.TrimSpace(tc.Reason.Message), ""
	}
}

func nunitDurationMillis(tc nunitCase) int64 {
	d := tc.Duration
	if d == 0 {
		d = tc.Time
	}
	return int64(d * 1000)
}

// NUnit 3 root.
type nunitRun struct {
	XMLName xml.Name     `xml:"test-run"`
	Suites  []nunitSuite `xml:"test-suite"`
}

// NUnit 2 root.
type nunitV2Results struct {
	XMLName xml.Name     `xml:"test-results"`
	Suites  []nunitSuite `xml:"test-suite"`
}

type nunitSuite struct {
	Name     string            `xml:"name,attr"`
	FullName string            `xml:"fullname,attr"`
	Result   string            `xml:"result,attr"`
	Duration float64           `xml:"duration,attr"`
	Cases    []nunitCase       `xml:"test-case"`
	Suites   []nunitSuite      `xml:"test-suite"`
	Nested   nunitNestedBlock  `xml:"results"`
}

// NUnit 2 nests test-case and test-suite under a <results> element.
type nunitNestedBlock struct {
	Cases  []nunitCase  `xml:"test-case"`
	Suites []nunitSuite `xml:"test-suite"`
}

type nunitCase struct {
	Name     string       `xml:"name,attr"`
	FullName string       `xml:"fullname,attr"`
	Result   string       `xml:"result,attr"`
	Duration float64      `xml:"duration,attr"`
	Time     float64      `xml:"time,attr"`
	Failure  nunitFailure `xml:"failure"`
	Reason   nunitReason  `xml:"reason"`
}

type nunitFailure struct {
	Message    string `xml:"message"`
	StackTrace string `xml:"stack-trace"`
}

type nunitReason struct {
	Message string `xml:"message"`
}
