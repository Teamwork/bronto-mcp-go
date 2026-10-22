package bronto

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestGetErrorSummary(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"GET /analytics/errors": {body: `{"groups_series":[
			{"name":"Error","count":5,"groups_series":[{"name":"a","count":5}]},
			{"name":"Warn","value":3,"groups_series":[{"name":"b","value":2},{"name":"a","count":1}]},
			{"name":"Other","count":99}
		]}`},
		"GET /logs": {body: `{"logs":[{"log_id":"a","log":"api","logset":"prod"}]}`},
	})

	var result errorSummary
	callTool(t, server, "get_error_summary", map[string]any{}, &result)

	if got := fake.request(t, http.MethodGet, "/analytics/errors").Query.Get("time_range"); got != "Last 24 hours" {
		t.Errorf("time_range = %q", got)
	}
	if result.Errors != 5 || result.Warnings != 3 || len(result.ByDataset) != 2 {
		t.Fatalf("result = %+v", result)
	}
	worst := result.ByDataset[0]
	if worst.LogID != "a" || worst.Errors != 5 || worst.Warnings != 1 || worst.Name != "api" {
		t.Errorf("worst dataset = %+v", worst)
	}
}

func TestGetErrorSummaryOmitsAnEmptyBreakdown(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"GET /analytics/errors": {body: `{"groups_series":[]}`},
		"GET /logs":             {body: `{"logs":[]}`},
	})

	var result map[string]any
	callTool(t, server, "get_error_summary", map[string]any{}, &result)
	if _, ok := result["by_dataset"]; ok {
		t.Errorf("result = %v, want no by_dataset", result)
	}
}

const usageRoutes = `{"groups_series":[{"name":"a","value":300,"count":3},{"name":"b","value":100,"count":0}]}`

func TestGetUsage(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"GET /usage/organizations/logs": {body: usageRoutes},
		"GET /limits": {body: `{"limits":[
			{"target":"ORGANISATION","type":"SYSTEM","category":"INGESTION_LIMITS","unit":"BYTES","value":1000},
			{"target":"ORGANISATION","type":"SYSTEM","category":"SEARCH_LIMITS","unit":"RATIO","value":2.5},
			{"target":"LOG","type":"SYSTEM","category":"INGESTION_LIMITS","unit":"BYTES","value":1}
		]}`},
	})

	var result struct {
		Summary  usageSummary   `json:"summary"`
		Datasets []datasetUsage `json:"datasets"`
	}
	callTool(t, server, "get_usage", map[string]any{}, &result)

	summary := result.Summary
	if summary.DatasetsTotal != 2 || summary.IngestBytes != 400 || summary.SearchBytes != 400 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.IngestLimit == nil || *summary.IngestLimit != 1000 ||
		summary.SearchLimit == nil || *summary.SearchLimit != 2500 {
		t.Errorf("limits = %v, %v", summary.IngestLimit, summary.SearchLimit)
	}
	if result.Datasets[0].LogID != "a" {
		t.Errorf("datasets = %+v, want the largest first", result.Datasets)
	}
}

func TestGetCoverage(t *testing.T) {
	at := time.UnixMilli(2_000_000_000_000)
	fixClock(t, at)
	recent := at.Add(-time.Hour).UnixMilli()

	server, _ := newTestServer(t, map[string]route{
		"GET /logs": {body: `{"logs":[
			{"log_id":"a","log":"api","logset":"prod"},
			{"log_id":"c","log":"audit","logset":".audit-trail","metadata":{"last_heartbeat_at":` +
			strconv.FormatInt(recent, 10) + `}},
			{"log_id":"d","log":"old","logset":"prod","metadata":{"last_heartbeat_at":1}},
			{"log":"no id"}
		]}`},
		"GET /monitors":                 {body: monitorsBody},
		"GET /metrics/definitions":      {body: definitionsBody},
		"GET /usage/organizations/logs": {body: usageRoutes},
	})

	var result struct {
		Summary  coverageSummary   `json:"summary"`
		Datasets []datasetCoverage `json:"datasets"`
	}
	callTool(t, server, "get_coverage", map[string]any{}, &result)

	if result.Summary != (coverageSummary{DatasetsTotal: 3, Monitored: 2, Unmonitored: 1}) {
		t.Errorf("summary = %+v", result.Summary)
	}
	byID := map[string]datasetCoverage{}
	for _, d := range result.Datasets {
		byID[d.LogID] = d
	}
	if got := byID["a"]; len(got.Monitors) != 1 || got.Monitors[0] != "m-1" || !got.Ingestion.Active {
		t.Errorf("dataset a = %+v", got)
	}
	// c reports no usage: it is a system collection, active from its heartbeat.
	if got := byID["c"]; !got.Ingestion.Active || len(got.Monitors) != 1 || got.Monitors[0] != "m-2" {
		t.Errorf("dataset c = %+v", got)
	}
	if got := byID["d"]; got.Ingestion.Active || got.Monitors == nil || len(got.Monitors) != 0 {
		t.Errorf("dataset d = %+v, want inactive from a stale heartbeat, with an empty monitor list", got)
	}
}
