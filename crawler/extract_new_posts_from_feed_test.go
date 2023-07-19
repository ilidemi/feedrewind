package crawler

import (
	neturl "net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractNewPostsFromFeed(t *testing.T) {
	type test struct {
		description              string
		feed                     string
		existingPostUrls         []string
		discardedFeedEntryUrls   map[string]bool
		missingFromFeedEntryUrls map[string]bool
		expectedNewLinkUrls      []string
		expectedOk               bool
	}

	tests := []test{
		{
			description: "handle feed without updates",
			feed: `
				<rss><channel>
					<item><link>https://blog/post1</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post3</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post1", "https://blog/post2", "https://blog/post3",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls:      nil,
			expectedOk:               true,
		},
		{
			description: "handle feed with good updates",
			feed: `
				<rss><channel>
					<item><link>https://blog/post1</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post3</link></item>
					<item><link>https://blog/post4</link></item>
					<item><link>https://blog/post5</link></item>
					<item><link>https://blog/post6</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post4", "https://blog/post5", "https://blog/post6",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls: []string{
				"https://blog/post1", "https://blog/post2", "https://blog/post3",
			},
			expectedOk: true,
		},
		{
			description: "bail on feed with not enough overlap",
			feed: `
				<rss><channel>
					<item><link>https://blog/post1</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post3</link></item>
					<item><link>https://blog/post4</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post3", "https://blog/post4", "https://blog/post5",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls:      nil,
			expectedOk:               false,
		},
		{
			description: "bail on feed with drastic changes",
			feed: `
				<rss><channel>
					<item><link>https://blog/post4</link></item>
					<item><link>https://blog/post3</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post1</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post2", "https://blog/post3", "https://blog/post4", "https://blog/post5",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls:      nil,
			expectedOk:               false,
		},
		{
			description: "bail on shuffled feed with matching suffix",
			feed: `
				<rss><channel>
					<item><link>https://blog/post5</link></item>
					<item><link>https://blog/post1</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post3</link></item>
					<item><link>https://blog/post4</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post2", "https://blog/post3", "https://blog/post4", "https://blog/post5",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls:      nil,
			expectedOk:               false,
		},
		{
			description: "handle feed with duplicate dates for new posts",
			feed: `
				<rss><channel>
					<item>
						<link>https://blog/post1</link>
						<pubDate>Sun, 21 Oct 2015 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post2</link>
						<pubDate>Wed, 21 Oct 2015 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post3</link>
						<pubDate>Wed, 21 Oct 2014 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post4</link>
						<pubDate>Wed, 21 Oct 2013 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post5</link>
						<pubDate>Wed, 21 Oct 2012 08:28:48 GMT</pubDate>
					</item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post3", "https://blog/post4", "https://blog/post5",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls: []string{
				"https://blog/post1", "https://blog/post2",
			},
			expectedOk: true,
		},
		{
			description: "handle feed with duplicate dates for the oldest new post and the newest old post",
			feed: `
				<rss><channel>
					<item>
						<link>https://blog/post1</link>
						<pubDate>Sun, 21 Oct 2016 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post2</link>
						<pubDate>Wed, 21 Oct 2014 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post3</link>
						<pubDate>Wed, 21 Oct 2014 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post4</link>
						<pubDate>Wed, 21 Oct 2013 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post5</link>
						<pubDate>Wed, 21 Oct 2012 08:28:48 GMT</pubDate>
					</item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post3", "https://blog/post4", "https://blog/post5",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls: []string{
				"https://blog/post1", "https://blog/post2",
			},
			expectedOk: true,
		},
		{
			description: "handle feed with duplicate dates for the old posts",
			feed: `
				<rss><channel>
					<item>
						<link>https://blog/post1</link>
						<pubDate>Sun, 21 Oct 2016 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post2</link>
						<pubDate>Wed, 21 Oct 2015 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post3</link>
						<pubDate>Wed, 21 Oct 2014 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post4</link>
						<pubDate>Wed, 21 Oct 2014 08:28:48 GMT</pubDate>
					</item>
					<item>
						<link>https://blog/post5</link>
						<pubDate>Wed, 21 Oct 2012 08:28:48 GMT</pubDate>
					</item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post3", "https://blog/post4", "https://blog/post5",
			},
			discardedFeedEntryUrls:   nil,
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls: []string{
				"https://blog/post1", "https://blog/post2",
			},
			expectedOk: true,
		},
		{
			description: "handle discarded feed entry urls",
			feed: `
				<rss><channel>
					<item><link>https://blog/post1</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post3</link></item>
					<item><link>https://blog/post4</link></item>
					<item><link>https://blog/post5</link></item>
					<item><link>https://blog/post6</link></item>
					<item><link>https://blog/post7</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post4", "https://blog/post6", "https://blog/post7",
			},
			discardedFeedEntryUrls: map[string]bool{
				"https://blog/post5": true,
			},
			missingFromFeedEntryUrls: nil,
			expectedNewLinkUrls: []string{
				"https://blog/post1", "https://blog/post2", "https://blog/post3",
			},
			expectedOk: true,
		},
		{
			description: "handle missing feed entry urls",
			feed: `
				<rss><channel>
					<item><link>https://blog/post1</link></item>
					<item><link>https://blog/post2</link></item>
					<item><link>https://blog/post3</link></item>
					<item><link>https://blog/post4</link></item>
					<item><link>https://blog/post6</link></item>
					<item><link>https://blog/post7</link></item>
				</channel></rss>
			`,
			existingPostUrls: []string{
				"https://blog/post4", "https://blog/post5", "https://blog/post6", "https://blog/post7",
			},
			discardedFeedEntryUrls: nil,
			missingFromFeedEntryUrls: map[string]bool{
				"https://blog/post5": true,
			},
			expectedNewLinkUrls: []string{
				"https://blog/post1", "https://blog/post2", "https://blog/post3",
			},
			expectedOk: true,
		},
	}

	feedUri, _ := neturl.Parse("https://blog/feed")
	logger := &DummyLogger{}
	curiEqCfg := CanonicalEqualityConfig{
		SameHosts:         nil,
		ExpectTumblrPaths: false,
	}

	for _, tc := range tests {
		var existingPostCuris []CanonicalUri
		for _, url := range tc.existingPostUrls {
			link, ok := ToCanonicalLink(url, logger, nil)
			assert.True(t, ok, tc.description)
			existingPostCuris = append(existingPostCuris, link.Curi)
		}
		parsedFeed, err := ParseFeed(tc.feed, feedUri, logger)
		assert.NoError(t, err, tc.description)
		newLinks, ok := MustExtractNewPostsFromFeed(
			parsedFeed, feedUri, existingPostCuris, tc.discardedFeedEntryUrls, tc.missingFromFeedEntryUrls,
			curiEqCfg, logger, logger,
		)
		assert.Equal(t, tc.expectedOk, ok, tc.description)
		var newUrls []string
		for _, link := range newLinks {
			newUrls = append(newUrls, link.Url)
		}
		assert.Equal(t, tc.expectedNewLinkUrls, newUrls, tc.description)
	}
}
