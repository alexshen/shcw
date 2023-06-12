package api

import (
	"strings"
	"time"
)

type JsonDate time.Time

func timeToJson(t time.Time, format string) []byte {
	if t.IsZero() {
		return []byte(`""`)
	}
	return []byte(`"` + t.Format(format) + `"`)
}

func jsonToTime(data []byte, format string) (time.Time, error) {
	value := strings.Trim(string(data), `"`)
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation(format, value, time.Local)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

func (d *JsonDate) MarshalJSON() ([]byte, error) {
	return timeToJson(time.Time(*d), time.DateOnly), nil
}

func (d *JsonDate) UnmarshalJSON(data []byte) error {
	t, err := jsonToTime(data, time.DateOnly)
	if err != nil {
		return err
	}
	*d = JsonDate(t)
	return nil
}

type JsonDateTime time.Time

func (d *JsonDateTime) MarshalJSON() ([]byte, error) {
	return timeToJson(time.Time(*d), time.DateTime), nil
}

func (d *JsonDateTime) UnmarshalJSON(data []byte) error {
	t, err := jsonToTime(data, time.DateTime)
	if err != nil {
		return err
	}
	*d = JsonDateTime(t)
	return nil
}
