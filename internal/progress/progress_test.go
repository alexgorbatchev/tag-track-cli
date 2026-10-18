package progress

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewReporter_UnixSocket(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listening on unix socket: %v", err)
	}
	defer l.Close()

	var received Event
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = json.NewDecoder(conn).Decode(&received)
	}()

	rep, err := NewReporter(context.Background(), "unix://"+sockPath, "")
	if err != nil {
		t.Fatalf("creating unix reporter: %v", err)
	}
	defer rep.Close()

	if err := rep.Emit(Event{Type: EventPhaseStart, Message: "testing"}); err != nil {
		t.Fatalf("emitting event: %v", err)
	}

	select {
	case <-done:
		if received.Type != EventPhaseStart || received.Message != "testing" {
			t.Fatalf("unexpected received event: %+v", received)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for socket event")
	}
}

func TestNewReporter_TCPSocket(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening on tcp: %v", err)
	}
	defer l.Close()

	var received Event
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = json.NewDecoder(conn).Decode(&received)
	}()

	rep, err := NewReporter(context.Background(), "tcp://"+l.Addr().String(), "")
	if err != nil {
		t.Fatalf("creating tcp reporter: %v", err)
	}
	defer rep.Close()

	if err := rep.Emit(Event{Type: EventMetadataFound, Message: "matched"}); err != nil {
		t.Fatalf("emitting event: %v", err)
	}

	select {
	case <-done:
		if received.Type != EventMetadataFound || received.Message != "matched" {
			t.Fatalf("unexpected received event: %+v", received)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tcp event")
	}
}

func TestNewReporter_FD(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	defer r.Close()

	rep, err := NewReporter(context.Background(), "fd://"+string(os.PathSeparator)+".."+string(os.PathSeparator)+"3", "")
	if err == nil && rep != nil {
		_ = rep.Close()
	}
	_ = w.Close()
}

func TestNewReporter_InvalidTarget(t *testing.T) {
	t.Run("empty target", func(t *testing.T) {
		_, err := NewReporter(context.Background(), "", "")
		if err == nil {
			t.Fatal("expected error on empty target")
		}
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, err := NewReporter(context.Background(), "http://example.com/stream", "")
		if err == nil {
			t.Fatal("expected error on http scheme")
		}
	})

	t.Run("invalid fd", func(t *testing.T) {
		_, err := NewReporter(context.Background(), "fd://notanumber", "")
		if err == nil {
			t.Fatal("expected error on invalid fd")
		}
	})

	t.Run("unreachable unix", func(t *testing.T) {
		_, err := NewReporter(context.Background(), "unix:///nonexistent/path/here.sock", "")
		if err == nil {
			t.Fatal("expected dial error")
		}
	})

	t.Run("unreachable tcp", func(t *testing.T) {
		_, err := NewReporter(context.Background(), "tcp://127.0.0.1:54321", "")
		if err == nil {
			t.Fatal("expected dial error")
		}
	})
}

func TestReporter_ConcurrentEmits(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	defer r.Close()

	rep := &Reporter{
		encoder: json.NewEncoder(w),
		closer:  w,
	}

	var wg sync.WaitGroup
	count := 20
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = rep.Emit(Event{Type: EventProgress, Step: idx})
		}(i)
	}

	wg.Wait()
	_ = rep.Close()

	scanner := bufio.NewScanner(r)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	if lines != count {
		t.Fatalf("expected %d emitted lines, got %d", count, lines)
	}
}

func TestReporter_NilSafe(t *testing.T) {
	var r *Reporter
	if err := r.Emit(Event{Type: EventPhaseStart}); err != nil {
		t.Fatalf("unexpected error on nil reporter emit: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("unexpected error on nil reporter close: %v", err)
	}
}

func TestNewReporter_StdoutStderr(t *testing.T) {
	rOut, err := NewReporter(context.Background(), "stdout", "")
	if err != nil {
		t.Fatalf("stdout reporter failed: %v", err)
	}
	_ = rOut.Emit(Event{Type: EventPhaseStart})
	_ = rOut.Close()

	rErr, err := NewReporter(context.Background(), "stderr", "")
	if err != nil {
		t.Fatalf("stderr reporter failed: %v", err)
	}
	_ = rErr.Emit(Event{Type: EventPhaseStart})
	_ = rErr.Close()
}

func TestParseTarget_AllSchemes(t *testing.T) {
	tests := []struct {
		target string
		wantScheme string
	}{
		{"stdout://", "stdout"},
		{"stderr://", "stderr"},
		{"/var/run/test.sock", "unix"},
		{"./test.sock", "unix"},
		{"../test.sock", "unix"},
		{"127.0.0.1:9099", "tcp"},
	}

	for _, tt := range tests {
		s, _, err := ParseTarget(tt.target)
		if err != nil || s != tt.wantScheme {
			t.Fatalf("ParseTarget(%q) = (%q, err=%v), want scheme %q", tt.target, s, err, tt.wantScheme)
		}
	}
}

func TestResolveTargetURI(t *testing.T) {
	tests := []struct {
		target string
		socket string
		env    string
		want   string
	}{
		{"tcp://127.0.0.1:9099", "", "", "tcp://127.0.0.1:9099"},
		{"", "unix:///tmp/test.sock", "", "unix:///tmp/test.sock"},
		{"", "", "stdout", "stdout"},
		{"", "", "", ""},
	}

	for _, tt := range tests {
		got := ResolveTargetURI(tt.target, tt.socket, tt.env)
		if got != tt.want {
			t.Fatalf("ResolveTargetURI(%q, %q, %q) = %q, want %q", tt.target, tt.socket, tt.env, got, tt.want)
		}
	}
}
