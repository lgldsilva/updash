//go:build darwin

package elevate

import "testing"

func TestParseDialogPassword(t *testing.T) {
	pw, err := parseDialogPassword("button returned:OK, text returned:secret")
	if err != nil || pw != "secret" {
		t.Fatalf("got pw=%q err=%v", pw, err)
	}
	if _, err := parseDialogPassword("button returned:Cancel, text returned:"); err != ErrDialogCancelled {
		t.Fatalf("expected cancel, got %v", err)
	}
	if _, err := parseDialogPassword("unexpected"); err == nil {
		t.Fatal("expected error for unexpected response")
	}
}
