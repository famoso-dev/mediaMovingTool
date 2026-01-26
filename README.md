# mediaMovingTool

A Go CLI application that automatically organizes TV show folders and movies. **Requires a configuration file (config.json) to run.** For TV shows, it scans a source directory for folders and files matching the pattern `showname.S##E##`, automatically wrapping single files in folders when needed, and moves them to a structured destination directory. For movies, it automatically wraps single files in folders (using filename without extension) before moving them to the destination, preserving structure.

## Features

- **TV Show Organization**: Automatically detects TV show folders and files with format `showname.S##E##` (case-insensitive) and organizes them into `shows/{showName}/{season}/`
- **Single File Processing**: Automatically wraps single files matching the pattern (e.g., `myshow.s01e15.mkv`) in folders before processing
- **Movie Organization**: Simple folder/file moving for movies without pattern matching, preserving source structure
- **Movie File Wrapping**: Automatically wraps single movie files in folders (using filename without extension) before processing
- **Dual Mode Support**: Process both TV shows and movies in a single run with separate directories
- **Pattern Matching**: Automatically detects TV show folders and files with format `showname.S##E##` (case-insensitive)
- **Duplicate Episode Detection**: Checks if an episode with the same season/episode number already exists in the destination. If found, moves the duplicate to `SourceDir/dupe/` instead of skipping it
- **Case-Insensitive Directory Matching**: On macOS/Linux, detects and reuses existing season directories with different case (e.g., `s01` vs `S01`) to prevent duplicate folders
- **Automatic Organization**: Moves folders to structured destination for shows, flat structure for movies
- **Directory Creation**: Automatically creates destination directories if they don't exist
- **Ignore Folder Support**: Automatically skips any files or folders inside `.ignore` directories
- **System File Ignoring**: Automatically skips common system files (`.DS_Store`, `Thumbs.db`, etc.)
- **Test Mode**: Preview operations without actually moving files or creating directories
- **Detailed Output**: Shows all operations performed with a summary at the end

## Installation

