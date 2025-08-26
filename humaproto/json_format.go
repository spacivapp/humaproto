package humaproto

import (
	"encoding/json"
	"io"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var JSONFormat = huma.Format{
	Marshal: func(w io.Writer, v any) error {
		if pv, ok := v.(proto.Message); ok {
			bytes, err := protojson.MarshalOptions{EmitUnpopulated: true, UseEnumNumbers: true}.Marshal(pv)
			if err != nil {
				return err
			}
			_, err = w.Write(bytes)
			return err
		}
		return json.NewEncoder(w).Encode(v)
	},
	Unmarshal: func(data []byte, v any) error {
		if reflect.TypeOf(v).Elem().Kind() == reflect.Pointer {
			rv := reflect.ValueOf(v).Elem()
			rv.Set(reflect.New(rv.Type().Elem()))
			if pv, ok := rv.Interface().(proto.Message); ok {
				return protojson.Unmarshal(data, pv)
			}
		}
		return json.Unmarshal(data, v)
	},
}
