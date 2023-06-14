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
	*Job
	UnitCode string
	// when the shift is open for application
	OpenDate time.Time
}

type UserShift struct {
	*Shift
	ApplyCode    string
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

	userId   int
	client   *resty.Client
	jobs     []Job
	myShifts []UserShift
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
func (c *Client) FetchJobs() error {
	c.jobs = nil
	c.myShifts = nil

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
		return err
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
				Job:      &job,
				UnitCode: s.UnitCode,
				OpenDate: time.Time(s.OpenDate),
			}
			c.myShifts = append(c.myShifts, UserShift{
				Shift:        &job.Shifts[i],
				ApplyCode:    s.ApplyCode,
				ClockInTime:  time.Time(s.ClockInTime),
				ClockOutTime: time.Time(s.ClockOutTime),
				State:        ShiftState(s.State),
				Settled:      s.Settled != 0,
			})
		}
		c.jobs = append(c.jobs, job)
	}
	return nil
}

func (c *Client) Jobs() []Job {
	return c.jobs
}

func (c *Client) MyShifts() []UserShift {
	return c.myShifts
}

type ShiftApplication struct {
	UnitCode    string
	Code        string `json:"code"`
	UserId      int    `json:"user,string"`
	UserName    string
	Ticket      string `json:"ticket"`
	TicketOrder string `json:"ticketOrder"`
}

func (c *Client) FetchApplications(shift *Shift) ([]ShiftApplication, error) {
	body := struct {
		UnitCode   string `json:"pkUnitCode"`
		UserId     int    `json:"user,string"`
		State      int    `json:"state"`
		PageNumber int    `json:"pageNumber"`
		PageSize   int    `json:"pageSize"`
		Settled    int    `json:"isSettle"`
	}{
		UnitCode:   shift.UnitCode,
		State:      int(NotApproved),
		PageNumber: 1,
		PageSize:   31,
	}

	type record struct {
		ShiftApplication
		NickName string `json:"nickName"`
	}
	var data struct {
		Records []record `json:"records"`
	}
	if err := c.doPost("/station/postApply/auditList", &body, &data); err != nil {
		return nil, fmt.Errorf("fetch %s: %v", shift.UnitCode, err)
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

	if err := c.doPost("/station/postApply/audit", &body, nil); err != nil {
		return err
	}
	// update the shift state for my shift
	if app.UserId == c.userId {
		_, i, _ := lo.FindIndexOf(c.myShifts, func(e UserShift) bool {
			return e.ApplyCode == app.Code
		})
		if i != -1 {
			c.myShifts[i].State = Approved
		}
	}
	return nil
}

func (c *Client) DoClock(shift *UserShift) error {
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
		JobCode:      shift.Job.Code,
		LocationType: "GPS",
		SourceType:   1,
		OptionUser:   c.userId,
		ConfirmCheck: 0,
		SignPageCode: 1,
		GPSCoords:    c.gps,
		Address:      c.address,
	}

	if err := c.doPost("/station/newPostSign", &body, nil); err != nil {
		return err
	}
	// set the local clocking time as the server do not return anything useful
	if shift.ClockInTime.IsZero() {
		shift.ClockInTime = time.Now()
	} else if shift.ClockOutTime.IsZero() {
		shift.ClockOutTime = time.Now()
	}
	return nil
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
