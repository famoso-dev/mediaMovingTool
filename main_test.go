package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeShowName(t *testing.T) {
	tests := map[string]string{
		"The.Boys":        "the.boys",
		"the.boys":        "the.boys",
		"The.Boys.2026":   "the.boys",
		"The Boys (2019)": "the boys (2019)",
		"The Boys":        "the boys",
		"The Pitt":        "the pitt",
		"  Dutton.Ranch  ": "dutton.ranch",
	}
	for input, want := range tests {
		if got := normalizeShowName(input); got != want {
			t.Errorf("normalizeShowName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStripEmbeddedDotYears(t *testing.T) {
	if got := stripEmbeddedDotYears("The.Boys.2026"); got != "The.Boys" {
		t.Fatalf("got %q want The.Boys", got)
	}
	if got := stripEmbeddedDotYears("The.Boys"); got != "The.Boys" {
		t.Fatalf("unchanged: got %q", got)
	}
}

func TestResolveShowDestFolder_DotYearUsesExistingBase(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "The.Boys"), 0755); err != nil {
		t.Fatal(err)
	}
	got := resolveShowDestFolder(dir, "The.Boys.2026", false, nil)
	if got != "The.Boys" {
		t.Fatalf("got %q want The.Boys", got)
	}
	dir2 := t.TempDir()
	got = resolveShowDestFolder(dir2, "The.Boys.2026", false, nil)
	if got != "The.Boys.2026" {
		t.Fatalf("empty dest: got %q want The.Boys.2026", got)
	}
}

func TestShowNameSimilarity(t *testing.T) {
	if s := showNameSimilarity("The.Boys", "the.boys"); s < 1.0 {
		t.Fatalf("The.Boys vs the.boys: got %v want 1.0", s)
	}
	if s := showNameSimilarity("The.Boys.2026", "The.Boys"); s < 1.0 {
		t.Fatalf("The.Boys.2026 vs The.Boys: got %v want 1.0", s)
	}
	if s := showNameSimilarity("The Boys (2019)", "The.Boys"); s >= 1.0 {
		t.Fatalf("release vs dot should not norm-match at 1.0: got %v", s)
	}
	if s := showNameSimilarity("The Boys (2019)", "The Boys"); s >= 1.0 {
		t.Fatalf("year release vs plain should not be 1.0: got %v", s)
	}
	if s := showNameSimilarity("The Boys", "The Pitt"); s > fuzzyPromptThreshold {
		t.Fatalf("The Boys vs The Pitt: got %v should be at or below prompt threshold", s)
	}
	// Dot-style show with minor name difference should auto-match (> 60%).
	s := showNameSimilarity("Georgie.Mandy.s.First.Marriage", "Georgie.and.Mandys.First.Marriage")
	if s <= fuzzyAutoThreshold {
		t.Fatalf("Georgie.Mandy.s vs Georgie.and.Mandys: got %.2f, want > %.2f", s, fuzzyAutoThreshold)
	}
}

func TestParseShowEntry_DotSeasonPack(t *testing.T) {
	cases := []struct {
		name  string
		show  string
		season string
	}{
		{"The.Madison.S01.1080p.10bit.WEBRip.6CH.x265.HEVC-PSA", "The.Madison", "S01"},
		{"Dorohedoro.S01.JAPANESE.1080p.NF.WEBRip.DDP2.0.x264-NTb", "Dorohedoro", "S01"},
	}
	for _, tc := range cases {
		info, amb := parseShowEntry(tc.name)
		if amb || info == nil {
			t.Fatalf("%q: got info=%v amb=%v", tc.name, info, amb)
		}
		if !info.SeasonOnly || info.ShowName != tc.show || info.Season != tc.season {
			t.Fatalf("%q: got %+v", tc.name, info)
		}
	}
}

func TestFindShowFolderCandidates_NoCrossStyleNorm(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"The.Boys", "The Boys (2019)"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	candidates := findShowFolderCandidates(dir, "The Boys (2019)")
	var normCount int
	for _, c := range candidates {
		if c.NormMatch {
			normCount++
		}
	}
	if normCount != 1 {
		t.Fatalf("expected exactly one norm match for release name, got %d candidates", normCount)
	}
}

func TestFindBestFuzzyShowFolder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"The.Boys", "Breaking.Bad"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	folder, score, ok := findBestFuzzyShowFolder(dir, "the.boys")
	if !ok || folder != "The.Boys" || score < 1.0 {
		t.Fatalf("the.boys: got folder=%q score=%v ok=%v", folder, score, ok)
	}

	_, _, ok = findBestFuzzyShowFolder(dir, "The Boys (2019)")
	if ok {
		t.Fatal("The Boys (2019) should not norm-match The.Boys without The Boys (2019) folder in dest")
	}

	_, _, ok = findBestFuzzyShowFolder(dir, "The Pitt")
	if ok {
		t.Fatal("The Pitt should not match The.Boys or Breaking.Bad")
	}
}

