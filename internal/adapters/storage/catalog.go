package storage

import (
	"context"
	"database/sql"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
	persistmigrate "github.com/wallissonmarinho/GoAnimes/internal/persistence/migrate"
)

// Catalog implements ports.CatalogRepository.
type Catalog struct {
	db *sql.DB
	pg bool
}

// OpenDB opens and pings a database handle without running migrations.
func OpenDB(dsn string) (*sql.DB, bool, error) {
	dsn = strings.TrimSpace(dsn)
	var (
		db  *sql.DB
		err error
		pg  bool
	)
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		db, err = sql.Open("pgx", dsn)
		pg = true
	} else {
		db, err = sql.Open("sqlite", dsn)
	}
	if err != nil {
		return nil, false, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, false, err
	}
	return db, pg, nil
}

// Open connects using a DSN and runs migrations.
func Open(dsn string) (*Catalog, error) {
	db, pg, err := OpenDB(dsn)
	if err != nil {
		return nil, err
	}
	if err := persistmigrate.RunMigrations(context.Background(), db, pg); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Catalog{db: db, pg: pg}, nil
}

// Close releases the DB handle.
func (c *Catalog) Close() error {
	return c.db.Close()
}

func (c *Catalog) base() *catalogRepo {
	return &catalogRepo{ex: c.db, pg: c.pg}
}

func (c *Catalog) FindRSSSourceByURL(ctx context.Context, url string) (*domain.RSSSource, error) {
	return c.base().FindRSSSourceByURL(ctx, url)
}

func (c *Catalog) CreateRSSSource(ctx context.Context, url, label string) (*domain.RSSSource, error) {
	return c.base().CreateRSSSource(ctx, url, label)
}

func (c *Catalog) ListRSSSources(ctx context.Context) ([]domain.RSSSource, error) {
	return c.base().ListRSSSources(ctx)
}

func (c *Catalog) DeleteRSSSource(ctx context.Context, id string) error {
	return c.base().DeleteRSSSource(ctx, id)
}

func (c *Catalog) SaveCatalogSnapshot(ctx context.Context, snap domain.CatalogSnapshot) error {
	return c.base().SaveCatalogSnapshot(ctx, snap)
}

func (c *Catalog) LoadCatalogSnapshot(ctx context.Context) (domain.CatalogSnapshot, error) {
	return c.base().LoadCatalogSnapshot(ctx)
}

var _ ports.CatalogRepository = (*Catalog)(nil)
