package chat

import (
	"testing"
)

func TestLinkDisplayText_Equivalence(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "IPv6 Host",
			raw:  "http://[::1]:8080/foo",
			want: "[::1]:8080/foo",
		},
		{
			name: "Empty path",
			raw:  "https://example.com",
			want: "example.com",
		},
		{
			name: "Slash path",
			raw:  "https://example.com/",
			want: "example.com",
		},
		{
			name: "Long path (49 chars)",
			raw:  "https://example.com/1234567890123456789012345678901234567890123456789",
			want: "example.com/12345678901234567890123456789012345678901234...",
		},
		}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := linkDisplayText(tt.raw); got != tt.want {
				t.Errorf("linkDisplayText(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestUnescapeMarkdownEscapes_Equivalence(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "No escapes",
			raw:  "hello world",
			want: "hello world",
		},
		{
			name: "Valid escape \\*",
			raw:  "hello \\*world\\*",
			want: "hello *world*",
		},
		{
			name: "Invalid escape \\a",
			raw:  "hello \\aworld",
			want: "hello \\aworld",
		},
		{
			name: "Trailing backslash",
			raw:  "hello world\\",
			want: "hello world\\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unescapeMarkdownEscapes(tt.raw); got != tt.want {
				t.Errorf("unescapeMarkdownEscapes(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
