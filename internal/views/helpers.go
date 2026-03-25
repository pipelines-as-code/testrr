package views

import (
	"fmt"
	"strings"
	"time"
)

func statusClass(status string) string {
	switch status {
	case "failed":
		return "badge-error"
	case "skipped":
		return "badge-warning"
	case "failed_import":
		return "badge-error"
	default:
		return "badge-success"
	}
}

func statusLabel(status string) string {
	if status == "" {
		return "unknown"
	}
	return strings.ReplaceAll(status, "_", " ")
}

func formatMillis(value int64) string {
	if value == 0 {
		return "0 ms"
	}
	if value >= 1000 {
		return fmt.Sprintf("%.2fs", float64(value)/1000)
	}
	return fmt.Sprintf("%d ms", value)
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format("2006-01-02 15:04")
}

func passRate(passed, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(passed)/float64(total)*100)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
