package bronto

import (
	"net/http"
	"strings"
	"testing"
)

const usersBody = `{"users":[{"id":1,"email":"Ops@Example.com"}]}`

func createMonitorArgs() map[string]any {
	return map[string]any{
		"name":                "5xx spike",
		"notification_email":  " ops@example.com ",
		"threshold":           10,
		"comparison_operator": "ABOVE",
		"window":              "Last 15 minutes",
		"log_ids":             []string{"a"},
		"search_filter":       `"status">=500`,
	}
}

func TestCreateMonitorIsAlwaysMuted(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"GET /users":     {body: usersBody},
		"POST /monitors": {status: http.StatusCreated, body: `{"id":"m-1"}`},
	})

	var created map[string]any
	callTool(t, server, "create_monitor", createMonitorArgs(), &created)
	if created["id"] != "m-1" {
		t.Errorf("created = %v", created)
	}

	body := fake.request(t, http.MethodPost, "/monitors").Body
	if body["mute_until"] != float64(-1) {
		t.Errorf("mute_until = %v, want -1", body["mute_until"])
	}
	query := body["queries"].([]any)[0].(map[string]any)
	if query["select"].([]any)[0] != "count(*)" || query["where"] != `"status">=500` {
		t.Errorf("query = %v", query)
	}
}

func TestCreateMonitorDryRunCreatesNothing(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{"GET /users": {body: usersBody}})

	args := createMonitorArgs()
	args["dry_run"] = true
	var result map[string]any
	callTool(t, server, "create_monitor", args, &result)

	if result["dry_run"] != true || result["monitor_body"] == nil {
		t.Errorf("result = %v", result)
	}
	if fake.count(http.MethodPost, "/monitors") != 0 {
		t.Error("dry run created a monitor")
	}
}

func TestCreateMonitorRejectsAnUnknownEmail(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{"GET /users": {body: usersBody}})

	args := createMonitorArgs()
	args["notification_email"] = "stranger@example.com"
	message := callToolError(t, server, "create_monitor", args)

	if !strings.Contains(message, "not an existing Bronto user") {
		t.Errorf("message = %q", message)
	}
	if fake.count(http.MethodPost, "/monitors") != 0 {
		t.Error("a monitor was created for an unknown email")
	}
}

const monitorsBody = `{"monitors":[
	{"id":"m-1","name":"5xx spike","status":"ALARM","metric_id":"d-1"},
	{"monitor_id":"m-2","name":"Latency","state":"OK","metric_id":"d-2"},
	{"id":"m-3","name":"Orphan","status":"OK","metric_id":"d-missing"}
]}`

const definitionsBody = `{"definitions":[
	{"id":"d-1","queries":[{"from":["a","b"],"where":"\"status\">=500"}]},
	{"id":"d-2","queries":[{"from":"c"}]}
]}`

func TestSearchMonitorsFilters(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /monitors": {body: monitorsBody}})

	var result struct {
		Monitors []map[string]any `json:"monitors"`
		Total    int              `json:"total"`
	}
	callTool(t, server, "search_monitors", map[string]any{"status": "ok"}, &result)
	if result.Total != 2 {
		t.Errorf("status filter matched %d, want 2: %v", result.Total, result.Monitors)
	}

	callTool(t, server, "search_monitors", map[string]any{"query": "LATENCY"}, &result)
	if result.Total != 1 || result.Monitors[0]["monitor_id"] != "m-2" {
		t.Errorf("query filter = %v", result.Monitors)
	}
}

func TestSearchMonitorsByID(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"GET /monitors":            {body: monitorsBody},
		"GET /monitors/m-2/events": {body: `{"events":[{"type":"TRIGGERED"}]}`},
	})

	var result struct {
		Monitor map[string]any `json:"monitor"`
		Events  []any          `json:"events"`
	}
	callTool(t, server, "search_monitors", map[string]any{"monitor_id": "m-2"}, &result)
	if result.Monitor["name"] != "Latency" || len(result.Events) != 1 {
		t.Errorf("result = %+v", result)
	}

	callTool(t, server, "search_monitors", map[string]any{"monitor_id": "nope"}, &result)
	if result.Monitor != nil || result.Events == nil {
		t.Errorf("unknown monitor = %+v, want a null monitor and no events", result)
	}
}

func TestValidateMonitors(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{
		"GET /monitors":            {body: monitorsBody},
		"GET /metrics/definitions": {body: definitionsBody},
		"POST /search":             {body: `{"totals":{"count":4}}`},
	})

	var result struct {
		Summary healthSummary   `json:"summary"`
		Results []monitorHealth `json:"results"`
	}
	callTool(t, server, "validate_monitors", map[string]any{}, &result)

	if result.Summary != (healthSummary{OK: 2, Error: 1}) {
		t.Errorf("summary = %+v", result.Summary)
	}
	if result.Results[2].Status != healthError || result.Results[2].MonitorID != "m-3" {
		t.Errorf("orphan monitor = %+v", result.Results[2])
	}
	body := fake.request(t, http.MethodPost, "/search").Body
	if body["time_range"] != healthCheckWindow || body["where"] == nil {
		t.Errorf("health check body = %v", body)
	}
}

func TestValidateMonitorsReportsRejectedQueries(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{
		"GET /monitors":            {body: monitorsBody},
		"GET /metrics/definitions": {body: definitionsBody},
		"POST /search":             {status: http.StatusBadRequest, body: `{"details":"bad where"}`},
	})

	var result struct {
		Summary healthSummary   `json:"summary"`
		Results []monitorHealth `json:"results"`
	}
	callTool(t, server, "validate_monitors", map[string]any{}, &result)

	if result.Summary.Error != 3 || !strings.Contains(result.Results[0].Reason, "bad where") {
		t.Errorf("result = %+v", result)
	}
}
