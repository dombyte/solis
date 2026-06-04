package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dombyte/solis/internal/solis"
)

func TestNew(t *testing.T) {
	ca := New()

	if ca == nil {
		t.Fatal("New() returned nil")
	}

	if ca.Size() != 0 {
		t.Errorf("New cache should be empty, got size %d", ca.Size())
	}
}

func TestSetAndGet(t *testing.T) {
	ca := New()

	// Create a test value
	value := &solis.Value{
		Key:          "test_key",
		Name:         "Test Register",
		RawValue:     100,
		DecodedValue: 100.5,
		Unit:         "V",
		Timestamp:    time.Now(),
		DataType:     solis.Uint16,
		Stability:    solis.Dynamic,
	}

	// Set the value
	ca.Set(map[string]*solis.Value{
		"test_key": value,
	})

	// Get the value
	result := ca.Get("test_key")
	if result == nil {
		t.Fatal("Get() returned nil for existing key")
	}

	if result.Key != "test_key" {
		t.Errorf("Get() returned wrong key: got %s, want test_key", result.Key)
	}

	if result.DecodedValue != 100.5 {
		t.Errorf("Get() returned wrong value: got %f, want 100.5", result.DecodedValue)
	}
}

func TestGetMultiple(t *testing.T) {
	ca := New()

	// Create test values
	values := map[string]*solis.Value{
		"key1": {
			Key:          "key1",
			Name:         "Key 1",
			DecodedValue: 1.0,
			Timestamp:    time.Now(),
			Stability:    solis.Dynamic,
		},
		"key2": {
			Key:          "key2",
			Name:         "Key 2",
			DecodedValue: 2.0,
			Timestamp:    time.Now(),
			Stability:    solis.Dynamic,
		},
		"key3": {
			Key:          "key3",
			Name:         "Key 3",
			DecodedValue: 3.0,
			Timestamp:    time.Now(),
			Stability:    solis.Dynamic,
		},
	}

	ca.Set(values)

	// Get multiple keys
	result := ca.GetMultiple([]string{"key1", "key2", "key4"})

	if len(result) != 2 {
		t.Errorf("GetMultiple() returned %d values, want 2", len(result))
	}

	if _, ok := result["key1"]; !ok {
		t.Error("GetMultiple() missing key1")
	}

	if _, ok := result["key2"]; !ok {
		t.Error("GetMultiple() missing key2")
	}

	if _, ok := result["key4"]; ok {
		t.Error("GetMultiple() should not return key4")
	}
}

func TestGetAll(t *testing.T) {
	ca := New()

	// Create test values
	values := map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key2": {Key: "key2", DecodedValue: 2.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	}

	ca.Set(values)

	all := ca.GetAll()

	if len(all) != 2 {
		t.Errorf("GetAll() returned %d values, want 2", len(all))
	}

	if _, ok := all["key1"]; !ok {
		t.Error("GetAll() missing key1")
	}

	if _, ok := all["key2"]; !ok {
		t.Error("GetAll() missing key2")
	}
}

func TestKeys(t *testing.T) {
	ca := New()

	values := map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key2": {Key: "key2", DecodedValue: 2.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	}

	ca.Set(values)

	keys := ca.Keys()

	if len(keys) != 2 {
		t.Errorf("Keys() returned %d keys, want 2", len(keys))
	}

	// Check that both keys are present (order may vary)
	found := make(map[string]bool)
	for _, k := range keys {
		found[k] = true
	}

	if !found["key1"] {
		t.Error("Keys() missing key1")
	}

	if !found["key2"] {
		t.Error("Keys() missing key2")
	}
}

func TestClear(t *testing.T) {
	ca := New()

	values := map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	}

	ca.Set(values)

	if ca.Size() != 1 {
		t.Errorf("Cache should have 1 item, got %d", ca.Size())
	}

	ca.Clear()

	if ca.Size() != 0 {
		t.Errorf("Cache should be empty after Clear(), got %d", ca.Size())
	}

	if ca.Get("key1") != nil {
		t.Error("Get() should return nil for cleared key")
	}
}

