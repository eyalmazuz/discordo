package http

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("token-123")
	if c.UserAgent != BrowserUserAgent {
		t.Errorf("expected UserAgent %q, got %q", BrowserUserAgent, c.UserAgent)
	}
}
