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
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "html with charset",
			contentType: "text/html; charset=utf-8",
			want:        true,
		},
		{
			name:        "json",
			contentType: "application/json",
			want:        true,
		},
		{
			name:        "application xml",
			contentType: "application/xml",
			want:        true,
		},
		{
			name:        "text xml",
			contentType: "text/xml",
			want:        true,
		},
		{
			name:        "text plain",
			contentType: "text/plain",
			want:        true,
		},
		{
			name:        "fallback malformed text plain",
			contentType: "text/plain; charset=",
			want:        true,
		},
		{
			name:        "pdf",
			contentType: "application/pdf",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTextContentType(tt.contentType))
		})
	}
}
