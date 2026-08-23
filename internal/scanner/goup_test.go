package scanner

import (
	"errors"
	"os"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestScanGoBinInventory_ReadDirErrorsAreTruthful(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want model.Status
	}{
		{name: "missing directory is an empty inventory", err: os.ErrNotExist, want: model.StatusInfo},
		{name: "other IO error is not empty", err: errors.New("permission denied"), want: model.StatusError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := scanGoBinInventory("/unimportant", func(string) ([]os.DirEntry, error) { return nil, tc.err })
			if len(items) != 1 || items[0].Status != tc.want {
				t.Fatalf("items=%+v, want %s", items, tc.want)
			}
		})
	}
}
