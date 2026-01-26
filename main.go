package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config represents the application configuration
type Config struct {
	// Legacy fields for backward compatibility
	SourceDir string `json:"sourceDir"`
	DestDir   string `json:"destDir"`

	// TV Show directories
	ShowsSourceDir string `json:"showsSourceDir"`
	ShowsDestDir   string `json:"showsDestDir"`

	// Movie directories
	MoviesSourceDir string `json:"moviesSourceDir"`
	MoviesDestDir   string `json:"moviesDestDir"`

	DevMode bool `json:"devMode"`
}

// loadConfig loads configuration from config.json - config file is required
func loadConfig(configPath string) (*Config, error) {
	config := &Config{
		SourceDir:       "",
		DestDir:         "",
		ShowsSourceDir:  "",
		ShowsDestDir:    "",
		MoviesSourceDir: "",
		MoviesDestDir:   "",
		DevMode:         false,
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file '%s' does not exist - configuration file is required", configPath)
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	// Parse JSON config
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	// Backward compatibility: if old sourceDir/destDir exist but new ones don't, map them
	if config.SourceDir != "" && config.ShowsSourceDir == "" {
		config.ShowsSourceDir = config.SourceDir
	}
	if config.DestDir != "" && config.ShowsDestDir == "" {
		config.ShowsDestDir = config.DestDir
	}

	return config, nil
}

// ProcessResult holds the results of processing operations
type ProcessResult struct {
	CreatedDirs []string
	MovedItems  []string
	MovedCount  int
}

// isInIgnoreFolder checks if a path is inside a .ignore folder
// Returns true if any part of the path contains a directory named .ignore
func isInIgnoreFolder(path string) bool {
	// Split path into components
	parts := strings.Split(filepath.ToSlash(path), "/")
	
	// Check if any part is .ignore
	for _, part := range parts {
		if part == ".ignore" {
			return true
		}
	}
	
	return false
}

// shouldIgnoreFile checks if a file should be ignored based on its name
// Returns true for system files, hidden files, and other files that should be skipped
func shouldIgnoreFile(fileName string) bool {
	// Common system and hidden files to ignore
	ignorePatterns := []string{
		".DS_Store",           // macOS Finder metadata
		"Thumbs.db",          // Windows thumbnail cache
		"desktop.ini",        // Windows folder customization
		"._",                 // macOS resource fork files (starts with ._)
		"~$",                 // Temporary files (starts with ~$)
		".Spotlight-V100",    // macOS Spotlight index
		".Trashes",           // macOS Trash
		".fseventsd",         // macOS file system events
		".VolumeIcon.icns",   // macOS volume icon
		".com.apple.",        // macOS Apple system files
		"$RECYCLE.BIN",       // Windows Recycle Bin
		"System Volume Information", // Windows system folder
	}
	
	// Check if filename matches any ignore pattern
	for _, pattern := range ignorePatterns {
		if strings.HasPrefix(fileName, pattern) || fileName == pattern {
			return true
		}
	}
	
	return false
}

// findCaseInsensitiveDir checks if a directory exists with case-insensitive matching
// Returns the actual directory name if found, or the desired name if not found
func findCaseInsensitiveDir(parentDir, desiredName string) (string, error) {
	// First check if exact match exists
	exactPath := filepath.Join(parentDir, desiredName)
	if info, err := os.Stat(exactPath); err == nil && info.IsDir() {
		return desiredName, nil
	}

	// Read parent directory to check for case-insensitive matches
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return desiredName, err
	}

	desiredLower := strings.ToLower(desiredName)
	for _, entry := range entries {
		if entry.IsDir() && strings.ToLower(entry.Name()) == desiredLower {
			// Found a case-insensitive match, return the actual name
			return entry.Name(), nil
		}
	}

	// No match found, return desired name
	return desiredName, nil
}