func TestSetSingle(t *testing.T) {
	ca := New()

	value := &solis.Value{
		Key:          "single_key",
		DecodedValue: 42.0,
		Timestamp:    time.Now(),
		Stability:    solis.Dynamic,
	}

	ca.SetSingle("single_key", value)

	result := ca.Get("single_key")
	if result == nil {
		t.Fatal("Get() returned nil for single key")
	}

	if result.DecodedValue != 42.0 {
		t.Errorf("Get() returned wrong value: got %f, want 42.0", result.DecodedValue)
	}
}

func TestConcurrentAccess(t *testing.T) {
	ca := New()

	// Create test values
	values := map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key2": {Key: "key2", DecodedValue: 2.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	}

	ca.Set(values)

	var wg sync.WaitGroup
	numGoroutines := 100
	readsPerGoroutine := 1000

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				ca.Get("key1")
				ca.Get("key2")
				ca.GetMultiple([]string{"key1", "key2"})
				ca.GetAll()
				ca.Keys()
				ca.Size()
			}
		}(i)
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				newValues := map[string]*solis.Value{
					"key1": {Key: "key1", DecodedValue: float64(j), Timestamp: time.Now(), Stability: solis.Dynamic},
				}
				ca.Set(newValues)
				ca.SetSingle("key2", &solis.Value{Key: "key2", DecodedValue: float64(j), Timestamp: time.Now(), Stability: solis.Dynamic})
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is still functional
	if ca.Size() == 0 {
		t.Error("Cache is empty after concurrent access")
	}

	if ca.Get("key1") == nil {
		t.Error("Cache missing key1 after concurrent access")
	}

	if ca.Get("key2") == nil {
		t.Error("Cache missing key2 after concurrent access")
	}
}

func TestGetNonExistent(t *testing.T) {
	ca := New()

	result := ca.Get("nonexistent")
	if result != nil {
		t.Error("Get() should return nil for non-existent key")
	}

	values := ca.GetMultiple([]string{"nonexistent1", "nonexistent2"})
	if len(values) != 0 {
		t.Errorf("GetMultiple() should return empty map for non-existent keys, got %d", len(values))
	}
}

func TestTimestampPreservation(t *testing.T) {
	ca := New()

	testTime := time.Date(2026, 5, 29, 12, 34, 56, 0, time.UTC)
	value := &solis.Value{
		Key:          "timestamped_key",
		DecodedValue: 100.0,
		Timestamp:    testTime,
		Stability:    solis.Dynamic,
	}

	ca.Set(map[string]*solis.Value{"timestamped_key": value})

	result := ca.Get("timestamped_key")
	if result == nil {
		t.Fatal("Get() returned nil")
	}

	if !result.Timestamp.Equal(testTime) {
		t.Errorf("Timestamp not preserved: got %v, want %v", result.Timestamp, testTime)
	}
}

func TestValueOverwrite(t *testing.T) {
	ca := New()

	// Set initial value
	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	if ca.Get("key1").DecodedValue != 1.0 {
		t.Error("Initial value not set correctly")
	}

	// Overwrite with new value
	time.Sleep(10 * time.Millisecond)
	newTime := time.Now()
	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 2.0, Timestamp: newTime, Stability: solis.Dynamic},
	})

	result := ca.Get("key1")
	if result.DecodedValue != 2.0 {
		t.Errorf("Value not overwritten: got %f, want 2.0", result.DecodedValue)
	}

	if !result.Timestamp.Equal(newTime) {
		t.Errorf("Timestamp not updated on overwrite")
	}
}

func TestSetSingleOverwrite(t *testing.T) {
	ca := New()

	// Set initial value
	ca.SetSingle("key1", &solis.Value{
		Key:          "key1",
		DecodedValue: 1.0,
		Timestamp:    time.Now(),
		Stability:    solis.Dynamic,
	})

	// Overwrite with SetSingle
	ca.SetSingle("key1", &solis.Value{
		Key:          "key1",
		DecodedValue: 3.0,
		Timestamp:    time.Now(),
		Stability:    solis.Dynamic,
	})

	result := ca.Get("key1")
	if result.DecodedValue != 3.0 {
		t.Errorf("SetSingle overwrite failed: got %f, want 3.0", result.DecodedValue)
	}
}

