package humaproto

import "github.com/danielgtaylor/huma/v2"

// NullableExtension marks a message or proto3 optional field, which
// JSONFormat sends as null when it is unset. The schema itself stays
// non-nullable: huma reports an anyOf mismatch without its branch's errors,
// so validating requests against the nullable form would lose them.
const NullableExtension = "x-proto-nullable"

func markNullable(s *huma.Schema) {
	if s.Extensions == nil {
		s.Extensions = map[string]any{}
	}
	s.Extensions[NullableExtension] = true
}

// ApplyNullable rewrites every marked field in the registry as its schema or
// null, for a spec that describes responses as JSONFormat writes them.
func ApplyNullable(r huma.Registry) {
	for _, s := range r.Map() {
		for name, property := range s.Properties {
			if marked, _ := property.Extensions[NullableExtension].(bool); !marked {
				continue
			}
			delete(property.Extensions, NullableExtension)
			// huma validates no "null" type, so the enum is what makes this
			// branch reject anything else.
			s.Properties[name] = &huma.Schema{
				AnyOf:       []*huma.Schema{property, {Type: "null", Enum: []any{nil}}},
				Description: property.Description,
				ReadOnly:    property.ReadOnly,
				WriteOnly:   property.WriteOnly,
			}
		}
	}
}
