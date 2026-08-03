package routines

import (
	"testing"
	"time"
)

func BenchmarkNextRun(b *testing.B) {
	schedule := Schedule{Kind: ScheduleWeekdays, At: "08:30", Timezone: "Europe/Berlin"}
	now := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if _, err := NextRun(schedule, now, time.Time{}); err != nil {
			b.Fatal(err)
		}
	}
}
