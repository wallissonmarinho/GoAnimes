package ports

// SynopsisTranslator translates plain synopsis text between language codes (e.g. en → pt).
// Implementations live under internal/adapters/translate; compose multiple with translate.Chain for fallback.
type SynopsisTranslator interface {
	// Name identifies the backend in logs (e.g. gilang-googletranslate).
	Name() string
	Translate(text, source, target string) (string, error)
}
