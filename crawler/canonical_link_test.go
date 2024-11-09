package crawler

import (
	"net/url"
	"testing"

	"feedrewind.com/oops"

	"github.com/stretchr/testify/require"
)

func TestToCanonicalLink(t *testing.T) {
	type Expected struct {
		url          string
		canonicalUrl string
	}
	type Test struct {
		description string
		url         string
		fetchUrl    string
		expected    *Expected
	}

	tests := []Test{
		{
			description: "should parse absolute http url",
			url:         "http://ya.ru/hi",
			fetchUrl:    "http://ya.ru",
			expected: &Expected{
				url:          "http://ya.ru/hi",
				canonicalUrl: "ya.ru/hi",
			},
		},
		{
			description: "should parse absolute https url",
			url:         "https://ya.ru/hi",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/hi",
				canonicalUrl: "ya.ru/hi",
			},
		},
		{
			description: "should ignore non-http(s) url",
			url:         "ftp://ya.ru/hi",
			fetchUrl:    "ftp://ya.ru",
			expected:    nil,
		},
		{
			description: "should parse relative url",
			url:         "20201227",
			fetchUrl:    "https://apenwarr.ca/log/",
			expected: &Expected{
				url:          "https://apenwarr.ca/log/20201227",
				canonicalUrl: "apenwarr.ca/log/20201227",
			},
		},
		{
			description: "should parse relative url with /",
			url:         "/abc",
			fetchUrl:    "https://ya.ru/hi/hello",
			expected: &Expected{
				url:          "https://ya.ru/abc",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should parse relative url with ./",
			url:         "./abc",
			fetchUrl:    "https://ya.ru/hi/hello",
			expected: &Expected{
				url:          "https://ya.ru/hi/abc",
				canonicalUrl: "ya.ru/hi/abc",
			},
		},
		{
			description: "should parse relative url with ../",
			url:         "../abc",
			fetchUrl:    "https://ya.ru/hi/hello/bonjour",
			expected: &Expected{
				url:          "https://ya.ru/hi/abc",
				canonicalUrl: "ya.ru/hi/abc",
			},
		},
		{
			description: "should parse relative url with //",
			url:         "//ya.ru/abc",
			fetchUrl:    "https://ya.ru/hi/hello",
			expected: &Expected{
				url:          "https://ya.ru/abc",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should drop fragment",
			url:         "https://ya.ru/abc#def",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/abc",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should include non-standard port in canonical url",
			url:         "https://ya.ru:444/abc",
			fetchUrl:    "https://ya.ru:444",
			expected: &Expected{
				url:          "https://ya.ru:444/abc",
				canonicalUrl: "ya.ru:444/abc",
			},
		},
		{
			description: "should drop standard http port in canonical url",
			url:         "http://ya.ru:80/abc",
			fetchUrl:    "http://ya.ru:80",
			expected: &Expected{
				url:          "http://ya.ru/abc",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should drop standard https port in canonical url",
			url:         "https://ya.ru:443/abc",
			fetchUrl:    "https://ya.ru:443",
			expected: &Expected{
				url:          "https://ya.ru/abc",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should include whitelisted query in canonical url",
			url:         "https://ya.ru/abc?page=2",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/abc?page=2",
				canonicalUrl: "ya.ru/abc?page=2",
			},
		},
		{
			description: "should include whitelisted query without value in canonical url",
			url:         "https://ya.ru/abc?page",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/abc?page",
				canonicalUrl: "ya.ru/abc?page",
			},
		},
		{
			description: "should include whitelisted query with empty value in canonical url",
			url:         "https://ya.ru/abc?page=",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/abc?page=",
				canonicalUrl: "ya.ru/abc?page=",
			},
		},
		{
			description: "should remove non-whitelisted query in canonical url",
			url:         "https://ya.ru/abc?a=1&b=2",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/abc?a=1&b=2",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should include only whitelisted query in canonical url",
			url:         "https://ya.ru/abc?page=1&b=2",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/abc?page=1&b=2",
				canonicalUrl: "ya.ru/abc?page=1",
			},
		},
		{
			description: "should drop root path from canonical url if no query",
			url:         "https://ya.ru/",
			fetchUrl:    "https://ya.ru/",
			expected: &Expected{
				url:          "https://ya.ru/",
				canonicalUrl: "ya.ru",
			},
		},
		{
			description: "should keep root path in canonical url if query",
			url:         "https://ya.ru/?page",
			fetchUrl:    "https://ya.ru/",
			expected: &Expected{
				url:          "https://ya.ru/?page",
				canonicalUrl: "ya.ru/?page",
			},
		},
		{
			description: "should ignore newlines",
			url:         "https://ya.ru/ab\nc",
			fetchUrl:    "https://ya.ru/",
			expected: &Expected{
				url:          "https://ya.ru/abc",
				canonicalUrl: "ya.ru/abc",
			},
		},
		{
			description: "should trim leading and trailing spaces",
			url:         " https://ya.ru",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru",
				canonicalUrl: "ya.ru",
			},
		},
		{
			description: "should trim leading and trailing escaped spaces",
			url:         "%20https://waitbutwhy.com/table/like-improve-android-phone%20",
			fetchUrl:    "https://waitbutwhy.com/table/like-improve-iphone",
			expected: &Expected{
				url:          "https://waitbutwhy.com/table/like-improve-android-phone",
				canonicalUrl: "waitbutwhy.com/table/like-improve-android-phone",
			},
		},
		{
			description: "should trim leading and trailing escaped crazy whitespace",
			url:         " \t\n\x00\v\f\r%20%09%0a%00%0b%0c%0d%0A%0B%0C%0Dhttps://ya.ru \t\n\x00\v\f\r%20%09%0a%00%0b%0c%0d%0A%0B%0C%0D",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru",
				canonicalUrl: "ya.ru",
			},
		},
		{
			description: "should escape middle spaces",
			url:         "/tagged/alex norris",
			fetchUrl:    "https://webcomicname.com/post/652255218526011392/amp",
			expected: &Expected{
				url:          "https://webcomicname.com/tagged/alex%20norris",
				canonicalUrl: "webcomicname.com/tagged/alex%20norris",
			},
		},
		{
			description: "should replace // in path with /",
			url:         "https://NQNStudios.github.io//2020//04//06/byte-size-mindfulness-1.html",
			fetchUrl:    "https://www.natquaylenelson.com/feed.xml",
			expected: &Expected{
				url:          "https://NQNStudios.github.io/2020/04/06/byte-size-mindfulness-1.html",
				canonicalUrl: "NQNStudios.github.io/2020/04/06/byte-size-mindfulness-1.html",
			},
		},
		{
			description: "should leave + in query escaped",
			url:         "https://blog.gardeviance.org/search?updated-max=2019-09-04T15:49:00%2B01:00&max-results=12",
			fetchUrl:    "https://blog.gardeviance.org",
			expected: &Expected{
				url:          "https://blog.gardeviance.org/search?updated-max=2019-09-04T15:49:00%2B01:00&max-results=12",
				canonicalUrl: "blog.gardeviance.org/search?updated-max=2019-09-04T15:49:00%2B01:00",
			},
		},
		{
			description: "should use the port from fetch uri",
			url:         "/rss",
			fetchUrl:    "http://localhost:8000/page",
			expected: &Expected{
				url:          "http://localhost:8000/rss",
				canonicalUrl: "localhost:8000/rss",
			},
		},
		{
			description: "should ignore invalid character in host",
			url:         "http://targetWindow.postMessage(message, targetOrigin, [transfer]);",
			fetchUrl:    "https://thewitchofendor.com/2019/02/20/",
			expected:    nil,
		},
		{
			description: "should ignore invalid port number",
			url:         "http://localhost:${port}`",
			fetchUrl:    "https://medium.com/samsung-internet-dev/hello-deno-ed1f8961be26?source=post_internal_links---------2----------------------------",
			expected:    nil,
		},
		{
			description: "should ignore url with userinfo",
			url:         "http://npm install phaser@3.15.1",
			fetchUrl:    "https://thewitchofendor.com/2019/01/page/2/",
			expected:    nil,
		},
		{
			description: "should ignore url with opaque",
			url:         "http:mgd1981.wordpress.com/2012/06/11/truth-in-spectacles-and-speculation-in-tentacles/#NoSpoilers",
			fetchUrl:    "https://thefatalistmarksman.com/page/2/",
			expected:    nil,
		},
		{
			description: "should ignore url with invalid scheme format",
			url:         "(https://github.com/facebook/react/)",
			fetchUrl:    "https://dev.to/t/react",
			expected:    nil,
		},
		{
			description: "should ignore url with missing hierarchical segment",
			url:         "http:",
			fetchUrl:    "https://ai.googleblog.com/2017/11/",
			expected:    nil,
		},
		{
			description: "should ignore mailto url",
			url:         "mailto:aras_at_nesnausk_dot_org",
			fetchUrl:    "https://aras-p.info/toys/game-industry-rumor.php",
			expected:    nil,
		},
		{
			description: "should escape url",
			url:         "https://ya.ru/Россия",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F",
				canonicalUrl: "ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F",
			},
		},
		{
			description: "should preserve escaped url",
			url:         "https://ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F",
				canonicalUrl: "ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F",
			},
		},
		{
			description: "should handle half-escaped url",
			url:         "https://ya.ru/Рос%D1%81%D0%B8%D1%8F%25",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F%25",
				canonicalUrl: "ya.ru/%D0%A0%D0%BE%D1%81%D1%81%D0%B8%D1%8F%25",
			},
		},
		{
			description: "should preserve badly escaped url",
			url:         "https://ya.ru/%25D1%2581%25D0%25B8%25D1%258F",
			fetchUrl:    "https://ya.ru",
			expected: &Expected{
				url:          "https://ya.ru/%25D1%2581%25D0%25B8%25D1%258F",
				canonicalUrl: "ya.ru/%25D1%2581%25D0%25B8%25D1%258F",
			},
		},
		{
			description: "should ignore invalid uri with two userinfos",
			url:         "http://ex.p.lo.si.v.edhq.g@silvia.woodw.o.r.t.h@www.temposicilia.it/index.php/component/-/index.php?option=com_kide",
			fetchUrl:    "http://yosefk.com/blog/a-better-future-animated-post.html",
			expected:    nil,
		},
		{
			description: "should ignore url starting with :",
			url:         ":2",
			fetchUrl:    "https://blog.mozilla.org/en/mozilla/password-security-part-ii/",
			expected:    nil,
		},
	}

	logger := NewDummyLogger()
	curiEqCfg := &CanonicalEqualityConfig{
		SameHosts:         nil,
		ExpectTumblrPaths: false,
	}
	for _, tc := range tests {
		fetchUri, err := url.Parse(tc.fetchUrl)
		oops.RequireNoError(t, err)
		canonicalLink, ok := ToCanonicalLink(tc.url, logger, fetchUri)
		if tc.expected != nil {
			require.True(t, ok, tc.description)
			require.Equal(t, tc.expected.url, canonicalLink.Url, tc.description)
			expectedCuri := CanonicalUriFromDbString(tc.expected.canonicalUrl)
			require.True(t, CanonicalUriEqual(expectedCuri, canonicalLink.Curi, curiEqCfg), tc.description)
		} else {
			require.False(t, ok, tc.description)
		}
	}
}
