package pageviewer

import (
	"mime"
	"strings"
	"time"
)

type Config struct {
	PoolSize       int
	AcquireTimeout time.Duration
	UserDataDir    string
	Warmup         int
}

func DefaultConfig() Config {
	return Config{
		PoolSize:       1,
		AcquireTimeout: 20 * time.Second,
	}
}

func isTextContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(contentType))
	}

	switch mediaType {
	case "application/json", "application/xml", "text/xml":
		return true
	}

	return strings.HasPrefix(mediaType, "text/")
}
