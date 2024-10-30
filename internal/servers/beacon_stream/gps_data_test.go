package beacon_stream

import (
	"encoding/json"
	"testing"
)

func TestGPSDataConstruction(t *testing.T) {
	tests := []struct {
		name     string
		input    GPSData
		expected string
	}{
		{
			name: "Test with data 1",
			input: GPSData{
				MarkerID: 500,
				AngleX:   0.17,
				AngleY:   0.039,
				Distance: 0.0,
			},
			expected: `{"markerId":500,"angle_x":0.17,"angle_y":0.039,"distance":0}`,
		},
		{
			name: "Test with data 2",
			input: GPSData{
				MarkerID: 500,
				AngleX:   0.144,
				AngleY:   0.013,
				Distance: 0.0,
			},
			expected: `{"markerId":500,"angle_x":0.144,"angle_y":0.013,"distance":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataBytes, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Failed to marshal GPSData: %v", err)
			}

			got := string(dataBytes)
			if got != tt.expected {
				t.Errorf("GPSData construction failed. Got %s, expected %s", got, tt.expected)
			}
		})
	}
}

func TestGPSDataToJSON(t *testing.T) {
	data := GPSData{
		MarkerID: 500,
		AngleX:   0.144,
		AngleY:   0.013,
		Distance: 0.0,
	}

	expectedJSON := `{"markerId":500,"angle_x":0.144,"angle_y":0.013,"distance":0}`

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal GPSData to JSON: %v", err)
	}

	jsonString := string(jsonBytes)
	if jsonString != expectedJSON {
		t.Errorf("JSON output mismatch. Got %s, expected %s", jsonString, expectedJSON)
	}
}
