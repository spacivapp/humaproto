# huma proto
This allows huma to work with protobuf

Example:
```
config := huma.Config{
    OpenAPI: &huma.OpenAPI{
        Components: &huma.Components{
            Schemas: humaproto.NewRegistry(schemaPrefix, huma.DefaultSchemaNamer),
        },
        ...
    },
    Formats: map[string]huma.Format{
        "application/json": humaproto.JSONFormat,
        "json":             humaproto.JSONFormat,
    },
    ...
}
```
## Null message fields in a client spec

`JSONFormat` sends an unset message field as `null`. The registry still describes those fields as non-nullable, because huma reports a failed `anyOf` without its branch's errors, and request validation uses these schemas. Instead, each such field carries `x-proto-nullable: true`.

To write a spec for generating client types, build the API only for that purpose and call `ApplyNullable` before marshalling. It rewrites every marked field as `anyOf: [<field>, {type: null, enum: [null]}]`:

```
humaproto.ApplyNullable(api.OpenAPI().Components.Schemas)
spec, err := api.OpenAPI().YAML()
```

`ApplyNullable` changes the registry in place. Don't call it on an API that serves requests.
