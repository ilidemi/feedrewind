//go:build testing

package publish

import (
	"feedrewind/db"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitSubscription(t *testing.T) {
	type subscriptionDesc struct {
		CountByDay map[schedule.DayOfWeek]int
	}

	type test struct {
		Description               string
		Subscription              subscriptionDesc
		MaybeExistingSubscription *subscriptionDesc
		ShouldPublishRssPosts     bool
		ExpectedSubBody           string
		ExpectedUserBody          string
	}

	tests := []test{
		{
			Description: "init with 0 posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"fri": 1},
			},
			MaybeExistingSubscription: nil,
			ShouldPublishRssPosts:     true,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "init with schedule but 0 posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 1},
			},
			MaybeExistingSubscription: nil,
			ShouldPublishRssPosts:     false,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "init with some posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 2, "fri": 2},
			},
			MaybeExistingSubscription: nil,
			ShouldPublishRssPosts:     true,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "init with some posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 2, "fri": 2},
			},
			MaybeExistingSubscription: nil,
			ShouldPublishRssPosts:     true,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "init another with 0 posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"fri": 1},
			},
			MaybeExistingSubscription: &subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"wed": 1},
			},
			ShouldPublishRssPosts: true,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "init another with schedule but 0 posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 1},
			},
			MaybeExistingSubscription: &subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"wed": 1},
			},
			ShouldPublishRssPosts: false,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "init another with some posts",
			Subscription: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 2, "fri": 2},
			},
			MaybeExistingSubscription: &subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"wed": 1},
			},
			ShouldPublishRssPosts: true,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
	}

	conn, err := db.Pool.AcquireBackground() //nolint:gocritic,staticcheck
	oops.RequireNoError(t, err)
	defer conn.Release()

	timeFormat := "2006-01-02 15:04:05-07:00"
	wed := "2022-05-04 00:00:00+00:00"
	thu := "2022-05-05 00:00:00+00:00"

	for _, tc := range tests {
		user, err := createUser(conn)
		oops.RequireNoError(t, err, tc.Description)

		finishedSetupAt, err := schedule.ParseTime(timeFormat, thu)
		oops.RequireNoError(t, err, tc.Description)

		subscription, err := createSubscription(
			conn, user.Id, 1, finishedSetupAt, 5, 0, tc.Subscription.CountByDay,
		)
		oops.RequireNoError(t, err, tc.Description)

		if tc.MaybeExistingSubscription != nil {
			existingFinishedSetupAt, err := schedule.ParseTime(timeFormat, wed)
			oops.RequireNoError(t, err, tc.Description)

			_, err = createSubscription(
				conn, user.Id, 2, existingFinishedSetupAt, 5, 1, tc.MaybeExistingSubscription.CountByDay,
			)
			oops.RequireNoError(t, err, tc.Description)
		}

		utcNow := finishedSetupAt
		utcNowDate := utcNow.Date()
		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			return InitSubscription(
				tx, user.Id, user.ProductUserId, subscription.Id, subscription.Name, subscription.BlogBestUrl,
				user.DeliveryChannel, tc.ShouldPublishRssPosts, utcNow, utcNow, utcNowDate,
			)
		})
		oops.RequireNoError(t, err, tc.Description)

		subBody, err := models.SubscriptionRss_GetBody(conn, subscription.Id)
		oops.RequireNoError(t, err, tc.Description)
		require.Equal(t, tc.ExpectedSubBody, subBody, tc.Description)

		userBody, err := models.UserRss_GetBody(conn, user.Id)
		oops.RequireNoError(t, err, tc.Description)
		require.Equal(t, tc.ExpectedUserBody, userBody, tc.Description)

		err = cleanup(conn)
		oops.RequireNoError(t, err, tc.Description)
	}
}

