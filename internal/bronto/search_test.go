package bronto

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSearchLogs(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"GET /search": {body: `{"events":[{
			"@raw":"GET /health 200","@status":"info","@time":1756063146000,
			"attributes":{"$host":"i-123"},"message_kvs":{"path":"/health"}
		}]}`},
	})

	var events []struct {
		Message    string         `json:"message"`
		Attributes map[string]any `json:"attributes"`
	}
	callTool(t, server, "search_logs", map[string]any{
		"log_ids":         []string{"a", "b"},
		"timerange_start": 1000,
		"timerange_end":   2000,
		"search_filter":   `"path"='/health'`,
	}, &events)

	query := fake.request(t, http.MethodGet, "/search").Query
	if got := query["from"]; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("from = %v", got)
	}
	if query.Get("from_ts") != "1000" || query.Get("to_ts") != "2000" {
		t.Errorf("time range = %s..%s", query.Get("from_ts"), query.Get("to_ts"))
	}
	if query.Get("where") != `"path"='/health'` {
		t.Errorf("where = %q", query.Get("where"))
	}
	if got := query["select"]; !slices.Equal(got, []string{"*", "@raw"}) {
		t.Errorf("select = %v", got)
	}

	if len(events) != 1 || events[0].Message != "GET /health 200" {
		t.Fatalf("events = %+v", events)
	}
	for _, key := range []string{"@status", "@time", "$host", "path"} {
		if _, ok := events[0].Attributes[key]; !ok {
			t.Errorf("attribute %s missing: %v", key, events[0].Attributes)
		}
	}
}

func TestSearchLogsDefaultsToTheLastTwentyMinutes(t *testing.T) {
	fixClock(t, time.UnixMilli(1_756_063_146_789))
	server, fake := newTestServer(t, map[string]route{"GET /search": {body: `{"events":[]}`}})

	var events []any
	callTool(t, server, "search_logs", map[string]any{"log_ids": []string{"a"}}, &events)

	query := fake.request(t, http.MethodGet, "/search").Query
	if got, want := query.Get("to_ts"), "1756063146000"; got != want {
		t.Errorf("to_ts = %s, want %s", got, want)
	}
	if got, want := query.Get("from_ts"), "1756061946000"; got != want {
		t.Errorf("from_ts = %s, want %s", got, want)
	}
}

func TestSearchLogsExplainsABadRequest(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"GET /search": {status: http.StatusBadRequest, body: `{"details":"unknown key \"pth\""}`},
	})

	message := callToolError(t, server, "search_logs", map[string]any{"log_ids": []string{"a"}})
	if !strings.Contains(message, "Check the filter syntax") || !strings.Contains(message, `unknown key "pth"`) {
		t.Errorf("message = %q", message)
	}
}

func TestTimeseriesGrouped(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"POST /search": {body: `{"groups_series":[{"name":"/health","count":3,"timeseries":[
			{"@timestamp":1000,"count":3,"quantiles":{"0.5":1.5},"value":12.5}
		]}]}`},
	})

	var result map[string]struct {
		Count      int `json:"count"`
		Timeseries []struct {
			Timestamp int64   `json:"timestamp"`
			Value     float64 `json:"value"`
		} `json:"timeseries"`
	}
	callTool(t, server, "timeseries", map[string]any{
		"log_ids":          []string{"a"},
		"metric_functions": []string{"AVG(duration)"},
		"group_by_keys":    []string{"path", "method"},
	}, &result)

	body := fake.request(t, http.MethodPost, "/search").Body
	if groups := body["groups"].([]any); len(groups) != 1 || groups[0] != "path,method" {
		t.Errorf("groups = %v, want one comma-joined entry", body["groups"])
	}
	if body["num_of_slices"] != float64(timeseriesSlices) {
		t.Errorf("num_of_slices = %v", body["num_of_slices"])
	}

	series, ok := result["/health"]
	if !ok || series.Count != 3 || len(series.Timeseries) != 1 || series.Timeseries[0].Value != 12.5 {
		t.Errorf("result = %+v", result)
	}
}

func TestTimeseriesUngroupedReportsTotals(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"POST /search": {body: `{"totals":{"count":7,"timeseries":[
			{"@timestamp":1000,"count":7,"quantiles":{},"value":7}
		]}}`},
	})

	var result map[string]struct {
		Count int `json:"count"`
	}
	callTool(t, server, "timeseries", map[string]any{
		"log_ids":          []string{"a"},
		"metric_functions": []string{"COUNT(*)"},
	}, &result)

	if result[""].Count != 7 {
		t.Errorf("result = %+v, want totals under the empty group name", result)
	}
}
