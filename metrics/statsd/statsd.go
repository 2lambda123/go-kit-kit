// Package statsd implements a statsd backend for package metrics.
//
// The current implementation ignores fields. In the future, it would be good
// to have an implementation that accepted a set of predeclared field names at
// construction time, and used field values to produce delimiter-separated
// bucket (key) names. That is,
//
//    c := NewFieldedCounter(..., "path", "status")
//    c.Add(1) // "myprefix.unknown.unknown:1|c\n"
//    c2 := c.With("path", "foo").With("status": "200")
//    c2.Add(1) // "myprefix.foo.status:1|c\n"
//
package statsd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/peterbourgon/gokit/metrics"
)

// statsd metrics take considerable influence from
// https://github.com/streadway/handy package statsd.

const maxBufferSize = 1400 // bytes

type statsdCounter chan string

// NewCounter returns a Counter that emits observations in the statsd protocol
// to the passed writer. Observations are buffered for the reporting interval
// or until the buffer exceeds a max packet size, whichever comes first.
// Fields are ignored.
//
// TODO: support for sampling.
func NewCounter(w io.Writer, key string, interval time.Duration) metrics.Counter {
	c := make(chan string)
	go fwd(w, key, interval, c)
	return statsdCounter(c)
}

func (c statsdCounter) With(metrics.Field) metrics.Counter { return c }

func (c statsdCounter) Add(delta uint64) { c <- fmt.Sprintf("%d|c", delta) }

type statsdGauge chan string

// NewGauge returns a Gauge that emits values in the statsd protocol to the
// passed writer. Values are buffered for the reporting interval or until the
// buffer exceeds a max packet size, whichever comes first. Fields are
// ignored.
//
// TODO: support for sampling.
func NewGauge(w io.Writer, key string, interval time.Duration) metrics.Gauge {
	g := make(chan string)
	go fwd(w, key, interval, g)
	return statsdGauge(g)
}

func (g statsdGauge) With(metrics.Field) metrics.Gauge { return g }

func (g statsdGauge) Add(delta int64) {
	// https://github.com/etsy/statsd/blob/master/docs/metric_types.md#gauges
	sign := "+"
	if delta < 0 {
		sign, delta = "-", -delta
	}
	g <- fmt.Sprintf("%s%d|g", sign, delta)
}

func (g statsdGauge) Set(value int64) {
	g <- fmt.Sprintf("%d|g", value)
}

type statsdHistogram chan string

// NewHistogram returns a Histogram that emits observations in the statsd
// protocol to the passed writer. Observations are buffered for the reporting
// interval or until the buffer exceeds a max packet size, whichever comes
// first. Fields are ignored.
//
// NewHistogram is mapped to a statsd Timing, so observations should represent
// milliseconds. If you observe in units of nanoseconds, you can make the
// translation with a ScaledHistogram:
//
//    NewScaledHistogram(statsdHistogram, time.Millisecond)
//
// You can also enforce the constraint in a typesafe way with a millisecond
// TimeHistogram:
//
//    NewTimeHistogram(statsdHistogram, time.Millisecond)
//
// TODO: support for sampling.
func NewHistogram(w io.Writer, key string, interval time.Duration) metrics.Histogram {
	h := make(chan string)
	go fwd(w, key, interval, h)
	return statsdHistogram(h)
}

func (h statsdHistogram) With(metrics.Field) metrics.Histogram { return h }

func (h statsdHistogram) Observe(value int64) {
	h <- fmt.Sprintf("%d|ms", value)
}

var tick = time.Tick

func fwd(w io.Writer, key string, interval time.Duration, c chan string) {
	buf := &bytes.Buffer{}
	tick := tick(interval)
	for {
		select {
		case s := <-c:
			fmt.Fprintf(buf, "%s:%s\n", key, s)
			if buf.Len() > maxBufferSize {
				flush(w, buf)
			}

		case <-tick:
			flush(w, buf)
		}
	}
}

func flush(w io.Writer, buf *bytes.Buffer) {
	if buf.Len() <= 0 {
		return
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("error: could not write to statsd: %v", err)
	}
	buf.Reset()
}
