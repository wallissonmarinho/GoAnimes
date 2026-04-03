package translate

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
)

func TestTimeoutFromGetter(t *testing.T) {
	require.Equal(t, 45*time.Second, timeoutFromGetter(nil, 45*time.Second))
	g := httpclient.NewGetter(30*time.Second, "x", 2<<20)
	require.Equal(t, 30*time.Second, timeoutFromGetter(g, 45*time.Second))
	g2 := &httpclient.Getter{Client: &http.Client{Timeout: 12 * time.Second}}
	require.Equal(t, 12*time.Second, timeoutFromGetter(g2, 45*time.Second))
}
