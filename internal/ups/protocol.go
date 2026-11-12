package ups

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var decimalField = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)

type Measurement struct {
	Value   *float64 `json:"value"`
	Unit    string   `json:"unit"`
	Quality string   `json:"quality"`
	Reason  string   `json:"reason,omitempty"`
}

type Reading struct {
	Measurements map[string]Measurement `json:"measurements"`
	Mode         string                 `json:"mode"`
	LowBattery   bool                   `json:"low_battery"`
	Fault        bool                   `json:"fault"`
}

func ParseQ1(raw string) (Reading, error) {
	var result Reading
	if len(raw) > 256 || !strings.HasPrefix(raw, "(") {
		return result, errors.New("invalid Q1 frame")
	}
	// Indexed-string transports may omit the terminal carriage return.
	body := strings.TrimSuffix(raw[1:], "\r")
	if strings.ContainsAny(body, "\r\n\x00") {
		return result, errors.New("invalid Q1 frame terminator")
	}
	fields := strings.Fields(body)
	if len(fields) != 8 {
		return result, fmt.Errorf("Q1 frame has %d fields; expected 8", len(fields))
	}
	values := make([]float64, 7)
	for i := range values {
		if !decimalField.MatchString(fields[i]) {
			return result, fmt.Errorf("invalid Q1 decimal field %d", i+1)
		}
		value, err := strconv.ParseFloat(fields[i], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return result, fmt.Errorf("invalid Q1 numeric field %d", i+1)
		}
		if i < 6 && value < 0 {
			return result, fmt.Errorf("negative Q1 numeric field %d", i+1)
		}
		values[i] = value
	}
	if values[3] > 100 {
		return result, errors.New("Q1 load must be between 0 and 100 percent")
	}
	bits := fields[7]
	if len(bits) != 8 || strings.Trim(bits, "01") != "" {
		return result, errors.New("Q1 status must contain exactly eight binary digits")
	}
	result.Measurements = make(map[string]Measurement, 7)
	for _, field := range []struct {
		name string
		unit string
		i    int
	}{
		{"input_voltage", "V AC", 0},
		{"output_voltage", "V AC", 2},
		{"load", "%", 3},
		{"input_frequency", "Hz", 4},
		{"battery_voltage", "V DC", 5},
		{"ups_temperature", "C", 6},
	} {
		v := values[field.i]
		result.Measurements[field.name] = Measurement{Value: &v, Unit: field.unit, Quality: "reported"}
	}
	result.Measurements["battery_capacity"] = Measurement{Unit: "%", Quality: "unavailable", Reason: "Not reported by the Q1 status query"}
	result.Mode = "online"
	if bits[0] == '1' {
		result.Mode = "on_battery"
	}
	result.LowBattery = bits[1] == '1'
	result.Fault = bits[3] == '1'
	return result, nil
}
