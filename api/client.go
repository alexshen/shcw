package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/samber/lo"
)

type ShiftState int

const (
	NotApproved = 10
	Approved    = 20
)

type Job struct {
	Code   string
	Name   string
	Shifts []Shift
}

type Shift struct {
	ApplyCode string
	UnitCode  string
	// when the shift is open for application
	OpenDate     time.Time
	ClockInTime  time.Time
	ClockOutTime time.Time
	State        ShiftState
	Settled      bool
}

func (j *Job) GetShift(openDate time.Time) *Shift {
	if shift, ok := lo.Find(j.Shifts, func(e Shift) bool {
		return e.OpenDate == openDate
	}); ok {
		return &shift
	}
	return nil
}

type GPSCoords struct {
	Lat float32 `json:"lat"`
	Lng float32 `json:"lng"`
}

type Client struct {
	username string
	password string
	gps      GPSCoords
	address  string

	userId int
	client *resty.Client
	jobs   []Job
}

func New(username string, password string, gps GPSCoords, address string) *Client {
	c := &Client{
		username: username,
		password: password,
		gps:      gps,
		address:  address,
		client:   resty.New().SetBaseURL("https://sq.shcvs.cn/962200/html5/v1"),
	}
	return c
}

func (c *Client) Login() error {
	body := struct {
		LoginName     string `json:"loginName"`
		LoginPassword string `json:"loginPassword"`
		LoginType     string `json:"loginType"`
	}{c.username, c.password, "1"}

	data := struct {
		UserId     int `json:"userId"`
		CookieResp struct {
			Token string `json:"token"`
		}
	}{}

	if err := c.doPost("/user/accountlogin", body, &data); err != nil {
		return err
	}
	c.userId = data.UserId
	c.client.SetHeader("token", data.CookieResp.Token)
	return nil
}

func (c *Client) UserId() int {
	return c.userId
}

// FetchJobs fetches the jobs from the server.
func (c *Client) FetchJobs() ([]Job, error) {
	c.jobs = nil
	// TODO: For now, jobs only open for 1 month, so PageSize of 31 is enough.
	body := struct {
		User       int `json:"user"`
		Status     int `json:"status"`
		PageNumber int `json:"pageNumber"`
		PageSize   int `json:"pageSize"`
	}{c.userId, 1, 1, 31}

	data := struct {
		Records []struct {
			FirstCode string `json:"pkFirstCode"`
			Name      string `json:"name"`
			List      FixedJsonValue[[]struct {
				ApplyCode    string       `json:"applyCode"`
				UnitCode     string       `json:"pk_unit_code"`
				OpenDate     JsonDate     `json:"day"`
				ClockInTime  JsonDateTime `json:"checkInTime"`
				ClockOutTime JsonDateTime `json:"checkOutTime"`
				State        int          `json:"state"`
				Settled      int          `json:"isSettle"`
			}] `json:"list"`
		} `json:"records"`
	}{}
	if err := c.doPost("/station/userTicket/queryPersonalPostByUserAndStatus", body, &data); err != nil {
		return nil, err
	}
	for _, r := range data.Records {
		if len(r.List.Value) == 0 {
			continue
		}
		job := Job{
			Code:   r.FirstCode,
			Name:   r.Name,
			Shifts: make([]Shift, len(r.List.Value)),
		}
		for i, s := range r.List.Value {
			job.Shifts[i] = Shift{
				ApplyCode:    s.ApplyCode,
				UnitCode:     s.UnitCode,
				OpenDate:     time.Time(s.OpenDate),
				ClockInTime:  time.Time(s.ClockInTime),
				ClockOutTime: time.Time(s.ClockOutTime),
				State:        ShiftState(s.State),
				Settled:      s.Settled != 0,
			}
		}
		c.jobs = append(c.jobs, job)
	}
	return c.jobs, nil
}

func (c *Client) Jobs() []Job {
	return c.jobs
}

type ShiftApplication struct {
	UnitCode    string `json:"pkUnitCode"`
	UserId      int    `json:"user,string"`
	UserName    string
	Ticket      string `json:"ticket"`
	TicketOrder string `json:"ticketOrder"`
}

func (c *Client) FetchApplications(shiftUnitCode string) ([]ShiftApplication, error) {
	body := struct {
		UnitCode   string `json:"pkUnitCode"`
		UserId     int    `json:"user,string"`
		State      int    `json:"state"`
		PageNumber int    `json:"pageNumber"`
		PageSize   int    `json:"pageSize"`
		Settled    int    `json:"isSettle"`
	}{
		UnitCode:   shiftUnitCode,
		State:      int(NotApproved),
		PageNumber: 1,
		PageSize:   31,
	}

	type record struct {
		ShiftApplication
		NickName json.RawMessage `json:"nickName"`
	}
	var data struct {
		Records []record `json:"records"`
	}
	if err := c.doPost("/station/postApply/auditList", &body, &data); err != nil {
		return nil, fmt.Errorf("fetch %s: %v", shiftUnitCode, err)
	}
	return lo.Map(data.Records, func(e record, i int) ShiftApplication {
		e.ShiftApplication.UserName = string(e.NickName)
		return e.ShiftApplication
	}), nil
}

func (c *Client) Approve(app *ShiftApplication) error {
	body := struct {
		*ShiftApplication
		State int `json:"state"`
	}{app, int(Approved)}

	return c.doPost("/station/postApply/audit", &body, nil)
}

func (c *Client) DoClock(jobCode string, shift *Shift) error {
	body := struct {
		User         int    `json:"user"`
		JobCode      string `json:"pkPostCode"`
		LocationType string `json:"locationType"`
		SourceType   int    `json:"sourceType"`
		OptionUser   int    `json:"optionUser"`
		ConfirmCheck int    `json:"confirmCheck"`
		SignPageCode int    `json:"signPageCode"`
		GPSCoords
		Address string `json:"address"`
	}{
		User:         c.userId,
		JobCode:      jobCode,
		LocationType: "GPS",
		SourceType:   1,
		OptionUser:   c.userId,
		ConfirmCheck: 0,
		SignPageCode: 1,
		GPSCoords:    c.gps,
		Address:      c.address,
	}

	return c.doPost("/station/newPostSign", &body, nil)
}

type responseMessage struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

var htmlMsgRgexp = regexp.MustCompile(`<p[^>]*>([^<]+)</p[^>]*>`)

func getPlainMsg(msg string) string {
	var para []string
	for _, sub := range htmlMsgRgexp.FindAllStringSubmatch(msg, -1) {
		para = append(para, sub[1])
	}
	if len(para) == 0 {
		return msg
	}
	return strings.Join(para, "\n")
}

func (c *Client) doPost(url string, body, resultData any) error {
	result := responseMessage{
		Data: resultData,
	}
	r := c.client.R().
		SetHeader("content-type", "application/json").
		SetResult(&result)
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		r.SetBody(data)
	}
	resp, err := r.Post(url)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.Status())
	}
	if result.Code != 0 {
		return errors.New(getPlainMsg(result.Msg))
	}
	return nil
}