func TestResolveShowDestFolder_AutoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Georgie.and.Mandys.First.Marriage"), 0755); err != nil {
		t.Fatal(err)
	}
	got := resolveShowDestFolder(dir, "Georgie.Mandy.s.First.Marriage", false, nil)
	if got != "Georgie.and.Mandys.First.Marriage" {
		t.Fatalf("expected auto-match to existing folder, got %q", got)
	}
}

func TestStripMediaExtensionFromDestFolder(t *testing.T) {
	// Folder named with a media extension should have it stripped in the destination.
	if got := stripMediaExtensions("Show.Name.S01E01.mkv"); got != "Show.Name.S01E01" {
		t.Fatalf("stripMediaExtensions: got %q", got)
	}
	if got := stripMediaExtensions("Show.Name.S01E01"); got != "Show.Name.S01E01" {
		t.Fatalf("stripMediaExtensions (no ext): got %q", got)
	}
}

func TestParseShowEntry_ReleaseStyle(t *testing.T) {
	tests := []struct {
		name      string
		show      string
		season    string
		episode   string
		seasonOnly bool
	}{
		{
			name:    "The Boys (2019) S02E08 What I Know (1080p AMZN Webrip x265 10bit EAC3 5.1 - Erie) [TAoE].mkv",
			show:    "The Boys (2019)",
			season:  "S02",
			episode: "S02E08",
		},
		{
			name:       "ted (2024) S01 (1080p BDRip x265 10bit EAC3 5.1 English - JBENT)[TAoE]",
			show:       "ted (2024)",
			season:     "S01",
			seasonOnly: true,
		},
		{
			name:    "The Boys (2019) S02E07 Butcher, Baker, Candlestick Maker (1080p AMZN Webrip x265 10bit EAC3 5.1 - Erie) [TAoE].mkv",
			show:    "The Boys (2019)",
			season:  "S02",
			episode: "S02E07",
		},
		{
			name:    "Marshals.S01E12.The.Devil.mkv",
			show:    "Marshals",
			season:  "S01",
			episode: "S01E12",
		},
	}

	for _, tt := range tests {
		info, amb := parseShowEntry(tt.name)
		if amb {
			t.Fatalf("%q: unexpected ambiguous", tt.name)
		}
		if info == nil {
			t.Fatalf("%q: expected match", tt.name)
		}
		if info.ShowName != tt.show || info.Season != tt.season {
			t.Fatalf("%q: got show=%q season=%q, want show=%q season=%q", tt.name, info.ShowName, info.Season, tt.show, tt.season)
		}
		if info.SeasonOnly != tt.seasonOnly {
			t.Fatalf("%q: seasonOnly=%v want %v", tt.name, info.SeasonOnly, tt.seasonOnly)
		}
		if !tt.seasonOnly && info.SeasonEpisode != tt.episode {
			t.Fatalf("%q: episode=%q want %q", tt.name, info.SeasonEpisode, tt.episode)
		}
	}
}
