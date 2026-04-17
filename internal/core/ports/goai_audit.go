package ports

import (
	"context"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// GoAIAuditRepository persists GoAI audit state and answers queue queries.
type GoAIAuditRepository interface {
	ListSeriesIDsWithCatalogItems(ctx context.Context) ([]string, error)
	GetSeriesAudit(ctx context.Context, seriesID string) (*domain.GoaiSeriesAuditRecord, error)
	UpsertSeriesAudit(ctx context.Context, seriesID string, auditedAt time.Time, promptVersion int, responseJSON string, needsReaudit bool) error
	UpsertReleaseAudit(ctx context.Context, seriesID string, season, episode int, isSpecial bool, auditedAt time.Time, promptVersion int, responseJSON, sourceTitle string) error
	ListUnauditedReleaseKeysForSeries(ctx context.Context, seriesID string) ([]domain.GoaiReleaseKey, error)
	SampleItemTitleForSeries(ctx context.Context, seriesID string) (string, error)
	SampleItemTitleForRelease(ctx context.Context, key domain.GoaiReleaseKey) (string, error)
	SampleItemContextForSeries(ctx context.Context, seriesID string) (*domain.GoaiAuditItemContext, error)
	SampleItemContextForRelease(ctx context.Context, key domain.GoaiReleaseKey) (*domain.GoaiAuditItemContext, error)
	CountSeriesAuditsForAdmin(ctx context.Context, params domain.GoaiAuditListParams) (int, error)
	ListSeriesAuditsForAdmin(ctx context.Context, params domain.GoaiAuditListParams) ([]domain.GoaiSeriesAuditListItem, error)
	DeleteReleaseAuditsForSeries(ctx context.Context, seriesID string) error
	SetSeriesNeedsReaudit(ctx context.Context, seriesID string) error
	UpdateSeriesEnrichmentTVDB(ctx context.Context, seriesID string, tvdbSeriesID int) error
	GetEnrichmentTVDBSeriesID(ctx context.Context, seriesID string) (int, error)
	GetCatalogSeriesName(ctx context.Context, seriesID string) (string, error)
}

// GoAIAuditHTTPClient calls the GoAI HTTP API (Bearer auth).
type GoAIAuditHTTPClient interface {
	AuditSeries(ctx context.Context, req domain.GoaiSeriesAuditRequest) (*domain.GoaiSeriesAuditResponse, error)
	AuditRelease(ctx context.Context, req domain.GoaiReleaseAuditRequest) (*domain.GoaiReleaseAuditResponse, error)
}
