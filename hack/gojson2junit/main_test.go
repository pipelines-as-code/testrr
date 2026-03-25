package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseEventsAndWriteJUnit(t *testing.T) {
	input := strings.NewReader(`
{"Action":"run","Package":"testrr/internal/parser","Test":"TestPass"}
{"Action":"output","Package":"testrr/internal/parser","Test":"TestPass","Output":"=== RUN   TestPass\n"}
{"Action":"pass","Package":"testrr/internal/parser","Test":"TestPass","Elapsed":0.12}
{"Action":"run","Package":"testrr/internal/parser","Test":"TestFail"}
{"Action":"output","Package":"testrr/internal/parser","Test":"TestFail","Output":"boom\n"}
{"Action":"fail","Package":"testrr/internal/parser","Test":"TestFail","Elapsed":0.34}
{"Action":"output","Package":"testrr/internal/parser","Output":"FAIL testrr/internal/parser\n"}
{"Action":"fail","Package":"testrr/internal/parser","Elapsed":0.46}
`)

	suites, err := parseEvents(input)
	if err != nil {
		t.Fatalf("parseEvents() error = %v", err)
	}
	if len(suites) != 1 {
		t.Fatalf("len(suites) = %d, want 1", len(suites))
	}

	var output bytes.Buffer
	if err := writeJUnit(&output, suites); err != nil {
		t.Fatalf("writeJUnit() error = %v", err)
	}

	xmlOutput := output.String()
	for _, want := range []string{
		`<testsuite name="testrr/internal/parser" tests="2" failures="1" skipped="0"`,
		`<testcase name="TestPass" classname="testrr/internal/parser" time="0.12">`,
		`<testcase name="TestFail" classname="testrr/internal/parser" time="0.34">`,
		`<failure message="go test failed"><![CDATA[boom]]></failure>`,
	} {
		if !strings.Contains(xmlOutput, want) {
			t.Fatalf("XML output missing %q:\n%s", want, xmlOutput)
		}
	}
}

func TestPackageFailureWithoutTestsProducesSyntheticCase(t *testing.T) {
	input := strings.NewReader(`
{"Action":"output","Package":"testrr/internal/build","Output":"# testrr/internal/build\n"}
{"Action":"output","Package":"testrr/internal/build","Output":"compile error\n"}
{"Action":"fail","Package":"testrr/internal/build","Elapsed":0.02}
`)

	suites, err := parseEvents(input)
	if err != nil {
		t.Fatalf("parseEvents() error = %v", err)
	}

	var output bytes.Buffer
	if err := writeJUnit(&output, suites); err != nil {
		t.Fatalf("writeJUnit() error = %v", err)
	}

	xmlOutput := output.String()
	for _, want := range []string{
		`<testsuite name="testrr/internal/build" tests="1" failures="1" skipped="0"`,
		`<testcase name="package" classname="testrr/internal/build" time="0.02">`,
		`<failure message="package setup failed"><![CDATA[# testrr/internal/build`,
	} {
		if !strings.Contains(xmlOutput, want) {
			t.Fatalf("XML output missing %q:\n%s", want, xmlOutput)
		}
	}
}
