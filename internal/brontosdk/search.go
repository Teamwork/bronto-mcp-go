package brontosdk

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
)

// EventSearchRequest selects log events over a time range.
type EventSearchRequest struct {
	// From and To bound the search, in Unix milliseconds.
	From, To int64
	// LogIDs are the datasets to search.
	LogIDs []string
	// Where is an SQL-style WHERE filter. Empty matches every event.
	Where string
	// Select lists the fields to return.
	Select []string
	// Limit caps how many events come back. Zero leaves it to Bronto.
	Limit int
}

// LogEvent is one event a search matched.
type LogEvent struct {
	// Message is the raw log line.
	Message string `json:"message"`
	// Attributes holds the event's @status and @time along with every
	// attribute and message key-value pair Bronto parsed from it. Keys whose
	// value is the empty string are left out: selecting "*" returns every key
	// in the dataset on every event, and most are empty on any one of them.
	Attributes map[string]any `json:"attributes"`
}

// SearchEvents runs an event search through GET /search.
func (c *Client) SearchEvents(ctx context.Context, req EventSearchRequest) ([]LogEvent, error) {
	query := url.Values{
		"from":    req.LogIDs,
		"from_ts": {strconv.FormatInt(req.From, 10)},
		"to_ts":   {strconv.FormatInt(req.To, 10)},
		"where":   {req.Where},
		"select":  req.Select,
	}
	if req.Limit > 0 {
		query.Set("limit", strconv.Itoa(req.Limit))
	}
	body, err := c.do(ctx, http.MethodGet, "search", query, nil)
	if err != nil {
		return nil, err
	}
	result, err := object(body)
	if err != nil {
		return nil, err
	}

	events := []LogEvent{}
	for _, item := range jsonvalue.Slice(result["events"]) {
		event := jsonvalue.Map(item)
		if event == nil {
			continue
		}
		attributes := map[string]any{
			"@status": event["@status"],
			"@time":   event["@time"],
		}
		copyNonEmpty(attributes, jsonvalue.Map(event["attributes"]))
		copyNonEmpty(attributes, jsonvalue.Map(event["message_kvs"]))
		events = append(events, LogEvent{
			Message:    jsonvalue.String(event["@raw"]),
			Attributes: attributes,
		})
		// Bronto honours the limit itself; this only guards against a
		// response that does not.
		if req.Limit > 0 && len(events) == req.Limit {
			break
		}
	}
	return events, nil
}

// copyNonEmpty copies src into dst, skipping keys whose value is "".
func copyNonEmpty(dst, src map[string]any) {
	for key, value := range src {
		if value != "" {
			dst[key] = value
		}
	}
}

// Search POSTs a raw /search body and returns the decoded response. It serves
// aggregate queries, saved searches re-run verbatim and monitor health checks.
func (c *Client) Search(ctx context.Context, body any) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodPost, "search", nil, body)
	if err != nil {
		return nil, err
	}
	return object(resp)
}
