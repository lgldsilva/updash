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