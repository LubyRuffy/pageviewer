package pageviewer

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

const (
	defaultTraceCapacity = 128
	traceModeDOM         = "dom"
	traceModeText        = "text"
)

var (
	traceSequence        atomic.Uint64
	traceAttemptSequence atomic.Uint64
)

type TraceAttempt struct {
	URL          string
	Mode         string
	WorkerID     int
	StartedAt    time.Time
	FinishedAt   time.Time
	AcquireWait  time.Duration
	StatusCode   int
	ContentType  string
	FinalURL     string
	ErrorMessage string
	BrokenWorker bool
	sequence     uint64
}

type Trace struct {
	TraceID string
	TraceAttempt
	AttemptCount int
	Attempts     []TraceAttempt
	latestSeq    uint64
}

type traceRecorder struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]Trace
	order    []string
	lastErr  string
	lastSeq  uint64
}

type traceSession struct {
	recorder *traceRecorder
	traceID  string
	attempt  TraceAttempt
}

func newTraceRecorder(capacity int) *traceRecorder {
	if capacity <= 0 {
		capacity = defaultTraceCapacity
	}

	return &traceRecorder{
		capacity: capacity,
		items:    make(map[string]Trace, capacity),
		order:    make([]string, 0, capacity),
	}
}

func (c *Client) beginTrace(traceID, mode, url string) traceSession {
	if c == nil || c.traces == nil {
		return traceSession{}
	}
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", traceSequence.Add(1))
	}

	return traceSession{
		recorder: c.traces,
		traceID:  traceID,
		attempt: TraceAttempt{
			URL:       url,
			Mode:      mode,
			StartedAt: time.Now(),
			sequence:  traceAttemptSequence.Add(1),
		},
	}
}

func (s *traceSession) setAcquireWait(wait time.Duration) {
	if s == nil {
		return
	}
	s.attempt.AcquireWait = wait
}

func (s *traceSession) setWorkerID(workerID int) {
	if s == nil {
		return
	}
	s.attempt.WorkerID = workerID
}

func (s *traceSession) setResponse(response *proto.NetworkResponseReceived) {
	if s == nil || response == nil || response.Response == nil {
		return
	}
	s.attempt.StatusCode = response.Response.Status
	s.attempt.ContentType = response.Response.MIMEType
	s.attempt.FinalURL = response.Response.URL
}

func (s *traceSession) markBrokenWorker() {
	if s == nil {
		return
	}
	s.attempt.BrokenWorker = true
}

func (s *traceSession) finish(err error) {
	if s == nil || s.recorder == nil || s.traceID == "" {
		return
	}

	s.attempt.FinishedAt = time.Now()
	if err != nil {
		s.attempt.ErrorMessage = err.Error()
	}

	s.recorder.store(s.traceID, s.attempt)
}

func (r *traceRecorder) store(traceID string, attempt TraceAttempt) {
	if r == nil || traceID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	trace, exists := r.items[traceID]
	if !exists {
		trace = Trace{
			TraceID:  traceID,
			Attempts: make([]TraceAttempt, 0, 1),
		}
		r.order = append(r.order, traceID)
	} else {
		r.promote(traceID)
	}

	trace.Attempts = append(trace.Attempts, attempt)
	sort.SliceStable(trace.Attempts, func(i, j int) bool {
		return trace.Attempts[i].sequence < trace.Attempts[j].sequence
	})
	if attempt.sequence >= trace.latestSeq {
		trace.TraceAttempt = attempt
		trace.latestSeq = attempt.sequence
	}
	trace.AttemptCount = len(trace.Attempts)
	r.items[traceID] = trace
	if attempt.sequence >= r.lastSeq {
		r.lastSeq = attempt.sequence
		r.lastErr = attempt.ErrorMessage
	}

	for len(r.order) > r.capacity {
		oldest := r.order[0]
		r.order = r.order[1:]
		delete(r.items, oldest)
	}
}

func (r *traceRecorder) promote(traceID string) {
	for i, id := range r.order {
		if id != traceID {
			continue
		}
		copy(r.order[i:], r.order[i+1:])
		r.order[len(r.order)-1] = traceID
		return
	}
}

func (r *traceRecorder) get(traceID string) (Trace, bool) {
	if r == nil || traceID == "" {
		return Trace{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	trace, ok := r.items[traceID]
	if ok {
		trace.Attempts = append([]TraceAttempt(nil), trace.Attempts...)
	}
	return trace, ok
}

func (r *traceRecorder) stats() (int, string) {
	if r == nil {
		return 0, ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.items), r.lastErr
}
