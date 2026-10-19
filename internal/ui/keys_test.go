package ui

import (
	"context"
	"image"
	"net/http"
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"lighttube/internal/store"
	"lighttube/internal/yt"
)

type fakeYTDLP struct{ queries chan string }

func (f fakeYTDLP) Search(ctx context.Context, query string) ([]byte, error) {
	f.queries <- query
	return []byte(`{"entries":[]}`), nil
}

func (f fakeYTDLP) ResolveChannel(ctx context.Context, target string) (string, error) {
	return "", nil
}

// Enter in the focused search box must reach the editor. The global key
// handler runs first in each frame and used to take Enter for itself.
func TestEnterInSearchBoxStartsSearch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	st, err := store.NewDeferred(dir)
	if err != nil {
		t.Fatal(err)
	}
	queries := make(chan string, 1)
	ui := New(Options{
		YTClient: yt.NewClientWithOptions(fakeYTDLP{queries}, http.DefaultClient, dir),
		Store:    st,
		Account:  yt.NewAccountWithOptions(yt.AccountOptions{CookieFile: dir + "/none", CacheDir: dir}),
	})

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         &ops,
			Source:      r.Source(),
			Constraints: layout.Exact(image.Pt(1600, 1000)),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		}
		ui.layout(gtx)
		r.Frame(&ops)
	}

	frame()
	ui.searchEditor.SetText("linux")
	r.Source().Execute(key.FocusCmd{Tag: &ui.searchEditor})
	frame()
	r.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	frame()

	select {
	case q := <-queries:
		if q != "linux" {
			t.Fatalf("searched %q, want %q", q, "linux")
		}
	default:
		if ui.state.CurrentTab != TabSearch {
			t.Fatalf("Enter did not start a search; tab is %v", ui.state.CurrentTab)
		}
	}
}
