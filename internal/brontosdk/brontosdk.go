// Package brontosdk is a small client for the Bronto API.
//
// It knows nothing about MCP: every operation returns ordinary Go values and
// errors, and the tools in internal/bronto decide what a caller sees.
package brontosdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
)

// DefaultEndpoint is the Bronto API the client talks to when none is given.
const DefaultEndpoint = "https://api.eu.bronto.io"

const userAgent = "bronto-mcp"

// ErrUnexpectedResponse is wrapped by every error caused by a response body that
// is not the JSON the client expected.
var ErrUnexpectedResponse = errors.New("unexpected Bronto response format")

// HTTPError is a response Bronto answered with a status other than 200 or 201.
type HTTPError struct {
	StatusCode int
	// Details is Bronto's own explanation, from the "details" field of the
	// error body, when it sent one.
	Details string
}

func (e *HTTPError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("bronto: status %d: %s", e.StatusCode, e.Details)
	}
	return fmt.Sprintf("bronto: status %d", e.StatusCode)
}

// Client calls the Bronto API with a single API key.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// NewClient returns a client for the API at endpoint. A nil httpClient uses
// http.DefaultClient.
//
// The API key travels in the x-bronto-api-key header, which a logging transport
// that only redacts Authorization would write out in full, so pass a client
// whose transport does not log request headers.
func NewClient(endpoint, apiKey string, httpClient *http.Client) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// do sends one request and decodes the JSON response. query may be nil, and a
// nil body sends none.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) (any, error) {
	target := c.endpoint + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("x-bronto-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Details: errorDetails(resp.Body)}
	}

	decoded, err := jsonvalue.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnexpectedResponse, err)
	}
	return decoded, nil
}

// errorDetails reads Bronto's explanation from an error body, if it has one.
func errorDetails(body io.Reader) string {
	decoded, err := jsonvalue.Decode(body)
	if err != nil {
		return ""
	}
	return jsonvalue.String(jsonvalue.Map(decoded)["details"])
}

// object asserts that a decoded response is a JSON object.
func object(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected an object, got %T", ErrUnexpectedResponse, v)
	}
	return m, nil
}
