package livetool

// schema_test.go - the schema the MCP provider serves is built from the same
// tags the SDK provider reads, so this is what keeps the two descriptions equal.

import (
	"strings"
	"testing"
)

func TestSchemaReadsTheTagsTheOtherProviderReads(t *testing.T) {
	got := Schema(SearchArgs{})
	if got["type"] != "object" {
		t.Fatalf("type = %v, want object", got["type"])
	}
	props, _ := got["properties"].(map[string]any)
	query, _ := props["query"].(map[string]any)
	if query == nil {
		t.Fatalf("no query property: %v", props)
	}
	if query["type"] != "string" {
		t.Errorf("query type = %v, want string", query["type"])
	}
	desc, _ := query["description"].(string)
	if !strings.Contains(desc, "search for") {
		t.Errorf("the description did not survive the tag: %q", desc)
	}
	req, _ := got["required"].([]string)
	if len(req) != 1 || req[0] != "query" {
		t.Errorf("required = %v, want [query]", req)
	}
	// max_results is optional and must NOT be listed as required.
	if maxr, ok := props["max_results"].(map[string]any); !ok {
		t.Error("max_results is missing from the schema")
	} else if maxr["type"] != "integer" {
		t.Errorf("max_results type = %v, want integer", maxr["type"])
	}
}

// Every tool the MCP provider serves must describe every argument, or the two
// providers offer the same tool with different instructions.
func TestEveryToolArgumentIsDescribed(t *testing.T) {
	for name, args := range map[string]any{
		ToolWebSearch:       SearchArgs{},
		ToolFetchURL:        FetchArgs{},
		ToolNewsSearch:      NewsArgs{},
		ToolReadNewsArticle: ReadArticleArgs{},
	} {
		props, _ := Schema(args)["properties"].(map[string]any)
		if len(props) == 0 {
			t.Errorf("%s: schema has no properties", name)
			continue
		}
		for field, raw := range props {
			p, _ := raw.(map[string]any)
			desc, _ := p["description"].(string)
			if strings.TrimSpace(desc) == "" {
				t.Errorf("%s.%s has no description", name, field)
			}
			// A comma truncates the SDK provider's tag, so it must never appear.
			if strings.Contains(desc, ",") {
				t.Errorf("%s.%s description contains a comma: %q", name, field, desc)
			}
		}
	}
}
