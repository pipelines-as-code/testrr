package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

func outputBlobID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TruncateOutput(value string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 || int64(len(value)) <= maxBytes {
		return value, false
	}
	return value[:maxBytes], true
}

func formatStoredOutput(value string, truncated bool) string {
	if value == "" {
		return ""
	}
	if !truncated {
		return value
	}
	return value + "\n\n[testrr output truncated]"
}

func (s *SQLStore) ensureOutputBlobSchema(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS output_blobs (
			id TEXT PRIMARY KEY,
			compressed_data BLOB NOT NULL,
			original_size_bytes INTEGER NOT NULL
		);`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure output blob schema: %w", err)
		}
	}

	type columnSpec struct {
		name       string
		definition string
	}
	for _, spec := range []columnSpec{
		{name: "failure_output_blob_id", definition: `TEXT NOT NULL DEFAULT ''`},
		{name: "failure_output_truncated", definition: `INTEGER NOT NULL DEFAULT 0`},
		{name: "system_out_blob_id", definition: `TEXT NOT NULL DEFAULT ''`},
		{name: "system_out_truncated", definition: `INTEGER NOT NULL DEFAULT 0`},
		{name: "system_err_blob_id", definition: `TEXT NOT NULL DEFAULT ''`},
		{name: "system_err_truncated", definition: `INTEGER NOT NULL DEFAULT 0`},
	} {
		exists, err := s.tableHasColumn(ctx, "test_result_outputs", spec.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE test_result_outputs ADD COLUMN %s %s`, spec.name, spec.definition)); err != nil {
			return fmt.Errorf("add test_result_outputs.%s: %w", spec.name, err)
		}
	}

	return nil
}

func (s *SQLStore) upsertOutputBlob(ctx context.Context, db execer, value string) (string, error) {
	if value == "" {
		return "", nil
	}

	compressed, err := compressOutput(value)
	if err != nil {
		return "", err
	}
	blobID := outputBlobID(value)
	if _, err := db.ExecContext(ctx, s.rebind(`
		INSERT INTO output_blobs (id, compressed_data, original_size_bytes)
		VALUES (?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`), blobID, compressed, len(value)); err != nil {
		return "", fmt.Errorf("upsert output blob: %w", err)
	}
	return blobID, nil
}

func (s *SQLStore) insertTestResultOutput(ctx context.Context, db execer, result TestResult) error {
	if !hasStoredOutput(result) {
		return nil
	}

	failureBlobID, err := s.upsertOutputBlob(ctx, db, result.FailureOutput)
	if err != nil {
		return err
	}
	systemOutBlobID, err := s.upsertOutputBlob(ctx, db, result.SystemOut)
	if err != nil {
		return err
	}
	systemErrBlobID, err := s.upsertOutputBlob(ctx, db, result.SystemErr)
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, s.rebind(`
		INSERT INTO test_result_outputs (
			test_result_id,
			failure_output,
			system_out,
			system_err,
			failure_output_blob_id,
			failure_output_truncated,
			system_out_blob_id,
			system_out_truncated,
			system_err_blob_id,
			system_err_truncated
		)
		VALUES (?, NULL, NULL, NULL, ?, ?, ?, ?, ?, ?)
	`),
		result.ID,
		failureBlobID,
		boolToInt(result.FailureOutputTruncated),
		systemOutBlobID,
		boolToInt(result.SystemOutTruncated),
		systemErrBlobID,
		boolToInt(result.SystemErrTruncated),
	); err != nil {
		return fmt.Errorf("insert test result output: %w", err)
	}

	return nil
}
