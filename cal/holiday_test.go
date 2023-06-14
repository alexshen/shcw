package cal

import (
	"testing"
	"time"
)

func TestJsonCalendar(t *testing.T) {
	calendar, err := NewJsonCalendar("holidays.json")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		Date     string
		expected bool
	}{
		{"2023-01-01", true},
		{"2023-01-03", false},
	}
	for _, test := range tests {
		d, err := time.ParseInLocation(time.DateOnly, test.Date, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		holiday, err := calendar.IsHoliday(d)
		if err != nil {
			t.Fatal(err)
		}
		if holiday != test.expected {
			t.Errorf("IsHoliday(%s) returns %v, expects %v", test.Date, holiday, test.expected)
		}
	}
}
