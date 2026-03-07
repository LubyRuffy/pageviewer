package pageviewer

import (
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestOptionsUsesDefaults(t *testing.T) {
	opts := NewRequestOptions()
	assert.Equal(t, DefaultWaitStableTimeout, opts.WaitTimeout)
	assert.Empty(t, opts.TraceID)
}

func TestRequestOptionsOverrideAcquireTimeout(t *testing.T) {
	opts := NewRequestOptions(WithAcquireTimeout(3 * time.Second))
	assert.Equal(t, 3*time.Second, opts.AcquireTimeout)
}

func TestRequestOptionsKeepTraceID(t *testing.T) {
	opts := NewRequestOptions(WithTraceID("trace-123"))
	assert.Equal(t, "trace-123", opts.TraceID)
}

func TestRequestOptionsKeepBeforeRequest(t *testing.T) {
	called := false
	opts := NewRequestOptions(WithBeforeRequest(func(page *rod.Page) error {
		called = true
		return nil
	}))
	require.NotNil(t, opts.BeforeRequest)
	assert.False(t, called)
}

func TestNewVisitOptionsKeepsLegacyWrappers(t *testing.T) {
	expectedBrowser := &Browser{}
	beforeRequest := func(page *rod.Page) error {
		return nil
	}

	opts := NewVisitOptions(
		WithBrowser(expectedBrowser),
		WithWaitTimeout(2*time.Second),
		WithRemoveInvisibleDiv(true),
		WithBeforeRequest(beforeRequest),
		WithAcquireTimeout(3*time.Second),
		WithTraceID("trace-123"),
	)

	require.NotNil(t, opts)
	require.NotNil(t, opts.PageOptions)
	assert.Same(t, expectedBrowser, opts.browser)
	assert.Equal(t, 2*time.Second, opts.PageOptions.waitTimeout)
	assert.True(t, opts.PageOptions.removeInvisibleDiv)
	assert.NotNil(t, opts.PageOptions.beforeRequest)
	assert.Equal(t, "trace-123", opts.traceID)
}

func TestCustomVisitOptionStillWorks(t *testing.T) {
	expectedBrowser := &Browser{}
	var custom VisitOption = func(vo *VisitOptions) {
		vo.PageOptions.waitTimeout = time.Second
		vo.browser = expectedBrowser
	}

	visitOptions := NewVisitOptions(custom)
	require.NotNil(t, visitOptions)
	assert.Equal(t, time.Second, visitOptions.PageOptions.waitTimeout)
	assert.Same(t, expectedBrowser, visitOptions.browser)

	requestOptions := NewRequestOptions(custom)
	assert.Equal(t, time.Second, requestOptions.WaitTimeout)
	assert.Same(t, expectedBrowser, requestOptions.browser)
}
