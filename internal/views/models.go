package views

import "testrr/internal/store"

type RunResultGroup struct {
	Name    string
	Results []RunResultRow
}

type RunResultRow struct {
	Result         store.TestResult
	RecentStatuses []string
}
