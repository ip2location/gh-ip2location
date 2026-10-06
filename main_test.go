package main

import "testing"

func TestParseArgsAllowsFlagsAfterIP(t *testing.T) {
	o, err := parseArgs([]string{"8.8.8.8", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.format != "json" {
		t.Errorf("format = %q, want json", o.format)
	}
	if len(o.ips) != 1 || o.ips[0] != "8.8.8.8" {
		t.Errorf("ips = %v, want [8.8.8.8]", o.ips)
	}
}

func TestParseArgsMultipleIPsAndShorthand(t *testing.T) {
	o, err := parseArgs([]string{"8.8.8.8", "1.1.1.1", "--csv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.format != "csv" {
		t.Errorf("format = %q, want csv", o.format)
	}
	if len(o.ips) != 2 {
		t.Errorf("ips = %v, want two entries", o.ips)
	}
}

func TestParseArgsInlineValue(t *testing.T) {
	o, err := parseArgs([]string{"--key=abc123", "--fields=ip, country_name", "8.8.8.8"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.key != "abc123" {
		t.Errorf("key = %q, want abc123", o.key)
	}
	if len(o.fields) != 2 || o.fields[1] != "country_name" {
		t.Errorf("fields = %v, want [ip country_name]", o.fields)
	}
}

func TestParseArgsNoIPMeansEmpty(t *testing.T) {
	o, err := parseArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(o.ips) != 0 {
		t.Errorf("ips = %v, want none", o.ips)
	}
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown format", []string{"--format", "yaml"}},
		{"missing value", []string{"8.8.8.8", "--key"}},
		{"unknown flag", []string{"--nope"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseArgs(tt.args); err == nil {
				t.Error("want an error, got nil")
			}
		})
	}
}

func TestAPIKeyPrecedence(t *testing.T) {
	t.Setenv("IP2LOCATION_API_KEY", "from-env")

	o := &options{}
	if got := o.apiKey(); got != "from-env" {
		t.Errorf("apiKey = %q, want from-env", got)
	}

	o = &options{key: "from-flag"}
	if got := o.apiKey(); got != "from-flag" {
		t.Errorf("apiKey = %q, want from-flag", got)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" ip , country_name ,, city_name ")
	want := []string{"ip", "country_name", "city_name"}
	if len(got) != len(want) {
		t.Fatalf("splitList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitList[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
