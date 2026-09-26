package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func argString(req mcp.CallToolRequest, k string) string {
	if v, ok := req.GetArguments()[k].(string); ok {
		return v
	}
	return ""
}

func argInt(req mcp.CallToolRequest, k string) int {
	switch v := req.GetArguments()[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func argBool(req mcp.CallToolRequest, k string) bool {
	switch v := req.GetArguments()[k].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "X" || v == "x"
	}
	return false
}

func (s *Server) handleCreateDataElement(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.DataElementOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"),
		Domain: argString(req, "domain"), DataType: argString(req, "data_type"),
		Length: argInt(req, "length"), Decimals: argInt(req, "decimals"),
		Short: argString(req, "short_label"), Medium: argString(req, "medium_label"),
		Long: argString(req, "long_label"), Heading: argString(req, "heading_label"),
		ChangeDocument: argBool(req, "change_document"),
	}
	if o.Name == "" || o.Description == "" {
		return newToolResultError("name and description are required"), nil
	}
	objURL, err := s.adtClient.CreateDataElement(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": o.Name, "objectUrl": objURL, "package": o.Package}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleCreateDomain(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.DomainOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"),
		DataType: argString(req, "data_type"), Length: argInt(req, "length"),
		Decimals: argInt(req, "decimals"), OutputLen: argInt(req, "output_length"),
		Lowercase: argBool(req, "lowercase"),
	}
	if raw, ok := req.GetArguments()["fixed_values"].([]any); ok {
		for i, it := range raw {
			m, ok := it.(map[string]any)
			if !ok {
				return newToolResultError(fmt.Sprintf("fixed_values[%d] must be an object {low, high, text}", i)), nil
			}
			str := func(k string) string { v, _ := m[k].(string); return v }
			o.FixedValues = append(o.FixedValues, adt.DomainFixedValue{Low: str("low"), High: str("high"), Text: str("text")})
		}
	}
	if o.Name == "" || o.Description == "" {
		return newToolResultError("name and description are required"), nil
	}
	objURL, err := s.adtClient.CreateDomain(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": o.Name, "objectUrl": objURL, "package": o.Package,
		"fixedValues": len(o.FixedValues)}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleGetDDICObjectXML(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	t, n := argString(req, "object_type"), argString(req, "name")
	if t == "" || n == "" {
		return newToolResultError("object_type (DTEL|DOMA) and name are required"), nil
	}
	x, err := s.adtClient.GetDDICObjectXML(ctx, t, n)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetDDICObjectXML failed: %v", err)), nil
	}
	return mcp.NewToolResultText(x), nil
}
