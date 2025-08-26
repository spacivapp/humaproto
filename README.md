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