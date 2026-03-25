package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type testEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

type suiteState struct {
	Name       string
	Cases      map[string]*caseState
	CaseOrder  []string
	Output     strings.Builder
	PackageErr bool
	Elapsed    float64
}

type caseState struct {
	Name    string
	Output  strings.Builder
	Status  string
	Elapsed float64
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name      string      `xml:"name,attr"`
	Tests     int         `xml:"tests,attr"`
	Failures  int         `xml:"failures,attr"`
	Skipped   int         `xml:"skipped,attr"`
	Time      string      `xml:"time,attr"`
	TestCases []junitCase `xml:"testcase"`
	SystemOut *junitCData `xml:"system-out,omitempty"`
}

type junitCase struct {
	Name      string         `xml:"name,attr"`
	Classname string         `xml:"classname,attr,omitempty"`
	Time      string         `xml:"time,attr"`
	Failure   *junitFailure  `xml:"failure,omitempty"`
	Skipped   *junitSkipped  `xml:"skipped,omitempty"`
	SystemOut *junitCData    `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",cdata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

type junitCData struct {
	Text string `xml:",cdata"`
}

func main() {
	var inputPath string
	var outputPath string

	flag.StringVar(&inputPath, "in", "", "path to go test -json output")
	flag.StringVar(&outputPath, "out", "", "path to write JUnit XML")
	flag.Parse()

	if inputPath == "" || outputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: gojson2junit -in <go-test-json> -out <junit-xml>")
		os.Exit(2)
	}

	input, err := os.Open(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open input: %v\n", err)
		os.Exit(1)
	}
	defer input.Close()

	suites, err := parseEvents(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse events: %v\n", err)
		os.Exit(1)
	}

	output, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create output: %v\n", err)
		os.Exit(1)
	}
	defer output.Close()

	if err := writeJUnit(output, suites); err != nil {
		fmt.Fprintf(os.Stderr, "write junit: %v\n", err)
		os.Exit(1)
	}
}

func parseEvents(r io.Reader) ([]*suiteState, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024), 4*1024*1024)

	suitesByName := map[string]*suiteState{}
	var suiteOrder []*suiteState

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}

		var event testEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		if event.Package == "" {
			continue
		}

		suite := suitesByName[event.Package]
		if suite == nil {
			suite = &suiteState{
				Name:  event.Package,
				Cases: map[string]*caseState{},
			}
			suitesByName[event.Package] = suite
			suiteOrder = append(suiteOrder, suite)
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
			testCase = &caseState{Name: event.Test}
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
		return nil, err
	}
	return suiteOrder, nil
}

func writeJUnit(w io.Writer, suites []*suiteState) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")

	doc := junitSuites{}
	for _, suiteState := range suites {
		doc.Suites = append(doc.Suites, toJUnitSuite(suiteState))
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return enc.Flush()
}

func toJUnitSuite(state *suiteState) junitSuite {
	suite := junitSuite{Name: state.Name}

	for _, name := range state.CaseOrder {
		testCase := state.Cases[name]
		junitCase := junitCase{
			Name:      testCase.Name,
			Classname: state.Name,
			Time:      formatSeconds(testCase.Elapsed),
		}

		output := strings.TrimSpace(testCase.Output.String())
		if output != "" {
			junitCase.SystemOut = &junitCData{Text: output}
		}

		switch testCase.Status {
		case "fail":
			suite.Failures++
			junitCase.Failure = &junitFailure{Message: "go test failed", Text: output}
		case "skip":
			suite.Skipped++
			junitCase.Skipped = &junitSkipped{Message: "go test skipped"}
		}

		suite.Tests++
		suite.TestCases = append(suite.TestCases, junitCase)
	}

	if suite.Tests == 0 && state.PackageErr {
		output := strings.TrimSpace(state.Output.String())
		suite.Tests = 1
		suite.Failures = 1
		suite.TestCases = append(suite.TestCases, junitCase{
			Name:      "package",
			Classname: state.Name,
			Time:      formatSeconds(state.Elapsed),
			Failure:   &junitFailure{Message: "package setup failed", Text: output},
			SystemOut: cdataOrNil(output),
		})
	}

	if suite.Tests == 0 {
		suite.Tests = 1
		suite.Skipped = 1
		suite.TestCases = append(suite.TestCases, junitCase{
			Name:      "package",
			Classname: state.Name,
			Time:      "0",
			Skipped:   &junitSkipped{Message: "no tests were executed"},
		})
	}

	if output := strings.TrimSpace(state.Output.String()); output != "" {
		suite.SystemOut = &junitCData{Text: output}
	}

	totalTime := 0.0
	for _, testCase := range suite.TestCases {
		var elapsed float64
		fmt.Sscanf(testCase.Time, "%f", &elapsed)
		totalTime += elapsed
	}
	if totalTime == 0 && state.Elapsed > 0 {
		totalTime = state.Elapsed
	}
	suite.Time = formatSeconds(totalTime)

	return suite
}

func cdataOrNil(text string) *junitCData {
	if text == "" {
		return nil
	}
	return &junitCData{Text: text}
}

func formatSeconds(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}
