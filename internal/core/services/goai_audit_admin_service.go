package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

var (
	ErrGoaiAuditInvalidScope = errors.New("invalid scope")
	ErrGoaiAuditSeriesAbsent = errors.New("series has no goai_series_audit row yet")
)

type GoaiAuditAdminService struct {
	repo ports.GoAIAuditRepository
}

func NewGoaiAuditAdminService(repo ports.GoAIAuditRepository) *GoaiAuditAdminService {
	return &GoaiAuditAdminService{repo: repo}
}

var _ ports.GoAIAuditAdmin = (*GoaiAuditAdminService)(nil)

func normalizeReauditScope(scope string) (fullClear bool, ok bool) {
	s := strings.TrimSpace(strings.ToLower(scope))
	switch s {
	case "", "full", "default":
		return true, true
	case "series_only", "flag_only":
		return false, true
	default:
		return false, false
	}
}

func (s *GoaiAuditAdminService) ListSeriesAudits(ctx context.Context, in domain.GoaiAuditListParams) (domain.GoaiSeriesAuditPage, error) {
	if s == nil || s.repo == nil {
		return domain.GoaiSeriesAuditPage{}, errors.New("goai audit repository not configured")
	}
	limit := in.Limit
	offset := in.Offset
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	total, err := s.repo.CountSeriesAuditsForAdmin(ctx, domain.GoaiAuditListParams{
		ConfidenceMin: in.ConfidenceMin,
		ConfidenceMax: in.ConfidenceMax,
	})
	if err != nil {
		return domain.GoaiSeriesAuditPage{}, err
	}
	items, err := s.repo.ListSeriesAuditsForAdmin(ctx, domain.GoaiAuditListParams{
		Limit:         limit,
		Offset:        offset,
		ConfidenceMin: in.ConfidenceMin,
		ConfidenceMax: in.ConfidenceMax,
	})
	if err != nil {
		return domain.GoaiSeriesAuditPage{}, err
	}
	return domain.GoaiSeriesAuditPage{
		Items:  items,
		Limit:  limit,
		Offset: offset,
		Total:  total,
	}, nil
}

func (s *GoaiAuditAdminService) RequestSeriesReaudit(ctx context.Context, in domain.GoaiSeriesReauditRequest) (domain.GoaiSeriesReauditResult, error) {
	if s == nil || s.repo == nil {
		return domain.GoaiSeriesReauditResult{}, errors.New("goai audit repository not configured")
	}
	seriesID := in.SeriesID
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return domain.GoaiSeriesReauditResult{}, errors.New("series id required")
	}
	fullClear, ok := normalizeReauditScope(in.Scope)
	if !ok {
		return domain.GoaiSeriesReauditResult{}, ErrGoaiAuditInvalidScope
	}
	if fullClear {
		if err := s.repo.DeleteReleaseAuditsForSeries(ctx, seriesID); err != nil {
			return domain.GoaiSeriesReauditResult{}, err
		}
	}
	if err := s.repo.SetSeriesNeedsReaudit(ctx, seriesID); err != nil {
		if err == sql.ErrNoRows {
			return domain.GoaiSeriesReauditResult{}, ErrGoaiAuditSeriesAbsent
		}
		return domain.GoaiSeriesReauditResult{}, err
	}
	return domain.GoaiSeriesReauditResult{
		SeriesID:             seriesID,
		ClearedReleaseAudits: fullClear,
	}, nil
}
