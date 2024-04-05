package schedule

import (
	"feedrewind/config"
	"feedrewind/oops"
	"html/template"
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v5/pgtype"
)

type DayOfWeek string

var DaysOfWeek = []DayOfWeek{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
var DaysOfWeekJson template.JS

func init() {
	daysOfWeekJsonBytes, err := json.Marshal(DaysOfWeek)
	if err != nil {
		panic(err)
	}
	DaysOfWeekJson = template.JS(string(daysOfWeekJsonBytes))
}

type Date string

func (d Date) NextDay() Date {
	parsed, err := time.Parse("2006-01-02", string(d))
	if err != nil {
		panic(err)
	}

	nextDay := parsed.AddDate(0, 0, 1)
	return Date(nextDay.Format("2006-01-02"))
}

func (d Date) Time() Time {
	parsed, err := ParseTime("2006-01-02", string(d))
	if err != nil {
		panic(err)
	}
	return parsed
}

func (d Date) TimeIn(location *time.Location) (Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", string(d), location)
	if err != nil {
		return Time(time.Time{}), oops.Wrap(err) //nolint:exhaustruct
	}

	return Time(parsed), nil
}

type Time time.Time

var EpochTime = Time(time.Unix(0, 0))

func NewTime(year int, month time.Month, day, hour, min, sec, nsec int, loc *time.Location) Time {
	return Time(time.Date(year, month, day, hour, min, sec, nsec, loc))
}

func ParseTime(format, value string) (Time, error) {
	t, err := time.Parse(format, value)
	return Time(t), err
}

func TimeSince(t Time) time.Duration {
	return UTCNow().Sub(t)
}

func (t Time) Add(d time.Duration) Time {
	return Time(time.Time(t).Add(d))
}

func (t Time) AddDate(years, months, days int) Time {
	return Time(time.Time(t).AddDate(years, months, days))
}

func (t Time) Sub(t2 Time) time.Duration {
	return time.Time(t).Sub(time.Time(t2))
}

func (t Time) BeginningOfDayIn(location *time.Location) Time {
	tt := time.Time(t)
	return Time(time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, location))
}

func (t Time) BeginningOfHour() Time {
	tt := time.Time(t)
	return Time(time.Date(tt.Year(), tt.Month(), tt.Day(), tt.Hour(), 0, 0, 0, time.UTC))
}

func (t Time) In(location *time.Location) Time {
	return Time(time.Time(t).In(location))
}

func (t Time) Before(t2 Time) bool {
	return time.Time(t).Before(time.Time(t2))
}

func (t Time) After(t2 Time) bool {
	return time.Time(t).After(time.Time(t2))
}

func (t Time) Equal(t2 Time) bool {
	return time.Time(t).Equal(time.Time(t2))
}

func (t Time) Compare(t2 Time) int {
	return time.Time(t).Compare(time.Time(t2))
}

func (t Time) UTC() Time {
	return Time(time.Time(t).UTC())
}

func (t Time) Day() int {
	return time.Time(t).Day()
}

func (t Time) DayOfWeek() DayOfWeek {
	return DaysOfWeek[time.Time(t).Weekday()]
}

func (t Time) Format(format string) string {
	return time.Time(t).Format(format)
}

func (t Time) String() string {
	return time.Time(t).String()
}

func (t Time) MustUTCString() string {
	if !isUTC(time.Time(t)) {
		panic("Expected UTC time")
	}
	return time.Time(t).Format("2006-01-02 15:04:05")
}

func (t Time) Date() Date {
	return Date(time.Time(t).Format("2006-01-02"))
}

func (t Time) IsEarlyMorning() bool {
	return time.Time(t).Hour() < 5
}

func (t *Time) ScanTimestamp(v pgtype.Timestamp) error {
	*t = Time(v.Time)
	return nil
}

func (t Time) TimestampValue() (pgtype.Timestamp, error) {
	return pgtype.Timestamp{
		Time:             time.Time(t),
		InfinityModifier: pgtype.Finite,
		Valid:            true,
	}, nil
}

var utcNowOverride *Time

func MustSetUTCNowOverride(t time.Time) {
	if !isUTC(t) {
		panic("Expected UTC override")
	}
	override := Time(t)
	utcNowOverride = &override
}

func IsSetUTCNowOverride() bool {
	return utcNowOverride != nil
}

func ResetUTCNowOverride() {
	utcNowOverride = nil
}

func UTCNow() Time {
	if config.Cfg.Env.IsDevOrTest() && utcNowOverride != nil {
		return *utcNowOverride
	} else {
		return Time(time.Now().UTC())
	}
}

func isUTC(t time.Time) bool {
	if t.Location().String() == "UTC" {
		return true
	}
	if name, offset := t.Zone(); name == "" && offset == 0 {
		return true
	}
	return false
}
