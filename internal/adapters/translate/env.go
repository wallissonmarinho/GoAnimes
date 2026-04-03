package translate

import (
	"os"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

func envTruthy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

// synopsisTranslateEnabled uses existing flags only (no new env names): either legacy Google toggle enables gilang.
func synopsisTranslateEnabled() bool {
	return envTruthy("GOANIMES_GOOGLE_GTX_TRANSLATE") || envTruthy("GOANIMES_GOOGLE_CLIENTS5_TRANSLATE")
}

// FromEnv returns a SynopsisTranslator when GOANIMES_GOOGLE_GTX_TRANSLATE or GOANIMES_GOOGLE_CLIENTS5_TRANSLATE is set.
// Backend: https://github.com/gilang-as/translator (gopkg.gilang.dev/translator/v2/googletranslate). Both flags behave the same.
func FromEnv(getter *httpclient.Getter) ports.SynopsisTranslator {
	if getter == nil || !synopsisTranslateEnabled() {
		return nil
	}
	timeout := timeoutFromGetter(getter, 45*time.Second)
	return NewGilangGoogle(timeout)
}
