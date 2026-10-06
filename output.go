package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Friendly field names
var fieldLabels = map[string]string{
	"ip":                   "IP Address",
	"country_code":         "Country Code",
	"country_name":         "Country",
	"region_name":          "Region",
	"district":             "District",
	"city_name":            "City",
	"latitude":             "Latitude",
	"longitude":            "Longitude",
	"zip_code":             "ZIP Code",
	"time_zone":            "Time Zone",
	"asn":                  "ASN",
	"as":                   "AS",
	"as_domain":            "AS Domain",
	"as_cidr":              "AS CIDR",
	"as_usage_type":        "AS Usage Type",
	"isp":                  "ISP",
	"domain":               "Domain",
	"net_speed":            "Net Speed",
	"idd_code":             "IDD Code",
	"area_code":            "Area Code",
	"weather_station_code": "Weather Station Code",
	"weather_station_name": "Weather Station",
	"mcc":                  "MCC",
	"mnc":                  "MNC",
	"mobile_brand":         "Mobile Brand",
	"elevation":            "Elevation (m)",
	"usage_type":           "Usage Type",
	"address_type":         "Address Type",
	"category":             "IAB Category",
	"is_proxy":             "Is Proxy",
	"fraud_score":          "Fraud Score",
}

// Render output
func render(w io.Writer, results []Response, format string, fields []string) error {
	if len(fields) > 0 {
		for i, r := range results {
			results[i] = project(r, fields)
		}
	}

	switch format {
	case "json":
		return renderJSON(w, results)
	case "csv":
		return renderCSV(w, results)
	default:
		return renderTable(w, results)
	}
}

// Render JSON outputs
func renderJSON(w io.Writer, results []Response) error {
	var payload any
	if len(results) == 1 {
		payload = results[0]
	} else {
		payload = results
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

// Render CSV, one row per IP
func renderCSV(w io.Writer, results []Response) error {
	rows := make([][]Field, 0, len(results))

	var cols []string
	seen := map[string]bool{}

	for _, r := range results {
		row := flattenResponse(r)
		rows = append(rows, row)

		for _, f := range row {
			if !seen[f.Key] {
				seen[f.Key] = true
				cols = append(cols, f.Key)
			}
		}
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}

	for _, row := range rows {
		vals := make(map[string]string, len(row))
		for _, f := range row {
			vals[f.Key] = scalarString(f.Value)
		}

		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = vals[c]
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

func renderTable(w io.Writer, results []Response) error {
	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if err := renderTableOne(w, r); err != nil {
			return err
		}
	}
	return nil
}

func renderTableOne(w io.Writer, r Response) error {
	type tableRow struct{ label, value string }

	var rows []tableRow
	for _, f := range r {
		if f.Key == "message" {
			continue
		}

		var collected []Field
		collectRows(f.Key, f.Value, &collected)
		for _, c := range collected {
			rows = append(rows, tableRow{labelFor(c.Key), scalarString(c.Value)})
		}
	}

	if len(rows) == 0 {
		return nil
	}

	width := 0
	for _, row := range rows {
		if n := len(row.label); n > width {
			width = n
		}
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%-*s  %s\n", width, row.label, row.value); err != nil {
			return err
		}
	}
	return nil
}

// Drops nulls and meaningless fields
func collectRows(path string, v any, rows *[]Field) {
	switch t := v.(type) {
	case Response:
		for _, f := range t {
			if skipInTable(f.Key) {
				continue
			}
			collectRows(path+"."+f.Key, f.Value, rows)
		}
	case nil:
	case []any:
		if len(t) == 0 {
			return
		}
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, scalarString(item))
		}
		*rows = append(*rows, Field{path, strings.Join(parts, ", ")})
	default:
		*rows = append(*rows, Field{path, t})
	}
}

// Drops translation strings and flag image URLs
func skipInTable(key string) bool {
	return key == "translation" || key == "flag"
}

func labelFor(path string) string {
	if l, ok := fieldLabels[path]; ok {
		return l
	}
	return path
}

// Flatten a response into dotted keys, dropping the keyless notice
func flattenResponse(r Response) []Field {
	var out []Field
	for _, f := range r {
		if f.Key == "message" {
			continue
		}
		flatten(f.Key, f.Value, &out)
	}
	return out
}

// Flatten nested objects into dotted keys
func flatten(prefix string, v any, out *[]Field) {
	switch t := v.(type) {
	case Response:
		for _, f := range t {
			flatten(prefix+"."+f.Key, f.Value, out)
		}
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, scalarString(item))
		}
		*out = append(*out, Field{prefix, strings.Join(parts, "; ")})
	case nil:
		*out = append(*out, Field{prefix, ""})
	default:
		*out = append(*out, Field{prefix, t})
	}
}

// Keeps only the requested dotted paths, in response order
func project(r Response, paths []string) Response {
	var out Response
	for _, f := range r {
		sub, ok := matchPaths(paths, f.Key)
		if !ok {
			continue
		}

		if sub == nil {
			out = append(out, f)
			continue
		}

		child, ok := f.Value.(Response)
		if !ok {
			continue
		}
		if pruned := project(child, sub); len(pruned) > 0 {
			out = append(out, Field{f.Key, pruned})
		}
	}
	return out
}

// Reports whether key is wanted; nil sub means the whole subtree
func matchPaths(paths []string, key string) ([]string, bool) {
	var sub []string
	for _, p := range paths {
		head, tail, _ := strings.Cut(p, ".")
		if head != key {
			continue
		}
		if tail == "" {
			return nil, true
		}
		sub = append(sub, tail)
	}

	if len(sub) == 0 {
		return nil, false
	}
	return sub, true
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}
