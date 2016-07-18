package log_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestSwapLogger(t *testing.T) {
	t.Parallel()
	var logger log.SwapLogger

	// Zero value does not panic or error.
	err := logger.Log("k", "v")
	if got, want := err, error(nil); got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	buf := &bytes.Buffer{}
	json := log.NewJSONLogger(buf)
	logger.Swap(json)

	if err := logger.Log("k", "v"); err != nil {
		t.Error(err)
	}
	if got, want := buf.String(), `{"k":"v"}`+"\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	buf.Reset()
	prefix := log.NewLogfmtLogger(buf)
	logger.Swap(prefix)

	if err := logger.Log("k", "v"); err != nil {
		t.Error(err)
	}
	if got, want := buf.String(), "k=v\n"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	buf.Reset()
	logger.Swap(nil)

	if err := logger.Log("k", "v"); err != nil {
		t.Error(err)
	}
	if got, want := buf.String(), ""; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSwapLoggerConcurrency(t *testing.T) {
	t.Parallel()
	testConcurrency(t, &log.SwapLogger{}, 10000)
}

func TestSyncWriterConcurrency(t *testing.T) {
	var w io.Writer
	w = &bytes.Buffer{}
	w = log.NewSyncWriter(w)
	testConcurrency(t, log.NewLogfmtLogger(w), 10000)
}

func TestSyncLoggerConcurrency(t *testing.T) {
	var w io.Writer
	w = &bytes.Buffer{}
	logger := log.NewLogfmtLogger(w)
	logger = log.NewSyncLogger(logger)
	testConcurrency(t, logger, 10000)
}

func TestAsyncLoggerConcurrency(t *testing.T) {
	for _, size := range []int{1, 100, 1000, 10000} {
		var w io.Writer
		w = &bytes.Buffer{}
		logger := log.NewLogfmtLogger(w)
		logger = log.NewAsyncLogger(logger, size)
		testConcurrency(t, logger, 10000)
	}
}

func TestAsyncLoggerLogs(t *testing.T) {
	t.Parallel()
	output := make(chan []interface{})
	logger := log.LoggerFunc(func(keyvals ...interface{}) error {
		output <- keyvals
		return nil
	})

	const size = 4
	const logcnt = size * 20
	al := log.NewAsyncLogger(logger, size)

	go func() {
		for i := 0; i < logcnt; i++ {
			al.Log("key", i)
		}
	}()

	for i := 0; i < logcnt; i++ {
		e := <-output
		if got, want := e[1], i; got != want {
			t.Errorf("log event mismatch, got %v, want %v", got, want)
		}
	}
}

func TestNonblockingLoggerConcurrency(t *testing.T) {
	var w io.Writer
	w = &bytes.Buffer{}
	logger := log.NewLogfmtLogger(w)
	al := log.NewNonblockingLogger(logger, 10000)
	testConcurrency(t, al, 10000)
	al.Stop()
	<-al.Stopped()
}

func TestNonblockingLoggerLogs(t *testing.T) {
	t.Parallel()
	output := [][]interface{}{}
	logger := log.LoggerFunc(func(keyvals ...interface{}) error {
		output = append(output, keyvals)
		return nil
	})

	const logcnt = 10
	al := log.NewNonblockingLogger(logger, logcnt)

	for i := 0; i < logcnt; i++ {
		al.Log("key", i)
	}

	al.Stop()
	al.Stop() // stop is idempotent
	<-al.Stopping()

	if got, want := al.Log("key", "late"), log.ErrNonblockingLoggerStopping; got != want {
		t.Errorf(`logger err: got "%v", want "%v"`, got, want)
	}

	<-al.Stopped()
	al.Stop() // stop is idempotent

	if got, want := len(output), logcnt; got != want {
		t.Errorf("logged events: got %v, want %v", got, want)
	}

	for i, e := range output {
		if got, want := e[1], i; got != want {
			t.Errorf("log event mismatch, got %v, want %v", got, want)
		}
	}
}

func TestNonblockingLoggerOverflow(t *testing.T) {
	t.Parallel()
	var (
		output     = make(chan []interface{}, 10)
		loggerdone = make(chan struct{})
	)

	logger := log.LoggerFunc(func(keyvals ...interface{}) error {
		output <- keyvals
		<-loggerdone // block here to stall the NonblockingLogger.run loop
		return nil
	})

	al := log.NewNonblockingLogger(logger, 1)

	if got, want := al.Log("k", 1), error(nil); got != want {
		t.Errorf(`first log err: got "%v", want "%v"`, got, want)
	}

	<-output
	// Now we know the NonblockingLogger.run loop has consumed the first log event
	// and will be stalled until loggerdone is closed.

	// This log event fills the buffer without error.
	if got, want := al.Log("k", 2), error(nil); got != want {
		t.Errorf(`second log err: got "%v", want "%v"`, got, want)
	}

	// Now we test for buffer overflow.
	if got, want := al.Log("k", 3), log.ErrNonblockingLoggerOverflow; got != want {
		t.Errorf(`third log err: got "%v", want "%v"`, got, want)
	}

	al.Stop()
	<-al.Stopping()

	if got, want := al.Log("key", "late"), log.ErrNonblockingLoggerStopping; got != want {
		t.Errorf(`log while stopping err: got "%v", want "%v"`, got, want)
	}

	// Release the NonblockingLogger.run loop and wait for it to stop.
	close(loggerdone)
	<-al.Stopped()
}
