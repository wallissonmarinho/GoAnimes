# GoAnimes

Addon **Stremio** em Go: cadastra **uma ou mais URLs de RSS** por API, sincroniza em intervalo, filtra releases com legenda **pt-BR** no modelo Erai (`[br]` em `<erai:subtitles>`) e expõe catálogo/meta/streams por torrent ou magnet.

**Erai:** não precisas cadastrar um RSS por anime. Basta **um** feed global (ex. `https://www.erai-raws.info/feed/?type=torrent&token=…`). Em cada sync o servidor lê esse feed, descobre slugs a partir dos links `/episodes/…` e `/anime-list/…` nos itens e **busca sozinho** cada `…/anime-list/{slug}/feed/?token=…` (mesmo token do feed cadastrado), até ao limite `GOANIMES_ERAI_MAX_PER_ANIME_FEEDS` (default 200; `0` = ilimitado).

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

O catálogo na secção **anime** usa `/catalog/anime/goanimes.json`. Cada item no JSON tem **`type: "movie"`** para o Stremio pedir **meta/stream** de forma estável (só `anime` no meta costumava impedir play nalguns clientes). Manifest inclui `types: ["anime","movie"]`. Depois de atualizar: reinstala o addon e **`POST /api/v1/rebuild`**.

## API admin

Autenticação: `Authorization: Bearer <chave>` ou `X-Admin-API-Key: <chave>`.

| Método | Caminho | Descrição |
|--------|---------|-----------|
| POST | `/api/v1/rss-sources` | `{"url":"https://...feed...","label":"..."}` — URL do **feed global**; expansão Erai por anime é automática |
| GET | `/api/v1/rss-sources` | Lista fontes |
| DELETE | `/api/v1/rss-sources/:id` | Remove |
| POST | `/api/v1/rebuild` | Sincroniza feeds agora (202) |
| GET | `/api/v1/sync-status` | Último estado do sync persistido + **`sync_running`** (`true` enquanto um sync está a correr — intervalo ou `POST /rebuild`) |

## Postman

1. **Import** → ficheiro [`postman/GoAnimes.postman_collection.json`](postman/GoAnimes.postman_collection.json).
2. (Opcional) **Import** do ambiente [`postman/GoAnimes.local.postman_environment.json`](postman/GoAnimes.local.postman_environment.json) e seleciona-o no canto superior direito.
3. Ajusta `baseUrl`, `adminApiKey` e `sampleRssUrl` (variáveis da coleção ou do ambiente).
4. Se o servidor **não** tiver chave admin, na pasta **Admin** define **Authorization** → **No Auth**.

## Variáveis de ambiente

| Variável | Descrição |
|----------|-----------|
| `PORT` / `GOANIMES_ADDR` | Porta (ex. `:8080`) |
| `GOANIMES_DATA_DIR` | Diretório dos dados (default `./data`) |
| `GOANIMES_ADMIN_API_KEY` / `ADMIN_API_KEY` | Chave admin |
| `GOANIMES_SYNC_INTERVAL` | Intervalo de sync (default `30m`) |
| `GOANIMES_HTTP_TIMEOUT` | Timeout HTTP ao buscar RSS (default `45s`) |
| `GOANIMES_ERAI_MAX_PER_ANIME_FEEDS` | Máx. GETs a feeds por anime num sync Erai (default `200`; `0` = sem limite) |
| `GOANIMES_ERAI_PER_ANIME_DELAY` | Pausa entre cada GET `anime-list/{slug}/feed` (default `400ms`; ex. `800ms`, `1s`) — reduz HTTP 429 |
| `GOANIMES_ERAI_PER_ANIME_MAX_ATTEMPTS` | Tentativas por slug em 429/503 (default `5`; máx. `20`) |
| `GOANIMES_ERAI_PER_ANIME_RETRY_BACKOFF` | Primeiro backoff se não houver cabeçalho `Retry-After` (default `2s`; depois dobra até ~90s) |
| `DATABASE_URL` | Postgres opcional (`postgres://` / `postgresql://`) |

## Docker

```bash
docker build -t goanimes .
docker run -p 8080:8080 -v "$(pwd)/data:/app/data" goanimes
```

## Deploy (VM / Docker Compose)

Em produção o layout típico é **Docker Compose** com Caddy (ex.: pasta `deploy/oracle` no repositório de infra, com `GoAnimes` e `GoTV` como pastas irmãs). Variáveis e volumes: README desse compose.

## GitHub Actions

- **`ci`** — `go test` + build em **PRs** para `main`/`master` e em **push** para outras branches. **Não** corre em push direto em `main`/`master` (evita duplicar testes com o oracle-deploy).
- **`oracle-deploy`** — `go test` + deploy por **SSH** na VM (pull do repo + `docker compose build/up` do serviço GoAnimes) em push para `main`/`master` ou manual.

O job **deploy** usa **`environment: prd`**. **Repository secrets:** **`OCI_*`**, **`GHCR_*`**. No ambiente **`prd`**, tudo o que definires como **Secret** ou **Variable** (nomes listados no comentário do `.github/workflows/oracle-deploy.yml`) é gravado em **`deploy/oracle/.env.goanimes.deploy`** na VM a cada deploy — não precisas de SSH para essas chaves. Secret opcional **`GOANIMES_ENV_B64`**: conteúdo extra em base64 (ex. `base64 -i snippet.env | tr -d '\n'`) acrescentado ao fim do ficheiro. **`ACME_EMAIL`** (Caddy) continua no **`.env`** na VM.

## Migrations

```bash
./bin/goanimes migrate up
```

## Testes

Ver [tests/README.md](tests/README.md).

```bash
go test ./...
```
