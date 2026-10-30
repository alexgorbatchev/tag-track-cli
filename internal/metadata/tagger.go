package metadata

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexgorbatchev/godeps"
	"github.com/alexgorbatchev/tag-track-cli/internal/cache"
	"github.com/alexgorbatchev/tag-track-cli/internal/display"
)

// NormalizeCoverArtToSquare center-crops and scales any input image to a 1:1 square (1400x1400) JPEG.
func NormalizeCoverArtToSquare(ctx context.Context, inputImagePath, outDir string) (string, error) {
	if inputImagePath == "" {
		return "", fmt.Errorf("empty input image path")
	}
	if _, err := os.Stat(inputImagePath); err != nil {
		return inputImagePath, err
	}

	squarePath := filepath.Join(outDir, fmt.Sprintf("square_cover_%d.jpg", time.Now().UnixNano()))
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "ffmpeg",
		"-v", "quiet",
		"-hide_banner",
		"-y",
		"-i", inputImagePath,
		"-vf", "crop='min(iw,ih)':'min(iw,ih)',scale=1400:1400",
		"-q:v", "2",
		squarePath,
	)
	if err := cmd.Run(); err != nil {
		return inputImagePath, err
	}
	if stat, err := os.Stat(squarePath); err == nil && stat.Size() > 0 {
		return squarePath, nil
	}
	return inputImagePath, nil
}

// DownloadArtwork downloads artwork from URL (or retrieves from cache) and saves to a local file.
func DownloadArtwork(ctx context.Context, artworkURL, tmpDir string, cacheInst *cache.Cache) (string, error) {
	if strings.TrimSpace(artworkURL) == "" {
		return "", fmt.Errorf("empty artwork URL")
	}
	parsed, err := url.Parse(artworkURL)
	if err != nil {
		return "", fmt.Errorf("parsing artwork location: %w", err)
	}
	if parsed.Scheme == "" {
		data, err := os.ReadFile(artworkURL)
		if err != nil {
			return "", fmt.Errorf("reading artwork file: %w", err)
		}
		path := filepath.Join(tmpDir, "local_cover"+filepath.Ext(artworkURL))
		if err := os.WriteFile(path, data, 0600); err != nil {
			return "", fmt.Errorf("copying artwork file: %w", err)
		}
		return path, nil
	}

	if cacheInst != nil {
		if cachedPath, found := cacheInst.GetFile("artworks", artworkURL, 30*24*time.Hour); found {
			return cachedPath, nil
		}
	}

	dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, artworkURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating artwork request: %w", err)
	}

	resp, err := getDefaultHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading artwork: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("artwork download returned status %d", resp.StatusCode)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading artwork payload: %w", err)
	}
	if len(imgBytes) == 0 {
		return "", fmt.Errorf("empty artwork payload")
	}

	if cacheInst != nil {
		if savedPath, errPut := cacheInst.PutFile("artworks", artworkURL, imgBytes); errPut == nil && savedPath != "" {
			return savedPath, nil
		}
	}

	cPath := filepath.Join(tmpDir, fmt.Sprintf("cover_%d.jpg", time.Now().UnixNano()))
	if errWrite := os.WriteFile(cPath, imgBytes, 0644); errWrite != nil {
		return "", fmt.Errorf("saving artwork temp file: %w", errWrite)
	}
	return cPath, nil
}

