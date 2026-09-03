---
created_on: 2026-09-03 04:00
last_modified: 2026-09-03 04:00
status: current
---

# tag-track-cli

Go CLI tool for inspecting, identifying, updating, and embedding normalized metadata and 1400x1400 square cover art into single audio tracks for DJ collections.

## Commands
- **Build Local Binary:** `just build` (compiles to `bin/tag-track`)
- **Run Standalone App:** `just run "<file...>"`
- **Inspect Audio Tags:** `just inspect "<file...>"`
- **Update Track Metadata:** `just update "<file...>"`
- **Run Tests:** `just test` (`go test -v ./...`)
- **Run Linter / Static Analysis:** `just vet` (`go vet ./...`)
- **Format Source Code:** `just fmt` (`go fmt ./...`)

## Setup & Environment
- **Prerequisites:** Go 1.26+, `ffmpeg`, `ffprobe`, `just`.
- **Output Directory:** In-place or destination specified by `-o <dir>` / `--out-dir`.
- **Temporary Files:** Temporary file operations use `.tmp/` within the project root.

## Conventions
- **Zero-Transcode Policy:** Never lossily re-encode MP3, OPUS, or FLAC streams to AAC. Always use `-c:a copy` during metadata tagging to preserve the original bitstream byte-for-byte.
- **Filename Sanitization:** Clean output filenames (`<outDir>/<Artist> - <Title>.<ext>`). Stripped of bracket video IDs `[...]` and illegal filesystem characters.
- **Hermetic Unit Tests:** All unit tests (`just test`) MUST remain 100% offline, hermetic, and deterministic. All HTTP network calls in tests use local mock transports, audio operations use synthetic test audio files, and zero unit tests depend on the live internet.
- **Output Formatting:** Plain text without emojis. Dual-mode output support via `AGENT=1`.
- **Help Screens:** CLI help output (`--help`) must display an aligned hierarchical tree view with `├─` and `╰─` glyphs powered by `github.com/alexgorbatchev/cobra-help-tree`.
- **Square Artwork Normalization:** Always ensure embedded album art is strictly 1:1 square (1400x1400) by applying center-crop and resize filters (`crop='min(iw,ih)':'min(iw,ih)',scale=1400:1400`).
- **Origin Date & Provenance Preservation:** Always preserve full release date (`YYYY-MM-DD`) and embed provenance comments (`Source: ... | Metadata: ... | Tagged: ...`).
- **Self-Upgrade:** `tag-track upgrade` checks for newer GitHub releases and upgrades in-place via `godeps.UpgradeSelf`.

## Boundaries
- **Always:** Run `just test` and `just vet` before committing changes.
- **Never:** Include YouTube video IDs or brackets in filenames.
- **Never:** Perform lossy-to-lossy audio re-encoding.
- **Never:** Commit compiled Go binaries (e.g. `bin/tag-track`) to git.
