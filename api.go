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

// Response for geolocation result
type Response map[string]any

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

	// Keeps coordinates and codes verbatim.
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var out Response
	if err := dec.Decode(&out); err != nil {
		// A non-JSON body on a bad status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d from IP2Location.io: %s", resp.StatusCode, firstLine(body))
		}
		return nil, fmt.Errorf("cannot parse response: %w", err)
	}

	// Failures as error
	if raw, ok := out["error"]; ok {
		return nil, decodeAPIError(raw)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from IP2Location.io: %s", resp.StatusCode, firstLine(body))
	}

	return out, nil
}

func decodeAPIError(raw any) error {
	obj, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected error response from IP2Location.io")
	}

	e := &APIError{}
	if n, ok := obj["error_code"].(json.Number); ok {
		if v, err := strconv.Atoi(n.String()); err == nil {
			e.Code = v
		}
	}
	if s, ok := obj["error_message"].(string); ok {
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
