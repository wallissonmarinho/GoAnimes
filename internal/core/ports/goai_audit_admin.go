package ports

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// GoAIAuditAdmin orchestrates admin operations for GoAI audit endpoints.
type GoAIAuditAdmin interface {
	ListSeriesAudits(ctx context.Context, in domain.GoaiAuditListParams) (domain.GoaiSeriesAuditPage, error)
	RequestSeriesReaudit(ctx context.Context, in domain.GoaiSeriesReauditRequest) (domain.GoaiSeriesReauditResult, error)
}
