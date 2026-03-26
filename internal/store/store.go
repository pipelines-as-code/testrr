package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Close() error
	Migrate(context.Context) error
	Compact(context.Context) error
	CreateProject(context.Context, CreateProjectInput) (Project, error)
	RotateProjectCredential(context.Context, string, string, string) error
	ListProjects(context.Context) ([]Project, error)
	GetProjectBySlug(context.Context, string) (Project, error)
	GetProjectCredential(context.Context, string, string) (ProjectCredential, error)
	FindPreviousRun(context.Context, string, string) (*Run, error)
	CreateRun(context.Context, CreateRunInput) (Run, error)
	ListRunsByProject(context.Context, string, string, int) ([]Run, error)
	GetRun(context.Context, string, string) (Run, error)
	ListRunResults(context.Context, string) ([]TestResult, error)
	GetDashboardData(context.Context, string, string, int) (DashboardData, error)
	GetChartSummary(context.Context, string, string, int) (ChartSummary, error)
	GetTestHistory(context.Context, string, string, int) ([]TestHistoryEntry, error)
	GetTestDurationChart(context.Context, string, string, int) (TestDurationChart, error)
	GetRecentTestStatuses(context.Context, string, string, []string, int) (map[string][]string, error)
	DeleteProject(context.Context, string) error
	GetFlakyTests(context.Context, string, string, int) ([]FlakyTest, error)
	GetSlowestTests(context.Context, string, string, int) ([]SlowTest, error)
	PruneTestOutputs(context.Context, time.Time) (int64, error)
	PruneRuns(context.Context, time.Time) (int64, error)
}

type SQLStore struct {
	db      *sql.DB
	dialect string
}

