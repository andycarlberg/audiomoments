package station_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/andycarlberg/audiomoments/internal/clock"
	"github.com/andycarlberg/audiomoments/internal/station"
)

func TestIsValidArchetype(t *testing.T) {
	tests := []struct {
		name      string
		archetype station.Archetype
		want      bool
	}{
		{name: "upbeat", archetype: station.ArchetypeUpbeat, want: true},
		{name: "focus", archetype: station.ArchetypeFocus, want: true},
		{name: "mellow", archetype: station.ArchetypeMellow, want: true},
		{name: "empty", archetype: "", want: false},
		{name: "uppercase", archetype: "UPBEAT", want: false},
		{name: "chill", archetype: "chill", want: false},
		{name: "unknown", archetype: "unknown", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := station.IsValidArchetype(tt.archetype); got != tt.want {
				t.Errorf("IsValidArchetype(%q) = %v, want %v", tt.archetype, got, tt.want)
			}
		})
	}
}

func validBaseStation() station.Station {
	return station.Station{
		ID:              "morning-upbeat",
		Title:           "Espresso & Indie Drive",
		Prompt:          "Upbeat morning indie rock and driving rhythms for waking up and commuting",
		Dayparts:        []clock.Daypart{clock.DaypartMorning, clock.DaypartAfternoon},
		Archetype:       station.ArchetypeUpbeat,
		TrackIDs:        []string{"track-1", "track-2", "track-3"},
		MinViableTracks: 3,
	}
}

