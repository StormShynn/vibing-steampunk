package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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

func (s *Server) handleGetADTObjectXML(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uri := argString(req, "object_uri")
	if uri == "" {
		return newToolResultError("object_uri is required"), nil
	}
	body, ctype, err := s.adtClient.GetADTObjectXML(ctx, uri)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetADTObjectXML failed: %v", err)), nil
	}
	if ctype != "" {
		body = "Content-Type: " + ctype + "\n\n" + body
	}
	return mcp.NewToolResultText(body), nil
}

func (s *Server) handleRunClass(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := argString(req, "class_name")
	if name == "" {
		return newToolResultError("class_name is required"), nil
	}
	out, err := s.adtClient.RunClass(ctx, name)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(out), nil
}

func (s *Server) handleCreateJobCatalogEntry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.JobCatalogOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		ClassName: argString(req, "class_name"), Package: argString(req, "package"), Transport: argString(req, "transport"),
	}
	objURL, err := s.adtClient.CreateJobCatalogEntry(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": o.Name, "objectUrl": objURL,
		"package": o.Package, "iamApp": strings.ToUpper(o.Name) + "_SAJC"}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleCreateJobTemplate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.JobTemplateOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		CatalogName: argString(req, "catalog_name"), Package: argString(req, "package"), Transport: argString(req, "transport"),
	}
	objURL, err := s.adtClient.CreateJobTemplate(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": o.Name, "objectUrl": objURL, "package": o.Package}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleCreateMessageClass(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.MessageClassOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"), Language: argString(req, "language"),
	}
	if raw := argString(req, "messages"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &o.Messages); err != nil {
			return newToolResultError(fmt.Sprintf("messages must be a JSON array of {\"number\":\"001\",\"text\":\"...\"}: %v", err)), nil
		}
	}
	objURL, err := s.adtClient.CreateMessageClass(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": strings.ToUpper(o.Name), "objectUrl": objURL,
		"messages": len(o.Messages)}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleCreateServerDrivenObject(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.ServerDrivenOptions{
		Type: argString(req, "object_type"), Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"), JSON: argString(req, "json_source"),
	}
	objURL, err := s.adtClient.CreateServerDrivenObject(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "type": strings.ToUpper(o.Type),
		"name": strings.ToUpper(o.Name), "objectUrl": objURL}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleCreateNumberRangeObject(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pw := 0.0
	if v, ok := req.GetArguments()["percent_warning"].(float64); ok {
		pw = v
	}
	o := adt.NumberRangeObjectOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"),
		Domain: argString(req, "domain"), PercentWarning: pw, Rolling: argBool(req, "rolling"),
		UntilYear: argBool(req, "until_year"), Buffering: argString(req, "buffering"), BufferedNumbers: argInt(req, "buffered_numbers"),
	}
	objURL, err := s.adtClient.CreateNumberRangeObject(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": strings.ToUpper(o.Name), "objectUrl": objURL,
		"next": "create intervals at runtime: CL_NUMBERRANGE_INTERVALS=>create (RunClass) or app Manage Number Range Intervals"}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (s *Server) handleCreateApplicationLogObject(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.ApplicationLogObjectOptions{
		Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"),
	}
	if raw := argString(req, "subobjects"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &o.Subobjects); err != nil {
			return newToolResultError(fmt.Sprintf("subobjects must be a JSON array of {\"name\":\"...\",\"description\":\"...\"}: %v", err)), nil
		}
	}
	objURL, err := s.adtClient.CreateApplicationLogObject(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{"status": "created", "name": strings.ToUpper(o.Name), "objectUrl": objURL,
		"subobjects": len(o.Subobjects)}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func argStringList(req mcp.CallToolRequest, k string) []string {
	raw := strings.TrimSpace(argString(req, k))
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var out []string
		if json.Unmarshal([]byte(raw), &out) == nil {
			return out
		}
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func createdResult(kind, name, objURL string, extra map[string]any) *mcp.CallToolResult {
	m := map[string]any{"status": "created", "type": kind, "name": strings.ToUpper(name), "objectUrl": objURL}
	for k, v := range extra {
		m[k] = v
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	return mcp.NewToolResultText(string(out))
}

func (s *Server) handleCreateAuthorizationField(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.AuthFieldOptions{Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"), DataElement: argString(req, "data_element")}
	u, err := s.adtClient.CreateAuthorizationField(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return createdResult("AUTH", o.Name, u, nil), nil
}

func (s *Server) handleCreateAuthorizationObject(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.AuthObjectOptions{Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"),
		Fields: argStringList(req, "fields"), Activities: argStringList(req, "activities"), NoActivity: argBool(req, "no_activity")}
	u, err := s.adtClient.CreateAuthorizationObject(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return createdResult("SUSO", o.Name, u, map[string]any{"next": "an IAM app / restriction type exposes it to business roles (SIA2/SIA5 in ADT)"}), nil
}

func (s *Server) handleCreateCommunicationScenario(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.CommScenarioOptions{Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"), InboundServices: argStringList(req, "inbound_services")}
	u, err := s.adtClient.CreateCommunicationScenario(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return createdResult("SCO1", o.Name, u, map[string]any{"next": "publish locally in ADT (Publish Locally) before an admin creates the communication arrangement"}), nil
}

func (s *Server) handleCreateBAdIImplementation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	o := adt.BAdIImplOptions{Name: argString(req, "name"), Description: argString(req, "description"),
		Package: argString(req, "package"), Transport: argString(req, "transport"),
		EnhancementSpot: argString(req, "enhancement_spot"), BAdIDefinition: argString(req, "badi_definition"),
		ImplementationName: argString(req, "implementation_name"), ImplementingClass: argString(req, "implementing_class"),
		Example: argBool(req, "example"), Default: argBool(req, "default")}
	u, err := s.adtClient.CreateBAdIImplementation(ctx, o)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return createdResult("ENHO", o.Name, u, nil), nil
}
