package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

func stableTestID(projectID, testKey string) string {
	sum := sha256.Sum256([]byte(projectID + "\x00" + testKey))
	return hex.EncodeToString(sum[:16])
}

func (s *SQLStore) ensureNormalizedTestCatalog(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS tests (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			test_key TEXT NOT NULL,
			suite_name TEXT NOT NULL,
			package_name TEXT NOT NULL,
			class_name TEXT NOT NULL,
			test_name TEXT NOT NULL,
			file_name TEXT NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS tests_project_key_idx ON tests(project_id, test_key);`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure normalized test catalog: %w", err)
		}
	}

	hasTestID, err := s.tableHasColumn(ctx, "test_results", "test_id")
	if err != nil {
		return err
	}
	if !hasTestID {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE test_results ADD COLUMN test_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add test_results.test_id: %w", err)
		}
	}

	if _, err := s.db.ExecContext(ctx, `DROP INDEX IF EXISTS test_results_project_key_idx`); err != nil {
		return fmt.Errorf("drop legacy test_results project key index: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS test_results_test_id_idx ON test_results(test_id)`); err != nil {
		return fmt.Errorf("create test_results test_id index: %w", err)
	}

	if err := s.backfillNormalizedTestCatalog(ctx); err != nil {
		return err
	}
	return nil
}

func (s *SQLStore) backfillNormalizedTestCatalog(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT project_id, test_key, suite_name, package_name, class_name, test_name, file_name
		FROM test_results
		WHERE test_id = '' AND test_key <> ''
	`))
	if err != nil {
		return fmt.Errorf("load legacy test catalog rows: %w", err)
	}
	defer rows.Close()

	type legacyCatalogRow struct {
		ProjectID   string
		TestKey     string
		SuiteName   string
		PackageName string
		ClassName   string
		TestName    string
		FileName    string
	}

	items := make([]legacyCatalogRow, 0)
	for rows.Next() {
		var item legacyCatalogRow
		if err := rows.Scan(
			&item.ProjectID,
			&item.TestKey,
			&item.SuiteName,
			&item.PackageName,
			&item.ClassName,
			&item.TestName,
			&item.FileName,
		); err != nil {
			return fmt.Errorf("scan legacy test catalog row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load legacy test catalog rows: %w", err)
	}
	if len(items) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("backfill normalized test catalog: %w", err)
	}
	defer tx.Rollback()

	for _, item := range items {
		if err := s.upsertTestCatalogEntry(ctx, tx, TestResult{
			ProjectID:   item.ProjectID,
			TestKey:     item.TestKey,
			SuiteName:   item.SuiteName,
			PackageName: item.PackageName,
			ClassName:   item.ClassName,
			TestName:    item.TestName,
			FileName:    item.FileName,
		}); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE test_results
		SET test_id = (
			SELECT t.id
			FROM tests t
			WHERE t.project_id = test_results.project_id
				AND t.test_key = test_results.test_key
		)
		WHERE test_id = '' AND test_key <> ''
	`)); err != nil {
		return fmt.Errorf("backfill test_results.test_id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE test_results
		SET test_key = '',
			suite_name = '',
			package_name = '',
			class_name = '',
			test_name = '',
			file_name = ''
		WHERE test_id <> ''
			AND (
				test_key <> ''
				OR suite_name <> ''
				OR package_name <> ''
				OR class_name <> ''
				OR test_name <> ''
				OR file_name <> ''
			)
	`); err != nil {
		return fmt.Errorf("clear legacy duplicated test columns: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("backfill normalized test catalog: %w", err)
	}
	return nil
}

func (s *SQLStore) upsertTestCatalogEntry(ctx context.Context, db execer, result TestResult) error {
	if result.ProjectID == "" || result.TestKey == "" {
		return nil
	}

	if _, err := db.ExecContext(ctx, s.rebind(`
		INSERT INTO tests (id, project_id, test_key, suite_name, package_name, class_name, test_name, file_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, test_key) DO UPDATE SET
			suite_name = COALESCE(NULLIF(EXCLUDED.suite_name, ''), tests.suite_name),
			package_name = COALESCE(NULLIF(EXCLUDED.package_name, ''), tests.package_name),
			class_name = COALESCE(NULLIF(EXCLUDED.class_name, ''), tests.class_name),
			test_name = COALESCE(NULLIF(EXCLUDED.test_name, ''), tests.test_name),
			file_name = COALESCE(NULLIF(EXCLUDED.file_name, ''), tests.file_name)
	`),
		stableTestID(result.ProjectID, result.TestKey),
		result.ProjectID,
		result.TestKey,
		result.SuiteName,
		result.PackageName,
		result.ClassName,
		result.TestName,
		result.FileName,
	); err != nil {
		return fmt.Errorf("upsert test catalog entry: %w", err)
	}

	return nil
}

func scanTestResultRow(
	row interface{ Scan(...any) error },
	includeOutputs bool,
) (TestResult, error) {
	var result TestResult
	var regression int
	var failureOutput sql.Null[[]byte]
	var failureOutputTruncated int
	var systemOut sql.Null[[]byte]
	var systemOutTruncated int
	var systemErr sql.Null[[]byte]
	var systemErrTruncated int

	destinations := []any{
		&result.ID,
		&result.RunID,
		&result.ProjectID,
		&result.TestKey,
		&result.SuiteName,
		&result.PackageName,
		&result.ClassName,
		&result.TestName,
		&result.FileName,
		&result.Status,
		&result.DurationMillis,
		&result.FailureMessage,
	}
	if includeOutputs {
		destinations = append(destinations, &failureOutput, &failureOutputTruncated, &systemOut, &systemOutTruncated, &systemErr, &systemErrTruncated)
	}
	destinations = append(destinations, &regression)

	if err := row.Scan(destinations...); err != nil {
		if err == sql.ErrNoRows {
			return TestResult{}, ErrNotFound
		}
		return TestResult{}, err
	}
	if includeOutputs {
		var err error
		if failureOutput.Valid {
			if result.FailureOutput, err = decompressOutput(failureOutput.V); err != nil {
				return TestResult{}, err
			}
		}
		result.FailureOutput = formatStoredOutput(result.FailureOutput, failureOutputTruncated == 1)
		if systemOut.Valid {
			if result.SystemOut, err = decompressOutput(systemOut.V); err != nil {
				return TestResult{}, err
			}
		}
		result.SystemOut = formatStoredOutput(result.SystemOut, systemOutTruncated == 1)
		if systemErr.Valid {
			if result.SystemErr, err = decompressOutput(systemErr.V); err != nil {
				return TestResult{}, err
			}
		}
		result.SystemErr = formatStoredOutput(result.SystemErr, systemErrTruncated == 1)
	}
	result.Regression = regression == 1
	return result, nil
}
