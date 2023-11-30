//go:build testing

package rubydate

// Adapted from https://github.com/ruby/ruby/blob/v3_2_2/test/date/test_date_parse.rb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	type Test struct {
		Str                  string
		ExpandTwoDigitYear   bool
		ExpectedYear         int
		ExpectedYearIsSet    bool
		ExpectedMonth        int
		ExpectedMonthIsSet   bool
		ExpectedDay          int
		ExpectedDayIsSet     bool
		ExpectedHour         int
		ExpectedHourIsSet    bool
		ExpectedMinute       int
		ExpectedMinuteIsSet  bool
		ExpectedSecond       int
		ExpectedSecondIsSet  bool
		ExpectedZone         string
		ExpectedOffset       int
		ExpectedOffsetIsSet  bool
		ExpectedWeekday      int
		ExpectedWeekdayIsSet bool
	}

	tests := []Test{
		// ctime(3), asctime(3)
		{"Sat Aug 28 02:55:50 1999", false, 1999, true, 8, true, 28, true, 2, true, 55, true, 50, true, "", 0, false, 6, true},
		{"Sat Aug 28 02:55:50 02", false, 2, true, 8, true, 28, true, 2, true, 55, true, 50, true, "", 0, false, 6, true},
		{"Sat Aug 28 02:55:50 02", true, 2002, true, 8, true, 28, true, 2, true, 55, true, 50, true, "", 0, false, 6, true},
		{"Sat Aug 28 02:55:50 0002", false, 2, true, 8, true, 28, true, 2, true, 55, true, 50, true, "", 0, false, 6, true},
		{"Sat Aug 28 02:55:50 0002", true, 2, true, 8, true, 28, true, 2, true, 55, true, 50, true, "", 0, false, 6, true},

		// date(1)
		{"Sat Aug 28 02:29:34 JST 1999", false, 1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "JST", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 MET DST 1999", false, 1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "MET DST", 2 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 AMT 1999", false, 1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "AMT", 0, false, 6, true},
		{"Sat Aug 28 02:29:34 PMT 1999", false, 1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "PMT", 0, false, 6, true},
		{"Sat Aug 28 02:29:34 PMT -1999", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "PMT", 0, false, 6, true},

		{"Sat Aug 28 02:29:34 JST 02", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "JST", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 JST 02", true, 2002, true, 8, true, 28, true, 2, true, 29, true, 34, true, "JST", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 JST 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "JST", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 JST 0002", true, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "JST", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 AEST 0002", true, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "AEST", 10 * 3600, true, 6, true},

		{"Sat Aug 28 02:29:34 GMT+09 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT+09", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT+0900 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT+0900", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT+09:00 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT+09:00", 9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT-09 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT-09", -9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT-0900 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT-0900", -9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT-09:00 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT-09:00", -9 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT-090102 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT-090102", -9*3600 - 60 - 2, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT-09:01:02 0002", false, 2, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT-09:01:02", -9*3600 - 60 - 2, true, 6, true},

		{"Sat Aug 28 02:29:34 GMT Standard Time 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT Standard Time", 0 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 Mountain Standard Time 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "Mountain Standard Time", -7 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 Mountain Daylight Time 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "Mountain Daylight Time", -6 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 Mexico Standard Time 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "Mexico Standard Time", -6 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 E. Australia Standard Time 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "E. Australia Standard Time", 10 * 3600, true, 6, true},
		{"Sat Aug 28 02:29:34 W.  Central  Africa  Standard  Time 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "W. Central Africa Standard Time", 1 * 3600, true, 6, true},

		// part of iso 8601
		{"1999-05-23 23:55:21", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"1999-05-23 23:55:21+0900", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+0900", 9 * 3600, true, 0, false},
		{"1999-05-23 23:55:21-0900", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"1999-05-23 23:55:21+09:00", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+09:00", 9 * 3600, true, 0, false},
		{"1999-05-23T23:55:21-09:00", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-09:00", -9 * 3600, true, 0, false},
		{"1999-05-23 23:55:21Z", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"1999-05-23T23:55:21Z", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"-1999-05-23T23:55:21Z", false, -1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"-1999-05-23T23:55:21Z", true, -1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"19990523T23:55:21Z", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},

		{"+011985-04-12", false, 11985, true, 4, true, 12, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"+011985-04-12T10:15:30", false, 11985, true, 4, true, 12, true, 10, true, 15, true, 30, true, "", 0, false, 0, false},
		{"-011985-04-12", false, -11985, true, 4, true, 12, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"-011985-04-12T10:15:30", false, -11985, true, 4, true, 12, true, 10, true, 15, true, 30, true, "", 0, false, 0, false},

		{"02-04-12", false, 2, true, 4, true, 12, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"02-04-12", true, 2002, true, 4, true, 12, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"0002-04-12", false, 2, true, 4, true, 12, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"0002-04-12", true, 2, true, 4, true, 12, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"19990523", true, 1999, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"-19990523", true, -1999, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"990523", true, 1999, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"0523", false, 0, false, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"23", false, 0, false, 0, false, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"19990523 235521", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"990523 235521", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"0523 2355", false, 0, false, 5, true, 23, true, 23, true, 55, true, 0, false, "", 0, false, 0, false},
		{"23 2355", false, 0, false, 0, false, 23, true, 23, true, 55, true, 0, false, "", 0, false, 0, false},

		{"19990523T235521", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"990523T235521", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"19990523T235521.99", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"990523T235521.99", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"0523T2355", false, 0, false, 5, true, 23, true, 23, true, 55, true, 0, false, "", 0, false, 0, false},

		{"19990523T235521+0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+0900", 9 * 3600, true, 0, false},
		{"990523T235521-0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"19990523T235521.99+0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+0900", 9 * 3600, true, 0, false},
		{"990523T235521.99-0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"0523T2355Z", false, 0, false, 5, true, 23, true, 23, true, 55, true, 0, false, "Z", 0, true, 0, false},

		{"19990523235521.123456+0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+0900", 9 * 3600, true, 0, false},
		{"19990523235521.123456-0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"19990523235521,123456+0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+0900", 9 * 3600, true, 0, false},
		{"19990523235521,123456-0900", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},

		{"990523235521,123456-0900", false, 99, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"0523235521,123456-0900", false, 0, false, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"23235521,123456-0900", false, 0, false, 0, false, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"235521,123456-0900", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"5521,123456-0900", false, 0, false, 0, false, 0, false, 0, false, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"21,123456-0900", false, 0, false, 0, false, 0, false, 0, false, 0, false, 21, true, "-0900", -9 * 3600, true, 0, false},

		{"3235521,123456-0900", false, 0, false, 0, false, 3, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"35521,123456-0900", false, 0, false, 0, false, 0, false, 3, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"521,123456-0900", false, 0, false, 0, false, 0, false, 0, false, 5, true, 21, true, "-0900", -9 * 3600, true, 0, false},

		// reversed iso 8601 (?)
		{"23-05-1999", false, 1999, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"23-05-1999 23:55:21", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"23-05--1999 23:55:21", false, -1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"23-05-'99", false, 99, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"23-05-'99", true, 1999, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// broken iso 8601 (?)
		{"19990523T23:55:21Z", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"19990523235521.1234-100", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-100", -1 * 3600, true, 0, false},
		{"19990523235521.1234-10", true, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-10", -10 * 3600, true, 0, false},

		// part of jis x0301
		{"M11.05.23", false, 1878, true, 5, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"T11.05.23 23:55:21+0900", false, 1922, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+0900", 9 * 3600, true, 0, false},
		{"S11.05.23 23:55:21-0900", false, 1936, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-0900", -9 * 3600, true, 0, false},
		{"S40.05.23 23:55:21+09:00", false, 1965, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+09:00", 9 * 3600, true, 0, false},
		{"S40.05.23T23:55:21-09:00", false, 1965, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-09:00", -9 * 3600, true, 0, false},
		{"H11.05.23 23:55:21Z", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"H11.05.23T23:55:21Z", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"H31.04.30 23:55:21Z", false, 2019, true, 4, true, 30, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},
		{"H31.04.30T23:55:21Z", false, 2019, true, 4, true, 30, true, 23, true, 55, true, 21, true, "Z", 0, true, 0, false},

		// ofx date
		{"19990523235521", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"19990523235521.123", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"19990523235521.123[-9]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-9", -(9 * 3600), true, 0, false},
		{"19990523235521.123[+9]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+9", +(9 * 3600), true, 0, false},
		{"19990523235521.123[9]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "9", +(9 * 3600), true, 0, false},
		{"19990523235521.123[9 ]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "9 ", +(9 * 3600), true, 0, false},
		{"19990523235521.123[-9.50]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "-9.50", -(9*3600 + 30*60), true, 0, false},
		{"19990523235521.123[+9.50]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "+9.50", +(9*3600 + 30*60), true, 0, false},
		{"19990523235521.123[-5:EST]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "EST", -5 * 3600, true, 0, false},
		{"19990523235521.123[+9:JST]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "JST", 9 * 3600, true, 0, false},
		{"19990523235521.123[+12:XXX YYY ZZZ]", false, 1999, true, 5, true, 23, true, 23, true, 55, true, 21, true, "XXX YYY ZZZ", 12 * 3600, true, 0, false},
		{"235521.123", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "", 0, false, 0, false},
		{"235521.123[-9]", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "-9", -9 * 3600, true, 0, false},
		{"235521.123[+9]", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "+9", +9 * 3600, true, 0, false},
		{"235521.123[-9 ]", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "-9 ", -9 * 3600, true, 0, false},
		{"235521.123[-5:EST]", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "EST", -5 * 3600, true, 0, false},
		{"235521.123[+9:JST]", false, 0, false, 0, false, 0, false, 23, true, 55, true, 21, true, "JST", +9 * 3600, true, 0, false},

		// rfc 2822
		{"Sun, 22 Aug 1999 00:45:29 -0400", false, 1999, true, 8, true, 22, true, 0, true, 45, true, 29, true, "-0400", -4 * 3600, true, 0, true},
		{"Sun, 22 Aug 1999 00:45:29 -9959", false, 1999, true, 8, true, 22, true, 0, true, 45, true, 29, true, "-9959", -(99*3600 + 59*60), true, 0, true},
		{"Sun, 22 Aug 1999 00:45:29 +9959", false, 1999, true, 8, true, 22, true, 0, true, 45, true, 29, true, "+9959", +(99*3600 + 59*60), true, 0, true},
		{"Sun, 22 Aug 05 00:45:29 -0400", true, 2005, true, 8, true, 22, true, 0, true, 45, true, 29, true, "-0400", -4 * 3600, true, 0, true},
		{"Sun, 22 Aug 49 00:45:29 -0400", true, 2049, true, 8, true, 22, true, 0, true, 45, true, 29, true, "-0400", -4 * 3600, true, 0, true},
		{"Sun, 22 Aug 1999 00:45:29 GMT", false, 1999, true, 8, true, 22, true, 0, true, 45, true, 29, true, "GMT", 0, true, 0, true},
		{"Sun,\x0022\r\nAug\r\n1999\r\n00:45:29\r\nGMT", false, 1999, true, 8, true, 22, true, 0, true, 45, true, 29, true, "GMT", 0, true, 0, true},
		{"Sun, 22 Aug 1999 00:45 GMT", false, 1999, true, 8, true, 22, true, 0, true, 45, true, 0, false, "GMT", 0, true, 0, true},
		{"Sun, 22 Aug -1999 00:45 GMT", false, -1999, true, 8, true, 22, true, 0, true, 45, true, 0, false, "GMT", 0, true, 0, true},
		{"Sun, 22 Aug 99 00:45:29 UT", true, 1999, true, 8, true, 22, true, 0, true, 45, true, 29, true, "UT", 0, true, 0, true},
		{"Sun, 22 Aug 0099 00:45:29 UT", true, 99, true, 8, true, 22, true, 0, true, 45, true, 29, true, "UT", 0, true, 0, true},

		// rfc 850, obsoleted by rfc 1036
		{"Tuesday, 02-Mar-99 11:20:32 GMT", true, 1999, true, 3, true, 2, true, 11, true, 20, true, 32, true, "GMT", 0, true, 2, true},

		// W3C Working Draft - XForms - 4.8 Time
		{"2000-01-31 13:20:00-5", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "-5", -5 * 3600, true, 0, false},

		// [-+]\d+.\d+
		{"2000-01-31 13:20:00-5.5", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "-5.5", -5*3600 - 30*60, true, 0, false},
		{"2000-01-31 13:20:00-5,5", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "-5,5", -5*3600 - 30*60, true, 0, false},
		{"2000-01-31 13:20:00+3.5", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "+3.5", 3*3600 + 30*60, true, 0, false},
		{"2000-01-31 13:20:00+3,5", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "+3,5", 3*3600 + 30*60, true, 0, false},

		// mil
		{"2000-01-31 13:20:00 Z", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "Z", 0 * 3600, true, 0, false},
		{"2000-01-31 13:20:00 H", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "H", 8 * 3600, true, 0, false},
		{"2000-01-31 13:20:00 M", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "M", 12 * 3600, true, 0, false},
		{"2000-01-31 13:20 M", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, false, "M", 12 * 3600, true, 0, false},
		{"2000-01-31 13:20:00 S", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "S", -6 * 3600, true, 0, false},
		{"2000-01-31 13:20:00 A", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "A", 1 * 3600, true, 0, false},
		{"2000-01-31 13:20:00 P", false, 2000, true, 1, true, 31, true, 13, true, 20, true, 0, true, "P", -3 * 3600, true, 0, false},

		// dot
		{"1999.5.2", false, 1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"1999.05.02", false, 1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"-1999.05.02", false, -1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"0099.5.2", false, 99, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"0099.5.2", true, 99, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"'99.5.2", false, 99, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"'99.5.2", true, 1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// reversed dot
		{"2.5.1999", false, 1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"02.05.1999", false, 1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"02.05.-1999", false, -1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"2.5.0099", false, 99, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"2.5.0099", true, 99, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"2.5.'99", false, 99, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"2.5.'99", true, 1999, true, 5, true, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// vms
		{"08-DEC-1988", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31-JAN-1999", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31-JAN--1999", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"08-DEC-88", false, 88, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"08-DEC-88", true, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"08-DEC-0088", false, 88, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"08-DEC-0088", true, 88, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// swapped vms
		{"DEC-08-1988", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"JAN-31-1999", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"JAN-31--1999", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"JAN-1999", false, 1999, true, 1, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"JAN--1999", false, -1999, true, 1, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// reversed vms
		{"1988-DEC-08", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"1999-JAN-31", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"-1999-JAN-31", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"0088-DEC-08", false, 88, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"0088-DEC-08", true, 88, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		{"'88/12/8", false, 88, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"'88/12/8", true, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// non-spaced eu
		{"08/dec/1988", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31/jan/1999", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31/jan/-1999", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"08.dec.1988", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31.jan.1999", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31.jan.-1999", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// non-spaced us
		{"dec/08/1988", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan/31/1999", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan/31/-1999", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan/31", false, 0, false, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan/1988", false, 1988, true, 1, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"dec.08.1988", false, 1988, true, 12, true, 8, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan.31.1999", false, 1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan.31.-1999", false, -1999, true, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan.31", false, 0, false, 1, true, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"jan.1988", false, 1988, true, 1, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// month and day of month
		{"Jan 1", false, 0, false, 1, true, 1, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Jul 11", false, 0, false, 7, true, 11, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"July 11", false, 0, false, 7, true, 11, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sept 23", false, 0, false, 9, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sep. 23", false, 0, false, 9, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sept. 23", false, 0, false, 9, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"September 23", false, 0, false, 9, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"October 1st", false, 0, false, 10, true, 1, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"October 23rd", false, 0, false, 10, true, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"October 25th 1999", false, 1999, true, 10, true, 25, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"October 25th -1999", false, -1999, true, 10, true, 25, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"october 25th 1999", false, 1999, true, 10, true, 25, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"OCTOBER 25th 1999", false, 1999, true, 10, true, 25, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"oCtoBer 25th 1999", false, 1999, true, 10, true, 25, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"aSep 23", false, 0, false, 0, false, 23, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// month and year
		{"Sept 1990", false, 1990, true, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sept '90", false, 90, true, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sept '90", true, 1990, true, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"1990/09", false, 1990, true, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"09/1990", false, 1990, true, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"aSep '90", false, 90, true, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// year
		{"'90", false, 90, true, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"'90", true, 1990, true, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// month
		{"Jun", false, 0, false, 6, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"June", false, 0, false, 6, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sep", false, 0, false, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"Sept", false, 0, false, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"September", false, 0, false, 9, true, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"aSep", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// day of month
		{"1st", false, 0, false, 0, false, 1, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"2nd", false, 0, false, 0, false, 2, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"3rd", false, 0, false, 0, false, 3, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"4th", false, 0, false, 0, false, 4, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"29th", false, 0, false, 0, false, 29, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"31st", false, 0, false, 0, false, 31, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"1sta", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// era
		{"Sat Aug 28 02:29:34 GMT CE 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT C.E. 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT BCE 2000", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT B.C.E. 2000", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT AD 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT A.D. 2000", false, 2000, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT BC 2000", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT B.C. 2000", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT 2000 BC", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT 2000 BCE", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT 2000 B.C.", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},
		{"Sat Aug 28 02:29:34 GMT 2000 B.C.E.", false, -1999, true, 8, true, 28, true, 2, true, 29, true, 34, true, "GMT", 0, true, 6, true},

		// collection
		{"Tuesday, May 18, 1999 Published at 13:36 GMT 14:36 UK", false, 1999, true, 5, true, 18, true, 13, true, 36, true, 0, false, "GMT", 0, true, 2, true},          // bbc.co.uk
		{"July 20, 2000 Web posted at: 3:37 p.m. EDT (1937 GMT)", false, 2000, true, 7, true, 20, true, 15, true, 37, true, 0, false, "EDT", -4 * 3600, true, 0, false}, // cnn.com
		{"12:54 p.m. EDT, September 11, 2006", false, 2006, true, 9, true, 11, true, 12, true, 54, true, 0, false, "EDT", -4 * 3600, true, 0, false},                    // cnn.com
		{"February 04, 2001 at 10:59 AM PST", false, 2001, true, 2, true, 4, true, 10, true, 59, true, 0, false, "PST", -8 * 3600, true, 0, false},                      // old amazon.com
		{"Monday May 08, @01:55PM", false, 0, false, 5, true, 8, true, 13, true, 55, true, 0, false, "", 0, false, 1, true},                                             // slashdot.org
		{"06.June 2005", false, 2005, true, 6, true, 6, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},                                                     // dhl.com

		// etc.
		{"8:00 pm lt", false, 0, false, 0, false, 0, false, 20, true, 0, true, 0, false, "lt", 0, false, 0, false},
		{"4:00 AM, Jan. 12, 1990", false, 1990, true, 1, true, 12, true, 4, true, 0, true, 0, false, "", 0, false, 0, false},
		{"Jan. 12 4:00 AM 1990", false, 1990, true, 1, true, 12, true, 4, true, 0, true, 0, false, "", 0, false, 0, false},
		{"1990-01-12 04:00:00+00", false, 1990, true, 1, true, 12, true, 4, true, 0, true, 0, true, "+00", 0, true, 0, false},
		{"1990-01-11 20:00:00-08", false, 1990, true, 1, true, 11, true, 20, true, 0, true, 0, true, "-08", -8 * 3600, true, 0, false},
		{"1990/01/12 04:00:00", false, 1990, true, 1, true, 12, true, 4, true, 0, true, 0, true, "", 0, false, 0, false},
		{"Thu Jan 11 20:00:00 PST 1990", false, 1990, true, 1, true, 11, true, 20, true, 0, true, 0, true, "PST", -8 * 3600, true, 4, true},
		{"Fri Jan 12 04:00:00 GMT 1990", false, 1990, true, 1, true, 12, true, 4, true, 0, true, 0, true, "GMT", 0, true, 5, true},
		{"Thu, 11 Jan 1990 20:00:00 -0800", false, 1990, true, 1, true, 11, true, 20, true, 0, true, 0, true, "-0800", -8 * 3600, true, 4, true},
		{"12-January-1990, 04:00 WET", false, 1990, true, 1, true, 12, true, 4, true, 0, true, 0, false, "WET", 0 * 3600, true, 0, false},
		{"jan 2 3 am +4 5", false, 5, true, 1, true, 2, true, 3, true, 0, false, 0, false, "+4", 4 * 3600, true, 0, false},
		{"jan 2 3 am +4 5", true, 2005, true, 1, true, 2, true, 3, true, 0, false, 0, false, "+4", 4 * 3600, true, 0, false},
		{"fri1feb3bc4pm+5", false, -2, true, 2, true, 1, true, 16, true, 0, false, 0, false, "+5", 5 * 3600, true, 5, true},
		{"fri1feb3bc4pm+5", true, -2, true, 2, true, 1, true, 16, true, 0, false, 0, false, "+5", 5 * 3600, true, 5, true},
		{"03 feb 1st", false, 03, true, 2, true, 1, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// apostrophe
		{"July 4, '79", true, 1979, true, 7, true, 4, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"4th July '79", true, 1979, true, 7, true, 4, true, 0, false, 0, false, 0, false, "", 0, false, 0, false},

		// day of week
		{"Sunday", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, true},
		{"Mon", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 1, true},
		{"Tue", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 2, true},
		{"Wed", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 3, true},
		{"Thurs", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 4, true},
		{"Friday", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 5, true},
		{"Sat.", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 6, true},
		{"sat.", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 6, true},
		{"SAT.", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 6, true},
		{"sAt.", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 6, true},

		// time
		{"09:55", false, 0, false, 0, false, 0, false, 9, true, 55, true, 0, false, "", 0, false, 0, false},
		{"09:55:30", false, 0, false, 0, false, 0, false, 9, true, 55, true, 30, true, "", 0, false, 0, false},
		{"09:55:30am", false, 0, false, 0, false, 0, false, 9, true, 55, true, 30, true, "", 0, false, 0, false},
		{"09:55:30pm", false, 0, false, 0, false, 0, false, 21, true, 55, true, 30, true, "", 0, false, 0, false},
		{"09:55:30a.m.", false, 0, false, 0, false, 0, false, 9, true, 55, true, 30, true, "", 0, false, 0, false},
		{"09:55:30p.m.", false, 0, false, 0, false, 0, false, 21, true, 55, true, 30, true, "", 0, false, 0, false},
		{"09:55:30pm GMT", false, 0, false, 0, false, 0, false, 21, true, 55, true, 30, true, "GMT", 0, true, 0, false},
		{"09:55:30p.m. GMT", false, 0, false, 0, false, 0, false, 21, true, 55, true, 30, true, "GMT", 0, true, 0, false},
		{"09:55+0900", false, 0, false, 0, false, 0, false, 9, true, 55, true, 0, false, "+0900", 9 * 3600, true, 0, false},
		{"09 AM", false, 0, false, 0, false, 0, false, 9, true, 0, false, 0, false, "", 0, false, 0, false},
		{"09am", false, 0, false, 0, false, 0, false, 9, true, 0, false, 0, false, "", 0, false, 0, false},
		{"09 A.M.", false, 0, false, 0, false, 0, false, 9, true, 0, false, 0, false, "", 0, false, 0, false},
		{"09 PM", false, 0, false, 0, false, 0, false, 21, true, 0, false, 0, false, "", 0, false, 0, false},
		{"09pm", false, 0, false, 0, false, 0, false, 21, true, 0, false, 0, false, "", 0, false, 0, false},
		{"09 P.M.", false, 0, false, 0, false, 0, false, 21, true, 0, false, 0, false, "", 0, false, 0, false},

		{"9h22m23s", false, 0, false, 0, false, 0, false, 9, true, 22, true, 23, true, "", 0, false, 0, false},
		{"9h 22m 23s", false, 0, false, 0, false, 0, false, 9, true, 22, true, 23, true, "", 0, false, 0, false},
		{"9h22m", false, 0, false, 0, false, 0, false, 9, true, 22, true, 0, false, "", 0, false, 0, false},
		{"9h 22m", false, 0, false, 0, false, 0, false, 9, true, 22, true, 0, false, "", 0, false, 0, false},
		{"9h", false, 0, false, 0, false, 0, false, 9, true, 0, false, 0, false, "", 0, false, 0, false},
		{"9h 22m 23s am", false, 0, false, 0, false, 0, false, 9, true, 22, true, 23, true, "", 0, false, 0, false},
		{"9h 22m 23s pm", false, 0, false, 0, false, 0, false, 21, true, 22, true, 23, true, "", 0, false, 0, false},
		{"9h 22m am", false, 0, false, 0, false, 0, false, 9, true, 22, true, 0, false, "", 0, false, 0, false},
		{"9h 22m pm", false, 0, false, 0, false, 0, false, 21, true, 22, true, 0, false, "", 0, false, 0, false},
		{"9h am", false, 0, false, 0, false, 0, false, 9, true, 0, false, 0, false, "", 0, false, 0, false},
		{"9h pm", false, 0, false, 0, false, 0, false, 21, true, 0, false, 0, false, "", 0, false, 0, false},

		{"00:00", false, 0, false, 0, false, 0, false, 0, true, 0, true, 0, false, "", 0, false, 0, false},
		{"01:00", false, 0, false, 0, false, 0, false, 1, true, 0, true, 0, false, "", 0, false, 0, false},
		{"11:00", false, 0, false, 0, false, 0, false, 11, true, 0, true, 0, false, "", 0, false, 0, false},
		{"12:00", false, 0, false, 0, false, 0, false, 12, true, 0, true, 0, false, "", 0, false, 0, false},
		{"13:00", false, 0, false, 0, false, 0, false, 13, true, 0, true, 0, false, "", 0, false, 0, false},
		{"23:00", false, 0, false, 0, false, 0, false, 23, true, 0, true, 0, false, "", 0, false, 0, false},
		{"24:00", false, 0, false, 0, false, 0, false, 24, true, 0, true, 0, false, "", 0, false, 0, false},

		{"00:00 AM", false, 0, false, 0, false, 0, false, 0, true, 0, true, 0, false, "", 0, false, 0, false},
		{"12:00 AM", false, 0, false, 0, false, 0, false, 0, true, 0, true, 0, false, "", 0, false, 0, false},
		{"01:00 AM", false, 0, false, 0, false, 0, false, 1, true, 0, true, 0, false, "", 0, false, 0, false},
		{"11:00 AM", false, 0, false, 0, false, 0, false, 11, true, 0, true, 0, false, "", 0, false, 0, false},
		{"00:00 PM", false, 0, false, 0, false, 0, false, 12, true, 0, true, 0, false, "", 0, false, 0, false},
		{"12:00 PM", false, 0, false, 0, false, 0, false, 12, true, 0, true, 0, false, "", 0, false, 0, false},
		{"01:00 PM", false, 0, false, 0, false, 0, false, 13, true, 0, true, 0, false, "", 0, false, 0, false},
		{"11:00 PM", false, 0, false, 0, false, 0, false, 23, true, 0, true, 0, false, "", 0, false, 0, false},

		// pick up the rest
		{"2000-01-02 1", false, 2000, true, 1, true, 2, true, 1, true, 0, false, 0, false, "", 0, false, 0, false},
		{"2000-01-02 23", false, 2000, true, 1, true, 2, true, 23, true, 0, false, 0, false, "", 0, false, 0, false},
		{"2000-01-02 24", false, 2000, true, 1, true, 2, true, 24, true, 0, false, 0, false, "", 0, false, 0, false},
		{"1 03:04:05", false, 0, false, 0, false, 1, true, 3, true, 4, true, 5, true, "", 0, false, 0, false},
		{"02 03:04:05", false, 0, false, 0, false, 2, true, 3, true, 4, true, 5, true, "", 0, false, 0, false},
		{"31 03:04:05", false, 0, false, 0, false, 31, true, 3, true, 4, true, 5, true, "", 0, false, 0, false},

		// null, space
		{"", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{" ", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"          ", true, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"\t", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"\n", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"\v", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"\f", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"\r", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"\t\n\v\f\r ", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", 0, false, 0, false},
		{"1999-05-23\t\n\v\f\r 21:34:56", false, 1999, true, 5, true, 23, true, 21, true, 34, true, 56, true, "", 0, false, 0, false},
	}

	for _, test := range tests {
		description := fmt.Sprintf("%s (two digit year: %t)", test.Str, test.ExpandTwoDigitYear)

		fd := DateParse(test.Str, test.ExpandTwoDigitYear)
		require.Equal(t, test.ExpectedYearIsSet, fd.YearIsSet, description)
		if fd.YearIsSet {
			require.Equal(t, test.ExpectedYear, fd.Year, description)
		}
		require.Equal(t, test.ExpectedMonthIsSet, fd.MonthIsSet, description)
		if fd.MonthIsSet {
			require.Equal(t, test.ExpectedMonth, fd.Month, description)
		}
		require.Equal(t, test.ExpectedDayIsSet, fd.DayIsSet, description)
		if fd.DayIsSet {
			require.Equal(t, test.ExpectedDay, fd.Day, description)
		}
		require.Equal(t, test.ExpectedHourIsSet, fd.HourIsSet, description)
		if fd.HourIsSet {
			require.Equal(t, test.ExpectedHour, fd.Hour, description)
		}
		require.Equal(t, test.ExpectedMinuteIsSet, fd.MinuteIsSet, description)
		if fd.MinuteIsSet {
			require.Equal(t, test.ExpectedMinute, fd.Minute, description)
		}
		require.Equal(t, test.ExpectedSecondIsSet, fd.SecondIsSet, description)
		if fd.SecondIsSet {
			require.Equal(t, test.ExpectedSecond, fd.Second, description)
		}
		require.Equal(t, test.ExpectedZone != "", fd.ZoneIsSet, description)
		if fd.ZoneIsSet {
			require.Equal(t, test.ExpectedZone, fd.Zone, description)
		}
		require.Equal(t, test.ExpectedOffsetIsSet, fd.OffsetIsSet, description)
		if fd.OffsetIsSet {
			require.Equal(t, test.ExpectedOffset, fd.Offset, description)
		}
		require.Equal(t, test.ExpectedWeekdayIsSet, fd.WeekdayIsSet, description)
		if fd.WeekdayIsSet {
			require.Equal(t, test.ExpectedWeekday, fd.Weekday, description)
		}
	}
}

func TestParseSlashExp(t *testing.T) {
	type Test struct {
		Str                string
		ExpandTwoDigitYear bool
		ExpectedYear       int
		ExpectedYearIsSet  bool
		ExpectedMonth      int
		ExpectedMonthIsSet bool
		ExpectedDay        int
		ExpectedDayIsSet   bool
	}

	tests := []Test{
		// little
		{"2/5/1999", false, 1999, true, 5, true, 2, true},
		{"02/05/1999", false, 1999, true, 5, true, 2, true},
		{"02/05/-1999", false, -1999, true, 5, true, 2, true},
		{"05/02", false, 0, false, 5, true, 2, true},
		{" 5/ 2", false, 0, false, 5, true, 2, true},

		{"2/5/'99", true, 1999, true, 5, true, 2, true},
		{"2/5/0099", false, 99, true, 5, true, 2, true},
		{"2/5/0099", true, 99, true, 5, true, 2, true},

		{"2/5 1999", false, 1999, true, 5, true, 2, true},
		{"2/5-1999", false, 1999, true, 5, true, 2, true},
		{"2/5--1999", false, -1999, true, 5, true, 2, true},

		// big
		{"99/5/2", false, 99, true, 5, true, 2, true},
		{"99/5/2", true, 1999, true, 5, true, 2, true},

		{"1999/5/2", false, 1999, true, 5, true, 2, true},
		{"1999/05/02", false, 1999, true, 5, true, 2, true},
		{"-1999/05/02", false, -1999, true, 5, true, 2, true},

		{"0099/5/2", false, 99, true, 5, true, 2, true},
		{"0099/5/2", true, 99, true, 5, true, 2, true},

		{"'99/5/2", false, 99, true, 5, true, 2, true},
		{"'99/5/2", true, 1999, true, 5, true, 2, true},
	}

	for _, test := range tests {
		description := fmt.Sprintf("%s (two digit year: %t)", test.Str, test.ExpandTwoDigitYear)

		fd := DateParse(test.Str, test.ExpandTwoDigitYear)
		require.Equal(t, test.ExpectedYearIsSet, fd.YearIsSet, description)
		if fd.YearIsSet {
			require.Equal(t, test.ExpectedYear, fd.Year, description)
		}
		require.Equal(t, test.ExpectedMonthIsSet, fd.MonthIsSet, description)
		if fd.MonthIsSet {
			require.Equal(t, test.ExpectedMonth, fd.Month, description)
		}
		require.Equal(t, test.ExpectedDayIsSet, fd.DayIsSet, description)
		if fd.DayIsSet {
			require.Equal(t, test.ExpectedDay, fd.Day, description)
		}
		require.False(t, fd.HourIsSet, description)
		require.False(t, fd.MinuteIsSet, description)
		require.False(t, fd.SecondIsSet, description)
		require.Empty(t, fd.Zone, description)
		require.False(t, fd.OffsetIsSet, description)
		require.False(t, fd.WeekdayIsSet, description)
	}
}

func TestParse2(t *testing.T) {
	type HMSTest struct {
		Str                string
		ExpectedHour       int
		ExpectedMinute     int
		ExpectedSecond     int
		ExpectedNanosecond int
	}
	hmsTests := []HMSTest{
		{"22:45:59.5", 22, 45, 59, 500000000},
		{"22:45:59.05", 22, 45, 59, 50000000},
		{"22:45:59.005", 22, 45, 59, 5000000},
		{"22:45:59.0123", 22, 45, 59, 12300000},
		{"224559.5", 22, 45, 59, 500000000},
		{"224559.05", 22, 45, 59, 50000000},
		{"224559.005", 22, 45, 59, 5000000},
		{"224559.0123", 22, 45, 59, 12300000},
	}
	for _, test := range hmsTests {
		fd := DateParse(test.Str, false)
		require.True(t, fd.HourIsSet, test.Str)
		require.Equal(t, test.ExpectedHour, fd.Hour, test.Str)
		require.True(t, fd.MinuteIsSet, test.Str)
		require.Equal(t, test.ExpectedMinute, fd.Minute, test.Str)
		require.True(t, fd.SecondIsSet, test.Str)
		require.Equal(t, test.ExpectedSecond, fd.Second, test.Str)
		require.True(t, fd.NanosecondIsSet, test.Str)
		require.Equal(t, test.ExpectedNanosecond, fd.Nanosecond, test.Str)
	}

	type CWTest struct {
		Str                 string
		ExpandTwoDigitYear  bool
		ExpectedCWYear      int
		ExpectedCWYearIsSet bool
		ExpectedCWeek       int
		ExpectedCWeekIsSet  bool
		ExpectedCWDay       int
		ExpectedCWDayIsSet  bool
	}
	cwTests := []CWTest{
		{"2006-w15-5", false, 2006, true, 15, true, 5, true},
		{"2006w155", false, 2006, true, 15, true, 5, true},
		{"06w155", false, 6, true, 15, true, 5, true},
		{"06w155", true, 2006, true, 15, true, 5, true},

		{"2006-w15", true, 2006, true, 15, true, 0, false},
		{"2006w15", false, 2006, true, 15, true, 0, false},

		{"-w15-5", false, 0, false, 15, true, 5, true},
		{"-w155", false, 0, false, 15, true, 5, true},

		{"-w15", false, 0, false, 15, true, 0, false},

		{"-w-5", false, 0, false, 0, false, 5, true},
	}
	for _, test := range cwTests {
		description := fmt.Sprintf("%s (two digit year: %t)", test.Str, test.ExpandTwoDigitYear)
		fd := DateParse(test.Str, test.ExpandTwoDigitYear)
		require.Equal(t, test.ExpectedCWYearIsSet, fd.CWYearIsSet, description)
		require.Equal(t, test.ExpectedCWYear, fd.CWYear, description)
		require.Equal(t, test.ExpectedCWeekIsSet, fd.CWeekIsSet, description)
		require.Equal(t, test.ExpectedCWeek, fd.CWeek, description)
		require.Equal(t, test.ExpectedCWDayIsSet, fd.CWDayIsSet, description)
		require.Equal(t, test.ExpectedCWDay, fd.CWDay, description)
	}

	type YMDTest struct {
		Str                string
		ExpectedYear       int
		ExpectedYearIsSet  bool
		ExpectedMonth      int
		ExpectedMonthIsSet bool
		ExpectedDay        int
		ExpectedDayIsSet   bool
	}
	ymdTests := []YMDTest{
		{"--11-29", 0, false, 11, true, 29, true},
		{"--1129", 0, false, 11, true, 29, true},
		{"--11", 0, false, 11, true, 0, false},
		{"---29", 0, false, 0, false, 29, true},
	}
	for _, test := range ymdTests {
		fd := DateParse(test.Str, false)
		require.Equal(t, test.ExpectedYearIsSet, fd.YearIsSet, test.Str)
		require.Equal(t, test.ExpectedYear, fd.Year, test.Str)
		require.Equal(t, test.ExpectedMonthIsSet, fd.MonthIsSet, test.Str)
		require.Equal(t, test.ExpectedMonth, fd.Month, test.Str)
		require.Equal(t, test.ExpectedDayIsSet, fd.DayIsSet, test.Str)
		require.Equal(t, test.ExpectedDay, fd.Day, test.Str)
	}

	type YDTest struct {
		Str                    string
		ExpandTwoDigitYear     bool
		ExpectedYear           int
		ExpectedYearIsSet      bool
		ExpectedDayOfYear      int
		ExpectedDayOfYearIsSet bool
	}
	ydTests := []YDTest{
		{"-333", false, 0, false, 333, true},
		{"2006-333", false, 2006, true, 333, true},
		{"2006333", false, 2006, true, 333, true},
		{"06333", false, 6, true, 333, true},
		{"06333", true, 2006, true, 333, true},
		{"333", false, 0, false, 333, true},
	}
	for _, test := range ydTests {
		fd := DateParse(test.Str, test.ExpandTwoDigitYear)
		require.Equal(t, test.ExpectedYearIsSet, fd.YearIsSet, test.Str)
		require.Equal(t, test.ExpectedYear, fd.Year, test.Str)
		require.Equal(t, test.ExpectedDayOfYearIsSet, fd.DayOfYearIsSet, test.Str)
		require.Equal(t, test.ExpectedDayOfYear, fd.DayOfYear, test.Str)
	}

	fd := DateParse("", false)
	require.False(t, fd.YearIsSet)
	require.False(t, fd.MonthIsSet)
	require.False(t, fd.DayIsSet)
	require.False(t, fd.WeekdayIsSet)
	require.False(t, fd.HourIsSet)
	require.False(t, fd.MinuteIsSet)
	require.False(t, fd.SecondIsSet)
	require.False(t, fd.NanosecondIsSet)
	require.False(t, fd.ZoneIsSet)
	require.False(t, fd.CWYearIsSet)
	require.False(t, fd.CWeekIsSet)
	require.False(t, fd.CWDayIsSet)
	require.False(t, fd.DayOfYearIsSet)
	require.False(t, fd.OffsetIsSet)
}

// Tests overriding length limit are not included as custom limits are not supported, but this should be easy
// to add if we need to

func TestLengthLimit(t *testing.T) {
	str := strings.Repeat("1", 1000)
	fd := DateParse(str, false)
	require.Equal(t, FuzzyDate{IsTooLong: true}, fd) //nolint:exhaustruct
}
