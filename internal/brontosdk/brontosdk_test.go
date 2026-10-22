package brontosdk

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serve answers every request with status and body, and hands each request to
// inspect first.
func serve(t *testing.T, status int, body string, inspect func(*http.Request)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inspect != nil {
			inspect(r)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	// A trailing slash on the endpoint must not double up in the path.
	return NewClient(server.URL+"/", "secret", server.Client())
}

func TestClientSendsTheAPIKey(t *testing.T) {
	var got *http.Request
	client := serve(t, http.StatusOK, `{"logs":[]}`, func(r *http.Request) { got = r })

	if _, err := client.Datasets(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/logs" {
		t.Errorf("path = %q", got.URL.Path)
	}
	for header, want := range map[string]string{
		"x-bronto-api-key": "secret",
		"User-Agent":       "bronto-mcp",
		"Content-Type":     "application/json",
	} {
		if value := got.Header.Get(header); value != want {
			t.Errorf("%s = %q, want %q", header, value, want)
		}
	}
}

func TestClientErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		check   func(error) bool
		details string
	}{
		{
			name:    "status with details",
			status:  http.StatusForbidden,
			body:    `{"details":"key lacks scope"}`,
			details: "key lacks scope",
		},
		{
			name:   "status without a JSON body",
			status: http.StatusBadGateway,
			body:   `<html>bad gateway</html>`,
		},
		{
			name:   "success that is not JSON",
			status: http.StatusOK,
			body:   `not json`,
			check:  func(err error) bool { return errors.Is(err, ErrUnexpectedResponse) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := serve(t, test.status, test.body, nil)
			_, err := client.Monitors(t.Context())
			if test.check != nil {
				if !test.check(err) {
					t.Errorf("err = %v", err)
				}
				return
			}
			httpErr, ok := errors.AsType[*HTTPError](err)
			if !ok {
				t.Fatalf("err = %v, want *HTTPError", err)
			}
			if httpErr.StatusCode != test.status || httpErr.Details != test.details {
				t.Errorf("err = %+v", httpErr)
			}
		})
	}
}

func TestListsAcceptABareArray(t *testing.T) {
	client := serve(t, http.StatusOK, `[{"id":"m-1"}]`, nil)
	monitors, err := client.Monitors(t.Context())
	if err != nil || len(monitors) != 1 {
		t.Errorf("monitors = %v, err = %v", monitors, err)
	}
}

func TestMonitorEventsEscapesTheID(t *testing.T) {
	var path string
	client := serve(t, http.StatusOK, `{"events":[]}`, func(r *http.Request) { path = r.URL.EscapedPath() })

	if _, err := client.MonitorEvents(t.Context(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if path != "/monitors/a%2Fb/events" {
		t.Errorf("path = %q", path)
	}
}
