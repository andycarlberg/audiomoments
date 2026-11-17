package clock_test

import (
	"testing"
	"time"

	"github.com/andycarlberg/audiomoments/internal/clock"
)

func TestEvaluate_DaypartBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		hour        int
		minute      int
		second      int
		wantDaypart clock.Daypart
	}{
		{name: "00:00:00 start of day", hour: 0, minute: 0, second: 0, wantDaypart: clock.DaypartNight},
		{name: "05:59:59 end of night", hour: 5, minute: 59, second: 59, wantDaypart: clock.DaypartNight},
		{name: "06:00:00 start of morning", hour: 6, minute: 0, second: 0, wantDaypart: clock.DaypartMorning},
		{name: "09:29:59 end of morning", hour: 9, minute: 29, second: 59, wantDaypart: clock.DaypartMorning},
		{name: "09:30:00 start of midday", hour: 9, minute: 30, second: 0, wantDaypart: clock.DaypartMidday},
		{name: "13:59:59 end of midday", hour: 13, minute: 59, second: 59, wantDaypart: clock.DaypartMidday},
		{name: "14:00:00 start of afternoon", hour: 14, minute: 0, second: 0, wantDaypart: clock.DaypartAfternoon},
		{name: "17:29:59 end of afternoon", hour: 17, minute: 29, second: 59, wantDaypart: clock.DaypartAfternoon},
		{name: "17:30:00 start of evening", hour: 17, minute: 30, second: 0, wantDaypart: clock.DaypartEvening},
		{name: "20:59:59 end of evening", hour: 20, minute: 59, second: 59, wantDaypart: clock.DaypartEvening},
		{name: "21:00:00 start of night", hour: 21, minute: 0, second: 0, wantDaypart: clock.DaypartNight},
		{name: "23:59:59 end of day", hour: 23, minute: 59, second: 59, wantDaypart: clock.DaypartNight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := time.Date(2026, time.November, 18, tt.hour, tt.minute, tt.second, 0, time.UTC)
			got := clock.Evaluate(input)

			if got.DayType != clock.DayTypeWeekday {
				t.Errorf("expected DayType %q, got %q", clock.DayTypeWeekday, got.DayType)
			}
			if got.Daypart != tt.wantDaypart {
				t.Errorf("expected Daypart %q, got %q", tt.wantDaypart, got.Daypart)
			}
		})
	}
}

func TestEvaluate_DayTypes(t *testing.T) {
	tests := []struct {
		name        string
		day         int
		wantWeekday time.Weekday
		wantDayType clock.DayType
	}{
		{name: "Monday", day: 16, wantWeekday: time.Monday, wantDayType: clock.DayTypeWeekday},
		{name: "Tuesday", day: 17, wantWeekday: time.Tuesday, wantDayType: clock.DayTypeWeekday},
		{name: "Wednesday", day: 18, wantWeekday: time.Wednesday, wantDayType: clock.DayTypeWeekday},
		{name: "Thursday", day: 19, wantWeekday: time.Thursday, wantDayType: clock.DayTypeWeekday},
		{name: "Friday", day: 20, wantWeekday: time.Friday, wantDayType: clock.DayTypeFriday},
		{name: "Saturday", day: 21, wantWeekday: time.Saturday, wantDayType: clock.DayTypeSaturday},
		{name: "Sunday", day: 22, wantWeekday: time.Sunday, wantDayType: clock.DayTypeSunday},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := time.Date(2026, time.November, tt.day, 12, 0, 0, 0, time.UTC)
			if input.Weekday() != tt.wantWeekday {
				t.Fatalf("test fixture bug: expected weekday %v, got %v", tt.wantWeekday, input.Weekday())
			}

			got := clock.Evaluate(input)
			if got.DayType != tt.wantDayType {
				t.Errorf("expected DayType %q, got %q", tt.wantDayType, got.DayType)
			}
		})
	}
}

func TestEvaluate_TimezoneLocalizedInput(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("failed to load America/Chicago: %v", err)
	}
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("failed to load Asia/Tokyo: %v", err)
	}

	tests := []struct {
		name        string
		utc         time.Time
		loc         *time.Location
		wantDayType clock.DayType
		wantDaypart clock.Daypart
	}{
		{
			name:        "Saturday 02:00 UTC in Chicago is Friday 20:00 CST",
			utc:         time.Date(2026, time.November, 21, 2, 0, 0, 0, time.UTC),
			loc:         chicago,
			wantDayType: clock.DayTypeFriday,
			wantDaypart: clock.DaypartEvening,
		},
		{
			name:        "Thursday 23:00 UTC in Tokyo is Friday 08:00 JST",
			utc:         time.Date(2026, time.November, 19, 23, 0, 0, 0, time.UTC),
			loc:         tokyo,
			wantDayType: clock.DayTypeFriday,
			wantDaypart: clock.DaypartMorning,
		},
		{
			name:        "Winter 15:00 UTC in Chicago is 09:00 CST (morning)",
			utc:         time.Date(2026, time.January, 15, 15, 0, 0, 0, time.UTC),
			loc:         chicago,
			wantDayType: clock.DayTypeWeekday,
			wantDaypart: clock.DaypartMorning,
		},
		{
			name:        "Summer 15:00 UTC in Chicago is 10:00 CDT (midday)",
			utc:         time.Date(2026, time.July, 15, 15, 0, 0, 0, time.UTC),
			loc:         chicago,
			wantDayType: clock.DayTypeWeekday,
			wantDaypart: clock.DaypartMidday,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clock.Evaluate(tt.utc.In(tt.loc))
			if got.DayType != tt.wantDayType {
				t.Errorf("expected DayType %q, got %q", tt.wantDayType, got.DayType)
			}
			if got.Daypart != tt.wantDaypart {
				t.Errorf("expected Daypart %q, got %q", tt.wantDaypart, got.Daypart)
			}
		})
	}
}

func TestIsValidDaypart(t *testing.T) {
	tests := []struct {
		name    string
		daypart clock.Daypart
		want    bool
	}{
		{name: "morning", daypart: clock.DaypartMorning, want: true},
		{name: "midday", daypart: clock.DaypartMidday, want: true},
		{name: "afternoon", daypart: clock.DaypartAfternoon, want: true},
		{name: "evening", daypart: clock.DaypartEvening, want: true},
		{name: "night", daypart: clock.DaypartNight, want: true},
		{name: "empty", daypart: "", want: false},
		{name: "uppercase", daypart: "MORNING", want: false},
		{name: "commute", daypart: "commute", want: false},
		{name: "unknown", daypart: "unknown", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clock.IsValidDaypart(tt.daypart); got != tt.want {
				t.Errorf("IsValidDaypart(%q) = %v, want %v", tt.daypart, got, tt.want)
			}
		})
	}
}

func TestIsValidDayType(t *testing.T) {
	tests := []struct {
		name    string
		dayType clock.DayType
		want    bool
	}{
		{name: "weekday", dayType: clock.DayTypeWeekday, want: true},
		{name: "friday", dayType: clock.DayTypeFriday, want: true},
		{name: "saturday", dayType: clock.DayTypeSaturday, want: true},
		{name: "sunday", dayType: clock.DayTypeSunday, want: true},
		{name: "empty", dayType: "", want: false},
		{name: "weekend", dayType: "weekend", want: false},
		{name: "monday", dayType: "monday", want: false},
		{name: "uppercase", dayType: "FRIDAY", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clock.IsValidDayType(tt.dayType); got != tt.want {
				t.Errorf("IsValidDayType(%q) = %v, want %v", tt.dayType, got, tt.want)
			}
		})
	}
}
