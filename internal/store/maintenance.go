package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *SQLStore) Compact(ctx context.Context) error {
	if s.dialect != "sqlite" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("vacuum sqlite: %w", err)
	}
	return nil
}

func (s *SQLStore) PruneTestOutputs(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, s.rebind(`
		DELETE FROM test_result_outputs
		WHERE test_result_id IN (
			SELECT tr.id
			FROM test_results tr
			INNER JOIN runs r ON r.id = tr.run_id
			WHERE r.uploaded_at < ?
		)
	`), formatTime(before))
	if err != nil {
		return 0, fmt.Errorf("prune test outputs: %w", err)
	}
	if err := s.clearLegacyTestResultOutputsBefore(ctx, before); err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune test outputs: %w", err)
	}
	return rowsAffected, nil
}

func (s *SQLStore) backfillLegacyTestResultOutputs(ctx context.Context) error {
	legacyColumns := []string{"failure_output", "system_out", "system_err"}
	for _, column := range legacyColumns {
		exists, err := s.tableHasColumn(ctx, "test_results", column)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}

	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT id, failure_output, system_out, system_err
		FROM test_results
		WHERE id NOT IN (SELECT test_result_id FROM test_result_outputs)
			AND (failure_output <> '' OR system_out <> '' OR system_err <> '')
	`))
	if err != nil {
		return fmt.Errorf("load legacy test result outputs: %w", err)
	}
	defer rows.Close()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("backfill test result outputs: %w", err)
	}
	defer tx.Rollback()

	for rows.Next() {
		var result TestResult
		if err := rows.Scan(&result.ID, &result.FailureOutput, &result.SystemOut, &result.SystemErr); err != nil {
			return fmt.Errorf("scan legacy test result output: %w", err)
		}
		if err := s.insertTestResultOutput(ctx, tx, result); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load legacy test result outputs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("backfill test result outputs: %w", err)
	}
	if err := s.clearLegacyTestResultOutputs(ctx); err != nil {
		return err
	}
	return nil
}

func (s *SQLStore) clearLegacyTestResultOutputs(ctx context.Context) error {
	return s.clearLegacyTestResultOutputsWithWhere(ctx, `
		WHERE failure_output <> '' OR system_out <> '' OR system_err <> ''
	`)
}

func (s *SQLStore) clearLegacyTestResultOutputsBefore(ctx context.Context, before time.Time) error {
	return s.clearLegacyTestResultOutputsWithWhere(ctx, s.rebind(`
		WHERE id IN (
			SELECT tr.id
			FROM test_results tr
			INNER JOIN runs r ON r.id = tr.run_id
			WHERE r.uploaded_at < ?
		)
	`), formatTime(before))
}

func (s *SQLStore) clearLegacyTestResultOutputsWithWhere(ctx context.Context, whereClause string, args ...any) error {
	exists, err := s.tableHasColumn(ctx, "test_results", "failure_output")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	query := `
		UPDATE test_results
		SET failure_output = '', system_out = '', system_err = ''
	` + whereClause
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("clear legacy test result outputs: %w", err)
	}
	return nil
}

func (s *SQLStore) tableHasColumn(ctx context.Context, tableName, columnName string) (bool, error) {
	switch s.dialect {
	case "postgres":
		var exists bool
		if err := s.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = current_schema()
					AND table_name = $1
					AND column_name = $2
			)
		`, tableName, columnName).Scan(&exists); err != nil {
			return false, fmt.Errorf("inspect table column: %w", err)
		}
		return exists, nil
	default:
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, quoteSQLiteIdentifier(tableName)))
		if err != nil {
			return false, fmt.Errorf("inspect table column: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var cid int
			var name string
			var dataType string
			var notNull int
			var defaultValue sql.NullString
			var primaryKey int
			if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
				return false, fmt.Errorf("inspect table column: %w", err)
			}
			if strings.EqualFold(name, columnName) {
				return true, nil
			}
		}
		if err := rows.Err(); err != nil {
			return false, fmt.Errorf("inspect table column: %w", err)
		}
		return false, nil
	}
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
