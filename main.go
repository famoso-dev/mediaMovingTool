package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

	DevMode     bool     `json:"devMode"`
	SkipFolders []string `json:"skipFolders"`
}

// loadConfig loads configuration from config.json - config file is required
func loadConfig(configPath string) (*Config, error) {
	config := &Config{}

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
	Unsure      []UnsureItem
}

// UnsureItem is a source entry that could not be classified with confidence.
type UnsureItem struct {
	SourceLabel string // e.g. "shows" or "movies"
	SourceDir   string
	Name        string
	Reason      string
}

// ShowInfo is parsed metadata for organizing a TV show folder/file.
type ShowInfo struct {
	ShowName      string // directory under showsDestDir; release names keep "(year)"
	Season        string // e.g. S01 (uppercase)
	SeasonEpisode string // e.g. S01E08, or empty for season-only packs
	SeasonOnly    bool
}

var (
	// Dot style: Show.Name.S01E08.extra
	showDotPattern = regexp.MustCompile(`^(.+?)\.([Ss]\d{1,2}[Ee]\d{1,2}).*$`)
	// Dot style with separated season/episode: Show.Name.S01.E08.extra
	showDotSeparatedEpisodePattern = regexp.MustCompile(`^(.+?)\.([Ss]\d{1,2})\.([Ee]\d{1,2}).*$`)
	// Release style: Show Name (2019) S02E08 ... — year must be 19xx/20xx (not 1080 etc.)
	showReleaseYearEpisodePattern = regexp.MustCompile(`^(.+?)\s+\(((?:19|20)\d{2})\)\s+([Ss]\d{1,2}[Ee]\d{1,2})\b`)
	showReleaseEpisodePattern     = regexp.MustCompile(`^(.+?)\s+([Ss]\d{1,2}[Ee]\d{1,2})\b`)
	// Season pack: Show Name (2024) S01 (1080p...) or Show Name (2024) S01[TAoE]
	showReleaseYearSeasonPattern = regexp.MustCompile(`^(.+?)\s+\(((?:19|20)\d{2})\)\s+([Ss]\d{1,2})(?:\s|\(|\[|$)`)
	showReleaseSeasonPattern     = regexp.MustCompile(`^(.+?)\s+([Ss]\d{1,2})(?:\s|\(|\[|$)`)
	// Release show folder suffix: "My Show (2020)"
	releaseYearSuffixPattern = regexp.MustCompile(`^(.+?)\s+\(((?:19|20)\d{2})\)$`)
	// Quality tags in parens: (1080p), (10bit) — digits with trailing letters
	qualityParenPattern = regexp.MustCompile(`\((\d+[a-zA-Z][a-zA-Z0-9]*)\)`)
	// Dot season pack: Show.Name.S01.extra or Show.Name.S01.
	showDotSeasonPattern = regexp.MustCompile(`^(.+?)\.([Ss]\d{1,2})(?:\.|\s|\[|$)`)
	seasonFromTokenPattern       = regexp.MustCompile(`^([Ss]\d{1,2})`)
	episodeTokenPattern          = regexp.MustCompile(`([Ss]\d{1,2}[Ee]\d{1,2})`)
	showYearParenPattern         = regexp.MustCompile(`\s*\(\d{4}\)\s*`)
	embeddedYearPattern          = regexp.MustCompile(`(?:^|[.\s_])(?:19|20)\d{2}(?:[.\s_)]|$)`)
)

// fuzzyAutoThreshold: similarity above this auto-merges show folders without prompting.
const fuzzyAutoThreshold = 0.60

// fuzzyPromptThreshold: similarity above this (and at or below fuzzyAutoThreshold) prompts the user.
const fuzzyPromptThreshold = 0.50

// minShowNameNormLen is the minimum normalized name length for fuzzy matching.
const minShowNameNormLen = 3

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

var ignorePatterns = []string{
	".DS_Store", "Thumbs.db", "desktop.ini", "._", "~$",
	".Spotlight-V100", ".Trashes", ".fseventsd", ".VolumeIcon.icns",
	".com.apple.", "$RECYCLE.BIN", "System Volume Information",
}

func shouldIgnoreFile(fileName string) bool {
	for _, pattern := range ignorePatterns {
		if strings.HasPrefix(fileName, pattern) || fileName == pattern {
			return true
		}
	}
	return false
}

// mediaExtensions lists common media file extensions (lowercase, with leading dot).
var mediaExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".mp3": true, ".mov": true, ".avi": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true, ".mpg": true,
	".mpeg": true, ".m2ts": true, ".ts": true, ".flac": true, ".aac": true,
	".ogg": true, ".wav": true, ".wma": true, ".m4a": true, ".divx": true,
	".vob": true, ".iso": true,
}

