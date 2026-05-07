package cache

import "testing"

func TestInvalidateDeletesPrefixesAndReturns(t *testing.T) {
	c := NewCache()
	c.Create("abc", 10)
	c.Create("ab", 10)
	c.Create("a", 10)
	c.Create("z", 1)

	c.Invalidate("abc", 5)

	for _, key := range []string{"abc", "ab", "a"} {
		if c.Exists(key) {
			t.Fatalf("expected %q to be invalidated", key)
		}
	}
	if !c.Exists("z") {
		t.Fatal("expected unrelated cache entry to remain")
	}
}

func TestInvalidateBelowLimitKeepsEntries(t *testing.T) {
	c := NewCache()
	c.Create("abc", 4)

	c.Invalidate("abc", 5)

	if !c.Exists("abc") {
		t.Fatal("expected below-limit cache entry to remain")
	}
}
