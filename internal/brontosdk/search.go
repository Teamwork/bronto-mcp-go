package brontosdk

import (
	"context"
	"maps"
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
}

// LogEvent is one event a search matched.
type LogEvent struct {
	// Message is the raw log line.
	Message string `json:"message"`
	// Attributes holds the event's @status and @time along with every
	// attribute and message key-value pair Bronto parsed from it.
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
		maps.Copy(attributes, jsonvalue.Map(event["attributes"]))
		maps.Copy(attributes, jsonvalue.Map(event["message_kvs"]))
		events = append(events, LogEvent{
			Message:    jsonvalue.String(event["@raw"]),
			Attributes: attributes,
		})
	}
	return events, nil
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