func TestStation_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*station.Station)
		wantErr bool
	}{
		{
			name:    "valid station",
			mutate:  func(*station.Station) {},
			wantErr: false,
		},
		{
			name:    "valid ID with uppercase, digits, and underscores",
			mutate:  func(s *station.Station) { s.ID = "Station_01-Focus_9" },
			wantErr: false,
		},
		{
			name:    "empty ID",
			mutate:  func(s *station.Station) { s.ID = "" },
			wantErr: true,
		},
		{
			name:    "whitespace ID",
			mutate:  func(s *station.Station) { s.ID = "   " },
			wantErr: true,
		},
		{
			name:    "path traversal ID",
			mutate:  func(s *station.Station) { s.ID = "../escape" },
			wantErr: true,
		},
		{
			name:    "slash in ID",
			mutate:  func(s *station.Station) { s.ID = "sub/station" },
			wantErr: true,
		},
		{
			name:    "padded whitespace ID",
			mutate:  func(s *station.Station) { s.ID = " morning-upbeat " },
			wantErr: true,
		},
		{
			name:    "empty Title",
			mutate:  func(s *station.Station) { s.Title = "" },
			wantErr: true,
		},
		{
			name:    "whitespace Title",
			mutate:  func(s *station.Station) { s.Title = "   " },
			wantErr: true,
		},
		{
			name:    "invalid UTF-8 Title",
			mutate:  func(s *station.Station) { s.Title = "Bad\xffTitle" },
			wantErr: true,
		},
		{
			name:    "empty Prompt",
			mutate:  func(s *station.Station) { s.Prompt = "" },
			wantErr: true,
		},
		{
			name:    "whitespace Prompt",
			mutate:  func(s *station.Station) { s.Prompt = "   " },
			wantErr: true,
		},
		{
			name:    "invalid UTF-8 Prompt",
			mutate:  func(s *station.Station) { s.Prompt = "Bad\xffPrompt" },
			wantErr: true,
		},
		{
			name:    "empty Archetype",
			mutate:  func(s *station.Station) { s.Archetype = "" },
			wantErr: true,
		},
		{
			name:    "invalid Archetype",
			mutate:  func(s *station.Station) { s.Archetype = "chill" },
			wantErr: true,
		},
		{
			name:    "nil Dayparts",
			mutate:  func(s *station.Station) { s.Dayparts = nil },
			wantErr: true,
		},
		{
			name:    "empty Dayparts",
			mutate:  func(s *station.Station) { s.Dayparts = []clock.Daypart{} },
			wantErr: true,
		},
		{
			name:    "invalid Daypart element",
			mutate:  func(s *station.Station) { s.Dayparts = []clock.Daypart{clock.DaypartMorning, "commute"} },
			wantErr: true,
		},
		{
			name:    "duplicate Daypart element",
			mutate:  func(s *station.Station) { s.Dayparts = []clock.Daypart{clock.DaypartMorning, clock.DaypartMorning} },
			wantErr: true,
		},
		{
			name:    "zero MinViableTracks",
			mutate:  func(s *station.Station) { s.MinViableTracks = 0 },
			wantErr: true,
		},
		{
			name:    "negative MinViableTracks",
			mutate:  func(s *station.Station) { s.MinViableTracks = -1 },
			wantErr: true,
		},
		{
			name: "fewer tracks than MinViableTracks",
			mutate: func(s *station.Station) {
				s.MinViableTracks = 3
				s.TrackIDs = []string{"t1", "t2"}
			},
			wantErr: true,
		},
		{
			name:    "blank track ID",
			mutate:  func(s *station.Station) { s.TrackIDs = []string{"t1", "  ", "t3"} },
			wantErr: true,
		},
		{
			name:    "invalid UTF-8 track ID",
			mutate:  func(s *station.Station) { s.TrackIDs = []string{"t1", "\xff", "\xfe"} },
			wantErr: true,
		},
		{
			name:    "duplicate track ID",
			mutate:  func(s *station.Station) { s.TrackIDs = []string{"t1", "t1", "t3"} },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validBaseStation()
			tt.mutate(&s)
			err := s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
	return path
}

func TestLoadFile_And_LoadBank(t *testing.T) {
	t.Run("loads valid station files in filename order and ignores non-json files and subdirs", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "README.md", "# ignore me")
		if err := os.Mkdir(filepath.Join(dir, "nested.json"), 0o750); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}

		writeTestFile(t, dir, "02_second.json", `{
  "id": "evening-mellow",
  "title": "Fireside Unwind",
  "prompt": "Test situational prompt",
  "dayparts": ["evening", "night"],
  "archetype": "mellow",
  "track_ids": ["t1", "t2"],
  "min_viable_tracks": 2
}`)
		writeTestFile(t, dir, "01_first.json", `{
  "id": "morning-upbeat",
  "title": "Espresso & Indie Drive",
  "prompt": "Test situational prompt",
  "dayparts": ["morning"],
  "archetype": "upbeat",
  "track_ids": ["t3", "t4"],
  "min_viable_tracks": 2
}`)

		stations, err := station.LoadBank(dir)
		if err != nil {
			t.Fatalf("LoadBank() unexpected error: %v", err)
		}
		if len(stations) != 2 {
			t.Fatalf("expected 2 stations, got %d", len(stations))
		}
		if stations[0].ID != "morning-upbeat" || stations[1].ID != "evening-mellow" {
			t.Errorf("expected deterministic filename order [morning-upbeat, evening-mellow], got [%s, %s]", stations[0].ID, stations[1].ID)
		}
	})

	t.Run("rejects unknown JSON fields", func(t *testing.T) {
		dir := t.TempDir()
		path := writeTestFile(t, dir, "unknown.json", `{
  "id": "morning-upbeat",
  "title": "Espresso & Indie Drive",
  "prompt": "Test situational prompt",
  "dayparts": ["morning"],
  "archetype": "upbeat",
  "track_ids": ["t1"],
  "min_viable_tracks": 1,
  "unexpected_field": "value"
}`)
		if _, err := station.LoadFile(path); err == nil {
			t.Fatal("LoadFile() expected error for unknown JSON field, got nil")
		}
	})

	t.Run("rejects trailing JSON data after root object", func(t *testing.T) {
		dir := t.TempDir()
		path := writeTestFile(t, dir, "trailing.json", `{
  "id": "morning-upbeat",
  "title": "Espresso & Indie Drive",
  "prompt": "Test situational prompt",
  "dayparts": ["morning"],
  "archetype": "upbeat",
  "track_ids": ["t1"],
  "min_viable_tracks": 1
}{"extra": true}`)
		if _, err := station.LoadFile(path); err == nil {
			t.Fatal("LoadFile() expected error for trailing JSON data, got nil")
		}
	})

	t.Run("rejects trailing malformed syntax after root object", func(t *testing.T) {
		dir := t.TempDir()
		path := writeTestFile(t, dir, "trailing_syntax.json", `{
  "id": "morning-upbeat",
  "title": "Espresso & Indie Drive",
  "prompt": "Test situational prompt",
  "dayparts": ["morning"],
  "archetype": "upbeat",
  "track_ids": ["t1"],
  "min_viable_tracks": 1
} invalid-trailing-token`)
		if _, err := station.LoadFile(path); err == nil {
			t.Fatal("LoadFile() expected error for trailing malformed syntax, got nil")
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := writeTestFile(t, dir, "malformed.json", `{"id": "broken"`)
		if _, err := station.LoadFile(path); err == nil {
			t.Fatal("LoadFile() expected error for malformed JSON, got nil")
		}
	})

	t.Run("rejects invalid station fields", func(t *testing.T) {
		dir := t.TempDir()
		path := writeTestFile(t, dir, "invalid.json", `{
  "id": "bad-station",
  "title": "Bad Station",
  "prompt": "Test situational prompt",
  "dayparts": ["morning"],
  "archetype": "chill",
  "track_ids": ["t1"],
  "min_viable_tracks": 1
}`)
		if _, err := station.LoadFile(path); err == nil {
			t.Fatal("LoadFile() expected error for invalid station, got nil")
		}
		if _, err := station.LoadBank(dir); err == nil {
			t.Fatal("LoadBank() expected error when directory contains invalid station file, got nil")
		}
	})

	t.Run("rejects duplicate station ID across files", func(t *testing.T) {
		dir := t.TempDir()
		content := `{
  "id": "duplicate-id",
  "title": "Station One",
  "prompt": "Test situational prompt",
  "dayparts": ["morning"],
  "archetype": "upbeat",
  "track_ids": ["t1"],
  "min_viable_tracks": 1
}`
		writeTestFile(t, dir, "a.json", content)
		writeTestFile(t, dir, "b.json", content)
		if _, err := station.LoadBank(dir); err == nil {
			t.Fatal("LoadBank() expected error for duplicate station ID across files, got nil")
		}
	})

	t.Run("fails on empty directory", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "notes.txt", "no json files here")
		if _, err := station.LoadBank(dir); err == nil {
			t.Fatal("LoadBank() expected error for directory with zero json files, got nil")
		}
	})

	t.Run("fails on non-existent directory", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		if _, err := station.LoadBank(missing); err == nil {
			t.Fatal("LoadBank() expected error for non-existent directory, got nil")
		}
	})

	t.Run("fails on non-existent file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.json")
		if _, err := station.LoadFile(missing); err == nil {
			t.Fatal("LoadFile() expected error for non-existent file, got nil")
		}
	})
}

