package pageviewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientDebugTraceReturnsRecentDOMTrace(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	traceID := "trace-dom-123"
	_, err = client.HTML(context.Background(), s.URL, WithTraceID(traceID))
	require.NoError(t, err)

	trace, ok := client.DebugTrace(traceID)
	require.True(t, ok)
	assert.Equal(t, traceID, trace.TraceID)
	assert.Equal(t, traceModeDOM, trace.Mode)
	assert.Equal(t, http.StatusOK, trace.StatusCode)
	assert.Equal(t, "text/html", trace.ContentType)
	assert.Equal(t, s.URL+"/", trace.FinalURL)
	assert.GreaterOrEqual(t, trace.WorkerID, 1)
	assert.Empty(t, trace.ErrorMessage)
	assert.False(t, trace.BrokenWorker)
	assert.False(t, trace.FinishedAt.Before(trace.StartedAt))

	stats := client.Stats()
	assert.Equal(t, 1, stats.RecentTraces)
	assert.Empty(t, stats.LastError)
}

func TestClientDebugTraceRecordsRawTextErrors(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	traceID := "trace-text-123"
	_, err = client.RawText(context.Background(), s.URL, WithTraceID(traceID))
	require.ErrorIs(t, err, ErrUnsupportedContentType)

	trace, ok := client.DebugTrace(traceID)
	require.True(t, ok)
	assert.Equal(t, traceID, trace.TraceID)
	assert.Equal(t, traceModeText, trace.Mode)
	assert.Equal(t, http.StatusOK, trace.StatusCode)
	assert.Equal(t, "application/pdf", trace.ContentType)
	assert.Equal(t, s.URL+"/", trace.FinalURL)
	assert.Contains(t, trace.ErrorMessage, ErrUnsupportedContentType.Error())
	assert.True(t, trace.BrokenWorker)

	stats := client.Stats()
	assert.Equal(t, trace.ErrorMessage, stats.LastError)
}

func TestClientDebugTracePreservesAttemptsForSameTraceID(t *testing.T) {
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer errorServer.Close()

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer successServer.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	traceID := "trace-retry-123"
	_, err = client.RawText(context.Background(), errorServer.URL, WithTraceID(traceID))
	require.ErrorIs(t, err, ErrUnsupportedContentType)

	_, err = client.HTML(context.Background(), successServer.URL, WithTraceID(traceID))
	require.NoError(t, err)

	trace, ok := client.DebugTrace(traceID)
	require.True(t, ok)
	assert.Equal(t, 2, trace.AttemptCount)
	require.Len(t, trace.Attempts, 2)
	assert.Equal(t, traceModeText, trace.Attempts[0].Mode)
	assert.Contains(t, trace.Attempts[0].ErrorMessage, ErrUnsupportedContentType.Error())
	assert.Equal(t, traceModeDOM, trace.Mode)
	assert.Equal(t, http.StatusOK, trace.StatusCode)
	assert.Empty(t, trace.ErrorMessage)

	stats := client.Stats()
	assert.Empty(t, stats.LastError)
}

func TestClientDebugTraceRecordsDOMErrorResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	traceID := "trace-dom-error-123"
	_, err = client.HTML(context.Background(), s.URL, WithTraceID(traceID))
	require.ErrorIs(t, err, ErrUnsupportedContentType)

	trace, ok := client.DebugTrace(traceID)
	require.True(t, ok)
	assert.Equal(t, traceID, trace.TraceID)
	assert.Equal(t, traceModeDOM, trace.Mode)
	assert.Equal(t, http.StatusTeapot, trace.StatusCode)
	assert.Equal(t, "application/pdf", trace.ContentType)
	assert.Equal(t, s.URL+"/", trace.FinalURL)
	assert.Contains(t, trace.ErrorMessage, ErrUnsupportedContentType.Error())
}

func TestTraceRecorderKeepsLatestStartedAttemptSummary(t *testing.T) {
	recorder := newTraceRecorder(8)
	traceID := "trace-concurrent-123"

	recorder.store(traceID, TraceAttempt{
		URL:        "https://example.com/newer",
		Mode:       traceModeText,
		StartedAt:  time.Unix(2, 0),
		FinishedAt: time.Unix(2, 0),
		sequence:   2,
	})
	recorder.store(traceID, TraceAttempt{
		URL:          "https://example.com/older",
		Mode:         traceModeDOM,
		StartedAt:    time.Unix(1, 0),
		FinishedAt:   time.Unix(3, 0),
		ErrorMessage: ErrNavigationFailed.Error(),
		sequence:     1,
	})

	trace, ok := recorder.get(traceID)
	require.True(t, ok)
	assert.Equal(t, traceModeText, trace.Mode)
	assert.Equal(t, "https://example.com/newer", trace.URL)
	assert.Empty(t, trace.ErrorMessage)
	assert.Equal(t, 2, trace.AttemptCount)
	require.Len(t, trace.Attempts, 2)
	assert.Equal(t, uint64(1), trace.Attempts[0].sequence)
	assert.Equal(t, uint64(2), trace.Attempts[1].sequence)

	count, lastErr := recorder.stats()
	assert.Equal(t, 1, count)
	assert.Empty(t, lastErr)
}
