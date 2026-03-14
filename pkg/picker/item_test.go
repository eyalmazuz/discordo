package picker

import "testing"

func TestItemsHelpers(t *testing.T) {
	items := Items{
		{Text: "first", FilterText: "alpha"},
		{Text: "second", FilterText: "beta"},
	}

	if got := items.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
	if got := items.String(0); got != "alpha" {
		t.Fatalf("String(0) = %q, want alpha", got)
	}
	if got := items.String(1); got != "beta" {
		t.Fatalf("String(1) = %q, want beta", got)
	}
}
