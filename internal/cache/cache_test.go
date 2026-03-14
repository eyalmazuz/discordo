package cache

import "testing"

func TestCacheCreateGetExists(t *testing.T) {
	c := NewCache()
	if c == nil {
		t.Fatal("expected NewCache to return a cache")
	}
	if c.Exists("missing") {
		t.Fatal("expected new cache to start empty")
	}

	c.Create("query", 3)
	if !c.Exists("query") {
		t.Fatal("expected created query to exist")
	}
	if got := c.Get("query"); got != 3 {
		t.Fatalf("Get(query) = %d, want 3", got)
	}
}

func TestCacheInvalidate(t *testing.T) {
	t.Run("NoopForMissingEntries", func(t *testing.T) {
		c := NewCache()
		c.Invalidate("abcd", 2)
		if c.Exists("abcd") {
			t.Fatal("expected missing entry to remain missing")
		}
	})

	t.Run("DeletesExactAndPrefixesAtLimit", func(t *testing.T) {
		c := NewCache()
		c.Create("a", 1)
		c.Create("ab", 5)
		c.Create("abc", 5)
		c.Create("abcd", 5)

		c.Invalidate("abcd", 5)
		if c.Exists("abcd") || c.Exists("abc") || c.Exists("ab") || c.Exists("a") {
			t.Fatal("expected all entries to be deleted when at limit")
		}
	})

	t.Run("LeavesEntriesBelowLimitUntilMatchingPrefix", func(t *testing.T) {
		c := NewCache()
		c.Create("a", 1)
		c.Create("ab", 5)
		c.Create("abc", 4)
		c.Create("abcd", 4)

		c.Invalidate("abcd", 5)
		if c.Exists("ab") || c.Exists("a") {
			t.Fatal("expected entries at or above limit to be deleted")
		}
		if !c.Exists("abcd") || !c.Exists("abc") {
			t.Fatal("expected entries below limit to be preserved")
		}
	})
}
