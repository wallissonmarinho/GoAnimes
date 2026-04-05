package rsssync

import "github.com/wallissonmarinho/GoAnimes/internal/core/domain"

func cloneAniListCache(m map[string]domain.AniListSeriesEnrichment) map[string]domain.AniListSeriesEnrichment {
	out := make(map[string]domain.AniListSeriesEnrichment)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func pruneAniListCache(m map[string]domain.AniListSeriesEnrichment, series []domain.CatalogSeries) {
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