// checkEpisodeExists checks if any folder with the same season/episode pattern already exists
// in the destination season directory. Returns true if a duplicate is found, false otherwise.
func checkEpisodeExists(destPath, seasonEpisode string) (bool, error) {
	// If destination directory doesn't exist yet, no duplicate can exist
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return false, nil
	}

	// Read all entries in the destination season directory
	entries, err := os.ReadDir(destPath)
	if err != nil {
		return false, err
	}

	// Normalize the season/episode pattern to uppercase for case-insensitive comparison
	seasonEpisodeUpper := strings.ToUpper(seasonEpisode)

	// Pattern to match season/episode in folder names (case-insensitive)
	// Matches patterns like S01E22, s01e22, S01e22, etc.
	pattern := regexp.MustCompile(`([Ss]\d{1,2}[Ee]\d{1,2})`)

	// Check each entry in the destination directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue // Skip files, only check directories
		}

		folderName := entry.Name()
		// Find all season/episode patterns in the folder name
		matches := pattern.FindAllStringSubmatch(folderName, -1)
		for _, match := range matches {
			if len(match) > 0 {
				// Normalize to uppercase for case-insensitive comparison
				matchUpper := strings.ToUpper(match[0])
				if matchUpper == seasonEpisodeUpper {
					// Found a duplicate episode
					return true, nil
				}
			}
		}
	}

	// No duplicate found
	return false, nil
}

// wrapFileInFolder wraps a single file in a folder with the filename (without extension)
// Returns the created folder name, or error if operation fails
func wrapFileInFolder(sourceDir, fileName string, testMode bool) (string, error) {
	// Get file extension to remove it from folder name
	ext := filepath.Ext(fileName)
	folderName := strings.TrimSuffix(fileName, ext)
	
	// Build paths
	filePath := filepath.Join(sourceDir, fileName)
	folderPath := filepath.Join(sourceDir, folderName)
	
	// Check if folder already exists
	if _, err := os.Stat(folderPath); err == nil {
		return folderName, fmt.Errorf("folder already exists: %s", folderPath)
	}
	
	// Create folder and move file
	if testMode {
		fmt.Printf("[TEST] Would wrap file in folder: %s -> %s/\n", fileName, folderName)
		return folderName, nil
	}
	
	// Create the folder
	err := os.MkdirAll(folderPath, 0755)
	if err != nil {
		return "", fmt.Errorf("error creating folder '%s': %v", folderPath, err)
	}
	
	// Move file into folder
	newFilePath := filepath.Join(folderPath, fileName)
	err = os.Rename(filePath, newFilePath)
	if err != nil {
		// Clean up folder if move fails
		os.Remove(folderPath)
		return "", fmt.Errorf("error moving file '%s' to '%s': %v", filePath, newFilePath, err)
	}
	
	fmt.Printf("Wrapped file in folder: %s -> %s/\n", fileName, folderName)
	return folderName, nil
}

// wrapMovieFileInFolder wraps a single movie file in a folder with the filename (without extension)
// Returns the created folder name, or error if operation fails
func wrapMovieFileInFolder(sourceDir, fileName string, testMode bool) (string, error) {
	// Get file extension to remove it from folder name
	ext := filepath.Ext(fileName)
	folderName := strings.TrimSuffix(fileName, ext)
	
	// Build paths
	filePath := filepath.Join(sourceDir, fileName)
	folderPath := filepath.Join(sourceDir, folderName)
	
	// Check if folder already exists
	if _, err := os.Stat(folderPath); err == nil {
		return folderName, fmt.Errorf("folder already exists: %s", folderPath)
	}
	
	// Create folder and move file
	if testMode {
		fmt.Printf("[TEST] Would wrap movie file in folder: %s -> %s/\n", fileName, folderName)
		return folderName, nil
	}
	
	// Create the folder
	err := os.MkdirAll(folderPath, 0755)
	if err != nil {
		return "", fmt.Errorf("error creating folder '%s': %v", folderPath, err)
	}
	
	// Move file into folder
	newFilePath := filepath.Join(folderPath, fileName)
	err = os.Rename(filePath, newFilePath)
	if err != nil {
		// Clean up folder if move fails
		os.Remove(folderPath)
		return "", fmt.Errorf("error moving file '%s' to '%s': %v", filePath, newFilePath, err)
	}
	
	fmt.Printf("Wrapped movie file in folder: %s -> %s/\n", fileName, folderName)
	return folderName, nil
}

