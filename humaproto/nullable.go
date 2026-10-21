package humaproto

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// NullableExtension marks a message field, which JSONFormat sends as null when
// it is unset. The schema itself stays non-nullable: huma reports an anyOf
// mismatch without its branch's errors, so validating requests against the
// nullable form would lose them.
const NullableExtension = "x-proto-nullable"

// A field inside a oneof, including a proto3 optional field's synthetic
// oneof, is left out when unset rather than sent as null.
func isPresenceProtoField(f reflect.StructField) bool {
	tag := f.Tag.Get("protobuf")
	return tag != "" && !slices.Contains(strings.Split(tag, ","), "oneof")
}

func markNullable(s *huma.Schema) {
	if s.Extensions == nil {
		s.Extensions = map[string]any{}
	}
	s.Extensions[NullableExtension] = true
}

// ApplyNullable rewrites every marked field in the registry as its schema or
// null, for a spec that describes responses as JSONFormat writes them.
func ApplyNullable(r huma.Registry) {
	seen := map[*huma.Schema]bool{}
	for _, s := range r.Map() {
		applyNullable(s, seen)
	}
}

func applyNullable(s *huma.Schema, seen map[*huma.Schema]bool) {
	if s == nil || seen[s] {
		return
	}
	seen[s] = true
	for name, property := range s.Properties {
		if marked, _ := property.Extensions[NullableExtension].(bool); marked {
			// Another property may share this schema, so it keeps its marker
			// and the wrapper holds an unmarked copy.
			unmarked := *property
			unmarked.Extensions = maps.Clone(property.Extensions)
			delete(unmarked.Extensions, NullableExtension)
			// huma validates no "null" type, so the enum is what makes this
			// branch reject anything else.
			s.Properties[name] = &huma.Schema{
				AnyOf:       []*huma.Schema{&unmarked, {Type: "null", Enum: []any{nil}}},
				Description: property.Description,
				ReadOnly:    property.ReadOnly,
				WriteOnly:   property.WriteOnly,
			}
			seen[s.Properties[name]] = true
			property = &unmarked
		}
		applyNullable(property, seen)
	}
	applyNullable(s.Items, seen)
	if additional, ok := s.AdditionalProperties.(*huma.Schema); ok {
		applyNullable(additional, seen)
	}
	for _, branch := range slices.Concat(s.OneOf, s.AnyOf, s.AllOf) {
		applyNullable(branch, seen)
	}
	applyNullable(s.Not, seen)
}
