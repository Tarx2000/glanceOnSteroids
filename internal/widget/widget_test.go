package widget

import (
	"testing"
	"time"
)

// TestRequiresUpdateCustomCache verifies that RequiresUpdate correctly respects
// the custom cache duration (CustomCacheDuration) for dynamic widgets.
func TestRequiresUpdateCustomCache(t *testing.T) {
	// Create a generic widget base
	w := &widgetBase{}
	w.CustomCacheDuration = DurationField(2 * time.Hour) // User set cache to 2 hours
	w.withTitle("Test Widget").withCacheDuration(10 * time.Minute) // 10 min default, overridden to 2 hours

	now := time.Now()

	// Fresh widget with no update history should require update
	if !w.RequiresUpdate(&now) {
		t.Error("expected fresh widget with zero NextUpdate to require update")
	}

	// Widget updated recently (5 minutes ago) with next update in 2 hours should NOT require update
	w.NextUpdate = now.Add(2 * time.Hour)
	if w.RequiresUpdate(&now) {
		t.Error("expected recently updated widget with future NextUpdate to NOT require update")
	}

	// Widget updated 1h 59m ago with next update in 1 minute should NOT require update
	w.NextUpdate = now.Add(1 * time.Minute)
	if w.RequiresUpdate(&now) {
		t.Error("expected widget with future NextUpdate to NOT require update")
	}

	// Expired widget should require update
	w.NextUpdate = now.Add(-1 * time.Minute)
	if !w.RequiresUpdate(&now) {
		t.Error("expected expired widget to require update")
	}
}

// TestWithCacheOnTheHourRespectsCustomCache verifies that even hourly-cached widgets
// respect the custom cache duration if the user specifies one.
func TestWithCacheOnTheHourRespectsCustomCache(t *testing.T) {
	w := &widgetBase{}
	w.CustomCacheDuration = DurationField(15 * time.Minute)
	w.withTitle("Hourly Widget").withCacheOnTheHour()

	if w.CacheType != cacheTypeDuration {
		t.Errorf("expected CacheType to be cacheTypeDuration, got %v", w.CacheType)
	}
	if w.CacheDuration != 15*time.Minute {
		t.Errorf("expected CacheDuration to be 15m, got %v", w.CacheDuration)
	}
}
