package main

import (
	"encoding/json"
	"testing"
)

// Structured Outputs requires additionalProperties:false on every object node
// and rejects dynamic-keyed maps. These guard against schema typos that the API
// would otherwise reject at call time.
func TestSchemasValid(t *testing.T) {
	for _, rationale := range []bool{false, true} {
		for name, s := range map[string]map[string]any{
			"groups":  groupsSchema(rationale),
			"winners": winnersSchema(rationale),
			"tips":    tipsSchema(rationale),
		} {
			if _, err := json.Marshal(s); err != nil {
				t.Fatalf("%s schema (rationale=%v) not serializable: %v", name, rationale, err)
			}
			assertClosedObjects(t, name, s)
		}
	}
}

// assertClosedObjects recursively checks every object node sets
// additionalProperties:false.
func assertClosedObjects(t *testing.T, path string, node map[string]any) {
	switch node["type"] {
	case "object":
		if v, ok := node["additionalProperties"].(bool); !ok || v {
			t.Fatalf("%s: object missing additionalProperties:false", path)
		}
		props, _ := node["properties"].(map[string]any)
		for k, p := range props {
			if pm, ok := p.(map[string]any); ok {
				assertClosedObjects(t, path+"."+k, pm)
			}
		}
	case "array":
		if items, ok := node["items"].(map[string]any); ok {
			assertClosedObjects(t, path+"[]", items)
		}
	}
}
