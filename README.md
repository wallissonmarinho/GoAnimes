# GoAnimes

Stremio addon em Go com arquitetura limpa/hexagonal. O sistema ingere feeds (RSS/Torznab), normaliza releases, resolve mapeamentos e entrega catalogo Stremio com fontes agrupadas por episodio.

## Build

```bash
go build -o bin/goanimes ./cmd/goanimes
```

## Run (local)

```bash
export GOANIMES_ADMIN_API_KEY="change-me"
export GOANIMES_MONGO_URI="mongodb://localhost:27017"
export GOANIMES_MONGO_DB="goanimes"
./bin/goanimes
```

- Escuta em `GOANIMES_ADDR` (default `:8080`) ou `PORT`.
- O sync e solicitado via `POST /admin/sync` (cron externo): resposta imediata (202); o trabalho corre em background; se ja houver sync ativo devolve `accepted: false`.

## Stremio

Em Addons -> Addon repository URL:

`http://<teu-host>:8080/manifest.json`

Catalogo principal: `/catalog/anime/goanimes.json`.

## API admin

Autenticacao: `Authorization: Bearer <chave>` ou `X-Admin-Key: <chave>`.

| Metodo | Caminho | Descricao |
|--------|---------|-----------|
| POST | `/admin/sync` | Agenda o sync em background (mutex: ignora se ja a correr) |
| DELETE | `/admin/clean/:feedId` | Remove todas as fontes de um feed específico do banco de dados |
| GET | `/admin/feeds` | Lista feeds |
| POST | `/admin/feeds` | Cria feed |
| PUT | `/admin/feeds/:id` | Atualiza feed |
| DELETE | `/admin/feeds/:id` | Remove feed |
| GET | `/admin/mapping-overrides` | Lista overrides |
| POST | `/admin/mapping-overrides` | Cria/atualiza override |
| GET | `/admin/unmatched` | Lista releases nao mapeadas |

## Variaveis de ambiente

| Variavel | Descricao |
|----------|-----------|
| `PORT` / `GOANIMES_ADDR` | Porta (ex: `:8080`) |
| `GOANIMES_ADMIN_API_KEY` / `ADMIN_API_KEY` | Chave admin |
| `GOANIMES_MONGO_URI` | URI do MongoDB (ex: `mongodb://localhost:27017`) |
| `GOANIMES_MONGO_DB` | Nome da base (default `goanimes`) |
| `GOANIMES_TMDB_API_KEY` | Chave TMDb para enrich de metadados |
| `GOANIMES_HTTP_TIMEOUT` | Timeout HTTP (default `45s`) |
| `OTEL_SERVICE_NAME` | Nome do servico para tracing (default `goanimes`) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Endpoint OTLP (ex: `http://jaeger.observability.svc.cluster.local:4318`) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Endpoint OTLP de traces (opcional, substitui o anterior) |
| `OTEL_SDK_DISABLED` | Desliga tracing quando `true` |

## Docker

```bash
docker build -t goanimes .
docker run -p 8080:8080 -e GOANIMES_MONGO_URI=mongodb://host.docker.internal:27017 goanimes
```

## Testes

```bash
go test ./...
```
