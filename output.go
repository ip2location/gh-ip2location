package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Display order for known fields
var fieldOrder = []string{"ip", "country_code", "country_name", "region_name", "district", "city_name", "latitude", "longitude", "zip_code", "time_zone", "asn", "as", "as_domain", "as_cidr", "as_usage_type", "isp", "domain", "net_speed", "idd_code", "area_code", "weather_station_code", "weather_station_name", "mcc", "mnc", "mobile_brand", "elevation", "usage_type", "address_type", "category", "is_proxy", "fraud_score", "continent", "country", "region", "city", "time_zone_info", "geotargeting", "proxy"}

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

func renderCSV(w io.Writer, results []Response) error {
	rows := make([]map[string]string, 0, len(results))
	seen := map[string]bool{}

	for _, r := range results {
		flat := map[string]string{}
		for k, v := range r {
			// The keyless notice
			if k == "message" {
				continue
			}
			flatten(k, v, flat)
		}
		rows = append(rows, flat)

		for k := range flat {
			seen[k] = true
		}
	}

	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sortByCanonical(cols)

	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}

	for _, row := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = row[c]
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
	for _, k := range orderedKeys(r) {
		if k == "message" {
			continue
		}

		var collected [][2]string
		collectRows(k, r[k], &collected)
		for _, c := range collected {
			rows = append(rows, tableRow{labelFor(c[0]), c[1]})
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
func collectRows(path string, v any, rows *[][2]string) {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(t) {
			if skipInTable(k) {
				continue
			}
			collectRows(path+"."+k, t[k], rows)
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
		*rows = append(*rows, [2]string{path, strings.Join(parts, ", ")})
	default:
		*rows = append(*rows, [2]string{path, scalarString(t)})
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

// Flatten nested objects into dotted keys
func flatten(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(t) {
			flatten(prefix+"."+k, t[k], out)
		}
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, scalarString(item))
		}
		out[prefix] = strings.Join(parts, "; ")
	case nil:
		out[prefix] = ""
	default:
		out[prefix] = scalarString(t)
	}
}

// Keeps only the requested dotted paths
func project(r Response, paths []string) Response {
	out := Response{}
	for _, p := range paths {
		copyPath(out, r, strings.Split(p, "."))
	}
	return out
}

func copyPath(dst, src map[string]any, parts []string) {
	key := parts[0]

	v, ok := src[key]
	if !ok {
		return
	}

	if len(parts) == 1 {
		dst[key] = v
		return
	}

	child, ok := v.(map[string]any)
	if !ok {
		return
	}

	sub, ok := dst[key].(map[string]any)
	if !ok {
		sub = map[string]any{}
		dst[key] = sub
	}

	copyPath(sub, child, parts[1:])
}

func orderedKeys(r Response) []string {
	keys := make([]string, 0, len(r))
	for k := range r {
		keys = append(keys, k)
	}
	sortByCanonical(keys)
	return keys
}

// Sort by field order, then alphabetically
func sortByCanonical(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := fieldRank(keys[i]), fieldRank(keys[j])
		if ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
}

// Key's position in fieldOrder
func fieldRank(key string) int {
	base := key
	if i := strings.Index(base, "."); i >= 0 {
		base = base[:i]
	}

	for i, f := range fieldOrder {
		if f == base {
			return i
		}
	}
	return len(fieldOrder)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
