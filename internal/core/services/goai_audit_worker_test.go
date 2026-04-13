package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

type workerTestRepo struct {
	seriesIDs   []string
	seriesRec   *domain.GoaiSeriesAuditRecord
	releaseKeys []domain.GoaiReleaseKey
	upserts     int
}

func (r *workerTestRepo) ListSeriesIDsWithCatalogItems(ctx context.Context) ([]string, error) {
	return r.seriesIDs, nil
}

func (r *workerTestRepo) GetSeriesAudit(ctx context.Context, seriesID string) (*domain.GoaiSeriesAuditRecord, error) {
	return r.seriesRec, nil
}

func (r *workerTestRepo) UpsertSeriesAudit(ctx context.Context, seriesID string, auditedAt time.Time, promptVersion int, responseJSON string, needsReaudit bool) error {
	panic("unexpected UpsertSeriesAudit")
}

func (r *workerTestRepo) UpsertReleaseAudit(ctx context.Context, seriesID string, season, episode int, isSpecial bool, auditedAt time.Time, promptVersion int, responseJSON, sourceTitle string) error {
	r.upserts++
	return nil
}

func (r *workerTestRepo) ListUnauditedReleaseKeysForSeries(ctx context.Context, seriesID string) ([]domain.GoaiReleaseKey, error) {
	return r.releaseKeys, nil
}

func (r *workerTestRepo) SampleItemTitleForSeries(ctx context.Context, seriesID string) (string, error) {
	panic("unexpected SampleItemTitleForSeries")
}

func (r *workerTestRepo) SampleItemTitleForRelease(ctx context.Context, key domain.GoaiReleaseKey) (string, error) {
	return "ep title", nil
}

func (r *workerTestRepo) ListSeriesAuditsForAdmin(ctx context.Context, limit, offset int) ([]domain.GoaiSeriesAuditListItem, error) {
	panic("unexpected ListSeriesAuditsForAdmin")
}

func (r *workerTestRepo) CountSeriesAuditsForAdmin(ctx context.Context) (int, error) {
	panic("unexpected CountSeriesAuditsForAdmin")
}

func (r *workerTestRepo) DeleteReleaseAuditsForSeries(ctx context.Context, seriesID string) error {
	panic("unexpected DeleteReleaseAuditsForSeries")
}

func (r *workerTestRepo) SetSeriesNeedsReaudit(ctx context.Context, seriesID string) error {
	panic("unexpected SetSeriesNeedsReaudit")
}

func (r *workerTestRepo) UpdateSeriesEnrichmentTVDB(ctx context.Context, seriesID string, tvdbSeriesID int) error {
	panic("unexpected UpdateSeriesEnrichmentTVDB")
}

func (r *workerTestRepo) GetEnrichmentTVDBSeriesID(ctx context.Context, seriesID string) (int, error) {
	panic("unexpected GetEnrichmentTVDBSeriesID")
}

func (r *workerTestRepo) GetCatalogSeriesName(ctx context.Context, seriesID string) (string, error) {
	return "Show", nil
}

var _ ports.GoAIAuditRepository = (*workerTestRepo)(nil)

type workerTestClient struct {
	releaseN int
}

func (c *workerTestClient) AuditSeries(ctx context.Context, req domain.GoaiSeriesAuditRequest) (*domain.GoaiSeriesAuditResponse, error) {
	panic("unexpected AuditSeries")
}

func (c *workerTestClient) AuditRelease(ctx context.Context, req domain.GoaiReleaseAuditRequest) (*domain.GoaiReleaseAuditResponse, error) {
	c.releaseN++
	if c.releaseN >= 2 {
		return nil, errors.New("simulated goai failure")
	}
	return &domain.GoaiReleaseAuditResponse{
		Season:    req.CurrentSeason,
		Episode:   req.CurrentEpisode,
		IsSpecial: req.IsSpecial,
	}, nil
}

var _ ports.GoAIAuditHTTPClient = (*workerTestClient)(nil)

// When GoAI fails on the second release in the same tick, the worker must not upsert the second release.
func TestGoaiAuditWorker_StopsMidRunOnGoAIError(t *testing.T) {
	repo := &workerTestRepo{
		seriesIDs: []string{"s1"},
		seriesRec: &domain.GoaiSeriesAuditRecord{NeedsReaudit: false},
		releaseKeys: []domain.GoaiReleaseKey{
			{SeriesID: "s1", Season: 1, Episode: 1},
			{SeriesID: "s1", Season: 1, Episode: 2},
		},
	}
	cli := &workerTestClient{}
	w := &GoaiAuditWorker{Repo: repo, Client: cli}
	w.Run(context.Background())
	if repo.upserts != 1 {
		t.Fatalf("expected 1 release upsert before stop, got %d", repo.upserts)
	}
	if cli.releaseN != 2 {
		t.Fatalf("expected 2 AuditRelease calls (second fails), got %d", cli.releaseN)
	}
}
