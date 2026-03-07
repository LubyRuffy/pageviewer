package pageviewer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 1, cfg.PoolSize)
	assert.Equal(t, 20*time.Second, cfg.AcquireTimeout)
}

func TestIsTextContentType(t *testing.T) {
	assert.True(t, isTextContentType("text/html; charset=utf-8"))
	assert.True(t, isTextContentType("application/json"))
	assert.False(t, isTextContentType("application/pdf"))
}