// processShows processes TV show folders with pattern matching and organization
func processShows(sourceDir, destDir string, testMode bool) (*ProcessResult, error) {
	// Validate source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory '%s' does not exist", sourceDir)
	}

	// Read all entries in source directory
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("error reading source directory: %v", err)
	}

	// Pattern to match: showname.S01E20 or showname.s01e20 (case-insensitive)
	// Group 1: show name, Group 2: season/episode pattern (e.g., S01E20)
	pattern := regexp.MustCompile(`^(.+?)\.([Ss]\d{1,2}[Ee]\d{1,2}).*$`)

	result := &ProcessResult{
		CreatedDirs: []string{},
		MovedItems:  []string{},
		MovedCount:  0,
	}

	// Track wrapped folders to include them in directory processing
	wrappedFolders := make(map[string]bool)

	// FIRST PASS: Process files matching the pattern and wrap them in folders
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories in first pass
		}

		fileName := entry.Name()
		
		// Skip system files and hidden files
		if shouldIgnoreFile(fileName) {
			continue
		}
		
		filePath := filepath.Join(sourceDir, fileName)
		
		// Skip files inside .ignore folders
		if isInIgnoreFolder(filePath) {
			continue
		}
		
		matches := pattern.FindStringSubmatch(fileName)

		if len(matches) != 3 {
			continue // Skip files that don't match the pattern
		}

		// Wrap the file in a folder
		folderName, err := wrapFileInFolder(sourceDir, fileName, testMode)
		if err != nil {
			fmt.Printf("Error wrapping file '%s' in folder: %v\n", fileName, err)
			continue
		}

		// Track this wrapped folder for second pass
		wrappedFolders[folderName] = true
		result.CreatedDirs = append(result.CreatedDirs, filepath.Join(sourceDir, folderName))
	}

	// SECOND PASS: Process directories (including newly wrapped folders)
	// Re-read entries to include newly created folders (only in non-test mode)
	// In test mode, we need to also process tracked wrapped folders that don't exist yet
	entries, err = os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("error reading source directory after wrapping files: %v", err)
	}

	// Process existing directories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue // Skip files, only process directories
		}

		folderName := entry.Name()
		
		// Skip system folders and hidden folders
		if shouldIgnoreFile(folderName) {
			continue
		}
		
		folderPath := filepath.Join(sourceDir, folderName)
		
		// Skip folders inside .ignore folders or .ignore folder itself
		if isInIgnoreFolder(folderPath) || folderName == ".ignore" {
			continue
		}
		
		matches := pattern.FindStringSubmatch(folderName)

		if len(matches) != 3 {
			continue // Skip folders that don't match the pattern
		}

		showName := matches[1]
		seasonEpisode := matches[2]

		// Extract season (e.g., "S01" from "S01E20")
		seasonPattern := regexp.MustCompile(`^([Ss]\d{1,2})`)
		seasonMatch := seasonPattern.FindString(seasonEpisode)
		if seasonMatch == "" {
			continue // Couldn't extract season
		}

		// Normalize season to uppercase (e.g., "S01")
		season := strings.ToUpper(seasonMatch)

		// Build base destination path for the show
		showDestPath := filepath.Join(destDir, showName)

		// Check for case-insensitive season directory match
		actualSeason, err := findCaseInsensitiveDir(showDestPath, season)
		if err != nil {
			// If show directory doesn't exist yet, that's okay - we'll create it
			actualSeason = season
		}

		// Build destination path: shows/showname/S01 (or existing case variant)
		destPath := filepath.Join(showDestPath, actualSeason)

		// Check if destination directory exists, create if not
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			if !testMode {
				err := os.MkdirAll(destPath, 0755)
				if err != nil {
					fmt.Printf("Error creating destination directory '%s': %v\n", destPath, err)
					continue
				}
			}
			result.CreatedDirs = append(result.CreatedDirs, destPath)
			if testMode {
				fmt.Printf("[TEST] Would create directory: %s\n", destPath)
			} else {
				fmt.Printf("Created directory: %s\n", destPath)
			}
		} else if actualSeason != season {
			// Found existing directory with different case
			if testMode {
				fmt.Printf("[TEST] Found existing directory with different case, using: %s (instead of %s)\n", destPath, filepath.Join(showDestPath, season))
			} else {
				fmt.Printf("Using existing directory: %s (instead of %s)\n", destPath, filepath.Join(showDestPath, season))
			}
		}

		// Build source and destination paths for the folder
		sourcePath := filepath.Join(sourceDir, folderName)
		destFolderPath := filepath.Join(destPath, folderName)

		// Check if episode already exists (duplicate detection)
		episodeExists, err := checkEpisodeExists(destPath, seasonEpisode)
		if err != nil {
			fmt.Printf("Error checking for duplicate episode: %v\n", err)
			// Continue with normal processing if check fails
		} else if episodeExists {
			// Duplicate episode found, move to dupe folder
			dupeDir := filepath.Join(sourceDir, "dupe")
			dupePath := filepath.Join(dupeDir, folderName)

			// Create dupe directory if it doesn't exist
			if _, err := os.Stat(dupeDir); os.IsNotExist(err) {
				if !testMode {
					err := os.MkdirAll(dupeDir, 0755)
					if err != nil {
						fmt.Printf("Error creating dupe directory '%s': %v\n", dupeDir, err)
						continue
					}
				}
				result.CreatedDirs = append(result.CreatedDirs, dupeDir)
				if testMode {
					fmt.Printf("[TEST] Would create dupe directory: %s\n", dupeDir)
				} else {
					fmt.Printf("Created dupe directory: %s\n", dupeDir)
				}
			}

			// Move to dupe folder
			if testMode {
				fmt.Printf("[TEST] Duplicate episode detected (%s), would move to dupe: %s -> %s\n", seasonEpisode, sourcePath, dupePath)
			} else {
				err := os.Rename(sourcePath, dupePath)
				if err != nil {
					fmt.Printf("Error moving duplicate folder '%s' to '%s': %v\n", sourcePath, dupePath, err)
					continue
				}
				fmt.Printf("Duplicate episode detected (%s), moved to dupe: %s -> %s\n", seasonEpisode, sourcePath, dupePath)
			}

			result.MovedCount++
			result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s (duplicate)", sourcePath, dupePath))
			continue
		}

		// Check if destination already exists (exact name match)
		if _, err := os.Stat(destFolderPath); err == nil {
			fmt.Printf("Warning: Destination already exists, skipping: %s\n", destFolderPath)
			continue
		}

		// Move the folder to normal destination
		if testMode {
			fmt.Printf("[TEST] Would move: %s -> %s\n", sourcePath, destFolderPath)
		} else {
			err := os.Rename(sourcePath, destFolderPath)
			if err != nil {
				fmt.Printf("Error moving folder '%s' to '%s': %v\n", sourcePath, destFolderPath, err)
				continue
			}
			fmt.Printf("Moved: %s -> %s\n", sourcePath, destFolderPath)
		}

		result.MovedCount++
		result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s", sourcePath, destFolderPath))
	}

	// In test mode, also process wrapped folders that don't exist physically yet
	if testMode {
		for wrappedFolderName := range wrappedFolders {
			// Check if this folder was already processed above (it might exist)
			alreadyProcessed := false
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() == wrappedFolderName {
					alreadyProcessed = true
					break
				}
			}
			
			if alreadyProcessed {
				continue // Already processed in the loop above
			}

			// Process wrapped folder as if it were in the directory
			folderName := wrappedFolderName
			folderPath := filepath.Join(sourceDir, folderName)
			
			// Skip system folders and hidden folders
			if shouldIgnoreFile(folderName) {
				continue
			}
			
			// Skip folders inside .ignore folders or .ignore folder itself
			if isInIgnoreFolder(folderPath) || folderName == ".ignore" {
				continue
			}
			
			matches := pattern.FindStringSubmatch(folderName)

			if len(matches) != 3 {
				continue // Skip folders that don't match the pattern
			}

			showName := matches[1]
			seasonEpisode := matches[2]

			// Extract season (e.g., "S01" from "S01E20")
			seasonPattern := regexp.MustCompile(`^([Ss]\d{1,2})`)
			seasonMatch := seasonPattern.FindString(seasonEpisode)
			if seasonMatch == "" {
				continue // Couldn't extract season
			}

			// Normalize season to uppercase (e.g., "S01")
			season := strings.ToUpper(seasonMatch)

			// Build base destination path for the show
			showDestPath := filepath.Join(destDir, showName)

			// Check for case-insensitive season directory match
			actualSeason, err := findCaseInsensitiveDir(showDestPath, season)
			if err != nil {
				// If show directory doesn't exist yet, that's okay - we'll create it
				actualSeason = season
			}

			// Build destination path: shows/showname/S01 (or existing case variant)
			destPath := filepath.Join(showDestPath, actualSeason)

			// Check if destination directory exists, create if not
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				result.CreatedDirs = append(result.CreatedDirs, destPath)
				fmt.Printf("[TEST] Would create directory: %s\n", destPath)
			} else if actualSeason != season {
				// Found existing directory with different case
				fmt.Printf("[TEST] Found existing directory with different case, using: %s (instead of %s)\n", destPath, filepath.Join(showDestPath, season))
			}

			// Build source and destination paths for the folder
			sourcePath := filepath.Join(sourceDir, folderName)
			destFolderPath := filepath.Join(destPath, folderName)

			// Check if episode already exists (duplicate detection)
			episodeExists, err := checkEpisodeExists(destPath, seasonEpisode)
			if err != nil {
				fmt.Printf("Error checking for duplicate episode: %v\n", err)
				// Continue with normal processing if check fails
			} else if episodeExists {
				// Duplicate episode found, move to dupe folder
				dupeDir := filepath.Join(sourceDir, "dupe")
				dupePath := filepath.Join(dupeDir, folderName)

				// Create dupe directory if it doesn't exist
				if _, err := os.Stat(dupeDir); os.IsNotExist(err) {
					result.CreatedDirs = append(result.CreatedDirs, dupeDir)
					fmt.Printf("[TEST] Would create dupe directory: %s\n", dupeDir)
				}

				// Move to dupe folder
				fmt.Printf("[TEST] Duplicate episode detected (%s), would move to dupe: %s -> %s\n", seasonEpisode, sourcePath, dupePath)

				result.MovedCount++
				result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s (duplicate)", sourcePath, dupePath))
				continue
			}

			// Check if destination already exists (exact name match)
			if _, err := os.Stat(destFolderPath); err == nil {
				fmt.Printf("Warning: Destination already exists, skipping: %s\n", destFolderPath)
				continue
			}

			// Move the folder to normal destination
			fmt.Printf("[TEST] Would move: %s -> %s\n", sourcePath, destFolderPath)

			result.MovedCount++
			result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s", sourcePath, destFolderPath))
		}
	}

	return result, nil
}