type CreateProjectInput struct {
	ID           string
	Slug         string
	Name         string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type CreateRunInput struct {
	Run             Run
	Artifacts       []Artifact
	TestResults     []TestResult
	PreviousRunID   string
	NewFailures     int
	PreviousResults map[string]string
}

type Project struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectCredential struct {
	Project      Project
	Username     string
	PasswordHash string
	UpdatedAt    time.Time
}

type Run struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	Branch         string    `json:"branch"`
	CommitSHA      string    `json:"commit_sha"`
	BuildID        string    `json:"build_id"`
	BuildURL       string    `json:"build_url"`
	Environment    string    `json:"environment"`
	RunLabel       string    `json:"run_label"`
	Status         string    `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	UploadedAt     time.Time `json:"uploaded_at"`
	PreviousRunID  string    `json:"previous_run_id"`
	TotalCount     int       `json:"total_count"`
	PassedCount    int       `json:"passed_count"`
	FailedCount    int       `json:"failed_count"`
	SkippedCount   int       `json:"skipped_count"`
	DurationMillis int64     `json:"duration_millis"`
	NewFailures    int       `json:"new_failures"`
}

type Artifact struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	FileName    string `json:"file_name"`
	Format      string `json:"format"`
	FilePath    string `json:"file_path"`
	Checksum    string `json:"checksum"`
	SizeBytes   int64  `json:"size_bytes"`
	ParseStatus string `json:"parse_status"`
	ParseError  string `json:"parse_error"`
}

type TestResult struct {
	ID                     string `json:"id"`
	RunID                  string `json:"run_id"`
	ProjectID              string `json:"project_id"`
	TestKey                string `json:"test_key"`
	SuiteName              string `json:"suite_name"`
	PackageName            string `json:"package_name"`
	ClassName              string `json:"class_name"`
	TestName               string `json:"test_name"`
	FileName               string `json:"file_name"`
	Status                 string `json:"status"`
	DurationMillis         int64  `json:"duration_millis"`
	FailureMessage         string `json:"failure_message"`
	FailureOutput          string `json:"failure_output"`
	SystemOut              string `json:"system_out"`
	SystemErr              string `json:"system_err"`
	Regression             bool   `json:"regression"`
	FailureOutputTruncated bool   `json:"-"`
	SystemOutTruncated     bool   `json:"-"`
	SystemErrTruncated     bool   `json:"-"`
}

type DashboardData struct {
	Latest        *Run          `json:"latest,omitempty"`
	RecentRuns    []Run         `json:"recent_runs"`
	TopFailing    []FailingTest `json:"top_failing"`
	TopFlaky      []FlakyTest   `json:"top_flaky"`
	SlowestTests  []SlowTest    `json:"slowest_tests"`
	TotalRuns     int           `json:"total_runs"`
	TotalFailures int           `json:"total_failures"`
}

type FailingTest struct {
	TestKey      string `json:"test_key"`
	DisplayName  string `json:"display_name"`
	FailureCount int    `json:"failure_count"`
}

type FlakyTest struct {
	TestKey         string `json:"test_key"`
	DisplayName     string `json:"display_name"`
	TransitionCount int    `json:"transition_count"`
	FailureCount    int    `json:"failure_count"`
	TotalCount      int    `json:"total_count"`
}

type SlowTest struct {
	TestKey               string `json:"test_key"`
	DisplayName           string `json:"display_name"`
	AverageDurationMillis int64  `json:"average_duration_millis"`
	SampleCount           int    `json:"sample_count"`
}

type ChartSummary struct {
	Labels    []string  `json:"labels"`
	PassRates []float64 `json:"pass_rates"`
	Failures  []int     `json:"failures"`
	Durations []int64   `json:"durations"`
}

type TestHistoryEntry struct {
	RunID                  string    `json:"run_id"`
	RunLabel               string    `json:"run_label"`
	Branch                 string    `json:"branch"`
	Status                 string    `json:"status"`
	DurationMillis         int64     `json:"duration_millis"`
	UploadedAt             time.Time `json:"uploaded_at"`
	FailureMessage         string    `json:"failure_message"`
	FailureOutput          string    `json:"failure_output"`
	SystemOut              string    `json:"system_out"`
	SystemErr              string    `json:"system_err"`
	FailureOutputTruncated bool      `json:"-"`
	SystemOutTruncated     bool      `json:"-"`
	SystemErrTruncated     bool      `json:"-"`
}

type TestDurationChart struct {
	Labels    []string `json:"labels"`
	Durations []int64  `json:"durations"`
	Statuses  []string `json:"statuses"`
}

func Open(ctx context.Context, databaseURL string) (*SQLStore, error) {
	dialect, dsn := detectDialect(databaseURL)
	if dialect == "sqlite" {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite dir: %w", err)
		}
		dsn = fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", filepath.ToSlash(dsn))
	}

	db, err := sql.Open(driverName(dialect), dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if dialect == "sqlite" {
		for _, statement := range []string{
			`PRAGMA journal_mode=WAL;`,
			`PRAGMA synchronous=NORMAL;`,
			`PRAGMA busy_timeout=5000;`,
			`PRAGMA wal_autocheckpoint=1000;`,
		} {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				db.Close()
				return nil, fmt.Errorf("configure sqlite: %w", err)
			}
		}
	}

	return &SQLStore{db: db, dialect: dialect}, nil
}

func detectDialect(databaseURL string) (string, string) {
	lower := strings.ToLower(databaseURL)
	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		return "postgres", databaseURL
	default:
		return "sqlite", databaseURL
	}
}

func driverName(dialect string) string {
	if dialect == "postgres" {
		return "pgx"
	}
	return "sqlite"
}

func (s *SQLStore) Close() error {
	return s.db.Close()
}

func (s *SQLStore) Migrate(ctx context.Context) error {
	var statements []string
	if s.dialect == "postgres" {
		statements = postgresMigrations
	} else {
		statements = sqliteMigrations
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("run migration: %w", err)
		}
	}
	if err := s.ensureOutputBlobSchema(ctx); err != nil {
		return err
	}
	if err := s.ensureNormalizedTestCatalog(ctx); err != nil {
		return err
	}
	if err := s.backfillLegacyTestResultOutputs(ctx); err != nil {
		return err
	}
	return nil
}

func (s *SQLStore) CreateProject(ctx context.Context, input CreateProjectInput) (Project, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO projects (id, slug, name, created_at)
		VALUES (?, ?, ?, ?)
	`), input.ID, input.Slug, input.Name, formatTime(input.CreatedAt)); err != nil {
		return Project{}, fmt.Errorf("insert project: %w", err)
	}

	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO project_credentials (project_id, username, password_hash, updated_at)
		VALUES (?, ?, ?, ?)
	`), input.ID, input.Username, input.PasswordHash, formatTime(input.CreatedAt)); err != nil {
		return Project{}, fmt.Errorf("insert project credential: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Project{}, err
	}

	return Project{
		ID:        input.ID,
		Slug:      input.Slug,
		Name:      input.Name,
		CreatedAt: input.CreatedAt,
	}, nil
}

func (s *SQLStore) RotateProjectCredential(ctx context.Context, slug, username, passwordHash string) error {
	project, err := s.GetProjectBySlug(ctx, slug)
	if err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, s.rebind(`
		UPDATE project_credentials
		SET username = ?, password_hash = ?, updated_at = ?
		WHERE project_id = ?
	`), username, passwordHash, formatTime(time.Now().UTC()), project.ID)
	if err != nil {
		return fmt.Errorf("rotate credential: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, slug, name, created_at
		FROM projects
		ORDER BY slug
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		var project Project
		var createdAt string
		if err := rows.Scan(&project.ID, &project.Slug, &project.Name, &createdAt); err != nil {
			return nil, err
		}
		project.CreatedAt = parseTime(createdAt)
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *SQLStore) GetProjectBySlug(ctx context.Context, slug string) (Project, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, slug, name, created_at
		FROM projects
		WHERE slug = ?
	`), slug)

	var project Project
	var createdAt string
	if err := row.Scan(&project.ID, &project.Slug, &project.Name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, ErrNotFound
		}
		return Project{}, err
	}
	project.CreatedAt = parseTime(createdAt)
	return project, nil
}

