---
name: mediaMovingTool
overview: Create a Go CLI application that scans a source directory for folders matching the pattern `showname.S##E##`, extracts the show name and season, then moves each folder to `shows/showname/S##/`, creating directories as needed and providing output of operations.
todos: []
---

# mediaMovingTool

## Overview

A Go CLI application that organizes TV show folders by scanning a source directory (e.g., `downloads/`) for folders matching the pattern `showname.S##E##`, then moves them to a structured destination (`shows/showname/S##/`).

## Implementation

### Files to Create

1. **`go.mod`** - Go module initialization with module name
2. **`main.go`** - Main application with CLI argument parsing, folder scanning, pattern matching, and file operations

### Core Functionality

1. **CLI Argument Parsing**: Accept source directory path as command-line argument (with sensible default like `./downloads`)
2. **Pattern Matching**: Use regex to match folder names like `showname.S01E20` or `showname.s01e20` (case-insensitive)

   - Extract: show name (e.g., `showname`), season (e.g., `S01`), episode (e.g., `E20`)

3. **Directory Operations**:

   - Check if destination `shows/{showName}/{season}/` exists
   - Create directory structure if it doesn't exist
   - Move the entire folder from source to destination

4. **Output**: Print operations performed (folders moved, directories created)

### Pattern Details

- Regex pattern: `^(.+?)\.([Ss]\d{1,2}[Ee]\d{1,2}).*$`
- Groups: 1 = show name, 2 = season/episode pattern (e.g., `S01E20`)
- Extract season prefix (e.g., `S01`) from the season/episode match

### Error Handling

- Handle missing source directories
- Handle permission errors
- Skip folders that don't match the pattern
- Handle duplicate folder names in destination

### Example Flow

```
Input: downloads/GameOfThrones.S01E01.mkv.folder/
Parse: showName="GameOfThrones", season="S01"
Check: shows/GameOfThrones/S01/ exists? No → Create
Move: downloads/GameOfThrones.S01E01.mkv.folder → shows/GameOfThrones/S01/GameOfThrones.S01E01.mkv.folder
Output: "Moved GameOfThrones.S01E01.mkv.folder to shows/GameOfThrones/S01/"
```
