package services

import (
	"log/slog"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// TranslateSynopsisToPT runs ports.SynopsisTranslator on the synopsis body (en→pt) when tr is non-nil.
// The trailing "(Source: …)" / "(Fonte: …)" line is preserved and not sent to the translator.
func TranslateSynopsisToPT(tr ports.SynopsisTranslator, log *slog.Logger, description string) string {
	// Split guards so the debugger shows whether the skip is empty text vs nil translator.
	if strings.TrimSpace(description) == "" {
		return description
	}
	if tr == nil {
		return description
	}
	localized := domain.LocalizeAniListDescriptionPTBR(description)
	body, attr := domain.SplitSynopsisBodyAndAttribution(localized)
	if body == "" {
		return localized
	}
	if !domain.SynopsisBodyLooksEnglish(body) {
		return localized
	}
	body = domain.PrepareEnglishSynopsisBodyForPTTranslate(body)
	out, err := tr.Translate(body, "en", "pt")
	if err != nil {
		if log != nil {
			log.Warn("synopsis translate skipped", slog.String("translator", tr.Name()), slog.Any("err", err))
		}
		return description
	}
	out = domain.FixPortugueseSynopsisTranslationGlitches(strings.TrimSpace(out))
	return domain.JoinSynopsisBodyAndAttribution(out, attr)
}

// TranslateEpisodeTitleToPT translates one episode title to pt-BR (Google auto-detect source) when it does not
// already look Portuguese. Used after all metadata sources merged episode titles.
func TranslateEpisodeTitleToPT(tr ports.SynopsisTranslator, log *slog.Logger, title string) string {
	title = strings.TrimSpace(title)
	if title == "" || tr == nil {
		return title
	}
	if !domain.EpisodeTitleWorthTranslating(title) {
		return title
	}
	body := domain.PrepareEnglishSynopsisBodyForPTTranslate(title)
	out, err := tr.Translate(body, "auto", "pt")
	if err != nil {
		if log != nil {
			log.Debug("episode title translate skipped", slog.String("title", title), slog.Any("err", err))
		}
		return title
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return title
	}
	out = domain.FixPortugueseSynopsisTranslationGlitches(out)
	return out
}
