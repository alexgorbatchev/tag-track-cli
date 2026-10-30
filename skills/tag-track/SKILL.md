---
name: tag-track
description: Inspect, identify, and update audio metadata and cover artwork with tag-track.
author: alexgorbatchev
metadata:
  created_on: 2026-10-18 09:57
  last_modified: 2026-10-30 12:00
  status: current
---

## Workflow

Use `AGENT=1` for compact output. Inspect before changing an existing collection; preview identification with `--dry-run`, then apply the chosen overrides and destination.

```sh
AGENT=1 tag-track track inspect "tracks/Artist - Title.flac"
AGENT=1 tag-track track update --dry-run "tracks/Artist - Title.flac"
AGENT=1 tag-track track update -o tagged --artist "Artist" --title "Title" "tracks/input.flac"
AGENT=1 tag-track artwork embed --url "https://example.com/cover.jpg" "tracks/Artist - Title.flac"
```

## Commands and arguments

| Invocation | Behavior |
| --- | --- |
| `tag-track [file...]` | Identify and tag files; same operation as track update. With no arguments, read piped stdin or display help on a terminal. |
| `tag-track track` | Display track command help. |
| `tag-track track update [file...]` | Identify metadata, apply overrides, embed artwork, and write a clean Artist - Title filename. |
| `tag-track track inspect <file...>` | Read codecs, duration, tags, and artwork dimensions without changing audio files. |
| `tag-track artwork` | Display artwork command help. |
| `tag-track artwork embed <file...>` | Embed the supplied image, cropped and scaled to 1400x1400. |
| `tag-track dependencies` | Alias `deps`; check ffmpeg and ffprobe, each minimum 4.4. |
| `tag-track dependencies install [dep...]` | Install missing ffmpeg/ffprobe using the platform package manager; omission selects all. |
| `tag-track dependencies update [dep...]` | Update selected ffmpeg/ffprobe installations; omission selects all. |
| `tag-track self` | Display executable management help. |
| `tag-track self upgrade` | Download a GitHub release and replace the running executable. |
| `tag-track skill` | Print the offline Skills CLI installation command and repository skill URL. Takes no arguments or command-specific flags. |
| `tag-track help [command...]` | Display help for a command path. |
| `tag-track completion` | Display shell completion help. |
| `tag-track completion bash` | Write Bash completion script to stdout. |
| `tag-track completion zsh` | Write Zsh completion script to stdout. |
| `tag-track completion fish` | Write Fish completion script to stdout. |
| `tag-track completion powershell` | Write PowerShell completion script to stdout. |

Use `-` as the sole file argument to read newline-delimited paths from stdin. Input recognizes `DONE: path` and `output: path` records from acquisition output, preferring those over other lines. Quote paths containing spaces.

Dependency install aliases are `add` and `get`; dependency update also accepts `upgrade`.

## Flags

Root and `track update` accept the following local flags. Empty string means no explicit override except for `--comment`, where an explicitly empty value clears user text.

| Flag | Type | Default | Effect |
| --- | --- | --- | --- |
| `--out-dir`, `-o` | string | "" | Destination; empty uses the input file's directory. |
| `--in-place`, `-i` | bool | false | Use input directory and remove original when renamed. |
| `--artist` | string | "" | Artist override. |
| `--title` | string | "" | Title override. |
| `--album` | string | "" | Album override. |
| `--date` | string | "" | Release date/year, YYYY or YYYY-MM-DD. |
| `--genre` | string | "" | Genre override. |
| `--json` | bool | false | Emit one JSON result on stdout; diagnostics go to stderr. |
| `--comment` | string | "" | Omitted preserves existing user text and refreshes generated provenance; supplied replaces the entire comment; `--comment ""` clears it. |
| `--source-url` | string | "" | Audio source recorded in provenance. |
| `--artwork` | string | "" | HTTP image URL or local image path. Hidden alias: `--cover`. |
| `--refresh` | bool | false | Bypass metadata/artwork cache reads and store fresh results. `--no-cache` takes precedence and disables reads and writes. |
| `--dry-run` | bool | false | Resolve and print metadata without writing audio; network lookups and cache activity still occur. |
| `--no-cache` | bool | false | Disable metadata and artwork caching. |
| `--progress-target` | string | "" | NDJSON destination: unix:///path.sock, tcp://host:port, fd://3, stdout, stderr. |
| `--progress-socket` | string | "" | Hidden socket shorthand; explicit target takes precedence. |
| `--auto-install` | bool | false | Install missing external dependencies without a prompt. |

`artwork embed` accepts `--url` or `--artwork` (string, default ""), `--in-place` / `-i` (bool, default false), and `--out-dir` / `-o` (string, default ""). Supply an image URL or local image path through either artwork option.

Global flags: `--verbose` / `-v` (bool, default false) enables diagnostic logging; `--job-id` (string, default "") supplies the progress identifier, otherwise generated. Every command accepts `--help` / `-h` (bool, default false). Root `--version` (bool, default false) prints only the version and newline. Completion shells accept `--no-descriptions` (bool, default false) where offered by Cobra.

## Environment, output, and effects

- `AGENT=1`, `true`, or `yes` selects compact key-value output and disables spinners. Human mode uses labeled status lines. Help writes stdout; command errors write stderr. Agent help begins with the skill-reading alert.
- `TAG_TRACK_PROGRESS_TARGET` supplies the destination after explicit progress flags. Events contain job_id, monotonically increasing sequence starting at 1, UTC timestamp, type, phase, and batch percent 0–100. Metadata and result payloads carry resolved metadata and actual paths.
- `XDG_CACHE_HOME` selects cache storage; otherwise use the platform cache directory under tag-track. `XDG_DATA_HOME` selects the managed tool root, defaulting to ~/.local/share/tag-track/bin. `PATH` locates external tools. Platform package managers may request privileges during dependency installation.
- Printed filesystem paths under the home directory use `~`. Audio filesystem operations and telemetry paths retain their actual paths.
- Identification sends acoustic fingerprints to Shazam/AcoustID and metadata queries to iTunes/MusicBrainz. Artwork may download over HTTP. Audio is copied without lossy re-encoding; metadata writes can replace destination files. Temporary processing uses .tmp and cleans run-created temporary content.
- Per-file inspection, tagging, and artwork failures print errors and continue with the remaining files, then exit 1 if any file failed. Check every file's result for partial success. Command/dependency failures exit 1; interruption exits 130. Failed progress connection setup is best-effort and does not stop tagging.


Use `--json` for one structured result on stdout. Track operations return a batch of per-file results; failures set the batch status to error and exit nonzero. Dependencies and self upgrade return JSON envelopes. Diagnostics remain on stderr.
