package cli

import (
	"errors"
	"testing"
)

func TestRunAllOutcome(t *testing.T) {
	cases := []struct {
		name         string
		ufail, cfail int
		partial      bool
		wantErr      bool
		wantExit2    bool
	}{
		{"clean run", 0, 0, false, false, false},
		{"partial only", 0, 0, true, true, true},
		{"update fail", 1, 0, false, true, false},
		{"clean fail", 0, 2, false, true, false},
		{"fail plus partial", 1, 1, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runAllOutcome(tc.ufail, tc.cfail, tc.partial)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			var ee *ExitError
			if got := errors.As(err, &ee) && ee.Code == 2; got != tc.wantExit2 {
				t.Fatalf("exit-2 = %v, want %v (err %v)", got, tc.wantExit2, err)
			}
		})
	}
}

func TestIsFatalErr(t *testing.T) {
	if isFatalErr(nil) {
		t.Fatal("nil must not be fatal")
	}
	if !isFatalErr(errors.New("boom")) {
		t.Fatal("plain error must be fatal")
	}
}
