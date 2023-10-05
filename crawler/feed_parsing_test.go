package crawler

import (
	"feedrewind/oops"
	neturl "net/url"
	"strings"
	"testing"

	"github.com/antchfx/xmlquery"
	"github.com/stretchr/testify/require"
)

func TestIsRSS(t *testing.T) {
	feed := `<rss version='2.0'>
		<channel>
			<title>Juho Snellman's Weblog</title>
			<link>https://www.snellman.net/blog/</link>
			<description>Lisp, Perl Golf</description>
		</channel>
	</rss>`

	reader := strings.NewReader(feed)
	xml, err := xmlquery.Parse(reader)
	oops.RequireNoError(t, err)
	require.True(t, isRSS(xml))
}

func TestIsRDF(t *testing.T) {
	feed := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
		xmlns:dc="http://purl.org/dc/elements/1.1/"
		xmlns:sy="http://purl.org/rss/1.0/modules/syndication/"
		xmlns:content="http://purl.org/rss/1.0/modules/content/"
		xmlns:foaf="http://xmlns.com/foaf/0.1/"
		xmlns:cc="http://web.resource.org/cc/"
		xmlns="http://purl.org/rss/1.0/">
		<channel rdf:about="http://www.aaronsw.com/weblog/index.xml">
			<title>Raw Thought (from Aaron Swartz)</title>
			<link>http://www.aaronsw.com/weblog/</link>
			<description>"capture what you experience and sort it out; only in this way can you hope to use it to guide and test your reflection, and in the process shape yourself as an intellectual craftsman" -- C. Wright Mills</description>
			<dc:language>en-us</dc:language>	
			<dc:creator>Aaron Swartz</dc:creator> 
		</channel>
	</rdf:RDF>`

	reader := strings.NewReader(feed)
	xml, err := xmlquery.Parse(reader)
	oops.RequireNoError(t, err)
	require.True(t, isRDF(xml))
}

func TestIsAtom(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
	<feed xmlns="http://www.w3.org/2005/Atom" xml:base="https://lab.whitequark.org/">
		<id>https://lab.whitequark.org/</id>
		<title>whitequark's lab notebook</title>
		<updated>2020-04-06T17:25:03Z</updated>
		<link rel="alternate" href="https://lab.whitequark.org/" type="text/html"/>
		<link rel="self" href="https://lab.whitequark.org/atom.xml" type="application/atom+xml"/>
		<author>
		<name>whitequark</name>
		<uri>https://lab.whitequark.org/</uri>
		</author>
	</feed>`

	reader := strings.NewReader(feed)
	xml, err := xmlquery.Parse(reader)
	oops.RequireNoError(t, err)
	require.True(t, isAtom(xml))
}

func TestParseFeedRootUrl(t *testing.T) {
	type Test struct {
		description     string
		content         string
		expectedRootUrl string
	}

	tests := []Test{
		{
			description: "RSS root url",
			content: `
				<rss>
					<channel>
						<link>https://root</link>
					</channel>
				</rss>
			`,
			expectedRootUrl: "https://root",
		},
		{
			description: "RSS root url is not present",
			content: `
				<rss>
					<channel>
					</channel>
				</rss>
			`,
			expectedRootUrl: "",
		},
		{
			description: "RDF root url",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<link>https://root</link>
					</channel>
				</rdf:RDF>
			`,
			expectedRootUrl: "https://root",
		},
		{
			description: "RDF root url is not present",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
					</channel>
				</rdf:RDF>
			`,
			expectedRootUrl: "",
		},
		{
			description: "Atom root url",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<link rel="alternate" href="https://root"/>
				</feed>
			`,
			expectedRootUrl: "https://root",
		},
		{
			description: "Atom root url without rel",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<link href="https://root"/>
				</feed>
			`,
			expectedRootUrl: "https://root",
		},
		{
			description: "Atom root url is not present",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<link/>
				</feed>
			`,
			expectedRootUrl: "",
		},
		{
			description: "Atom root link is not present",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
				</feed>
			`,
			expectedRootUrl: "",
		},
	}

	logger := &DummyLogger{}
	fetchUri, err := neturl.Parse("https://root/feed")
	oops.RequireNoError(t, err)
	for _, tc := range tests {
		var parsedFeed *ParsedFeed
		var err error
		require.NotPanics(t, func() {
			parsedFeed, err = ParseFeed(tc.content, fetchUri, logger)
		}, tc.description)
		oops.RequireNoError(t, err, tc.description)
		if tc.expectedRootUrl == "" {
			require.Nil(t, parsedFeed.RootLink)
		} else {
			require.Equal(t, parsedFeed.RootLink.Url, tc.expectedRootUrl)
		}
	}
}

