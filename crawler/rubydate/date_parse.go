package rubydate

// Adapted from https://github.com/ruby/ruby/blob/v3_2_2/ext/date/date_parse.c

// date_parse.c: Coded by Tadayoshi Funaba 2011,2012

/*
Copyright (C) 1993-2013 Yukihiro Matsumoto. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions
are met:
1. Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright
notice, this list of conditions and the following disclaimer in the
documentation and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE AUTHOR AND CONTRIBUTORS ``AS IS'' AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED.  IN NO EVENT SHALL THE AUTHOR OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS
OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT
LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY
OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF
SUCH DAMAGE.
*/

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dlclark/regexp2"
)

type FuzzyDate struct {
	Year            int
	YearIsSet       bool
	Month           int
	MonthIsSet      bool
	Day             int // "mday"
	DayIsSet        bool
	Weekday         int
	WeekdayIsSet    bool
	Hour            int
	HourIsSet       bool
	Minute          int
	MinuteIsSet     bool
	Second          int
	SecondIsSet     bool
	Nanosecond      int
	NanosecondIsSet bool
	Zone            string
	ZoneIsSet       bool
	CWYear          int
	CWYearIsSet     bool
	CWeek           int
	CWeekIsSet      bool
	CWDay           int
	CWDayIsSet      bool
	DayOfYear       int // "yday"
	DayOfYearIsSet  bool
	Offset          int // in seconds
	OffsetIsSet     bool
	IsTooLong       bool

	isBC               bool
	expandTwoDigitYear bool // "_comp"
}

var abbrDaysArr = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
var abbrMonthsArr = [12]string{
	"jan", "feb", "mar", "apr", "may", "jun",
	"jul", "aug", "sep", "oct", "nov", "dec",
}

