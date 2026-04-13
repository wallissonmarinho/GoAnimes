# GoAnimes

Addon **Stremio** em Go: cadastra **uma ou mais URLs de RSS** por API, sincroniza em intervalo, filtra releases com legenda **pt-BR** no modelo Erai (`[br]` em `<erai:subtitles>`) e expõe catálogo/meta/streams por torrent ou magnet.

**Erai:** não precisas cadastrar um RSS por anime. Basta **um** feed global (ex. `https://www.erai-raws.info/feed/?type=torrent&token=…`). Em cada sync o servidor lê esse feed, descobre slugs a partir dos links `/episodes/…` e `/anime-list/…` nos itens e **busca sozinho** cada `…/anime-list/{slug}/feed/?token=…` (mesmo token do feed cadastrado), até ao limite `GOANIMES_ERAI_MAX_PER_ANIME_FEEDS` (default 200; `0` = ilimitado).

## Build

```bash
go build -o bin/goanimes ./cmd/goanimes
```

## IDE (Cursor / VS Code) — “missing metadata for import” / `go.work`

- O `go.mod` fixa **Go 1.25** (dependências como `gopkg.gilang.dev/translator/v2` exigem ≥ 1.25). Usa **Go 1.25+** no PATH ou mantém **`GOTOOLCHAIN=auto`** para o toolchain descarregar a versão certa.
- Se a raiz do workspace for uma pasta **mãe** (ex.: `www`) sem `go.mod`, o **gopls** pode falhar imports: **abre a pasta `GoAnimes` como raiz** (recomendado).
- Um **`go.work`** na pasta mãe com `go 1.25` **exige** binário `go` ≥ 1.25; com Go 1.24 vês `go.work requires go >= 1.25`. **Atualiza o Go** ou **não uses** `go.work` nesse cenário.

Isto **não** vem do golangci-lint; o lint corre no `go build`/`go test` e na CI.

## Lint (local)

