package brontosdk

import (
	"net/http"
	"net/url"
	"testing"
)

func TestUsageByLog(t *testing.T) {
	var query url.Values
	client := serve(t, http.StatusOK, `{"groups_series":[
		{"name":"a","value":12.9,"count":3},
		{"name":123,"value":null},
		{"value":5}
	]}`, func(r *http.Request) { query = r.URL.Query() })

	rows, err := client.UsageByLog(t.Context(), UsageSearch, "This month")
	if err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{
		"usage_type": "search", "delta": "true", "limit": "500", "time_range": "This month",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	want := []UsageRow{{LogID: "a", BytesTotal: 12, Count: 3}, {LogID: "123"}}
	if len(rows) != len(want) || rows[0] != want[0] || rows[1] != want[1] {
		t.Errorf("rows = %+v, want %+v (a group without a name is skipped)", rows, want)
	}
}
