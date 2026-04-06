package domain

import (
	"sort"
)

// MergeSnapshotSeriesBySharedMalID collapses multiple Stremio series buckets that share the same
// MyAnimeList id (AniList idMal / enrichment MalID) into one canonical series id.
//
// Items are remapped; AniListBySeries rows are merged with MergeAniListEnrichment in sorted key order
// for determinism. Series with MalID==0 or a unique MalID are left unchanged.
//
// Canonical id is the series id with the most catalog items; ties break by lexicographically smallest id.
func MergeSnapshotSeriesBySharedMalID(snap *CatalogSnapshot) {
	if snap == nil || len(snap.Items) == 0 || len(snap.AniListBySeries) == 0 {
		return
	}

	seriesIDs := make(map[string]struct{})
	for i := range snap.Items {
		if id := snap.Items[i].SeriesID; id != "" {
			seriesIDs[id] = struct{}{}
		}
	}

	// malID -> distinct series IDs that have items and MalID set
	malToSeries := make(map[int]map[string]struct{})
	for id := range seriesIDs {
		en := snap.AniListBySeries[id]
		if en.MalID <= 0 {
			continue
		}
		m := malToSeries[en.MalID]
		if m == nil {
			m = make(map[string]struct{})
			malToSeries[en.MalID] = m
		}
		m[id] = struct{}{}
	}

	remap := make(map[string]string) // duplicate -> canonical
	for _, idSet := range malToSeries {
		if len(idSet) < 2 {
			continue
		}
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		canon := pickCanonicalSeriesIDForMerge(ids, snap.Items)
		for _, id := range ids {
			if id != canon {
				remap[id] = canon
			}
		}
	}
	if len(remap) == 0 {
		return
	}

	for i := range snap.Items {
		if to, ok := remap[snap.Items[i].SeriesID]; ok {
			snap.Items[i].SeriesID = to
		}
	}

	oldKeys := make([]string, 0, len(snap.AniListBySeries))
	for k := range snap.AniListBySeries {
		oldKeys = append(oldKeys, k)
	}
	sort.Strings(oldKeys)

	merged := make(map[string]AniListSeriesEnrichment, len(snap.AniListBySeries))
	for _, oldID := range oldKeys {
		en := snap.AniListBySeries[oldID]
		canon := oldID
		if c, ok := remap[oldID]; ok {
			canon = c
		}
		merged[canon] = MergeAniListEnrichment(merged[canon], en)
	}
	snap.AniListBySeries = merged

	// Rebuild series rows from items only — do not call AssignSeriesFields here or it would
	// recompute SeriesID from RSS titles and undo the MalID merge.
	snap.Series = BuildSeriesList(snap.Items)
	SortCatalogItemsInPlace(snap.Items)
	snap.ItemCount = len(snap.Items)
}

func pickCanonicalSeriesIDForMerge(ids []string, items []CatalogItem) string {
	if len(ids) == 1 {
		return ids[0]
	}
	sort.Strings(ids)
	best := ids[0]
	bestN := countItemsWithSeriesID(items, best)
	for _, id := range ids[1:] {
		n := countItemsWithSeriesID(items, id)
		if n > bestN || (n == bestN && id < best) {
			best = id
			bestN = n
		}
	}
	return best
}

func countItemsWithSeriesID(items []CatalogItem, seriesID string) int {
	n := 0
	for i := range items {
		if items[i].SeriesID == seriesID {
			n++
		}
	}
	return n
}