func (s *SQLStore) DeleteProject(ctx context.Context, slug string) error {
	project, err := s.GetProjectBySlug(ctx, slug)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.rebind(`DELETE FROM projects WHERE id = ?`), project.ID)
	return err
}

func (s *SQLStore) GetProjectCredential(ctx context.Context, slug, username string) (ProjectCredential, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT p.id, p.slug, p.name, p.created_at, c.username, c.password_hash, c.updated_at
		FROM project_credentials c
		INNER JOIN projects p ON p.id = c.project_id
		WHERE p.slug = ? AND c.username = ?
	`), slug, username)

	var credential ProjectCredential
	var project Project
	var createdAt, updatedAt string
	if err := row.Scan(&project.ID, &project.Slug, &project.Name, &createdAt, &credential.Username, &credential.PasswordHash, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectCredential{}, ErrNotFound
		}
		return ProjectCredential{}, err
	}
	project.CreatedAt = parseTime(createdAt)
	credential.Project = project
	credential.UpdatedAt = parseTime(updatedAt)
	return credential, nil
}

func (s *SQLStore) FindPreviousRun(ctx context.Context, projectID, branch string) (*Run, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, project_id, branch, commit_sha, build_id, build_url, environment, run_label, status, started_at, uploaded_at, previous_run_id, total_count, passed_count, failed_count, skipped_count, duration_millis, new_failures
		FROM runs
		WHERE project_id = ? AND branch = ? AND status = 'complete'
		ORDER BY uploaded_at DESC
		LIMIT 1
	`), projectID, branch)

	run, err := scanRun(row)
	if err == nil {
		return &run, nil
	}
	if !errors.Is(err, ErrNotFound) || branch == "" {
		return nil, err
	}

	row = s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, project_id, branch, commit_sha, build_id, build_url, environment, run_label, status, started_at, uploaded_at, previous_run_id, total_count, passed_count, failed_count, skipped_count, duration_millis, new_failures
		FROM runs
		WHERE project_id = ? AND status = 'complete'
		ORDER BY uploaded_at DESC
		LIMIT 1
	`), projectID)
	run, err = scanRun(row)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *SQLStore) CreateRun(ctx context.Context, input CreateRunInput) (Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO runs (
			id, project_id, branch, commit_sha, build_id, build_url, environment, run_label, status,
			started_at, uploaded_at, previous_run_id, total_count, passed_count, failed_count, skipped_count,
			duration_millis, new_failures
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		input.Run.ID,
		input.Run.ProjectID,
		input.Run.Branch,
		input.Run.CommitSHA,
		input.Run.BuildID,
		input.Run.BuildURL,
		input.Run.Environment,
		input.Run.RunLabel,
		input.Run.Status,
		formatTime(input.Run.StartedAt),
		formatTime(input.Run.UploadedAt),
		input.Run.PreviousRunID,
		input.Run.TotalCount,
		input.Run.PassedCount,
		input.Run.FailedCount,
		input.Run.SkippedCount,
		input.Run.DurationMillis,
		input.Run.NewFailures,
	); err != nil {
		return Run{}, fmt.Errorf("insert run: %w", err)
	}

	for _, artifact := range input.Artifacts {
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO run_artifacts (id, run_id, file_name, format_name, file_path, checksum, size_bytes, parse_status, parse_error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), artifact.ID, artifact.RunID, artifact.FileName, artifact.Format, artifact.FilePath, artifact.Checksum, artifact.SizeBytes, artifact.ParseStatus, artifact.ParseError); err != nil {
			return Run{}, fmt.Errorf("insert artifact: %w", err)
		}
	}

	for _, result := range input.TestResults {
		if err := s.upsertTestCatalogEntry(ctx, tx, result); err != nil {
			return Run{}, err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO test_results (
				id, run_id, test_id, project_id, test_key, suite_name, package_name, class_name, test_name, file_name,
				status, duration_millis, failure_message, regression
			) VALUES (?, ?, ?, ?, '', '', '', '', '', '', ?, ?, ?, ?)
		`),
			result.ID,
			result.RunID,
			stableTestID(result.ProjectID, result.TestKey),
			result.ProjectID,
			result.Status,
			result.DurationMillis,
			result.FailureMessage,
			boolToInt(result.Regression),
		); err != nil {
			return Run{}, fmt.Errorf("insert test result: %w", err)
		}
		if err := s.insertTestResultOutput(ctx, tx, result); err != nil {
			return Run{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return input.Run, nil
}

func (s *SQLStore) ListRunsByProject(ctx context.Context, projectID, branch string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, project_id, branch, commit_sha, build_id, build_url, environment, run_label, status, started_at, uploaded_at, previous_run_id, total_count, passed_count, failed_count, skipped_count, duration_millis, new_failures
		FROM runs
		WHERE project_id = ? AND (? = '' OR branch = ?)
		ORDER BY uploaded_at DESC
		LIMIT ?
	`), projectID, branch, branch, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (s *SQLStore) GetRun(ctx context.Context, projectID, runID string) (Run, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT id, project_id, branch, commit_sha, build_id, build_url, environment, run_label, status, started_at, uploaded_at, previous_run_id, total_count, passed_count, failed_count, skipped_count, duration_millis, new_failures
		FROM runs
		WHERE project_id = ? AND id = ?
	`), projectID, runID)
	return scanRun(row)
}

