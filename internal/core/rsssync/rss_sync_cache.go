package rsssync

import "github.com/wallissonmarinho/GoAnimes/internal/core/domain"

func cloneSeriesEnrichmentCache(m map[string]domain.SeriesEnrichment) map[string]domain.SeriesEnrichment {
	out := make(map[string]domain.SeriesEnrichment)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func pruneSeriesEnrichmentCache(m map[string]domain.SeriesEnrichment, series []domain.CatalogSeries) {
	want := make(map[string]struct{}, len(series))
	for _, s := range series {
		want[s.ID] = struct{}{}
	}
	for id := range m {
		if _, ok := want[id]; !ok {
			delete(m, id)
		}
	}
}
