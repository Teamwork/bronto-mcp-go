package bronto

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/mcp/pkg/testutil"
)

// route is one canned Bronto answer.
type route struct {
	status int
	body   string
}

// recordedRequest is a request the fake Bronto received.
type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// fakeBronto answers "METHOD /path" with a canned body and records every
// request, so a test can assert on what a tool sent as well as what it made
// of the answer. Unrouted requests get a 404.
type fakeBronto struct {
	mu       sync.Mutex
	routes   map[string]route
	requests []recordedRequest
}

func (f *fakeBronto) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recorded := recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		_ = json.Unmarshal(raw, &recorded.Body)
	}

	f.mu.Lock()
	f.requests = append(f.requests, recorded)
	answer, ok := f.routes[r.Method+" "+r.URL.Path]
	f.mu.Unlock()

	if !ok {
		answer = route{status: http.StatusNotFound, body: `{"details":"no such route"}`}
	}
	if answer.status == 0 {
		answer.status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(answer.status)
	_, _ = io.WriteString(w, answer.body)
}

// request returns the first recorded request to method and path.
func (f *fakeBronto) request(t *testing.T, method, path string) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.Method == method && r.Path == path {
			return r
		}
	}
	t.Fatalf("no %s %s request was made; got %+v", method, path, f.requests)
	return recordedRequest{}
}

// count returns how many requests went to method and path.
func (f *fakeBronto) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

// newTestServer builds an MCP server with every toolset, its write tools
// included, talking to a fake Bronto that answers with routes.
func newTestServer(t *testing.T, routes map[string]route) (*mcp.Server, *fakeBronto) {
	t.Helper()
	fake := &fakeBronto{routes: routes}
	backend := httptest.NewServer(fake)
	t.Cleanup(backend.Close)

	client := brontosdk.NewClient(backend.URL, "test-key", backend.Client())
	return testutil.MCPServer(t, DefaultToolsetGroup(false, client)), fake
}

// fixClock pins the clock the search and coverage tools read.
func fixClock(t *testing.T, at time.Time) {
	t.Helper()
	previous := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = previous })
}

// callTool runs a tool expecting success and decodes its JSON result into out.
func callTool(t *testing.T, server *mcp.Server, tool string, args map[string]any, out any) {
	t.Helper()
	text := resultText(t, server, tool, args, false)
	if err := json.Unmarshal([]byte(text), out); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, text)
	}
}

// callToolError runs a tool expecting an error tool result and returns its text.
func callToolError(t *testing.T, server *mcp.Server, tool string, args map[string]any) string {
	t.Helper()
	return resultText(t, server, tool, args, true)
}

func resultText(t *testing.T, server *mcp.Server, tool string, args map[string]any, wantError bool) string {
	t.Helper()
	var text string
	testutil.ExecuteToolRequest(t, server, tool, args,
		testutil.ExecuteToolRequestWithCheckMessage(func(t *testing.T, result mcp.Result) {
			t.Helper()
			toolResult, ok := result.(*mcp.CallToolResult)
			if !ok {
				t.Fatalf("unexpected result type %T", result)
			}
			for _, content := range toolResult.Content {
				if content, ok := content.(*mcp.TextContent); ok {
					text += content.Text
				}
			}
			if toolResult.IsError != wantError {
				t.Fatalf("IsError = %v, want %v: %s", toolResult.IsError, wantError, text)
			}
		}),
	)
	return text
}
