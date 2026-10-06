package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func sample() Response {
	return Response{
		"ip":           "8.8.8.8",
		"country_code": "US",
		"city_name":    "Mountain View",
		"is_proxy":     false,
		"message":      "Limit to 1,000 queries per day.",
		"country": map[string]any{
			"name": "United States of America",
			"currency": map[string]any{
				"code": "USD",
			},
		},
		"continent": map[string]any{
			"name":        "North America",
			"hemisphere":  []any{"north", "west"},
			"translation": map[string]any{"lang": nil, "value": nil},
		},
	}
}

func TestProjectKeepsNestedPathOnly(t *testing.T) {
	got := project(sample(), []string{"ip", "country.currency.code"})

	if got["ip"] != "8.8.8.8" {
		t.Errorf("ip = %v, want 8.8.8.8", got["ip"])
	}

	country, ok := got["country"].(map[string]any)
	if !ok {
		t.Fatalf("country missing from projection: %v", got)
	}
	if _, ok := country["name"]; ok {
		t.Error("country.name should have been pruned")
	}

	currency, ok := country["currency"].(map[string]any)
	if !ok || currency["code"] != "USD" {
		t.Errorf("country.currency.code = %v, want USD", currency)
	}
}

func TestProjectKeepsWholeSubtree(t *testing.T) {
	got := project(sample(), []string{"country"})
	country, ok := got["country"].(map[string]any)
	if !ok || country["name"] == nil {
		t.Errorf("whole country subtree should be kept, got %v", got["country"])
	}
}

func TestFieldRankGroupsNestedUnderParent(t *testing.T) {
	if fieldRank("country.currency.code") != fieldRank("country") {
		t.Error("nested key should rank with its top-level parent")
	}
	if fieldRank("country_code") == fieldRank("country") {
		t.Error("country_code must not rank with the country object")
	}
}

func TestSortByCanonical(t *testing.T) {
	keys := []string{"zzz_unknown", "city_name", "aaa_unknown", "ip", "country_code"}
	sortByCanonical(keys)

	want := []string{"ip", "country_code", "city_name", "aaa_unknown", "zzz_unknown"}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
}

func TestRenderCSVSkipsMessageAndOrdersColumns(t *testing.T) {
	var buf bytes.Buffer
	if err := renderCSV(&buf, []Response{sample()}); err != nil {
		t.Fatalf("renderCSV: %v", err)
	}

	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d rows, want header + 1", len(recs))
	}

	header := recs[0]
	for _, c := range header {
		if c == "message" {
			t.Error("message column should not appear in CSV")
		}
	}

	if header[0] != "ip" {
		t.Errorf("first column = %q, want ip", header[0])
	}

	idx := map[string]int{}
	for i, c := range header {
		idx[c] = i
	}

	if got := recs[1][idx["country.currency.code"]]; got != "USD" {
		t.Errorf("country.currency.code = %q, want USD", got)
	}
	if got := recs[1][idx["continent.hemisphere"]]; got != "north; west" {
		t.Errorf("continent.hemisphere = %q, want %q", got, "north; west")
	}
	if got := recs[1][idx["is_proxy"]]; got != "false" {
		t.Errorf("is_proxy = %q, want false", got)
	}
}

func TestRenderCSVUnionsColumnsAcrossIPs(t *testing.T) {
	first := Response{"ip": "8.8.8.8"}
	second := Response{"ip": "1.1.1.1", "fraud_score": json.Number("12")}

	var buf bytes.Buffer
	if err := renderCSV(&buf, []Response{first, second}); err != nil {
		t.Fatalf("renderCSV: %v", err)
	}

	recs, _ := csv.NewReader(&buf).ReadAll()
	if len(recs) != 3 {
		t.Fatalf("got %d rows, want header + 2", len(recs))
	}
	if len(recs[0]) != 2 {
		t.Errorf("header = %v, want both ip and fraud_score", recs[0])
	}
	if recs[1][1] != "" {
		t.Errorf("missing value should be empty, got %q", recs[1][1])
	}
}

func TestRenderJSONSingleObjectAndNoHTMLEscaping(t *testing.T) {
	resp := Response{"ip": "8.8.8.8", "as": json.Number("1"), "org": "A & B"}

	var buf bytes.Buffer
	if err := renderJSON(&buf, []Response{resp}); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}

	if strings.Contains(buf.String(), `\u0026`) {
		t.Error("ampersand should not be HTML escaped")
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestRenderJSONArrayForMultipleIPs(t *testing.T) {
	results := []Response{{"ip": "8.8.8.8"}, {"ip": "1.1.1.1"}}

	var buf bytes.Buffer
	if err := renderJSON(&buf, results); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("want a JSON array, got: %s", buf.String())
	}
	if len(decoded) != 2 {
		t.Errorf("got %d entries, want 2", len(decoded))
	}
}

func TestRenderTableSkipsNoiseAndShowsNested(t *testing.T) {
	var buf bytes.Buffer
	if err := renderTable(&buf, []Response{sample()}); err != nil {
		t.Fatalf("renderTable: %v", err)
	}

	out := buf.String()
	for _, unwanted := range []string{"message", "translation", "Limit to"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("table output should not contain %q:\n%s", unwanted, out)
		}
	}
	for _, want := range []string{"IP Address", "8.8.8.8", "country.currency.code", "USD", "continent.hemisphere", "north, west"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestFlattenArray(t *testing.T) {
	out := map[string]string{}
	flatten("hemisphere", []any{"north", "west"}, out)
	if out["hemisphere"] != "north; west" {
		t.Errorf("flatten = %q, want %q", out["hemisphere"], "north; west")
	}
}

func TestScalarString(t *testing.T) {
	if got := scalarString(json.Number("37.38605")); got != "37.38605" {
		t.Errorf("json.Number = %q", got)
	}
	if got := scalarString(nil); got != "" {
		t.Errorf("nil = %q, want empty", got)
	}
	if got := scalarString(true); got != "true" {
		t.Errorf("bool = %q, want true", got)
	}
}