// stripMediaExtensions removes trailing media extensions from a basename.
func stripMediaExtensions(name string) string {
	for {
		ext := filepath.Ext(name)
		if ext == "" {
			break
		}
		if !mediaExtensions[strings.ToLower(ext)] {
			break
		}
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

// parseShowEntry detects TV show metadata from a file or folder name.
// Tries dot style first, then release-style (spaces/parens). Returns ambiguous=true
// when multiple patterns disagree on show name or season.
func parseShowEntry(name string) (*ShowInfo, bool) {
	parseName := stripMediaExtensions(name)
	var candidates []*ShowInfo

	// Dot-style names use "." separators; release-style requires spaces before S## tokens.
	if strings.Contains(parseName, ".") {
		if m := showDotPattern.FindStringSubmatch(parseName); len(m) == 3 {
			candidates = append(candidates, showInfoFromToken(m[1], m[2], false))
		}
		if m := showDotSeparatedEpisodePattern.FindStringSubmatch(parseName); len(m) == 4 {
			if !showDotPattern.MatchString(parseName) {
				candidates = append(candidates, showInfoFromSeparatedEpisode(m[1], m[2], m[3]))
			}
		}
		if m := showDotSeasonPattern.FindStringSubmatch(parseName); len(m) == 3 {
			if !showDotPattern.MatchString(parseName) && !showDotSeparatedEpisodePattern.MatchString(parseName) {
				candidates = append(candidates, showInfoFromToken(m[1], m[2], true))
			}
		}
	}
	if strings.Contains(name, " ") {
		if m := showReleaseYearEpisodePattern.FindStringSubmatch(name); len(m) == 4 {
			show := strings.TrimSpace(m[1]) + " (" + m[2] + ")"
			candidates = append(candidates, showInfoFromToken(show, m[3], false))
		}
		if m := showReleaseEpisodePattern.FindStringSubmatch(name); len(m) == 3 {
			candidates = append(candidates, showInfoFromToken(strings.TrimSpace(m[1]), m[2], false))
		}
		if m := showReleaseYearSeasonPattern.FindStringSubmatch(name); len(m) == 4 {
			show := strings.TrimSpace(m[1]) + " (" + m[2] + ")"
			candidates = append(candidates, showInfoFromToken(show, m[3], true))
		}
		if m := showReleaseSeasonPattern.FindStringSubmatch(name); len(m) == 3 {
			// Avoid matching episode rows already captured (S01E08 has S01 before space)
			if !showReleaseYearEpisodePattern.MatchString(name) && !showReleaseEpisodePattern.MatchString(name) {
				candidates = append(candidates, showInfoFromToken(strings.TrimSpace(m[1]), m[2], true))
			}
		}
	}

	if len(candidates) == 0 {
		return nil, false
	}
	first := candidates[0]
	for _, c := range candidates[1:] {
		if c.ShowName != first.ShowName || c.Season != first.Season || c.SeasonEpisode != first.SeasonEpisode {
			return nil, true
		}
	}
	return first, false
}

func showInfoFromSeparatedEpisode(showName, seasonPart, episodePart string) *ShowInfo {
	seasonMatch := seasonFromTokenPattern.FindString(seasonPart)
	if seasonMatch == "" {
		return nil
	}
	return &ShowInfo{
		ShowName:      strings.TrimSpace(showName),
		Season:        strings.ToUpper(seasonMatch),
		SeasonEpisode: strings.ToUpper(seasonMatch + episodePart),
		SeasonOnly:    false,
	}
}

// parseShowEntryForUnsure parses show metadata for unsure-item routing. Uses the
// standard parser first, then falls back to dot-separated S##.E## when needed.
func parseShowEntryForUnsure(name string) *ShowInfo {
	if info, ambiguous := parseShowEntry(name); info != nil && !ambiguous {
		return info
	}
	parseName := stripMediaExtensions(name)
	if m := showDotSeparatedEpisodePattern.FindStringSubmatch(parseName); len(m) == 4 {
		return showInfoFromSeparatedEpisode(m[1], m[2], m[3])
	}
	return nil
}

func showInfoFromToken(showName, seasonToken string, seasonOnly bool) *ShowInfo {
	seasonMatch := seasonFromTokenPattern.FindString(seasonToken)
	if seasonMatch == "" {
		return nil
	}
	info := &ShowInfo{
		ShowName:   strings.TrimSpace(showName),
		Season:     strings.ToUpper(seasonMatch),
		SeasonOnly: seasonOnly,
	}
	if seasonOnly {
		info.SeasonEpisode = ""
	} else {
		info.SeasonEpisode = strings.ToUpper(seasonToken)
	}
	return info
}

// normalizeShowName returns a fuzzy comparison key. Dot-style names keep dots and only
// strip embedded .YYYY.; release-style names keep spaces and (year) so cross-style
// series (The.Boys vs The Boys (2019)) do not auto-merge on a shared key.
func normalizeShowName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(s, ".") {
		for embeddedYearPattern.MatchString(s) {
			s = embeddedYearPattern.ReplaceAllString(s, ".")
		}
		s = strings.Trim(s, "._ ")
		var parts []string
		for _, p := range strings.Split(s, ".") {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, ".")
	}
	return strings.Join(strings.Fields(s), " ")
}

// releaseShowBaseAndYear splits a release-style folder name into base + year.
// e.g. "My Show (2020)" -> "My Show", "2020".
func releaseShowBaseAndYear(showName string) (base, year string, ok bool) {
	m := releaseYearSuffixPattern.FindStringSubmatch(strings.TrimSpace(showName))
	if len(m) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(m[1]), m[2], true
}

// isQualityToken reports whether a token looks like a video/format tag (1080p, 10bit).
func isQualityToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	hasDigit, hasLetter := false, false
	for i, r := range token {
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
			if i == 0 {
				return false
			}
		} else {
			return false
		}
	}
	return hasDigit && hasLetter
}

func findQualityParenToken(showName string) (string, bool) {
	m := qualityParenPattern.FindStringSubmatch(showName)
	if len(m) != 2 || !isQualityToken(m[1]) {
		return "", false
	}
	return m[1], true
}

// existingReleaseYearlessFolder returns the dest folder when parsedShowName is a
// release-style year variant (My Show (2020)) and showsDestDir already has the
// base folder (My Show). Only matches an existing folder; never creates one.
func existingReleaseYearlessFolder(idx *showLibraryIndex, parsedShowName string) (string, bool) {
	base, _, ok := releaseShowBaseAndYear(parsedShowName)
	if !ok {
		return "", false
	}
	actual, exists := idx.exists(base)
	if !exists {
		return "", false
	}
	return actual, true
}

// promptReleaseYearFolder asks whether to create/use the yearless release name.
func promptReleaseYearFolder(destDir, parsedShowName, baseName string, testMode bool, reader *bufio.Reader) string {
	examplePath := filepath.ToSlash(filepath.Join(destDir, baseName, "S01", "episode-folder"))
	if testMode {
		fmt.Printf("[TEST] No show folder for %q — would prompt: use %q without year?\n", parsedShowName, baseName)
		fmt.Printf("[TEST]   Example path without year: %s\n", examplePath)
		fmt.Printf("[TEST]   Using %q for preview\n", parsedShowName)
		return parsedShowName
	}
	fmt.Printf("\nNo show folder found for %q.\n", parsedShowName)
	fmt.Printf("Without the year, the library path would look like:\n  %s\n", examplePath)
	fmt.Printf("Use folder name %q (without year) instead of %q? [Y/N]: ", baseName, parsedShowName)
	line, err := readPromptLine(reader)
	if err != nil {
		fmt.Printf("Error reading input: %v — using %q\n", err, parsedShowName)
		return parsedShowName
	}
	if isYesResponse(line) {
		fmt.Printf("Using show folder without year: %s\n", baseName)
		return baseName
	}
	return parsedShowName
}

// promptQualityTagConfirm asks whether a parenthetical token is a quality tag, not a year.
func promptQualityTagConfirm(token, showName string, testMode bool, reader *bufio.Reader) bool {
	if testMode {
		fmt.Printf("[TEST] (%s) in %q looks like a quality tag — would prompt to confirm [Y/N]\n", token, showName)
		return true
	}
	fmt.Printf("(%s) in %q looks like a quality/format tag (not a year). Treat as quality? [Y/N]: ", token, showName)
	line, err := readPromptLine(reader)
	if err != nil {
		fmt.Printf("Error reading input: %v — assuming quality tag\n", err)
		return true
	}
	if line == "" {
		return true
	}
	return isYesResponse(line)
}

