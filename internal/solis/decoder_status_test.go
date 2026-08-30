package solis

import (
	"encoding/json"
	"testing"
)

// TestDecodeRegister_WithStatusDecoding tests that status registers get their decoded values
func TestDecodeRegister_WithStatusDecoding(t *testing.T) {
	// Test solis_status register
	reg, ok := RegisterMapByKey["solis_status"]
	if !ok {
		t.Fatal("solis_status register not found")
	}

	// Simulate a raw value (e.g., 3000 which maps to a status)
	raw := []uint16{0x0BB8} // 3000 in uint16

	value := DecodeRegister(reg, raw)

	// Check basic fields
	if value.Key != "solis_status" {
		t.Errorf("Expected key 'solis_status', got '%s'", value.Key)
	}

	if value.RawValue != 3000 {
		t.Errorf("Expected RawValue 3000, got %.0f", value.RawValue)
	}

	// Check that StatusDecoded is populated for status register
	if value.StatusDecoded == nil {
		t.Fatal("Expected StatusDecoded to be populated for status register")
	}

	// The StatusDecoded should be a map for solis_status
	if statusMap, ok := value.StatusDecoded.(map[string]string); !ok {
		t.Errorf("Expected StatusDecoded to be map[string]string, got %T", value.StatusDecoded)
	} else {
		if statusMap["name"] == "" {
			t.Error("Expected status name to be non-empty")
		}
	}

	// Test JSON marshaling includes status_decoded
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Failed to marshal value: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check that status_decoded is in the JSON
	if _, ok := result["status_decoded"]; !ok {
		t.Error("Expected status_decoded field in JSON output")
	}
}

// TestDecodeRegister_WithFaultStatus tests that fault status registers get decoded
func TestDecodeRegister_WithFaultStatus(t *testing.T) {
	// Test operating_status register (bitmask)
	reg, ok := RegisterMapByKey["operating_status"]
	if !ok {
		t.Fatal("operating_status register not found")
	}

	// Simulate a raw value with some bits set
	raw := []uint16{0x0001} // 1 in uint16

	value := DecodeRegister(reg, raw)

	// Check that StatusDecoded is populated
	if value.StatusDecoded == nil {
		t.Fatal("Expected StatusDecoded to be populated for fault status register")
	}

	// The StatusDecoded should be a []string for operating_status
	if _, ok := value.StatusDecoded.([]string); !ok {
		t.Errorf("Expected StatusDecoded to be []string, got %T", value.StatusDecoded)
	}
}

// TestDecodeRegister_NonStatusRegister tests that non-status registers don't get StatusDecoded
func TestDecodeRegister_NonStatusRegister(t *testing.T) {
	// Test a non-status register
	reg, ok := RegisterMapByKey["pv_energy_daily"]
	if !ok {
		t.Fatal("pv_energy_daily register not found")
	}

	raw := []uint16{0x012C} // 300 in uint16

	value := DecodeRegister(reg, raw)

	// Check that StatusDecoded is NOT populated for non-status register
	if value.StatusDecoded != nil {
		t.Error("Expected StatusDecoded to be nil for non-status register")
	}
}
