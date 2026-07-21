package imagedownloader

import (
	"context"
	"testing"
	"time"
)

func TestPinterestViaDDG(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := searchDuckDuckGo(ctx, "site:pinterest.com con mèo", 50)
	if err != nil {
		t.Fatalf("searchDuckDuckGo site:pinterest.com failed: %v", err)
	}

	t.Logf("Found %d Pinterest entries via DDG site search!", len(result.Entries))
	for i, entry := range result.Entries {
		t.Logf("[%d] Title: %s\n  URL: %s\n  ThumbURL: %s", i+1, entry.Title, entry.URL, entry.ThumbURL)
	}

	if len(result.Entries) == 0 {
		t.Fatalf("expected > 0 entries")
	}
}
