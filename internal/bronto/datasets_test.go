package bronto

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

const datasetsBody = `{"logs":[
	{"log":"api","logset":"prod","log_id":"11111111-1111-1111-1111-111111111111","tags":{"environment":"production"}},
	{"log":"api","logset":"staging","log_id":"22222222-2222-2222-2222-222222222222","tags":{}}
]}`

const topKeysBody = `{
	"11111111-1111-1111-1111-111111111111":{
		"status":{"values":{"200":10,"500":2}},
		"path":{"values":{"/health":5}}
	},
	"22222222-2222-2222-2222-222222222222":{"level":{"values":{"info":1}}}
}`

func TestGetDatasets(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /logs": {body: datasetsBody}})

	var datasets []dataset
	callTool(t, server, "get_datasets", map[string]any{}, &datasets)

	if len(datasets) != 2 {
		t.Fatalf("datasets = %+v", datasets)
	}
	first := datasets[0]
	if first.Name != "api" || first.Collection != "prod" || first.Tags["environment"] != "production" {
		t.Errorf("first dataset = %+v", first)
	}
}

func TestGetDatasetsByName(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /logs": {body: datasetsBody}})

	var datasets []dataset
	callTool(t, server, "get_datasets_by_name",
		map[string]any{"dataset_name": "api", "collection_name": "staging"}, &datasets)

	if len(datasets) != 1 || datasets[0].LogID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("datasets = %+v", datasets)
	}

	callTool(t, server, "get_datasets_by_name",
		map[string]any{"dataset_name": "api", "collection_name": "nope"}, &datasets)
	if len(datasets) != 0 {
		t.Errorf("unmatched collection returned %+v", datasets)
	}
}

func TestGetKeys(t *testing.T) {
	server, fake := newTestServer(t, map[string]route{"GET /top-keys": {body: topKeysBody}})

	var keys []string
	callTool(t, server, "get_keys",
		map[string]any{"log_id": "11111111-1111-1111-1111-111111111111"}, &keys)

	if got := fake.request(t, http.MethodGet, "/top-keys").Query.Get("log_id"); got == "" {
		t.Error("log_id was not sent")
	}
	if !slices.Equal(keys, []string{"path", "status"}) {
		t.Errorf("keys = %v", keys)
	}
}

func TestGetKeysRejectsAMalformedLogID(t *testing.T) {
	server, fake := newTestServer(t, nil)

	message := callToolError(t, server, "get_keys", map[string]any{"log_id": "short"})
	if message == "" || fake.count(http.MethodGet, "/top-keys") != 0 {
		t.Errorf("a log_id that is not 36 characters reached Bronto: %q", message)
	}
}

func TestGetAllDatasetsKeys(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /top-keys": {body: topKeysBody}})

	var keys map[string][]string
	callTool(t, server, "get_all_datasets_keys", map[string]any{}, &keys)

	if got := keys["22222222-2222-2222-2222-222222222222"]; !slices.Equal(got, []string{"level"}) {
		t.Errorf("keys = %v", keys)
	}
}

func TestGetKeyValues(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /top-keys": {body: topKeysBody}})

	var values []string
	callTool(t, server, "get_key_values",
		map[string]any{"key": "status", "log_id": "11111111-1111-1111-1111-111111111111"}, &values)
	if !slices.Equal(values, []string{"200", "500"}) {
		t.Errorf("values = %v", values)
	}

	callTool(t, server, "get_key_values", map[string]any{"key": "missing", "log_id": "x"}, &values)
	if values == nil || len(values) != 0 {
		t.Errorf("an unknown key returned %v, want []", values)
	}
}

func TestDatasetToolsReportAuthFailures(t *testing.T) {
	server, _ := newTestServer(t, map[string]route{"GET /logs": {status: http.StatusUnauthorized}})

	message := callToolError(t, server, "get_datasets", map[string]any{})
	if !strings.Contains(message, "Bronto API key") {
		t.Errorf("message = %q", message)
	}
}
