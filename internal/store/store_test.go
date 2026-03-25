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

func newTestStore(t *testing.T) *SQLStore {
	t.Helper()
	repo, err := Open(context.Background(), filepath.Join(t.TempDir(), "testrr.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return repo
}
