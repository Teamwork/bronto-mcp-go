package bronto

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

const savedSearchesBody = `{"saved_searches":[
	{"id":"s-1","name":"Errors"},
	{"id":"s-2","name":"Slow requests"},
	{"id":"s-3","name":"Error budget"}
]}`

func TestGetSavedSearchesFiltersAndLimits(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /saved-searches": {body: savedSearchesBody}})

	var result struct {
		SavedSearches []map[string]any `json:"saved_searches"`
		Total         int              `json:"total"`
		Shown         int              `json:"shown"`
		Truncated     bool             `json:"truncated"`
	}
	callTool(t, server, "get_saved_searches", map[string]any{"query": "error", "limit": 1}, &result)

	if result.Total != 2 || result.Shown != 1 || !result.Truncated || result.SavedSearches[0]["id"] != "s-1" {
		t.Errorf("result = %+v", result)
	}
}

func TestGetSavedSearchByID(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"GET /saved-searches/s 1": {body: `{"saved_search":{"id":"s 1"}}`},
	})

	var result map[string]any
	callTool(t, server, "get_saved_searches", map[string]any{"saved_search_id": "s 1"}, &result)
	if result["saved_search"] == nil {
		t.Errorf("result = %v", result)
	}
}

func TestCreateSavedSearch(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"POST /saved-searches": {status: http.StatusCreated, body: `{"id":"s-9"}`},
	})

	var result map[string]map[string]any
	callTool(t, server, "create_saved_search", map[string]any{
		"name":          "Errors",
		"log_ids":       []string{"a", "b"},
		"search_filter": `"level"='error'`,
	}, &result)
	if result["saved_search"]["id"] != "s-9" {
		t.Errorf("result = %v", result)
	}

	body := fake.request(t, http.MethodPost, "/saved-searches").Body
	details := body["search_details"].(map[string]any)
	if details["from"] != "a,b" || details["select"] != defaultSavedSearchSelect || details["where"] == nil {
		t.Errorf("search_details = %v", details)
	}
	if _, ok := details["time_range"]; ok {
		t.Error("an unset time_range was sent")
	}
	if ids := body["log_ids"].([]any); len(ids) != 2 {
		t.Errorf("log_ids = %v", ids)
	}
}

func TestRunSavedSearch(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		// A UI-created search: its datasets live only in the top-level log_ids,
		// and its select is a comma-separated string.
		"GET /saved-searches/s-1": {body: `{"saved_search":{"id":"s-1","log_ids":["a","b"],
			"search_details":{"where":" \"level\"='error' ","select":"*, @raw","timeRange":"Last 1 hour"}}}`},
		"POST /search": {body: `{
			"events":[{"@raw":"boom","@time":1,"@status":"error","message_kvs":{"k":"v"},"links":["x"]}],
			"result":[{"@raw":"boom"}],
			"explain":{"cost":1}
		}`},
	})

	var result struct {
		Events  []map[string]any `json:"events"`
		Explain map[string]any   `json:"explain"`
		Result  any              `json:"result"`
	}
	callTool(t, server, "run_saved_search", map[string]any{"saved_search_id": "s-1", "limit": 5}, &result)

	body := fake.request(t, http.MethodPost, "/search").Body
	if from := body["from"].([]any); len(from) != 2 {
		t.Errorf("from = %v", body["from"])
	}
	if body["where"] != `"level"='error'` || body["time_range"] != "Last 1 hour" || body["limit"] != float64(5) {
		t.Errorf("body = %v", body)
	}
	if selected := body["select"].([]any); !slices.Equal(selected, []any{"*", "@raw"}) {
		t.Errorf("select = %v", selected)
	}

	if len(result.Events) != 1 || result.Events[0]["links"] != nil || result.Events[0]["@raw"] != "boom" {
		t.Errorf("events = %v", result.Events)
	}
	if result.Result != nil || result.Explain == nil {
		t.Errorf("result = %+v", result)
	}
}

func TestRunSavedSearchTimeRangeOverride(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"GET /saved-searches/s-1": {body: `{"id":"s-1","search_details":{"from":"a","time_range":"Last 1 hour"}}`},
		"POST /search":            {body: `{"totals":{"count":1},"groups_series":[]}`},
	})

	var result map[string]any
	callTool(t, server, "run_saved_search",
		map[string]any{"saved_search_id": "s-1", "time_range": "Last 7 days"}, &result)

	if got := fake.request(t, http.MethodPost, "/search").Body["time_range"]; got != "Last 7 days" {
		t.Errorf("time_range = %v", got)
	}
	if result["totals"] == nil || result["events"] != nil {
		t.Errorf("aggregate result = %v", result)
	}
}

func TestRunSavedSearchWithoutDetails(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /saved-searches/s-1": {body: `{"id":"s-1"}`}})

	message := callToolError(t, server, "run_saved_search", map[string]any{"saved_search_id": "s-1"})
	if !strings.Contains(message, "no search_details") {
		t.Errorf("message = %q", message)
	}
}
