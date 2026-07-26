package dashboard

import (
	"io/fs"
	"testing"
)

func TestEmbeddedDashboardContainsIndex(t *testing.T) {
	data, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("embedded dashboard index is empty")
	}
}
