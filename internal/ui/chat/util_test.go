package chat

import "testing"

func TestHumanJoin(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  string
	}{
		{name: "empty", want: ""},
		{name: "one", items: []string{"alpha"}, want: "alpha"},
		{name: "two", items: []string{"alpha", "beta"}, want: "alpha and beta"},
		{name: "many", items: []string{"alpha", "beta", "gamma"}, want: "alpha, beta, and gamma"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanJoin(tt.items); got != tt.want {
				t.Fatalf("humanJoin(%v) = %q, want %q", tt.items, got, tt.want)
			}
		})
	}
}
