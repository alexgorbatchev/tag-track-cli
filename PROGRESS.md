# Out-of-Band Socket & Telemetry Protocol

`tag-track` supports streaming real-time JSON progress updates over UNIX domain sockets (`unix:///path/to.sock`), TCP sockets (`tcp://127.0.0.1:9099`), file descriptors (`fd://3`), standard output (`stdout`), or standard error (`stderr`).

## Progress Event Schema

Each line emitted is a standalone NDJSON object:

```json
{
  "timestamp": "2026-09-03T04:00:00Z",
  "type": "phase_start | metadata_found | artwork_downloaded | tagged | progress | complete | error",
  "phase": "identify | embed | complete",
  "step": 1,
  "total_steps": 1,
  "message": "identifying track metadata for Space X.m4a",
  "percent": 50.0,
  "metadata": {
    "title": "Space X",
    "artist": "Boris Brejcha",
    "album": "Level One",
    "genre": "Minimal Techno",
    "release_date": "2024-05-10",
    "release_year": "2024",
    "cover_art_url": "https://is1-ssl.mzstatic.com/image/thumb/Music123/v4/1400x1400bb.jpg",
    "source": "iTunes API"
  },
  "result": {
    "path": "tracks/Boris Brejcha - Space X.m4a",
    "title": "Space X",
    "artist": "Boris Brejcha",
    "album": "Level One",
    "release_year": "2024",
    "source": "iTunes API",
    "status": "ok"
  },
  "error": "optional error description"
}
```
