package bronto

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
	"github.com/teamwork/mcp/pkg/helpers"
	"github.com/teamwork/mcp/pkg/toolsets"
)

// defaultSearchWindow is how far back a search reaches when the caller gives
// no start.
const defaultSearchWindow = 20 * time.Minute

// timeseriesSlices is how many data points an aggregate query splits its time
// range into.
const timeseriesSlices = 10

// timeRange reads a search's bounds in Unix milliseconds. Unset, they cover the
// last defaultSearchWindow, counted from the start of the current second.
func timeRange(arguments helpers.ToolArguments) (start, end int64) {
	current := now().Truncate(time.Second)
	start = int64(arguments.GetInt("timerange_start", int(current.Add(-defaultSearchWindow).UnixMilli())))
	end = int64(arguments.GetInt("timerange_end", int(current.UnixMilli())))
	return start, end
}

func searchTimeSchemas(properties map[string]*jsonschema.Schema) map[string]*jsonschema.Schema {
	properties["timerange_start"] = optional(timestampSchema(
		"Unix timestamp in milliseconds representing the start of a time range, e.g. 1756063146000. " +
			fmt.Sprintf("If not specified, defaults to %d minutes ago.", int(defaultSearchWindow.Minutes()))))
	properties["timerange_end"] = optional(timestampSchema(
		"Unix timestamp in milliseconds representing the end of a time range, e.g. 1756063254000. " +
			"If not specified, defaults to the current time."))
	properties["log_ids"] = logIDsSchema(
		"List of dataset IDs, identifying sets of log data. Each log ID is a UUID.")
	properties["search_filter"] = optional(filterSchema())
	return properties
}

// SearchLogs searches log events.
func SearchLogs(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodSearchLogs),
			Annotations: readAnnotations("Execute Event Query"),
			Description: "Searches log data. This tool returns a list of log events and their attributes. " +
				"Bare words in search_filter perform full-text matching across the event. Field predicates " +
				"such as \"field\"='value' only match that exact field; call " + string(MethodGetKeys) +
				" before using a field name unless the field came from the user or a saved search. For trace " +
				"ids, request ids, or other opaque-identifier investigations, start with a bare full-text word " +
				"search unless the user explicitly asks for a field-specific match. If an exact-field query " +
				"returns zero, recheck the field spelling with " + string(MethodGetKeys) +
				" before reporting zero.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: searchTimeSchemas(map[string]*jsonschema.Schema{}),
				Required:   []string{"log_ids"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var logIDs []string
			arguments, err := bindArguments(request, requiredStrings(&logIDs, "log_ids"))
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			start, end := timeRange(arguments)

			events, err := client.SearchEvents(ctx, brontosdk.EventSearchRequest{
				From:   start,
				To:     end,
				LogIDs: logIDs,
				Where:  arguments.GetString("search_filter", ""),
				Select: defaultSelect,
			})
			if err != nil {
				return errorResult(err, searchBadRequestHint)
			}
			return helpers.NewToolResultJSON(events)
		},
	}
}

// datapoint is one slice of an aggregate query's time range.
type datapoint struct {
	Timestamp any `json:"timestamp"`
	Count     any `json:"count"`
	Quantiles any `json:"quantiles"`
	Value     any `json:"value"`
}

// timeseries is one group's aggregate over the whole time range and per slice.
type timeseries struct {
	Count      any         `json:"count"`
	Timeseries []datapoint `json:"timeseries"`
}

// Timeseries computes aggregate metrics over time.
func Timeseries(client *brontosdk.Client) toolsets.ToolWrapper {
	properties := searchTimeSchemas(map[string]*jsonschema.Schema{
		"metric_functions": {
			Type: "array",
			Description: "The metric functions to compute. Each is one of AVG, MIN, MAX, COUNT, MEAN, MEDIAN " +
				"and SUM. Each takes a key name as attribute, except for COUNT which only takes the character " +
				"'*' (i.e. \"COUNT(*)\"). Key names for a dataset can be retrieved with " +
				string(MethodGetKeys) + ".",
			Items:    &jsonschema.Schema{Type: "string"},
			MinItems: new(1),
		},
		"group_by_keys": optional(&jsonschema.Schema{
			Type: "array",
			Description: "Keys expected to be present in the datasets, by which the computed metrics " +
				"should be grouped.",
			Items: &jsonschema.Schema{Type: "string"},
		}),
	})

	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name:        string(MethodTimeseries),
			Annotations: readAnnotations("Execute Aggregate or Time-Series Query"),
			Description: "Computes metric data from log data. This tool returns a list of data points for each " +
				"group of the group_by_keys list. Each list represents the value of the computed metrics for a " +
				"subset of the provided time range.\n\n" +
				"For instance, for web access or CDN logs, the question could be \"Can you please provide the " +
				"average response time per path for the last hour?\". The answer would then return the " +
				"AVG(response_time) metric, grouped by URL path, and split into a list of data points over the " +
				"time range.\n\n" +
				"Each timeseries has a 'count' field (matching event count, not a byte value) and each data " +
				"point a 'value' field (the numeric aggregate result, e.g. SUM of bytes). Use 'value' as the " +
				"answer for byte/numeric metrics; do not report 'count' as a byte total.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: properties,
				Required:   []string{"log_ids", "metric_functions"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var logIDs, metricFunctions []string
			arguments, err := bindArguments(request,
				requiredStrings(&logIDs, "log_ids"),
				requiredStrings(&metricFunctions, "metric_functions"),
			)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			groupByKeys := arguments.GetStringSlice("group_by_keys", nil)
			start, end := timeRange(arguments)

			resp, err := client.Search(ctx, map[string]any{
				"from_ts": start,
				"to_ts":   end,
				"where":   arguments.GetString("search_filter", ""),
				"select":  metricFunctions,
				"from":    logIDs,
				// Bronto takes the grouping keys as one comma-separated entry.
				"groups":        []string{strings.Join(groupByKeys, ",")},
				"num_of_slices": timeseriesSlices,
			})
			if err != nil {
				return errorResult(err, searchBadRequestHint)
			}
			return helpers.NewToolResultJSON(shapeTimeseries(resp, len(groupByKeys) > 0))
		},
	}
}

// shapeTimeseries maps each group's name to its series. An ungrouped query
// answers with its totals alone, reported as one group named "".
func shapeTimeseries(resp map[string]any, grouped bool) map[string]timeseries {
	var groups []any
	if grouped {
		groups = jsonvalue.Slice(resp["groups_series"])
	} else {
		totals := jsonvalue.Map(resp["totals"])
		groups = []any{map[string]any{
			"name":       "",
			"timeseries": totals["timeseries"],
			"count":      totals["count"],
		}}
	}

	result := make(map[string]timeseries, len(groups))
	for _, item := range groups {
		group := jsonvalue.Map(item)
		if group == nil {
			continue
		}
		points := []datapoint{}
		for _, raw := range jsonvalue.Slice(group["timeseries"]) {
			point := jsonvalue.Map(raw)
			points = append(points, datapoint{
				Timestamp: point["@timestamp"],
				Count:     point["count"],
				Quantiles: point["quantiles"],
				Value:     point["value"],
			})
		}
		result[jsonvalue.String(group["name"])] = timeseries{Count: group["count"], Timeseries: points}
	}
	return result
}
