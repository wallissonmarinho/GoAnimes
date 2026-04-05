package ports

import "context"

// UnitOfWork runs multiple catalog persistence steps in a single database transaction.
// On any error from fn, the transaction rolls back so partial writes do not leave inconsistent state.
type UnitOfWork interface {
	WithinCatalogTx(ctx context.Context, fn func(ctx context.Context, repo CatalogRepository) error) error
}