// stripEmbeddedDotYears removes dotted .YYYY. segments from dot-style show names.
// e.g. "The.Boys.2026" -> "The.Boys". Names without embedded years are unchanged.
func stripEmbeddedDotYears(name string) string {
	if !strings.Contains(name, ".") {
		return name
	}
	s := strings.TrimSpace(name)
	lower := strings.ToLower(s)
	for {
		loc := embeddedYearPattern.FindStringIndex(lower)
		if loc == nil {
			break
		}
		s = s[:loc[0]] + s[loc[1]:]
		lower = strings.ToLower(s)
		s = strings.Trim(s, "._ ")
		lower = strings.Trim(lower, "._ ")
	}
	return s
}

// fsCache memoizes directory listings to avoid repeated ReadDir calls per run.
type fsCache struct {
	dirs map[string][]os.DirEntry
}

func newFSCache() *fsCache {
	return &fsCache{dirs: make(map[string][]os.DirEntry)}
}

func (c *fsCache) readDir(path string) ([]os.DirEntry, error) {
	if entries, ok := c.dirs[path]; ok {
		return entries, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	c.dirs[path] = entries
	return entries, nil
}

func (c *fsCache) invalidate(path string) {
	delete(c.dirs, path)
}

// showLibraryIndex caches existing show folders under showsDestDir for O(1) lookups.
type showLibraryIndex struct {
	folders []string
	byLower map[string]string
	byNorm  map[string][]string
}

func buildShowLibraryIndex(destDir string, cache *fsCache) *showLibraryIndex {
	idx := &showLibraryIndex{
		byLower: make(map[string]string),
		byNorm:  make(map[string][]string),
	}
	entries, err := cache.readDir(destDir)
	if err != nil {
		return idx
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		idx.folders = append(idx.folders, name)
		lower := strings.ToLower(name)
		if _, exists := idx.byLower[lower]; !exists {
			idx.byLower[lower] = name
		}
		norm := normalizeShowName(name)
		if norm != "" {
			idx.byNorm[norm] = append(idx.byNorm[norm], name)
		}
	}
	return idx
}

func (idx *showLibraryIndex) addFolder(name string) {
	if idx == nil {
		return
	}
	lower := strings.ToLower(name)
	if existing, ok := idx.byLower[lower]; ok && existing == name {
		return
	}
	idx.folders = append(idx.folders, name)
	if _, exists := idx.byLower[lower]; !exists {
		idx.byLower[lower] = name
	}
	norm := normalizeShowName(name)
	if norm != "" {
		idx.byNorm[norm] = append(idx.byNorm[norm], name)
	}
}

func (idx *showLibraryIndex) exists(desiredName string) (actual string, ok bool) {
	if idx == nil {
		return "", false
	}
	if actual, ok := idx.byLower[strings.ToLower(desiredName)]; ok {
		return actual, true
	}
	return "", false
}

// existingDotSeriesFolder returns the dest folder name when parsedShowName is a
// dot-style year variant (The.Boys.2026) and showsDestDir already has the base
// series folder (The.Boys). Only matches an existing folder; never creates one.
func existingDotSeriesFolder(idx *showLibraryIndex, parsedShowName string) (string, bool) {
	if !strings.Contains(parsedShowName, ".") {
		return "", false
	}
	canonical := stripEmbeddedDotYears(parsedShowName)
	if canonical == parsedShowName {
		return "", false
	}
	actual, ok := idx.exists(canonical)
	if !ok {
		return "", false
	}
	return actual, true
}

// showFolderExists reports whether a show folder exists under destDir (case-insensitive).
func showFolderExists(destDir, desiredName string) (actual string, ok bool) {
	cache := newFSCache()
	idx := buildShowLibraryIndex(destDir, cache)
	return idx.exists(desiredName)
}

// levenshteinDistance returns the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				min(prev[j]+1, curr[j-1]+1),
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// showNameSimilarity returns a ratio in [0,1] comparing normalized show names.
func showNameSimilarity(a, b string) float64 {
	na := normalizeShowName(a)
	nb := normalizeShowName(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	if len(na) < minShowNameNormLen || len(nb) < minShowNameNormLen {
		return 0
	}
	dist := levenshteinDistance(na, nb)
	maxLen := len(na)
	if len(nb) > maxLen {
		maxLen = len(nb)
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// showFolderCandidate is a possible existing show folder under showsDestDir.
type showFolderCandidate struct {
	FolderName string
	Score      float64
	NormMatch  bool
}

// findShowFolderCandidates lists existing show folders matching parsedShowName.
func findShowFolderCandidates(idx *showLibraryIndex, parsedShowName string) []showFolderCandidate {
	if idx == nil || len(idx.folders) == 0 {
		return nil
	}
	parsedNorm := normalizeShowName(parsedShowName)
	var out []showFolderCandidate

	for _, existing := range idx.folders {
		score := showNameSimilarity(parsedShowName, existing)
		normMatch := parsedNorm != "" && normalizeShowName(existing) == parsedNorm
		if strings.EqualFold(existing, parsedShowName) {
			score = 1
			normMatch = true
		}
		if normMatch || score > fuzzyPromptThreshold {
			out = append(out, showFolderCandidate{
				FolderName: existing,
				Score:      score,
				NormMatch:  normMatch,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NormMatch != out[j].NormMatch {
			return out[i].NormMatch
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// findBestFuzzyShowFolder returns the top candidate if any (for tests).
func findBestFuzzyShowFolder(destDir, parsedShowName string) (folder string, score float64, found bool) {
	cache := newFSCache()
	idx := buildShowLibraryIndex(destDir, cache)
	candidates := findShowFolderCandidates(idx, parsedShowName)
	if len(candidates) == 0 {
		return "", 0, false
	}
	return candidates[0].FolderName, candidates[0].Score, true
}

func readPromptLine(reader *bufio.Reader) (string, error) {
	if reader == nil {
		reader = bufio.NewReader(os.Stdin)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptShowFolderChoice asks the user to pick among multiple matching show folders.
func promptShowFolderChoice(parsedShowName string, candidates []showFolderCandidate, testMode bool, reader *bufio.Reader) string {
	if testMode {
		fmt.Printf("[TEST] Multiple show folders match %q:\n", parsedShowName)
		for i, c := range candidates {
			fmt.Printf("[TEST]   %d) %s (%.0f%%)\n", i+1, c.FolderName, c.Score*100)
		}
		fmt.Printf("[TEST]   0) Create new folder %q\n", parsedShowName)
		fmt.Printf("[TEST] Would prompt for choice [0-%d]; using new folder %q\n", len(candidates), parsedShowName)
		return parsedShowName
	}
	fmt.Printf("\nMultiple show folders may match %q:\n", parsedShowName)
	for i, c := range candidates {
		label := fmt.Sprintf("%.0f%%", c.Score*100)
		if c.NormMatch {
			label = "same series (normalized)"
		}
		fmt.Printf("  %d) %s (%s)\n", i+1, c.FolderName, label)
	}
	fmt.Printf("  0) Create new folder %q\n", parsedShowName)
	fmt.Printf("Choice [0-%d]: ", len(candidates))
	line, err := readPromptLine(reader)
	if err != nil {
		fmt.Printf("Error reading input: %v — using new folder %q\n", err, parsedShowName)
		return parsedShowName
	}
	if line == "0" || line == "" {
		return parsedShowName
	}
	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err != nil || idx < 1 || idx > len(candidates) {
		fmt.Printf("Invalid choice — using new folder %q\n", parsedShowName)
		return parsedShowName
	}
	chosen := candidates[idx-1].FolderName
	fmt.Printf("Using existing show folder: %s\n", chosen)
	return chosen
}

// promptMergeShowFolder asks Y/N to merge into a single fuzzy match.
func promptMergeShowFolder(parsedShowName, match string, score float64, testMode bool, reader *bufio.Reader) bool {
	if testMode {
		fmt.Printf("[TEST] %s exists — would prompt: move %q into that folder? [Y/N] (%.0f%%)\n",
			match, parsedShowName, score*100)
		return false
	}
	fmt.Printf("%s exists — move %q into that folder? [Y/N]: ", match, parsedShowName)
	line, err := readPromptLine(reader)
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return false
	}
	return isYesResponse(line)
}

// resolveShowDestFolder searches showsDestDir for an existing show folder before creating one.
func resolveShowDestFolder(destDir, parsedShowName string, testMode bool, reader *bufio.Reader) string {
	cache := newFSCache()
	idx := buildShowLibraryIndex(destDir, cache)
	return resolveShowDestFolderWithIndex(idx, destDir, parsedShowName, nil, testMode, reader)
}

func resolveShowDestFolderWithIndex(idx *showLibraryIndex, destDir, parsedShowName string, resolvedCache map[string]string, testMode bool, reader *bufio.Reader) string {
	if resolvedCache != nil {
		if folder, ok := resolvedCache[parsedShowName]; ok {
			return folder
		}
	}

	var resolved string
	defer func() {
		if resolvedCache != nil && resolved != "" {
			resolvedCache[parsedShowName] = resolved
		}
	}()

	if folder, ok := existingDotSeriesFolder(idx, parsedShowName); ok {
		if testMode {
			fmt.Printf("[TEST] Dot-series folder %q exists — would use it for %q (preview path keeps %q)\n",
				folder, parsedShowName, parsedShowName)
			resolved = parsedShowName
			return resolved
		}
		fmt.Printf("Using existing show folder: %s (dot-series match for %s)\n", folder, parsedShowName)
		resolved = folder
		return resolved
	}

	if token, ok := findQualityParenToken(parsedShowName); ok {
		promptQualityTagConfirm(token, parsedShowName, testMode, reader)
	}

	// Fast path: exact or case-insensitive match — skip fuzzy scan entirely.
	if actual, ok := idx.exists(parsedShowName); ok {
		if actual != parsedShowName {
			fmt.Printf("Using existing show folder: %s (matches %s)\n", actual, parsedShowName)
		}
		resolved = actual
		return resolved
	}

	// Release-style year: reuse an existing yearless folder (My Show) for My Show (2020).
	if folder, ok := existingReleaseYearlessFolder(idx, parsedShowName); ok {
		if testMode {
			fmt.Printf("[TEST] Release folder %q exists — would use it for %q (preview path keeps %q)\n",
				folder, parsedShowName, parsedShowName)
			resolved = parsedShowName
			return resolved
		}
		fmt.Printf("Using existing show folder: %s (release match for %s)\n", folder, parsedShowName)
		resolved = folder
		return resolved
	}

	// Normalized match: same show name after normalization (e.g. different year suffix).
	parsedNorm := normalizeShowName(parsedShowName)
	if parsedNorm != "" {
		matches := idx.byNorm[parsedNorm]
		if len(matches) == 1 {
			fmt.Printf("Using existing show folder: %s (matches %s)\n", matches[0], parsedShowName)
			resolved = matches[0]
			return resolved
		}
		if len(matches) > 1 {
			normMatches := make([]showFolderCandidate, len(matches))
			for i, name := range matches {
				normMatches[i] = showFolderCandidate{FolderName: name, Score: 1, NormMatch: true}
			}
			resolved = promptShowFolderChoice(parsedShowName, normMatches, testMode, reader)
			return resolved
		}
	}

	// Fuzzy scan: only reached when no direct or normalized match exists.
	candidates := findShowFolderCandidates(idx, parsedShowName)
	var autoMatches, promptMatches []showFolderCandidate
	for _, c := range candidates {
		if c.NormMatch {
			continue // already handled above
		}
		if c.Score > fuzzyAutoThreshold {
			autoMatches = append(autoMatches, c)
		} else {
			promptMatches = append(promptMatches, c)
		}
	}

	if len(autoMatches) > 0 {
		best := autoMatches[0]
		if testMode {
			fmt.Printf("[TEST] Auto-matched show folder %q (%.0f%%) for %q — would use it\n",
				best.FolderName, best.Score*100, parsedShowName)
			resolved = parsedShowName
			return resolved
		}
		fmt.Printf("Using existing show folder: %s (%.0f%% match for %s)\n",
			best.FolderName, best.Score*100, parsedShowName)
		resolved = best.FolderName
		return resolved
	}

	if len(promptMatches) > 1 {
		resolved = promptShowFolderChoice(parsedShowName, promptMatches, testMode, reader)
		return resolved
	}
	if len(promptMatches) == 1 {
		if promptMergeShowFolder(parsedShowName, promptMatches[0].FolderName, promptMatches[0].Score, testMode, reader) {
			fmt.Printf("Using existing show folder: %s\n", promptMatches[0].FolderName)
			resolved = promptMatches[0].FolderName
			return resolved
		}
		resolved = parsedShowName
		return resolved
	}

	// Release with year but no library match — offer yearless folder name.
	if base, _, hasYear := releaseShowBaseAndYear(parsedShowName); hasYear {
		resolved = promptReleaseYearFolder(destDir, parsedShowName, base, testMode, reader)
		return resolved
	}

	resolved = parsedShowName
	return resolved
}

func isYesResponse(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// findCaseInsensitiveDir checks if a directory exists with case-insensitive matching
// Returns the actual directory name if found, or the desired name if not found
func findCaseInsensitiveDir(parentDir, desiredName string) (string, error) {
	return findCaseInsensitiveDirCached(parentDir, desiredName, nil)
}

func findCaseInsensitiveDirCached(parentDir, desiredName string, cache *fsCache) (string, error) {
	exactPath := filepath.Join(parentDir, desiredName)
	if info, err := os.Stat(exactPath); err == nil && info.IsDir() {
		return desiredName, nil
	}

	var entries []os.DirEntry
	var err error
	if cache != nil {
		entries, err = cache.readDir(parentDir)
	} else {
		entries, err = os.ReadDir(parentDir)
	}
	if err != nil {
		return desiredName, err
	}

	desiredLower := strings.ToLower(desiredName)
	for _, entry := range entries {
		if entry.IsDir() && strings.ToLower(entry.Name()) == desiredLower {
			return entry.Name(), nil
		}
	}

	return desiredName, nil
}

// checkEpisodeExists checks if any folder with the same season/episode pattern already exists
// in the destination season directory. Returns true if a duplicate is found, false otherwise.
func checkEpisodeExists(destPath, seasonEpisode string) (bool, error) {
	return checkEpisodeExistsCached(destPath, seasonEpisode, nil)
}

func checkEpisodeExistsCached(destPath, seasonEpisode string, cache *fsCache) (bool, error) {
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return false, nil
	}

	var entries []os.DirEntry
	var err error
	if cache != nil {
		entries, err = cache.readDir(destPath)
	} else {
		entries, err = os.ReadDir(destPath)
	}
	if err != nil {
		return false, err
	}

	seasonEpisodeUpper := strings.ToUpper(seasonEpisode)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, match := range episodeTokenPattern.FindAllStringSubmatch(entry.Name(), -1) {
			if strings.ToUpper(match[0]) == seasonEpisodeUpper {
				return true, nil
			}
		}
	}
	return false, nil
}

// checkSeasonPackExists checks if a season pack (S## without episode) already exists in dest season dir.
func checkSeasonPackExists(destPath, season string) (bool, error) {
	return checkSeasonPackExistsCached(destPath, season, nil)
}

func checkSeasonPackExistsCached(destPath, season string, cache *fsCache) (bool, error) {
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		return false, nil
	}
	var entries []os.DirEntry
	var err error
	if cache != nil {
		entries, err = cache.readDir(destPath)
	} else {
		entries, err = os.ReadDir(destPath)
	}
	if err != nil {
		return false, err
	}
	seasonUpper := strings.ToUpper(season)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		nameUpper := strings.ToUpper(entry.Name())
		idx := strings.Index(nameUpper, seasonUpper)
		if idx == -1 {
			continue
		}
		after := idx + len(seasonUpper)
		if after < len(nameUpper) {
			ch := nameUpper[after]
			if (ch >= '0' && ch <= '9') || ch == 'E' {
				continue
			}
		}
		return true, nil
	}
	return false, nil
}

// wrapFileInFolder wraps a single file in a folder named from the basename without media extensions.
// wrapLabel is used in log output (e.g. "file" or "movie file").
func wrapFileInFolder(sourceDir, fileName, wrapLabel string, testMode bool) (string, error) {
	folderName := stripMediaExtensions(fileName)

	filePath := filepath.Join(sourceDir, fileName)
	folderPath := filepath.Join(sourceDir, folderName)

	if _, err := os.Stat(folderPath); err == nil {
		// Folder already exists — check if the file is already inside it
		// (e.g. from a previous run that created the folder but didn't finish moving)
		innerPath := filepath.Join(folderPath, fileName)
		if _, err2 := os.Stat(innerPath); err2 == nil {
			// File is already wrapped correctly; treat as success
			return folderName, nil
		}
		return folderName, fmt.Errorf("folder already exists: %s", folderPath)
	}

	if testMode {
		fmt.Printf("[TEST] Would wrap %s in folder: %s -> %s/\n", wrapLabel, fileName, folderName)
		return folderName, nil
	}

	err := os.MkdirAll(folderPath, 0755)
	if err != nil {
		return "", fmt.Errorf("error creating folder '%s': %v", folderPath, err)
	}

	newFilePath := filepath.Join(folderPath, fileName)
	err = os.Rename(filePath, newFilePath)
	if err != nil {
		os.Remove(folderPath)
		return "", fmt.Errorf("error moving file '%s' to '%s': %v", filePath, newFilePath, err)
	}

	fmt.Printf("Wrapped %s in folder: %s -> %s/\n", wrapLabel, fileName, folderName)
	return folderName, nil
}

// moveEntryToDest moves a folder from sourceDir to destDir/entryName.
func moveEntryToDest(sourceDir, destDir, entryName string, result *ProcessResult, testMode bool, logPrefix string) bool {
	sourcePath := filepath.Join(sourceDir, entryName)
	destPath := filepath.Join(destDir, entryName)

	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("Warning: Destination already exists, skipping: %s\n", destPath)
		return false
	}

	if testMode {
		fmt.Printf("[TEST] %s: %s -> %s\n", logPrefix, sourcePath, destPath)
	} else {
		if err := os.Rename(sourcePath, destPath); err != nil {
			fmt.Printf("Error moving '%s' to '%s': %v\n", sourcePath, destPath, err)
			return false
		}
		fmt.Printf("%s: %s -> %s\n", logPrefix, sourcePath, destPath)
	}

	result.MovedCount++
	result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s", sourcePath, destPath))
	return true
}

func isShowsSourceReserved(name string) bool {
	return name == ".ignore" || name == "dupe"
}

func isInSkipList(name string, skipFolders []string) bool {
	lowerName := strings.ToLower(name)
	for _, s := range skipFolders {
		if strings.ToLower(s) == lowerName {
			return true
		}
	}
	return false
}

func shouldSkipShowsEntry(sourceDir, name string, isDir bool, skipFolders []string) bool {
	if shouldIgnoreFile(name) {
		return true
	}
	path := filepath.Join(sourceDir, name)
	if isInIgnoreFolder(path) {
		return true
	}
	if isDir && isShowsSourceReserved(name) {
		return true
	}
	if isInSkipList(name, skipFolders) {
		return true
	}
	return false
}

// ensureSeasonDir creates the season folder only after checking for an existing match.
func ensureSeasonDir(showDestPath, season string, testMode bool, result *ProcessResult, cache *fsCache) (destPath string, ok bool) {
	actualSeason, err := findCaseInsensitiveDirCached(showDestPath, season, cache)
	if err != nil {
		actualSeason = season
	}
	destPath = filepath.Join(showDestPath, actualSeason)
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		if !testMode {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				fmt.Printf("Error creating destination directory '%s': %v\n", destPath, err)
				return "", false
			}
			if cache != nil {
				cache.invalidate(showDestPath)
			}
		}
		result.CreatedDirs = append(result.CreatedDirs, destPath)
		if testMode {
			fmt.Printf("[TEST] Would create directory: %s\n", destPath)
		} else {
			fmt.Printf("Created directory: %s\n", destPath)
		}
	} else if actualSeason != season {
		if testMode {
			fmt.Printf("[TEST] Found existing season directory: %s (instead of %s)\n", destPath, filepath.Join(showDestPath, season))
		} else {
			fmt.Printf("Using existing season directory: %s (instead of %s)\n", destPath, filepath.Join(showDestPath, season))
		}
	}
	return destPath, true
}

func isMediaFileName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return mediaExtensions[ext]
}

// collectMediaFiles returns media file paths under root (recursive), relative to root.
func collectMediaFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if isInIgnoreFolder(path) || d.Name() == ".ignore" {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnoreFile(d.Name()) {
			return nil
		}
		if isMediaFileName(d.Name()) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// shouldOfferSeasonPackFlatten returns true when unpacking into the season folder is useful.
func shouldOfferSeasonPackFlatten(sourcePath, destSeasonPath string) bool {
	if _, err := os.Stat(destSeasonPath); err == nil {
		return true
	}
	if hasSubdirectories(sourcePath) {
		return true
	}
	files, err := collectMediaFiles(sourcePath)
	if err != nil || len(files) == 0 {
		return false
	}
	if len(files) > 1 {
		return true
	}
	return strings.Contains(files[0], string(filepath.Separator))
}

func hasSubdirectories(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != ".ignore" {
			return true
		}
	}
	return false
}

// promptSeasonPackFlatten asks whether to place all pack files directly in the season folder.
func promptSeasonPackFlatten(showFolder, season, sourcePath, destSeasonPath string, testMode bool, reader *bufio.Reader) bool {
	if testMode {
		fmt.Printf("[TEST] Season pack %s — would prompt: unpack all files into %s/%s/? [Y/N]\n",
			filepath.Base(sourcePath), showFolder, season)
		return false
	}
	fmt.Printf("\nSeason pack: %s\n", filepath.Base(sourcePath))
	fmt.Printf("Unpack all files directly into %s/%s/ (not in a subfolder)? [Y/N]: ", showFolder, season)
	line, err := readPromptLine(reader)
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return false
	}
	return isYesResponse(line)
}

func uniqueDestFilePath(destDir, baseName string) string {
	dest := filepath.Join(destDir, baseName)
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(destDir, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return dest
}

// flattenSeasonPack moves all media files from sourcePath into destSeasonPath (flat).
func flattenSeasonPack(sourcePath, destSeasonPath string, testMode bool, result *ProcessResult) bool {
	files, err := collectMediaFiles(sourcePath)
	if err != nil {
		fmt.Printf("Error scanning season pack '%s': %v\n", sourcePath, err)
		return false
	}
	if len(files) == 0 {
		fmt.Printf("No media files found in season pack '%s'\n", sourcePath)
		return false
	}
	for _, rel := range files {
		srcFile := filepath.Join(sourcePath, rel)
		baseName := filepath.Base(rel)
		destFile := uniqueDestFilePath(destSeasonPath, baseName)
		if testMode {
			fmt.Printf("[TEST] Would flatten: %s -> %s\n", srcFile, destFile)
		} else {
			if err := os.Rename(srcFile, destFile); err != nil {
				fmt.Printf("Error flattening '%s' to '%s': %v\n", srcFile, destFile, err)
				continue
			}
			fmt.Printf("Flattened: %s -> %s\n", srcFile, destFile)
		}
		result.MovedCount++
		result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s (flattened)", srcFile, destFile))
	}
	if !testMode {
		removeEmptyDirs(sourcePath)
		if entries, _ := os.ReadDir(sourcePath); len(entries) == 0 {
			os.Remove(sourcePath)
		}
	}
	return true
}

func removeEmptyDirs(root string) {
	var dirs []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		if entries, _ := os.ReadDir(dirs[i]); len(entries) == 0 {
			os.Remove(dirs[i])
		}
	}
}

// processOneShowFolder moves a matched show folder into shows/{show}/{season}/.
func processOneShowFolder(sourceDir, destDir, folderName string, info *ShowInfo, testMode bool, reader *bufio.Reader, result *ProcessResult, idx *showLibraryIndex, cache *fsCache, resolvedCache map[string]string) {
	showFolder := resolveShowDestFolderWithIndex(idx, destDir, info.ShowName, resolvedCache, testMode, reader)
	showDestPath := filepath.Join(destDir, showFolder)
	if _, err := os.Stat(showDestPath); os.IsNotExist(err) && !testMode {
		if err := os.MkdirAll(showDestPath, 0755); err != nil {
			fmt.Printf("Error creating show directory '%s': %v\n", showDestPath, err)
			return
		}
		idx.addFolder(showFolder)
		if cache != nil {
			cache.invalidate(destDir)
		}
	}
	destPath, ok := ensureSeasonDir(showDestPath, info.Season, testMode, result, cache)
	if !ok {
		return
	}

	sourcePath := filepath.Join(sourceDir, folderName)
	destFolderName := stripMediaExtensions(folderName)

	if info.SeasonOnly && shouldOfferSeasonPackFlatten(sourcePath, destPath) {
		if promptSeasonPackFlatten(showFolder, info.Season, sourcePath, destPath, testMode, reader) {
			if flattenSeasonPack(sourcePath, destPath, testMode, result) {
				return
			}
		}
	}

	destFolderPath := filepath.Join(destPath, destFolderName)

	var isDuplicate bool
	var err error
	if info.SeasonOnly {
		isDuplicate, err = checkSeasonPackExistsCached(destPath, info.Season, cache)
	} else {
		isDuplicate, err = checkEpisodeExistsCached(destPath, info.SeasonEpisode, cache)
	}
	if err != nil {
		fmt.Printf("Error checking for duplicate: %v\n", err)
	} else if isDuplicate {
		dupeDir := filepath.Join(sourceDir, "dupe")
		dupePath := filepath.Join(dupeDir, destFolderName)
		if _, err := os.Stat(dupeDir); os.IsNotExist(err) {
			if !testMode {
				if err := os.MkdirAll(dupeDir, 0755); err != nil {
					fmt.Printf("Error creating dupe directory '%s': %v\n", dupeDir, err)
					return
				}
			}
			result.CreatedDirs = append(result.CreatedDirs, dupeDir)
			if testMode {
				fmt.Printf("[TEST] Would create dupe directory: %s\n", dupeDir)
			} else {
				fmt.Printf("Created dupe directory: %s\n", dupeDir)
			}
		}
		dupLabel := info.SeasonEpisode
		if dupLabel == "" {
			dupLabel = info.Season + " (season pack)"
		}
		if testMode {
			fmt.Printf("[TEST] Duplicate detected (%s), would move to dupe: %s -> %s\n", dupLabel, sourcePath, dupePath)
		} else {
			if err := os.Rename(sourcePath, dupePath); err != nil {
				fmt.Printf("Error moving duplicate folder '%s' to '%s': %v\n", sourcePath, dupePath, err)
				return
			}
			fmt.Printf("Duplicate detected (%s), moved to dupe: %s -> %s\n", dupLabel, sourcePath, dupePath)
		}
		result.MovedCount++
		result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s (duplicate)", sourcePath, dupePath))
		if cache != nil {
			cache.invalidate(destPath)
		}
		return
	}

	if _, err := os.Stat(destFolderPath); err == nil {
		fmt.Printf("Warning: Destination already exists, skipping: %s\n", destFolderPath)
		return
	}

	if testMode {
		fmt.Printf("[TEST] Would move: %s -> %s\n", sourcePath, destFolderPath)
	} else {
		if err := os.Rename(sourcePath, destFolderPath); err != nil {
			fmt.Printf("Error moving folder '%s' to '%s': %v\n", sourcePath, destFolderPath, err)
			return
		}
		fmt.Printf("Moved: %s -> %s\n", sourcePath, destFolderPath)
	}
	result.MovedCount++
	result.MovedItems = append(result.MovedItems, fmt.Sprintf("%s -> %s", sourcePath, destFolderPath))
	if cache != nil {
		cache.invalidate(destPath)
	}
}

func collectUnsureShows(sourceDir string, processed map[string]bool, skipFolders []string) []UnsureItem {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil
	}
	var files, dirs []UnsureItem
	for _, entry := range entries {
		name := entry.Name()
		if processed[name] || shouldSkipShowsEntry(sourceDir, name, entry.IsDir(), skipFolders) {
			continue
		}
		item := UnsureItem{SourceLabel: "shows", SourceDir: sourceDir, Name: name, Reason: "no show pattern matched"}
		if entry.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}
	return append(files, dirs...)
}

// processShows processes TV show folders with pattern matching and organization
func processShows(sourceDir, destDir string, testMode bool, skipFolders []string) (*ProcessResult, error) {
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory '%s' does not exist", sourceDir)
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("error reading source directory: %v", err)
	}

	result := &ProcessResult{
		CreatedDirs: []string{},
		MovedItems:  []string{},
		MovedCount:  0,
		Unsure:      []UnsureItem{},
	}
	fsCache := newFSCache()
	showIndex := buildShowLibraryIndex(destDir, fsCache)
	resolvedShows := make(map[string]string)
	var stdinReader *bufio.Reader
	if !testMode {
		stdinReader = bufio.NewReader(os.Stdin)
	}
	processed := make(map[string]bool)
	wrappedFolders := make(map[string]bool)

	// FIRST PASS: wrap files that match a show pattern
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if shouldSkipShowsEntry(sourceDir, fileName, false, skipFolders) {
			continue
		}
		info, ambiguous := parseShowEntry(fileName)
		if ambiguous {
			result.Unsure = append(result.Unsure, UnsureItem{
				SourceLabel: "shows",
				SourceDir:   sourceDir,
				Name:        fileName,
				Reason:      "ambiguous: multiple show patterns matched",
			})
			continue
		}
		if info == nil {
			continue
		}
		folderName, err := wrapFileInFolder(sourceDir, fileName, "file", testMode)
		if err != nil {
			fmt.Printf("Error wrapping file '%s' in folder: %v\n", fileName, err)
			continue
		}
		wrappedFolders[folderName] = true
		processed[fileName] = true
		result.CreatedDirs = append(result.CreatedDirs, filepath.Join(sourceDir, folderName))
	}

	entries, err = os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("error reading source directory after wrapping files: %v", err)
	}

	processShowDir := func(folderName string) {
		if processed[folderName] {
			return
		}
		if shouldSkipShowsEntry(sourceDir, folderName, true, skipFolders) {
			return
		}
		info, ambiguous := parseShowEntry(folderName)
		if ambiguous {
			result.Unsure = append(result.Unsure, UnsureItem{
				SourceLabel: "shows",
				SourceDir:   sourceDir,
				Name:        folderName,
				Reason:      "ambiguous: multiple show patterns matched",
			})
			return
		}
		if info == nil {
			return
		}
		processOneShowFolder(sourceDir, destDir, folderName, info, testMode, stdinReader, result, showIndex, fsCache, resolvedShows)
		processed[folderName] = true
	}

	for _, entry := range entries {
		if entry.IsDir() {
			processShowDir(entry.Name())
		}
	}

	if testMode {
		dirSet := make(map[string]bool, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				dirSet[e.Name()] = true
			}
		}
		for folderName := range wrappedFolders {
			if !dirSet[folderName] {
				processShowDir(folderName)
			}
		}
	}

	result.Unsure = append(result.Unsure, collectUnsureShows(sourceDir, processed, skipFolders)...)
	return result, nil
}

