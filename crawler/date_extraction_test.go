//go:build testing

package crawler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTryExtractTextDatePassing(t *testing.T) {
	type Test struct {
		Text  string
		Year  int
		Month int
		Day   int
	}
	tests := []Test{
		{Text: "-  1 February 2015", Year: 2015, Month: 2, Day: 1},
		{Text: "- April 18, 2021", Year: 2021, Month: 4, Day: 18},
		{Text: ", April 27, 2014", Year: 2014, Month: 4, Day: 27},
		{Text: "(04-05-2020)", Year: 2020, Month: 5, Day: 4},
		{Text: "(December 10 2005)", Year: 2005, Month: 12, Day: 10},
		{Text: "(February 8 2012, last updated February 7 2013)", Year: 2012, Month: 2, Day: 8},
		{Text: "(May 11 2007)", Year: 2007, Month: 5, Day: 11},
		{Text: "01 Dec 2015", Year: 2015, Month: 12, Day: 1},
		{Text: "01 November 2016", Year: 2016, Month: 11, Day: 1},
		{Text: "04 Nov 2019 »", Year: 2019, Month: 11, Day: 4},
		{Text: "05 October 2018 at 09:21 UTC", Year: 2018, Month: 10, Day: 5},
		{Text: "07 JAN 2017", Year: 2017, Month: 1, Day: 7},
		{Text: "07 Mar 2011:", Year: 2011, Month: 3, Day: 7},
		{Text: "1 Jul 2018", Year: 2018, Month: 7, Day: 1},
		{Text: "11 Apr, 2021", Year: 2021, Month: 4, Day: 11},
		{Text: "2010-11-06", Year: 2010, Month: 11, Day: 6},
		{Text: "2011-05-21 | ", Year: 2011, Month: 5, Day: 21},
		{Text: "2013 Aug 14 -", Year: 2013, Month: 8, Day: 14},
		{Text: "2013 August 24", Year: 2013, Month: 8, Day: 24},
		{Text: "2014-11-12 04:13", Year: 2014, Month: 11, Day: 12},
		{Text: "2014-11-14", Year: 2014, Month: 11, Day: 14},
		{Text: "2020-02-06:", Year: 2020, Month: 2, Day: 6},
		{Text: "April 21, 2021", Year: 2021, Month: 4, Day: 21},
		{Text: "By Michael Altfield, on April 15th, 2020", Year: 2020, Month: 4, Day: 15},
		{Text: "Dec 10 2020", Year: 2020, Month: 12, Day: 10},
		{Text: "Dec 15 '20", Year: 2020, Month: 12, Day: 15},
		{Text: "December 17th, 2010 | Tags:", Year: 2010, Month: 12, Day: 17},
		{Text: "Disabling Emojis In WordPress — November 28, 2016", Year: 2016, Month: 11, Day: 28},
		{Text: "entry was around 2009-04-11.", Year: 2009, Month: 4, Day: 11},
		{Text: "Friday, 6 November 2015", Year: 2015, Month: 11, Day: 6},
		{Text: "Jan 13, 2018				•", Year: 2018, Month: 1, Day: 13},
		{Text: "May 31, 2020: A toy compiler from scratch", Year: 2020, Month: 5, Day: 31},
		{Text: "Never Graduate Week 2018! — May 16, 2018", Year: 2018, Month: 5, Day: 16},
		{Text: "on 2020-10-09", Year: 2020, Month: 10, Day: 9},
		{Text: "Posted by ＳｔｕｆｆｏｎｍｙＭｉｎｄ on February 25, 2021", Year: 2021, Month: 2, Day: 25},
		{Text: "Things I learnt 23 June 2019", Year: 2019, Month: 6, Day: 23},
		{Text: "Tue 18 November 2014", Year: 2014, Month: 11, Day: 18},
		{Text: "2021-07-16 17:00:00+00:00", Year: 2021, Month: 7, Day: 16},
		{Text: "/ Dec 14, 2018", Year: 2018, Month: 12, Day: 14},
		{Text: "07H05 (2018-04-13)", Year: 2018, Month: 4, Day: 13}, //Trips up Ruby DateParse
	}

	for _, test := range tests {
		date := tryExtractTextDate(test.Text, false)
		require.NotNil(t, date, test.Text)
		require.Equal(t, test.Year, date.Year, test.Text)
		require.Equal(t, time.Month(test.Month), date.Month, test.Text)
		require.Equal(t, test.Day, date.Day, test.Text)
	}
}

func TestTryExtractTextDatePassingGuessYear(t *testing.T) {
	type Test struct {
		Text  string
		Month int
		Day   int
	}
	tests := []Test{
		{Text: "Jan 23", Month: 1, Day: 23},
		{Text: "July 5", Month: 7, Day: 5},
	}

	for _, test := range tests {
		date := tryExtractTextDate(test.Text, true)
		require.NotNil(t, date, test.Text)
		require.Equal(t, time.Month(test.Month), date.Month, test.Text)
		require.Equal(t, test.Day, date.Day, test.Text)
	}
}

func TestTryExtractTextDateFailing(t *testing.T) {
	tests := []string{
		"",
		"   \n\t",
		"asdf",
		".",
		"A quick brown fox is jumping over the lazy dog. It is jumping and jumping. Jumping and jumping. Jumping and jumping. Merry Christmas Dec 25, 2019",
		"4/5/2020",
		"4/16/2020",
		"16/4/2020",
		"'11'",
		"(21 comments)",
		", April 2019",
		"(Dec '17)",
		"#21 -",
		"21",
		"21 Comments",
		"21 minutes",
		"And how this can help you think about 2021.",
		"hg advent -m '02: extensions'",
		"Zig: December 2017 in Review",
		"Source: Posted on twitter by user @mrdrozdov in Jan 2020.",
	}

	for _, test := range tests {
		date := tryExtractTextDate(test, true)
		require.Nil(t, date, test)
	}
}
