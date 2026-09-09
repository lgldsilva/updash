package scanner

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lgldsilva/updash/internal/model"
)

func TestErrItemCarriesCause(t *testing.T) {
	it := errItem(binPnpm, model.CatPnpm, errors.New("fork/exec /x/pnpm: exec format error"))
	if it.Status != model.StatusError || it.CurrentVer != statusError {
		t.Fatalf("bad item: %+v", it)
	}
	if it.Error != "fork/exec /x/pnpm: exec format error" {
		t.Errorf("cause not kept: %q", it.Error)
	}

	if it := errItem(binPnpm, model.CatPnpm, nil); it.Error != "" {
		t.Errorf("nil cause must leave Error empty, got %q", it.Error)
	}
}

func TestErrCauseCondenses(t *testing.T) {
	if got := errCause(nil); got != "" {
		t.Errorf("nil error must map to empty, got %q", got)
	}
	if got := errCause(errors.New("first line\nsecond line\n")); got != "first line" {
		t.Errorf("want first line only, got %q", got)
	}
	long := strings.Repeat("x", errCauseMaxLen+50)
	got := errCause(errors.New(long))
	if n := utf8.RuneCountInString(got); n != errCauseMaxLen {
		t.Errorf("want %d-rune truncated cause, got %d runes", errCauseMaxLen, n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("want ellipsis suffix, got %q", got)
	}
}

func TestErrItemFormatsWithSource(t *testing.T) {
	// Simulate the placeholder-pnpm failure: the item must tell the user WHY,
	// not just "error".
	cause := fmt.Errorf("fork/exec %s: exec format error", "/home/u/.npm-global/bin/pnpm")
	it := errItem(binPnpm, model.CatPnpm, cause)
	if it.Error == "" || it.Error == statusError {
		t.Fatalf("expected actionable cause, got %q", it.Error)
	}
}