func promptUnsureItems(config *Config, unsure []UnsureItem, testMode bool) {
	if len(unsure) == 0 {
		return
	}

	fmt.Println("\n=== Unsure items ===")
	for i, item := range unsure {
		fmt.Printf("  %d. [%s] %s — %s\n", i+1, item.SourceLabel, item.Name, item.Reason)
	}

	if testMode {
		fmt.Println("\n[TEST] Would prompt for each item (no input required):")
		fmt.Println("  1 = move to shows destination (wrapped in folder if media file)")
		fmt.Println("  2 = move to movies destination (wrapped in folder if media file)")
		fmt.Println("  3 = move to shows source dupe/")
		fmt.Println("  4 = skip (leave in place)")
		return
	}

	reader := bufio.NewReader(os.Stdin)
	for i, item := range unsure {
		fmt.Printf("\n--- Item %d/%d: %s ---\n", i+1, len(unsure), item.Name)
		fmt.Printf("Reason: %s\n", item.Reason)
		fmt.Println("  1) Move to shows destination")
		if config.MoviesDestDir != "" {
			fmt.Println("  2) Move to movies destination")
		} else {
			fmt.Println("  2) (unavailable — moviesDestDir not configured)")
		}
		fmt.Println("  3) Move to shows source dupe/")
		fmt.Println("  4) Skip (leave in source)")
		fmt.Print("Choice [1-4]: ")

		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v — skipping item\n", err)
			continue
		}
		choice := strings.TrimSpace(line)

		sourcePath := filepath.Join(item.SourceDir, item.Name)
		switch choice {
		case "1":
			if config.ShowsDestDir == "" {
				fmt.Println("showsDestDir not configured — skipping")
				continue
			}
			moveUnsureItemToDest(item, config.ShowsDestDir)
		case "2":
			if config.MoviesDestDir == "" {
				fmt.Println("moviesDestDir not configured — skipping")
				continue
			}
			moveUnsureItemToDest(item, config.MoviesDestDir)
		case "3":
			dupeDir := filepath.Join(item.SourceDir, "dupe")
			if err := os.MkdirAll(dupeDir, 0755); err != nil {
				fmt.Printf("Error creating dupe directory: %v\n", err)
				continue
			}
			applyUnsureMove(sourcePath, filepath.Join(dupeDir, item.Name))
		case "4":
			fmt.Println("Skipped.")
		default:
			fmt.Println("Invalid choice — skipped.")
		}
	}
}

