package mongo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	mongodb "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CatalogRepository struct {
	store *Store
}

func NewCatalogRepository(store *Store) *CatalogRepository {
	return &CatalogRepository{store: store}
}

func (r *CatalogRepository) UpsertSeason(ctx context.Context, anime domain.Anime) error {
	if err := anime.Validate(); err != nil {
		return err
	}
	if anime.ID == "" {
		anime.ID = uuid.NewString()
	}
	anime.UpdatedAt = time.Now().UTC()
	doc := toAnimeDoc(anime)
	filter := bson.M{"tmdb_id": anime.TMDBID, "season_number": anime.SeasonNumber}
	update := bson.M{
		"$set": bson.M{
			"tmdb_id":        doc.TMDBID,
			"season_number":  doc.SeasonNumber,
			"title":          doc.Title,
			"anime_type":     doc.AnimeType,
			"slug":           doc.Slug,
			"aliases":        doc.Aliases,
			"logo_path":      doc.LogoPath,
			"release_info":   doc.ReleaseInfo,
			"year":           doc.Year,
			"status":         doc.Status,
			"runtime":        doc.Runtime,
			"overview":       doc.Overview,
			"genres":         doc.Genres,
			"rating":         doc.Rating,
			"vote_count":     doc.VoteCount,
			"popularity":     doc.Popularity,
			"poster_path":    doc.PosterPath,
			"backdrop_path":  doc.BackdropPath,
			"last_episode_at": doc.LastEpisodeAt,
			"last_episode_no": doc.LastEpisodeNo,
			"next_episode_at": doc.NextEpisodeAt,
			"next_episode_no": doc.NextEpisodeNo,
			"episodes":       doc.Episodes,
			"mapping_status": doc.MappingStatus,
			"updated_at":     doc.UpdatedAt,
		},
		"$setOnInsert": bson.M{"_id": doc.ID},
	}
	_, err := r.store.Animes.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *CatalogRepository) AddEpisodeSource(ctx context.Context, tmdbID, season, episode int, src domain.Source) (bool, error) {
	if tmdbID <= 0 || season <= 0 || episode <= 0 {
		return false, errors.New("invalid episode identity")
	}
	anime, found, err := r.GetByTMDBSeason(ctx, tmdbID, season)
	if err != nil {
		return false, err
	}
	if !found {
		anime = domain.Anime{ID: uuid.NewString(), TMDBID: tmdbID, SeasonNumber: season, Title: ""}
	} else if anime.ID == "" {
		anime.ID = uuid.NewString()
	}
	ep := anime.EnsureEpisode(episode)
	added := ep.AddSource(src)
	anime.UpdatedAt = time.Now().UTC()
	if !found {
		anime.MappingStatus = domain.MappingStatusMapped
	}
	doc := toAnimeDoc(anime)
	filter := bson.M{"tmdb_id": tmdbID, "season_number": season}
	update := bson.M{
		"$set": bson.M{
			"tmdb_id":        doc.TMDBID,
			"season_number":  doc.SeasonNumber,
			"title":          doc.Title,
			"anime_type":     doc.AnimeType,
			"slug":           doc.Slug,
			"aliases":        doc.Aliases,
			"logo_path":      doc.LogoPath,
			"release_info":   doc.ReleaseInfo,
			"year":           doc.Year,
			"status":         doc.Status,
			"runtime":        doc.Runtime,
			"overview":       doc.Overview,
			"genres":         doc.Genres,
			"rating":         doc.Rating,
			"vote_count":     doc.VoteCount,
			"popularity":     doc.Popularity,
			"poster_path":    doc.PosterPath,
			"backdrop_path":  doc.BackdropPath,
			"last_episode_at": doc.LastEpisodeAt,
			"last_episode_no": doc.LastEpisodeNo,
			"next_episode_at": doc.NextEpisodeAt,
			"next_episode_no": doc.NextEpisodeNo,
			"episodes":       doc.Episodes,
			"mapping_status": doc.MappingStatus,
			"updated_at":     doc.UpdatedAt,
		},
		"$setOnInsert": bson.M{"_id": doc.ID},
	}
	_, err = r.store.Animes.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return added, err
}

