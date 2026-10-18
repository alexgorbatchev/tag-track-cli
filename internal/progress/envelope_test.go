package progress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"
)

func TestReporterCompletion(t *testing.T) {
	var buf bytes.Buffer
	r := &Reporter{encoder: json.NewEncoder(&buf)}
	tests := []struct {
		name  string
		event Event
		want  float64
	}{
		{"initial", Event{Type: EventPhaseStart, Step: 1, TotalSteps: 5}, 0},
		{"search", Event{Type: EventPhaseStart, Step: 2, TotalSteps: 5}, 20},
		{"found", Event{Type: EventMetadataFound}, 20},
		{"selected", Event{Type: EventArtworkDownloaded}, 20},
		{"download", Event{Type: EventPhaseStart, Step: 3, TotalSteps: 5}, 40},
		{"explicit", Event{Type: EventProgress, Percent: 45.5}, 45.5},
		{"unspecified", Event{Type: EventProgress}, 45.5},
		{"error", Event{Type: EventError}, 45.5},
		{"retry", Event{Type: EventPhaseStart, Step: 3, TotalSteps: 5}, 45.5},
		{"negative", Event{Type: EventProgress, Percent: -10}, 45.5},
		{"nan", Event{Type: EventProgress, Percent: math.NaN()}, 45.5},
		{"infinite", Event{Type: EventProgress, Percent: math.Inf(1)}, 45.5},
		{"invalid step", Event{Type: EventPhaseStart, Step: 6, TotalSteps: 5}, 45.5},
		{"missing total", Event{Type: EventPhaseStart, Step: 4}, 45.5},
		{"verify", Event{Type: EventPhaseStart, Step: 4, TotalSteps: 5}, 60},
		{"metadata", Event{Type: EventPhaseStart, Step: 5, TotalSteps: 5}, 80},
		{"complete", Event{Type: EventComplete}, 100},
		{"above maximum", Event{Type: EventProgress, Percent: 120}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			if err := r.Emit(tt.event); err != nil {
				t.Fatalf("Emit: %v", err)
			}
			var got struct {
				Percent *float64 `json:"percent"`
			}
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Percent == nil {
				t.Fatalf("percent missing: %s", buf.String())
			}
			if *got.Percent != tt.want {
				t.Errorf("percent = %v, want %v", *got.Percent, tt.want)
			}
		})
	}
}

func TestReporterCallerJobFD(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	r, err := NewReporter(context.Background(), fmt.Sprintf("fd://%d", writer.Fd()), "caller")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	stamp := time.Date(2026, 10, 18, 12, 0, 0, 0, time.FixedZone("offset", -7*60*60))
	if err := r.Emit(Event{Type: EventComplete, Step: 1, TotalSteps: 2, Timestamp: stamp}); err != nil {
		t.Fatal(err)
	}
	var ev Event
	if err := json.NewDecoder(reader).Decode(&ev); err != nil {
		t.Fatal(err)
	}
	if ev.JobID != "caller" || ev.Sequence != 1 || ev.Percent != 50 || ev.Timestamp.Location() != time.UTC {
		t.Fatalf("bad event: %+v", ev)
	}
}

func TestReporterFailedWrite(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	r := &Reporter{encoder: json.NewEncoder(f)}
	if err := r.Emit(Event{Type: EventComplete}); err == nil {
		t.Fatal("expected closed-file error")
	}
	var buf bytes.Buffer
	r.encoder = json.NewEncoder(&buf)
	if err := r.Emit(Event{Type: EventPhaseStart}); err != nil {
		t.Fatal(err)
	}
	var ev Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Sequence != 1 || ev.Percent != 0 {
		t.Fatalf("failed write advanced state: %+v", ev)
	}
}

func TestReporterEnvelope(t *testing.T) {
	var buf bytes.Buffer
	r := &Reporter{encoder: json.NewEncoder(&buf)}
	const count = 24
	var wg sync.WaitGroup
	for range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Emit(Event{Type: EventProgress, Phase: "download"}); err != nil {
				t.Errorf("Emit: %v", err)
			}
		}()
	}
	wg.Wait()
	dec := json.NewDecoder(&buf)
	var jobID string
	for i := 1; i <= count; i++ {
		var got struct {
			JobID    string `json:"job_id"`
			Sequence int    `json:"sequence"`
		}
		if err := dec.Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.JobID == "" {
			t.Fatal("missing job_id")
		}
		if i == 1 {
			jobID = got.JobID
		}
		if got.JobID != jobID || got.Sequence != i {
			t.Fatalf("event %d: %+v, want job %q and sequence %d", i, got, jobID, i)
		}
	}
}