// moveUnsureItemToDest moves an unsure item to destDir, wrapping it in a folder first
// if it is a single media file (extension stripped from the folder name).
// For items with a "shows" label, it attempts to parse the show pattern and route
// directly into destDir/ShowName/Season/ — falling back to destDir root if parsing fails.
func moveUnsureItemToDest(item UnsureItem, destDir string) {
	sourcePath := filepath.Join(item.SourceDir, item.Name)

	// For shows items, try to parse the show pattern for proper routing.
	if item.SourceLabel == "shows" {
		info := parseShowEntryForUnsure(item.Name)
		if info != nil {
			entryName := item.Name
			if isMediaFileName(item.Name) {
				folderName := stripMediaExtensions(item.Name)
				folderPath := filepath.Join(item.SourceDir, folderName)
				innerPath := filepath.Join(folderPath, item.Name)
				if _, err := os.Stat(innerPath); err == nil {
					sourcePath = folderPath
					entryName = folderName
				} else if _, err := os.Stat(folderPath); os.IsNotExist(err) {
					if err := os.MkdirAll(folderPath, 0755); err != nil {
						fmt.Printf("Error creating folder '%s': %v\n", folderPath, err)
						return
					}
					newFilePath := filepath.Join(folderPath, item.Name)
					if err := os.Rename(sourcePath, newFilePath); err != nil {
						os.Remove(folderPath)
						fmt.Printf("Error moving '%s' into folder: %v\n", sourcePath, err)
						return
					}
					fmt.Printf("Wrapped in folder: %s -> %s/\n", item.Name, folderName)
					sourcePath = folderPath
					entryName = folderName
				} else {
					fmt.Printf("Warning: folder '%s' already exists but file not inside — moving file directly\n", folderPath)
				}
			}
			showFolder := resolveShowDestFolder(destDir, info.ShowName, false, nil)
			showDestPath := filepath.Join(destDir, showFolder)
			if err := os.MkdirAll(showDestPath, 0755); err != nil {
				fmt.Printf("Error creating show directory '%s': %v\n", showDestPath, err)
				return
			}
			seasonDestPath := filepath.Join(showDestPath, info.Season)
			if err := os.MkdirAll(seasonDestPath, 0755); err != nil {
				fmt.Printf("Error creating season directory '%s': %v\n", seasonDestPath, err)
				return
			}
			destFolderName := stripMediaExtensions(entryName)
			applyUnsureMove(sourcePath, filepath.Join(seasonDestPath, destFolderName))
			return
		}
	}

	// Non-shows items or unparseable shows: wrap media files and move to destDir root.
	if isMediaFileName(item.Name) {
		folderName := stripMediaExtensions(item.Name)
		folderPath := filepath.Join(item.SourceDir, folderName)
		innerPath := filepath.Join(folderPath, item.Name)
		if _, err := os.Stat(innerPath); err == nil {
			applyUnsureMove(folderPath, filepath.Join(destDir, folderName))
			return
		}
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			fmt.Printf("Error creating folder '%s': %v\n", folderPath, err)
			return
		}
		newFilePath := filepath.Join(folderPath, item.Name)
		if err := os.Rename(sourcePath, newFilePath); err != nil {
			os.Remove(folderPath)
			fmt.Printf("Error moving '%s' into folder: %v\n", sourcePath, err)
			return
		}
		fmt.Printf("Wrapped in folder: %s -> %s/\n", item.Name, folderName)
		applyUnsureMove(folderPath, filepath.Join(destDir, folderName))
		return
	}
	applyUnsureMove(sourcePath, filepath.Join(destDir, item.Name))
}

