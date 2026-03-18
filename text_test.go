package pageviewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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

	client := newTestClient(t, Config{PoolSize: 1, Warmup: 1})

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

	client := newTestClient(t, Config{PoolSize: 1, Warmup: 1})

	_, err := client.RawText(context.Background(), s.URL)
	assert.ErrorIs(t, err, ErrUnsupportedContentType)
}

func TestClientRawTextBlocksSubresourcesForHTMLDocuments(t *testing.T) {
	var styleRequests atomic.Int32
	var imageRequests atomic.Int32

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/style.css":
			styleRequests.Add(1)
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte("body { color: red; }"))
		case "/image.png":
			imageRequests.Add(1)
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <link rel="stylesheet" href="/style.css">
  </head>
  <body>
    <h1>hello</h1>
    <img src="/image.png" alt="blocked">
  </body>
</html>`))
		}
	}))
	defer s.Close()

	client := newTestClient(t, Config{PoolSize: 1, Warmup: 1})

	resp, err := client.RawText(context.Background(), s.URL)
	require.NoError(t, err)
	assert.Contains(t, resp.Body, "<h1>hello</h1>")
	assert.Zero(t, styleRequests.Load())
	assert.Zero(t, imageRequests.Load())
}

func TestClientRawText_ContextDeadlineCancelsBlockedNavigation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body><p>partial"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		select {
		case requestStarted <- struct{}{}:
		default:
		}

		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer func() {
		err := client.closeResources()
		if err != nil && err != context.Canceled {
			require.NoError(t, err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		_, err := client.RawText(ctx, server.URL)
		resultCh <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("expected raw-text request to start")
	}

	select {
	case err := <-resultCh:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(200 * time.Millisecond):
		err := client.closeResources()
		if err != nil && err != context.Canceled {
			require.NoError(t, err)
		}
		select {
		case <-resultCh:
		case <-time.After(time.Second):
			t.Fatal("RawText remained blocked after forced cleanup")
		}
		t.Fatal("RawText did not return after context deadline")
	}
}
