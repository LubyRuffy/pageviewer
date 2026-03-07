package pageviewer

import (
	"mime"
	"strings"
	"time"
)

type Config struct {
	PoolSize            int
	AcquireTimeout      time.Duration
	UserDataDir         string
	Warmup              int
	Debug               bool
	NoHeadless          bool
	DevTools            bool
	Proxy               string
	IgnoreCertErrors    bool
	ChromePath          string
	UserModeBrowser     bool
	RemoteDebuggingPort int
}

func DefaultConfig() Config {
	return Config{
		PoolSize:       1,
		Warmup:         1,
		AcquireTimeout: 20 * time.Second,
	}
}

func (cfg Config) withDefaults() Config {
	defaults := DefaultConfig()

	if cfg.PoolSize <= 0 {
		cfg.PoolSize = defaults.PoolSize
	}
	if cfg.AcquireTimeout <= 0 {
		cfg.AcquireTimeout = defaults.AcquireTimeout
	}
	if cfg.Warmup <= 0 {
		cfg.Warmup = min(cfg.PoolSize, defaults.Warmup)
	}
	if cfg.Warmup > cfg.PoolSize {
		cfg.Warmup = cfg.PoolSize
	}

	return cfg
}

func (cfg Config) browserOptions() []BrowserOption {
	return []BrowserOption{
		WithDebug(cfg.Debug),
		WithNoHeadless(cfg.NoHeadless),
		WithDevTools(cfg.DevTools),
		WithProxy(cfg.Proxy),
		WithIgnoreCertErrors(cfg.IgnoreCertErrors),
		WithChromePath(cfg.ChromePath),
		WithUserModeBrowser(cfg.UserModeBrowser),
		WithRemoteDebuggingPort(cfg.RemoteDebuggingPort),
		WithUserDataDir(cfg.UserDataDir),
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
