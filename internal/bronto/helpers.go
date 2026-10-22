package bronto

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
	"github.com/teamwork/mcp/pkg/helpers"
)

// now is the clock the search tools default their time range from. Tests
// replace it.
var now = time.Now

// Defaults the tools fall back to when the caller leaves a field unset.
const (
	// defaultUsageWindow is the relative window the usage and coverage
	// reports cover. Bronto's limits are per calendar month.
	defaultUsageWindow = "This month"
	// defaultErrorWindow is the relative window the error summary covers.
	defaultErrorWindow = "Last 24 hours"
	// countAll is the aggregate a monitor evaluates when given none.
	countAll = "count(*)"
)

// defaultSelect is what an event search returns when it names no fields:
// every parsed field plus the raw line. It is shared, so never modify it.
var defaultSelect = []string{"*", "@raw"}

// searchBadRequestHint explains a 400 from a search: almost always a filter
// that does not parse, or a key the dataset does not have.
const searchBadRequestHint = "One of the search parameters is unsuitable. Check the filter syntax as well as " +
	"the names of the keys used in the filter, the selected fields and the grouping keys."

// errorResult turns a failed Bronto call into a tool result the model can read
// and act on. badRequestHint replaces the generic explanation of a 400; empty
// keeps it.
func errorResult(err error, badRequestHint string) (*mcp.CallToolResult, error) {
	var httpErr *brontosdk.HTTPError
	switch {
	case errors.As(err, &httpErr):
		var message string
		switch httpErr.StatusCode {
		case http.StatusBadRequest:
			message = "The request was rejected as malformed. Check the parameters."
			if badRequestHint != "" {
				message = badRequestHint
			}
		case http.StatusUnauthorized:
			message = "Not authorised. Please check your Bronto API key and endpoint."
		case http.StatusForbidden:
			message = "Not allowed. Please check your Bronto API key permissions."
		default:
			message = fmt.Sprintf("Bronto request failed with status %d.", httpErr.StatusCode)
		}
		if httpErr.Details != "" {
			message += " " + httpErr.Details
		}
		return helpers.NewToolResultTextError("%s", message), nil

	case errors.Is(err, brontosdk.ErrUnexpectedResponse):
		return helpers.NewToolResultTextError("Unexpected Bronto response format: %v", err), nil

	default:
		return helpers.NewToolResultTextError(
			"Cannot interact with Bronto (%v). Please check endpoint configuration.", err,
		), nil
	}
}

// isBrontoAnswer reports whether err came from Bronto itself — an error status
// or a body that would not decode — rather than from failing to reach it.
func isBrontoAnswer(err error) bool {
	var httpErr *brontosdk.HTTPError
	return errors.As(err, &httpErr) || errors.Is(err, brontosdk.ErrUnexpectedResponse)
}

// bindArguments reads a call's arguments and binds them with params, stopping
// at the first that is missing or of the wrong type.
func bindArguments(request *mcp.CallToolRequest, params ...helpers.ParamFunc) (helpers.ToolArguments, error) {
	arguments, err := helpers.NewToolArguments(request)
	if err != nil {
		return nil, err
	}
	if err := helpers.ParamGroup(arguments, params...); err != nil {
		return nil, err
	}
	return arguments, nil
}

// requiredStrings binds a required list of strings. helpers.RequiredParam
// cannot, as JSON decodes an array as []any.
func requiredStrings(target *[]string, key string) helpers.ParamFunc {
	return func(params map[string]any) error {
		values, err := helpers.ToolArguments(params).RequireStringSlice(key)
		if err != nil {
			return err
		}
		*target = values
		return nil
	}
}

// stringOr reads an optional string argument, falling back when it is unset,
// null or empty.
func stringOr(arguments helpers.ToolArguments, key, fallback string) string {
	if value := arguments.GetString(key, ""); value != "" {
		return value
	}
	return fallback
}

// readAnnotations describes a tool that only reads from Bronto.
func readAnnotations(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		OpenWorldHint:   new(false),
	}
}

// writeAnnotations describes a tool that creates something in Bronto without
// changing or removing anything that exists.
func writeAnnotations(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		DestructiveHint: new(false),
		OpenWorldHint:   new(false),
	}
}

// noArgumentsSchema is the input schema of a tool that takes no arguments.
func noArgumentsSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{}}
}

// optional lets a property also be null, which is how clients commonly send a
// parameter they mean to leave unset. The description moves to the wrapper so
// clients show it.
func optional(schema *jsonschema.Schema) *jsonschema.Schema {
	description := schema.Description
	schema.Description = ""
	return &jsonschema.Schema{
		Description: description,
		AnyOf:       []*jsonschema.Schema{schema, {Type: "null"}},
	}
}

// logIDsSchema is a required, non-empty list of dataset ids.
func logIDsSchema(description string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "array",
		Description: description,
		Items:       &jsonschema.Schema{Type: "string"},
		MinItems:    new(1),
	}
}

// filterSchema documents the SQL-style WHERE filter the search tools take.
func filterSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Description: "If no value is specified for this field, then no filter is applied. Otherwise, this " +
			"field must follow the syntax of an SQL WHERE clause. Unless the search filter is explicitly " +
			"provided by the user, it is CRITICAL to use keys present in the dataset, e.g. " +
			"\"key_name\"='key_value'. The keys present in a dataset can be retrieved with " +
			string(MethodGetKeys) + ". In any case, following SQL syntax, key names should be double-quoted, " +
			"string values single-quoted, and numeric values unquoted.",
	}
}

// timestampSchema describes one end of a search's time range.
func timestampSchema(description string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer", Description: description}
}

// asList reads a saved search's list field, which may be stored as an array or
// as a comma-separated string. Blank entries are dropped.
func asList(v any) []string {
	var parts []string
	switch v := v.(type) {
	case []any:
		for _, item := range v {
			parts = append(parts, jsonvalue.String(item))
		}
	case string:
		parts = strings.Split(v, ",")
	}

	var result []string
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

// fromLogIDs reads a query's "from", which is either a list of dataset ids or
// a single one.
func fromLogIDs(from any) []string {
	if list, ok := from.([]any); ok {
		ids := make([]string, 0, len(list))
		for _, id := range list {
			ids = append(ids, jsonvalue.String(id))
		}
		return ids
	}
	if jsonvalue.Truthy(from) {
		return []string{jsonvalue.String(from)}
	}
	return nil
}

// indexBy maps each object in items to its field, skipping those without one.
func indexBy(items []any, field string) map[string]map[string]any {
	index := make(map[string]map[string]any, len(items))
	for _, item := range items {
		object := jsonvalue.Map(item)
		if id := object[field]; jsonvalue.Truthy(id) {
			index[jsonvalue.String(id)] = object
		}
	}
	return index
}

// monitorID reads a monitor's id, which older payloads call monitor_id.
func monitorID(monitor map[string]any) string {
	return jsonvalue.String(jsonvalue.FirstTruthy(monitor["id"], monitor["monitor_id"]))
}