1. Ensure you have [Go 1.21 or later](https://golang.org/dl/) installed
2. Clone or download this project
3. Build the application:
   ```bash
   # Windows
   go build -o mediaMovingTool.exe
   
   # macOS/Linux
   go build -o mediaMovingTool
   ```

## Usage

### Configuration File Required

**The application requires a `config.json` file to run.** Create this file before running the application.

### Basic Usage

**Running the application:**
```bash
# Windows
.\mediaMovingTool.exe

# macOS/Linux
./mediaMovingTool
```

The application will read configuration from `config.json` in the same directory as the executable.

**To specify a different config file location:**
```bash
# Windows
.\mediaMovingTool.exe -config "C:\path\to\config.json"

# macOS/Linux
./mediaMovingTool -config "/path/to/config.json"
```

### Test Mode / Dev Mode

Test mode allows you to preview what the application would do without actually moving files or creating directories. Enable it by setting `devMode: true` in your config file:

```json
{
  "showsSourceDir": "./downloads/shows",
  "showsDestDir": "./shows",
  "moviesSourceDir": "./downloads/movies",
  "moviesDestDir": "./movies",
  "devMode": true
}
```

With `devMode: true` in your config file, the application will run in test mode and preview all operations without making changes.

### Configuration File

**The configuration file (`config.json`) is required.** The application will exit with an error if the config file is missing or invalid.

**Config file location:** `config.json` (in the same directory as the executable)

**Example config.json:**
```json
{
  "showsSourceDir": "./downloads/shows",
  "showsDestDir": "./shows",
  "moviesSourceDir": "./downloads/movies",
  "moviesDestDir": "./movies",
  "devMode": false
}
```

To get started, copy `config.json.example` to `config.json` and modify the values as needed:
```bash
# Windows
copy config.json.example config.json

# macOS/Linux
cp config.json.example config.json
```

**Config file fields:**
- `showsSourceDir`: Source directory to scan for TV show folders (required if processing shows)
- `showsDestDir`: Base destination directory for organized shows (required if processing shows)
- `moviesSourceDir`: Source directory to scan for movies (required if processing movies)
- `moviesDestDir`: Destination directory for movies (required if processing movies)
- `devMode`: Enable dev mode, which acts like test mode (default: `false`)

**Backward Compatibility:** The old `sourceDir` and `destDir` fields are still supported and will be mapped to `showsSourceDir` and `showsDestDir` respectively.

**Important Notes:**
- The config file is **required** - the application will not run without it
- If the config file doesn't exist, the application will exit with an error
- If the config file has errors, the application will exit with an error
- You must configure at least one of: shows (showsSourceDir + showsDestDir) or movies (moviesSourceDir + moviesDestDir)
- If neither shows nor movies are configured, the application will exit with an error

## Examples

### Example 1: Basic Organization

**Before:**
```
downloads/
  ├── GameOfThrones.S01E01/
  ├── GameOfThrones.S01E02/
  ├── BreakingBad.S02E05/
  └── .ignore/
      └── temp.S01E99/
```

**Config.json:**
```json
{
  "showsSourceDir": "./downloads",
  "showsDestDir": "./shows",
  "devMode": false
}
```

**After running:**
```bash
.\mediaMovingTool.exe
```

**Result:**
```
shows/
  ├── GameOfThrones/
  │   └── S01/
  │       ├── GameOfThrones.S01E01/
  │       └── GameOfThrones.S01E02/
  └── BreakingBad/
      └── S02/
          └── BreakingBad.S02E05/

downloads/
  └── .ignore/
      └── temp.S01E99/  (skipped, not moved)
```

Note: The folder `temp.S01E99` inside `.ignore` was automatically skipped and not moved.

### Example 2: Movie Organization

**Before:**
```
downloads/movies/
  ├── The.Matrix.1999/
  ├── Inception.2010/
  ├── Interstellar.2014.mkv
  └── .ignore/
      └── incomplete.mkv
```

**Config.json:**
```json
{
  "moviesSourceDir": "./downloads/movies",
  "moviesDestDir": "./movies",
  "devMode": false
}
```

**After running:**
```bash
.\mediaMovingTool.exe
```

**Result:**
```
movies/
  ├── The.Matrix.1999/
  ├── Inception.2010/
  └── Interstellar.2014/
      └── Interstellar.2014.mkv

downloads/movies/
  └── .ignore/
      └── incomplete.mkv  (skipped, not moved)
```

The single file `Interstellar.2014.mkv` was automatically wrapped in a folder `Interstellar.2014/` before being moved to the destination. The file `incomplete.mkv` inside `.ignore` was automatically skipped.

**Output:**
```
=== Processing Movies ===
Wrapped movie file in folder: Interstellar.2014.mkv -> Interstellar.2014/
Moved folder: ./downloads/movies/The.Matrix.1999 -> ./movies/The.Matrix.1999
Moved folder: ./downloads/movies/Inception.2010 -> ./movies/Inception.2010
Moved folder: ./downloads/movies/Interstellar.2014 -> ./movies/Interstellar.2014
```

### Example 3: Processing Both Shows and Movies

**Config.json:**
```json
{
  "showsSourceDir": "./downloads/shows",
  "showsDestDir": "./shows",
  "moviesSourceDir": "./downloads/movies",
  "moviesDestDir": "./movies",
  "devMode": false
}
```

**Running:**
```bash
.\mediaMovingTool.exe
```

The application will process both TV shows and movies in a single run, showing separate sections for each.

### Example 4: Single File Processing

**Scenario**: You have a single file `myshow.s01e15.mkv` in your downloads folder that needs to be organized.

**Before:**
```
downloads/
  └── myshow.s01e15.mkv
```

**Config.json:**
```json
{
  "showsSourceDir": "./downloads",
  "showsDestDir": "./shows",
  "devMode": false
}
```

**After running:**
```bash
.\mediaMovingTool.exe
```

**Result:**
```
shows/
  └── myshow/
      └── S01/
          └── myshow.s01e15/
              └── myshow.s01e15.mkv
```

The single file was automatically wrapped in a folder (`myshow.s01e15/`) and then processed normally through the organization system.

**Output:**
```
=== Processing TV Shows ===
Wrapped file in folder: myshow.s01e15.mkv -> myshow.s01e15/
Created directory: ./shows/myshow/S01
Moved: ./downloads/myshow.s01e15 -> ./shows/myshow/S01/myshow.s01e15
```

### Example 5: Duplicate Episode Detection

**Scenario**: You have `GameOfThrones.S01E22.720p` in your downloads folder, but `GameOfThrones.S01E22.1080p` already exists in the destination.

**Before:**
```
downloads/
  └── GameOfThrones.S01E22.720p/

shows/
  └── GameOfThrones/
      └── S01/
          └── GameOfThrones.S01E22.1080p/
```

**Config.json:**
```json
{
  "showsSourceDir": "./downloads",
  "showsDestDir": "./shows",
  "devMode": false
}
```

**After running:**
```bash
.\mediaMovingTool.exe
```

**Result:**
```
downloads/
  └── dupe/
      └── GameOfThrones.S01E22.720p/

shows/
  └── GameOfThrones/
      └── S01/
          └── GameOfThrones.S01E22.1080p/
```

The duplicate episode (S01E22) was detected and moved to `downloads/dupe/` instead of being skipped. The dupe directory is automatically created if it doesn't exist.

**Output:**
```
=== Processing TV Shows ===
Duplicate episode detected (S01E22), moved to dupe: ./downloads/GameOfThrones.S01E22.720p -> ./downloads/dupe/GameOfThrones.S01E22.720p
```

### Example 6: Test Mode Preview

**Config.json:**
```json
{
  "showsSourceDir": "./downloads",
  "showsDestDir": "./shows",
  "devMode": true
}
```

**Running:**
```bash
.\mediaMovingTool.exe
```

**Output:**
```
=== Processing TV Shows ===
[TEST] Would create directory: ./shows/GameOfThrones/S01
[TEST] Would move: ./downloads/GameOfThrones.S01E01 -> ./shows/GameOfThrones/S01/GameOfThrones.S01E01

=== Processing Movies ===
[TEST] Would move folder: ./downloads/movies/The.Matrix.1999 -> ./movies/The.Matrix.1999

=== Summary ===
[TEST MODE - No actual changes were made]
Directories created: 1
Items moved: 2

Created directories:
  - ./shows/GameOfThrones/S01

Moved items:
  - ./downloads/GameOfThrones.S01E01 -> ./shows/GameOfThrones/S01/GameOfThrones.S01E01
  - ./downloads/movies/The.Matrix.1999 -> ./movies/The.Matrix.1999
```

## Pattern Matching

The application matches folders and files with the following pattern:
- Format: `{showName}.S##E##`
- Case-insensitive: Works with `S01E20`, `s01e20`, `S01e20`, etc.
- Flexible: Supports additional text after the episode number (e.g., `showname.S01E20.mkv.folder`, `showname.S01E20.mkv`)

**Examples of matching folder names:**
- `GameOfThrones.S01E01`
- `breaking.bad.s02e05`
- `TheOffice.S03E12.mkv`
- `stranger.things.S01E08.1080p`

**Examples of matching file names:**
- `myshow.s01e15.mkv`
- `TheOffice.S03E12.mp4`
- `breaking.bad.s02e05.avi`

**Note**: Single files matching the pattern are automatically wrapped in folders (using the filename without extension as the folder name) before being processed.

## Behavior

### TV Shows Mode
- **Pattern Matching**: Processes folders and files matching `showname.S##E##` pattern
- **Single File Processing**: Automatically wraps single files in folders (using filename without extension) before processing
- **Two-Pass Processing**: 
  1. First pass: Wraps matching single files in folders
  2. Second pass: Processes all folders (including newly wrapped ones) normally
- **Ignore Folder Support**: Skips any files or folders inside `.ignore` directories
- **Directory Creation**: Creates `shows/{showName}/{season}/` if it doesn't exist
- **Case-Insensitive Directory Matching**: On case-sensitive filesystems (macOS/Linux), checks for existing season directories with different case and reuses them (e.g., if `s01` exists, uses it instead of creating `S01`)
- **Moving Folders**: Moves entire folders from source to destination
- **Organization**: Organizes shows by name and season

### Movies Mode
- **No Pattern Matching**: Processes all folders and files in the source directory
- **Single File Processing**: Automatically wraps single files in folders (using filename without extension) before processing
- **Two-Pass Processing**: 
  1. First pass: Wraps single files in folders
  2. Second pass: Processes all folders (including newly wrapped ones)
- **Ignore Folder Support**: Skips any files or folders inside `.ignore` directories
- **Directory Creation**: Creates destination directory if it doesn't exist
- **Moving Items**: Moves folders from source to destination (files are wrapped first)
- **Structure Preservation**: Preserves the source folder structure (flat move)

### General Behavior
- **Dual Processing**: Can process both TV shows and movies in a single run
- **Ignore Folder Support**: Automatically skips files and folders inside `.ignore` directories
- **Duplicate Detection**: 
  - **TV Shows**: Checks if an episode with the same season/episode pattern (e.g., S01E22) already exists in the destination season directory. If found, moves the duplicate to `SourceDir/dupe/` folder
  - **Movies**: Skips items if the exact destination already exists
- **Case-Insensitive Directory Handling**: On macOS/Linux, prevents duplicate season folders by detecting and reusing existing directories with different case
- **Error Handling**: Continues processing other items if one fails
- **Test Mode**: Preview all operations without making changes

## Ignore Folder Support

The application automatically skips any files or folders inside `.ignore` directories:

- **Automatic Detection**: Any directory named `.ignore` (case-sensitive) is automatically skipped
- **Recursive Skipping**: Files and folders inside `.ignore` directories are skipped at any nesting level
- **Works for Both Modes**: Applies to both TV shows and movies processing
- **Example**: If you have `sourceDir/.ignore/temp.mkv`, it will be skipped during processing

**Use Cases:**
- Skip temporary files or folders
- Exclude test data from processing
- Ignore files that are still downloading or incomplete

**Note**: The `.ignore` folder itself is also skipped, so it won't be moved to the destination.

## System File Ignoring

The application automatically skips common system files and hidden files that should not be processed:

**Ignored Files:**
- `.DS_Store` - macOS Finder metadata files
- `Thumbs.db` - Windows thumbnail cache files
- `desktop.ini` - Windows folder customization files
- `._*` - macOS resource fork files (any file starting with `._`)
- `~$*` - Temporary files (any file starting with `~$`)
- `.Spotlight-V100` - macOS Spotlight index
- `.Trashes` - macOS Trash folder
- `.fseventsd` - macOS file system events
- `.VolumeIcon.icns` - macOS volume icon
- `.com.apple.*` - macOS Apple system files
- `$RECYCLE.BIN` - Windows Recycle Bin folder
- `System Volume Information` - Windows system folder

**Behavior:**
- System files are automatically skipped during processing
- Applies to both TV shows and movies processing
- No configuration needed - works automatically
- Prevents errors when encountering system files

**Note**: These files are skipped silently - they won't cause errors or warnings during processing.

## Error Handling

The application handles various error conditions:
- **Missing config file** (exits with error - config file is required)
- **Invalid config file** (exits with error - must be valid JSON)
- **Missing source directory** (exits with error)
- **Permission errors** (logs error and continues)
- **Duplicate episodes** (TV shows: moves to `SourceDir/dupe/`, Movies: skips with warning)
- **Duplicate exact names** (skips with warning)
- **Invalid folder names** (skips non-matching folders)
- **Case-insensitive directory matching errors** (falls back to creating new directory)
- **No directories configured** (exits with error - must configure at least shows or movies)

## Case-Insensitive Directory Matching

On case-sensitive filesystems (macOS/Linux), the application intelligently handles existing season directories with different case:

- **Problem**: If a show already has an `s01` folder, creating a new `S01` folder would result in duplicate season folders
- **Solution**: The application checks for case-insensitive matches before creating directories
- **Behavior**: If `s01` exists, it will use that directory instead of creating `S01`
- **Output**: When reusing an existing directory with different case, a message is displayed: `Using existing directory: .../s01 (instead of .../S01)`

This ensures consistent organization and prevents duplicate season folders on macOS and Linux systems.

## Single File Processing

The application automatically handles single files that match the TV show pattern:

- **Detection**: Files matching `showname.S##E##` pattern are automatically detected
- **Wrapping**: Files are wrapped in folders using the filename without extension (e.g., `myshow.s01e15.mkv` → folder `myshow.s01e15/`)
- **Processing**: Wrapped folders are then processed normally through the organization system
- **Example**: If `myshow.s01e15.mkv` exists in the source, it will be wrapped in `myshow.s01e15/` folder, then moved to `shows/myshow/S01/myshow.s01e15/`

**Note**: Files are processed before folders, ensuring wrapped files are included in the normal processing flow including duplicate detection.

## Movie File Wrapping

The application automatically wraps single movie files in folders before processing:

- **Detection**: Any single file found in the movie source directory is automatically detected
- **Wrapping**: Files are wrapped in folders using the filename without extension (e.g., `The.Matrix.1999.mkv` → folder `The.Matrix.1999/`)
- **Processing**: Wrapped folders are then processed normally and moved to the destination
- **Example**: If `Interstellar.2014.mkv` exists in the source, it will be wrapped in `Interstellar.2014/` folder, then moved to `movies/Interstellar.2014/`

**Note**: Unlike TV shows which require pattern matching, movies wrap any single file found in the source directory. Files are processed before folders, ensuring wrapped files are included in the normal processing flow.

## Duplicate Episode Detection

The application automatically detects duplicate TV show episodes based on season/episode patterns:

- **Detection Method**: Scans the destination season directory for any folders containing the same season/episode pattern (e.g., S01E22)
- **Case-Insensitive**: Matches are case-insensitive, so `S01E22`, `s01e22`, and `S01e22` are all considered the same episode
- **Dupe Folder**: When a duplicate is detected, the source folder (or wrapped file folder) is moved to `SourceDir/dupe/` instead of being skipped
- **Automatic Creation**: The dupe directory is automatically created if it doesn't exist
- **Example**: If `GameOfThrones.S01E22.1080p` already exists in `shows/GameOfThrones/S01/`, then a new folder `GameOfThrones.S01E22.720p` (or wrapped file `GameOfThrones.S01E22.720p.mkv`) will be moved to `downloads/dupe/GameOfThrones.S01E22.720p/`

**Note**: The duplicate detection checks for the season/episode pattern, not exact folder names. So different releases of the same episode (e.g., 720p vs 1080p) are recognized as duplicates. This applies to both folders and files (after wrapping).

## Build from Source

```bash
# Windows - Build executable
go build -o mediaMovingTool.exe

# macOS/Linux - Build executable
go build -o mediaMovingTool

# Run without building (all platforms)
go run main.go

# Run with test mode (set devMode: true in config.json)
go run main.go
```

## Requirements

- Go 1.21 or later
- Windows, Linux, or macOS

## License

This project is provided as-is for personal use.
