package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsRequiresURLAndMode(t *testing.T) {
	_, err := parseFlags([]string{"--mode", "html"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--url is required")

	_, err = parseFlags([]string{"--url", "https://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--mode is required")
}

func TestParseFlagsRejectsInvalidMode(t *testing.T) {
	_, err := parseFlags([]string{"--url", "https://example.com", "--mode", "pdf"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --mode")
}

func TestParseFlagsParsesCommonOptions(t *testing.T) {
	opts, err := parseFlags([]string{
		"--url", "https://example.com",
		"--mode", "article",
		"--json",
		"--wait-timeout", "15s",
		"--trace-id", "trace-1",
		"--remove-invisible-div",
		"--acquire-timeout", "5s",
		"--proxy", "http://127.0.0.1:8080",
		"--no-headless",
		"--devtools",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", opts.url)
	assert.Equal(t, "article", opts.mode)
	assert.True(t, opts.jsonOutput)
	assert.Equal(t, 15*time.Second, opts.waitTimeout)
	assert.Equal(t, "trace-1", opts.traceID)
	assert.True(t, opts.removeInvisibleDiv)
	assert.Equal(t, 5*time.Second, opts.acquireTimeout)
	assert.Equal(t, "http://127.0.0.1:8080", opts.proxy)
	assert.True(t, opts.noHeadless)
	assert.True(t, opts.devTools)
}
