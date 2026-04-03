package domain

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// translateGenreENtoPT maps AniList/MAL English genre labels to Brazilian Portuguese.
// Unknown labels are returned unchanged (e.g. already "Romance").
var translateGenreENtoPT = map[string]string{
	"Action":            "Ação",
	"Adventure":         "Aventura",
	"Comedy":            "Comédia",
	"Drama":             "Drama",
	"Ecchi":             "Ecchi",
	"Fantasy":           "Fantasia",
	"Horror":            "Terror",
	"Mahou Shoujo":      "Mahou shoujo",
	"Mecha":             "Mecha",
	"Music":             "Música",
	"Mystery":           "Mistério",
	"Psychological":     "Psicológico",
	"Romance":           "Romance",
	"Sci-Fi":            "Ficção científica",
	"Slice of Life":     "Slice of life",
	"Sports":            "Esportes",
	"Supernatural":      "Sobrenatural",
	"Thriller":          "Suspense",
	"Superhero":         "Super-herói",
	"Martial Arts":      "Artes marciais",
	"School":            "Escolar",
	"Shounen":           "Shounen",
	"Shoujo":            "Shoujo",
	"Seinen":            "Seinen",
	"Josei":             "Josei",
	"Kids":              "Infantil",
	"Boys Love":         "Boys love",
	"Girls Love":        "Girls love",
	"Gourmet":           "Gastronomia",
	"Harem":             "Harém",
	"Isekai":            "Isekai",
	"Military":          "Militar",
	"Parody":            "Paródia",
	"Police":            "Policial",
	"Samurai":           "Samurai",
	"Space":             "Espaço",
	"Vampire":           "Vampiros",
	"Work Life":         "Vida profissional",
	"Strategy Game":     "Jogo de estratégia",
	"Suspense":          "Suspense",
	"Historical":        "Histórico",
	"Demons":            "Demônios",
	"Game":              "Jogos",
	"Reverse Harem":     "Harém reverso",
	"Award Winning":     "Premiado",
	"Survival":          "Sobrevivência",
	"Time Travel":       "Viagem no tempo",
	"Video Game":        "Videogame",
	"Visual Arts":       "Artes visuais",
}

var translateGenreLowerToPT = func() map[string]string {
	m := make(map[string]string, len(translateGenreENtoPT)*2)
	for en, pt := range translateGenreENtoPT {
		m[strings.ToLower(en)] = pt
	}
	return m
}()

// TranslateAnimeGenresToPTBR returns a copy of genres with common English labels translated to pt-BR.
func TranslateAnimeGenresToPTBR(genres []string) []string {
	if len(genres) == 0 {
		return nil
	}
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if pt, ok := translateGenreENtoPT[g]; ok {
			out = append(out, pt)
			continue
		}
		if pt, ok := translateGenreLowerToPT[strings.ToLower(g)]; ok {
			out = append(out, pt)
			continue
		}
		out = append(out, g)
	}
	return out
}

var reSourceSuffix = regexp.MustCompile(`(?i)\(\s*Source:\s*([^)]+)\)`)

// reSynopsisAttributionTail matches a trailing AniList-style source line (English or already localized).
var reSynopsisAttributionTail = regexp.MustCompile(`(?is)\s*\(\s*(?:Source|Fonte):\s*[^)]+\)\s*$`)

// synopsisLikelyEnglishRE matches common English words in AniList-style blurbs (default language is English).
var synopsisLikelyEnglishRE = regexp.MustCompile(`(?i)\b(the|and|with|that|from|their|will|this|have|been|was|were|his|her|for|not|you|all|can|out|just|into|about|to|of|in|is|it|as|at|be|he|or|on|an|we|they|she|them|then|than|who|years|year|one|two|earth|life|back|time|story|world|after|when|where|what|young|old|new|first|last|giant|robot|battle|must|war|evil|save|return|human|boy|girl|home|school|friend|power|space|planet|city|people|again|still|even|only|such|through|between|against|while|during|before|because|another|something|everything|nothing|himself|herself|themselves)\b`)

// synopsisLikelyPortugueseRE catches obvious pt-BR so we do not send already-local blurbs to the translator.
var synopsisLikelyPortugueseRE = regexp.MustCompile(`(?i)\b(não|nao|você|voce|também|tambem|está|estão|estamos|será|serão|muito|pelo|pelas|história|historia|primeira|segunda|temporada|anos|cidades|título|titulo|sinopse|episódio|episodio)\b`)

// SplitSynopsisBodyAndAttribution separates the main blurb from a trailing "(Source: …)" or "(Fonte: …)" line.
func SplitSynopsisBodyAndAttribution(s string) (body string, attribution string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	loc := reSynopsisAttributionTail.FindStringIndex(s)
	if loc == nil {
		return s, ""
	}
	body = strings.TrimSpace(s[:loc[0]])
	attr := strings.TrimSpace(s[loc[0]:loc[1]])
	return body, attr
}

// JoinSynopsisBodyAndAttribution joins body and attribution with a single space.
func JoinSynopsisBodyAndAttribution(body, attribution string) string {
	body = strings.TrimSpace(body)
	attribution = strings.TrimSpace(attribution)
	if body == "" {
		return attribution
	}
	if attribution == "" {
		return body
	}
	return body + " " + attribution
}

func synopsisBodyMostlyLatinLetters(body string) bool {
	var letters, latin int
	for _, r := range body {
		if unicode.IsLetter(r) {
			letters++
			if r <= unicode.MaxASCII && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				latin++
			}
		}
	}
	return letters >= 12 && latin*4 >= letters*3
}

// SynopsisBodyLooksEnglish is a cheap heuristic to avoid re-translating text that is already pt-BR.
// Long Latin blurbs without typical English tokens (e.g. proper-noun-heavy AniList copy) still qualify
// when they do not look Portuguese — fixes cases like Snowball Earth staying in English.
func SynopsisBodyLooksEnglish(body string) bool {
	body = strings.TrimSpace(body)
	if len(body) < 20 {
		return false
	}
	if synopsisLikelyEnglishRE.MatchString(body) {
		return true
	}
	// Medium/long Latin blurbs that never hit the keyword list (proper nouns, odd wording) still
	// need translation; 80 chars was too strict and skipped many real AniList EN blurbs (~50–79).
	if len(body) >= 50 && !synopsisLikelyPortugueseRE.MatchString(body) && synopsisBodyMostlyLatinLetters(body) {
		return true
	}
	return false
}

// LocalizeAniListDescriptionPTBR keeps the AniList English blurb but normalizes the attribution line to Portuguese.
// AniList’s public GraphQL API does not return descriptions in pt-BR; GoAnimes translates via gilang in-process.
func LocalizeAniListDescriptionPTBR(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	return reSourceSuffix.ReplaceAllStringFunc(s, func(m string) string {
		sub := reSourceSuffix.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		src := strings.TrimSpace(sub[1])
		return "(Fonte: " + src + ")"
	})
}

// StremioGenreFilterOptions returns the full sorted pt-BR palette (AniList→pt mapping). The Stremio manifest
// uses UniqueGenreLabelsFromCatalogSeries instead so the genre filter only lists genres present in the catalog.
func StremioGenreFilterOptions() []string {
	seen := make(map[string]struct{}, len(translateGenreENtoPT))
	for _, pt := range translateGenreENtoPT {
		if strings.TrimSpace(pt) == "" {
			continue
		}
		seen[pt] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}
