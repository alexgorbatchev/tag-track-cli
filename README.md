`tag-track` identifies and updates metadata and 1400x1400 square cover art in DJ audio collections while preserving the audio bitstream.

# What It Does

- **Multi-Source Metadata Discovery**: Automatically identifies tracks through acoustic fingerprinting (Shazam, AcoustID) and API fallbacks (iTunes Search API, MusicBrainz).
- **Square Cover Art Normalization**: Fetches and center-crops/scales album artwork to 1:1 square (1400x1400 JPEG) for crisp rendering on DJ decks, CDJs, and jog wheels.
- **Zero-Transcode Byte-for-Byte Tagging**: Preserves underlying audio bitstreams without lossy re-encoding using native stream copying (`-c:a copy`).
- **Origin Date & Provenance Tracking**: Retains full release dates (`YYYY-MM-DD`) and records acquisition provenance comments (`Source: ... | Metadata: ... | Tagged: ...`).
- **Batch Tagging & Collection Refresh**: Inspects, retags, and cleans filenames across entire DJ music libraries in place or into destination folders.
- **Composable Pipeline Integration**: Accepts file paths as arguments or lines from `stdin` to pair seamlessly with audio retrieval tools like `fetch-track`.

# How It Works

- Provide audio files as arguments or stream paths directly through standard input.
- `tag-track` inspects existing audio codecs, metadata tags, and artwork attachments.
- `tag-track` resolves title, artist, album, and release information for each file, reusing cached results when available.
- High-resolution artwork is downloaded and formatted into a 1:1 square image.
- Tags and artwork are embedded directly into the native audio container without transcoding.
- The file is renamed to a clean `<Artist> - <Title>.<ext>` format.

# How it Really Works

- Sends acoustic fingerprints to Shazam and AcoustID to identify tracks from their audio.
- Queries AcoustID, iTunes Search API, and MusicBrainz API in a fallback cascade to resolve official artist names, track titles, album releases, and year.
- Center-crops and resizes retrieved artwork via `ffmpeg` video filters (`crop='min(iw,ih)':'min(iw,ih)',scale=1400:1400`) to guarantee exact 1:1 square aspect ratio.
- Streams audio payloads via `-c:a copy` into target containers (ID3v2.3 for MP3, MP4 metadata atoms for M4A/AAC, and Vorbis comments for FLAC).
- Emits atomic structured NDJSON progress telemetry over UNIX domain sockets, TCP endpoints, or custom file descriptors when requested. Every event includes `job_id`, an increasing `sequence` starting at 1, a UTC `timestamp`, and numeric batch completion `percent` (0–100). `--job-id` accepts the caller's identifier; otherwise an identifier is generated for the run. File starts report the fraction of preceding files and file completion reports the fraction through the current file, including dry-run previews.
- With `--json`, returns one ordered batch of per-file results, including preflight errors for missing inputs, unreadable input streams, missing dependencies, invalid artwork options, and progress initialization failures. Failed commands exit nonzero.

# Installation

Download the precompiled binary for your platform from [GitHub Releases](https://github.com/alexgorbatchev/tag-track-cli/releases/latest):

```bash
curl -fsSL https://github.com/alexgorbatchev/tag-track-cli/releases/download/v1.0.0/tag-track_1.0.0_darwin_arm64.tar.gz | tar -xz
chmod +x tag-track
mkdir -p ~/.local/bin
mv tag-track ~/.local/bin/
```

# Setup

- Install [ffmpeg & ffprobe](https://ffmpeg.org/download.html) version 4.4 or newer for audio inspection and tagging.

# Quick Start

```bash
# Identify and tag an individual audio file
tag-track "tracks/Artist - Title.m4a"

# Inspect existing metadata tags, audio codec, and artwork
tag-track track inspect "tracks/Artist - Title.m4a"

# Update an entire collection of tracks in-place
tag-track track update --in-place ~/Music/DJ/*.m4a

# Embed a specific high-resolution artwork image
tag-track artwork embed "tracks/Artist - Title.m4a" --url "https://example.com/cover.jpg"

# Pipeline composition with fetch-track
fetch-track "Boris Brejcha - Space X" --no-metadata -o .tmp/ | tag-track -o tracks/
```

Sample Output (inspection of a one-second test file, excerpt):

```text
codec: aac
duration: 1s
bitrate: 10 kbps
sample_rate: 44100 Hz
channels: 2
title: Example Track
artist: Example Artist
has_artwork: false
```

# Options & Flags

Global options are `--verbose` / `-v`, `--job-id`, and `--help` / `-h`. Root `--version` prints the raw version. `tag-track skill` prints installation instructions for the [agent usage skill](skills/tag-track/SKILL.md), which documents every command and its flags. Use `tag-track self upgrade` to update the executable.

`tag-track` and `tag-track track update`:

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--out-dir <dir>` | `-o` | *(in-place / file dir)* | Output directory for tagged audio tracks |
| `--in-place` | `-i` | `false` | Update audio files in-place |
| `--artist <artist>` | | `""` | Explicit artist tag override |
| `--title <title>` | | `""` | Explicit title tag override |
| `--album <album>` | | `""` | Explicit album tag override |
| `--date <date>` | | `""` | Explicit release date/year override (YYYY or YYYY-MM-DD) |
| `--genre <genre>` | | `""` | Explicit genre tag override |
| `--comment <comment>` | | `""` | Preserve user text when omitted; replace it when supplied; `--comment ""` clears it. Generated provenance is refreshed only when the comment is omitted. |
| `--source-url <url>` | | `""` | Audio source recorded in provenance |
| `--artwork <url\|path>` | | `""` | Cover artwork image URL or local file path to embed |
| `--refresh` | | `false` | Bypass metadata/artwork cache reads and store fresh results; `--no-cache` takes precedence and disables reads and writes |
| `--dry-run` | | `false` | Preview proposed metadata without modifying files on disk |
| `--no-cache` | | `false` | Disable local caching for search queries and artwork |
| `--progress-target <uri>` | | `""` | Target URI for streaming NDJSON progress telemetry |
| `--job-id <id>` | | generated | Job ID included in each progress event; available on root and subcommands |
| `--auto-install` | | `false` | Automatically install missing dependencies |
| `--verbose` | `-v` | `false` | Enable verbose logging |
| `--help` | `-h` | | Display command help |
| `--version` | | | Display version |

`tag-track artwork embed`:

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--url <url\|path>` | | `""` | Artwork image URL or local path; required |
| `--artwork <url\|path>` | | `""` | Alternative to `--url` |
| `--in-place` | `-i` | `false` | Embed into the source directory |
| `--out-dir <dir>` | `-o` | `""` | Destination; empty uses the source directory |

# Output & Automation

- `AGENT=1` (also `true` or `yes`) emits compact key-value output; human mode uses labeled messages and a spinner. Paths under home display as `~`. Piped `DONE:` and `output:` paths are accepted, including `~/` paths.
- Help and results go to stdout; command errors go to stderr. Per-file inspection, tagging, and artwork failures continue the batch, then the command exits 1 if any file failed.
- `--dry-run` preserves audio files but can perform network lookups and cache writes. Metadata/artwork caching uses `XDG_CACHE_HOME` or the platform cache directory.

# License

[MIT License](LICENSE) (c) 2026 Alex Gorbatchev
