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

func TestRequestOptionsKeepBeforeRequest(t *testing.T) {
	called := false
	opts := NewRequestOptions(WithBeforeRequest(func(page *rod.Page) error {
		called = true
		return nil
	}))
	require.NotNil(t, opts.BeforeRequest)
	assert.False(t, called)
}
