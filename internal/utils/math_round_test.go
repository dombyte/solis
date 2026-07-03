package utils

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestRealWorldExamples(t *testing.T) {
	// Example 1: raw 3086, scale 0.01
	raw1 := 3086.0
	scale1 := 0.01
	scaled1 := raw1 * scale1
	rounded1 := RoundTo2DecimalPlaces(scaled1)
	formatted1 := Float64With2Decimals(rounded1)
	data1, _ := json.Marshal(formatted1)
	fmt.Printf("Raw 3086, scale 0.01: scaled=%f, rounded=%f, json=%s\n", scaled1, rounded1, string(data1))
	
	// Example 2: raw 30864, scale 0.001
	raw2 := 30864.0
	scale2 := 0.001
	scaled2 := raw2 * scale2
	rounded2 := RoundTo2DecimalPlaces(scaled2)
	formatted2 := Float64With2Decimals(rounded2)
	data2, _ := json.Marshal(formatted2)
	fmt.Printf("Raw 30864, scale 0.001: scaled=%f, rounded=%f, json=%s\n", scaled2, rounded2, string(data2))
	
	// Example 3: raw 30866, scale 0.001 (to test rounding up)
	raw3 := 30866.0
	scale3 := 0.001
	scaled3 := raw3 * scale3
	rounded3 := RoundTo2DecimalPlaces(scaled3)
	formatted3 := Float64With2Decimals(rounded3)
	data3, _ := json.Marshal(formatted3)
	fmt.Printf("Raw 30866, scale 0.001: scaled=%f, rounded=%f, json=%s\n", scaled3, rounded3, string(data3))
}
