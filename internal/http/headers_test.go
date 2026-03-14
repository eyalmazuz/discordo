package http

import (
	"errors"
	"testing"
)

func TestHeaders(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		oldSuperPropsHeaderValue := superPropsHeaderValue
		t.Cleanup(func() { superPropsHeaderValue = oldSuperPropsHeaderValue })
		superPropsHeaderValue = func() (string, error) { return "encoded", nil }

		h := Headers()
		if h.Get("Origin") != "https://discord.com" {
			t.Errorf("expected Origin https://discord.com, got %q", h.Get("Origin"))
		}
		if h.Get("X-Super-Properties") != "encoded" {
			t.Errorf("expected X-Super-Properties to be set, got %q", h.Get("X-Super-Properties"))
		}
	})

	t.Run("super props error", func(t *testing.T) {
		oldSuperPropsHeaderValue := superPropsHeaderValue
		t.Cleanup(func() { superPropsHeaderValue = oldSuperPropsHeaderValue })
		superPropsHeaderValue = func() (string, error) { return "", errors.New("boom") }

		h := Headers()
		if got := h.Get("X-Super-Properties"); got != "" {
			t.Fatalf("expected missing X-Super-Properties on error, got %q", got)
		}
	})
}
