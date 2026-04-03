package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetBytesGETRetry_429_thenOK(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := n.Add(1)
		if c < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	g := NewGetter(5*time.Second, "test/1", 1<<20)
	b, err := g.GetBytesGETRetry(context.Background(), srv.URL, 5, 100*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "ok", string(b))
	require.EqualValues(t, 3, n.Load())
}

func TestGetBytesGETRetry_404_noRetry(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	g := NewGetter(5*time.Second, "test/1", 1<<20)
	_, err := g.GetBytesGETRetry(context.Background(), srv.URL, 5, time.Second)
	require.Error(t, err)
	var st *HTTPStatusError
	require.ErrorAs(t, err, &st)
	require.Equal(t, 404, st.StatusCode)
	require.EqualValues(t, 1, n.Load())
}