func TestPublishForUser(t *testing.T) {
	type subscriptionDesc struct {
		CountByDay      map[schedule.DayOfWeek]int
		ExpectedRssBody string
	}

	type test struct {
		Description        string
		MaybeSubscription1 *subscriptionDesc
		Subscription2      subscriptionDesc
		ExpectedUserBody   string
	}

	tests := []test{
		{
			Description:        "update one",
			MaybeSubscription1: nil,
			Subscription2: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 2, "fri": 2},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 2 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/2</link>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/2_4/</link>
      <guid isPermaLink="false">fc56dbc6d4652b315b86b71c8d688c1ccdea9c5f1fd07763d2659fde2e2fc49a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/2_3/</link>
      <guid isPermaLink="false">4621c1d55fa4e86ce0dae4288302641baac86dd53f76227c892df9d300682d41</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/2_2/</link>
      <guid isPermaLink="false">c17edaae86e4016a583e098582f6dbf3eccade8ef83747df9ba617ded9d31309</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/2_4/</link>
      <guid isPermaLink="false">fc56dbc6d4652b315b86b71c8d688c1ccdea9c5f1fd07763d2659fde2e2fc49a</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/2_3/</link>
      <guid isPermaLink="false">4621c1d55fa4e86ce0dae4288302641baac86dd53f76227c892df9d300682d41</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/2_2/</link>
      <guid isPermaLink="false">c17edaae86e4016a583e098582f6dbf3eccade8ef83747df9ba617ded9d31309</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "update multiple at once",
			MaybeSubscription1: &subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"wed": 2, "fri": 2},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/1_4/</link>
      <guid isPermaLink="false">5ef6fdf32513aa7cd11f72beccf132b9224d33f271471fff402742887a171edf</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/1_3/</link>
      <guid isPermaLink="false">454f63ac30c8322997ef025edff6abd23e0dbe7b8a3d5126a894e4a168c1b59b</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			Subscription2: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 1, "fri": 1},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 2 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/2</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/2_2/</link>
      <guid isPermaLink="false">c17edaae86e4016a583e098582f6dbf3eccade8ef83747df9ba617ded9d31309</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/2_2/</link>
      <guid isPermaLink="false">c17edaae86e4016a583e098582f6dbf3eccade8ef83747df9ba617ded9d31309</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/1_4/</link>
      <guid isPermaLink="false">5ef6fdf32513aa7cd11f72beccf132b9224d33f271471fff402742887a171edf</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/1_3/</link>
      <guid isPermaLink="false">454f63ac30c8322997ef025edff6abd23e0dbe7b8a3d5126a894e4a168c1b59b</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "update some but not all",
			MaybeSubscription1: &subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"wed": 1, "fri": 1},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			Subscription2: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 1},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 2 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/2</link>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description: "update none",
			MaybeSubscription1: &subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"wed": 1},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			Subscription2: subscriptionDesc{
				CountByDay: map[schedule.DayOfWeek]int{"thu": 1},
				ExpectedRssBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 2 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/2</link>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
			},
			ExpectedUserBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
	}

	conn, err := db.Pool.AcquireBackground() //nolint:gocritic,staticcheck
	oops.RequireNoError(t, err)
	defer conn.Release()

	timeFormat := "2006-01-02 15:04:05-07:00"
	wed := "2022-05-04 00:00:00+00:00"
	thu := "2022-05-05 00:00:00+00:00"
	fri := "2022-05-06 00:00:00+00:00"

	for _, tc := range tests {
		user, err := createUser(conn)
		oops.RequireNoError(t, err, tc.Description)

		var Maybesubscription1 *testSubscription
		if tc.MaybeSubscription1 != nil {
			finishedSetupAt1, err := schedule.ParseTime(timeFormat, wed)
			oops.RequireNoError(t, err, tc.Description)

			Maybesubscription1, err = createSubscription(
				conn, user.Id, 1, finishedSetupAt1, 5, 0, tc.MaybeSubscription1.CountByDay,
			)
			oops.RequireNoError(t, err, tc.Description)

			finishedSetupAt1Date := finishedSetupAt1.Date()
			err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
				return InitSubscription(
					tx, user.Id, user.ProductUserId, Maybesubscription1.Id, Maybesubscription1.Name,
					Maybesubscription1.BlogBestUrl, user.DeliveryChannel, true, finishedSetupAt1, finishedSetupAt1,
					finishedSetupAt1Date,
				)
			})
			oops.RequireNoError(t, err, tc.Description)
		}

		finishedSetupAt2, err := schedule.ParseTime(timeFormat, thu)
		oops.RequireNoError(t, err, tc.Description)

		subscription2, err := createSubscription(
			conn, user.Id, 2, finishedSetupAt2, 5, 0, tc.Subscription2.CountByDay,
		)
		oops.RequireNoError(t, err, tc.Description)

		finishedSetupAt2Date := finishedSetupAt2.Date()
		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			return InitSubscription(
				tx, user.Id, user.ProductUserId, subscription2.Id, subscription2.Name,
				subscription2.BlogBestUrl, user.DeliveryChannel, true, finishedSetupAt2, finishedSetupAt2,
				finishedSetupAt2Date,
			)
		})
		oops.RequireNoError(t, err, tc.Description)

		utcNow, err := schedule.ParseTime(timeFormat, fri)
		oops.RequireNoError(t, err, tc.Description)

		utcNow = utcNow.UTC()
		utcNowDate := utcNow.Date()
		utcNowScheduledFor := utcNow.MustUTCString()

		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			return PublishForUser(
				tx, user.Id, user.ProductUserId, user.DeliveryChannel, utcNow, utcNow, utcNowDate,
				utcNowScheduledFor,
			)
		})
		oops.RequireNoError(t, err, tc.Description)

		if tc.MaybeSubscription1 != nil {
			sub1Body, err := models.SubscriptionRss_GetBody(conn, Maybesubscription1.Id)
			oops.RequireNoError(t, err, tc.Description)
			require.Equal(t, tc.MaybeSubscription1.ExpectedRssBody, sub1Body, tc.Description)
		}

		sub2Body, err := models.SubscriptionRss_GetBody(conn, subscription2.Id)
		oops.RequireNoError(t, err, tc.Description)
		require.Equal(t, tc.Subscription2.ExpectedRssBody, sub2Body, tc.Description)

		userBody, err := models.UserRss_GetBody(conn, user.Id)
		oops.RequireNoError(t, err, tc.Description)
		require.Equal(t, tc.ExpectedUserBody, userBody, tc.Description)

		err = cleanup(conn)
		oops.RequireNoError(t, err, tc.Description)
	}
}

