package spinner

import (
	"testing"
	"time"
)

func TestSpinnerLifecycle(t *testing.T) {
	sp := New("testing")
	if sp == nil {
		t.Fatal("expected spinner to be non-nil")
	}

	sp.PrintAbove("Inactive message")
	sp.Start()
	sp.Update("working hard...")
	time.Sleep(100 * time.Millisecond)
	sp.PrintAbove("Candidate found above spinner")
	sp.Stop()
}

func TestSpinnerNilSafe(t *testing.T) {
	var sp *Spinner
	sp.Start()
	sp.Update("noop")
	sp.PrintAbove("test")
	sp.Stop()

	empty := &Spinner{}
	empty.Start()
	empty.Update("noop")
	empty.PrintAbove("test")
	empty.Stop()
}