func (r *CatalogRepository) UpdateEpisodeDetails(ctx context.Context, tmdbID, season, episode int, airDate, title, overview, stillPath string) error {
	if tmdbID <= 0 || season <= 0 || episode <= 0 {
		return errors.New("invalid episode identity")
	}
	filter := bson.M{
		"tmdb_id":         tmdbID,
		"season_number":   season,
		"episodes.number": episode,
	}
	update := bson.M{
		"$set": bson.M{
			"episodes.$.air_date":    airDate,
			"episodes.$.title":      title,
			"episodes.$.overview":   overview,
			"episodes.$.still_path": stillPath,
		},
	}
	_, err := r.store.Animes.UpdateOne(ctx, filter, update)
	return err
}

func (r *CatalogRepository) GetByTMDBSeason(ctx context.Context, tmdbID, season int) (domain.Anime, bool, error) {
	filter := bson.M{"tmdb_id": tmdbID, "season_number": season}
	var doc animeDoc
	err := r.store.Animes.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongodb.ErrNoDocuments) {
			return domain.Anime{}, false, nil
		}
		return domain.Anime{}, false, err
	}
	return fromAnimeDoc(doc), true, nil
}

func (r *CatalogRepository) ListByTMDBID(ctx context.Context, tmdbID int) ([]domain.Anime, error) {
	cur, err := r.store.Animes.Find(ctx, bson.M{"tmdb_id": tmdbID}, options.Find().SetSort(bson.D{
		{Key: "season_number", Value: 1},
	}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Anime{}
	for cur.Next(ctx) {
		var doc animeDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, fromAnimeDoc(doc))
	}
	return out, cur.Err()
}

func (r *CatalogRepository) ListByGenre(ctx context.Context, genre string, limit, skip int) ([]domain.Anime, error) {
	filter := bson.M{"genres": genre}
	return r.list(ctx, filter, limit, skip)
}

func (r *CatalogRepository) ListRecent(ctx context.Context, days, limit, skip int) ([]domain.Anime, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	filter := bson.M{"updated_at": bson.M{"$gte": cutoff}}
	return r.list(ctx, filter, limit, skip)
}

func (r *CatalogRepository) ListAll(ctx context.Context, limit, skip int) ([]domain.Anime, error) {
	return r.list(ctx, bson.M{}, limit, skip)
}

func (r *CatalogRepository) ListGenres(ctx context.Context) ([]string, error) {
	pipeline := mongodb.Pipeline{
		bson.D{bson.E{Key: "$unwind", Value: "$genres"}},
		bson.D{bson.E{Key: "$group", Value: bson.M{"_id": "$genres"}}},
		bson.D{bson.E{Key: "$sort", Value: bson.M{"_id": 1}}},
	}
	cursor, err := r.store.Animes.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	genres := []string{}
	for cursor.Next(ctx) {
		var row struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		if row.ID != "" {
			genres = append(genres, row.ID)
		}
	}
	return genres, cursor.Err()
}

func (r *CatalogRepository) RemoveSourcesByProvider(ctx context.Context, provider string) (int, error) {
	if provider == "" {
		return 0, errors.New("provider cannot be empty")
	}
	result, err := r.store.Animes.UpdateMany(ctx, bson.M{}, bson.M{
		"$pull": bson.M{
			"episodes.$[].sources": bson.M{
				"provider": provider,
			},
		},
	})
	if err != nil {
		return 0, err
	}
	return int(result.ModifiedCount), nil
}

func (r *CatalogRepository) list(ctx context.Context, filter bson.M, limit, skip int) ([]domain.Anime, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	opts := options.Find().SetLimit(int64(limit)).SetSkip(int64(skip)).SetSort(bson.D{bson.E{Key: "updated_at", Value: -1}})
	cur, err := r.store.Animes.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Anime{}
	for cur.Next(ctx) {
		var doc animeDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, fromAnimeDoc(doc))
	}
	return out, cur.Err()
}