func TestRssCountLimit(t *testing.T) {
	type test struct {
		Description     string
		TotalPosts      int
		PublishedPosts  int
		ExpectedSubBody string
	}

	tests := []test{
		{
			Description:    "evict welcome",
			TotalPosts:     6,
			PublishedPosts: 4,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Post 5</title>
      <link>https://feedrewind.com/posts/post-5/1_5/</link>
      <guid isPermaLink="false">1253e9373e781b7500266caa55150e08e210bc8cd8cc70d89985e3600155e860</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/1_4/</link>
      <guid isPermaLink="false">5ef6fdf32513aa7cd11f72beccf132b9224d33f271471fff402742887a171edf</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/1_3/</link>
      <guid isPermaLink="false">454f63ac30c8322997ef025edff6abd23e0dbe7b8a3d5126a894e4a168c1b59b</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description:    "finish with welcome",
			TotalPosts:     3,
			PublishedPosts: 2,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>You&#39;re all caught up with Test Subscription 1</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">3fe72a84a4c123fd67940ca3f338f28aa8de4991a1e444991f42aa7a1549e174</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/add&#34;&gt;Want to read something else?&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/1_3/</link>
      <guid isPermaLink="false">454f63ac30c8322997ef025edff6abd23e0dbe7b8a3d5126a894e4a168c1b59b</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description:    "finish without welcome",
			TotalPosts:     4,
			PublishedPosts: 3,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>You&#39;re all caught up with Test Subscription 1</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">3fe72a84a4c123fd67940ca3f338f28aa8de4991a1e444991f42aa7a1549e174</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/add&#34;&gt;Want to read something else?&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/1_4/</link>
      <guid isPermaLink="false">5ef6fdf32513aa7cd11f72beccf132b9224d33f271471fff402742887a171edf</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/1_3/</link>
      <guid isPermaLink="false">454f63ac30c8322997ef025edff6abd23e0dbe7b8a3d5126a894e4a168c1b59b</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
		{
			Description:    "finish without welcome and first post",
			TotalPosts:     5,
			PublishedPosts: 4,
			ExpectedSubBody: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>You&#39;re all caught up with Test Subscription 1</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">3fe72a84a4c123fd67940ca3f338f28aa8de4991a1e444991f42aa7a1549e174</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/add&#34;&gt;Want to read something else?&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 5</title>
      <link>https://feedrewind.com/posts/post-5/1_5/</link>
      <guid isPermaLink="false">1253e9373e781b7500266caa55150e08e210bc8cd8cc70d89985e3600155e860</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 4</title>
      <link>https://feedrewind.com/posts/post-4/1_4/</link>
      <guid isPermaLink="false">5ef6fdf32513aa7cd11f72beccf132b9224d33f271471fff402742887a171edf</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 3</title>
      <link>https://feedrewind.com/posts/post-3/1_3/</link>
      <guid isPermaLink="false">454f63ac30c8322997ef025edff6abd23e0dbe7b8a3d5126a894e4a168c1b59b</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`,
		},
	}

	conn, err := db.Pool.AcquireBackground() //nolint:gocritic,staticcheck
	oops.RequireNoError(t, err)
	defer conn.Release()

	timeFormat := "2006-01-02 15:04:05-07:00"
	thu := "2022-05-05 00:00:00+00:00"
	fri := "2022-05-06 00:00:00+00:00"

	for _, tc := range tests {
		user, err := createUser(conn)
		oops.RequireNoError(t, err, tc.Description)

		finishedSetupAt, err := schedule.ParseTime(timeFormat, thu)
		oops.RequireNoError(t, err, tc.Description)

		subscription, err := createSubscription(
			conn, user.Id, 1, finishedSetupAt, tc.TotalPosts, tc.PublishedPosts,
			map[schedule.DayOfWeek]int{"fri": 1},
		)
		oops.RequireNoError(t, err, tc.Description)

		finishedSetupAtDate := finishedSetupAt.Date()
		postsInRss := 5
		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			return initSubscriptionImpl(
				tx, user.Id, user.ProductUserId, subscription.Id, subscription.Name,
				subscription.BlogBestUrl, user.DeliveryChannel, true, finishedSetupAt, finishedSetupAt,
				finishedSetupAtDate, postsInRss,
			)
		})
		oops.RequireNoError(t, err, tc.Description)

		utcNow, err := schedule.ParseTime(timeFormat, fri)
		oops.RequireNoError(t, err, tc.Description)

		utcNow = utcNow.UTC()
		utcNowDate := utcNow.Date()
		utcNowScheduledFor := utcNow.MustUTCString()

		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			return publishForUserImpl(
				tx, user.Id, user.ProductUserId, user.DeliveryChannel, utcNow, utcNow, utcNowDate,
				utcNowScheduledFor, postsInRss,
			)
		})
		oops.RequireNoError(t, err, tc.Description)

		subBody, err := models.SubscriptionRss_GetBody(conn, subscription.Id)
		oops.RequireNoError(t, err, tc.Description)
		require.Equal(t, tc.ExpectedSubBody, subBody, tc.Description)

		err = cleanup(conn)
		oops.RequireNoError(t, err, tc.Description)
	}
}

func TestIsPausedHandling(t *testing.T) {
	conn, err := db.Pool.AcquireBackground() //nolint:gocritic,staticcheck
	oops.RequireNoError(t, err)
	defer conn.Release()

	timeFormat := "2006-01-02 15:04:05-07:00"
	thu := "2022-05-05 00:00:00+00:00"
	fri := "2022-05-06 00:00:00+00:00"

	user, err := createUser(conn)
	oops.RequireNoError(t, err)

	finishedSetupAt, err := schedule.ParseTime(timeFormat, thu)
	oops.RequireNoError(t, err)

	subscription, err := createSubscription(
		conn, user.Id, 1, finishedSetupAt, 5, 0, map[schedule.DayOfWeek]int{"fri": 1},
	)
	oops.RequireNoError(t, err)

	_, err = conn.Exec(`
        update subscriptions_without_discarded
        set is_paused = true where id = $1
    `, subscription.Id)
	oops.RequireNoError(t, err)

	finishedSetupAtDate := finishedSetupAt.Date()
	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		return InitSubscription(
			tx, user.Id, user.ProductUserId, subscription.Id, subscription.Name,
			subscription.BlogBestUrl, user.DeliveryChannel, true, finishedSetupAt, finishedSetupAt,
			finishedSetupAtDate,
		)
	})
	oops.RequireNoError(t, err)

	utcNow, err := schedule.ParseTime(timeFormat, fri)
	oops.RequireNoError(t, err)

	utcNow = utcNow.UTC()
	utcNowDate := utcNow.Date()
	utcNowScheduledFor := utcNow.MustUTCString()

	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		return PublishForUser(
			tx, user.Id, user.ProductUserId, user.DeliveryChannel, utcNow, utcNow, utcNowDate,
			utcNowScheduledFor,
		)
	})
	oops.RequireNoError(t, err)

	subBody, err := models.SubscriptionRss_GetBody(conn, subscription.Id)
	oops.RequireNoError(t, err)

	expectedSubBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Subscription 1 · FeedRewind</title>
    <link>https://feedrewind.com/subscriptions/1</link>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`
	require.Equal(t, expectedSubBody, subBody)

	err = cleanup(conn)
	oops.RequireNoError(t, err)
}

func TestUserFeedStableSort(t *testing.T) {
	conn, err := db.Pool.AcquireBackground() //nolint:gocritic,staticcheck
	oops.RequireNoError(t, err)
	defer conn.Release()

	timeFormat := "2006-01-02 15:04:05-07:00"
	tue := "2022-05-03 00:00:00+00:00"
	wed := "2022-05-04 00:00:00+00:00"
	thu := "2022-05-05 00:00:00+00:00"
	fri := "2022-05-06 00:00:00+00:00"

	user, err := createUser(conn)
	oops.RequireNoError(t, err)

	finishedSetupAt1, err := schedule.ParseTime(timeFormat, tue)
	oops.RequireNoError(t, err)

	_, err = createSubscription(
		conn, user.Id, 1, finishedSetupAt1, 5, 0, map[schedule.DayOfWeek]int{"fri": 2},
	)
	oops.RequireNoError(t, err)

	finishedSetupAt2, err := schedule.ParseTime(timeFormat, wed)
	oops.RequireNoError(t, err)

	_, err = createSubscription(
		conn, user.Id, 2, finishedSetupAt2, 5, 0, map[schedule.DayOfWeek]int{"fri": 1},
	)
	oops.RequireNoError(t, err)

	finishedSetupAt3, err := schedule.ParseTime(timeFormat, thu)
	oops.RequireNoError(t, err)

	subscription3, err := createSubscription(
		conn, user.Id, 3, finishedSetupAt3, 1, 1, map[schedule.DayOfWeek]int{"sat": 1},
	)
	oops.RequireNoError(t, err)

	finishedSetupAt3Date := finishedSetupAt3.Date()
	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		return InitSubscription(
			tx, user.Id, user.ProductUserId, subscription3.Id, subscription3.Name,
			subscription3.BlogBestUrl, user.DeliveryChannel, true, finishedSetupAt3, finishedSetupAt3,
			finishedSetupAt3Date,
		)
	})
	oops.RequireNoError(t, err)

	utcNow, err := schedule.ParseTime(timeFormat, fri)
	oops.RequireNoError(t, err)

	utcNow = utcNow.UTC()
	utcNowDate := utcNow.Date()
	utcNowScheduledFor := utcNow.MustUTCString()

	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		return PublishForUser(
			tx, user.Id, user.ProductUserId, user.DeliveryChannel, utcNow, utcNow, utcNowDate,
			utcNowScheduledFor,
		)
	})
	oops.RequireNoError(t, err)

	userBody, err := models.UserRss_GetBody(conn, user.Id)
	oops.RequireNoError(t, err)

	expectedUserBody := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>FeedRewind</title>
    <link>https://feedrewind.com</link>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/2_1/</link>
      <guid isPermaLink="false">43974ed74066b207c30ffd0fed5146762e6c60745ac977004bc14507c7c42b50</guid>
      <description>from Test Subscription 2&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 2</title>
      <link>https://feedrewind.com/posts/post-2/1_2/</link>
      <guid isPermaLink="false">37834f2f25762f23e1f74a531cbe445db73d6765ebe60878a7dfbecd7d4af6e1</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/1_1/</link>
      <guid isPermaLink="false">16dc368a89b428b2485484313ba67a3912ca03f2b2b42429174a4f8b3dc84e44</guid>
      <description>from Test Subscription 1&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Fri, 06 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>You&#39;re all caught up with Test Subscription 3</title>
      <link>https://feedrewind.com/subscriptions/3</link>
      <guid isPermaLink="false">43b8e4fb7c0526d3ef514cac8554894843f36a7c0b3a5e3439f024fd5771cfd1</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/add&#34;&gt;Want to read something else?&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Post 1</title>
      <link>https://feedrewind.com/posts/post-1/3_1/</link>
      <guid isPermaLink="false">c3ea99f86b2f8a74ef4145bb245155ff5f91cd856f287523481c15a1959d5fd1</guid>
      <description>from Test Subscription 3&lt;br&gt;&lt;br&gt;&lt;a href=&#34;https://feedrewind.com/subscriptions/3&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 3 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/3</link>
      <guid isPermaLink="false">6b8620fd9d02c36e8581ecd6e56fe54122f2c7f58f3a8bc94b41551ee82f1693</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/3&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Thu, 05 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 2 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/2</link>
      <guid isPermaLink="false">ebd09a71ff012c43b03f497b6551b9b41fe889ecc73aeceb2ab6c002bfbb6a91</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/2&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Wed, 04 May 2022 00:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Subscription 1 added to FeedRewind</title>
      <link>https://feedrewind.com/subscriptions/1</link>
      <guid isPermaLink="false">02d00b67b9732798e803e344a5e57d80e3f7a620991f9cd5f2256ff8644de37a</guid>
      <description>&lt;a href=&#34;https://feedrewind.com/subscriptions/1&#34;&gt;Manage&lt;/a&gt;</description>
      <pubDate>Tue, 03 May 2022 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`
	require.Equal(t, expectedUserBody, userBody)

	err = cleanup(conn)
	oops.RequireNoError(t, err)
}

type testUser struct {
	Id              models.UserId
	ProductUserId   models.ProductUserId
	DeliveryChannel models.DeliveryChannel
}

func createUser(tx pgw.Queryable) (*testUser, error) {
	if err := ensureTestDb(tx); err != nil {
		return nil, err
	}

	if err := db.EnsureLatestMigration(); err != nil {
		return nil, err
	}

	if err := cleanup(tx); err != nil {
		return nil, err
	}

	userId := models.UserId(0)
	productUserId := models.ProductUserId("00000000-0000-0000-0000-000000000000")
	_, err := tx.Exec(`
        insert into users_without_discarded (id, email, name, password_digest, auth_token, product_user_id)
        values ($1, 'test@feedrewind.com', 'test', 'asdf', 'asdf', $2)
    `, userId, productUserId)
	if err != nil {
		return nil, err
	}

	deliveryChannel := models.DeliveryChannelMultipleFeeds
	_, err = tx.Exec(`
        insert into user_settings (user_id, timezone, version, delivery_channel)
        values (0, 'America/Los_Angeles', 1, $1)
    `, deliveryChannel)
	if err != nil {
		return nil, err
	}

	return &testUser{
		Id:              userId,
		ProductUserId:   productUserId,
		DeliveryChannel: deliveryChannel,
	}, nil
}

type testSubscription struct {
	Id          models.SubscriptionId
	Name        string
	BlogBestUrl string
}

func createSubscription(
	tx pgw.Queryable, userId models.UserId, id int64, finishedSetupAt schedule.Time, totalCount int,
	publishedCount int, countsByDay map[schedule.DayOfWeek]int,
) (*testSubscription, error) {
	if err := ensureTestDb(tx); err != nil {
		return nil, err
	}

	if err := db.EnsureLatestMigration(); err != nil {
		return nil, err
	}

	blogName := fmt.Sprintf("Test blog %d", id)
	feedUrl := fmt.Sprintf("https://blog%d/feed.xml", id)
	_, err := tx.Exec(`
        insert into blogs (id, name, feed_url, status, status_updated_at, version, update_action)
        values ($1, $2, $3, $4, $5, $6, $7)
    `, id, blogName, feedUrl, models.BlogStatusCrawledConfirmed, finishedSetupAt, models.BlogLatestVersion,
		models.BlogUpdateActionRecrawl)
	if err != nil {
		return nil, err
	}

	subscriptionName := fmt.Sprintf("Test Subscription %d", id)
	_, err = tx.Exec(`
        insert into subscriptions_without_discarded (
            id, user_id, blog_id, name, status, is_paused, is_added_past_midnight, schedule_version,
            finished_setup_at
        )
        values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, id, userId, id, subscriptionName, models.SubscriptionStatusLive, false, false, 1, finishedSetupAt)
	if err != nil {
		return nil, err
	}

	for _, dayOfWeek := range schedule.DaysOfWeek {
		_, err := tx.Exec(`
            insert into schedules (subscription_id, day_of_week, count)
            values ($1, $2, $3)
        `, id, dayOfWeek, countsByDay[dayOfWeek])
		if err != nil {
			return nil, err
		}
	}

	for i := 1; i <= totalCount; i++ {
		postId := id*100 + int64(i)
		postUrl := fmt.Sprintf("https://blog%d/%d", id, i)
		postTitle := fmt.Sprintf("Post %d", i)
		_, err := tx.Exec(`
            insert into blog_posts (id, blog_id, index, url, title)
            values ($1, $2, $3, $4, $5)
        `, postId, id, i, postUrl, postTitle)
		if err != nil {
			return nil, err
		}
		randomId := fmt.Sprintf("%d_%d", id, i)
		var publishedAt *schedule.Time
		if i <= publishedCount {
			publishedAt = &finishedSetupAt
		}
		_, err = tx.Exec(`
            insert into subscription_posts (id, blog_post_id, subscription_id, random_id, published_at)
            values ($1, $2, $3, $4, $5)
        `, postId, postId, id, randomId, publishedAt)
		if err != nil {
			return nil, err
		}
	}

	return &testSubscription{
		Id:          models.SubscriptionId(id),
		Name:        subscriptionName,
		BlogBestUrl: feedUrl,
	}, nil
}

func cleanup(tx pgw.Queryable) error {
	if err := ensureTestDb(tx); err != nil {
		return err
	}

	tables := []string{
		"subscription_posts", "blog_posts", "schedules", "subscriptions_without_discarded", "blogs",
		"user_settings", "users_with_discarded",
	}
	for _, table := range tables {
		_, err := tx.Exec(`delete from ` + table)
		if err != nil {
			return err
		}
	}

	return nil
}

func ensureTestDb(tx pgw.Queryable) error {
	row := tx.QueryRow(`select current_database()`)
	var dbName string
	err := row.Scan(&dbName)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(dbName, "_test") {
		return oops.Newf("Running outside of test db: %s", dbName)
	}

	return nil
}