func isSign(c byte) bool {
	return c == '+' || c == '-'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSpace(c byte) bool {
	return unicode.IsSpace(rune(c))
}

func digitSpan(str string, start int) int {
	span := 0
	for start+span < len(str) && isDigit(str[start+span]) {
		span++
	}
	return span
}

func s3e(fd *FuzzyDate, year, month, day string, bc bool) {
	if year != "" && month != "" && day == "" {
		year, month, day = day, year, month
	}

	if year == "" {
		if day != "" && len(day) > 2 {
			year, day = day, ""
		}
		if day != "" && len(day) > 0 && day[0] == '\'' {
			year, day = day, ""
		}
	}

	if year != "" {
		i := 0
		for i < len(year) && !isSign(year[i]) && !isDigit(year[i]) {
			i++
		}
		if i < len(year) {
			start := i
			if isSign(year[i]) {
				i++
			}
			span := digitSpan(year, i)
			end := i + span
			if end < len(year) {
				year, day = day, year[start:end]
			}
		}
	}

	if month != "" {
		if month[0] == '\'' || len(month) > 2 {
			// us -> be
			year, month, day = month, day, year
		}
	}

	if day != "" {
		if day[0] == '\'' || len(day) > 2 {
			year, day = day, year
		}
	}

	var expandTwoDigitYear bool
	expandTwoDigitYearIsSet := false
	if year != "" {
		i := 0
		for i < len(year) && !isSign(year[i]) && !isDigit(year[i]) {
			i++
		}
		if i < len(year) {
			start := i
			sign := false
			if isSign(year[i]) {
				i++
				sign = true
			}
			if sign {
				expandTwoDigitYear = false
				expandTwoDigitYearIsSet = true
			}
			span := digitSpan(year, i)
			end := i + span
			if span > 2 {
				expandTwoDigitYear = false
				expandTwoDigitYearIsSet = true
			}

			yearInt, err := strconv.Atoi(year[start:end])
			if err == nil {
				fd.Year = yearInt
				fd.YearIsSet = true
			}
		}
	}

	if bc {
		fd.isBC = true
	}

	if month != "" {
		i := 0
		for i < len(month) && !isDigit(month[i]) {
			i++
		}
		if i < len(month) {
			start := i
			span := digitSpan(month, start)
			end := i + span

			monthInt, err := strconv.Atoi(month[start:end])
			if err == nil {
				fd.Month = monthInt
				fd.MonthIsSet = true
			}
		}
	}

	if day != "" {
		i := 0
		for i < len(day) && !isDigit(day[i]) {
			i++
		}
		if i < len(day) {
			start := i
			span := digitSpan(day, start)
			end := i + span

			dayInt, err := strconv.Atoi(day[start:end])
			if err == nil {
				fd.Day = dayInt
				fd.DayIsSet = true
			}
		}
	}

	if expandTwoDigitYearIsSet {
		fd.expandTwoDigitYear = expandTwoDigitYear
	}
}

const abbrDays = "sun|mon|tue|wed|thu|fri|sat"
const abbrMonths = "jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec"

const backtrackingNumber = "(?<!\\d)\\d"

const backtrackingRegexTimeout = 250 * time.Millisecond

func subs(str *string, match *regexp2.Match) {
	*str = strings.Replace(*str, match.String(), " ", 1)
}

func strEndWithWord(s string, l int, w string) int {
	n := len(w)
	if l <= n || !isSpace(s[l-n-1]) {
		return 0
	}
	if !strings.EqualFold(s[l-n:l], w) {
		return 0
	}
	n++
	for n < l && isSpace(s[l-n-1]) {
		n++
	}
	return n
}

func shrunkSize(s string, l int) int {
	ni := 0
	sp := false
	for i := 0; i < l; i++ {
		if !isSpace(s[i]) {
			if sp {
				ni++
			}
			sp = false
			ni++
		} else {
			sp = true
		}
	}
	if ni < l {
		return ni
	}
	return 0
}

func shrinkSpace(d []byte, s string, l int) int {
	ni := 0
	sp := false
	for i := 0; i < l; i++ {
		if !isSpace(s[i]) {
			if sp {
				d[ni] = ' '
				ni++
			}
			sp = false
			d[ni] = s[i]
			ni++
		} else {
			sp = true
		}
	}
	return ni
}

var hourMinSecRegex *regexp2.Regexp
var fracHourDigitsRegex *regexp2.Regexp

func init() {
	hourMinSecRegex = regexp2.MustCompile("\\A(\\d+)(:\\d+)?(:\\d+)?", regexp2.None)
	// no over precision for offset; 10**-7 hour = 0.36 milliseconds should be enough.
	fracHourDigitsRegex = regexp2.MustCompile("\\A(\\d{0,7})", regexp2.None)
}

func dateZoneToDiff(str string) (int, bool) {
	l := len(str)
	{
		dst := false
		if w := strEndWithWord(str, l, "time"); w > 0 {
			wTime := w
			l -= wTime
			if w := strEndWithWord(str, l, "standard"); w > 0 {
				l -= w
			} else if w := strEndWithWord(str, l, "daylight"); w > 0 {
				l -= w
				dst = true
			} else {
				l += wTime
			}
		} else if w := strEndWithWord(str, l, "dst"); w > 0 {
			l -= w
			dst = true
		}

		{
			zone := str[:l]
			sl := shrunkSize(str, l)
			const maxWordLength = 17
			var shrunkBuff [maxWordLength]byte

			if sl <= 0 {
				sl = l
			} else if sl <= maxWordLength {
				sl = shrinkSpace(shrunkBuff[:], str, l)
				zone = string(shrunkBuff[:sl])
			}

			if sl > 0 && sl <= maxWordLength {
				if offset, ok := offsetByZone[strings.ToLower(zone)]; ok {
					if dst {
						offset += 3600
					}
					return offset, true
				}
			}
		}

		{
			hour := 0
			minute := 0
			second := 0
			s := 0

			if l > 3 && (strings.EqualFold(str[:3], "gmt") || strings.EqualFold(str[:3], "utc")) {
				s += 3
				l -= 3
			}

			if s < len(str) && isSign(str[s]) {
				sign := str[s] == '-'
				s++
				l--

				outOfRange := func(v, min, max int) bool {
					return v < min || max < v
				}

				match, _ := hourMinSecRegex.FindStringMatch(str[s:])
				groups := match.Groups()
				hourStr := groups[1].String()
				p := s + len(hourStr)
				var hourErr error
				hour, hourErr = strconv.Atoi(hourStr)
				switch {
				case groups[2].Length > 0:
					if hourErr != nil || outOfRange(hour, 0, 23) {
						return 0, false
					}
					s += len(hourStr) + 1
					var minuteErr error
					minuteStr := groups[2].String()[1:]
					minute, minuteErr = strconv.Atoi(minuteStr)
					if minuteErr != nil || outOfRange(minute, 0, 59) {
						return 0, false
					}
					if groups[3].Length > 0 {
						s += len(minuteStr) + 1 //nolint:ineffassign,staticcheck
						var secondErr error
						second, secondErr = strconv.Atoi(groups[3].String()[1:])
						// Ruby code doesn't account for leap seconds but let's do it here
						if secondErr != nil || outOfRange(second, 0, 60) {
							return 0, false
						}
					}
				case p < len(str) && (str[p] == '.' || str[p] == ','):
					if hourErr != nil || outOfRange(hour, 0, 23) {
						return 0, false
					}

					p++
					fracHourMatch, _ := fracHourDigitsRegex.FindStringMatch(str[p:])
					fracHourStr := fracHourMatch.String()
					second, _ := strconv.Atoi(fracHourStr)
					n := len(fracHourStr)
					p += n
					if p < s+l && str[p] >= ('5'+byte(1-(second%2))) && str[p] <= '9' {
						// Round half to even
						second++
					}
					second *= 36
					if sign {
						hour = -hour
						second = -second
					}
					var offset int
					if n <= 2 {
						// HH.nn or HH.n
						if n == 1 {
							second *= 10
						}
						offset = second + hour*3600
					} else {
						for second >= 3600 {
							second /= 10
						}
						offset = second + hour*3600
					}
					return offset, true
				case l > 2:
					if l >= 1 {
						hour, _ = strconv.Atoi(str[s : s+2-l%2])
					}
					if l >= 3 {
						minute, _ = strconv.Atoi(str[s+2-l%2 : s+4-l%2])
					}
					if l >= 5 {
						second, _ = strconv.Atoi(str[s+4-l%2 : s+6-l%2])
					}
				}
				offset := second + minute*60 + hour*3600
				if sign {
					offset = -offset
				}
				return offset, true
			}
		}
	}

	return 0, false
}

func dayNum(str string) (int, bool) {
	for i, weekday := range abbrDaysArr {
		if strings.EqualFold(weekday, str) {
			return i, true
		}
	}
	return 0, false
}

func monthNum(str string) (int, bool) {
	for i, weekday := range abbrMonthsArr {
		if strings.EqualFold(weekday, str) {
			return i + 1, true
		}
	}
	return 0, false
}

var dayRegex *regexp2.Regexp

func init() {
	dayRegex = regexp2.MustCompile(
		"\\b("+abbrDays+")[^-/\\d\\s]*",
		regexp2.IgnoreCase,
	)
}

func parseDay(str *string, fd *FuzzyDate) {
	match, _ := dayRegex.FindStringMatch(*str)
	if match == nil {
		return
	}
	subs(str, match)
	groups := match.Groups()

	fd.Weekday, fd.WeekdayIsSet = dayNum(groups[1].String())
}

func parseNanosecond(fd *FuzzyDate, secondFractionStr string) {
	if secondFractionStr != "" && len(secondFractionStr) <= 9 {
		secondFraction, err := strconv.Atoi(secondFractionStr)
		if err == nil {
			multZeros := 9 - len(secondFractionStr)
			mult := [9]int{1, 10, 100, 1000, 10000, 100000, 1000000, 10000000, 100000000}[multZeros]
			fd.Nanosecond = secondFraction * mult
			fd.NanosecondIsSet = true
		}
	}
}

var timeRegex *regexp2.Regexp
var timeRegex2 *regexp2.Regexp

func init() {
	timeRegex = regexp2.MustCompile(""+
		/**/ "("+
		/*  */ ""+backtrackingNumber+"+\\s*"+
		/*  */ "(?:"+
		/*    */ "(?:"+
		/*      */ ":\\s*\\d+"+
		/*      */ "(?:"+
		/*        */ "\\s*:\\s*\\d+(?:[,.]\\d*)?"+
		/*      */ ")?"+
		/*    */ "|"+
		/*      */ "h(?:\\s*\\d+m?(?:\\s*\\d+s?)?)?"+
		/*    */ ")"+
		/*    */ "(?:"+
		/*      */ "\\s*"+
		/*      */ "[ap](?:m\\b|\\.m\\.)"+
		/*    */ ")?"+
		/*  */ "|"+
		/*    */ "[ap](?:m\\b|\\.m\\.)"+
		/*  */ ")"+
		/**/ ")"+
		/**/ "(?:"+
		/*  */ "\\s*"+
		/*  */ "("+
		/*    */ "(?:gmt|utc?)?[-+]\\d+(?:[,.:]\\d+(?::\\d+)?)?"+
		/*  */ "|"+
		/*    */ "(?-i:[A-Za-z.\\s]+)(?:standard|daylight)\\stime\\b"+
		/*  */ "|"+
		/*    */ "(?-i:[A-Za-z]+)(?:\\sdst)?\\b"+
		/*  */ ")"+
		/**/ ")?",
		regexp2.IgnoreCase,
	)
	timeRegex.MatchTimeout = backtrackingRegexTimeout

	timeRegex2 = regexp2.MustCompile(""+
		/**/ "\\A(\\d+)h?"+
		/*  */ "(?:\\s*:?\\s*(\\d+)m?"+
		/*    */ "(?:"+
		/*      */ "\\s*:?\\s*(\\d+)(?:[,.](\\d+))?s?"+
		/*    */ ")?"+
		/*  */ ")?"+
		/**/ "(?:\\s*([ap])(?:m\\b|\\.m\\.))?",
		regexp2.IgnoreCase,
	)
}

func parseTime(str *string, fd *FuzzyDate) {
	match, err := timeRegex.FindStringMatch(*str)
	if err != nil {
		return
	}
	if match == nil {
		return
	}
	subs(str, match)
	groups := match.Groups()

	if groups[2].Length > 0 {
		fd.Zone = groups[2].String()
		fd.ZoneIsSet = true
	}

	timeMatch, _ := timeRegex2.FindStringMatch(groups[1].String())
	if timeMatch != nil {
		timeGroups := timeMatch.Groups()
		hour, err := strconv.Atoi(timeGroups[1].String())
		hourIsSet := false
		if err == nil {
			hourIsSet = true
		}

		if timeGroups[2].Length > 0 {
			minute, err := strconv.Atoi(timeGroups[2].String())
			if err == nil {
				fd.Minute = minute
				fd.MinuteIsSet = true
			}
		}

		if timeGroups[3].Length > 0 {
			second, err := strconv.Atoi(timeGroups[3].String())
			if err == nil {
				fd.Second = second
				fd.SecondIsSet = true
			}
		}

		secondFractionStr := timeGroups[4].String()
		parseNanosecond(fd, secondFractionStr)

		ampm := timeGroups[5].String()
		if ampm != "" && hourIsSet {
			hour %= 12
			if ampm[0] == 'P' || ampm[0] == 'p' {
				hour += 12
			}
		}
		fd.Hour = hour
		fd.HourIsSet = true
	}
}

const beginEra = "\\b"
const backtrackingEndEra = "(?!(?<!\\.)[a-z])"

var euRegex *regexp2.Regexp

func init() {
	euRegex = regexp2.MustCompile(
		""+
			/**/ "('?"+backtrackingNumber+"+)[^-\\d\\s]*"+
			/**/ "\\s*"+
			/**/ "("+abbrMonths+")[^-\\d\\s']*"+
			/**/ "(?:"+
			/*  */ "\\s*"+
			/*  */ "(?:"+
			/*    */ ""+beginEra+
			/*    */ "(c(?:e|\\.e\\.)|b(?:ce|\\.c\\.e\\.)|a(?:d|\\.d\\.)|b(?:c|\\.c\\.))"+
			/*    */ ""+backtrackingEndEra+
			/*  */ ")?"+
			/*  */ "\\s*"+
			/*  */ "('?-?\\d+(?:(?:st|nd|rd|th)\\b)?)"+
			/**/ ")?",
		regexp2.IgnoreCase,
	)
	euRegex.MatchTimeout = backtrackingRegexTimeout
}

func parseEU(str *string, fd *FuzzyDate) bool {
	match, err := euRegex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	day := groups[1].String()
	monthName := groups[2].String()
	monthNum, _ := monthNum(monthName)
	month := fmt.Sprint(monthNum)
	era := groups[3].String()
	bc := len(era) > 0 && (era[0] == 'B' || era[0] == 'b')
	year := groups[4].String()

	s3e(fd, year, month, day, bc)
	return true
}

var usRegex *regexp2.Regexp

func init() {
	usRegex = regexp2.MustCompile(""+
		/**/ "\\b("+abbrMonths+")[^-\\d\\s']*"+
		/**/ "\\s*"+
		/**/ "('?\\d+)[^-\\d\\s']*"+
		/**/ "(?:"+
		/*  */ "\\s*,?"+
		/*  */ "\\s*"+
		/*  */ "(c(?:e|\\.e\\.)|b(?:ce|\\.c\\.e\\.)|a(?:d|\\.d\\.)|b(?:c|\\.c\\.))?"+
		/*  */ "\\s*"+
		/*  */ "('?-?\\d+)"+
		/**/ ")?",
		regexp2.IgnoreCase,
	)
}

func parseUS(str *string, fd *FuzzyDate) bool {
	match, _ := usRegex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	monthName := groups[1].String()
	monthNum, _ := monthNum(monthName)
	month := fmt.Sprint(monthNum)

	day := groups[2].String()

	era := groups[3].String()
	bc := len(era) > 0 && (era[0] == 'B' || era[0] == 'b')

	year := groups[4].String()

	s3e(fd, year, month, day, bc)
	return true

}

var isoRegex *regexp2.Regexp

func init() {
	isoRegex = regexp2.MustCompile(""+
		"('?[-+]?"+backtrackingNumber+"+)-(\\d+)-('?-?\\d+)",
		regexp2.None,
	)
	isoRegex.MatchTimeout = backtrackingRegexTimeout
}

func parseISO(str *string, fd *FuzzyDate) bool {
	match, err := isoRegex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	year := groups[1].String()
	month := groups[2].String()
	day := groups[3].String()

	s3e(fd, year, month, day, false)
	return true
}

var iso21Regex *regexp2.Regexp

func init() {
	iso21Regex = regexp2.MustCompile("\\b(\\d{2}|\\d{4})?-?w(\\d{2})(?:-?(\\d))?\\b", regexp2.IgnoreCase)
}

func parseISO21(str *string, fd *FuzzyDate) bool {
	match, _ := iso21Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	if groups[1].Length > 0 {
		year, err := strconv.Atoi(groups[1].String())
		if err == nil {
			fd.CWYear = year
			fd.CWYearIsSet = true
		}
	}

	week, err := strconv.Atoi(groups[2].String())
	if err == nil {
		fd.CWeek = week
		fd.CWeekIsSet = true
	}

	if groups[3].Length > 0 {
		day, err := strconv.Atoi(groups[3].String())
		if err == nil {
			fd.CWDay = day
			fd.CWDayIsSet = true
		}
	}

	return true
}

var iso22Regex *regexp2.Regexp

func init() {
	iso22Regex = regexp2.MustCompile(""+
		"-w-(\\d)\\b",
		regexp2.IgnoreCase,
	)
}

func parseISO22(str *string, fd *FuzzyDate) bool {
	match, _ := iso22Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	day, err := strconv.Atoi(groups[1].String())
	if err == nil {
		fd.CWDay = day
		fd.CWDayIsSet = true
	}

	return true
}

var iso23Regex *regexp2.Regexp

func init() {
	iso23Regex = regexp2.MustCompile("--(\\d{2})?-(\\d{2})\\b", regexp2.None)
}

func parseISO23(str *string, fd *FuzzyDate) bool {
	match, _ := iso23Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	if groups[1].Length > 0 {
		month, err := strconv.Atoi(groups[1].String())
		if err == nil {
			fd.Month = month
			fd.MonthIsSet = true
		}
	}

	day, err := strconv.Atoi(groups[2].String())
	if err == nil {
		fd.Day = day
		fd.DayIsSet = true
	}

	return true
}

var iso24Regex *regexp2.Regexp

func init() {
	iso24Regex = regexp2.MustCompile("--(\\d{2})(\\d{2})?\\b", regexp2.None)
}

func parseISO24(str *string, fd *FuzzyDate) bool {
	match, _ := iso24Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	month, err := strconv.Atoi(groups[1].String())
	if err == nil {
		fd.Month = month
		fd.MonthIsSet = true
	}

	if groups[2].Length > 0 {
		day, err := strconv.Atoi(groups[2].String())
		if err == nil {
			fd.Day = day
			fd.DayIsSet = true
		}
	}

	return true
}

var iso25Regex0 *regexp2.Regexp
var iso25Regex *regexp2.Regexp

func init() {
	iso25Regex0 = regexp2.MustCompile("[,.](\\d{2}|\\d{4})-\\d{3}\\b", regexp2.None)
	iso25Regex = regexp2.MustCompile("\\b(\\d{2}|\\d{4})-(\\d{3})\\b", regexp2.None)
}

func parseISO25(str *string, fd *FuzzyDate) bool {
	if ok, _ := iso25Regex0.MatchString(*str); ok {
		return false
	}

	match, _ := iso25Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	year, err := strconv.Atoi(groups[1].String())
	if err == nil {
		fd.Year = year
		fd.YearIsSet = true
	}

	day, err := strconv.Atoi(groups[2].String())
	if err == nil {
		fd.DayOfYear = day
		fd.DayOfYearIsSet = true
	}

	return true
}

var iso26Regex0 *regexp2.Regexp
var iso26Regex *regexp2.Regexp

func init() {
	iso26Regex0 = regexp2.MustCompile("\\d-\\d{3}\\b", regexp2.None)
	iso26Regex = regexp2.MustCompile("\\b-(\\d{3})\\b", regexp2.None)
}

func parseISO26(str *string, fd *FuzzyDate) bool {
	if ok, _ := iso26Regex0.MatchString(*str); ok {
		return false
	}

	match, _ := iso26Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	day, err := strconv.Atoi(groups[1].String())
	if err == nil {
		fd.DayOfYear = day
		fd.DayOfYearIsSet = true
	}

	return true
}

func parseISO2(str *string, fd *FuzzyDate) bool {
	if parseISO21(str, fd) {
		return true
	}
	if parseISO22(str, fd) {
		return true
	}
	if parseISO23(str, fd) {
		return true
	}
	if parseISO24(str, fd) {
		return true
	}
	if parseISO25(str, fd) {
		return true
	}
	if parseISO26(str, fd) {
		return true
	}
	return false
}

const jisx0301EraInitials = "mtshr"

func gengo(c byte) int {
	switch c {
	case 'M', 'm':
		return 1867
	case 'T', 't':
		return 1911
	case 'S', 's':
		return 1925
	case 'H', 'h':
		return 1988
	case 'R', 'r':
		return 2018
	default:
		return 0
	}
}

var jisRegex *regexp2.Regexp

func init() {
	jisRegex = regexp2.MustCompile(""+
		"\\b(["+jisx0301EraInitials+"])(\\d+)\\.(\\d+)\\.(\\d+)",
		regexp2.IgnoreCase,
	)
}

func parseJIS(str *string, fd *FuzzyDate) bool {
	match, _ := jisRegex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	era := groups[1].String()
	year, err := strconv.Atoi(groups[2].String())
	if err == nil {
		eraStart := gengo(era[0])
		fd.Year = year + eraStart
		fd.YearIsSet = true
	}

	month, err := strconv.Atoi(groups[3].String())
	if err == nil {
		fd.Month = month
		fd.MonthIsSet = true
	}

	day, err := strconv.Atoi(groups[4].String())
	if err == nil {
		fd.Day = day
		fd.DayIsSet = true
	}

	return true
}

var vms11Regex *regexp2.Regexp

func init() {
	vms11Regex = regexp2.MustCompile(""+
		"('?-?"+backtrackingNumber+"+)-("+abbrMonths+")[^-/.]*"+
		"-('?-?\\d+)",
		regexp2.IgnoreCase,
	)
	vms11Regex.MatchTimeout = backtrackingRegexTimeout
}

func parseVMS11(str *string, fd *FuzzyDate) bool {
	match, err := vms11Regex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	day := groups[1].String()
	monthName := groups[2].String()
	year := groups[3].String()

	monthNum, _ := monthNum(monthName)
	month := fmt.Sprint(monthNum)
	s3e(fd, year, month, day, false)
	return true
}

var vms12Regex *regexp2.Regexp

func init() {
	vms12Regex = regexp2.MustCompile(""+
		"\\b("+abbrMonths+")[^-/.]*"+
		"-('?-?\\d+)(?:-('?-?\\d+))?",
		regexp2.IgnoreCase,
	)
}

func parseVMS12(str *string, fd *FuzzyDate) bool {
	match, _ := vms12Regex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	monthName := groups[1].String()
	day := groups[2].String()
	year := groups[3].String()

	monthNum, _ := monthNum(monthName)
	month := fmt.Sprint(monthNum)
	s3e(fd, year, month, day, false)
	return true
}

func parseVMS(str *string, fd *FuzzyDate) bool {
	if parseVMS11(str, fd) {
		return true
	}
	if parseVMS12(str, fd) {
		return true
	}
	return false
}

var slaRegex *regexp2.Regexp

func init() {
	slaRegex = regexp2.MustCompile(""+
		"('?-?"+backtrackingNumber+"+)/\\s*('?\\d+)(?:\\D\\s*('?-?\\d+))?",
		regexp2.IgnoreCase,
	)
	slaRegex.MatchTimeout = backtrackingRegexTimeout
}

func parseSla(str *string, fd *FuzzyDate) bool {
	match, err := slaRegex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	year := groups[1].String()
	month := groups[2].String()
	day := groups[3].String()

	s3e(fd, year, month, day, false)
	return true
}

var dotRegex *regexp2.Regexp

func init() {
	dotRegex = regexp2.MustCompile(""+
		"('?-?"+backtrackingNumber+"+)\\.\\s*('?\\d+)\\.\\s*('?-?\\d+)",
		regexp2.IgnoreCase,
	)
	dotRegex.MatchTimeout = backtrackingRegexTimeout
}

func parseDot(str *string, fd *FuzzyDate) bool {
	match, err := dotRegex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	year := groups[1].String()
	month := groups[2].String()
	day := groups[3].String()

	s3e(fd, year, month, day, false)
	return true
}

var yearRegex *regexp2.Regexp

func init() {
	yearRegex = regexp2.MustCompile("'(\\d+)\\b", regexp2.None)
}

func parseYear(str *string, fd *FuzzyDate) bool {
	match, _ := yearRegex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	year, err := strconv.Atoi(groups[1].String())
	if err == nil {
		fd.Year = year
		fd.YearIsSet = true
	}

	return true
}

var monRegex *regexp2.Regexp

func init() {
	monRegex = regexp2.MustCompile(""+
		"\\b("+abbrMonths+")\\S*",
		regexp2.IgnoreCase,
	)
}

func parseMon(str *string, fd *FuzzyDate) bool {
	match, _ := monRegex.FindStringMatch(*str)
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	monthNum, _ := monthNum(groups[1].String())
	fd.Month = int(monthNum)
	fd.MonthIsSet = true

	return true
}

var mdayRegex *regexp2.Regexp

func init() {
	mdayRegex = regexp2.MustCompile(""+
		"("+backtrackingNumber+"+)(st|nd|rd|th)\\b",
		regexp2.IgnoreCase,
	)
	mdayRegex.MatchTimeout = backtrackingRegexTimeout
}

func parseMDay(str *string, fd *FuzzyDate) bool {
	match, err := mdayRegex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	day, err := strconv.Atoi(groups[1].String())
	if err == nil {
		fd.Day = day
		fd.DayIsSet = true
	}

	return true
}

var dddRegex *regexp2.Regexp

func init() {
	dddRegex = regexp2.MustCompile(""+
		/**/ "([-+]?)("+backtrackingNumber+"{2,14})"+
		/**/ "(?:"+
		/*  */ "\\s*"+
		/*  */ "t?"+
		/*  */ "\\s*"+
		/*  */ "(\\d{2,6})?(?:[,.](\\d*))?"+
		/**/ ")?"+
		/**/ "(?:"+
		/*  */ "\\s*"+
		/*  */ "("+
		/*    */ "z\\b"+
		/*  */ "|"+
		/*    */ "[-+]\\d{1,4}\\b"+
		/*  */ "|"+
		/*    */ "\\[[-+]?\\d[^\\]]*\\]"+
		/*  */ ")"+
		/**/ ")?",
		regexp2.IgnoreCase,
	)
	dddRegex.MatchTimeout = backtrackingRegexTimeout
}

func n2i(str string, start, length int) int {
	end := start + length
	result := 0
	for i := start; i < end; i++ {
		result *= 10
		result += int(str[i] - '0')
	}
	return result
}

func parseDDD(str *string, fd *FuzzyDate) bool {
	match, err := dddRegex.FindStringMatch(*str)
	if err != nil {
		return false
	}
	if match == nil {
		return false
	}
	subs(str, match)
	groups := match.Groups()

	s1 := groups[1].String()
	s2 := groups[2].String()
	s3 := groups[3].String()
	s4 := groups[4].String()
	s5 := groups[5].String()

	l2 := len(s2)

	switch l2 {
	case 2:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
		} else {
			fd.Day = n2i(s2, 0, 2)
			fd.DayIsSet = true
		}
	case 4:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
			fd.Minute = n2i(s2, l2-4, 2)
			fd.MinuteIsSet = true
		} else {
			fd.Month = n2i(s2, 0, 2)
			fd.MonthIsSet = true
			fd.Day = n2i(s2, 2, 2)
			fd.DayIsSet = true
		}
	case 6:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
			fd.Minute = n2i(s2, l2-4, 2)
			fd.MinuteIsSet = true
			fd.Hour = n2i(s2, l2-6, 2)
			fd.HourIsSet = true
		} else {
			year := n2i(s2, 0, 2)
			if s1 != "" && s1[0] == '-' {
				year = -year
			}
			fd.Year = int(year)
			fd.YearIsSet = true
			fd.Month = n2i(s2, 2, 2)
			fd.MonthIsSet = true
			fd.Day = n2i(s2, 4, 2)
			fd.DayIsSet = true
		}
	case 8, 10, 12, 14:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
			fd.Minute = n2i(s2, l2-4, 2)
			fd.MinuteIsSet = true
			fd.Hour = n2i(s2, l2-6, 2)
			fd.HourIsSet = true
			fd.Day = n2i(s2, l2-8, 2)
			fd.DayIsSet = true
			if l2 >= 10 {
				fd.Month = n2i(s2, l2-10, 2)
				fd.MonthIsSet = true
			}
			if l2 >= 12 {
				year := n2i(s2, l2-12, 2)
				if s1 != "" && s1[0] == '-' {
					year = -year
				}
				fd.Year = year
				fd.YearIsSet = true
			}
			if l2 == 14 {
				year := n2i(s2, l2-14, 4)
				if s1 != "" && s1[0] == '-' {
					year = -year
				}
				fd.Year = year
				fd.YearIsSet = true
				fd.expandTwoDigitYear = false
			}
		} else {
			year := n2i(s2, 0, 4)
			if s1 != "" && s1[0] == '-' {
				year = -year
			}
			fd.Year = year
			fd.YearIsSet = true
			fd.Month = n2i(s2, 4, 2)
			fd.MonthIsSet = true
			fd.Day = n2i(s2, 6, 2)
			fd.DayIsSet = true
			if l2 >= 10 {
				fd.Hour = n2i(s2, 8, 2)
				fd.HourIsSet = true
			}
			if l2 >= 12 {
				fd.Minute = n2i(s2, 10, 2)
				fd.MinuteIsSet = true
			}
			if l2 >= 14 {
				fd.Second = n2i(s2, 12, 2)
				fd.SecondIsSet = true
			}
			fd.expandTwoDigitYear = false
		}
	case 3:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
			fd.Minute = n2i(s2, l2-3, 1)
			fd.MinuteIsSet = true
		} else {
			fd.DayOfYear = n2i(s2, 0, 3)
			fd.DayOfYearIsSet = true
		}
	case 5:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
			fd.Minute = n2i(s2, l2-4, 2)
			fd.MinuteIsSet = true
			fd.Hour = n2i(s2, l2-5, 1)
			fd.HourIsSet = true
		} else {
			year := n2i(s2, 0, 2)
			if s1 != "" && s1[0] == '-' {
				year = -year
			}
			fd.Year = year
			fd.YearIsSet = true
			fd.DayOfYear = n2i(s2, 2, 3)
			fd.DayOfYearIsSet = true
		}
	case 7:
		if s3 == "" && s4 != "" {
			fd.Second = n2i(s2, l2-2, 2)
			fd.SecondIsSet = true
			fd.Minute = n2i(s2, l2-4, 2)
			fd.MinuteIsSet = true
			fd.Hour = n2i(s2, l2-6, 2)
			fd.HourIsSet = true
			fd.Day = n2i(s2, l2-7, 1)
			fd.DayIsSet = true
		} else {
			year := n2i(s2, 0, 4)
			if s1 != "" && s1[0] == '-' {
				year = -year
			}
			fd.Year = year
			fd.YearIsSet = true
			fd.DayOfYear = n2i(s2, 4, 3)
			fd.DayOfYearIsSet = true
		}
	}

	if s3 != "" {
		l3 := len(s3)
		if s4 != "" {
			switch l3 {
			case 2, 4, 6:
				fd.Second = n2i(s3, l3-2, 2)
				fd.SecondIsSet = true
				if l3 >= 4 {
					fd.Minute = n2i(s3, l3-4, 2)
					fd.MinuteIsSet = true
				}
				if l3 >= 6 {
					fd.Hour = n2i(s3, l3-6, 2)
					fd.HourIsSet = true
				}
			}
		} else {
			switch l3 {
			case 2, 4, 6:
				fd.Hour = n2i(s3, 0, 2)
				fd.HourIsSet = true
				if l3 >= 4 {
					fd.Minute = n2i(s3, 2, 2)
					fd.MinuteIsSet = true
				}
				if l3 >= 6 {
					fd.Second = n2i(s3, 4, 2)
					fd.SecondIsSet = true
				}
			}
		}
	}

	parseNanosecond(fd, s4)

	if s5 != "" {
		if s5[0] == '[' {
			s2 := strings.Index(s5[1:len(s5)-1], ":")
			var zone string
			if s2 >= 0 {
				s2++
				zone = s5[1+s2 : len(s5)-1]
				s5 = s5[1 : 1+s2-1]
			} else {
				zone = s5[1 : len(s5)-1]
				if isDigit(s5[1]) {
					s5 = fmt.Sprintf("+%s", zone)
				} else {
					s5 = zone
				}
			}
			fd.Zone = zone
			fd.ZoneIsSet = true
			fd.Offset, fd.OffsetIsSet = dateZoneToDiff(s5)
		} else {
			fd.Zone = s5
			fd.ZoneIsSet = true
		}
	}

	return true
}

