package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"testrr/internal/auth"
	"testrr/internal/config"
	"testrr/internal/parser"
	"testrr/internal/store"
)

func TestServerDashboardIsPublicAndDoesNotExposeUploadForm(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	dashboardResp, err := client.Get(serverURL + "/projects/demo")
	if err != nil {
		t.Fatalf("dashboard request: %v", err)
	}
	defer dashboardResp.Body.Close()
	if dashboardResp.StatusCode != http.StatusOK {
		t.Fatalf("expected public dashboard, got %d", dashboardResp.StatusCode)
	}

	body, err := ioReadAll(dashboardResp.Body)
	if err != nil {
		t.Fatalf("read dashboard page: %v", err)
	}
	if strings.Contains(string(body), "Upload JUnit XML") {
		t.Fatalf("dashboard should not expose browser upload controls")
	}
}

func TestServerUIUploadRouteIsUnavailable(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	client := &http.Client{}
	uploadResp, err := postMultipartWithBasicAuth(client, serverURL+"/projects/demo/upload", "demo-user", "secret", map[string]string{
		"branch":    "main",
		"run_label": "should-not-exist",
	}, filepath.Join("..", "..", "testdata", "junit-mixed.xml"))
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusNotFound && uploadResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected ui upload route to be unavailable, got %d", uploadResp.StatusCode)
	}
}

func TestServerAPIUploadAndListRuns(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	client := &http.Client{}
	uploadResp, err := postMultipartWithBasicAuth(client, serverURL+"/api/v1/projects/demo/runs", "demo-user", "secret", map[string]string{
		"branch":    "release",
		"run_label": "release-001",
	}, filepath.Join("..", "..", "testdata", "junit-mixed.xml"))
	if err != nil {
		t.Fatalf("api upload: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected created response, got %d", uploadResp.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/projects/demo/runs?branch=release", nil)
	if err != nil {
		t.Fatalf("new list request: %v", err)
	}
	request.SetBasicAuth("demo-user", "secret")
	listResp, err := client.Do(request)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	defer listResp.Body.Close()

	var runs []store.Run
	if err := json.NewDecoder(listResp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].RunLabel != "release-001" {
		t.Fatalf("unexpected run label: %s", runs[0].RunLabel)
	}
}

func TestServerAPIUploadGoTestJSON(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	client := &http.Client{}
	uploadResp, err := postMultipartWithBasicAuth(client, serverURL+"/api/v1/projects/demo/runs", "demo-user", "secret", map[string]string{
		"branch":    "main",
		"run_label": "go-json",
	}, filepath.Join("..", "..", "testdata", "go-test-sample.json"))
	if err != nil {
		t.Fatalf("api upload: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected created response, got %d", uploadResp.StatusCode)
	}

	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/projects/demo/runs?branch=main", nil)
	if err != nil {
		t.Fatalf("new list request: %v", err)
	}
	request.SetBasicAuth("demo-user", "secret")
	listResp, err := client.Do(request)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	defer listResp.Body.Close()

	var runs []store.Run
	if err := json.NewDecoder(listResp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].PassedCount != 1 {
		t.Fatalf("expected 1 passed test, got %d", runs[0].PassedCount)
	}
	if runs[0].FailedCount != 2 {
		t.Fatalf("expected 2 failed tests, got %d", runs[0].FailedCount)
	}
}

func TestServerAPIUploadAggregatesMultipleReportFormats(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	junitContents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "junit-mixed.xml"))
	if err != nil {
		t.Fatalf("read junit fixture: %v", err)
	}
	goJSONContents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "go-test-sample.json"))
	if err != nil {
		t.Fatalf("read go test json fixture: %v", err)
	}

	client := &http.Client{}
	uploadResp, err := postMultipartFilesWithBasicAuth(client, serverURL+"/api/v1/projects/demo/runs", "demo-user", "secret", map[string]string{
		"branch":    "main",
		"run_label": "mixed-upload",
	}, []multipartUploadFile{
		{Name: "junit-mixed.xml", Contents: junitContents},
		{Name: "go-test-sample.json", Contents: goJSONContents},
	})
	if err != nil {
		t.Fatalf("api upload: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected created response, got %d", uploadResp.StatusCode)
	}

	var created store.Run
	if err := json.NewDecoder(uploadResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created run: %v", err)
	}
	if created.TotalCount != 6 || created.PassedCount != 2 || created.FailedCount != 3 || created.SkippedCount != 1 {
		t.Fatalf("unexpected aggregate counts: %+v", created)
	}

	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/projects/demo/runs/"+created.ID, nil)
	if err != nil {
		t.Fatalf("new get run request: %v", err)
	}
	request.SetBasicAuth("demo-user", "secret")
	runResp, err := client.Do(request)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	defer runResp.Body.Close()
	if runResp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok run response, got %d", runResp.StatusCode)
	}

	var runPayload struct {
		Run     store.Run          `json:"run"`
		Results []store.TestResult `json:"results"`
	}
	if err := json.NewDecoder(runResp.Body).Decode(&runPayload); err != nil {
		t.Fatalf("decode run payload: %v", err)
	}
	if len(runPayload.Results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(runPayload.Results))
	}
	foundSkip := false
	foundGoTestFailure := false
	for _, result := range runPayload.Results {
		if result.Status == "skipped" {
			foundSkip = true
		}
		if result.TestName == "TestParseFail" && result.Status == "failed" {
			foundGoTestFailure = true
		}
	}
	if !foundSkip || !foundGoTestFailure {
		t.Fatalf("expected mixed run payload to include skipped junit and failed go test results, got %+v", runPayload.Results)
	}
}

