package bronto

import (
	"context"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
	"github.com/teamwork/mcp/pkg/helpers"
	"github.com/teamwork/mcp/pkg/toolsets"
)

// maxSavedSearchLimit caps how many saved searches one listing returns.
const maxSavedSearchLimit = 100

// GetSavedSearches lists or fetches saved searches.
func GetSavedSearches(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodGetSavedSearches),
			Annotations: readAnnotations("Get Saved Searches"),
			Description: "Lists saved searches, or fetches one by id. Optionally filter the list by a " +
				"case-insensitive text query. Use this to discover reusable saved queries.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"query": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional case-insensitive text filter.",
					}),
					"saved_search_id": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Fetch one saved search by id.",
					}),
					"limit": optional(&jsonschema.Schema{
						Type:        "integer",
						Description: "Optional maximum number of saved searches to return after filtering.",
						Minimum:     new(1.0),
						Maximum:     new(float64(maxSavedSearchLimit)),
					}),
				},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			if id := arguments.GetString("saved_search_id", ""); id != "" {
				saved, err := client.SavedSearch(ctx, id)
				if err != nil {
					return errorResult(err, "")
				}
				return helpers.NewToolResultJSON(saved)
			}

			body, err := client.SavedSearches(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			query := strings.ToLower(arguments.GetString("query", ""))
			items := []any{}
			for _, item := range jsonvalue.List(body, "saved_searches") {
				if query == "" || strings.Contains(strings.ToLower(jsonvalue.Text(item)), query) {
					items = append(items, item)
				}
			}
			total := len(items)
			if limit := arguments.GetInt("limit", 0); limit > 0 && limit < total {
				items = items[:limit]
			}
			return helpers.NewToolResultJSON(map[string]any{
				"saved_searches": items,
				"total":          total,
				"shown":          len(items),
				"truncated":      len(items) < total,
			})
		},
	}
}

// savedSearchBody is what POST /saved-searches takes.
type savedSearchBody struct {
	Name   string   `json:"name"`
	LogIDs []string `json:"log_ids"`
	// SearchDetails must be a string-to-string map: the API rejects a list, so
	// the dataset ids go in it comma-separated as well as top-level.
	SearchDetails map[string]string `json:"search_details"`
}

// CreateSavedSearch creates a saved search.
func CreateSavedSearch(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodCreateSavedSearch),
			Annotations: writeAnnotations("Create Saved Search"),
			Description: "Saves a reusable search. Provide a name, the dataset log IDs, and optionally a WHERE " +
				"filter, a select, and a relative time range. Returns the created saved search, including its " +
				"id. With dry_run, returns {dry_run: true, saved_search_body} and creates nothing.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {
						Type:        "string",
						Description: "Name of the saved search.",
					},
					"log_ids": logIDsSchema("Dataset IDs the search runs over."),
					"search_filter": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional SQL-style WHERE filter.",
					}),
					"select": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Fields or aggregate to return. Defaults to \"" + strings.Join(defaultSelect, ",") + "\".",
					}),
					"time_range": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional relative window, e.g. \"Last 1 hour\".",
					}),
					"dry_run": optional(&jsonschema.Schema{
						Type:        "boolean",
						Description: "Validate and return the resolved saved-search body without creating anything.",
					}),
				},
				Required: []string{"name", "log_ids"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var name string
			var logIDs []string
			arguments, err := bindArguments(request,
				helpers.RequiredParam(&name, "name"),
				requiredStrings(&logIDs, "log_ids"),
			)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			details := map[string]string{
				"from":   strings.Join(logIDs, ","),
				"select": stringOr(arguments, "select", strings.Join(defaultSelect, ",")),
			}
			if where := arguments.GetString("search_filter", ""); where != "" {
				details["where"] = where
			}
			if timeRange := arguments.GetString("time_range", ""); timeRange != "" {
				details["time_range"] = timeRange
			}
			body := savedSearchBody{Name: name, LogIDs: logIDs, SearchDetails: details}
			if arguments.GetBool("dry_run", false) {
				return helpers.NewToolResultJSON(map[string]any{"dry_run": true, "saved_search_body": body})
			}

			created, err := client.CreateSavedSearch(ctx, body)
			if err != nil {
				return errorResult(err, "")
			}
			return helpers.NewToolResultJSON(map[string]any{"saved_search": created})
		},
	}
}