var bcRegex *regexp2.Regexp

func init() {
	bcRegex = regexp2.MustCompile(""+
		"\\b(bc\\b|bce\\b|b\\.c\\.|b\\.c\\.e\\.)",
		regexp2.IgnoreCase,
	)
}

func parseBC(str *string, fd *FuzzyDate) {
	match, _ := bcRegex.FindStringMatch(*str)
	if match == nil {
		return
	}
	subs(str, match)

	fd.isBC = true
}

var fragRegex *regexp2.Regexp

func init() {
	fragRegex = regexp2.MustCompile(""+
		"\\A\\s*(\\d{1,2})\\s*\\z",
		regexp2.IgnoreCase,
	)
}

func parseFrag(str *string, fd *FuzzyDate) {
	match, _ := fragRegex.FindStringMatch(*str)
	if match == nil {
		return
	}
	subs(str, match)
	groups := match.Groups()

	if fd.HourIsSet && !fd.DayIsSet {
		num, _ := strconv.Atoi(groups[1].String())
		if num >= 1 && num <= 31 {
			fd.Day = num
			fd.DayIsSet = true
		}
	}

	if fd.DayIsSet && !fd.HourIsSet {
		num, _ := strconv.Atoi(groups[1].String())
		// Ruby code says <= 24, not <24
		if num >= 0 && num <= 24 {
			fd.Hour = num
			fd.HourIsSet = true
		}
	}
}