// processMovies processes movie folders/files with simple moving (no pattern matching)
func processMovies(sourceDir, destDir string, testMode bool) (*ProcessResult, error) {
	// Validate source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory '%s' does not exist", sourceDir)
	}

	// Read all entries in source directory
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("error reading source directory: %v", err)
	}

	result := &ProcessResult{
		CreatedDirs: []string{},
		MovedItems:  []string{},
		MovedCount:  0,
	}

	// Ensure destination directory exists
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		if !testMode {
			err := os.MkdirAll(destDir, 0755)
			if err != nil {
				return nil, fmt.Errorf("error creating destination directory '%s': %v", destDir, err)
			}
		}
		result.CreatedDirs = append(result.CreatedDirs, destDir)
		if testMode {
			fmt.Printf("[TEST] Would create directory: %s\n", destDir)
		} else {
			fmt.Printf("Created directory: %s\n", destDir)
		}
	}

	// Track wrapped folders to include them in directory processing
	wrappedFolders := make(map[string]bool)

	// FIRST PASS: Process files and wrap them in folders
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories in first pass
		}

		fileName := entry.Name()
		
		// Skip system files and hidden files
		if shouldIgnoreFile(fileName) {
			continue
		}
		
		filePath := filepath.Join(sourceDir, fileName)
		
		// Skip files inside .ignore folders
		if isInIgnoreFolder(filePath) {
			continue
		}

		// Wrap the file in a folder
		folderName, err := wrapMovieFileInFolder(sourceDir, fileName, testMode)
		if err != nil {
			fmt.Printf("Error wrapping movie file '%s' in folder: %v\n", fileName, err)
			continue
		}

		// Track this wrapped folder for second pass
		wrappedFolders[folderName] = true
		result.CreatedDirs = append(result.CreatedDirs, filepath.Join(sourceDir, folderName))
	}

	// SECOND PASS: Process directories (including newly wrapped folders)
	// Re-read entries to include newly created folders
	entries, err = os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("error reading source directory after wrapping files: %v", err)
	}

	// Process existing directories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue // Skip files, only process directories
		}

		entryName := entry.Name()
		
		// Skip system folders and hidden folders
		if shouldIgnoreFile(entryName) {
			continue
		}
		
		sourcePath := filepath.Join(sourceDir, entryName)
		
		// Skip folders inside .ignore folders or .ignore folder itself
		if isInIgnoreFolder(sourcePath) || entryName == ".ignore" {
			continue
		}
		
		destPath := filepath.Join(destDir, entryName)

		// Check if destination already exists
		if _, err := os.Stat(destPath); err == nil {
			fmt.Printf("Warning: Destination already exists, skipping: %s\n", destPath)
			continue
		}

		// Move the folder
		if testMode {
			fmt.Printf("[TEST] Would move folder: %s -> %s\n", sourcePath, destPath)
		} else {
			err := os.Rename(sourcePath, destPath)
			if err != nil {
				fmt.Printf("Error moving folder '%s' to '%s': %v\n", sourcePath, destPath, err)
				continue
			}
			fmt.Printf("Moved folder: %s -> %s\n", sourcePath, destPath)
		}

		result.MovedCount++
		result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s", sourcePath, destPath))
	}

	// In test mode, also process wrapped folders that don't exist physically yet
	if testMode {
		for wrappedFolderName := range wrappedFolders {
			// Check if this folder was already processed above (it might exist)
			alreadyProcessed := false
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() == wrappedFolderName {
					alreadyProcessed = true
					break
				}
			}
			
			if alreadyProcessed {
				continue // Already processed in the loop above
			}

			// Process wrapped folder as if it were in the directory
			entryName := wrappedFolderName
			
			// Skip system folders and hidden folders
			if shouldIgnoreFile(entryName) {
				continue
			}
			
			sourcePath := filepath.Join(sourceDir, entryName)
			
			// Skip folders inside .ignore folders or .ignore folder itself
			if isInIgnoreFolder(sourcePath) || entryName == ".ignore" {
				continue
			}
			
			destPath := filepath.Join(destDir, entryName)

			// Check if destination already exists
			if _, err := os.Stat(destPath); err == nil {
				fmt.Printf("Warning: Destination already exists, skipping: %s\n", destPath)
				continue
			}

			// Move the folder
			fmt.Printf("[TEST] Would move folder: %s -> %s\n", sourcePath, destPath)

			result.MovedCount++
			result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s", sourcePath, destPath))
		}
	}

	return result, nil
}

