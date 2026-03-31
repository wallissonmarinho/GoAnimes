# Testes (`tests/`)

Estrutura alinhada ao [GoTV](https://github.com/wallissonmarinho/GoTV): ficheiros `*_test.go` ficam **só** aqui, não junto ao código de produção em `internal/`.

| Pasta | Conteúdo |
|-------|----------|
| `unit/` | Testes rápidos sem servidor HTTP (ex.: parser RSS em `unit/adapters/rss/`). |
| `integration/http/ginapi/` | Testes `httptest` contra o router Gin (Stremio, CORS, etc.). |

Executar tudo:

```bash
go test ./...
```

Com verbosidade num pacote:

```bash
go test ./tests/integration/http/ginapi -v
```
