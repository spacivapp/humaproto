package humaproto

import (
	"encoding"
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
)

func SchemaFromType(r huma.Registry, t reflect.Type) *huma.Schema {
	s := schemaFromType(r, t)
	t = deref(t)

	// Transform generated schema if type implements SchemaTransformer
	v := reflect.New(t).Interface()
	if st, ok := v.(huma.SchemaTransformer); ok {
		return st.TransformSchema(r, s)
	}
	return s
}

var protobufNameRegex = regexp.MustCompile(`^.*name\=([a-zA-Z0-9]+)\,.*$`)

func schemaFromType(r huma.Registry, t reflect.Type) *huma.Schema {
	isPointer := t.Kind() == reflect.Pointer

	oriT := t

	s := huma.Schema{}
	t = deref(t)

	v := reflect.New(t).Interface()
	if sp, ok := v.(huma.SchemaProvider); ok {
		// Special case: type provides its own schema. Do not try to generate.
		custom := sp.Schema(r)
		custom.PrecomputeMessages()
		return custom
	}

	// Handle special cases for known stdlib types.
	switch t {
	case timeType:
		return &huma.Schema{Type: huma.TypeString, Nullable: isPointer, Format: "date-time"}
	case urlType:
		return &huma.Schema{Type: huma.TypeString, Nullable: isPointer, Format: "uri"}
	case ipType:
		return &huma.Schema{Type: huma.TypeString, Nullable: isPointer, Format: "ipv4"}
	case ipAddrType:
		return &huma.Schema{Type: huma.TypeString, Nullable: isPointer, Format: "ipv4"}
	case rawMessageType:
		return &huma.Schema{}
	}

	if _, ok := v.(encoding.TextUnmarshaler); ok {
		// Special case: types that implement encoding.TextUnmarshaler are able to
		// be loaded from plain text, and so should be treated as strings.
		// This behavior can be overidden by implementing `huma.SchemaProvider`
		// and returning a custom schema.
		return &huma.Schema{Type: huma.TypeString, Nullable: isPointer}
	}

	minZero := 0.0
	switch t.Kind() {
	case reflect.Bool:
		s.Type = huma.TypeBoolean
	case reflect.Int:
		s.Type = huma.TypeInteger
		if bits.UintSize == 32 {
			s.Format = "int32"
		} else {
			s.Format = "int64"
		}
	case reflect.Int8, reflect.Int16, reflect.Int32:
		s.Type = huma.TypeInteger
		s.Format = "int32"
	case reflect.Int64:
		s.Type = huma.TypeInteger
		s.Format = "int64"
	case reflect.Uint:
		s.Type = huma.TypeInteger
		if bits.UintSize == 32 {
			s.Format = "int32"
		} else {
			s.Format = "int64"
		}
		s.Minimum = &minZero
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		// Unsigned integers can't be negative.
		s.Type = huma.TypeInteger
		s.Format = "int32"
		s.Minimum = &minZero
	case reflect.Uint64:
		// Unsigned integers can't be negative.
		s.Type = huma.TypeInteger
		s.Format = "int64"
		s.Minimum = &minZero
	case reflect.Float32:
		s.Type = huma.TypeNumber
		s.Format = "float"
	case reflect.Float64:
		s.Type = huma.TypeNumber
		s.Format = "double"
	case reflect.String:
		s.Type = huma.TypeString
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// Special case: []byte will be serialized as a base64 string.
			s.Type = huma.TypeString
			s.ContentEncoding = "base64"
		} else {
			s.Type = huma.TypeArray
			s.Nullable = true
			s.Items = r.Schema(t.Elem(), true, t.Name()+"Item")

			if t.Kind() == reflect.Array {
				l := t.Len()
				s.MinItems = &l
				s.MaxItems = &l
			}
		}
	case reflect.Map:
		s.Type = huma.TypeObject
		s.AdditionalProperties = r.Schema(t.Elem(), true, t.Name()+"Value")
	case reflect.Struct:
		var required []string
		// requiredMap := map[string]bool{}
		// var propNames []string
		fieldSet := map[string]struct{}{}
		props := map[string]*huma.Schema{}
		dependentRequiredMap := map[string][]string{}
		ignoreAdditionalProperties := false

		fields := getFields(t, make(map[reflect.Type]struct{}))
		for _, info := range fields {
			f := info.Field

			if _, ok := fieldSet[f.Name]; ok {
				// This field was overridden by an ancestor type, so we
				// should ignore it.
				continue
			}

			if j := f.Tag.Get("protobuf_oneof"); j != "" {
				for i := range oriT.NumMethod() {
					m := oriT.Method(i)

					if m.Func.Type().NumOut() != 1 {
						continue
					}

					if !strings.HasPrefix(m.Name, "Get") {
						continue
					}

					if slices.ContainsFunc(fields, func(f fieldInfo) bool { return m.Name == "Get"+f.Field.Name }) {
						continue
					}

					cft := m.Func.Type().Out(0)

					fs := r.Schema(cft, true, t.Name()+cft.Name()+"Struct")
					if fs == nil {
						continue
					}

					getName := func(s string) string {
						s = s[3:]

						r, size := utf8.DecodeRuneInString(s)
						if r == utf8.RuneError && size <= 1 {
							return s
						}
						lc := unicode.ToLower(r)
						if r == lc {
							return s
						}
						return string(lc) + s[size:]
					}

					ignoreAdditionalProperties = true
					s.OneOf = append(s.OneOf, &huma.Schema{
						Type: "object",
						Properties: map[string]*huma.Schema{
							getName(m.Name): fs,
						},
						Required: []string{getName(m.Name)},
					})
				}

				continue
			}

			fieldSet[f.Name] = struct{}{}

			// Controls whether the field is required or not. All fields start as
			// required, then can be made optional with the `omitempty` JSON tag or it
			// can be overridden manually via the `required` tag.
			fieldRequired := true

			name := f.Name
			if j := f.Tag.Get("protobuf"); j != "" {
				if n := protobufNameRegex.FindStringSubmatch(j)[1]; n != "" {
					name = n
				}
				fieldRequired = !slices.Contains(strings.Split(j, ","), "oneof")
			} else if j := f.Tag.Get("json"); j != "" {
				if n := strings.Split(j, ",")[0]; n != "" {
					name = n
				}
				if strings.Contains(j, "omitempty") {
					fieldRequired = false
				}
			}
			if name == "-" {
				// This field is deliberately ignored.
				continue
			}

			if _, ok := f.Tag.Lookup("required"); ok {
				fieldRequired = boolTag(f, "required")
			}

			if boolTag(f, "hidden") {
				// This field is deliberately ignored. It may still exist, but won't
				// be documented.
				continue
			}

			if dr := f.Tag.Get("dependentRequired"); strings.TrimSpace(dr) != "" {
				dependentRequiredMap[name] = strings.Split(dr, ",")
			}

			fs := huma.SchemaFromField(r, f, t.Name()+f.Name+"Struct")
			if fs != nil {
				if j := f.Tag.Get("protobuf"); j != "" && f.Type.Kind() == reflect.Int64 { // Int64 in protojson are strings
					fs.Type = huma.TypeString
				}

				props[name] = fs
				// propNames = append(propNames, name)

				if fieldRequired {
					required = append(required, name)
					// requiredMap[name] = true
				}

				// Special case: pointer with omitempty and not manually set to
				// nullable, which will never get `null` sent over the wire.
				if f.Type.Kind() == reflect.Ptr && strings.Contains(f.Tag.Get("json"), "omitempty") && f.Tag.Get("nullable") != "true" {
					fs.Nullable = false
				}
			}
		}
		s.Type = huma.TypeObject

		// Check if the dependent fields exists. If they don't, panic with the correct message.
		var errs []string
		depKeys := make([]string, 0, len(dependentRequiredMap))
		for field := range dependentRequiredMap {
			depKeys = append(depKeys, field)
		}
		sort.Strings(depKeys)
		for _, field := range depKeys {
			dependents := dependentRequiredMap[field]
			for _, dependent := range dependents {
				if _, ok := props[dependent]; ok {
					continue
				}
				errs = append(errs, fmt.Sprintf("dependent field '%s' for field '%s' does not exist", dependent, field))
			}
		}
		if errs != nil {
			panic(errors.New(strings.Join(errs, "; ")))
		}

		if s.AdditionalProperties == nil && !ignoreAdditionalProperties {
			additionalProps := false
			if f, ok := t.FieldByName("_"); ok {
				if _, ok = f.Tag.Lookup("additionalProperties"); ok {
					additionalProps = boolTag(f, "additionalProperties")
				}

				if _, ok := f.Tag.Lookup("nullable"); ok {
					// Allow overriding nullability per struct.
					s.Nullable = boolTag(f, "nullable")
				}
			}
			s.AdditionalProperties = additionalProps
		}

		s.Properties = props
		// s.propertyNames = propNames
		s.Required = required
		s.DependentRequired = dependentRequiredMap
		// s.requiredMap = requiredMap
		s.PrecomputeMessages()
	case reflect.Interface:
		// Interfaces mean any object.
	default:
		return nil
	}

	switch s.Type {
	case huma.TypeBoolean, huma.TypeInteger, huma.TypeNumber, huma.TypeString:
		// Scalar types which are pointers are nullable by default. This can be
		// overidden via the `nullable:"false"` field tag in structs.
		s.Nullable = isPointer
	}

	return &s
}

