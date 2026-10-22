package bronto

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teamwork/bronto-mcp-go/internal/brontosdk"
	"github.com/teamwork/bronto-mcp-go/internal/jsonvalue"
	"github.com/teamwork/mcp/pkg/helpers"
	"github.com/teamwork/mcp/pkg/toolsets"
)

// logIDLength is the length of a dataset id, a UUID.
const logIDLength = 36

// dataset is how the tools describe one dataset.
type dataset struct {
	// Name often names the service whose logs the dataset holds.
	Name string `json:"name"`
	// Collection is the collection the dataset belongs to. The same name may
	// appear in several collections.
	Collection string `json:"collection"`
	// LogID identifies the dataset uniquely.
	LogID string `json:"log_id"`
	// Tags are key-value pairs such as environment=production.
	Tags map[string]any `json:"tags"`
}

func newDataset(raw map[string]any) dataset {
	tags := jsonvalue.Map(raw["tags"])
	if tags == nil {
		tags = map[string]any{}
	}
	return dataset{
		Name:       jsonvalue.String(raw["log"]),
		Collection: jsonvalue.String(raw["logset"]),
		LogID:      jsonvalue.String(raw["log_id"]),
		Tags:       tags,
	}
}

// GetDatasets lists every dataset.
func GetDatasets(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetDatasets),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Retrieve a List of Logs",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Fetches all dataset details. Each dataset carries its name, the collection it belongs " +
				"to, its log ID (a 36-character UUID), and its tags. Tags are string key-value pairs; the " +
				"description tag is particularly useful to understand the type of data a dataset contains, and " +
				"other common tags are service, teams and environment.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		Handler: func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			raw, err := client.Datasets(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			datasets := make([]dataset, 0, len(raw))
			for _, item := range raw {
				datasets = append(datasets, newDataset(jsonvalue.Map(item)))
			}
			return helpers.NewToolResultJSON(datasets)
		},
	}
}

// GetDatasetsByName finds datasets by name and collection.
func GetDatasetsByName(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetDatasetsByName),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Find Datasets by Name and Collection",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Fetches details about a Bronto dataset. A dataset is uniquely identified by its name " +
				"and its collection name: several datasets with the same name can belong to different " +
				"collections, but only one dataset with a given name can belong to a given collection.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"dataset_name": {
						Type:        "string",
						Description: "The dataset name.",
						MinLength:   new(1),
					},
					"collection_name": {
						Type:        "string",
						Description: "The collection that the dataset is part of.",
						MinLength:   new(1),
					},
				},
				Required: []string{"dataset_name", "collection_name"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			name, err := arguments.RequireString("dataset_name")
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			collection, err := arguments.RequireString("collection_name")
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			raw, err := client.Datasets(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			matches := []dataset{}
			for _, item := range raw {
				candidate := newDataset(jsonvalue.Map(item))
				if candidate.Name == name && candidate.Collection == collection {
					matches = append(matches, candidate)
				}
			}
			return helpers.NewToolResultJSON(matches)
		},
	}
}

// GetKeys lists the keys present in one dataset.
func GetKeys(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetKeys),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Get Top Keys for a Specific Log ID",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Fetches all keys present in a dataset, which is represented by a log ID. A log ID is " +
				"a UUID that maps to a dataset and collection name; given those names, " +
				string(MethodGetDatasetsByName) + " returns it. This tool returns the names of the keys " +
				"present in the dataset.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"log_id": {
						Type:        "string",
						Description: "The dataset ID, also named log ID.",
						MinLength:   new(logIDLength),
						MaxLength:   new(logIDLength),
					},
				},
				Required: []string{"log_id"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			logID, err := arguments.RequireString("log_id")
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			keys, err := client.DatasetKeys(ctx, logID)
			if err != nil {
				return errorResult(err, "")
			}
			return helpers.NewToolResultJSON(keys)
		},
	}
}

// GetAllDatasetsKeys lists the keys present in every dataset.
func GetAllDatasetsKeys(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetAllDatasetsKeys),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Retrieve Top Keys for Logs",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Fetches all keys present in all datasets, as a map from dataset ID to the names of the " +
				"keys present in that dataset. This tool is useful to select datasets that contain certain " +
				"keys, and to identify the exact key name based on some description.",
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		Handler: func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			keys, err := client.AllDatasetKeys(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			return helpers.NewToolResultJSON(keys)
		},
	}
}

// GetKeyValues lists sample values of one key in one dataset.
func GetKeyValues(client *brontosdk.Client) toolsets.ToolWrapper {
	return toolsets.ToolWrapper{
		Tool: &mcp.Tool{
			Name: string(MethodGetKeyValues),
			Annotations: &mcp.ToolAnnotations{
				Title:           "Retrieve Values for a Dataset Key",
				ReadOnlyHint:    true,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
			Description: "Fetches the values of the provided key in the provided dataset. This tool returns a " +
				"list of strings, each a value of the key seen in that dataset.",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"key": {
						Type:        "string",
						Description: "The name of a key.",
					},
					"log_id": {
						Type:        "string",
						Description: "The dataset ID, also named log ID.",
					},
				},
				Required: []string{"key", "log_id"},
			},
		},
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments, err := helpers.NewToolArguments(request)
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			key, err := arguments.RequireString("key")
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}
			logID, err := arguments.RequireString("log_id")
			if err != nil {
				return helpers.NewToolResultTextError("%v", err), nil
			}

			keyValues, err := client.AllDatasetKeyValues(ctx)
			if err != nil {
				return errorResult(err, "")
			}
			values := keyValues[logID][key]
			if values == nil {
				values = []string{}
			}
			return helpers.NewToolResultJSON(values)
		},
	}
}