type classFlags int

const (
	classFlagHaveAlpha classFlags = 1 << iota
	classFlagHaveDigit
	classFlagHaveDash
	classFlagHaveDot
	classFlagHaveSlash
)

func hasClass(str string, flags classFlags) bool {
	return checkClass(str)&flags == flags
}

func checkClass(str string) classFlags {
	var flags classFlags
	for i := 0; i < len(str); i++ {
		c := str[i]
		switch {
		case isAlpha(c):
			flags |= classFlagHaveAlpha
		case isDigit(c):
			flags |= classFlagHaveDigit
		case c == '-':
			flags |= classFlagHaveDash
		case c == '.':
			flags |= classFlagHaveDot
		case c == '/':
			flags |= classFlagHaveSlash
		}
	}
	return flags
}

var invisibleRegex *regexp2.Regexp

func init() {
	invisibleRegex = regexp2.MustCompile("[^-+',./:@A-Za-z0-9\\[\\]]+", regexp2.None)
}

func DateParse(str string, expandTwoDigitYear bool) FuzzyDate {
	if len(str) > 128 {
		return FuzzyDate{IsTooLong: true} //nolint:exhaustruct
	}

	str, _ = invisibleRegex.Replace(str, " ", -1, -1)
	var fd FuzzyDate

	fd.expandTwoDigitYear = expandTwoDigitYear

	if hasClass(str, classFlagHaveAlpha) {
		parseDay(&str, &fd)
	}
	if hasClass(str, classFlagHaveDigit) {
		parseTime(&str, &fd)
	}

	if hasClass(str, classFlagHaveAlpha|classFlagHaveDigit) {
		if parseEU(&str, &fd) {
			goto ok
		}
		if parseUS(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit|classFlagHaveDash) {
		if parseISO(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit|classFlagHaveDot) {
		if parseJIS(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveAlpha|classFlagHaveDigit|classFlagHaveDash) {
		if parseVMS(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit|classFlagHaveSlash) {
		if parseSla(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit|classFlagHaveDot) {
		if parseDot(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit) {
		if parseISO2(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit) {
		if parseYear(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveAlpha) {
		if parseMon(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit) {
		if parseMDay(&str, &fd) {
			goto ok
		}
	}
	if hasClass(str, classFlagHaveDigit) {
		if parseDDD(&str, &fd) {
			goto ok
		}
	}

ok:
	if hasClass(str, classFlagHaveAlpha) {
		parseBC(&str, &fd)
	}
	if hasClass(str, classFlagHaveDigit) {
		parseFrag(&str, &fd)
	}

	{
		if fd.isBC {
			if fd.CWYearIsSet {
				fd.CWYear = -fd.CWYear + 1
			}
			if fd.YearIsSet {
				fd.Year = -fd.Year + 1
			}
		}

		if fd.expandTwoDigitYear {
			if fd.CWYearIsSet && fd.CWYear >= 0 && fd.CWYear <= 99 {
				if fd.CWYear >= 69 {
					fd.CWYear += 1900
				} else {
					fd.CWYear += 2000
				}
			}
			if fd.YearIsSet && fd.Year >= 0 && fd.Year <= 99 {
				if fd.Year >= 69 {
					fd.Year += 1900
				} else {
					fd.Year += 2000
				}
			}
		}
	}

	if fd.ZoneIsSet && !fd.OffsetIsSet {
		fd.Offset, fd.OffsetIsSet = dateZoneToDiff(fd.Zone)
	}

	return fd
}
