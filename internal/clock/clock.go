// Package clock maps timestamps into AudioMoments daypart windows and day types.
package clock

import "time"

// Daypart represents one of the five daily situational listening windows.
type Daypart string

// Recognized daily situational listening windows.
const (
	DaypartMorning   Daypart = "morning"
	DaypartMidday    Daypart = "midday"
	DaypartAfternoon Daypart = "afternoon"
	DaypartEvening   Daypart = "evening"
	DaypartNight     Daypart = "night"
)

// DayType represents one of the four weekly temporal profiles.
type DayType string

// Recognized weekly temporal profiles.
const (
	DayTypeWeekday  DayType = "weekday"
	DayTypeFriday   DayType = "friday"
	DayTypeSaturday DayType = "saturday"
	DayTypeSunday   DayType = "sunday"
)

// Boundary constants in minutes since midnight (hour*60 + minute).
const (
	morningStartMinutes   = 6 * 60     // 06:00 (360)
	middayStartMinutes    = 9*60 + 30  // 09:30 (570)
	afternoonStartMinutes = 14 * 60    // 14:00 (840)
	eveningStartMinutes   = 17*60 + 30 // 17:30 (1050)
	nightStartMinutes     = 21 * 60    // 21:00 (1260)
)

// Moment captures the resolved DayType and Daypart for a point in time.
type Moment struct {
	DayType DayType
	Daypart Daypart
}

// Evaluate maps t (using its embedded time.Location) to its active Moment.
func Evaluate(t time.Time) Moment {
	return Moment{
		DayType: evaluateDayType(t.Weekday()),
		Daypart: evaluateDaypart(t.Hour(), t.Minute()),
	}
}

func evaluateDayType(weekday time.Weekday) DayType {
	switch weekday {
	case time.Friday:
		return DayTypeFriday
	case time.Saturday:
		return DayTypeSaturday
	case time.Sunday:
		return DayTypeSunday
	default:
		return DayTypeWeekday
	}
}

func evaluateDaypart(hour, minute int) Daypart {
	minutes := hour*60 + minute
	switch {
	case minutes >= morningStartMinutes && minutes < middayStartMinutes:
		return DaypartMorning
	case minutes >= middayStartMinutes && minutes < afternoonStartMinutes:
		return DaypartMidday
	case minutes >= afternoonStartMinutes && minutes < eveningStartMinutes:
		return DaypartAfternoon
	case minutes >= eveningStartMinutes && minutes < nightStartMinutes:
		return DaypartEvening
	default:
		return DaypartNight
	}
}

// IsValidDaypart reports whether d is a recognized Daypart constant.
func IsValidDaypart(d Daypart) bool {
	switch d {
	case DaypartMorning, DaypartMidday, DaypartAfternoon, DaypartEvening, DaypartNight:
		return true
	default:
		return false
	}
}

// IsValidDayType reports whether d is a recognized DayType constant.
func IsValidDayType(d DayType) bool {
	switch d {
	case DayTypeWeekday, DayTypeFriday, DayTypeSaturday, DayTypeSunday:
		return true
	default:
		return false
	}
}