func toAnimeDoc(a domain.Anime) animeDoc {
	eps := make([]episodeDoc, 0, len(a.Episodes))
	for _, ep := range a.Episodes {
		sources := make([]sourceDoc, 0, len(ep.Sources))
		for _, src := range ep.Sources {
			sources = append(sources, sourceDoc{Provider: src.Provider, MagnetLink: src.MagnetLink, Quality: src.Quality})
		}
		eps = append(eps, episodeDoc{Number: ep.Number, AirDate: ep.AirDate, Title: ep.Title, Overview: ep.Overview, StillPath: ep.StillPath, Sources: sources, AddedAt: ep.AddedAt})
	}
	return animeDoc{
		ID:            a.ID,
		TMDBID:        a.TMDBID,
		SeasonNumber:  a.SeasonNumber,
		Title:         a.Title,
		AnimeType:     a.AnimeType,
		Slug:          a.Slug,
		Aliases:       a.Aliases,
		LogoPath:      a.LogoPath,
		ReleaseInfo:   a.ReleaseInfo,
		Year:          a.Year,
		Status:        a.Status,
		Runtime:       a.Runtime,
		Overview:      a.Overview,
		Genres:        a.Genres,
		Rating:        a.Rating,
		VoteCount:     a.VoteCount,
		Popularity:    a.Popularity,
		PosterPath:    a.PosterPath,
		BackdropPath:  a.BackdropPath,
		LastEpisodeAt: a.LastEpisodeAt,
		LastEpisodeNo: a.LastEpisodeNo,
		NextEpisodeAt: a.NextEpisodeAt,
		NextEpisodeNo: a.NextEpisodeNo,
		Episodes:      eps,
		MappingStatus: string(a.MappingStatus),
		UpdatedAt:     a.UpdatedAt,
	}
}

func fromAnimeDoc(doc animeDoc) domain.Anime {
	eps := make([]domain.Episode, 0, len(doc.Episodes))
	for _, ep := range doc.Episodes {
		sources := make([]domain.Source, 0, len(ep.Sources))
		for _, src := range ep.Sources {
			sources = append(sources, domain.Source{Provider: src.Provider, MagnetLink: src.MagnetLink, Quality: src.Quality})
		}
		eps = append(eps, domain.Episode{Number: ep.Number, AirDate: ep.AirDate, Title: ep.Title, Overview: ep.Overview, StillPath: ep.StillPath, Sources: sources, AddedAt: ep.AddedAt})
	}
	return domain.Anime{
		ID:            doc.ID,
		TMDBID:        doc.TMDBID,
		SeasonNumber:  doc.SeasonNumber,
		Title:         doc.Title,
		AnimeType:     doc.AnimeType,
		Slug:          doc.Slug,
		Aliases:       doc.Aliases,
		LogoPath:      doc.LogoPath,
		ReleaseInfo:   doc.ReleaseInfo,
		Year:          doc.Year,
		Status:        doc.Status,
		Runtime:       doc.Runtime,
		Overview:      doc.Overview,
		Genres:        doc.Genres,
		Rating:        doc.Rating,
		VoteCount:     doc.VoteCount,
		Popularity:    doc.Popularity,
		PosterPath:    doc.PosterPath,
		BackdropPath:  doc.BackdropPath,
		LastEpisodeAt: doc.LastEpisodeAt,
		LastEpisodeNo: doc.LastEpisodeNo,
		NextEpisodeAt: doc.NextEpisodeAt,
		NextEpisodeNo: doc.NextEpisodeNo,
		Episodes:      eps,
		MappingStatus: domain.MappingStatus(doc.MappingStatus),
		UpdatedAt:     doc.UpdatedAt,
	}
}
