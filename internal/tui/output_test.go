package tui

import (
	"testing"
)

func TestOutputLog_Write(t *testing.T) {
	log := newOutputLog(nil)
	payload := []byte("line one\nline two\npartial")
	n, err := log.Write(payload)
	if err != nil || n != len(payload) {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if len(log.buf) == 0 || string(log.buf) != "partial" {
		t.Fatalf("partial buffer = %q", log.buf)
	}
}

func TestOutputLog_FlushClearsFinalPartialLine(t *testing.T) {
	log := newOutputLog(nil)
	if _, err := log.Write([]byte("final partial")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	log.Flush()
	if len(log.buf) != 0 {
		t.Fatalf("Flush must clear the final partial line, got %q", log.buf)
	}
	// Calling Flush more than once must not emit or retain the line again.
	log.Flush()
	if len(log.buf) != 0 {
		t.Fatalf("second Flush must remain idempotent, got %q", log.buf)
	}
}
