# Testes (`tests/`)

Estrutura alinhada ao projeto GoAnimes: ficheiros `*_test.go` ficam **só** aqui, não junto ao código de produção em `internal/`.

Em Go o nome da pasta é **`tests/`** (inglês); o conteúdo é o que em PT costuma chamar-se "testes".

| Pasta | Conteúdo |
|-------|----------|
| `unit/` | Testes rápidos sem servidor HTTP (ex.: normalizacao de título RSS em `unit/app/sync/`). |
| `integration/http/` | Testes `httptest` contra o router Gin (Stremio, admin endpoints, etc.). |

Executar tudo:

```bash
go test ./...
```

Com verbosidade num pacote:

```bash
go test ./tests/integration/http -v
```
