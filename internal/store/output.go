package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
)

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func hasStoredOutput(result TestResult) bool {
	return result.FailureOutput != "" || result.SystemOut != "" || result.SystemErr != ""
}

func compressOutput(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}

	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write([]byte(value)); err != nil {
		return nil, fmt.Errorf("compress output: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("compress output: %w", err)
	}
	return buffer.Bytes(), nil
}

func decompressOutput(value []byte) (string, error) {
	if len(value) == 0 {
		return "", nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(value))
	if err != nil {
		return "", fmt.Errorf("decompress output: %w", err)
	}
	defer reader.Close()

	contents, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("decompress output: %w", err)
	}
	return string(contents), nil
}

func (s *SQLStore) insertTestResultOutput(ctx context.Context, db execer, result TestResult) error {
	if !hasStoredOutput(result) {
		return nil
	}

	failureOutput, err := compressOutput(result.FailureOutput)
	if err != nil {
		return err
	}
	systemOut, err := compressOutput(result.SystemOut)
	if err != nil {
		return err
	}
	systemErr, err := compressOutput(result.SystemErr)
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, s.rebind(`
		INSERT INTO test_result_outputs (test_result_id, failure_output, system_out, system_err)
		VALUES (?, ?, ?, ?)
	`), result.ID, failureOutput, systemOut, systemErr); err != nil {
		return fmt.Errorf("insert test result output: %w", err)
	}

	return nil
}