func TestServerAPIUploadFailedImportCreatesFailedRun(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	client := &http.Client{}
	uploadResp, err := postMultipartContentsWithBasicAuth(client, serverURL+"/api/v1/projects/demo/runs", "demo-user", "secret", map[string]string{
		"branch":    "main",
		"run_label": "broken-upload",
	}, "broken.xml", []byte("<testsuites><testsuite>"))
	if err != nil {
		t.Fatalf("api upload: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected created response, got %d", uploadResp.StatusCode)
	}

	var created store.Run
	if err := json.NewDecoder(uploadResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created run: %v", err)
	}
	if created.Status != "failed_import" {
		t.Fatalf("expected failed import status, got %+v", created)
	}
	if created.TotalCount != 0 || created.PassedCount != 0 || created.FailedCount != 0 || created.SkippedCount != 0 {
		t.Fatalf("expected failed import to avoid storing test counts, got %+v", created)
	}

	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/v1/projects/demo/runs?branch=main", nil)
	if err != nil {
		t.Fatalf("new list request: %v", err)
	}
	request.SetBasicAuth("demo-user", "secret")
	listResp, err := client.Do(request)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	defer listResp.Body.Close()

	var runs []store.Run
	if err := json.NewDecoder(listResp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "failed_import" {
		t.Fatalf("expected failed import run in listing, got %+v", runs)
	}
}

func TestServerRunPageAndTestHistoryRenderE2EOutput(t *testing.T) {
	t.Parallel()

	serverURL, cleanup := newTestServer(t)
	defer cleanup()

	report := `<testsuites>
  <testsuite name="suite-a" package="pkg/a">
    <testcase classname="pkg/a.E2E" name="TestAnsi" time="0.05">
      <failure message="boom">plain failure output</failure>
      <system-out>plain stdout</system-out>
      <system-err>plain stderr</system-err>
    </testcase>
  </testsuite>
</testsuites>`

	client := &http.Client{}
	uploadResp, err := postMultipartContentsWithBasicAuth(client, serverURL+"/api/v1/projects/demo/runs", "demo-user", "secret", map[string]string{
		"branch":    "main",
		"run_label": "ansi-run",
	}, "ansi.xml", []byte(report))
	if err != nil {
		t.Fatalf("api upload: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected created response, got %d", uploadResp.StatusCode)
	}

	var created store.Run
	if err := json.NewDecoder(uploadResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created run: %v", err)
	}

	runResp, err := client.Get(serverURL + "/projects/demo/runs/" + created.ID)
	if err != nil {
		t.Fatalf("run page request: %v", err)
	}
	defer runResp.Body.Close()
	runBody, err := ioReadAll(runResp.Body)
	if err != nil {
		t.Fatalf("read run page: %v", err)
	}
	runPage := string(runBody)
	for _, snippet := range []string{"TestAnsi", "Stdout", "Stderr", "term-container"} {
		if !strings.Contains(runPage, snippet) {
			t.Fatalf("expected run page to contain %q", snippet)
		}
	}

	testKey := "pkg/a::pkg/a.e2e::pkg/a.e2e::testansi"
	historyResp, err := client.Get(serverURL + "/projects/demo/tests?test_key=" + url.QueryEscape(testKey))
	if err != nil {
		t.Fatalf("test history request: %v", err)
	}
	defer historyResp.Body.Close()
	if historyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected public test history page, got %d", historyResp.StatusCode)
	}
	historyBody, err := ioReadAll(historyResp.Body)
	if err != nil {
		t.Fatalf("read test history page: %v", err)
	}
	historyPage := string(historyBody)
	for _, snippet := range []string{"data-chart-kind=\"test-duration\"", "Stdout", "Stderr", "term-container"} {
		if !strings.Contains(historyPage, snippet) {
			t.Fatalf("expected test history page to contain %q", snippet)
		}
	}

	chartResp, err := client.Get(serverURL + "/projects/demo/tests/chart?test_key=" + url.QueryEscape(testKey))
	if err != nil {
		t.Fatalf("test chart request: %v", err)
	}
	defer chartResp.Body.Close()
	if chartResp.StatusCode != http.StatusOK {
		t.Fatalf("expected test chart response, got %d", chartResp.StatusCode)
	}
	var chart store.TestDurationChart
	if err := json.NewDecoder(chartResp.Body).Decode(&chart); err != nil {
		t.Fatalf("decode test chart: %v", err)
	}
	if len(chart.Labels) != 1 || chart.Labels[0] != "ansi-run" || chart.Statuses[0] != "failed" {
		t.Fatalf("unexpected test chart payload: %+v", chart)
	}
}

