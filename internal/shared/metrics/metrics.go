package metrics

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	analysisStartedTotal   atomic.Uint64
	analysisCompletedTotal atomic.Uint64
	analysisFailedTotal    atomic.Uint64

	analysisDuration = newHistogram([]float64{100, 250, 500, 1000, 2000, 5000, 10000, 30000, 60000})
)

// IncAnalysisStarted increments the started counter.
func IncAnalysisStarted() {
	analysisStartedTotal.Add(1)
}

// IncAnalysisCompleted increments the completed counter.
func IncAnalysisCompleted() {
	analysisCompletedTotal.Add(1)
}

// IncAnalysisFailed increments the failed counter.
func IncAnalysisFailed() {
	analysisFailedTotal.Add(1)
}

// ObserveAnalysisDurationMs records an analysis duration in milliseconds.
func ObserveAnalysisDurationMs(value float64) {
	if value < 0 {
		value = 0
	}
	analysisDuration.Observe(value)
}

// Handler exposes metrics in Prometheus text format.
func Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4")
		c.String(http.StatusOK, Render())
	}
}

// Render renders metrics in Prometheus text format.
func Render() string {
	var buf bytes.Buffer
	writeCounter(&buf, "analysis_started_total", "Total analyses started", analysisStartedTotal.Load())
	writeCounter(&buf, "analysis_completed_total", "Total analyses completed", analysisCompletedTotal.Load())
	writeCounter(&buf, "analysis_failed_total", "Total analyses failed", analysisFailedTotal.Load())
	writeHistogram(&buf, "analysis_duration_ms", "Analysis duration in milliseconds", analysisDuration.Snapshot())
	return buf.String()
}

type histogram struct {
	mu      sync.Mutex
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

type histogramSnapshot struct {
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

func newHistogram(buckets []float64) *histogram {
	return &histogram{
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
}

func (h *histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	h.sum += value
	for i, bound := range h.buckets {
		if value <= bound {
			h.counts[i]++
		}
	}
}

func (h *histogram) Snapshot() histogramSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := histogramSnapshot{
		buckets: append([]float64(nil), h.buckets...),
		counts:  append([]uint64(nil), h.counts...),
		sum:     h.sum,
		count:   h.count,
	}
	return out
}

func writeCounter(buf *bytes.Buffer, name, help string, value uint64) {
	fmt.Fprintf(buf, "# HELP %s %s\n", name, help)
	fmt.Fprintf(buf, "# TYPE %s counter\n", name)
	fmt.Fprintf(buf, "%s %d\n", name, value)
}

func writeHistogram(buf *bytes.Buffer, name, help string, snap histogramSnapshot) {
	fmt.Fprintf(buf, "# HELP %s %s\n", name, help)
	fmt.Fprintf(buf, "# TYPE %s histogram\n", name)
	var cumulative uint64
	for i, bound := range snap.buckets {
		cumulative += snap.counts[i]
		fmt.Fprintf(buf, "%s_bucket{le=\"%s\"} %d\n", name, formatFloat(bound), cumulative)
	}
	fmt.Fprintf(buf, "%s_bucket{le=\"+Inf\"} %d\n", name, snap.count)
	fmt.Fprintf(buf, "%s_sum %s\n", name, formatFloat(snap.sum))
	fmt.Fprintf(buf, "%s_count %d\n", name, snap.count)
}

func formatFloat(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// NowMillis returns current time in milliseconds, useful for callers without time utilities.
func NowMillis() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Millisecond)
}
