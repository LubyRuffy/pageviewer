package pageviewer

import "errors"

var (
	ErrClosed                 = errors.New("pageviewer: client closed")
	ErrAcquireTimeout         = errors.New("pageviewer: acquire timeout")
	ErrBrowserUnavailable     = errors.New("pageviewer: browser unavailable")
	ErrNavigationFailed       = errors.New("pageviewer: navigation failed")
	ErrUnsupportedContentType = errors.New("pageviewer: unsupported content type")
	ErrWorkerBroken           = errors.New("pageviewer: worker broken")
)
