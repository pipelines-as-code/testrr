package views

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	terminal "github.com/buildkite/terminal-to-html/v3"
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

func RenderANSI(input string) templ.Component {
	if strings.TrimSpace(input) == "" {
		return templ.NopComponent
	}
	return templ.Raw(`<div class="term-container">` + terminal.Render([]byte(input)) + `</div>`)
}

func testHistoryURL(projectSlug, testKey string) templ.SafeURL {
	return templ.URL("/projects/" + projectSlug + "/tests?test_key=" + url.QueryEscape(testKey))
}

func testDurationChartURL(projectSlug, testKey string) string {
	return "/projects/" + projectSlug + "/tests/chart?test_key=" + url.QueryEscape(testKey)
}

func historyStatusClass(status string) string {
	switch status {
	case "failed":
		return "bg-error border-error/30"
	case "skipped":
		return "bg-warning border-warning/30"
	default:
		return "bg-success border-success/30"
	}
}
