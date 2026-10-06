package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const apiEndpoint = "https://api.ip2location.io/"

// Maximum body size
const maxBody = 1 << 20

// HTTP client
type Client struct {
	APIKey string
	Lang   string
	HTTP   *http.Client
}

func NewClient(apiKey, lang string) *Client {
	return &Client{
		APIKey: apiKey,
		Lang:   lang,
		HTTP:   &http.Client{Timeout: 20 * time.Second},
	}
}

// One key/value pair from the response
type Field struct {
	Key   string
	Value any
}

// Geolocation result, kept in the order the API returned it
type Response []Field

// Get returns a top-level field
func (r Response) Get(key string) (any, bool) {
	for _, f := range r {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

// String returns a top-level string field
func (r Response) String(key string) (string, bool) {
	v, ok := r.Get(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// MarshalJSON writes the fields in order
func (r Response) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	for i, f := range r {
		if i > 0 {
			buf.WriteByte(',')
		}

		key, err := marshalValue(f.Key)
		if err != nil {
			return nil, err
		}

		val, err := marshalValue(f.Value)
		if err != nil {
			return nil, err
		}

		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalValue(v any) ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// API Error
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("IP2Location.io error %d: %s", e.Code, e.Message)
}

// Hint for the error codes
func (e *APIError) Hint() string {
	switch e.Code {
	case 10000:
		return "set a valid key with: gh ip2location --set-key <key>"
	case 10001:
		return "expected a valid IPv4 or IPv6 address"
	}
	return ""
}

// Lookup the IP
func (c *Client) Lookup(ctx context.Context, ip string) (Response, error) {
	endpoint, err := url.Parse(apiEndpoint)
	if err != nil {
		return nil, err
	}

	q := endpoint.Query()
	if ip != "" {
		q.Set("ip", ip)
	}
	if c.APIKey != "" {
		q.Set("key", c.APIKey)
	}
	if c.Lang != "" {
		q.Set("lang", c.Lang)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	out, err := decodeResponse(body)
	if err != nil {
		// A non-JSON body on a bad status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d from IP2Location.io: %s", resp.StatusCode, firstLine(body))
		}
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}

	// Failures as error
	if raw, ok := out.Get("error"); ok {
		return nil, decodeAPIError(raw)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from IP2Location.io: %s", resp.StatusCode, firstLine(body))
	}

	return out, nil
}

// Decode a body into an ordered response
func decodeResponse(body []byte) (Response, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	// Keeps coordinates and codes verbatim
	dec.UseNumber()

	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}

	r, ok := v.(Response)
	if !ok {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return r, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}

	switch delim {
	case '{':
		var obj Response
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := keyTok.(string)

			val, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			obj = append(obj, Field{key, val})
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return obj, nil

	case '[':
		var arr []any
		for dec.More() {
			val, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return arr, nil
	}

	return nil, fmt.Errorf("unexpected JSON delimiter %q", delim)
}

func decodeAPIError(raw any) error {
	obj, ok := raw.(Response)
	if !ok {
		return fmt.Errorf("unexpected error response from IP2Location.io")
	}

	e := &APIError{}
	if n, ok := obj.Get("error_code"); ok {
		if num, ok := n.(json.Number); ok {
			if v, err := strconv.Atoi(num.String()); err == nil {
				e.Code = v
			}
		}
	}
	if s, ok := obj.String("error_message"); ok {
		e.Message = s
	}
	if e.Message == "" {
		e.Message = "unknown error"
	}

	return e
}

// Shorten body for error message
func firstLine(body []byte) string {
	s := string(bytes.TrimSpace(body))
	if i := len(s); i > 200 {
		s = s[:200] + "..."
	}
	return s
}
