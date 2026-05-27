# mediaMovingTool

A Go CLI that organizes TV shows and movies from configured source directories into a library layout. **Requires `config.json`.** Shows are parsed from folder/file names, optionally matched to existing library folders, and placed under `showsDestDir/{show}/{season}/`. Movies are moved flat into `moviesDestDir` with optional single-file wrapping. Unmatched show items can be handled interactively at the end of the run.

There is **no** separate “other” destination folder — items that do not match a show pattern stay in the shows source until you act on them via the unsure-items prompt (or leave them in place).

## Features

- **TV shows**: Dot-style and release-style name parsing; two-pass wrap-then-move
- **Show library matching**: Reuse existing folders in `showsDestDir` (exact, normalized, fuzzy, dot-series year stripping)
- **Interactive prompts**: Multiple show-folder matches, fuzzy merge Y/N, season-pack flatten, end-of-run unsure items
- **Season packs**: Dot (`Show.Name.S01.…`) and release (`My Show (2020) S01 …`) season-only detection; optional flatten into season folder
- **Movies**: No pattern matching; wrap single files, move folders/files flat to destination
- **Duplicates (shows)**: Same episode or season pack in dest → move source entry to `showsSourceDir/dupe/`
- **Duplicates (movies)**: Exact destination path exists → skip with warning
- **Extension stripping**: Media extensions removed when wrapping single files (shows, movies, and unsure-item moves)
- **Case-insensitive seasons**: Reuse existing season folder if case differs (`s01` vs `S01`)
- **`.ignore` and system files**: Skipped automatically
- **`devMode`**: Preview all operations; print prompts without stdin; no file moves or directory creation

## Installation