func applyUnsureMove(sourcePath, destPath string) {
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("Warning: destination already exists, skipping: %s\n", destPath)
		return
	}
	if err := os.Rename(sourcePath, destPath); err != nil {
		fmt.Printf("Error moving '%s' to '%s': %v\n", sourcePath, destPath, err)
		return
	}
	fmt.Printf("Moved: %s -> %s\n", sourcePath, destPath)
}

// processMovies processes movie folders/files with simple moving (no pattern matching)
func processMovies(sourceDir, destDir string, testMode bool, skipFolders []string) (*ProcessResult, error) {
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

	// FIRST PASS: wrap loose media files in folders
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

		if isInSkipList(fileName, skipFolders) {
			continue
		}

		if !isMediaFileName(fileName) {
			continue
		}

		// Wrap the file in a folder
		folderName, err := wrapFileInFolder(sourceDir, fileName, "movie file", testMode)
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

		if isInSkipList(entryName, skipFolders) {
			continue
		}

		logPrefix := "Moved folder"
		if testMode {
			logPrefix = "Would move folder"
		}
		moveEntryToDest(sourceDir, destDir, entryName, result, testMode, logPrefix)
	}

	if testMode {
		dirSet := make(map[string]bool, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				dirSet[e.Name()] = true
			}
		}
		for wrappedFolderName := range wrappedFolders {
			if dirSet[wrappedFolderName] {
				continue
			}
			if shouldIgnoreFile(wrappedFolderName) {
				continue
			}
			sourcePath := filepath.Join(sourceDir, wrappedFolderName)
			if isInIgnoreFolder(sourcePath) || wrappedFolderName == ".ignore" {
				continue
			}
			moveEntryToDest(sourceDir, destDir, wrappedFolderName, result, testMode, "Would move folder")
		}
	}

	return result, nil
}

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Printf("Error: Could not load config file '%s': %v\n", *configPath, err)
		fmt.Println("\nPlease create a config.json file. See config.json.example for reference.")
		os.Exit(1)
	}

	// Use config values directly
	testMode := config.DevMode

	var allCreatedDirs []string
	var allMovedItems []string
	var allUnsure []UnsureItem
	totalMovedCount := 0
	processedShows := false
	processedMovies := false

	// Process TV shows if directories are configured
	if config.ShowsSourceDir != "" && config.ShowsDestDir != "" {
		fmt.Println("=== Processing TV Shows ===")
		showsResult, err := processShows(config.ShowsSourceDir, config.ShowsDestDir, testMode, config.SkipFolders)
		if err != nil {
			fmt.Printf("Error processing shows: %v\n", err)
		} else {
			allCreatedDirs = append(allCreatedDirs, showsResult.CreatedDirs...)
			allMovedItems = append(allMovedItems, showsResult.MovedItems...)
			allUnsure = append(allUnsure, showsResult.Unsure...)
			totalMovedCount += showsResult.MovedCount
			processedShows = true
		}
		fmt.Println()
	}

	// Process movies if directories are configured
	if config.MoviesSourceDir != "" && config.MoviesDestDir != "" {
		fmt.Println("=== Processing Movies ===")
		moviesResult, err := processMovies(config.MoviesSourceDir, config.MoviesDestDir, testMode, config.SkipFolders)
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

	promptUnsureItems(config, allUnsure, testMode)
}
