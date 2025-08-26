package humaproto

import (
	"encoding"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func NewRegistry(prefix string, namer func(t reflect.Type, hint string) string) huma.Registry {
	return &protoJSONHumaRegistry{
		prefix: prefix,

		schemas: map[string]*huma.Schema{},
		types:   map[string]reflect.Type{},
		seen:    map[reflect.Type]bool{},
		aliases: map[reflect.Type]reflect.Type{},
		namer:   namer,
	}
}

type protoJSONHumaRegistry struct {
	prefix string

	schemas map[string]*huma.Schema
	types   map[string]reflect.Type
	seen    map[reflect.Type]bool
	namer   func(reflect.Type, string) string
	aliases map[reflect.Type]reflect.Type
}

var (
	timeType       = reflect.TypeOf(time.Time{})
	ipType         = reflect.TypeOf(net.IP{})
	ipAddrType     = reflect.TypeOf(netip.Addr{})
	urlType        = reflect.TypeOf(url.URL{})
	rawMessageType = reflect.TypeOf(json.RawMessage{})
)

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func (r *protoJSONHumaRegistry) Schema(t reflect.Type, allowRef bool, hint string) *huma.Schema {
	origType := t
	t = deref(t)

	// Pointer to array should decay to array
	if t.Kind() == reflect.Array || t.Kind() == reflect.Slice {
		origType = t
	}

	alias, ok := r.aliases[t]
	if ok {
		return r.Schema(alias, allowRef, hint)
	}

	getsRef := t.Kind() == reflect.Struct
	if t == timeType {
		// Special case: time.Time is always a string.
		getsRef = false
	}

	v := reflect.New(t).Interface()
	if _, ok := v.(huma.SchemaProvider); ok {
		// Special case: type provides its own schema
		getsRef = false
	}
	if _, ok := v.(encoding.TextUnmarshaler); ok {
		// Special case: type can be unmarshalled from text so will be a `string`
		// and doesn't need a ref. This simplifies the schema a little bit.
		getsRef = false
	}

	name := r.namer(origType, hint)

	if getsRef {
		if s, ok := r.schemas[name]; ok {
			if _, ok := r.seen[t]; !ok {
				// Name matches but type is different, so we have a dupe.

				panic(fmt.Errorf("duplicate name: %s, new type: %s, existing type: %s", name, t, r.types[name]))
			}
			if allowRef {
				return &huma.Schema{Ref: r.prefix + name}
			}
			return s
		}
	}

	// First, register the type so refs can be created above for recursive types.
	if getsRef {
		r.schemas[name] = &huma.Schema{}
		r.types[name] = t
		r.seen[t] = true
	}
	s := SchemaFromType(r, origType)
	if getsRef {
		r.schemas[name] = s
	}

	if getsRef && allowRef {
		return &huma.Schema{Ref: r.prefix + name}
	}
	return s
}

func (r *protoJSONHumaRegistry) SchemaFromRef(ref string) *huma.Schema {
	if !strings.HasPrefix(ref, r.prefix) {
		return nil
	}
	return r.schemas[ref[len(r.prefix):]]
}

func (r *protoJSONHumaRegistry) TypeFromRef(ref string) reflect.Type {
	return r.types[ref[len(r.prefix):]]
}

func (r *protoJSONHumaRegistry) Map() map[string]*huma.Schema {
	return r.schemas
}

func (r *protoJSONHumaRegistry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.schemas)
}

func (r *protoJSONHumaRegistry) MarshalYAML() (interface{}, error) {
	return r.schemas, nil
}

// RegisterTypeAlias(t, alias) makes the schema generator use the `alias` type instead of `t`.
func (r *protoJSONHumaRegistry) RegisterTypeAlias(t reflect.Type, alias reflect.Type) {
	r.aliases[t] = alias
}
