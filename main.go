/*
shcw is a helper for automating clock-in and clock-out.
*/
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alexshen/shcw/api"
)

var (
	fLog      = flag.String("log", "cw.log", "path to the log file")
	fUsername = flag.String("username", "", "username")
	fAddress  = flag.String("address", "", "name for the gps position")
	fGPS      gpsValue
)

func init() {
	flag.Var(&fGPS, "gps", "gps position for clock-in and clock-out, e.g. 123,456")
}

type gpsValue api.GPSCoords

func (v *gpsValue) String() string {
	return fmt.Sprintf("%f,%f", v.Lat, v.Lng)
}

func (v *gpsValue) Set(d string) error {
	f := strings.Split(d, ",")
	if len(f) != 2 {
		return errors.New("gpsValue: invalid number of coordinates")
	}
	lng, err := strconv.ParseFloat(f[0], 32)
	if err != nil {
		return errors.New("gpsValue: invalid longtitude")
	}
	lat, err := strconv.ParseFloat(f[1], 32)
	if err != nil {
		return errors.New("gpsValue: invalid latitude")
	}
	v.Lng = float32(lng)
	v.Lat = float32(lat)
	return nil
}

func readPassword() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return scanner.Text(), scanner.Err()
}

func today() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

func initLogging() func() {
	if *fLog != "" {
		f, err := os.OpenFile(*fLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal("failed to open log: ", err)
		}
		log.SetOutput(f)
		return func() {
			f.Close()
		}
	}
	return func() {}
}

func approveApplications(client *api.Client, jobs []api.Job) {
	var numApproved int
	for _, job := range jobs {
		shift := job.GetShift(today())
		if shift == nil {
			continue
		}
		apps, err := client.FetchApplications(shift.UnitCode)
		if err != nil {
			log.Print(err)
			continue
		}
		// approve all applications
		for _, app := range apps {
			if err := client.Approve(&app); err != nil {
				log.Print(err)
				continue
			}
			log.Printf("approved job: %s, open date: %v, user: %s", job.Name, shift.OpenDate, app.UserName)
			numApproved++
		}
	}
	log.Printf("num of approved applications: %d", numApproved)
}

func doClock(client *api.Client, jobs []api.Job) {
	var numClocked int
	for _, job := range jobs {
		shift := job.GetShift(today())
		if shift == nil {
			continue
		}

		if shift.ClockInTime.IsZero() || shift.ClockOutTime.IsZero() {
			if err := client.DoClock(job.Code, shift); err != nil {
				log.Print(err)
				continue
			}
			if shift.ClockInTime.IsZero() {
				log.Printf("job %s clocked in at %v", job.Name, time.Now().Local())
			} else {
				log.Printf("job %s clocked out at %v", job.Name, time.Now().Local())
			}
			numClocked++
		}
	}
	log.Printf("num of clocked shifts: %d", numClocked)
}

func main() {
	flag.Parse()
	defer initLogging()()

	password, err := readPassword()
	if err != nil {
		log.Fatal("failed to read password:", err)
	}
	client := api.New(*fUsername, password, api.GPSCoords(fGPS), *fAddress)
	if err := client.Login(); err != nil {
		log.Fatal("failed to login:", err)
	}
	log.Printf("user %s has logged in", *fUsername)

	jobs, err := client.FetchJobs()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("number of jobs: %d", len(jobs))
	for _, job := range jobs {
		log.Printf("job %s", job.Name)
	}

	approveApplications(client, jobs)
	doClock(client, jobs)
}
