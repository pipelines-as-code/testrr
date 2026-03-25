package store

import (
	"context"
	"fmt"
	"math"
	"strings"
)

const recentTestObservationLimit = 30

func (s *SQLStore) getTopFailingTests(ctx context.Context, projectID, branch string, limit int) ([]FailingTest, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		SELECT tr.test_key, COALESCE(MAX(NULLIF(tr.test_name, '')), tr.test_key) AS display_name, COUNT(*)
		FROM test_results tr
		INNER JOIN runs r ON r.id = tr.run_id
		WHERE tr.project_id = ? AND tr.status = 'failed' AND (? = '' OR r.branch = ?)
		GROUP BY tr.test_key
		ORDER BY COUNT(*) DESC, tr.test_key
		LIMIT ?
	`), projectID, branch, branch, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]FailingTest, 0, limit)
	for rows.Next() {
		var item FailingTest
		if err := rows.Scan(&item.TestKey, &item.DisplayName, &item.FailureCount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *SQLStore) GetFlakyTests(ctx context.Context, projectID, branch string, limit int) ([]FlakyTest, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		WITH recent AS (
			SELECT
				tr.test_key,
				COALESCE(NULLIF(tr.test_name, ''), tr.test_key) AS display_name,
				tr.status,
				r.uploaded_at,
				ROW_NUMBER() OVER (PARTITION BY tr.test_key ORDER BY r.uploaded_at DESC) AS rn
			FROM test_results tr
			INNER JOIN runs r ON r.id = tr.run_id
			WHERE tr.project_id = ? AND (? = '' OR r.branch = ?) AND tr.status IN ('passed', 'failed')
		),
		windowed AS (
			SELECT test_key, display_name, status, uploaded_at
			FROM recent
			WHERE rn <= ?
		),
		transitions AS (
			SELECT
				test_key,
				display_name,
				status,
				LAG(status) OVER (PARTITION BY test_key ORDER BY uploaded_at ASC) AS previous_status
			FROM windowed
		),
		scored AS (
			SELECT
				test_key,
				MAX(display_name) AS display_name,
				SUM(CASE WHEN previous_status IS NOT NULL AND previous_status <> status THEN 1 ELSE 0 END) AS transition_count,
				SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failure_count,
				COUNT(*) AS total_count
			FROM transitions
			GROUP BY test_key
			HAVING COUNT(DISTINCT status) = 2
		)
		SELECT
			test_key,
			display_name,
			transition_count,
			failure_count,
			total_count
		FROM scored
		WHERE transition_count > 1
		ORDER BY transition_count DESC, failure_count DESC, test_key
		LIMIT ?
	`), projectID, branch, branch, recentTestObservationLimit, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]FlakyTest, 0, limit)
	for rows.Next() {
		var item FlakyTest
		if err := rows.Scan(&item.TestKey, &item.DisplayName, &item.TransitionCount, &item.FailureCount, &item.TotalCount); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *SQLStore) GetSlowestTests(ctx context.Context, projectID, branch string, limit int) ([]SlowTest, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		WITH recent AS (
			SELECT
				tr.test_key,
				COALESCE(NULLIF(tr.test_name, ''), tr.test_key) AS display_name,
				tr.duration_millis,
				r.uploaded_at,
				ROW_NUMBER() OVER (PARTITION BY tr.test_key ORDER BY r.uploaded_at DESC) AS rn
			FROM test_results tr
			INNER JOIN runs r ON r.id = tr.run_id
			WHERE tr.project_id = ? AND (? = '' OR r.branch = ?) AND tr.duration_millis > 0
		)
		SELECT
			test_key,
			MAX(display_name) AS display_name,
			AVG(duration_millis) AS average_duration,
			COUNT(*) AS sample_count
		FROM recent
		WHERE rn <= ?
		GROUP BY test_key
		ORDER BY average_duration DESC, sample_count DESC, test_key
		LIMIT ?
	`), projectID, branch, branch, recentTestObservationLimit, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SlowTest, 0, limit)
	for rows.Next() {
		var item SlowTest
		var average float64
		if err := rows.Scan(&item.TestKey, &item.DisplayName, &average, &item.SampleCount); err != nil {
			return nil, err
		}
		item.AverageDurationMillis = int64(math.Round(average))
		results = append(results, item)
	}
	return results, rows.Err()
}

func (s *SQLStore) GetTestDurationChart(ctx context.Context, projectID, testKey string, limit int) (TestDurationChart, error) {
	rows, err := s.db.QueryContext(ctx, s.rebind(`
		WITH recent AS (
			SELECT r.run_label, r.uploaded_at, tr.duration_millis, tr.status
			FROM test_results tr
			INNER JOIN runs r ON r.id = tr.run_id
			WHERE tr.project_id = ? AND tr.test_key = ?
			ORDER BY r.uploaded_at DESC
			LIMIT ?
		)
		SELECT run_label, uploaded_at, duration_millis, status
		FROM recent
		ORDER BY uploaded_at ASC
	`), projectID, testKey, limit)
	if err != nil {
		return TestDurationChart{}, err
	}
	defer rows.Close()

	chart := TestDurationChart{
		Labels:    make([]string, 0, limit),
		Durations: make([]int64, 0, limit),
		Statuses:  make([]string, 0, limit),
	}
	for rows.Next() {
		var runLabel string
		var uploadedAt string
		var duration int64
		var status string
		if err := rows.Scan(&runLabel, &uploadedAt, &duration, &status); err != nil {
			return TestDurationChart{}, err
		}
		label := strings.TrimSpace(runLabel)
		if label == "" {
			label = parseTime(uploadedAt).Format("2006-01-02 15:04")
		}
		chart.Labels = append(chart.Labels, label)
		chart.Durations = append(chart.Durations, duration)
		chart.Statuses = append(chart.Statuses, status)
	}
	return chart, rows.Err()
}

func (s *SQLStore) GetRecentTestStatuses(ctx context.Context, projectID, branch string, testKeys []string, limit int) (map[string][]string, error) {
	results := make(map[string][]string, len(testKeys))
	if len(testKeys) == 0 || limit <= 0 {
		return results, nil
	}

	placeholders := make([]string, 0, len(testKeys))
	args := make([]any, 0, len(testKeys)+4)
	args = append(args, projectID, branch, branch)
	for _, testKey := range testKeys {
		placeholders = append(placeholders, "?")
		args = append(args, testKey)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		WITH ranked AS (
			SELECT
				tr.test_key,
				tr.status,
				r.uploaded_at,
				ROW_NUMBER() OVER (PARTITION BY tr.test_key ORDER BY r.uploaded_at DESC) AS rn
			FROM test_results tr
			INNER JOIN runs r ON r.id = tr.run_id
			WHERE tr.project_id = ? AND (? = '' OR r.branch = ?) AND tr.test_key IN (%s)
		)
		SELECT test_key, status
		FROM ranked
		WHERE rn <= ?
		ORDER BY test_key, uploaded_at DESC
	`, strings.Join(placeholders, ", "))

	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var testKey string
		var status string
		if err := rows.Scan(&testKey, &status); err != nil {
			return nil, err
		}
		results[testKey] = append(results[testKey], status)
	}
	return results, rows.Err()
}
