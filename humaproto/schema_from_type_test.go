package humaproto

import (
	"encoding/json"
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
	Name  string      `protobuf:"bytes,3,opt,name=name,proto3" json:"name,omitempty"`
}

type protoExplicit struct {
	Label *string `protobuf:"bytes,1,opt,name=label,proto3,oneof" json:"label,omitempty" nullable:"false"`
}

type protoFlagged struct {
	Inner *protoInner `protobuf:"bytes,1,opt,name=inner,proto3" json:"inner,omitempty" readOnly:"true" doc:"The inner value."`
}

type plainOuter struct {
	Count *int `json:"count,omitempty"`
}

func outerSchema(t *testing.T, value any) (huma.Registry, *huma.Schema) {
	t.Helper()
	r := NewRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(value), true, "")
	return r, r.SchemaFromRef(s.Ref)
}

func validates(t *testing.T, r huma.Registry, s *huma.Schema, value string) bool {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		t.Fatal(err)
	}
	res := &huma.ValidateResult{}
	huma.Validate(r, s, huma.NewPathBuffer([]byte(""), 0), huma.ModeWriteToServer, v, res)
	return len(res.Errors) == 0
}

func TestProtoPointerFieldsAreMarkedNullable(t *testing.T) {
	_, s := outerSchema(t, protoOuter{})

	for name, want := range map[string]bool{"inner": true, "label": true, "name": false} {
		marked, _ := s.Properties[name].Extensions[NullableExtension].(bool)
		if marked != want {
			t.Errorf("%s marked = %v, want %v", name, marked, want)
		}
	}
}

func TestAPlainPointerIsNotMarked(t *testing.T) {
	_, s := outerSchema(t, plainOuter{})

	if _, marked := s.Properties["count"].Extensions[NullableExtension]; marked || s.Properties["count"].Nullable {
		t.Fatalf("count = %+v, but encoding/json never sends null for it", s.Properties["count"])
	}
}

func TestValidationIgnoresTheMarker(t *testing.T) {
	r, s := outerSchema(t, protoOuter{})

	if validates(t, r, s, `{"inner": null, "name": ""}`) {
		t.Fatal("a marked field still validates as non-nullable until ApplyNullable")
	}
}

func TestApplyNullableAcceptsNullOrTheValueAndNothingElse(t *testing.T) {
	r, s := outerSchema(t, protoOuter{})
	ApplyNullable(r)
	ApplyNullable(r)

	inner := s.Properties["inner"]
	if len(inner.AnyOf) != 2 || inner.AnyOf[0].Ref == "" || inner.AnyOf[1].Type != "null" {
		t.Fatalf("inner = %+v, want anyOf the message reference and null", inner)
	}
	for value, want := range map[string]bool{
		`{"inner": null, "label": null, "name": ""}`:          true,
		`{"inner": {"value": "x"}, "label": "l", "name": ""}`: true,
		`{"inner": "text", "name": ""}`:                       false,
		`{"label": 3, "name": ""}`:                            false,
	} {
		if got := validates(t, r, s, value); got != want {
			t.Errorf("%s: valid = %v, want %v", value, got, want)
		}
	}
}

func TestAnExplicitlyNonNullableProtoFieldIsNotMarked(t *testing.T) {
	_, s := outerSchema(t, protoExplicit{})

	if _, marked := s.Properties["label"].Extensions[NullableExtension]; marked {
		t.Fatal("label is tagged nullable:\"false\" but was marked nullable")
	}
}

func TestApplyNullableKeepsTheFieldsFlagsAndDescription(t *testing.T) {
	r, s := outerSchema(t, protoFlagged{})
	ApplyNullable(r)

	inner := s.Properties["inner"]
	if !inner.ReadOnly || inner.WriteOnly || inner.Description != "The inner value." {
		t.Fatalf("inner = %+v, want readOnly and its description kept on the nullable wrapper", inner)
	}
}