O repositório inclui [`.golangci.yml`](.golangci.yml) (golangci-lint **v2**). O **`make lint`** usa **`go run …/golangci-lint@v2.11.4`** por defeito (não depende de um `golangci-lint` no PATH compilado com Go 1.25). Opcional: binário global — [install](https://golangci-lint.run/welcome/install/) **v2.11+** e `make lint GOLANGCI_LINT=golangci-lint`. Na CI usa-se a action **v2.11**.

```bash
make lint    # check-layout + go vet + go run golangci-lint@v2.11.4 run ./...
```

O script `scripts/check-layout.sh` falha se existirem cópias de RSS sync em `internal/core/services/` (ex. `rss_sync_fetch.go`) — causa típica do erro **undefined: RSSSyncService** no IDE.

A **CI** (`ci`, `oracle-deploy`, `release`) corre `go vet` e **golangci-lint** antes dos testes; falhas bloqueiam merge/deploy/tags.

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
| `GOANIMES_SYNC_STATUS_TZ` | Fuso IANA só para `started_at` / `finished_at` em `GET /api/v1/sync-status`. **Brasília:** `America/Sao_Paulo`. Vazio = **UTC** (`Z`). O servidor continua a gravar sync em UTC na base. |
| `GOANIMES_SYNC_INTERVAL` | Intervalo de **sync completo** (default `30m`) — metadados, Erai por anime, etc. |
| `GOANIMES_RSS_POLL_INTERVAL` | Sondagem dos feeds RSS **principais** (default `1m`; `0` desliga). Compara o feed atual com o **último corpo usado no build** guardado no snapshot (`sha256` + `ETag`/`Last-Modified`); se diferente (ou fonte nova/removida), dispara sync completo. Sem baseline ainda (primeira subida), adota o feed atual sem rebuild até o próximo sync gravar metadados. Não cobre mudança só em feeds Erai por-anime se o feed global não mudar. |
| `GOANIMES_GOAI_AUDIT_ENABLED` | `true`, `1`, `yes` ou `on` liga o **loop em background** que audita o catálogo via serviço **GoAI** (independente do sync RSS). Sem isto o loop não arranca. |
| `GOANIMES_GOAI_AUDIT_INTERVAL` | Intervalo entre passadas do auditor (default `12h`). Tem de ser **> 0** para o loop arrancar quando o audit está ligado. |
| `GOANIMES_GOAI_HTTP_TIMEOUT` | Timeout HTTP dedicado para chamadas do GoAnimes ao GoAI (default: herda `GOANIMES_HTTP_TIMEOUT`, que por sua vez default `45s`). Em produção com Gemini, costuma ser melhor `90s`/`120s`. |
| `GOANIMES_GOAI_BASE_URL` | URL base do GoAI (ex. `https://goai.example.com`), sem barra final. Obrigatória para o loop quando o audit está ligado. |
| `GOANIMES_GOAI_ADMIN_API_KEY` | Token **Bearer** para os endpoints `/v1/audit/*` do GoAI. Tratar como segredo (GitHub **Secret** em `prd`). Obrigatória para o loop quando o audit está ligado. |

**GoAI (admin HTTP):** com `GOANIMES_ADMIN_API_KEY` (ou `ADMIN_API_KEY`), `GET /api/v1/goai-audit/series` lista séries já auditadas (retorna `items`, `limit`, `offset`, `total` para paginação); `POST /api/v1/goai-audit/series/:id/reaudit` marca re-auditoria (só se já existir linha em `goai_series_audit`). Corpo JSON opcional: `{"scope":"full"}` (omissão ou `full`/`default`) apaga antes as linhas `goai_release_audit` dessa série; `{"scope":"series_only"}` ou `"flag_only"` só define `needs_reaudit` sem apagar releases em cache.

**Fingerprint RSS (várias URLs):** a fonte de verdade continua a ser o mapa `rss_main_feed_build` dentro de `items_json` (uma entrada por URL de feed principal: `sha256` do corpo + `etag` / `last_modified` quando existem). Uma única coluna tipo `last_rss_content_fingerprint` não substitui isso com várias fontes; mantém-se o modelo em JSON até haver requisito explícito de tabela dedicada por URL.

**Catálogo relacional:** migração `00002` cria `catalog_series` e `catalog_item` (FK `series_id`). Cada `SaveCatalogSnapshot` atualiza **numa transação** o `items_json` (itens + AniList + fingerprints) e substitui as linhas normalizadas, para não ficar BD inconsistente se falhar a meio. Na primeira leitura após migrar, se as tabelas estiverem vazias mas o JSON tiver itens, o servidor faz backfill automático para as tabelas. Com dados nas tabelas, o carregamento usa **itens e séries** a partir do SQL (metadados AniList / RSS continuam no JSON).
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

- **`ci`** — `go vet`, **golangci-lint**, `go test`, build em **PRs** para `main`/`master` e em **push** para outras branches. **Não** corre em push direto em `main`/`master` (evita duplicar testes com o oracle-deploy).
- **`oracle-deploy`** — `go vet`, **golangci-lint**, `go test`, imagem Docker e deploy por **SSH** na VM em push para `main`/`master` ou manual.
- **`release`** (tags `v*`) — mesmo gate de lint + testes antes de publicar o binário.

O job **deploy** usa **`environment: prd`**. **Repository secrets:** **`OCI_*`**, **`GHCR_*`**. No ambiente **`prd`**, tudo o que definires como **Secret** ou **Variable** (nomes listados no comentário do `.github/workflows/oracle-deploy.yml`) é gravado em **`deploy/oracle/.env.goanimes.deploy`** na VM a cada deploy — não precisas de SSH para essas chaves. Secret opcional **`GOANIMES_ENV_B64`**: conteúdo extra em base64 (ex. `base64 -i snippet.env | tr -d '\n'`) acrescentado ao fim do ficheiro. **`ACME_EMAIL`** (Caddy) continua no **`.env`** na VM.

## Migrations

Ao arrancar, `storage.Open` (usado por `app.OpenCatalog`) corre **Goose** em cima do DSN — **não** é preciso um passo separado no GitHub Actions nem no `docker compose up`: cada novo deploy que sobe o binário aplica migrações em falta na base antes de servir tráfego.

Para correr migrações à mão (operacional, outro DSN):

```bash
./bin/goanimes migrate up
```

## Testes

Ver [tests/README.md](tests/README.md).

```bash
go test ./...
```
