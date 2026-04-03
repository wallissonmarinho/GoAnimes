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
