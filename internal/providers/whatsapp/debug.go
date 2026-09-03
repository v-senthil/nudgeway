package whatsapp

import (
	"io"
	"os"
)

// getDebugSink returns the io.Writer diagnostic dumps go to. Defaults to
// stderr; a test may swap this to a buffer via SetDebugSink. Kept out of
// client.go so the swap point is trivially greppable.
func getDebugSink() io.Writer {
	if debugSink != nil {
		return debugSink
	}
	return os.Stderr
}

// debugSink is nil unless the process explicitly configures it.
var debugSink io.Writer

// SetDebugSink redirects the adapter's error-path dumps. Nil restores the
// default (stderr).
func SetDebugSink(w io.Writer) { debugSink = w }