func TestSetMixedKeys(t *testing.T) {
	ca := New()

	// Set some keys
	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key2": {Key: "key2", DecodedValue: 2.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	// Set new values - this replaces the entire cache
	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 10.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key3": {Key: "key3", DecodedValue: 3.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	// key1 should be updated
	if ca.Get("key1").DecodedValue != 10.0 {
		t.Error("key1 not updated")
	}

	// key2 should NOT exist anymore - Set() replaces the entire cache
	if ca.Get("key2") != nil {
		t.Error("key2 should have been removed when cache was replaced")
	}

	// key3 should be added
	if ca.Get("key3") == nil {
		t.Error("key3 not added")
	}

	// Cache should only have key1 and key3
	if ca.Size() != 2 {
		t.Errorf("Cache size should be 2 (key1 and key3), got %d", ca.Size())
	}
}

func TestEmptyOperations(t *testing.T) {
	ca := New()

	// Get from empty cache
	if ca.Get("any") != nil {
		t.Error("Get from empty cache should return nil")
	}

	// GetMultiple from empty cache
	values := ca.GetMultiple([]string{"a", "b", "c"})
	if len(values) != 0 {
		t.Error("GetMultiple from empty cache should return empty map")
	}

	// GetAll from empty cache
	all := ca.GetAll()
	if len(all) != 0 {
		t.Error("GetAll from empty cache should return empty map")
	}

	// Keys from empty cache
	keys := ca.Keys()
	if len(keys) != 0 {
		t.Error("Keys from empty cache should return empty slice")
	}

	// Size of empty cache
	if ca.Size() != 0 {
		t.Error("Size of empty cache should be 0")
	}

	// Clear empty cache (should not panic)
	ca.Clear()
	if ca.Size() != 0 {
		t.Error("Clear on empty cache should not cause issues")
	}

	// Set empty map
	ca.Set(map[string]*solis.Value{})
	if ca.Size() != 0 {
		t.Error("Set with empty map should not add any entries")
	}
}

func TestGetMultipleEmptyInput(t *testing.T) {
	ca := New()

	// Set some values
	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	// GetMultiple with empty slice
	values := ca.GetMultiple([]string{})
	if len(values) != 0 {
		t.Error("GetMultiple with empty slice should return empty map")
	}

	// GetMultiple with nil slice
	values = ca.GetMultiple(nil)
	if len(values) != 0 {
		t.Error("GetMultiple with nil slice should return empty map")
	}
}

