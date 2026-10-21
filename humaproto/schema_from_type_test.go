package humaproto

import (
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// Tagged as protoc-gen-go tags a proto3 message.
type protoInner struct {
	Value string `protobuf:"bytes,1,opt,name=value,proto3" json:"value,omitempty"`
}

type protoOuter struct {
	Inner *protoInner `protobuf:"bytes,1,opt,name=inner,proto3" json:"inner,omitempty"`
	Label *string     `protobuf:"bytes,2,opt,name=label,proto3,oneof" json:"label,omitempty"`
}

func schemaOf(t *testing.T, value any) *huma.Schema {
	t.Helper()
	r := NewRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(value), true, "")
	if s.Ref != "" {
		s = r.SchemaFromRef(s.Ref)
	}
	return s
}

func TestAProtoMessageFieldIsItsMessageOrNull(t *testing.T) {
	s := schemaOf(t, protoOuter{})

	inner := s.Properties["inner"]
	if len(inner.AnyOf) != 2 || inner.AnyOf[0].Ref == "" || inner.AnyOf[1].Type != "null" {
		t.Fatalf("inner = %+v, want anyOf the message reference and null", inner)
	}
}

func TestAnOptionalProtoScalarIsNullable(t *testing.T) {
	s := schemaOf(t, protoOuter{})

	if label := s.Properties["label"]; label.Type != huma.TypeString || !label.Nullable {
		t.Fatalf("label = %+v, want a nullable string", label)
	}
}

func TestAPlainPointerWithOmitemptyStaysNonNullable(t *testing.T) {
	type plain struct {
		Count *int `json:"count,omitempty"`
	}

	s := schemaOf(t, plain{})

	if s.Properties["count"].Nullable {
		t.Fatalf("count is nullable, but encoding/json never sends null for it")
	}
}