func TestParseFeedTitle(t *testing.T) {
	type Test struct {
		description   string
		content       string
		expectedTitle string
	}

	tests := []Test{
		{
			description: "RSS title",
			content: `
				<rss>
					<channel>
						<title>Title</title>
					</channel>
				</rss>
			`,
			expectedTitle: "Title",
		},
		{
			description: "RSS title trimmed",
			content: `
				<rss>
					<channel>
						<title>  Title  </title>
					</channel>
				</rss>
			`,
			expectedTitle: "Title",
		},
		{
			description: "RSS title is not present",
			content: `
				<rss>
					<channel>
					</channel>
				</rss>
			`,
			expectedTitle: "root", // from host
		},
		{
			description: "RDF title",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<title>Title</title>
					</channel>
				</rdf:RDF>
			`,
			expectedTitle: "Title",
		},
		{
			description: "RDF title trimmed",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<title>  Title  </title>
					</channel>
				</rdf:RDF>
			`,
			expectedTitle: "Title",
		},
		{
			description: "RDF title is not present",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
					</channel>
				</rdf:RDF>
			`,
			expectedTitle: "root", // from host
		},
		{
			description: "Atom title",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<title>Title</title>
				</feed>
			`,
			expectedTitle: "Title",
		},
		{
			description: "Atom title trimmed",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<title>  Title  </title>
				</feed>
			`,
			expectedTitle: "Title",
		},
		{
			description: "Atom title is not present",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
				</feed>
			`,
			expectedTitle: "root", // from host
		},
		{
			description: "Title HTML unescaped",
			content: `
				<rss>
					<channel>
						<title>&gt;Title&lt;</title>
					</channel>
				</rss>
			`,
			expectedTitle: ">Title<",
		},
		{
			description: "Title normalized",
			content: `
				<rss>
					<channel>
						<title>Title` + "\r\n\r\n" + `Title  Title</title>
					</channel>
				</rss>
			`,
			expectedTitle: "Title Title Title",
		},
	}

	logger := &DummyLogger{}
	fetchUri, err := neturl.Parse("https://root/feed")
	oops.RequireNoError(t, err)
	for _, tc := range tests {
		var parsedFeed *ParsedFeed
		var err error
		require.NotPanics(t, func() {
			parsedFeed, err = ParseFeed(tc.content, fetchUri, logger)
		}, tc.description)
		oops.RequireNoError(t, err, tc.description)
		require.Equal(t, parsedFeed.Title, tc.expectedTitle)
	}
}