func TestSaveFile(t *testing.T) {
	t.Run("writes valid station JSON and round-trips via LoadFile", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested-bank")
		want := validBaseStation()

		if err := station.SaveFile(dir, want); err != nil {
			t.Fatalf("SaveFile() unexpected error: %v", err)
		}

		path := filepath.Join(dir, want.ID+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected file %s to exist: %v", path, err)
		}

		got, err := station.LoadFile(path)
		if err != nil {
			t.Fatalf("LoadFile() round-trip failed: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", got, want)
		}

		want.Title = "Updated Title"
		if err := station.SaveFile(dir, want); err != nil {
			t.Fatalf("SaveFile() overwrite failed: %v", err)
		}
		got, err = station.LoadFile(path)
		if err != nil {
			t.Fatalf("LoadFile() overwrite round-trip failed: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("overwrite round-trip mismatch:\n got: %+v\nwant: %+v", got, want)
		}
	})

	t.Run("rejects unfulfilled or invalid station", func(t *testing.T) {
		dir := t.TempDir()
		unfulfilled := validBaseStation()
		unfulfilled.TrackIDs = nil

		if err := station.SaveFile(dir, unfulfilled); err == nil {
			t.Fatal("SaveFile() expected error for unfulfilled station, got nil")
		}

		path := filepath.Join(dir, unfulfilled.ID+".json")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected no file written on validation error, stat err = %v", err)
		}
	})

	t.Run("fails when target directory path is an existing regular file", func(t *testing.T) {
		fileAsDir := writeTestFile(t, t.TempDir(), "not-a-dir", "x")
		if err := station.SaveFile(fileAsDir, validBaseStation()); err == nil {
			t.Fatal("SaveFile() expected error when dir is a regular file, got nil")
		}
	})

	t.Run("fails when destination file path is an existing directory", func(t *testing.T) {
		dir := t.TempDir()
		s := validBaseStation()
		if err := os.Mkdir(filepath.Join(dir, s.ID+".json"), 0o750); err != nil {
			t.Fatalf("failed to create blocking dir: %v", err)
		}
		if err := station.SaveFile(dir, s); err == nil {
			t.Fatal("SaveFile() expected error when destination file path is a directory, got nil")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("os.ReadDir(%q) unexpected error: %v", dir, err)
		}
		if len(entries) != 1 || entries[0].Name() != s.ID+".json" {
			t.Errorf("expected only blocking dir in %q (no leaked temp files), got %v", dir, entries)
		}
	})
}