// RunSavedSearch runs a saved search.
func RunSavedSearch(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodRunSavedSearch),
			Annotations: readAnnotations("Run Saved Search"),
			Description: "Executes a saved search by id. Returns its events (each with @raw, @time, @status and " +
				"message_kvs), or for an aggregate search the grouped results under totals/groups_series. Pass " +
				"time_range to run over a different window than it was saved with. Use " +
				string(MethodGetSavedSearches) + " to find the id.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"saved_search_id": {
						Type:        "string",
						Description: "Id of the saved search to run.",
					},
					"time_range": optional(&jsonschema.Schema{
						Type:        "string",
						Description: "Optional window override, e.g. \"Last 1 hour\".",
					}),
					"limit": optional(&jsonschema.Schema{
						Type:        "integer",
						Description: "Optional max rows/events to return.",
						Minimum:     new(1.0),
					}),
				},
				Required: []string{"saved_search_id"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var id string
			arguments, err := bindArguments(request, helpers.RequiredParam(&id, "saved_search_id"))
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			fetched, err := client.SavedSearch(ctx, id)
			if err != nil {
				return errorResult(err, "")
			}
			saved := jsonvalue.Map(fetched)
			if wrapped, ok := saved["saved_search"]; ok {
				saved = jsonvalue.Map(wrapped)
			}
			details := jsonvalue.Map(saved["search_details"])
			if len(details) == 0 {
				return helpers.NewToolResultTextError("saved search '%s' has no search_details to run", id), nil
			}

			body := savedSearchQuery(saved, details)
			if timeRange := arguments.GetString("time_range", ""); timeRange != "" {
				body["time_range"] = timeRange
			}
			if limit := arguments.GetInt("limit", 0); limit > 0 {
				body["limit"] = limit
			}

			resp, err := client.Search(ctx, body)
			if err != nil {
				return errorResult(err, searchBadRequestHint)
			}
			return helpers.NewToolResultJSON(shapeSearchResponse(resp))
		},
	}
}

// savedSearchQuery rebuilds the /search body a saved search describes. Saved
// searches made in the UI and through the API spell their fields differently,
// so each is read under every name it is known by.
func savedSearchQuery(saved, details map[string]any) map[string]any {
	body := map[string]any{}

	// UI-created saved searches keep their datasets only in the top-level log_ids.
	from := asList(jsonvalue.FirstTruthy(details["from"], details["logIds"], details["log_ids"], saved["log_ids"]))
	if len(from) > 0 {
		body["from"] = from
	}
	fromExpr := jsonvalue.String(jsonvalue.FirstTruthy(details["from_expr"], details["fromExpr"]))
	if fromExpr = strings.TrimSpace(fromExpr); fromExpr != "" {
		body["from_expr"] = fromExpr
	}
	if where := strings.TrimSpace(jsonvalue.String(details["where"])); where != "" {
		body["where"] = where
	}
	selected := asList(details["select"])
	if len(selected) == 0 {
		selected = defaultSelect
	}
	body["select"] = selected
	if groups := asList(details["groups"]); len(groups) > 0 {
		body["groups"] = groups
	}
	timeRange := jsonvalue.String(jsonvalue.FirstTruthy(details["time_range"], details["timeRange"]))
	if timeRange = strings.TrimSpace(timeRange); timeRange != "" {
		body["time_range"] = timeRange
	}
	return body
}

// eventFields are the fields of each event a search result keeps.
var eventFields = []string{"@raw", "@time", "@status", "message_kvs"}

// shapeSearchResponse trims a raw /search response.
//
// The raw payload repeats every event under both result and events, with
// per-event metadata and context links; only each event's raw line, time,
// status and parsed key-values are kept. Grouped and aggregate responses keep
// their groups_series and totals instead.
func shapeSearchResponse(resp map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"groups_series", "totals"} {
		if jsonvalue.Truthy(resp[key]) {
			out[key] = resp[key]
		}
	}
	if len(out) == 0 {
		events := []any{}
		for _, item := range jsonvalue.Slice(resp["events"]) {
			event, ok := item.(map[string]any)
			if !ok {
				continue
			}
			kept := map[string]any{}
			for _, field := range eventFields {
				if value, ok := event[field]; ok {
					kept[field] = value
				}
			}
			events = append(events, kept)
		}
		out["events"] = events
	}
	for _, key := range []string{"explain", "links"} {
		if jsonvalue.Truthy(resp[key]) {
			out[key] = resp[key]
		}
	}
	return out
}
