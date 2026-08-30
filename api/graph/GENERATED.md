`generated.go` and `model/models_gen.go` are gqlgen output (`// Code generated ... DO NOT EDIT`).

Do not review them. The source of truth is `schema.graphqls`. To regenerate:

```sh
cd api && go generate ./...
```