func assertDaypartArchetypeCoverage(t *testing.T, stations []station.Station) {
	t.Helper()

	dayparts := []clock.Daypart{
		clock.DaypartMorning,
		clock.DaypartMidday,
		clock.DaypartAfternoon,
		clock.DaypartEvening,
		clock.DaypartNight,
	}
	archetypes := []station.Archetype{
		station.ArchetypeUpbeat,
		station.ArchetypeFocus,
		station.ArchetypeMellow,
	}

	for _, dp := range dayparts {
		for _, arch := range archetypes {
			matches := 0
			for _, s := range stations {
				if s.Archetype != arch {
					continue
				}
				for _, sdp := range s.Dayparts {
					if sdp == dp {
						matches++
						break
					}
				}
			}
			if matches == 0 {
				t.Errorf("stations missing coverage for daypart %q and archetype %q", dp, arch)
			}
		}
	}
}

func TestSeedStations(t *testing.T) {
	seeds := station.SeedStations()
	if len(seeds) != 6 {
		t.Fatalf("expected 6 seed stations, got %d", len(seeds))
	}

	seenIDs := make(map[string]struct{}, len(seeds))
	for _, s := range seeds {
		if _, exists := seenIDs[s.ID]; exists {
			t.Errorf("duplicate seed station ID %q", s.ID)
		}
		seenIDs[s.ID] = struct{}{}

		if s.MinViableTracks != 5 {
			t.Errorf("seed station %q expected MinViableTracks = 5, got %d", s.ID, s.MinViableTracks)
		}
		if len(s.TrackIDs) != 0 {
			t.Errorf("seed station %q expected unfulfilled TrackIDs (len 0), got %v", s.ID, s.TrackIDs)
		}

		fulfilled := s
		fulfilled.TrackIDs = []string{"t1", "t2", "t3", "t4", "t5"}
		if err := fulfilled.Validate(); err != nil {
			t.Errorf("seed station %q failed Validate() once fulfilled: %v", s.ID, err)
		}
	}

	assertDaypartArchetypeCoverage(t, seeds)

	originalID := seeds[0].ID
	originalDaypart := seeds[0].Dayparts[0]
	seeds[0].ID = "mutated"
	seeds[0].Dayparts[0] = clock.DaypartNight

	fresh := station.SeedStations()
	if fresh[0].ID != originalID || fresh[0].Dayparts[0] != originalDaypart {
		t.Errorf("SeedStations() shares mutable backing state across calls")
	}
}

func TestSeedStationBank(t *testing.T) {
	stations, err := station.LoadBank("testdata")
	if err != nil {
		t.Fatalf("LoadBank(testdata) failed: %v", err)
	}

	if len(stations) != 6 {
		t.Fatalf("expected 6 seed fixture stations, got %d", len(stations))
	}

	assertDaypartArchetypeCoverage(t, stations)
}
