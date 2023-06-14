package cal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"github.com/alexshen/shcw/api"
)

type Calendar interface {
	IsHoliday(date time.Time) (bool, error)
}

type JsonCalendar struct {
	holidays map[int]map[time.Time]struct{}
}

func NewJsonCalendar(db string) (*JsonCalendar, error) {
	c := &JsonCalendar{
		holidays: make(map[int]map[time.Time]struct{}),
	}
	if err := c.load(db); err != nil {
		return nil, fmt.Errorf("JsonCalendar load error: %v", err)
	}
	return c, nil
}

func (c *JsonCalendar) load(db string) error {
	data, err := ioutil.ReadFile(path.Join(db))
	if err != nil {
		return err
	}
	var holidays []struct {
		Year   int `json:"year"`
		Ranges []struct {
			Begin api.JsonDateNoYear `json:"begin"`
			End   api.JsonDateNoYear `json:"end"`
		} `json:"dates"`
	}

	if err := json.Unmarshal(data, &holidays); err != nil {
		return err
	}
	for _, h := range holidays {
		dates := make(map[time.Time]struct{})
		c.holidays[h.Year] = dates
		for _, rng := range h.Ranges {
			beg := rng.Begin.WithYear(h.Year)
			end := rng.End.WithYear(h.Year)
			if beg.After(end) {
				return fmt.Errorf("year %d, range %v-%v", h.Year, rng.Begin, rng.End)
			}
			for !beg.After(end) {
				dates[beg] = struct{}{}
				beg = beg.Add(time.Hour * 24)
			}
		}
	}
	return nil
}

func (c *JsonCalendar) IsHoliday(date time.Time) (bool, error) {
	holidays, ok := c.holidays[date.Year()]
	if !ok {
		return false, fmt.Errorf("JsonCalendar: no records for year %d", date.Year())
	}
	_, isHoliday := holidays[date]
	return isHoliday, nil
}