func (s *SQLStore) ListRunResults(ctx context.Context, runID string) ([]TestResult, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT tr.id, tr.run_id, t.project_id, t.test_key, t.suite_name, t.package_name, t.class_name, t.test_name, t.file_name,
			tr.status, tr.duration_millis, tr.failure_message,
			COALESCE(fob.compressed_data, tro.failure_output),
			COALESCE(tro.failure_output_truncated, 0),
			COALESCE(sob.compressed_data, tro.system_out),
			COALESCE(tro.system_out_truncated, 0),
			COALESCE(seb.compressed_data, tro.system_err),
			COALESCE(tro.system_err_truncated, 0),
			tr.regression
		FROM test_results tr
		INNER JOIN tests t ON t.id = tr.test_id
		LEFT JOIN test_result_outputs tro ON tro.test_result_id = tr.id
		LEFT JOIN output_blobs fob ON fob.id = tro.failure_output_blob_id
		LEFT JOIN output_blobs sob ON sob.id = tro.system_out_blob_id
		LEFT JOIN output_blobs seb ON seb.id = tro.system_err_blob_id
		WHERE tr.run_id = ?
		ORDER BY tr.status DESC, t.test_name
	`), runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]TestResult, 0)
	for rows.Next() {
		result, err := scanTestResultRow(rows, true)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *SQLStore) GetDashboardData(ctx context.Context, projectID, branch string, limit int) (DashboardData, error) {
	runs, err := s.ListRunsByProject(ctx, projectID, branch, limit)
	if err != nil {
		return DashboardData{}, err
	}

	data := DashboardData{RecentRuns: runs}
	if len(runs) > 0 {
		data.Latest = &runs[0]
	}
	data.TotalRuns = len(runs)
	for _, run := range runs {
		data.TotalFailures += run.FailedCount
	}

	topFailing, err := s.getTopFailingTests(ctx, projectID, branch, 5)
	if err != nil {
		return DashboardData{}, err
	}
	data.TopFailing = topFailing

	topFlaky, err := s.GetFlakyTests(ctx, projectID, branch, 5)
	if err != nil {
		return DashboardData{}, err
	}
	data.TopFlaky = topFlaky

	slowest, err := s.GetSlowestTests(ctx, projectID, branch, 5)
	if err != nil {
		return DashboardData{}, err
	}
	data.SlowestTests = slowest

	return data, nil
}

func (s *SQLStore) GetChartSummary(ctx context.Context, projectID, branch string, limit int) (ChartSummary, error) {
	runs, err := s.ListRunsByProject(ctx, projectID, branch, limit)
	if err != nil {
		return ChartSummary{}, err
	}

	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
	}

	summary := ChartSummary{
		Labels:    make([]string, 0, len(runs)),
		PassRates: make([]float64, 0, len(runs)),
		Failures:  make([]int, 0, len(runs)),
		Durations: make([]int64, 0, len(runs)),
	}
	for _, run := range runs {
		label := run.RunLabel
		if label == "" {
			label = run.UploadedAt.Format("2006-01-02 15:04")
		}
		summary.Labels = append(summary.Labels, label)
		if run.TotalCount == 0 {
			summary.PassRates = append(summary.PassRates, 0)
		} else {
			summary.PassRates = append(summary.PassRates, float64(run.PassedCount)/float64(run.TotalCount)*100)
		}
		summary.Failures = append(summary.Failures, run.FailedCount)
		summary.Durations = append(summary.Durations, run.DurationMillis)
	}
	return summary, nil
}

func (s *SQLStore) GetTestHistory(ctx context.Context, projectID, testKey string, limit int) ([]TestHistoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT
			r.id,
			r.run_label,
			r.branch,
			tr.status,
			tr.duration_millis,
			r.uploaded_at,
			tr.failure_message,
			COALESCE(fob.compressed_data, tro.failure_output),
			COALESCE(tro.failure_output_truncated, 0),
			COALESCE(sob.compressed_data, tro.system_out),
			COALESCE(tro.system_out_truncated, 0),
			COALESCE(seb.compressed_data, tro.system_err),
			COALESCE(tro.system_err_truncated, 0)
		FROM test_results tr
		INNER JOIN tests t ON t.id = tr.test_id
		INNER JOIN runs r ON r.id = tr.run_id
		LEFT JOIN test_result_outputs tro ON tro.test_result_id = tr.id
		LEFT JOIN output_blobs fob ON fob.id = tro.failure_output_blob_id
		LEFT JOIN output_blobs sob ON sob.id = tro.system_out_blob_id
		LEFT JOIN output_blobs seb ON seb.id = tro.system_err_blob_id
		WHERE t.project_id = ? AND t.test_key = ?
		ORDER BY r.uploaded_at DESC
		LIMIT ?
	`), projectID, testKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]TestHistoryEntry, 0)
	for rows.Next() {
		var entry TestHistoryEntry
		var uploadedAt string
		var failureOutput sql.Null[[]byte]
		var failureOutputTruncated int
		var systemOut sql.Null[[]byte]
		var systemOutTruncated int
		var systemErr sql.Null[[]byte]
		var systemErrTruncated int
		if err := rows.Scan(
			&entry.RunID,
			&entry.RunLabel,
			&entry.Branch,
			&entry.Status,
			&entry.DurationMillis,
			&uploadedAt,
			&entry.FailureMessage,
			&failureOutput,
			&failureOutputTruncated,
			&systemOut,
			&systemOutTruncated,
			&systemErr,
			&systemErrTruncated,
		); err != nil {
			return nil, err
		}
		entry.UploadedAt = parseTime(uploadedAt)
		if failureOutput.Valid {
			if entry.FailureOutput, err = decompressOutput(failureOutput.V); err != nil {
				return nil, err
			}
		}
		entry.FailureOutput = formatStoredOutput(entry.FailureOutput, failureOutputTruncated == 1)
		if systemOut.Valid {
			if entry.SystemOut, err = decompressOutput(systemOut.V); err != nil {
				return nil, err
			}
		}
		entry.SystemOut = formatStoredOutput(entry.SystemOut, systemOutTruncated == 1)
		if systemErr.Valid {
			if entry.SystemErr, err = decompressOutput(systemErr.V); err != nil {
				return nil, err
			}
		}
		entry.SystemErr = formatStoredOutput(entry.SystemErr, systemErrTruncated == 1)
		history = append(history, entry)
	}
	return history, rows.Err()
}

