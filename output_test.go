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
		{"ip", "8.8.8.8"},
		{"country_code", "US"},
		{"city_name", "Mountain View"},
		{"is_proxy", false},
		{"message", "Limit to 1,000 queries per day."},
		{"country", Response{
			{"name", "United States of America"},
			{"currency", Response{{"code", "USD"}}},
		}},
		{"continent", Response{
			{"name", "North America"},
			{"hemisphere", []any{"north", "west"}},
			{"translation", Response{{"lang", nil}, {"value", nil}}},
		}},
	}
}

func sampleKeys(r Response) []string {
	keys := make([]string, 0, len(r))
	for _, f := range r {
		keys = append(keys, f.Key)
	}
	return keys
}

func TestProjectKeepsNestedPathOnly(t *testing.T) {
	got := project(sample(), []string{"ip", "country.currency.code"})

	if v, _ := got.String("ip"); v != "8.8.8.8" {
		t.Errorf("ip = %q, want 8.8.8.8", v)
	}

	raw, ok := got.Get("country")
	if !ok {
		t.Fatalf("country missing from projection: %v", sampleKeys(got))
	}
	country, ok := raw.(Response)
	if !ok {
		t.Fatalf("country is %T, want Response", raw)
	}
	if _, ok := country.Get("name"); ok {
		t.Error("country.name should have been pruned")
	}

	raw, ok = country.Get("currency")
	if !ok {
		t.Fatal("country.currency missing")
	}
	currency, _ := raw.(Response)
	if v, _ := currency.String("code"); v != "USD" {
		t.Errorf("country.currency.code = %q, want USD", v)
	}
}

func TestProjectKeepsWholeSubtree(t *testing.T) {
	got := project(sample(), []string{"country"})

	raw, ok := got.Get("country")
	if !ok {
		t.Fatal("country missing")
	}
	country, _ := raw.(Response)
	if _, ok := country.Get("name"); !ok {
		t.Error("naming a parent should keep its whole subtree")
	}
}

func TestProjectPreservesResponseOrder(t *testing.T) {
	// Requested in a different order than the response carries them.
	got := project(sample(), []string{"country_code", "ip"})

	want := []string{"ip", "country_code"}
	gotKeys := sampleKeys(got)
	if len(gotKeys) != len(want) {
		t.Fatalf("keys = %v, want %v", gotKeys, want)
	}
	for i := range want {
		if gotKeys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", gotKeys, want)
		}
	}
}

func TestMatchPaths(t *testing.T) {
	if sub, ok := matchPaths([]string{"country"}, "country"); !ok || sub != nil {
		t.Errorf("whole subtree match = %v, %v; want nil, true", sub, ok)
	}
	if sub, ok := matchPaths([]string{"country.currency.code"}, "country"); !ok || len(sub) != 1 || sub[0] != "currency.code" {
		t.Errorf("nested match = %v, %v", sub, ok)
	}
	if _, ok := matchPaths([]string{"country"}, "city_name"); ok {
		t.Error("unrelated key should not match")
	}
}

func TestRenderJSONPreservesFieldOrder(t *testing.T) {
	var buf bytes.Buffer
	if err := renderJSON(&buf, []Response{sample()}); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "{\n  \"ip\":") {
		t.Errorf("first field should be ip, got:\n%s", out)
	}

	// Alphabetical ordering would put city_name first.
	if strings.Index(out, `"ip"`) > strings.Index(out, `"city_name"`) {
		t.Error("ip should come before city_name")
	}
	if strings.Index(out, `"country_code"`) > strings.Index(out, `"city_name"`) {
		t.Error("country_code should come before city_name")
	}
	if strings.Index(out, `"country"`) > strings.Index(out, `"continent"`) {
		t.Error("country should come before continent")
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestRenderJSONArrayForMultipleIPs(t *testing.T) {
	results := []Response{{{Key: "ip", Value: "8.8.8.8"}}, {{Key: "ip", Value: "1.1.1.1"}}}

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

func TestRenderJSONNoHTMLEscaping(t *testing.T) {
	resp := Response{{Key: "ip", Value: "8.8.8.8"}, {Key: "org", Value: "A & B"}}

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
	if decoded["org"] != "A & B" {
		t.Errorf("org = %v, want %q", decoded["org"], "A & B")
	}
}

func TestRenderCSVFollowsResponseOrder(t *testing.T) {
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
	idx := map[string]int{}
	for i, c := range header {
		idx[c] = i
		if c == "message" {
			t.Error("message column should not appear in CSV")
		}
	}

	// Alphabetical ordering would put city_name first.
	if header[0] != "ip" || header[1] != "country_code" {
		t.Errorf("header should follow response order, got %v", header)
	}
	if idx["country_code"] > idx["city_name"] {
		t.Error("country_code should come before city_name")
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
	first := Response{{Key: "ip", Value: "8.8.8.8"}}
	second := Response{{Key: "ip", Value: "1.1.1.1"}, {Key: "fraud_score", Value: json.Number("12")}}

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
	if recs[2][1] != "12" {
		t.Errorf("fraud_score = %q, want 12", recs[2][1])
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
	var out []Field
	flatten("hemisphere", []any{"north", "west"}, &out)

	if len(out) != 1 {
		t.Fatalf("got %d fields, want 1", len(out))
	}
	if out[0].Value != "north; west" {
		t.Errorf("flatten = %v, want %q", out[0].Value, "north; west")
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