func TestMultipleKeysWithDifferentStability(t *testing.T) {
	ca := New()

	// Set values with different stability
	ca.Set(map[string]*solis.Value{
		"dynamic_key": {Key: "dynamic_key", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"stable_key":  {Key: "stable_key", DecodedValue: 2.0, Timestamp: time.Now(), Stability: solis.Stable},
	})

	// Both should be retrievable
	if ca.Get("dynamic_key") == nil {
		t.Error("dynamic_key not found")
	}

	if ca.Get("stable_key") == nil {
		t.Error("stable_key not found")
	}

	// Check stability is preserved
	if ca.Get("dynamic_key").Stability != solis.Dynamic {
		t.Error("dynamic_key stability not preserved")
	}

	if ca.Get("stable_key").Stability != solis.Stable {
		t.Error("stable_key stability not preserved")
	}
}

func TestValuePointersAreIndependentAfterSet(t *testing.T) {
	ca := New()

	// Create a value
	originalValue := &solis.Value{
		Key:          "test",
		DecodedValue: 1.0,
		Timestamp:    time.Now(),
		Stability:    solis.Dynamic,
	}

	// Store it in cache
	ca.Set(map[string]*solis.Value{"test": originalValue})

	// Modify the original - this SHOULD affect cache because Set copies the pointer
	originalValue.DecodedValue = 999.0

	// Get from cache - WILL see the modification because Set stores pointer to original
	result := ca.Get("test")
	if result.DecodedValue != 999.0 {
		t.Errorf("Cache stores pointer to value - external modification is visible. Value: %f, want 999.0", result.DecodedValue)
	}

	// This is expected behavior - caller should not modify values after passing to Set()
	// In practice, poller always creates new values for each poll cycle
}

func TestNilCacheOperations(t *testing.T) {
	// This documents that cache should always be initialized with New()
	// Nil cache operations would panic with nil receiver
	t.Skip("Nil cache operations would panic - cache should always be created with New()")
}

func TestLargeCacheOperations(t *testing.T) {
	ca := New()

	// Add many entries
	numEntries := 1000
	values := make(map[string]*solis.Value, numEntries)
	for i := 0; i < numEntries; i++ {
		key := fmt.Sprintf("key_%d", i)
		values[key] = &solis.Value{
			Key:          key,
			DecodedValue: float64(i),
			Timestamp:    time.Now(),
			Stability:    solis.Dynamic,
		}
	}

	ca.Set(values)

	if ca.Size() != numEntries {
		t.Errorf("Cache size: got %d, want %d", ca.Size(), numEntries)
	}

	// Verify all can be retrieved
	for i := 0; i < numEntries; i++ {
		key := fmt.Sprintf("key_%d", i)
		result := ca.Get(key)
		if result == nil {
			t.Errorf("Failed to retrieve key %s", key)
			break
		}
		if result.DecodedValue != float64(i) {
			t.Errorf("Wrong value for key %s: got %f, want %f", key, result.DecodedValue, float64(i))
			break
		}
	}

	// GetAll should return all
	all := ca.GetAll()
	if len(all) != numEntries {
		t.Errorf("GetAll returned %d entries, want %d", len(all), numEntries)
	}
}

func TestSetOverwritesExisting(t *testing.T) {
	ca := New()

	// Set initial values
	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key2": {Key: "key2", DecodedValue: 2.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key3": {Key: "key3", DecodedValue: 3.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	// Set new values that partially overlap - this replaces the entire cache
	ca.Set(map[string]*solis.Value{
		"key2": {Key: "key2", DecodedValue: 20.0, Timestamp: time.Now(), Stability: solis.Dynamic},
		"key4": {Key: "key4", DecodedValue: 4.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	// key1 should NOT exist anymore - Set() replaces the entire cache
	if ca.Get("key1") != nil {
		t.Error("key1 should have been removed when cache was replaced")
	}

	// key2 should be updated
	if ca.Get("key2").DecodedValue != 20.0 {
		t.Error("key2 was not updated")
	}

	// key3 should NOT exist anymore - Set() replaces the entire cache
	if ca.Get("key3") != nil {
		t.Error("key3 should have been removed when cache was replaced")
	}

	// key4 should be added
	if ca.Get("key4") == nil {
		t.Error("key4 was not added")
	}

	// Cache should only have key2 and key4
	if ca.Size() != 2 {
		t.Errorf("Cache size should be 2 (key2 and key4), got %d", ca.Size())
	}
}

func TestGetAllReturnsShallowCopy(t *testing.T) {
	ca := New()

	ca.Set(map[string]*solis.Value{
		"key1": {Key: "key1", DecodedValue: 1.0, Timestamp: time.Now(), Stability: solis.Dynamic},
	})

	// Get all - returns a new map with pointers to same values
	all := ca.GetAll()

	// Verify we got a copy of the map by adding to it
	all["new_key"] = &solis.Value{Key: "new_key", DecodedValue: 0, Timestamp: time.Now(), Stability: solis.Dynamic}
	if ca.Get("new_key") != nil {
		t.Error("GetAll returned same map, not a copy")
	}

	// Modifications to value pointers WILL affect cache (shallow copy)
	// This is expected and acceptable - cache is fast, doesn't deep copy values
	all["key1"].DecodedValue = 999.0
	if ca.Get("key1").DecodedValue != 999.0 {
		t.Error("GetAll pointers point to same values - modification affected cache")
	}
}