// ApplyMetadataToLocalTrack embeds metadata and high-res cover art into the audio file
// using ffmpeg and writes the tagged track to <outDir>/<SanitizedArtist - SanitizedTitle>.<ext>.
func ApplyMetadataToLocalTrack(ctx context.Context, filePath string, metadata TrackMetadataResult, outDir string, verboseAndCache ...interface{}) (string, error) {
	isVerbose := false
	var cacheInst *cache.Cache

	for _, v := range verboseAndCache {
		switch val := v.(type) {
		case bool:
			isVerbose = val
		case *cache.Cache:
			cacheInst = val
		}
	}

	if outDir == "" {
		outDir = filepath.Dir(filePath)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return filePath, fmt.Errorf("creating output directory %s: %w", outDir, err)
	}

	if isVerbose {
		display.Fprintf(os.Stderr, "tagging: %s\n", filePath)
		if metadata.CoverArtURL != "" {
			display.Fprintf(os.Stderr, "coverart: downloading %s\n", metadata.CoverArtURL)
		}
	}

	_, statErr := os.Stat(".tmp")
	if err := os.MkdirAll(".tmp", 0755); err != nil {
		return filePath, fmt.Errorf("creating temporary directory: %w", err)
	}
	if os.IsNotExist(statErr) {
		defer os.Remove(".tmp") // Remove only when empty; concurrent operations own their files.
	}
	tmpDir, err := os.MkdirTemp(".tmp", "tag-")
	if err != nil {
		return filePath, fmt.Errorf("creating tagging directory: %w", err)
	}
	defer os.RemoveAll(tmpDir) // Best-effort cleanup of files owned by this operation.

	var coverTempPath string
	if metadata.CoverArtURL != "" {
		if p, err := DownloadArtwork(ctx, metadata.CoverArtURL, tmpDir, cacheInst); err == nil {
			coverTempPath = p
		} else if metadata.RequireArtwork {
			return filePath, err
		}

		if coverTempPath == "" {
			// Check if input audio file has an embedded video / thumbnail stream
			embeddedThumbPath := filepath.Join(tmpDir, fmt.Sprintf("embedded_thumb_%d.jpg", time.Now().UnixNano()))
			cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			thumbCmd := exec.CommandContext(cmdCtx, "ffmpeg",
				"-v", "quiet",
				"-hide_banner",
				"-y",
				"-i", filePath,
				"-an",
				"-vcodec", "copy",
				embeddedThumbPath,
			)
			if errThumb := thumbCmd.Run(); errThumb == nil {
				if stat, err := os.Stat(embeddedThumbPath); err == nil && stat.Size() > 0 {
					coverTempPath = embeddedThumbPath
				}
			}
			cancel()
		}

		if coverTempPath != "" {
			if squareCover, errNorm := NormalizeCoverArtToSquare(ctx, coverTempPath, tmpDir); errNorm == nil {
				coverTempPath = squareCover
			} else if metadata.RequireArtwork {
				return filePath, fmt.Errorf("normalizing artwork: %w", errNorm)
			}
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	tempOutPath := filepath.Join(tmpDir, fmt.Sprintf("tagged_%d%s", time.Now().UnixNano(), ext))

	defer func() {
		if coverTempPath != "" && strings.HasPrefix(coverTempPath, tmpDir) {
			_ = os.Remove(coverTempPath)
		}
		_ = os.Remove(tempOutPath)
	}()

	var cleanFilename string
	if metadata.Artist != "" && metadata.Artist != "Unknown Artist" {
		cleanFilename = SanitizeFilename(fmt.Sprintf("%s - %s%s", metadata.Artist, metadata.Title, ext))
	} else {
		base := strings.TrimSuffix(filepath.Base(filePath), ext)
		cleanFilename = SanitizeFilename(fmt.Sprintf("%s%s", base, ext))
	}

	finalOutPath := filepath.Join(outDir, cleanFilename)

	dateTag := metadata.ReleaseYear
	if metadata.ReleaseDate != "" {
		dateTag = metadata.ReleaseDate
	}

	provenanceDate := time.Now().UTC().Format("2006-01-02")
	if !metadata.FetchedAt.IsZero() {
		provenanceDate = metadata.FetchedAt.UTC().Format("2006-01-02")
	}

	sourceURL := metadata.AudioSourceURL
	if sourceURL == "" {
		sourceURL = "local file"
	}
	metaProvider := metadata.Source
	if metaProvider == "" {
		metaProvider = "tag-track"
	}

	commentTag := fmt.Sprintf("Source: %s | Metadata: %s | Tagged: %s", sourceURL, metaProvider, provenanceDate)
	if metadata.Comment != nil {
		commentTag = *metadata.Comment
	} else {
		userComment, err := resolveUserComment(ctx, filePath, nil)
		if err != nil {
			return filePath, err
		}
		if userComment != "" {
			commentTag = userComment + " | " + commentTag
		}
	}

	args := []string{
		"-v", "quiet",
		"-hide_banner",
		"-y",
		"-i", filePath,
	}

	if coverTempPath != "" && ext != ".opus" && ext != ".ogg" {
		args = append(args,
			"-i", coverTempPath,
			"-map", "0:a:0",
			"-map", "1:0",
			"-c:a", "copy",
			"-c:v", "copy",
		)
		if ext == ".m4a" || ext == ".mp4" || ext == ".flac" {
			args = append(args, "-disposition:v", "attached_pic")
		} else if ext == ".mp3" {
			args = append(args,
				"-id3v2_version", "3",
				"-metadata:s:v", "title=Album cover",
				"-metadata:s:v", "comment=Cover (front)",
			)
		}
	} else {
		args = append(args, "-map", "0:a:0", "-map", "0:v?", "-c", "copy")
		if ext == ".mp3" {
			args = append(args, "-id3v2_version", "3")
		}
	}

	args = append(args, "-metadata", fmt.Sprintf("title=%s", metadata.Title))
	args = append(args, "-metadata", fmt.Sprintf("artist=%s", metadata.Artist))
	args = append(args, "-metadata", fmt.Sprintf("album=%s", metadata.Album))
	args = append(args, "-metadata", fmt.Sprintf("date=%s", dateTag))
	args = append(args, "-metadata", fmt.Sprintf("year=%s", dateTag))
	args = append(args, "-metadata", fmt.Sprintf("genre=%s", metadata.Genre))
	args = append(args, "-metadata", fmt.Sprintf("comment=%s", commentTag))

	args = append(args, tempOutPath)

	if isVerbose {
		if coverTempPath != "" {
			display.Fprintf(os.Stderr, "ffmpeg: embedding tags + square 1400x1400 artwork\n")
		} else {
			display.Fprintf(os.Stderr, "ffmpeg: embedding tags\n")
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		cleanErr := godeps.SanitizeStderr(stderr.String())
		if cleanErr != "" {
			return filePath, fmt.Errorf("ffmpeg tagging failed: %w (%s)", err, cleanErr)
		}
		return filePath, fmt.Errorf("ffmpeg tagging failed: %w", err)
	}

	// Move the temp tagged file to final output path
	if err := os.Rename(tempOutPath, finalOutPath); err != nil {
		inputBytes, errRead := os.ReadFile(tempOutPath)
		if errRead != nil {
			return filePath, fmt.Errorf("reading tagged temp file: %w", errRead)
		}
		if errWrite := os.WriteFile(finalOutPath, inputBytes, 0644); errWrite != nil {
			return filePath, fmt.Errorf("writing final output track %s: %w", finalOutPath, errWrite)
		}
		_ = os.Remove(tempOutPath)
	}

	return finalOutPath, nil
}
