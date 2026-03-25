package views

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRenderANSI(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	if err := RenderANSI("\x1b[31mred failure\x1b[0m").Render(context.Background(), &buffer); err != nil {
		t.Fatalf("render ansi: %v", err)
	}

	rendered := buffer.String()
	for _, snippet := range []string{"term-container", "term-fg31", "red failure"} {
		if !strings.Contains(rendered, snippet) {
			t.Fatalf("expected rendered ansi output to contain %q, got %q", snippet, rendered)
		}
	}
}
