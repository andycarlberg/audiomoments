package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultStationBankPath is the fallback directory where station JSON fixtures live.
const DefaultStationBankPath = "./stations"

// Config encapsulates the runtime settings for AudioMoments.
type Config struct {
	SubsonicURL      string
	SubsonicUser     string
	SubsonicPassword string
	StationBankPath  string
	Timezone         string
	Location         *time.Location
}

// Load populates configuration from the process environment, validating required fields
// and parsing the target IANA timezone location.
func Load() (*Config, error) {
	rawURL := strings.TrimSpace(os.Getenv("SUBSONIC_URL"))
	if rawURL == "" {
		return nil, fmt.Errorf("SUBSONIC_URL is required")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid SUBSONIC_URL %q: must be a valid http or https URL", rawURL)
	}
	subsonicURL := strings.TrimRight(rawURL, "/")

	subsonicUser := strings.TrimSpace(os.Getenv("SUBSONIC_USER"))
	if subsonicUser == "" {
		return nil, fmt.Errorf("SUBSONIC_USER is required")
	}

	subsonicPassword := os.Getenv("SUBSONIC_PASSWORD")
	if subsonicPassword == "" {
		return nil, fmt.Errorf("SUBSONIC_PASSWORD is required")
	}

	stationBankPath := strings.TrimSpace(os.Getenv("STATION_BANK_PATH"))
	if stationBankPath == "" {
		stationBankPath = DefaultStationBankPath
	}

	tzStr := strings.TrimSpace(os.Getenv("TIMEZONE"))
	var loc *time.Location
	if tzStr != "" {
		parsedLoc, err := time.LoadLocation(tzStr)
		if err != nil {
			return nil, fmt.Errorf("invalid TIMEZONE %q: %w", tzStr, err)
		}
		loc = parsedLoc
	} else {
		loc = time.Local
		tzStr = loc.String()
	}

	return &Config{
		SubsonicURL:      subsonicURL,
		SubsonicUser:     subsonicUser,
		SubsonicPassword: subsonicPassword,
		StationBankPath:  stationBankPath,
		Timezone:         tzStr,
		Location:         loc,
	}, nil
}
