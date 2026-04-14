package renderer

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

type captureHandler struct {
	mu       sync.Mutex
	messages []string
}

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, r.Message)
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(name string) slog.Handler       { return h }
func (h *captureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func TestRewriteImageRefs_A11yLint(t *testing.T) {
	handler := &captureHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	tests := []struct {
		name        string
		html        string
		isDev       bool
		expectWarn  bool
		warnMessage string
	}{
		{
			name:        "missing alt in production",
			html:        `<img src="test.png">`,
			isDev:       false,
			expectWarn:  true,
			warnMessage: "A11y Lint: Image missing alt text",
		},
		{
			name:       "has alt in production",
			html:       `<img src="test.png" alt="description">`,
			isDev:      false,
			expectWarn: false,
		},
		{
			name:       "missing alt in dev mode",
			html:       `<img src="test.png">`,
			isDev:      true,
			expectWarn: false,
		},
		{
			name:       "empty alt in production",
			html:       `<img src="test.png" alt="">`,
			isDev:      false,
			expectWarn: false, // Technically has alt attribute, even if empty (decorative)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler.mu.Lock()
			handler.messages = nil
			handler.mu.Unlock()

			rewriteImageRefs([]byte(tt.html), "test.html", tt.isDev)

			handler.mu.Lock()
			found := false
			for _, msg := range handler.messages {
				if msg == tt.warnMessage {
					found = true
					break
				}
			}
			handler.mu.Unlock()

			if tt.expectWarn && !found {
				t.Errorf("Expected warning %q, but not found in %v", tt.warnMessage, handler.messages)
			}
			if !tt.expectWarn && found {
				t.Errorf("Did not expect warning, but found %q", tt.warnMessage)
			}
		})
	}
}
