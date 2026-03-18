package pageviewer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestClientReusesSharedBrowser(t *testing.T) {
	first := newTestClient(t, Config{PoolSize: 1, Warmup: 1})
	second := newTestClient(t, Config{PoolSize: 1, Warmup: 1})

	require.NotNil(t, first.browser)
	require.NotNil(t, second.browser)
	assert.False(t, first.ownsBrowser)
	assert.False(t, second.ownsBrowser)
	assert.Same(t, first.browser.Browser, second.browser.Browser)
}

func TestSharedTestBrowserCloseDoesNotCloseUnderlyingBrowser(t *testing.T) {
	first := sharedTestBrowser(t)
	page, err := first.GetPage()
	require.NoError(t, err)
	require.NoError(t, page.Close())

	require.NoError(t, first.Close())

	second := sharedTestBrowser(t)
	page, err = second.GetPage()
	require.NoError(t, err)
	require.NoError(t, page.Close())
}
