package pageviewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientHTMLUsesSharedBrowser(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	html, err := client.HTML(context.Background(), s.URL)
	require.NoError(t, err)
	assert.Contains(t, html, `id="app"`)
}

func TestVisitWithBrowserKeepsBrowserReusable(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	browser, err := NewBrowser()
	require.NoError(t, err)
	defer browser.Close()

	err = Visit(s.URL, func(page *rod.Page) error {
		_, err := page.HTML()
		return err
	}, WithBrowser(browser))
	require.NoError(t, err)

	page, err := browser.GetPage()
	require.NoError(t, err)
	require.NoError(t, page.Close())

	err = Visit(s.URL, func(page *rod.Page) error {
		_, err := page.HTML()
		return err
	}, WithBrowser(browser))
	require.NoError(t, err)
}

func TestClientVisitRepairsClosedWorker(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)
	defer client.Close()

	err = client.Visit(context.Background(), s.URL, func(page *rod.Page) error {
		return page.Close()
	})
	require.NoError(t, err)

	html, err := client.HTML(context.Background(), s.URL)
	require.NoError(t, err)
	assert.Contains(t, html, `id="app"`)
}

func TestCloseWaitsForInFlightVisit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)

	started := make(chan struct{})
	releaseVisit := make(chan struct{})
	visitDone := make(chan error, 1)
	closeDone := make(chan error, 1)

	go func() {
		visitDone <- client.Visit(context.Background(), s.URL, func(page *rod.Page) error {
			close(started)
			<-releaseVisit
			return nil
		})
	}()

	<-started

	go func() {
		closeDone <- client.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("close returned before in-flight request completed: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(releaseVisit)

	require.NoError(t, <-visitDone)
	require.NoError(t, <-closeDone)
}

func TestCloseUnblocksWaitingAcquireWithErrClosed(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)

	started := make(chan struct{})
	releaseVisit := make(chan struct{})
	firstDone := make(chan error, 1)
	waitingDone := make(chan error, 1)
	closeDone := make(chan error, 1)

	go func() {
		firstDone <- client.Visit(context.Background(), s.URL, func(page *rod.Page) error {
			close(started)
			<-releaseVisit
			return nil
		})
	}()

	<-started

	go func() {
		_, err := client.HTML(context.Background(), s.URL)
		waitingDone <- err
	}()

	time.Sleep(100 * time.Millisecond)

	go func() {
		closeDone <- client.Close()
	}()

	require.ErrorIs(t, <-waitingDone, ErrClosed)

	select {
	case err := <-closeDone:
		t.Fatalf("close returned before active request completed: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(releaseVisit)

	require.NoError(t, <-firstDone)
	require.NoError(t, <-closeDone)
}

func TestClosePreventsQueuedRequestFromRunning(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div id="app">ok</div></body></html>`))
	}))
	defer s.Close()

	client, err := Start(context.Background(), Config{PoolSize: 1, Warmup: 1})
	require.NoError(t, err)

	started := make(chan struct{})
	releaseVisit := make(chan struct{})
	firstDone := make(chan error, 1)
	waitingStarted := make(chan struct{}, 1)
	waitingDone := make(chan error, 1)
	closeDone := make(chan error, 1)

	go func() {
		firstDone <- client.Visit(context.Background(), s.URL, func(page *rod.Page) error {
			close(started)
			<-releaseVisit
			return nil
		})
	}()

	<-started

	go func() {
		waitingDone <- client.Visit(context.Background(), s.URL, func(page *rod.Page) error {
			waitingStarted <- struct{}{}
			return nil
		})
	}()

	time.Sleep(100 * time.Millisecond)

	go func() {
		closeDone <- client.Close()
	}()

	require.ErrorIs(t, <-waitingDone, ErrClosed)

	select {
	case <-waitingStarted:
		t.Fatal("queued request ran after close")
	default:
	}

	close(releaseVisit)

	require.NoError(t, <-firstDone)
	require.NoError(t, <-closeDone)
}
