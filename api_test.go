package main

import (
	"encoding/json"
	"testing"
)

func TestDecodeResponsePreservesOrder(t *testing.T) {
	r, err := decodeResponse([]byte(`{"ip":"8.8.8.8","country_code":"US","city_name":"Mountain View"}`))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}

	want := []string{"ip", "country_code", "city_name"}
	if len(r) != len(want) {
		t.Fatalf("got %d fields, want %d", len(r), len(want))
	}
	for i, k := range want {
		if r[i].Key != k {
			t.Errorf("field %d = %q, want %q", i, r[i].Key, k)
		}
	}
}

func TestDecodeResponseNestedOrder(t *testing.T) {
	r, err := decodeResponse([]byte(`{"country":{"name":"US","currency":{"code":"USD"}}}`))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}

	v, _ := r.Get("country")
	country, ok := v.(Response)
	if !ok {
		t.Fatalf("country is %T, want Response", v)
	}
	if len(country) != 2 || country[0].Key != "name" || country[1].Key != "currency" {
		t.Errorf("nested order lost: %v", country)
	}
}

func TestDecodeResponseKeepsNumbersVerbatim(t *testing.T) {
	r, err := decodeResponse([]byte(`{"latitude":37.38605,"population":339665118}`))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}

	if v, _ := r.Get("latitude"); v != json.Number("37.38605") {
		t.Errorf("latitude = %v (%T)", v, v)
	}
	if v, _ := r.Get("population"); v != json.Number("339665118") {
		t.Errorf("population = %v (%T)", v, v)
	}
}

func TestDecodeResponseRejectsNonObject(t *testing.T) {
	if _, err := decodeResponse([]byte(`[1,2,3]`)); err == nil {
		t.Error("want an error for a JSON array")
	}
}

func TestDecodeAPIErrorEnvelope(t *testing.T) {
	r, err := decodeResponse([]byte(`{"error":{"error_code":10001,"error_message":"Invalid IP address."}}`))
	if err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}

	raw, ok := r.Get("error")
	if !ok {
		t.Fatal("error field missing")
	}

	apiErr, ok := decodeAPIError(raw).(*APIError)
	if !ok {
		t.Fatalf("want *APIError")
	}
	if apiErr.Code != 10001 {
		t.Errorf("code = %d, want 10001", apiErr.Code)
	}
	if apiErr.Message != "Invalid IP address." {
		t.Errorf("message = %q", apiErr.Message)
	}
	if apiErr.Hint() == "" {
		t.Error("expected a hint for 10001")
	}
}