func TestGroupRunResultsUsesDeterministicBuckets(t *testing.T) {
	groups := groupRunResults([]store.TestResult{
		{TestKey: "b", SuiteName: "suite-b", TestName: "TestB"},
		{TestKey: "a", SuiteName: "suite-a", TestName: "TestA"},
		{TestKey: "c", FileName: "fallback.xml", TestName: "TestC"},
	}, map[string][]string{
		"a": {"passed", "failed"},
	})

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if groups[0].Name != "suite-b" || groups[1].Name != "suite-a" || groups[2].Name != "fallback.xml" {
		t.Fatalf("unexpected group order: %+v", groups)
	}
	if len(groups[1].Results) != 1 || len(groups[1].Results[0].RecentStatuses) != 2 {
		t.Fatalf("expected recent statuses to be attached to grouped result, got %+v", groups[1].Results)
	}
}

func TestSortRunResults(t *testing.T) {
	results := []store.TestResult{
		{TestKey: "a", SuiteName: "suite-b", TestName: "Bravo", Status: "passed", DurationMillis: 10},
		{TestKey: "b", SuiteName: "suite-a", TestName: "Alpha", Status: "failed", DurationMillis: 50},
		{TestKey: "c", SuiteName: "suite-a", TestName: "Charlie", Status: "skipped", DurationMillis: 20},
	}

	slowest := sortRunResults(results, "slowest")
	if slowest[0].TestKey != "b" || slowest[1].TestKey != "c" || slowest[2].TestKey != "a" {
		t.Fatalf("unexpected slowest order: %+v", slowest)
	}

	byStatus := sortRunResults(results, "status")
	if byStatus[0].TestKey != "b" || byStatus[1].TestKey != "c" || byStatus[2].TestKey != "a" {
		t.Fatalf("unexpected status order: %+v", byStatus)
	}

	byName := sortRunResults(results, "name")
	if byName[0].TestKey != "b" || byName[1].TestKey != "a" || byName[2].TestKey != "c" {
		t.Fatalf("unexpected name order: %+v", byName)
	}

	uploaded := sortRunResults(results, "uploaded")
	if uploaded[0].TestKey != "b" || uploaded[1].TestKey != "c" || uploaded[2].TestKey != "a" {
		t.Fatalf("unexpected uploaded order: %+v", uploaded)
	}
}

func newTestServer(t *testing.T) (string, func()) {
	t.Helper()

	dataDir := t.TempDir()
	repo, err := store.Open(context.Background(), filepath.Join(dataDir, "testrr.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	hashed, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := repo.CreateProject(context.Background(), store.CreateProjectInput{
		ID:           "project-demo",
		Slug:         "demo",
		Name:         "Demo",
		Username:     "demo-user",
		PasswordHash: hashed,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	server, err := New(config.Config{
		DataDir:        dataDir,
		MaxUploadBytes: 10 * 1024 * 1024,
	}, repo, parser.NewRegistry(
		parser.NewJUnitParser(),
		parser.NewTRXParser(),
		parser.NewNUnitParser(),
		parser.NewGoTestJSONParser(),
	))
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ts := httptest.NewServer(server)
	return ts.URL, func() {
		ts.Close()
		repo.Close()
	}
}

func postMultipartWithBasicAuth(client *http.Client, endpoint, username, password string, fields map[string]string, fixturePath string) (*http.Response, error) {
	fileContents, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, err
	}
	return postMultipartContentsWithBasicAuth(client, endpoint, username, password, fields, filepath.Base(fixturePath), fileContents)
}

type multipartUploadFile struct {
	Name     string
	Contents []byte
}

func postMultipartContentsWithBasicAuth(client *http.Client, endpoint, username, password string, fields map[string]string, fileName string, fileContents []byte) (*http.Response, error) {
	return postMultipartFilesWithBasicAuth(client, endpoint, username, password, fields, []multipartUploadFile{{
		Name:     fileName,
		Contents: fileContents,
	}})
}

func postMultipartFilesWithBasicAuth(client *http.Client, endpoint, username, password string, fields map[string]string, files []multipartUploadFile) (*http.Response, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	for _, file := range files {
		fileWriter, err := writer.CreateFormFile("files", file.Name)
		if err != nil {
			return nil, err
		}
		if _, err := fileWriter.Write(file.Contents); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	request, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if username != "" {
		request.SetBasicAuth(username, password)
	}
	return client.Do(request)
}

func ioReadAll(responseBody io.Reader) ([]byte, error) {
	buffer := new(bytes.Buffer)
	if _, err := buffer.ReadFrom(responseBody); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return buffer.Bytes(), nil
}
