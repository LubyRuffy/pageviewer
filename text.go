package pageviewer

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-rod/rod/lib/proto"
)

func (c *Client) RawText(ctx context.Context, url string, opts ...RequestOption) (TextResponse, error) {
	if c == nil {
		return TextResponse{}, ErrBrowserUnavailable
	}
	if c.closed.Load() {
		return TextResponse{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return TextResponse{}, err
	}
	if c.browser == nil || c.pool == nil {
		return TextResponse{}, ErrBrowserUnavailable
	}

	ro := NewRequestOptions(opts...)
	worker, release, err := c.pool.acquire(ctx, c.acquireWorkerTimeout(ctx, ro))
	if err != nil {
		return TextResponse{}, err
	}

	state := workerStateReady
	defer func() {
		release(state)
		if state == workerStateBroken {
			c.repairWorker(worker)
		}
	}()

	result, err := c.browser.navigateTextPage(worker.page, url, ro.pageOptions())
	if err != nil {
		state = workerStateBroken
		return TextResponse{}, err
	}
	if !isReusableWorkerPage(worker.page) {
		state = workerStateBroken
	}

	return newTextResponse(result.body, result.response), nil
}

func newTextResponse(body string, document *proto.NetworkResponseReceived) TextResponse {
	if document == nil || document.Response == nil {
		return TextResponse{Body: body}
	}

	return TextResponse{
		Body:        body,
		ContentType: document.Response.MIMEType,
		StatusCode:  document.Response.Status,
		FinalURL:    document.Response.URL,
		Header:      newHTTPHeader(document.Response.Headers),
	}
}

func newHTTPHeader(headers proto.NetworkHeaders) http.Header {
	if len(headers) == 0 {
		return http.Header{}
	}

	result := make(http.Header, len(headers))
	for key, value := range headers {
		result.Add(key, strings.TrimSpace(value.Str()))
	}
	return result
}
