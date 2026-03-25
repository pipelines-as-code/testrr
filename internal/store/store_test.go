package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLStoreCreateProjectAndRunHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	project, err := repo.CreateProject(ctx, CreateProjectInput{
		ID:           "project-1",
		Slug:         "demo",
		Name:         "Demo",
		Username:     "demo",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	run1 := Run{
		ID:             "run-1",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "run-1",
		Status:         "complete",
		StartedAt:      time.Now().UTC(),
		UploadedAt:     time.Now().UTC(),
		TotalCount:     1,
		PassedCount:    1,
		DurationMillis: 20,
	}
	if _, err := repo.CreateRun(ctx, CreateRunInput{
		Run: run1,
		TestResults: []TestResult{{
			ID:        "result-1",
			RunID:     run1.ID,
			ProjectID: project.ID,
			TestKey:   "pkg::suite::TestPass",
			SuiteName: "suite",
			ClassName: "suite",
			TestName:  "TestPass",
			Status:    "passed",
		}},
	}); err != nil {
		t.Fatalf("create first run: %v", err)
	}

	previous, err := repo.FindPreviousRun(ctx, project.ID, "main")
	if err != nil {
		t.Fatalf("find previous run: %v", err)
	}
	if previous.ID != run1.ID {
		t.Fatalf("expected previous run %s, got %s", run1.ID, previous.ID)
	}

	run2 := Run{
		ID:             "run-2",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "run-2",
		Status:         "complete",
		StartedAt:      time.Now().UTC(),
		UploadedAt:     time.Now().UTC().Add(time.Minute),
		PreviousRunID:  run1.ID,
		TotalCount:     1,
		FailedCount:    1,
		DurationMillis: 30,
		NewFailures:    1,
	}
	if _, err := repo.CreateRun(ctx, CreateRunInput{
		Run: run2,
		TestResults: []TestResult{{
			ID:         "result-2",
			RunID:      run2.ID,
			ProjectID:  project.ID,
			TestKey:    "pkg::suite::TestPass",
			SuiteName:  "suite",
			ClassName:  "suite",
			TestName:   "TestPass",
			Status:     "failed",
			Regression: true,
		}},
	}); err != nil {
		t.Fatalf("create second run: %v", err)
	}

	summary, err := repo.GetChartSummary(ctx, project.ID, "main", 10)
	if err != nil {
		t.Fatalf("chart summary: %v", err)
	}
	if len(summary.Labels) != 2 {
		t.Fatalf("expected 2 chart points, got %d", len(summary.Labels))
	}

	history, err := repo.GetTestHistory(ctx, project.ID, "pkg::suite::TestPass", 10)
	if err != nil {
		t.Fatalf("test history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 history rows, got %d", len(history))
	}
	if history[0].Status != "failed" {
		t.Fatalf("expected latest history entry to be failed, got %s", history[0].Status)
	}
}

func TestSQLStoreE2EAnalytics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	project, err := repo.CreateProject(ctx, CreateProjectInput{
		ID:           "project-analytics",
		Slug:         "analytics",
		Name:         "Analytics",
		Username:     "demo",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	base := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	mustCreateRun(t, repo, project.ID, Run{
		ID:             "main-run-1",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "main-run-1",
		Status:         "complete",
		StartedAt:      base,
		UploadedAt:     base,
		TotalCount:     3,
		PassedCount:    2,
		FailedCount:    1,
		DurationMillis: 1310,
	}, []TestResult{
		{ID: "result-1", RunID: "main-run-1", ProjectID: project.ID, TestKey: "pkg/a::suite::testflaky", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestFlaky", Status: "passed", DurationMillis: 10},
		{ID: "result-2", RunID: "main-run-1", ProjectID: project.ID, TestKey: "pkg/a::suite::testslow", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestSlow", Status: "passed", DurationMillis: 1000},
		{ID: "result-3", RunID: "main-run-1", ProjectID: project.ID, TestKey: "pkg/a::suite::teststable", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestStable", Status: "failed", DurationMillis: 300, FailureMessage: "boom", FailureOutput: "\x1b[31mred failure\x1b[0m", SystemOut: "\x1b[32mgreen out\x1b[0m", SystemErr: "\x1b[33myellow err\x1b[0m"},
	})
	mustCreateRun(t, repo, project.ID, Run{
		ID:             "main-run-2",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "main-run-2",
		Status:         "complete",
		StartedAt:      base.Add(time.Minute),
		UploadedAt:     base.Add(time.Minute),
		TotalCount:     3,
		PassedCount:    1,
		FailedCount:    2,
		DurationMillis: 1550,
	}, []TestResult{
		{ID: "result-4", RunID: "main-run-2", ProjectID: project.ID, TestKey: "pkg/a::suite::testflaky", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestFlaky", Status: "failed", DurationMillis: 20, FailureMessage: "flake", FailureOutput: "\x1b[31mflaky fail\x1b[0m"},
		{ID: "result-5", RunID: "main-run-2", ProjectID: project.ID, TestKey: "pkg/a::suite::testslow", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestSlow", Status: "passed", DurationMillis: 1200},
		{ID: "result-6", RunID: "main-run-2", ProjectID: project.ID, TestKey: "pkg/a::suite::teststable", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestStable", Status: "failed", DurationMillis: 330, FailureMessage: "boom again"},
	})
	mustCreateRun(t, repo, project.ID, Run{
		ID:             "main-run-3",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "main-run-3",
		Status:         "complete",
		StartedAt:      base.Add(2 * time.Minute),
		UploadedAt:     base.Add(2 * time.Minute),
		TotalCount:     3,
		PassedCount:    3,
		DurationMillis: 1425,
	}, []TestResult{
		{ID: "result-7", RunID: "main-run-3", ProjectID: project.ID, TestKey: "pkg/a::suite::testflaky", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestFlaky", Status: "passed", DurationMillis: 15},
		{ID: "result-8", RunID: "main-run-3", ProjectID: project.ID, TestKey: "pkg/a::suite::testslow", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestSlow", Status: "passed", DurationMillis: 1100},
		{ID: "result-9", RunID: "main-run-3", ProjectID: project.ID, TestKey: "pkg/a::suite::teststable", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestStable", Status: "passed", DurationMillis: 310},
	})
	mustCreateRun(t, repo, project.ID, Run{
		ID:             "release-run-1",
		ProjectID:      project.ID,
		Branch:         "release",
		RunLabel:       "release-run-1",
		Status:         "complete",
		StartedAt:      base.Add(3 * time.Minute),
		UploadedAt:     base.Add(3 * time.Minute),
		TotalCount:     1,
		FailedCount:    1,
		DurationMillis: 900,
	}, []TestResult{
		{ID: "result-10", RunID: "release-run-1", ProjectID: project.ID, TestKey: "pkg/a::suite::teststable", SuiteName: "suite-a", ClassName: "pkg/a.E2E", TestName: "TestStable", Status: "failed", DurationMillis: 900, FailureMessage: "release-only"},
	})

	data, err := repo.GetDashboardData(ctx, project.ID, "main", 10)
	if err != nil {
		t.Fatalf("dashboard data: %v", err)
	}
	if len(data.TopFailing) == 0 || data.TopFailing[0].TestKey != "pkg/a::suite::teststable" || data.TopFailing[0].FailureCount != 2 {
		t.Fatalf("expected branch-aware top failing data, got %+v", data.TopFailing)
	}
	if len(data.TopFlaky) != 1 || data.TopFlaky[0].TestKey != "pkg/a::suite::testflaky" || data.TopFlaky[0].TransitionCount != 2 {
		t.Fatalf("expected flaky test ranking, got %+v", data.TopFlaky)
	}
	if len(data.SlowestTests) == 0 || data.SlowestTests[0].TestKey != "pkg/a::suite::testslow" || data.SlowestTests[0].AverageDurationMillis != 1100 {
		t.Fatalf("expected slowest test ranking, got %+v", data.SlowestTests)
	}

	releaseData, err := repo.GetDashboardData(ctx, project.ID, "release", 10)
	if err != nil {
		t.Fatalf("release dashboard data: %v", err)
	}
	if len(releaseData.TopFailing) == 0 || releaseData.TopFailing[0].FailureCount != 1 {
		t.Fatalf("expected release branch failures to stay isolated, got %+v", releaseData.TopFailing)
	}

	statuses, err := repo.GetRecentTestStatuses(ctx, project.ID, "main", []string{"pkg/a::suite::testflaky"}, 3)
	if err != nil {
		t.Fatalf("recent test statuses: %v", err)
	}
	gotStatuses := statuses["pkg/a::suite::testflaky"]
	if len(gotStatuses) != 3 || gotStatuses[0] != "passed" || gotStatuses[1] != "failed" || gotStatuses[2] != "passed" {
		t.Fatalf("unexpected recent statuses: %+v", gotStatuses)
	}

	history, err := repo.GetTestHistory(ctx, project.ID, "pkg/a::suite::teststable", 10)
	if err != nil {
		t.Fatalf("test history: %v", err)
	}
	foundFailureOutput := false
	foundSystemOut := false
	foundSystemErr := false
	for _, entry := range history {
		if entry.FailureOutput != "" {
			foundFailureOutput = true
		}
		if entry.SystemOut != "" {
			foundSystemOut = true
		}
		if entry.SystemErr != "" {
			foundSystemErr = true
		}
	}
	if len(history) < 3 || !foundFailureOutput || !foundSystemOut || !foundSystemErr {
		t.Fatalf("expected rich failure output in history, got %+v", history)
	}

	chart, err := repo.GetTestDurationChart(ctx, project.ID, "pkg/a::suite::testflaky", 10)
	if err != nil {
		t.Fatalf("test duration chart: %v", err)
	}
	if len(chart.Labels) != 3 || chart.Labels[0] != "main-run-1" || chart.Durations[1] != 20 || chart.Statuses[2] != "passed" {
		t.Fatalf("unexpected duration chart: %+v", chart)
	}
}

func newTestStore(t *testing.T) *SQLStore {
	t.Helper()
	repo, err := Open(context.Background(), filepath.Join(t.TempDir(), "testrr.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return repo
}

func mustCreateRun(t *testing.T, repo *SQLStore, projectID string, run Run, results []TestResult) {
	t.Helper()
	for i := range results {
		results[i].ProjectID = projectID
	}
	if _, err := repo.CreateRun(context.Background(), CreateRunInput{
		Run:         run,
		TestResults: results,
	}); err != nil {
		t.Fatalf("create run %s: %v", run.ID, err)
	}
}
