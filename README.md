tag-track is a specialized CLI tool for discovering, identifying, refreshing, and embedding normalized metadata and high-resolution 1400x1400 square cover art into DJ audio collections with zero lossy transcoding.

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
- If metadata is missing or a refresh is requested, `tag-track` finger-prints the audio and looks up canonical title, artist, album, and release information.
- High-resolution artwork is downloaded and formatted into a 1:1 square image.
- Tags and artwork are embedded directly into the native audio container without transcoding.
- The file is renamed to a clean `<Artist> - <Title>.<ext>` format.

# How it Really Works

- Generates 16-bit 11025 Hz mono PCM samples via `ffmpeg` to compute Chromaprint fingerprints (`gochromaprint`) and Shazam signatures (`goshazam`).
- Queries AcoustID, iTunes Search API, and MusicBrainz API in a fallback cascade to resolve official artist names, track titles, album releases, and year.
- Center-crops and resizes retrieved artwork via `ffmpeg` video filters (`crop='min(iw,ih)':'min(iw,ih)',scale=1400:1400`) to guarantee exact 1:1 square aspect ratio.
- Streams audio payloads via `-c:a copy` into target containers (ID3v2.3 for MP3, MP4 metadata atoms for M4A/AAC, and Vorbis comments for FLAC).
- Emits atomic structured NDJSON progress telemetry over UNIX domain sockets, TCP endpoints, or custom file descriptors when requested.
- With `--json`, returns one ordered batch of per-file results, including preflight errors for missing inputs, unreadable input streams, missing dependencies, invalid artwork options, and progress initialization failures. Failed commands exit nonzero.

# Prerequisites

- [ffmpeg & ffprobe](https://ffmpeg.org/download.html) (version 4.4 or newer)

# Installation

Download the precompiled binary for your platform from [GitHub Releases](https://github.com/alexgorbatchev/tag-track-cli/releases/latest):

```bash
curl -fsSL https://github.com/alexgorbatchev/tag-track-cli/releases/latest/download/tag-track_darwin_arm64.tar.gz | tar -xz
chmod +x tag-track
mv tag-track ~/.local/bin/
```

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

# Options & Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--out-dir <dir>` | `-o` | *(in-place / file dir)* | Output directory for tagged audio tracks |
| `--in-place` | `-i` | `false` | Update audio files in-place |
| `--artist <artist>` | | `""` | Explicit artist tag override |
| `--title <title>` | | `""` | Explicit title tag override |
| `--album <album>` | | `""` | Explicit album tag override |
| `--date <date>` | | `""` | Explicit release date/year override (YYYY or YYYY-MM-DD) |
| `--genre <genre>` | | `""` | Explicit genre tag override |
| `--comment <comment>` | | `""` | Explicit comment tag override |
| `--artwork <url\|path>` | | `""` | Cover artwork image URL or local file path to embed |
| `--refresh` | | `false` | Force re-identification and re-tagging of files |
| `--dry-run` | | `false` | Preview proposed metadata without modifying files on disk |
| `--no-cache` | | `false` | Disable local caching for search queries and artwork |
| `--progress-target <uri>` | | `""` | Target URI for streaming NDJSON progress telemetry |
| `--auto-install` | | `false` | Automatically install missing dependencies |
| `--verbose` | `-v` | `false` | Enable verbose logging |
| `--help` | `-h` | | Display command help |
| `--version` | | | Display version |

# License

MIT License (c) 2026 Alex Gorbatchev
