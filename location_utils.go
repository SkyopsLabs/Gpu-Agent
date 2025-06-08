package main

import (
	"encoding/json"
	"fmt"
)

// LocationData represents the full location response from ipinfo.io
type LocationData struct {
	IP       string `json:"ip"`
	City     string `json:"city"`
	Region   string `json:"region"`
	Country  string `json:"country"`
	Loc      string `json:"loc"`      // "latitude,longitude"
	Org      string `json:"org"`      // Organization/ISP
	Postal   string `json:"postal"`   // Postal code
	Timezone string `json:"timezone"` // Timezone
}

// FormatLocationForDisplay formats location data for human-readable display
func FormatLocationForDisplay(location interface{}) string {
	switch v := location.(type) {
	case string:
		// Handle legacy string format
		if v == "" {
			return "Unknown"
		}
		return v
	case LocationData:
		// Handle structured location data
		return formatLocationStruct(v)
	case map[string]interface{}:
		// Handle JSON object
		city, _ := v["city"].(string)
		region, _ := v["region"].(string)
		country, _ := v["country"].(string)
		return formatLocationString(city, region, country)
	case []byte:
		// Handle JSON bytes
		var loc LocationData
		if err := json.Unmarshal(v, &loc); err == nil {
			return formatLocationStruct(loc)
		}
		return "Unknown"
	default:
		return "Unknown"
	}
}

// formatLocationStruct formats a LocationData struct
func formatLocationStruct(loc LocationData) string {
	return formatLocationString(loc.City, loc.Region, loc.Country)
}

// formatLocationString formats city, region, country into a display string
func formatLocationString(city, region, country string) string {
	var parts []string
	if city != "" {
		parts = append(parts, city)
	}
	if region != "" {
		parts = append(parts, region)
	}
	if country != "" {
		parts = append(parts, country)
	}
	
	if len(parts) == 0 {
		return "Unknown"
	}
	
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += ", "
		}
		result += part
	}
	return result
}

// ParseLocationFromJSON parses location data from JSON string
func ParseLocationFromJSON(jsonStr string) (LocationData, error) {
	var loc LocationData
	err := json.Unmarshal([]byte(jsonStr), &loc)
	return loc, err
}

// LocationToJSON converts location data to JSON string
func LocationToJSON(loc LocationData) (string, error) {
	data, err := json.Marshal(loc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal location: %v", err)
	}
	return string(data), nil
}
