// Package station defines the AudioMoments station schema, seed templates, and persistence helpers.
package station

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/andycarlberg/audiomoments/internal/clock"
)

// Archetype classifies the situational listening intent of a station.
type Archetype string

// Supported station listening intent archetypes.
const (
	ArchetypeUpbeat Archetype = "upbeat"
	ArchetypeFocus  Archetype = "focus"
	ArchetypeMellow Archetype = "mellow"
)

// IsValidArchetype reports whether a is a recognized Archetype constant.
func IsValidArchetype(a Archetype) bool {
	switch a {
	case ArchetypeUpbeat, ArchetypeFocus, ArchetypeMellow:
		return true
	default:
		return false
	}
}

// Station defines a curated set of Subsonic track IDs targeting specific dayparts and an intent archetype.
type Station struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	Prompt          string          `json:"prompt"`
	Dayparts        []clock.Daypart `json:"dayparts"`
	Archetype       Archetype       `json:"archetype"`
	TrackIDs        []string        `json:"track_ids"`
	MinViableTracks int             `json:"min_viable_tracks"`
}

// SeedStations returns a freshly allocated slice of the 6 unfulfilled seed station templates.
func SeedStations() []Station {
	return []Station{
		{
			ID:              "morning-upbeat",
			Title:           "Espresso & Indie Drive",
			Prompt:          "Upbeat morning indie rock, energetic synthpop, and driving rhythms for waking up and commuting",
			Dayparts:        []clock.Daypart{clock.DaypartMorning, clock.DaypartAfternoon},
			Archetype:       ArchetypeUpbeat,
			MinViableTracks: 5,
		},
		{
			ID:              "evening-upbeat",
			Title:           "Golden Hour Groove",
			Prompt:          "Feel-good upbeat funk, nu-disco, and lively melodic grooves for midday momentum and evening socializing",
			Dayparts:        []clock.Daypart{clock.DaypartMidday, clock.DaypartEvening, clock.DaypartNight},
			Archetype:       ArchetypeUpbeat,
			MinViableTracks: 5,
		},
		{
			ID:              "workday-focus",
			Title:           "Deep Work Synth & Pulse",
			Prompt:          "Instrumental electronic, steady ambient techno, and lyric-free post-rock for deep workday concentration",
			Dayparts:        []clock.Daypart{clock.DaypartMorning, clock.DaypartMidday, clock.DaypartAfternoon},
			Archetype:       ArchetypeFocus,
			MinViableTracks: 5,
		},
		{
			ID:              "evening-focus",
			Title:           "Late Night Ambient Study",
			Prompt:          "Atmospheric minimalist piano, warm analog synth pads, and calm lyric-free soundscapes for evening and late-night focus",
			Dayparts:        []clock.Daypart{clock.DaypartEvening, clock.DaypartNight},
			Archetype:       ArchetypeFocus,
			MinViableTracks: 5,
		},
		{
			ID:              "morning-mellow",
			Title:           "Acoustic Sunrise",
			Prompt:          "Gentle acoustic guitar, warm folk, and soft vocal melodies for an easy morning wake-up and relaxed midday listening",
			Dayparts:        []clock.Daypart{clock.DaypartMorning, clock.DaypartMidday},
			Archetype:       ArchetypeMellow,
			MinViableTracks: 5,
		},
		{
			ID:              "evening-mellow",
			Title:           "Fireside Unwind",
			Prompt:          "Mellow downtempo, laid-back indie folk, and warm evening acoustic tracks for unwinding after work",
			Dayparts:        []clock.Daypart{clock.DaypartAfternoon, clock.DaypartEvening, clock.DaypartNight},
			Archetype:       ArchetypeMellow,
			MinViableTracks: 5,
		},
	}
}