func TestParseFeedEntryUrls(t *testing.T) {
	type Test struct {
		description          string
		content              string
		expectedEntryUrls    []string
		expectedNotEntryUrls []string
		expectedError        string
	}

	tests := []Test{
		{
			description: "parse RSS feed",
			content: `
				<rss>
					<channel>
						<item>
							<link>https://root/a</link>
						</item>
						<item>
							<link>https://root/b</link>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "parse urls from RSS guid permalinks",
			content: `
				<rss>
					<channel>
						<item>
							<guid isPermaLink="true">https://root/a</guid>
						</item>
						<item>
							<guid isPermaLink="true">https://root/b</guid>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "fail RSS parsing if item url is not present",
			content: `
				<rss>
					<channel>
						<item>
							<link>https://root/a</link>
						</item>
						<item>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    nil,
			expectedNotEntryUrls: nil,
			expectedError:        "couldn't extract item urls from RSS",
		},
		{
			description: "reverse RSS items if they are chronological",
			content: `
				<rss>
					<channel>
						<item>
							<pubDate>Wed, 21 Oct 2015 08:28:48 GMT</pubDate>
							<link>https://root/a</link>
						</item>
						<item>
							<pubDate>Sun, 25 Oct 2015 05:04:05 GMT</pubDate>
							<link>https://root/b</link>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RSS items if dates are shuffled and unique",
			content: `
				<rss>
					<channel>
						<item>
							<pubDate>Wed, 21 Oct 2015 08:28:48 GMT</pubDate>
							<link>https://root/a</link>
						</item>
						<item>
							<pubDate>Sun, 25 Oct 2015 05:04:05 GMT</pubDate>
							<link>https://root/b</link>
						</item>
						<item>
							<pubDate>Sun, 20 Oct 2015 05:04:05 GMT</pubDate>
							<link>https://root/c</link>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a", "root/c"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RSS items if dates are shuffled and repeating",
			content: `
				<rss>
					<channel>
					<item>
						<pubDate>Wed, 20 Oct 2015 05:04:05 GMT</pubDate>
						<link>https://root/a</link>
					</item>
					<item>
						<pubDate>Sun, 25 Oct 2015 05:04:05 GMT</pubDate>
						<link>https://root/b</link>
					</item>
					<item>
						<pubDate>Sun, 20 Oct 2015 05:04:05 GMT</pubDate>
						<link>https://root/c</link>
					</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a", "root/c"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RSS items if dates are shuffled and repeating (another order)",
			content: `
				<rss>
					<channel>
					<item>
						<pubDate>Wed, 20 Oct 2015 05:04:05 GMT</pubDate>
						<link>https://root/a</link>
					</item>
					<item>
						<pubDate>Sun, 25 Oct 2015 05:04:05 GMT</pubDate>
						<link>https://root/b</link>
					</item>
					<item>
						<pubDate>Sun, 20 Oct 2015 05:04:05 GMT</pubDate>
						<link>https://root/c</link>
					</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/b", "root/c", "root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RSS items by time within date",
			content: `
				<rss>
					<channel>
						<item>
							<pubDate>Wed, 20 Oct 2015 05:04:06 GMT</pubDate>
							<link>https://root/a</link>
						</item>
						<item>
							<pubDate>Sun, 20 Oct 2015 05:04:05 GMT</pubDate>
							<link>https://root/b</link>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: []string{"root/b", "root/a"},
			expectedError:        "",
		},
		{
			description: "parse RSS if a date is invalid",
			content: `
				<rss>
					<channel>
						<item>
							<pubDate>asdf</pubDate>
							<link>https://root/a</link>
						</item>
					</channel>
				</rss>
			`,
			expectedEntryUrls:    []string{"root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "parse RDF feed",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<item>
							<link>https://root/a</link>
						</item>
						<item>
							<link>https://root/b</link>
						</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "fail RDF parsing if item url is not present",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<item>
							<link>https://root/a</link>
						</item>
						<item>
						</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    nil,
			expectedNotEntryUrls: nil,
			expectedError:        "couldn't extract item urls from RDF",
		},
		{
			description: "reverse RDF items if they are chronological",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<item>
							<dc:date>2012-10-08T22:35:14-00:00</dc:date>
							<link>https://root/a</link>
						</item>
						<item>
							<dc:date>2012-11-01T13:15:49-00:00</dc:date>
							<link>https://root/b</link>
						</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RDF items if dates are shuffled and unique",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<item>
							<dc:date>2012-10-08T22:35:14-00:00</dc:date>
							<link>https://root/a</link>
						</item>
						<item>
							<dc:date>2012-11-01T13:15:49-00:00</dc:date>
							<link>https://root/b</link>
						</item>
						<item>
							<dc:date>2012-09-25T14:21:42-00:00</dc:date>
							<link>https://root/c</link>
						</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a", "root/c"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RDF items if dates are shuffled and repeating",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
					<item>
						<dc:date>2012-10-08T22:35:14-00:00</dc:date>
						<link>https://root/a</link>
					</item>
					<item>
						<dc:date>2012-11-01T13:15:49-00:00</dc:date>
						<link>https://root/b</link>
					</item>
					<item>
						<dc:date>2012-10-08T22:35:14-00:00</dc:date>
						<link>https://root/c</link>
					</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a", "root/c"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RDF items if dates are shuffled and repeating (another order)",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
					<item>
						<dc:date>2012-10-08T22:35:14-00:00</dc:date>
						<link>https://root/a</link>
					</item>
					<item>
						<dc:date>2012-11-01T13:15:49-00:00</dc:date>
						<link>https://root/b</link>
					</item>
					<item>
						<dc:date>2012-10-08T22:35:14-00:00</dc:date>
						<link>https://root/c</link>
					</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/b", "root/c", "root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort RDF items by time within date",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<item>
							<dc:date>2012-10-08T22:35:15-00:00</dc:date>
							<link>https://root/a</link>
						</item>
						<item>
							<dc:date>2012-10-08T22:35:14-00:00</dc:date>
							<link>https://root/b</link>
						</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: []string{"root/b", "root/a"},
			expectedError:        "",
		},
		{
			description: "parse RDF if a date is invalid",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
						<item>
							<pubDate>asdf</pubDate>
							<link>https://root/a</link>
						</item>
					</channel>
				</rdf:RDF>
			`,
			expectedEntryUrls:    []string{"root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "parse Atom feeds",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<link rel="alternate" href="https://root/a"/>
					</entry>
					<entry>
						<link rel="alternate" href="https://root/b"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "parse Atom links without rel",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<link href="https://root/b"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "fail Atom parsing if item url is not present",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<link/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    nil,
			expectedNotEntryUrls: nil,
			expectedError:        "couldn't extract entry urls from Atom",
		},
		{
			description: "fail Atom parsing if item link is not present",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<link href="https://root/a"/>
					</entry>
					<entry>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    nil,
			expectedNotEntryUrls: nil,
			expectedError:        "couldn't extract entry urls from Atom",
		},
		{
			description: "reverse Atom items if they are chronological",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<published>2020-05-16T00:00:00-04:00</published>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<published>2021-05-16T00:00:00-04:00</published>
						<link href="https://root/b"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort Atom items if dates are shuffled and unique",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<published>2020-05-16T00:00:00-04:00</published>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<published>2021-05-16T00:00:00-04:00</published>
						<link href="https://root/b"/>
					</entry>
					<entry>
						<published>2019-05-16T00:00:00-04:00</published>
						<link href="https://root/c"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a", "root/c"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort Atom items if dates are shuffled and repeating",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<published>2019-05-16T00:00:00-04:00</published>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<published>2021-05-16T00:00:00-04:00</published>
						<link href="https://root/b"/>
					</entry>
					<entry>
						<published>2019-05-16T00:00:00-04:00</published>
						<link href="https://root/c"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/b", "root/a", "root/c"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort Atom items if dates are shuffled and repeating (another order)",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<published>2019-05-16T00:00:00-04:00</published>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<published>2021-05-16T00:00:00-04:00</published>
						<link href="https://root/b"/>
					</entry>
					<entry>
						<published>2019-05-16T00:00:00-04:00</published>
						<link href="https://root/c"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/b", "root/c", "root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "sort Atom items by times within date",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<published>2019-05-16T00:00:01-04:00</published>
						<link href="https://root/a"/>
					</entry>
					<entry>
						<published>2019-05-16T00:00:00-04:00</published>
						<link href="https://root/b"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: []string{"root/b", "root/a"},
			expectedError:        "",
		},
		{
			description: "parse Atom if a date is invalid",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<entry>
						<published>asdf</published>
						<link href="https://root/a"/>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/a"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
		{
			description: "prioritize feedburner orig links in Atom",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom" xmlns:feedburner="http://rssnamespace.org/feedburner/ext/1.0">
					<entry>
						<link rel="alternate" href="https://feedburner/a"/>
						<feedburner:origLink>https://root/a</feedburner:origLink>
					</entry>
					<entry>
						<link rel="alternate" href="https://feedburner/b"/>
						<feedburner:origLink>https://root/b</feedburner:origLink>
					</entry>
				</feed>
			`,
			expectedEntryUrls:    []string{"root/a", "root/b"},
			expectedNotEntryUrls: nil,
			expectedError:        "",
		},
	}

	logger := &DummyLogger{}
	fetchUri, err := neturl.Parse("https://root/feed")
	oops.RequireNoError(t, err)
	for _, tc := range tests {
		var parsedFeed *ParsedFeed
		var err error
		require.NotPanics(t, func() {
			parsedFeed, err = ParseFeed(tc.content, fetchUri, logger)
		}, tc.description)
		if tc.expectedError != "" {
			require.ErrorContains(t, err, tc.expectedError, tc.description)
			require.Nil(t, tc.expectedEntryUrls, tc.description)
			require.Nil(t, tc.expectedNotEntryUrls, tc.description)
		} else {
			oops.RequireNoError(t, err, tc.description)

			curiEqCfg := &CanonicalEqualityConfig{
				SameHosts:         nil,
				ExpectTumblrPaths: false,
			}
			var entryCuris []CanonicalUri
			for _, url := range tc.expectedEntryUrls {
				entryCuris = append(entryCuris, CanonicalUriFromDbString(url))
			}
			matchedLinks := parsedFeed.EntryLinks.sequenceMatch(entryCuris, curiEqCfg)
			require.Equal(t, len(tc.expectedEntryUrls), len(matchedLinks), tc.description)

			if tc.expectedNotEntryUrls != nil {
				var notEntryCuris []CanonicalUri
				for _, url := range tc.expectedNotEntryUrls {
					notEntryCuris = append(notEntryCuris, CanonicalUriFromDbString(url))
				}
				notMatchedLinks := parsedFeed.EntryLinks.sequenceMatch(notEntryCuris, curiEqCfg)
				require.Zero(t, len(notMatchedLinks), tc.description)
			}
		}
	}
}

func TestParseFeedGenerator(t *testing.T) {
	type Test struct {
		description       string
		content           string
		expectedGenerator FeedGenerator
	}

	tests := []Test{
		{
			description: "handle no RSS generator",
			content: `
				<rss>
					<channel>
					</channel>
				</rss>
			`,
			expectedGenerator: FeedGeneratorOther,
		},
		{
			description: "recognize Tumblr RSS generator",
			content: `
				<rss>
					<channel>
						<generator>Tumblr (3.0; @webcomicname)</generator>
					</channel>
				</rss>
			`,
			expectedGenerator: FeedGeneratorTumblr,
		},
		{
			description: "recognize Blogger RSS generator",
			content: `
				<rss>
					<channel>
						<generator>Blogger</generator>
					</channel>
				</rss>
			`,
			expectedGenerator: FeedGeneratorBlogger,
		},
		{
			description: "recognize Medium RSS generator",
			content: `
				<rss>
					<channel>
						<generator>Medium</generator>
					</channel>
				</rss>
			`,
			expectedGenerator: FeedGeneratorMedium,
		},
		{
			description: "handle random RSS generator",
			content: `
				<rss>
					<channel>
						<generator>Who Dis</generator>
					</channel>
				</rss>
			`,
			expectedGenerator: FeedGeneratorOther,
		},
		{
			description: "Handle RDF no generator",
			content: `
				<rdf:RDF
					xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
					xmlns:dc="http://purl.org/dc/elements/1.1/"
					xmlns="http://purl.org/rss/1.0/"
				>
					<channel>
					</channel>
				</rdf:RDF>
			`,
			expectedGenerator: FeedGeneratorOther,
		},
		{
			description: "Handle Atom no generator",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
				</feed>
			`,
			expectedGenerator: FeedGeneratorOther,
		},
		{
			description: "Recognize Blogger Atom no generator",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<generator version='7.00' uri='http://www.blogger.com'>Blogger</generator>
				</feed>
			`,
			expectedGenerator: FeedGeneratorBlogger,
		},
		{
			description: "Handle random Atom generator",
			content: `
				<feed xmlns="http://www.w3.org/2005/Atom">
					<generator>Who Dis</generator>
				</feed>
			`,
			expectedGenerator: FeedGeneratorOther,
		},
	}

	logger := &DummyLogger{}
	fetchUri, err := neturl.Parse("https://root/feed")
	oops.RequireNoError(t, err)
	for _, tc := range tests {
		var parsedFeed *ParsedFeed
		var err error
		require.NotPanics(t, func() {
			parsedFeed, err = ParseFeed(tc.content, fetchUri, logger)
		}, tc.description)
		oops.RequireNoError(t, err, tc.description)
		require.Equal(t, parsedFeed.Generator, tc.expectedGenerator)
	}
}
