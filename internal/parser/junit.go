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

type UploadFile struct {
	Name     string
	Contents []byte
}

type Parser interface {
	Name() string
	Detect(name string, contents []byte) bool
	Parse(ctx context.Context, file UploadFile) (ParsedFile, error)
}

type ParsedFile struct {
	Format      string
	TestResults []ParsedTestResult
}

type ParsedTestResult struct {
	SuiteName      string
	PackageName    string
	ClassName      string
	TestName       string
	FileName       string
	TestKey        string
	Status         string
	DurationMillis int64
	FailureMessage string
	FailureOutput  string
	SystemOut      string
	SystemErr      string
}

type Registry struct {
	parsers []Parser
}

func NewRegistry(parsers ...Parser) Registry {
	return Registry{parsers: parsers}
}

func (r Registry) ParseFiles(ctx context.Context, files []UploadFile) ([]ParsedFile, error) {
	parsed := make([]ParsedFile, 0, len(files))
	for _, file := range files {
		var matched Parser
		for _, parser := range r.parsers {
			if parser.Detect(file.Name, file.Contents) {
				matched = parser
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("unsupported report format for %s", file.Name)
		}
		item, err := matched.Parse(ctx, file)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, item)
	}
	return parsed, nil
}

type JUnitParser struct{}

func NewJUnitParser() JUnitParser {
	return JUnitParser{}
}

func (JUnitParser) Name() string {
	return "junit"
}

func (JUnitParser) Detect(name string, contents []byte) bool {
	if ext := strings.ToLower(filepath.Ext(name)); ext != ".xml" {
		return false
	}

	decoder := xml.NewDecoder(bytes.NewReader(contents))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		switch node := token.(type) {
		case xml.StartElement:
			return node.Name.Local == "testsuite" || node.Name.Local == "testsuites"
		}
	}
}

func (JUnitParser) Parse(_ context.Context, file UploadFile) (ParsedFile, error) {
	root := struct {
		XMLName xml.Name
	}{}
	if err := xml.Unmarshal(file.Contents, &root); err != nil {
		return ParsedFile{}, fmt.Errorf("parse junit root %s: %w", file.Name, err)
	}

	var suites []junitSuite
	switch root.XMLName.Local {
	case "testsuite":
		var suite junitSuite
		if err := xml.Unmarshal(file.Contents, &suite); err != nil {
			return ParsedFile{}, fmt.Errorf("parse junit suite %s: %w", file.Name, err)
		}
		suites = append(suites, suite)
	case "testsuites":
		var wrapper junitSuites
		if err := xml.Unmarshal(file.Contents, &wrapper); err != nil {
			return ParsedFile{}, fmt.Errorf("parse junit suites %s: %w", file.Name, err)
		}
		suites = wrapper.Suites
	default:
		return ParsedFile{}, fmt.Errorf("unsupported junit root %q in %s", root.XMLName.Local, file.Name)
	}

	results := make([]ParsedTestResult, 0)
	for _, suite := range suites {
		walkSuite(&results, suite, nil)
	}

	return ParsedFile{
		Format:      "junit",
		TestResults: results,
	}, nil
}

type junitSuites struct {
	Suites []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name      string       `xml:"name,attr"`
	Package   string       `xml:"package,attr"`
	Time      string       `xml:"time,attr"`
	Cases     []junitCase  `xml:"testcase"`
	Suites    []junitSuite `xml:"testsuite"`
	SystemOut string       `xml:"system-out"`
	SystemErr string       `xml:"system-err"`
}

type junitCase struct {
	Name      string       `xml:"name,attr"`
	ClassName string       `xml:"classname,attr"`
	File      string       `xml:"file,attr"`
	Time      string       `xml:"time,attr"`
	Failure   *junitStatus `xml:"failure"`
	Error     *junitStatus `xml:"error"`
	Skipped   *junitStatus `xml:"skipped"`
	SystemOut string       `xml:"system-out"`
	SystemErr string       `xml:"system-err"`
}

type junitStatus struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func walkSuite(results *[]ParsedTestResult, suite junitSuite, parents []string) {
	currentPath := append(append([]string{}, parents...), strings.TrimSpace(suite.Name))
	for _, testCase := range suite.Cases {
		suiteName := firstNonEmpty(testCase.ClassName, suite.Name, strings.Join(filterNonEmpty(currentPath), " / "))
		packageName := firstNonEmpty(suite.Package, suite.Name)
		className := firstNonEmpty(testCase.ClassName, suite.Name)
		status, message, output := caseStatus(testCase)
		*results = append(*results, ParsedTestResult{
			SuiteName:      strings.Join(filterNonEmpty(currentPath), " / "),
			PackageName:    packageName,
			ClassName:      className,
			TestName:       testCase.Name,
			FileName:       testCase.File,
			TestKey:        deriveTestKey(packageName, className, suiteName, testCase.Name, testCase.File),
			Status:         status,
			DurationMillis: parseDurationMillis(testCase.Time),
			FailureMessage: message,
			FailureOutput:  output,
			SystemOut:      strings.TrimSpace(testCase.SystemOut),
			SystemErr:      strings.TrimSpace(testCase.SystemErr),
		})
	}

	for _, child := range suite.Suites {
		walkSuite(results, child, currentPath)
	}
}

func caseStatus(testCase junitCase) (string, string, string) {
	switch {
	case testCase.Failure != nil:
		return "failed", testCase.Failure.Message, strings.TrimSpace(testCase.Failure.Text)
	case testCase.Error != nil:
		return "failed", firstNonEmpty(testCase.Error.Message, "error"), strings.TrimSpace(testCase.Error.Text)
	case testCase.Skipped != nil:
		return "skipped", testCase.Skipped.Message, strings.TrimSpace(testCase.Skipped.Text)
	default:
		return "passed", "", ""
	}
}

func deriveTestKey(parts ...string) string {
	return strings.ToLower(strings.Join(filterNonEmpty(parts), "::"))
}

func filterNonEmpty(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseDurationMillis(raw string) int64 {
	if raw == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return int64(seconds * 1000)
}
