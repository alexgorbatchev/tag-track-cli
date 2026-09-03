package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EventType represents the category of a progress event.
type EventType string

const (
	EventPhaseStart         EventType = "phase_start"
	EventMetadataFound      EventType = "metadata_found"
	EventArtworkDownloaded  EventType = "artwork_downloaded"
	EventTagged             EventType = "tagged"
	EventProgress           EventType = "progress"
	EventComplete           EventType = "complete"
	EventError              EventType = "error"
)

// MetadataInfo describes resolved track metadata and artwork information.
type MetadataInfo struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album,omitempty"`
	Genre       string `json:"genre,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
	ReleaseYear string `json:"release_year,omitempty"`
	CoverArtURL string `json:"cover_art_url,omitempty"`
	Source      string `json:"source,omitempty"`
}

// ResultInfo describes the final tagged audio file.
type ResultInfo struct {
	Path        string `json:"path"`
	Title       string `json:"title,omitempty"`
	Artist      string `json:"artist,omitempty"`
	Album       string `json:"album,omitempty"`
	ReleaseYear string `json:"release_year,omitempty"`
	Source      string `json:"source,omitempty"`
	Status      string `json:"status,omitempty"`
}

// Event represents an atomic structured telemetry/progress update sent over the socket.
type Event struct {
	Timestamp  time.Time     `json:"timestamp"`
	Type       EventType     `json:"type"`
	Phase      string        `json:"phase,omitempty"`
	Step       int           `json:"step,omitempty"`
	TotalSteps int           `json:"total_steps,omitempty"`
	Message    string        `json:"message,omitempty"`
	Percent    float64       `json:"percent,omitempty"`
	Metadata   *MetadataInfo `json:"metadata,omitempty"`
	Result     *ResultInfo   `json:"result,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// Reporter handles streaming progress events to a socket, file descriptor, or stream.
type Reporter struct {
	mu         sync.Mutex
	closer     io.Closer
	encoder    *json.Encoder
	shouldSkip bool
}

// ParseTarget parses a target string into a normalized scheme and address/path.
func ParseTarget(raw string) (scheme, address string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", errors.New("empty progress target")
	}

	switch trimmed {
	case "stdout", "stdout://":
		return "stdout", "", nil
	case "stderr", "stderr://":
		return "stderr", "", nil
	}

	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err == nil && u.Scheme != "" {
			addr := u.Host + u.Path
			return u.Scheme, addr, nil
		}
	}

	// Auto-detect unix domain socket by path prefix or .sock extension
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasSuffix(trimmed, ".sock") {
		return "unix", trimmed, nil
	}

	// Check if it's host:port
	if strings.Contains(trimmed, ":") {
		return "tcp", trimmed, nil
	}

	return "", "", fmt.Errorf("unrecognized progress target format %q", raw)
}

// NewReporter connects to the specified progress target and returns a Reporter.
func NewReporter(ctx context.Context, target string) (*Reporter, error) {
	scheme, address, err := ParseTarget(target)
	if err != nil {
		return nil, err
	}

	var writer io.Writer
	var closer io.Closer

	switch scheme {
	case "stdout":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	case "unix":
		var d net.Dialer
		conn, err := d.DialContext(ctx, "unix", address)
		if err != nil {
			return nil, fmt.Errorf("connecting to unix progress socket %s: %w", address, err)
		}
		writer = conn
		closer = conn
	case "tcp":
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("connecting to tcp progress target %s: %w", address, err)
		}
		writer = conn
		closer = conn
	case "fd":
		fdNum, err := strconv.Atoi(address)
		if err != nil {
			return nil, fmt.Errorf("invalid file descriptor %s: %w", address, err)
		}
		file := os.NewFile(uintptr(fdNum), fmt.Sprintf("fd-%d", fdNum))
		if file == nil {
			return nil, fmt.Errorf("cannot open file descriptor %d", fdNum)
		}
		writer = file
		closer = file
	default:
		return nil, fmt.Errorf("unsupported progress target scheme: %s", scheme)
	}

	return &Reporter{
		closer:  closer,
		encoder: json.NewEncoder(writer),
	}, nil
}

// Emit sends an event to the progress target.
func (r *Reporter) Emit(event Event) error {
	if r == nil || r.encoder == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	return r.encoder.Encode(event)
}

// Close closes any underlying network connection or file descriptor.
func (r *Reporter) Close() error {
	if r == nil || r.closer == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closer.Close()
}

// ResolveTargetURI resolves shorthand or environment-provided target URIs.
func ResolveTargetURI(flagTarget, socketTarget, envTarget string) string {
	if strings.TrimSpace(flagTarget) != "" {
		return flagTarget
	}
	if strings.TrimSpace(socketTarget) != "" {
		return socketTarget
	}
	return strings.TrimSpace(envTarget)
}
