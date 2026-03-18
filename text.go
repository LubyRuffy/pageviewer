package pageviewer

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

func (c *Client) RawText(ctx context.Context, url string, opts ...RequestOption) (resp TextResponse, err error) {
	ro := NewRequestOptions(opts...)
	trace := c.beginTrace(ro.TraceID, traceModeText, url)
	defer func() {
		trace.finish(err)
	}()

	if err = c.beginTrackedOperation(); err != nil {
		return TextResponse{}, err
	}
	defer c.inflight.Done()

	if err = ctx.Err(); err != nil {
		return TextResponse{}, err
	}
	if c.browser == nil || c.pool == nil {
		return TextResponse{}, ErrBrowserUnavailable
	}

	acquireCtx, stopAcquire := c.acquireContext(ctx)
	defer stopAcquire()

	acquireStart := time.Now()
	worker, release, err := c.pool.acquire(acquireCtx, c.acquireWorkerTimeout(ctx, ro))
	trace.setAcquireWait(time.Since(acquireStart))
	if err != nil {
		if errors.Is(err, context.Canceled) && c.closed.Load() && ctx.Err() == nil {
			err = ErrClosed
		}
		return TextResponse{}, err
	}
	trace.setWorkerID(worker.id)

	state := workerStateReady
	defer func() {
		release(state)
		if state == workerStateBroken {
			trace.markBrokenWorker()
			if c.repairWorkers {
				c.scheduleRepair(worker)
				return
			}
			c.retireWorker(worker)
		}
	}()

	po := ro.pageOptions()
	po.blockSubresources = true

	result, err := c.browser.navigateTextPage(worker.page, url, po)
	trace.setResponse(result.response)
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
