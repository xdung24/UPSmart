package policy

import (
	"testing"
	"time"
)

func TestOptionsValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Options)
	}{
		{"missing_mode", func(o *Options) { o.Mode = "" }},
		{"unknown_mode", func(o *Options) { o.Mode = "shutdown" }},
		{"no_trigger", func(o *Options) { o.Automatic.OutageAfter = ""; o.Automatic.LowBattery = false }},
		{"missing_auto_countdown", func(o *Options) { o.Automatic.Countdown = "" }},
		{"short_auto_countdown", func(o *Options) { o.Automatic.Countdown = "4.999s" }},
		{"long_auto_countdown", func(o *Options) { o.Automatic.Countdown = "10m1ns" }},
		{"bad_outage", func(o *Options) { o.Automatic.OutageAfter = "ten" }},
		{"short_outage", func(o *Options) { o.Automatic.OutageAfter = "999ms" }},
		{"long_outage", func(o *Options) { o.Automatic.OutageAfter = "24h1ns" }},
		{"missing_manual_countdown", func(o *Options) { o.Manual.Countdown = "" }},
		{"short_manual_countdown", func(o *Options) { o.Manual.Countdown = "4s" }},
		{"long_manual_countdown", func(o *Options) { o.Manual.Countdown = "11m" }},
		{"zero_user", func(o *Options) { o.Manual.AllowedUserIDs = []int64{0} }},
		{"negative_user", func(o *Options) { o.Manual.AllowedUserIDs = []int64{-1} }},
		{"duplicate_users", func(o *Options) { o.Manual.AllowedUserIDs = []int64{1, 1} }},
		{"too_many_users", func(o *Options) {
			for i := int64(1); i <= 17; i++ {
				o.Manual.AllowedUserIDs = append(o.Manual.AllowedUserIDs, i)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := withManual(automatic("live"))
			tc.edit(&o)
			if err := o.Validate(); err == nil {
				t.Fatal("invalid options accepted")
			}
			if _, err := New(o, func(time.Time) Input { return Input{} }, nil, nil); err == nil {
				t.Fatal("constructor accepted invalid options")
			}
		})
	}
	for _, o := range []Options{
		{},
		{Automatic: &AutomaticOptions{}, Manual: &ManualOptions{}},
		automatic("dry_run"),
		{Mode: "live", Automatic: &AutomaticOptions{Enabled: true, LowBattery: true, Countdown: "10m"}},
		{Mode: "live", Automatic: &AutomaticOptions{Enabled: true, OutageAfter: "1s", Countdown: "5s"}},
		{Mode: "live", Automatic: &AutomaticOptions{Enabled: true, OutageAfter: "24h", Countdown: "10m"}},
		withManual(Options{Mode: "dry_run"}),
		{Mode: "dry_run", Manual: &ManualOptions{Enabled: true, Countdown: "5s"}},
	} {
		if err := o.Validate(); err != nil {
			t.Fatalf("valid options rejected: %+v: %v", o, err)
		}
		if o.Enabled() != (o.AutomaticEnabled() || o.ManualEnabled()) {
			t.Fatal("enablement helpers inconsistent")
		}
	}
}

func TestRequiredCallbacks(t *testing.T) {
	read := func(time.Time) Input { return Input{} }
	if _, err := New(automatic("live"), read, nil, nil); err == nil {
		t.Fatal("live mode accepted nil requester")
	}
	if _, err := New(automatic("dry_run"), nil, nil, nil); err == nil {
		t.Fatal("enabled mode accepted nil reader")
	}
	if _, err := New(automatic("dry_run"), read, nil, nil); err != nil {
		t.Fatal("dry run requires adapter:", err)
	}
}