1. [Go 1.21+](https://golang.org/dl/)
2. Build:

```bash
go build -o mediaMovingTool      # macOS/Linux
go build -o mediaMovingTool.exe    # Windows
```

## Configuration

Copy `config.json.example` to `config.json` and edit:

```bash
cp config.json.example config.json
```

| Field | Description |
|-------|-------------|
| `showsSourceDir` | Incoming TV content (required with `showsDestDir` for shows) |
| `showsDestDir` | TV library root — existing show folders here are used for matching |
| `moviesSourceDir` | Incoming movies (required with `moviesDestDir` for movies) |
| `moviesDestDir` | Movie library (flat layout) |
| `devMode` | `true` = test/preview mode (default `false`) |

**Legacy:** `sourceDir` / `destDir` map to `showsSourceDir` / `showsDestDir` if the new fields are empty.

At least one pair (shows or movies) must be configured or the program exits.

**Custom config path:**

```bash
./mediaMovingTool -config /path/to/config.json
```

## Usage

```bash
./mediaMovingTool
```

Processing order: TV shows (if configured) → movies (if configured) → summary → **unsure items** prompt (shows only).

## Naming conventions

Patterns are tried in order; ambiguous names (multiple patterns disagree) are not auto-processed.

### Dot style

| Pattern | Example | Parsed show folder | Season |
|---------|---------|-------------------|--------|
| Episode | `Show.Name.S01E12` | `Show.Name` | `S01` |
| Episode + tags | `Show.Name.S02E14.1080p.mkv` | `Show.Name` | `S02` |
| Season pack | `Series.S01.1080p.WEB-DL` | `Series` | `S01` |

- `S##E##` and `s##e##` are case-insensitive.
- Text after the episode or season token is allowed (quality, group, etc.).
- Parsing uses the basename with **media extensions stripped** first (e.g. `.mkv`, `.mp4`).

### Release style (spaces / optional year)

| Pattern | Example | Parsed show folder | Season |
|---------|---------|-------------------|--------|
| Year + episode | `My Show (2020) S02E08 …` | `My Show (2020)` | `S02` |
| Episode, no year | `My Show S02E08 …` | `My Show` | `S02` |
| Year + season pack | `My Show (2024) S01 (1080p …)` | `My Show (2024)` | `S01` |
| Season pack, no year | `My Show S01 …` | `My Show` | `S01` |

Release-style show folders **keep `(year)` in the name** when present.

### Destination layout (shows)

```
showsDestDir/
  Show.Name/
    S01/
      Show.Name.S01E12/          # whole folder moved here
        Show.Name.S01E12.mkv
```

## TV show processing flow

1. **Pass 1 — wrap files**: Single files whose names match a show pattern are wrapped in a folder named from the basename **without media extensions** (e.g. `Show.Name.S01E12.mkv` → folder `Show.Name.S01E12/`).
2. **Pass 2 — move folders**: Each matching folder is moved to `showsDestDir/{show}/{season}/{folderName}/`.
3. **Non-matching** files/folders are left in the source and listed in **unsure items** after the summary; if the user moves a media file via that prompt it is wrapped in a folder (extension stripped) at the destination.
4. **Reserved names** in the shows source are never processed: `.ignore`, `dupe`.

Before creating show or season directories, the tool searches **`showsDestDir`** (the library), not the source.

## Show folder matching (`showsDestDir`)

Matching uses parsed `ShowName` against **existing folders** under `showsDestDir`.

### Dot-series year stripping

For dot-style names with an embedded year segment (e.g. `Series.Name.2024.S05E06` → parsed show `Series.Name.2024`):

- Canonical base: `Series.Name` (year removed for lookup only).
- If **`showsDestDir/Series.Name`** already exists (case-insensitive) → use that folder.
- If it does **not** exist → create/use the **parsed** name (e.g. `Series.Name.2024`).

This rule only applies when the year-stripped base folder is already in the library.

### Normalized keys (auto-merge, same style only)

| Style | Normalization | Example keys |
|-------|---------------|--------------|
| Dot | Lowercase; strip embedded `.YYYY.`; keep dots | `Show.Name` → `show.name` |
| Release | Lowercase; keep spaces and `(year)` | `My Show (2020)` → `my show (2020)` |

- Dot vs release (e.g. `Show.Name` vs `My Show (2020)`) → **not** auto-merged on a shared key.
- `My Show (2020)` vs `My Show` → **not** auto-merged (year kept in release key).

### Fuzzy match (≥ 85% similar)

One cross-style candidate → Y/N prompt. Several candidates → numbered choice.

**`devMode: true`**

- Prints `[TEST]` lines for what would be prompted.
- Does **not** read stdin.
- Dot-series: logs if an existing base folder would be used; preview paths keep the **parsed** show name.
- Fuzzy / multi-match: does not merge; uses parsed name for preview moves.
- Season flatten: logs prompt; does not flatten.
- Unsure: lists items and choices only; nothing moved.

### Prompt examples

**Multiple matches:**

```text
Multiple show folders may match "My Show S02":
  1) Show.Name (87%)
  2) My Show (2020) (same series (normalized))
  0) Create new folder "My Show S02"
Choice [0-2]:
```

**Single fuzzy match:**

```text
Show.Name exists — move "My Show (2020)" into that folder? [Y/N]:
```

**Dot-series (non-dev):**

```text
Using existing show folder: Show.Name (dot-series match for Show.Name.2024)
```

## Season packs

**Season-only** entries (no episode): dot `Series.S01.…` or release `My Show (2020) S01 …`.

Default: move the whole pack folder to `showsDestDir/{show}/{season}/{packFolder}/`.

### Flatten prompt

Offered when **any** of:

- Destination season folder already exists
- Pack contains subdirectories (nested structure)
- Pack contains multiple media files, or one file in a nested path

```text
Season pack: Series.S01.1080p
Unpack all files directly into Show.Name/S01/ (not in a subfolder)? [Y/N]:
```

- **Y** — all media files under the pack are moved **flat** into the season folder (name collisions get `_1`, `_2`, … suffixes); empty pack dirs removed.
- **N** — standard folder move (whole pack as one subdirectory).

Duplicate season pack or episode in dest → source moved to `showsSourceDir/dupe/` (see below).

## Unsure items (end of run)

After the summary, remaining shows-source entries that were **not processed**:

- No pattern matched
- Ambiguous pattern (multiple parsers disagreed)

```text
=== Unsure items ===
  1. [shows] Some.Random.Folder — no show pattern matched
```

Per item (interactive, not in `devMode`):

1. Move to `showsDestDir` — media files are wrapped in a folder (extension stripped); folders move as-is
2. Move to `moviesDestDir` — same wrapping behaviour; unavailable if not configured
3. Move to `showsSourceDir/dupe/` (flat, no wrapping)
4. Skip (leave in source)

## Movies

- **No** show-style pattern matching.
- **Pass 1**: Every non-ignored file in `moviesSourceDir` is wrapped: `Movie.Title.2024.1080p.mkv` → folder `Movie.Title.2024.1080p/`.
- **Pass 2**: Every top-level folder (including wrapped) is moved to `moviesDestDir/{sameName}/` (flat).
- If `moviesDestDir/{name}` already exists → **warning and skip** (not moved to dupe).
- Skips: `.ignore`, system/hidden files, paths inside `.ignore`.

## Dupe folder (shows)

When the same **episode** (`S01E22` in any folder name under the season) or **season pack** (season already present in dest) is detected:

- Source folder is moved to **`showsSourceDir/dupe/`** (created if needed).
- `dupe` in the shows source is never scanned for new content.

Movies do not use dupe for duplicates; they are skipped with a warning.

## `devMode` (test mode)

Set `"devMode": true` in `config.json`:

- All moves/creates/wraps print `[TEST] Would …` / `[TEST] …`.
- Summary includes `[TEST MODE - No actual changes were made]`.
- Interactive prompts are described but not executed; no stdin for show matching or unsure items.
- Wrapped show folders that only exist as previews are still evaluated in pass 2 (test-only path).

## Ignored paths and files

- **`.ignore`**: Any path under a directory named `.ignore` is skipped (shows and movies).
- **System files**: `.DS_Store`, `Thumbs.db`, `desktop.ini`, `._*`, `~$*`, and other built-in patterns (see `shouldIgnoreFile` in `main.go`).

## Examples

### Shows — basic

**Source:**

```
downloads/shows/
  Show.Name.S01E01/
  Show.Name.S01E02/
```

**Config:** `showsSourceDir`, `showsDestDir`, `devMode: false`

**Library:**

```
shows/
  Show.Name/
    S01/
      Show.Name.S01E01/
      Show.Name.S01E02/
```

### Shows — wrap single file

`Series.S01E05.mkv` → wrap `Series.S01E05/` → `shows/Series/S01/Series.S01E05/`.

### Shows — duplicate episode

Dest already has `…/S01/Show.Name.S01E22.1080p/`. New `Show.Name.S01E22.720p/` → `showsSourceDir/dupe/Show.Name.S01E22.720p/`.

### Shows — dot-series year

Library has `Show.Name/`. Incoming `Show.Name.2024.S05E06` → uses `Show.Name/`.  
If `Show.Name/` does **not** exist → new folder `Show.Name.2024/`.

### Movies

**Source:**

```
downloads/movies/
  Movie.Title.2024/
  Another.Film.2020.mkv
```

**Result:**

```
movies/
  Movie.Title.2024/
  Another.Film.2020/
    Another.Film.2020.mkv
```

### `devMode` output (excerpt)

```
=== Processing TV Shows ===
[TEST] Would create directory: ./shows/Show.Name/S01
[TEST] Would move: ./downloads/shows/Show.Name.S01E01 -> ./shows/Show.Name/S01/Show.Name.S01E01

=== Summary ===
[TEST MODE - No actual changes were made]
```

## Error handling

- Missing or invalid `config.json` → exit
- Missing source directory → error for that section
- No shows/movies configured → exit
- Per-item failures → log and continue where possible

## Build from source

```bash
go build -o mediaMovingTool .
go test ./...
go run .
```

## Requirements

- Go 1.21+
- Windows, Linux, or macOS

## License

Provided as-is for personal use.