func isValidStationID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// Validate checks that all required Station fields are well-formed and meet viability constraints.
func (s Station) Validate() error {
	if !isValidStationID(s.ID) {
		return fmt.Errorf("invalid station id %q: must be non-empty and contain only alphanumeric characters, hyphens, or underscores", s.ID)
	}
	if !utf8.ValidString(s.Title) || strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("station %q: title must be non-empty valid UTF-8", s.ID)
	}
	if !utf8.ValidString(s.Prompt) || strings.TrimSpace(s.Prompt) == "" {
		return fmt.Errorf("station %q: prompt must be non-empty valid UTF-8", s.ID)
	}
	if !IsValidArchetype(s.Archetype) {
		return fmt.Errorf("station %q: invalid archetype %q", s.ID, s.Archetype)
	}
	if len(s.Dayparts) == 0 {
		return fmt.Errorf("station %q: at least one daypart is required", s.ID)
	}

	seenDayparts := make(map[clock.Daypart]struct{}, len(s.Dayparts))
	for _, dp := range s.Dayparts {
		if !clock.IsValidDaypart(dp) {
			return fmt.Errorf("station %q: invalid daypart %q", s.ID, dp)
		}
		if _, exists := seenDayparts[dp]; exists {
			return fmt.Errorf("station %q: duplicate daypart %q", s.ID, dp)
		}
		seenDayparts[dp] = struct{}{}
	}

	if s.MinViableTracks <= 0 {
		return fmt.Errorf("station %q: min_viable_tracks must be > 0, got %d", s.ID, s.MinViableTracks)
	}
	if len(s.TrackIDs) < s.MinViableTracks {
		return fmt.Errorf("station %q: has %d tracks, requires at least %d", s.ID, len(s.TrackIDs), s.MinViableTracks)
	}

	seenTracks := make(map[string]struct{}, len(s.TrackIDs))
	for i, trackID := range s.TrackIDs {
		if !utf8.ValidString(trackID) || strings.TrimSpace(trackID) == "" {
			return fmt.Errorf("station %q: track_ids[%d] must be non-blank valid UTF-8", s.ID, i)
		}
		if _, exists := seenTracks[trackID]; exists {
			return fmt.Errorf("station %q: duplicate track_id %q", s.ID, trackID)
		}
		seenTracks[trackID] = struct{}{}
	}

	return nil
}

// SaveFile validates s and atomically writes it as indented JSON to <dir>/<s.ID>.json.
func SaveFile(dir string, s Station) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("save station %q: %w", s.ID, err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create station bank dir %q: %w", dir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal station %q: %w", s.ID, err)
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(dir, s.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp station file in %q: %w", dir, err)
	}
	tmpPath := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp station %q: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp station %q: %w", tmpPath, err)
	}

	path := filepath.Join(dir, s.ID+".json")
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace station %q: %w", path, err)
	}
	cleanupTmp = false
	return nil
}

// LoadFile reads, strictly JSON-decodes, and validates a single station definition file.
func LoadFile(path string) (Station, error) {
	f, err := os.Open(path)
	if err != nil {
		return Station{}, fmt.Errorf("open station %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()

	var s Station
	if err := dec.Decode(&s); err != nil {
		return Station{}, fmt.Errorf("decode station %q: %w", path, err)
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Station{}, fmt.Errorf("decode station %q: unexpected trailing JSON data", path)
		}
		return Station{}, fmt.Errorf("decode station %q: %w", path, err)
	}

	if err := s.Validate(); err != nil {
		return Station{}, fmt.Errorf("validate station %q: %w", path, err)
	}

	return s, nil
}

// LoadBank loads and validates all .json station files from dir in deterministic filename order.
func LoadBank(dir string) ([]Station, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read station bank %q: %w", dir, err)
	}

	var stations []Station
	seenIDs := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		s, err := LoadFile(path)
		if err != nil {
			return nil, err
		}

		if prevFile, exists := seenIDs[s.ID]; exists {
			return nil, fmt.Errorf("duplicate station id %q in %q (already defined in %q)", s.ID, entry.Name(), prevFile)
		}
		seenIDs[s.ID] = entry.Name()
		stations = append(stations, s)
	}

	if len(stations) == 0 {
		return nil, fmt.Errorf("station bank %q contains no .json station files", dir)
	}

	return stations, nil
}