func (s *SQLStore) rebind(query string) string {
	if s.dialect != "postgres" {
		return query
	}
	var builder strings.Builder
	index := 1
	for _, char := range query {
		if char == '?' {
			builder.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var run Run
	var startedAt, uploadedAt string
	if err := row.Scan(
		&run.ID,
		&run.ProjectID,
		&run.Branch,
		&run.CommitSHA,
		&run.BuildID,
		&run.BuildURL,
		&run.Environment,
		&run.RunLabel,
		&run.Status,
		&startedAt,
		&uploadedAt,
		&run.PreviousRunID,
		&run.TotalCount,
		&run.PassedCount,
		&run.FailedCount,
		&run.SkippedCount,
		&run.DurationMillis,
		&run.NewFailures,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	run.StartedAt = parseTime(startedAt)
	run.UploadedAt = parseTime(uploadedAt)
	return run, nil
}

func scanRuns(rows *sql.Rows) ([]Run, error) {
	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

var sqliteMigrations = []string{
	`CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		slug TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS project_credentials (
		project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
		username TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runs (
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
	`CREATE INDEX IF NOT EXISTS runs_project_uploaded_idx ON runs(project_id, uploaded_at DESC);`,
	`CREATE TABLE IF NOT EXISTS run_artifacts (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
		file_name TEXT NOT NULL,
		format_name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		checksum TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		parse_status TEXT NOT NULL,
		parse_error TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS test_results (
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
		regression INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS test_result_outputs (
		test_result_id TEXT PRIMARY KEY REFERENCES test_results(id) ON DELETE CASCADE,
		failure_output BLOB,
		system_out BLOB,
		system_err BLOB
	);`,
	`CREATE INDEX IF NOT EXISTS test_results_run_idx ON test_results(run_id);`,
	`CREATE INDEX IF NOT EXISTS test_results_project_key_idx ON test_results(project_id, test_key);`,
}

var postgresMigrations = []string{
	`CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		slug TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS project_credentials (
		project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
		username TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS runs (
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
		duration_millis BIGINT NOT NULL,
		new_failures INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE INDEX IF NOT EXISTS runs_project_uploaded_idx ON runs(project_id, uploaded_at DESC);`,
	`CREATE TABLE IF NOT EXISTS run_artifacts (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
		file_name TEXT NOT NULL,
		format_name TEXT NOT NULL,
		file_path TEXT NOT NULL,
		checksum TEXT NOT NULL,
		size_bytes BIGINT NOT NULL,
		parse_status TEXT NOT NULL,
		parse_error TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS test_results (
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
		duration_millis BIGINT NOT NULL,
		failure_message TEXT NOT NULL DEFAULT '',
		regression INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS test_result_outputs (
		test_result_id TEXT PRIMARY KEY REFERENCES test_results(id) ON DELETE CASCADE,
		failure_output BYTEA,
		system_out BYTEA,
		system_err BYTEA
	);`,
	`CREATE INDEX IF NOT EXISTS test_results_run_idx ON test_results(run_id);`,
	`CREATE INDEX IF NOT EXISTS test_results_project_key_idx ON test_results(project_id, test_key);`,
}
