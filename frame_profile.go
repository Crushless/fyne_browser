package fynecef

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const frameProfileEnv = "FYNECEF_FRAME_PROFILE"
const frameProfileIntervalEnv = "FYNECEF_FRAME_PROFILE_INTERVAL_MS"

var frameProfilerState = newFrameProfiler()

type frameProfiler struct {
	enabled  bool
	interval time.Duration

	mu         sync.Mutex
	lastReport time.Time
	totals     frameProfileTotals
}

type frameProfileTotals struct {
	frames           int64
	fullFrames       int64
	partialFrames    int64
	partialFallbacks int64
	rects            int64
	copiedBytes      int64

	callback durationTotals
	copy     durationTotals
	queue    durationTotals
	clone    durationTotals
	wait     durationTotals
	apply    durationTotals
}

type durationTotals struct {
	count int64
	total time.Duration
	max   time.Duration
}

func newFrameProfiler() *frameProfiler {
	if !envEnabled(os.Getenv(frameProfileEnv)) {
		return &frameProfiler{}
	}

	interval := 2 * time.Second
	if raw := strings.TrimSpace(os.Getenv(frameProfileIntervalEnv)); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms > 0 {
			interval = time.Duration(ms) * time.Millisecond
		}
	}

	now := time.Now()
	log.Printf("fynecef: frame profiling enabled (interval=%s)", interval)
	return &frameProfiler{
		enabled:    true,
		interval:   interval,
		lastReport: now,
	}
}

func envEnabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (p *frameProfiler) observeFrame(callback, copy time.Duration, full bool, rects, copiedBytes int, fallback bool) {
	if p == nil || !p.enabled {
		return
	}

	now := time.Now()
	p.mu.Lock()
	p.totals.frames++
	if full {
		p.totals.fullFrames++
	} else {
		p.totals.partialFrames++
	}
	if fallback {
		p.totals.partialFallbacks++
	}
	p.totals.rects += int64(rects)
	p.totals.copiedBytes += int64(copiedBytes)
	p.totals.callback.add(callback)
	p.totals.copy.add(copy)
	p.maybeReportLocked(now)
	p.mu.Unlock()
}

func (p *frameProfiler) observeQueue(queue, clone time.Duration) {
	if p == nil || !p.enabled {
		return
	}

	now := time.Now()
	p.mu.Lock()
	p.totals.queue.add(queue)
	if clone > 0 {
		p.totals.clone.add(clone)
	}
	p.maybeReportLocked(now)
	p.mu.Unlock()
}

func (p *frameProfiler) observeApply(wait, apply time.Duration) {
	if p == nil || !p.enabled {
		return
	}

	now := time.Now()
	p.mu.Lock()
	p.totals.wait.add(wait)
	p.totals.apply.add(apply)
	p.maybeReportLocked(now)
	p.mu.Unlock()
}

func (p *frameProfiler) maybeReportLocked(now time.Time) {
	if now.Sub(p.lastReport) < p.interval || p.totals.frames == 0 {
		return
	}

	elapsed := now.Sub(p.lastReport)
	totals := p.totals
	p.totals = frameProfileTotals{}
	p.lastReport = now

	rectsPerFrame := 0.0
	if totals.frames > 0 {
		rectsPerFrame = float64(totals.rects) / float64(totals.frames)
	}

	mbPerSecond := 0.0
	if elapsed > 0 {
		mbPerSecond = float64(totals.copiedBytes) / elapsed.Seconds() / (1024 * 1024)
	}

	log.Printf(
		"fynecef frame profile: frames=%d full=%d partial=%d fallback=%d rects/frame=%.2f copy=%.1fMiB/s callback(avg/max)=%s/%s copy(avg/max)=%s/%s queue(avg/max)=%s/%s clone(avg/max)=%s/%s wait(avg/max)=%s/%s apply(avg/max)=%s/%s",
		totals.frames,
		totals.fullFrames,
		totals.partialFrames,
		totals.partialFallbacks,
		rectsPerFrame,
		mbPerSecond,
		totals.callback.avg(),
		totals.callback.max,
		totals.copy.avg(),
		totals.copy.max,
		totals.queue.avg(),
		totals.queue.max,
		totals.clone.avg(),
		totals.clone.max,
		totals.wait.avg(),
		totals.wait.max,
		totals.apply.avg(),
		totals.apply.max,
	)
}

func (d *durationTotals) add(value time.Duration) {
	if value < 0 {
		return
	}
	d.count++
	d.total += value
	if value > d.max {
		d.max = value
	}
}

func (d durationTotals) avg() time.Duration {
	if d.count == 0 {
		return 0
	}
	return d.total / time.Duration(d.count)
}
