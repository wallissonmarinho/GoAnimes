# GoAnimes

Addon **Stremio** em Go: cadastra feeds **RSS** por API, sincroniza em intervalo, filtra releases com legenda **pt-BR** no modelo Erai (`[br]` em `<erai:subtitles>`) e expõe catálogo/meta/streams por torrent ou magnet.

## Build

```bash
go build -o bin/goanimes ./cmd/goanimes
```

## Run (local)

```bash
export GOANIMES_ADMIN_API_KEY="change-me"   # opcional; sem isto as rotas /api/v1 ficam abertas
./bin/goanimes
```

- Escuta em `GOANIMES_ADDR` (default `:8080`) ou `PORT`.
- SQLite: `./data/goanimes.db` (ou `GOANIMES_SQLITE_DSN` / `DATABASE_URL` para Postgres).

## Stremio

Em **Addons** → **Addon repository URL**:

`http://<teu-host>:8080/manifest.json`

## API admin

Autenticação: `Authorization: Bearer <chave>` ou `X-Admin-API-Key: <chave>`.

| Método | Caminho | Descrição |
|--------|---------|-----------|
| POST | `/api/v1/rss-sources` | `{"url":"https://...feed...","label":"..."}` |
| GET | `/api/v1/rss-sources` | Lista fontes |
| DELETE | `/api/v1/rss-sources/:id` | Remove |
| POST | `/api/v1/rebuild` | Sincroniza feeds agora (202) |
| GET | `/api/v1/sync-status` | Último estado do sync |

## Variáveis de ambiente

| Variável | Descrição |
|----------|-----------|
| `PORT` / `GOANIMES_ADDR` | Porta (ex. `:8080`) |
| `GOANIMES_DATA_DIR` | Diretório dos dados (default `./data`) |
| `GOANIMES_ADMIN_API_KEY` / `ADMIN_API_KEY` | Chave admin |
| `GOANIMES_SYNC_INTERVAL` | Intervalo de sync (default `30m`) |
| `GOANIMES_HTTP_TIMEOUT` | Timeout HTTP ao buscar RSS (default `45s`) |
| `DATABASE_URL` | Postgres opcional (`postgres://` / `postgresql://`) |

## Docker

```bash
docker build -t goanimes .
docker run -p 8080:8080 -v "$(pwd)/data:/app/data" goanimes
```

## Fly.io

1. Ajuste `app` em `fly.toml`.
2. `fly volumes create goanimes_data --region gru --size 1`
3. `fly secrets set GOANIMES_ADMIN_API_KEY=...`
4. `fly deploy`

## Migrations

```bash
./bin/goanimes migrate up
```

## Testes

Ver [tests/README.md](tests/README.md).

```bash
go test ./...
```
