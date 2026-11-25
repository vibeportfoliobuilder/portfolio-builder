package livetool

// schema.go - one JSON Schema built from the SAME jsonschema tags the SDK provider reads, so
// the providers cannot describe a tool differently; the MCP SDK reads that tag in another dialect.

import (
	"reflect"
	"strings"
)

// Schema builds a JSON Schema object for an argument struct. It understands the
// two tag words this repository uses: "required" and "description=".
func Schema(v any) map[string]any {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	props := map[string]any{}
	var required []string
	if t == nil || t.Kind() != reflect.Struct {
		return object(props, required)
	}
	for i := range t.NumField() {
		f := t.Field(i)
		name, ok := jsonName(f)
		if !ok {
			continue
		}
		p := map[string]any{"type": jsonType(f.Type)}
		for _, word := range strings.Split(f.Tag.Get("jsonschema"), ",") {
			switch word = strings.TrimSpace(word); {
			case word == "required":
				required = append(required, name)
			case strings.HasPrefix(word, "description="):
				p["description"] = strings.TrimPrefix(word, "description=")
			}
		}
		props[name] = p
	}
	return object(props, required)
}

func object(props map[string]any, required []string) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// jsonName is the field's wire name, and whether it is serialised at all.
func jsonName(f reflect.StructField) (string, bool) {
	if f.PkgPath != "" {
		return "", false
	}
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		name = f.Name
	}
	return name, true
}

// jsonType maps the handful of Go kinds these argument structs use. Anything
// else is described as a string rather than guessed at.
func jsonType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return "string"
	}
}
