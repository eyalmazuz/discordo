package http

import (
	"testing"
)

func TestHeaders(t *testing.T) {
	h := Headers()
	if h.Get("Origin") != "https://discord.com" {
		t.Errorf("expected Origin https://discord.com, got %q", h.Get("Origin"))
	}
	if h.Get("X-Super-Properties") == "" {
		t.Error("expected X-Super-Properties to be set")
	}
}
