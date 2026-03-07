package pageviewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientRawTextReturnsJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.RawText(context.Background(), s.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, "application/json", resp.ContentType)
	assert.Equal(t, s.URL+"/", resp.FinalURL)
	assert.Equal(t, "ok", resp.Header.Get("X-Test"))
	assert.JSONEq(t, `{"ok":true}`, resp.Body)
}

func TestClientRawTextRejectsPDF(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	_, err = client.RawText(context.Background(), s.URL)
	assert.ErrorIs(t, err, ErrUnsupportedContentType)
}
