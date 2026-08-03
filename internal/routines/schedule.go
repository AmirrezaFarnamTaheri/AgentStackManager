package routines

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func NextRun(schedule Schedule, now, lastRun time.Time) (time.Time, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	switch schedule.Kind {
	case ScheduleManual:
		return time.Time{}, nil
	case ScheduleInterval:
		if schedule.EveryMinutes < 1 || schedule.EveryMinutes > 525600 {
			return time.Time{}, fmt.Errorf("interval minutes must be between 1 and 525600")
		}
		base := now
		if !lastRun.IsZero() && lastRun.After(base) {
			base = lastRun
		}
		return base.Add(time.Duration(schedule.EveryMinutes) * time.Minute).UTC(), nil
	case ScheduleDaily, ScheduleWeekdays:
		location, err := scheduleLocation(schedule.Timezone)
		if err != nil {
			return time.Time{}, err
		}
		hour, minute, err := parseClock(schedule.At)
		if err != nil {
			return time.Time{}, err
		}
		localNow := now.In(location)
		candidate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, location)
		if !candidate.After(localNow) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		if schedule.Kind == ScheduleWeekdays {
			for candidate.Weekday() == time.Saturday || candidate.Weekday() == time.Sunday {
				candidate = candidate.AddDate(0, 0, 1)
			}
		}
		return candidate.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported schedule kind %q", schedule.Kind)
	}
}

func scheduleLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "UTC"
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load schedule timezone %q: %w", name, err)
	}
	return location, nil
}

func parseClock(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("schedule time must use HH:MM")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("schedule hour is invalid")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("schedule minute is invalid")
	}
	return hour, minute, nil
}
