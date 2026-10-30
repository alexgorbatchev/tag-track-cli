package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FFprobeFormat describes format metadata returned by ffprobe.
type FFprobeFormat struct {
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

// FFprobeStream describes individual stream metadata from ffprobe.
type FFprobeStream struct {
	CodecType   string            `json:"codec_type"`
	CodecName   string            `json:"codec_name"`
	SampleRate  string            `json:"sample_rate"`
	Channels    int               `json:"channels"`
	BitRate     string            `json:"bit_rate"`
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	Disposition map[string]int    `json:"disposition"`
	Tags        map[string]string `json:"tags"`
}

// FFprobeOutput describes the top-level ffprobe JSON container.
type FFprobeOutput struct {
	Format  FFprobeFormat   `json:"format"`
	Streams []FFprobeStream `json:"streams"`
}

// TrackInspection contains parsed tag, audio, and artwork details for a track.
type TrackInspection struct {
	Path            string            `json:"path"`
	Filename        string            `json:"filename"`
	Format          string            `json:"format"`
	Codec           string            `json:"codec"`
	Duration        time.Duration     `json:"duration"`
	DurationSeconds float64           `json:"duration_seconds"`
	BitRateKbps     int               `json:"bit_rate_kbps"`
	SampleRateHz    int               `json:"sample_rate_hz"`
	Channels        int               `json:"channels"`
	Title           string            `json:"title,omitempty"`
	Artist          string            `json:"artist,omitempty"`
	Album           string            `json:"album,omitempty"`
	Date            string            `json:"date,omitempty"`
	Genre           string            `json:"genre,omitempty"`
	Comment         string            `json:"comment,omitempty"`
	HasArtwork      bool              `json:"has_artwork"`
	ArtworkWidth    int               `json:"artwork_width,omitempty"`
	ArtworkHeight   int               `json:"artwork_height,omitempty"`
	IsSquareArtwork bool              `json:"is_square_artwork"`
	RawTags         map[string]string `json:"raw_tags,omitempty"`
}

// getTagCaseInsensitive retrieves tag values regardless of uppercase/lowercase metadata keys.
func getTagCaseInsensitive(tags map[string]string, keys ...string) string {
	for _, target := range keys {
		for k, v := range tags {
			if strings.EqualFold(k, target) && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// InspectAudioTrack analyzes an audio file and returns its tags, audio streams, and artwork properties.
func InspectAudioTrack(ctx context.Context, filePath string, runner CommandRunner) (*TrackInspection, error) {
	if filePath == "" {
		return nil, errors.New("empty audio file path")
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("audio file not found: %w", err)
	}

	if runner == nil {
		runner = defaultRunner
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	outputBytes, err := runner(cmdCtx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("probing file %s: %w", filePath, err)
	}

	var probe FFprobeOutput
	if err := json.Unmarshal(outputBytes, &probe); err != nil {
		return nil, fmt.Errorf("parsing ffprobe json for %s: %w", filePath, err)
	}

	info := &TrackInspection{
		Path:     filePath,
		Filename: filepath.Base(filePath),
		Format:   probe.Format.FormatName,
		RawTags:  probe.Format.Tags,
	}

	if durSec, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
		info.DurationSeconds = durSec
		info.Duration = time.Duration(durSec * float64(time.Second))
	}

	if br, err := strconv.Atoi(probe.Format.BitRate); err == nil && br > 0 {
		info.BitRateKbps = br / 1000
	}

	// Extract standard tags from format
	info.Title = getTagCaseInsensitive(probe.Format.Tags, "title", "TIT2")
	info.Artist = getTagCaseInsensitive(probe.Format.Tags, "artist", "TPE1", "album_artist", "TPE2")
	info.Album = getTagCaseInsensitive(probe.Format.Tags, "album", "TALB")
	info.Date = getTagCaseInsensitive(probe.Format.Tags, "date", "year", "TDRC", "TYER")
	info.Genre = getTagCaseInsensitive(probe.Format.Tags, "genre", "TCON")
	info.Comment = getTagCaseInsensitive(probe.Format.Tags, "comment", "COMM", "description")

	// Inspect streams for audio properties and artwork attachments
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" && info.Codec == "" {
			info.Codec = stream.CodecName
			info.Channels = stream.Channels
			if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
				info.SampleRateHz = sr
			}
			if info.BitRateKbps == 0 {
				if sbr, err := strconv.Atoi(stream.BitRate); err == nil && sbr > 0 {
					info.BitRateKbps = sbr / 1000
				}
			}
		} else if stream.CodecType == "video" || stream.Disposition["attached_pic"] == 1 {
			info.HasArtwork = true
			info.ArtworkWidth = stream.Width
			info.ArtworkHeight = stream.Height
			if stream.Width > 0 && stream.Height > 0 && stream.Width == stream.Height {
				info.IsSquareArtwork = true
			}
		}
	}

	return info, nil
}
