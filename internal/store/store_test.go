package store

import (
	"context"
	"os"
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

func TestSQLStorePruneTestOutputsKeepsRunHistory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	project, err := repo.CreateProject(ctx, CreateProjectInput{
		ID:           "project-prune",
		Slug:         "prune",
		Name:         "Prune",
		Username:     "demo",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	run := Run{
		ID:             "run-prune",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "prune-run",
		Status:         "complete",
		StartedAt:      time.Now().UTC(),
		UploadedAt:     time.Now().UTC(),
		TotalCount:     1,
		FailedCount:    1,
		DurationMillis: 123,
	}
	result := TestResult{
		ID:             "result-prune",
		RunID:          run.ID,
		ProjectID:      project.ID,
		TestKey:        "pkg::suite::TestTrimmed",
		SuiteName:      "suite",
		ClassName:      "suite",
		TestName:       "TestTrimmed",
		Status:         "failed",
		FailureMessage: "boom",
		FailureOutput:  "stacktrace",
		SystemOut:      "stdout",
		SystemErr:      "stderr",
	}
	if _, err := repo.CreateRun(ctx, CreateRunInput{
		Run:         run,
		TestResults: []TestResult{result},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	var storedOutputs int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM test_result_outputs`).Scan(&storedOutputs); err != nil {
		t.Fatalf("count stored outputs: %v", err)
	}
	if storedOutputs != 1 {
		t.Fatalf("expected 1 stored output row, got %d", storedOutputs)
	}

	history, err := repo.GetTestHistory(ctx, project.ID, result.TestKey, 10)
	if err != nil {
		t.Fatalf("history before prune: %v", err)
	}
	if len(history) != 1 || history[0].FailureOutput != "stacktrace" || history[0].SystemOut != "stdout" || history[0].SystemErr != "stderr" {
		t.Fatalf("expected stored outputs before prune, got %+v", history)
	}

	pruned, err := repo.PruneTestOutputs(ctx, run.UploadedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("prune outputs: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned output row, got %d", pruned)
	}
	if err := repo.Compact(ctx); err != nil {
		t.Fatalf("compact store: %v", err)
	}

	results, err := repo.ListRunResults(ctx, run.ID)
	if err != nil {
		t.Fatalf("list results after prune: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after prune, got %d", len(results))
	}
	if results[0].FailureMessage != "boom" {
		t.Fatalf("expected failure message to remain, got %q", results[0].FailureMessage)
	}
	if results[0].FailureOutput != "" || results[0].SystemOut != "" || results[0].SystemErr != "" {
		t.Fatalf("expected heavy outputs to be pruned, got %+v", results[0])
	}
}

func TestSQLStoreCreateRunDeduplicatesOutputBlobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	project, err := repo.CreateProject(ctx, CreateProjectInput{
		ID:           "project-output-dedup",
		Slug:         "output-dedup",
		Name:         "Output Dedup",
		Username:     "demo",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	run := Run{
		ID:             "run-output-dedup",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "output-dedup",
		Status:         "complete",
		StartedAt:      time.Now().UTC(),
		UploadedAt:     time.Now().UTC(),
		TotalCount:     2,
		FailedCount:    2,
		DurationMillis: 42,
	}
	sharedOutput := "same failure output"
	if _, err := repo.CreateRun(ctx, CreateRunInput{
		Run: run,
		TestResults: []TestResult{
			{
				ID:            "result-output-dedup-1",
				RunID:         run.ID,
				ProjectID:     project.ID,
				TestKey:       "pkg::suite::TestOne",
				SuiteName:     "suite",
				ClassName:     "suite",
				TestName:      "TestOne",
				Status:        "failed",
				FailureOutput: sharedOutput,
			},
			{
				ID:            "result-output-dedup-2",
				RunID:         run.ID,
				ProjectID:     project.ID,
				TestKey:       "pkg::suite::TestTwo",
				SuiteName:     "suite",
				ClassName:     "suite",
				TestName:      "TestTwo",
				Status:        "failed",
				FailureOutput: sharedOutput,
			},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	var blobCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM output_blobs`).Scan(&blobCount); err != nil {
		t.Fatalf("count output blobs: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("expected 1 deduplicated output blob, got %d", blobCount)
	}

	var outputRows int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM test_result_outputs`).Scan(&outputRows); err != nil {
		t.Fatalf("count test result outputs: %v", err)
	}
	if outputRows != 2 {
		t.Fatalf("expected 2 output references, got %d", outputRows)
	}
}

func TestSQLStorePruneRunsDeletesArtifactsAndOrphanedBlobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	project, err := repo.CreateProject(ctx, CreateProjectInput{
		ID:           "project-run-prune",
		Slug:         "run-prune",
		Name:         "Run Prune",
		Username:     "demo",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	artifactDir := t.TempDir()
	artifactPath := filepath.Join(artifactDir, "report.xml.gz")
	if err := os.WriteFile(artifactPath, []byte("artifact"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	run := Run{
		ID:             "run-prune-full",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "run-prune-full",
		Status:         "complete",
		StartedAt:      time.Now().UTC().Add(-48 * time.Hour),
		UploadedAt:     time.Now().UTC().Add(-48 * time.Hour),
		TotalCount:     1,
		FailedCount:    1,
		DurationMillis: 12,
	}
	if _, err := repo.CreateRun(ctx, CreateRunInput{
		Run: run,
		Artifacts: []Artifact{{
			ID:          "artifact-prune-full",
			RunID:       run.ID,
			FileName:    "report.xml",
			Format:      "junit",
			FilePath:    artifactPath,
			Checksum:    "sum",
			SizeBytes:   8,
			ParseStatus: "failed",
		}},
		TestResults: []TestResult{{
			ID:            "result-prune-full",
			RunID:         run.ID,
			ProjectID:     project.ID,
			TestKey:       "pkg::suite::TestPrune",
			SuiteName:     "suite",
			ClassName:     "suite",
			TestName:      "TestPrune",
			Status:        "failed",
			FailureOutput: "prune me",
		}},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	prunedRuns, err := repo.PruneRuns(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("prune runs: %v", err)
	}
	if prunedRuns != 1 {
		t.Fatalf("expected 1 pruned run, got %d", prunedRuns)
	}
	if _, err := os.Stat(artifactPath); !os.IsNotExist(err) {
		t.Fatalf("expected artifact to be deleted, stat err=%v", err)
	}

	var runCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE id = ?`, run.ID).Scan(&runCount); err != nil {
		t.Fatalf("count pruned runs: %v", err)
	}
	if runCount != 0 {
		t.Fatalf("expected run to be deleted, got %d", runCount)
	}

	var blobCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM output_blobs`).Scan(&blobCount); err != nil {
		t.Fatalf("count pruned blobs: %v", err)
	}
	if blobCount != 0 {
		t.Fatalf("expected orphaned output blobs to be deleted, got %d", blobCount)
	}
}

func TestSQLStoreCreateRunNormalizesTestCatalog(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	project, err := repo.CreateProject(ctx, CreateProjectInput{
		ID:           "project-normalized",
		Slug:         "normalized",
		Name:         "Normalized",
		Username:     "demo",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	testKey := "pkg::suite::TestShared"
	firstRun := Run{
		ID:             "run-normalized-1",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "normalized-1",
		Status:         "complete",
		StartedAt:      time.Now().UTC(),
		UploadedAt:     time.Now().UTC(),
		TotalCount:     1,
		PassedCount:    1,
		DurationMillis: 10,
	}
	secondRun := Run{
		ID:             "run-normalized-2",
		ProjectID:      project.ID,
		Branch:         "main",
		RunLabel:       "normalized-2",
		Status:         "complete",
		StartedAt:      time.Now().UTC().Add(time.Minute),
		UploadedAt:     time.Now().UTC().Add(time.Minute),
		TotalCount:     1,
		PassedCount:    1,
		DurationMillis: 12,
	}

	for _, item := range []struct {
		run    Run
		result TestResult
	}{
		{
			run: firstRun,
			result: TestResult{
				ID:        "result-normalized-1",
				RunID:     firstRun.ID,
				ProjectID: project.ID,
				TestKey:   testKey,
				SuiteName: "suite",
				ClassName: "pkg.Suite",
				TestName:  "TestShared",
				Status:    "passed",
			},
		},
		{
			run: secondRun,
			result: TestResult{
				ID:        "result-normalized-2",
				RunID:     secondRun.ID,
				ProjectID: project.ID,
				TestKey:   testKey,
				SuiteName: "suite",
				ClassName: "pkg.Suite",
				TestName:  "TestShared",
				Status:    "passed",
			},
		},
	} {
		if _, err := repo.CreateRun(ctx, CreateRunInput{
			Run:         item.run,
			TestResults: []TestResult{item.result},
		}); err != nil {
			t.Fatalf("create run %s: %v", item.run.ID, err)
		}
	}

	var catalogCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tests WHERE project_id = ? AND test_key = ?`, project.ID, testKey).Scan(&catalogCount); err != nil {
		t.Fatalf("count normalized tests: %v", err)
	}
	if catalogCount != 1 {
		t.Fatalf("expected a single normalized test row, got %d", catalogCount)
	}

	rows, err := repo.db.QueryContext(ctx, `
		SELECT test_id, project_id, test_key, suite_name, package_name, class_name, test_name, file_name
		FROM test_results
		ORDER BY id
	`)
	if err != nil {
		t.Fatalf("load normalized result rows: %v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var testID string
		var projectID string
		var storedTestKey string
		var suiteName string
		var packageName string
		var className string
		var testName string
		var fileName string
		if err := rows.Scan(&testID, &projectID, &storedTestKey, &suiteName, &packageName, &className, &testName, &fileName); err != nil {
			t.Fatalf("scan normalized result row: %v", err)
		}
		if testID == "" {
			t.Fatal("expected normalized test_id to be stored")
		}
		if projectID != project.ID || storedTestKey != "" || suiteName != "" || packageName != "" || className != "" || testName != "" || fileName != "" {
			t.Fatalf("expected only project_id to remain on normalized rows, got project=%q key=%q suite=%q class=%q test=%q file=%q", projectID, storedTestKey, suiteName, className, testName, fileName)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("load normalized result rows: %v", err)
	}
	if seen != 2 {
		t.Fatalf("expected 2 normalized test rows, got %d", seen)
	}
}

func TestSQLStoreMigrateBackfillsLegacyOutputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newTestStore(t)
	defer repo.Close()

	for _, statement := range []string{
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			slug TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE runs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			branch TEXT NOT NULL DEFAULT '',
			commit_sha TEXT NOT NULL DEFAULT '',
			build_id TEXT NOT NULL DEFAULT '',
			build_url TEXT NOT NULL DEFAULT '',
			environment TEXT NOT NULL DEFAULT '',
			run_label TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			uploaded_at TEXT NOT NULL,
			previous_run_id TEXT NOT NULL DEFAULT '',
			total_count INTEGER NOT NULL,
			passed_count INTEGER NOT NULL,
			failed_count INTEGER NOT NULL,
			skipped_count INTEGER NOT NULL,
			duration_millis INTEGER NOT NULL,
			new_failures INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE TABLE test_results (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			test_key TEXT NOT NULL,
			suite_name TEXT NOT NULL,
			package_name TEXT NOT NULL,
			class_name TEXT NOT NULL,
			test_name TEXT NOT NULL,
			file_name TEXT NOT NULL,
			status TEXT NOT NULL,
			duration_millis INTEGER NOT NULL,
			failure_message TEXT NOT NULL DEFAULT '',
			failure_output TEXT NOT NULL DEFAULT '',
			system_out TEXT NOT NULL DEFAULT '',
			system_err TEXT NOT NULL DEFAULT '',
			regression INTEGER NOT NULL DEFAULT 0
		);`,
	} {
		if _, err := repo.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("seed legacy schema: %v", err)
		}
	}

	createdAt := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO projects (id, slug, name, created_at) VALUES (?, ?, ?, ?)
	`, "project-legacy", "legacy", "Legacy", formatTime(createdAt)); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO runs (
			id, project_id, branch, commit_sha, build_id, build_url, environment, run_label, status,
			started_at, uploaded_at, previous_run_id, total_count, passed_count, failed_count, skipped_count,
			duration_millis, new_failures
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "run-legacy", "project-legacy", "main", "", "", "", "", "legacy-run", "complete",
		formatTime(createdAt), formatTime(createdAt), "", 1, 0, 1, 0, 12, 0); err != nil {
		t.Fatalf("insert legacy run: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO test_results (
			id, run_id, project_id, test_key, suite_name, package_name, class_name, test_name, file_name,
			status, duration_millis, failure_message, failure_output, system_out, system_err, regression
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "result-legacy", "run-legacy", "project-legacy", "pkg::suite::TestLegacy", "suite", "pkg", "suite", "TestLegacy", "",
		"failed", 12, "legacy boom", "legacy stacktrace", "legacy stdout", "legacy stderr", 0); err != nil {
		t.Fatalf("insert legacy result: %v", err)
	}

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var storedOutputs int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM test_result_outputs`).Scan(&storedOutputs); err != nil {
		t.Fatalf("count backfilled outputs: %v", err)
	}
	if storedOutputs != 1 {
		t.Fatalf("expected 1 backfilled output row, got %d", storedOutputs)
	}

	var catalogCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tests WHERE project_id = ? AND test_key = ?`, "project-legacy", "pkg::suite::TestLegacy").Scan(&catalogCount); err != nil {
		t.Fatalf("count backfilled tests: %v", err)
	}
	if catalogCount != 1 {
		t.Fatalf("expected 1 backfilled normalized test row, got %d", catalogCount)
	}

	var testID string
	var projectID string
	var storedTestKey string
	if err := repo.db.QueryRowContext(ctx, `
		SELECT test_id, project_id, test_key
		FROM test_results
		WHERE id = ?
	`, "result-legacy").Scan(&testID, &projectID, &storedTestKey); err != nil {
		t.Fatalf("load migrated legacy result row: %v", err)
	}
	if testID == "" {
		t.Fatal("expected migrated legacy result to have a normalized test_id")
	}
	if projectID != "project-legacy" || storedTestKey != "" {
		t.Fatalf("expected migrated legacy duplicate fields to be cleared except project_id, got project=%q key=%q", projectID, storedTestKey)
	}

	results, err := repo.ListRunResults(ctx, "run-legacy")
	if err != nil {
		t.Fatalf("list backfilled results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 backfilled result, got %d", len(results))
	}
	if results[0].FailureOutput != "legacy stacktrace" || results[0].SystemOut != "legacy stdout" || results[0].SystemErr != "legacy stderr" {
		t.Fatalf("expected legacy outputs to survive migration, got %+v", results[0])
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