func boolTag(f reflect.StructField, tag string) bool {
	if v := f.Tag.Get(tag); v != "" {
		if v == "true" {
			return true
		} else if v == "false" {
			return false
		} else {
			panic(fmt.Errorf("invalid bool tag '%s' for field '%s': %v", tag, f.Name, v))
		}
	}
	return false
}

// fieldInfo stores information about a field, which may come from an
// embedded type. The `Parent` stores the field's direct parent.
type fieldInfo struct {
	Parent reflect.Type
	Field  reflect.StructField
}

// getFields performs a breadth-first search for all fields including embedded
// ones. It may return multiple fields with the same name, the first of which
// represents the outermost declaration.
func getFields(typ reflect.Type, visited map[reflect.Type]struct{}) []fieldInfo {
	fields := make([]fieldInfo, 0, typ.NumField())
	var embedded []reflect.StructField

	if _, ok := visited[typ]; ok {
		return fields
	}
	visited[typ] = struct{}{}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}

		if f.Anonymous {
			embedded = append(embedded, f)
			continue
		}

		fields = append(fields, fieldInfo{typ, f})
	}

	for _, f := range embedded {
		newTyp := f.Type
		for newTyp.Kind() == reflect.Ptr {
			newTyp = newTyp.Elem()
		}
		if newTyp.Kind() == reflect.Struct {
			fields = append(fields, getFields(newTyp, visited)...)
		}
	}

	return fields
}
