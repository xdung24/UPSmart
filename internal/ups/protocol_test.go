package ups

import (
	"strings"
	"testing"
)

func TestParseQ1(t *testing.T) {
	for _, terminal := range []string{"", "\r"} {
		got, err := ParseQ1("(230.1 000.0 229.8 012 50.0 27.4 31.0 11010001" + terminal)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode != "on_battery" || !got.LowBattery || !got.Fault {
			t.Fatalf("incorrect flags: %+v", got)
		}
		for key, expected := range map[string]float64{
			"input_voltage": 230.1, "output_voltage": 229.8, "load": 12,
			"input_frequency": 50, "battery_voltage": 27.4, "ups_temperature": 31,
		} {
			m := got.Measurements[key]
			if m.Value == nil || *m.Value != expected || m.Quality != "reported" {
				t.Errorf("%s = %+v; expected %v", key, m, expected)
			}
		}
		capacity := got.Measurements["battery_capacity"]
		if capacity.Value != nil || capacity.Quality != "unavailable" {
			t.Fatalf("fabricated capacity: %+v", capacity)
		}
	}
}

func TestParseQ1RejectsMalformedReplies(t *testing.T) {
	valid := "(230.1 000.0 229.8 012 50.0 27.4 31.0 00001001\r"
	cases := map[string]string{
		"no ack": "UPS No Ack", "missing prefix": valid[1:],
		"truncated":          "(230.1 000.0 229.8",
		"NaN":                strings.Replace(valid, "230.1", "NaN", 1),
		"infinite":           strings.Replace(valid, "27.4", "+Inf", 1),
		"negative voltage":   strings.Replace(valid, "230.1", "-1", 1),
		"hexadecimal number": strings.Replace(valid, "230.1", "0x1p2", 1),
		"exponential number": strings.Replace(valid, "230.1", "2e2", 1),
		"load above 100":     strings.Replace(valid, "012", "101", 1),
		"invalid bits":       strings.Replace(valid, "00001001", "00002001", 1),
		"short bits":         strings.Replace(valid, "00001001", "0000100", 1),
		"embedded newline":   strings.Replace(valid, "000.0", "000.0\n", 1),
		"trailing junk":      valid + "x", "oversize": "(" + strings.Repeat("0", 256),
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseQ1(raw); err == nil {
				t.Fatal("accepted malformed reply")
			}
		})
	}
}

func TestCapturedAR2000Status(t *testing.T) {
	// Captured through read-only HID string index 3 on 2026-11-12.
	got, err := ParseQ1("(232.0 000.0 234.0 013 49.6 27.0 29.0 00001000\r")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "online" || got.LowBattery || got.Fault {
		t.Fatalf("unexpected baseline flags: %+v", got)
	}
	for key, expected := range map[string]float64{
		"input_voltage": 232, "output_voltage": 234, "load": 13,
		"input_frequency": 49.6, "battery_voltage": 27, "ups_temperature": 29,
	} {
		if *got.Measurements[key].Value != expected {
			t.Errorf("%s does not match captured field: %+v", key, got.Measurements[key])
		}
	}
}
