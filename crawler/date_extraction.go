package crawler

import (
	"feedrewind/crawler/rubydate"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type dateSource struct {
	Date       date
	SourceKind dateSourceKind
}

type date struct {
	Year  int
	Month time.Month
	Day   int
}

func (d date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

func dateCompare(d1, d2 date) int {
	if d1.Year == d2.Year && d1.Month == d2.Month && d1.Day == d2.Day {
		return 0
	}
	if d1.Year < d2.Year {
		return -1
	}
	if d1.Year > d2.Year {
		return 1
	}
	if d1.Month < d2.Month {
		return -1
	}
	if d1.Month > d2.Month {
		return 1
	}
	if d1.Day < d2.Day {
		return -1
	}
	return 1
}

type dateSourceKind int

const (
	dateSourceKindUnknown dateSourceKind = iota
	dateSourceKindTime
	dateSourceKindText
	dateSourceKindMeta
)

func (k dateSourceKind) String() string {
	switch k {
	case dateSourceKindUnknown:
		return "Ø"
	case dateSourceKindTime:
		return "time"
	case dateSourceKindText:
		return "text"
	case dateSourceKindMeta:
		return "meta"
	default:
		panic("Unknown date source kind")
	}
}

func tryExtractElementDate(maybeElement *html.Node, guessYear bool) *dateSource {
	if maybeElement == nil {
		return nil
	}

	if maybeElement.Type == html.ElementNode && maybeElement.Data == "time" {
		for _, attr := range maybeElement.Attr {
			if attr.Key == "datetime" {
				date := tryExtractTextDate(attr.Val, guessYear)
				if date != nil {
					return &dateSource{
						Date:       *date,
						SourceKind: dateSourceKindTime,
					}
				}
			}
		}
	} else if maybeElement.Type == html.TextNode {
		date := tryExtractTextDate(maybeElement.Data, guessYear)
		if date != nil {
			return &dateSource{
				Date:       *date,
				SourceKind: dateSourceKindText,
			}
		}
	}

	return nil
}

var digitRegex *regexp.Regexp
var digitSlashDigitRegex *regexp.Regexp
var yyyymmddRegex *regexp.Regexp
var digitsRegex *regexp.Regexp

func init() {
	digitRegex = regexp.MustCompile(`\d`)
	digitSlashDigitRegex = regexp.MustCompile(`\d/\d`)
	yyyymmddRegex = regexp.MustCompile(`(?:[\D]|^)(\d\d\d\d)-(\d\d)-(\d\d)(?:[\D]|$)`)
	digitsRegex = regexp.MustCompile(`\d+`)
}

var daysInMonth = [13]int64{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

func tryExtractTextDate(text string, guessYear bool) *date {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	if digitSlashDigitRegex.MatchString(text) {
		// Can't distinguish between MM/DD/YY and DD/MM/YY
		return nil
	}

	if !digitRegex.MatchString(text) {
		// Dates must have numbers
		return nil
	}

	isLeap := func(year int64) bool {
		return year%4 == 0 && (year%100 != 0 || year%400 == 0)
	}

	yyyymmddMatch := yyyymmddRegex.FindStringSubmatch(text)
	if yyyymmddMatch != nil {
		year, err := strconv.ParseInt(yyyymmddMatch[1], 10, 64)
		if err == nil && year >= 1900 && year < 2200 {
			month, err := strconv.ParseInt(yyyymmddMatch[2], 10, 64)
			if err == nil && month >= 1 && month <= 12 {
				day, err := strconv.ParseInt(yyyymmddMatch[3], 10, 64)
				if err == nil && day >= 1 &&
					(day <= daysInMonth[month] ||
						(isLeap(year) && month == int64(time.February) && day == 29)) {
					return &date{
						Year:  int(year),
						Month: time.Month(month),
						Day:   int(day),
					}
				}
			}
		}
	}

	fuzzyDate := rubydate.DateParse(text, true)
	if !fuzzyDate.MonthIsSet || !fuzzyDate.DayIsSet {
		return nil
	}

	textNumbers := digitsRegex.FindAllString(text, -1)

	if fuzzyDate.YearIsSet {
		yearStr := fmt.Sprint(fuzzyDate.Year)
		var yearTwoDigitStr string
		if len(yearStr) >= 2 {
			yearTwoDigitStr = yearStr[len(yearStr)-2:]
		} else {
			yearTwoDigitStr = yearStr
		}
		found := false
		for _, textNumber := range textNumbers {
			if textNumber == yearStr || textNumber == yearTwoDigitStr {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	} else if guessYear {
		// Special treatment only for missing year but not month or day
		fuzzyDate.Year = time.Now().UTC().Year()
		fuzzyDate.YearIsSet = true
	} else {
		return nil
	}

	dayStr := fmt.Sprint(fuzzyDate.Day)
	dayStrPadded := fmt.Sprintf("%02d", fuzzyDate.Day)
	dayFound := false
	for _, textNumber := range textNumbers {
		if textNumber == dayStr || textNumber == dayStrPadded {
			dayFound = true
			break
		}
	}
	if !dayFound {
		return nil
	}

	return &date{
		Year:  fuzzyDate.Year,
		Month: time.Month(fuzzyDate.Month),
		Day:   fuzzyDate.Day,
	}
}