func main() {
	// First, check for config file path in command line arguments
	configPath := "config.json"
	for i, arg := range os.Args[1:] {
		if arg == "-config" || arg == "--config" {
			if i+1 < len(os.Args)-1 {
				configPath = os.Args[i+2]
				break
			}
		}
	}

	// Load configuration from config.json - config file is required
	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("Error: Could not load config file '%s': %v\n", configPath, err)
		fmt.Println("\nPlease create a config.json file. See config.json.example for reference.")
		os.Exit(1)
	}

	// Use config values directly
	testMode := config.DevMode

	var allCreatedDirs []string
	var allMovedItems []string
	totalMovedCount := 0
	processedShows := false
	processedMovies := false

	// Process TV shows if directories are configured
	if config.ShowsSourceDir != "" && config.ShowsDestDir != "" {
		fmt.Println("=== Processing TV Shows ===")
		showsResult, err := processShows(config.ShowsSourceDir, config.ShowsDestDir, testMode)
		if err != nil {
			fmt.Printf("Error processing shows: %v\n", err)
		} else {
			allCreatedDirs = append(allCreatedDirs, showsResult.CreatedDirs...)
			allMovedItems = append(allMovedItems, showsResult.MovedItems...)
			totalMovedCount += showsResult.MovedCount
			processedShows = true
		}
		fmt.Println()
	}

	// Process movies if directories are configured
	if config.MoviesSourceDir != "" && config.MoviesDestDir != "" {
		fmt.Println("=== Processing Movies ===")
		moviesResult, err := processMovies(config.MoviesSourceDir, config.MoviesDestDir, testMode)
		if err != nil {
			fmt.Printf("Error processing movies: %v\n", err)
		} else {
			allCreatedDirs = append(allCreatedDirs, moviesResult.CreatedDirs...)
			allMovedItems = append(allMovedItems, moviesResult.MovedItems...)
			totalMovedCount += moviesResult.MovedCount
			processedMovies = true
		}
		fmt.Println()
	}

	// Check if anything was configured
	if !processedShows && !processedMovies {
		fmt.Println("Error: No source/destination directories configured.")
		fmt.Println("Please configure showsSourceDir/showsDestDir and/or moviesSourceDir/moviesDestDir in config.json")
		os.Exit(1)
	}

	// Print summary
	fmt.Println("=== Summary ===")
	if testMode {
		fmt.Println("[TEST MODE - No actual changes were made]")
	}
	fmt.Printf("Directories created: %d\n", len(allCreatedDirs))
	fmt.Printf("Items moved: %d\n", totalMovedCount)

	if len(allCreatedDirs) > 0 {
		fmt.Println("\nCreated directories:")
		for _, dir := range allCreatedDirs {
			fmt.Printf("  - %s\n", dir)
		}
	}

	if len(allMovedItems) > 0 {
		fmt.Println("\nMoved items:")
		for _, item := range allMovedItems {
			fmt.Printf("  - %s\n", item)
		}
	}

	if totalMovedCount == 0 && len(allCreatedDirs) == 0 {
		if processedShows && processedMovies {
			fmt.Println("No items were found or moved for shows or movies.")
		} else if processedShows {
			fmt.Println("No folders matching the TV show pattern were found or moved.")
		} else {
			fmt.Println("No items were found or moved.")
		}
	}
}
