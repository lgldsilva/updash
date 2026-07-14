package tui

import "testing"

func TestNeedsSpinner(t *testing.T) {
	s := New()
	if s.NeedsSpinner() {
		t.Fatal("should not spin when idle")
	}
	s.Updating = true
	if !s.NeedsSpinner() {
		t.Fatal("should spin when updating")
	}
}

func TestAdvanceSpinner(t *testing.T) {
	s := New()
	before := s.SpinnerFrame
	s.AdvanceSpinner()
	if s.SpinnerFrame == before {
		t.Fatal("spinner frame should advance")
	}
}
