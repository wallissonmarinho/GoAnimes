package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"

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
	pingCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
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
	migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer migrateCancel()
	if err := persistmigrate.RunMigrations(migrateCtx, db, pg); err != nil {
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
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	r := &catalogRepo{ex: tx, pg: c.pg}
	if err := r.saveCatalogSnapshotAndNormalized(ctx, snap); err != nil {
		return err
	}
	return tx.Commit()
}

// WithinCatalogTx runs fn with a CatalogRepository backed by a single SQL transaction (commit on nil error).
func (c *Catalog) WithinCatalogTx(ctx context.Context, fn func(ctx context.Context, repo ports.CatalogRepository) error) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	r := &catalogRepo{ex: tx, pg: c.pg}
	if err := fn(ctx, r); err != nil {
		return err
	}
	return tx.Commit()
}

func (c *Catalog) LoadCatalogSnapshot(ctx context.Context) (domain.CatalogSnapshot, error) {
	r0 := &catalogRepo{ex: c.db, pg: c.pg}
	snap, err := r0.loadCatalogSnapshotRow(ctx)
	if err != nil {
		return snap, err
	}
	n, err := r0.countCatalogItems(ctx)
	if err != nil {
		return domain.CatalogSnapshot{}, err
	}
	if n > 0 {
		return r0.mergeNormalizedIntoSnapshot(ctx, snap)
	}
	if len(snap.Items) > 0 {
		tx, err := c.db.BeginTx(ctx, nil)
		if err != nil {
			return snap, err
		}
		defer func() { _ = tx.Rollback() }()
		rt := &catalogRepo{ex: tx, pg: c.pg}
		s2 := snap
		domain.EnsureSnapshotGrouped(&s2)
		if err := rt.replaceNormalizedCatalog(ctx, s2); err != nil {
			return domain.CatalogSnapshot{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.CatalogSnapshot{}, err
		}
	}
	return r0.mergeNormalizedIntoSnapshot(ctx, snap)
}

var _ ports.CatalogRepository = (*Catalog)(nil)
var _ ports.CatalogRepository = (*catalogRepo)(nil)
var _ ports.UnitOfWork = (*Catalog)(nil)
