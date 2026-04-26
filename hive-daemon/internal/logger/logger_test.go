package logger_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
)

func TestLog_DefaultOutputIsStderr(t *testing.T) {
	writer := logger.Log.Writer()
	if writer == os.Stderr {
		return
	}

	writerFile, okWriter := writer.(*os.File)
	if !okWriter {
		t.Fatalf("expected logger writer to be *os.File, got %T", writer)
	}
	if writerFile.Fd() == os.Stdout.Fd() {
		t.Fatalf("expected logger writer not to point to stdout fd=%d", os.Stdout.Fd())
	}
}

func TestLog_HasHivePrefix(t *testing.T) {
	var buf bytes.Buffer
	logger.Log.SetOutput(&buf)
	defer logger.Log.SetOutput(os.Stderr)

	logger.Log.Println("prefix check")

	if !strings.Contains(buf.String(), "[hive]") {
		t.Errorf("expected '[hive]' prefix in log output, got: %q", buf.String())
	}
}

func TestLog_DoesNotWriteToStdout(t *testing.T) {
	// Capture stdout
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	// Redirect logger to a buffer (not stdout)
	var buf bytes.Buffer
	logger.Log.SetOutput(&buf)
	defer func() {
		logger.Log.SetOutput(os.Stderr)
		os.Stdout = origStdout
	}()

	logger.Log.Println("should not appear on stdout")

	// Flush and restore stdout
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = origStdout
	var stdoutContent bytes.Buffer
	if _, err := io.Copy(&stdoutContent, r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	if stdoutContent.Len() > 0 {
		t.Errorf("expected no output on stdout, got: %q", stdoutContent.String())
	}

	if !strings.Contains(buf.String(), "should not appear on stdout") {
		t.Errorf("expected log message in buffer, got: %q", buf.String())
	}
}
