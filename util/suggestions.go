package util

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"
)

// When adding more lists besides ScreenshotLinks, SuggestedCategories and MiscellaneousBlogs,
// also add them to RefreshSuggestionsJob

var SuggestionFeedUrls = map[string]bool{}

type ScreenshotLink struct {
	Url         string
	TitleStr    string
	Title       template.HTML
	TitleMobile template.HTML
	Slug        string
	PreviewPath string
	BlogName    string
	FeedUrl     string
	IsEarliest  bool
	IsNewest    bool
}

//nolint:exhaustruct
var ScreenshotLinks = []ScreenshotLink{
	{
		Url:      "https://www.kalzumeus.com/2012/01/23/salary-negotiation/",
		TitleStr: "Salary Negotiation: Make More<br>Money, Be More Valued",
		Slug:     "kalzumeus-salary-negotiation",
		BlogName: "Kalzumeus Software",
		FeedUrl:  "https://www.kalzumeus.com/feed/articles/",
	},
	{
		Url:      "https://waitbutwhy.com/2014/05/life-weeks.html",
		TitleStr: "Your Life in Weeks",
		Slug:     "wbw-your-life-in-weeks",
		BlogName: "Wait But Why",
		FeedUrl:  "https://waitbutwhy.com/feed",
	},
	{
		Url:      "https://slatestarcodex.com/2014/07/30/meditations-on-moloch/",
		TitleStr: "Meditations On Moloch",
		Slug:     "ssc-meditations-on-moloch",
		BlogName: "Slate Star Codex",
		FeedUrl:  "https://slatestarcodex.com/feed/",
	},
	{
		Url:      "https://www.inkandswitch.com/local-first/",
		TitleStr: "Local-first software",
		Slug:     "inkandswitch-local-first-software",
		BlogName: "Ink & Switch",
		FeedUrl:  "https://www.inkandswitch.com/index.xml",
	},
	{
		Url:      "https://paulgraham.com/kids.html",
		TitleStr: "Having Kids",
		Slug:     "pg-having-kids",
		BlogName: "Paul Graham: Essays",
		FeedUrl:  "https://paulgraham.com/articles.html",
	},
	{
		Url:      "https://jvns.ca/blog/2020/07/14/when-your-coworker-does-great-work-tell-their-manager/",
		TitleStr: "When your coworker does great<br>work, tell their manager",
		Slug:     "jvns-great-work",
		BlogName: "Julia Evans",
		FeedUrl:  "https://jvns.ca/atom.xml",
	},
	{
		Url:      "https://www.benkuhn.net/attention/",
		TitleStr: "Attention is your scarcest resource",
		Slug:     "benkuhn-attention",
		BlogName: "Ben Kuhn",
		FeedUrl:  "https://www.benkuhn.net/index.xml",
	},
	{
		Url:      "https://danluu.com/look-stupid/",
		TitleStr: "Willingness to look stupid",
		Slug:     "danluu-willingness-to-look-stupid",
		BlogName: "Dan Luu",
		FeedUrl:  "https://danluu.com/atom.xml",
	},
	{
		Url:      "https://ciechanow.ski/mechanical-watch/",
		TitleStr: "Mechanical Watch",
		Slug:     "ciechanowski-mechanical-watch",
		BlogName: "Bartosz Ciechanowski",
		FeedUrl:  "https://ciechanow.ski/atom.xml",
	},
	{
		Url:      "https://acoup.blog/2022/08/26/collections-why-no-roman-industrial-revolution/",
		TitleStr: "Why No Roman Industrial Revolution?",
		Slug:     "acoup-roman-industrial-revolution",
		BlogName: "A Collection of Unmitigated Pedantry",
		FeedUrl:  "https://acoup.blog/feed/",
	},
}

var ScreenshotLinksBySlug = make(map[string]*ScreenshotLink)

func init() {
	for i := range ScreenshotLinks {
		link := &ScreenshotLinks[i]
		link.Title = template.HTML(link.TitleStr)
		titleMobileStr := strings.ReplaceAll(link.TitleStr, "<br>", " ")
		link.TitleMobile = template.HTML(titleMobileStr)
		link.PreviewPath = fmt.Sprintf("/preview/%s", link.Slug)

		ScreenshotLinksBySlug[link.Slug] = link
		SuggestionFeedUrls[link.FeedUrl] = true
	}
	ScreenshotLinks[0].IsEarliest = true
	ScreenshotLinks[len(ScreenshotLinks)-1].IsNewest = true
}

type Suggestions struct {
	Session             *Session
	SuggestedCategories []SuggestedCategory
	MiscellaneousBlogs  []MiscellaneousBlog
	WidthClass          string
}

type SuggestedCategory struct {
	Name           string
	IsRightAligned bool
	Blogs          []SuggestedBlog
}

type SuggestedBlog struct {
	Url         string
	FeedUrl     string
	AddFeedPath string
	Name        string
}

type MiscellaneousBlog struct {
	Url            string
	FeedUrl        string
	AddFeedPath    string
	Name           string
	Tag            string
	NonBreakingTag string
}

//nolint:exhaustruct
var SuggestedCategories = []SuggestedCategory{
	{
		Name:           "Programming",
		IsRightAligned: false,
		Blogs: []SuggestedBlog{
			{
				Url:     "https://danluu.com",
				FeedUrl: "https://danluu.com/atom.xml",
				Name:    "Dan Luu",
			},
			{
				Url:     "https://jvns.ca",
				FeedUrl: "https://jvns.ca/atom.xml",
				Name:    "Julia Evans",
			},
			{
				Url:     "https://brandur.org/articles",
				FeedUrl: "https://brandur.org/articles.atom",
				Name:    "Brandur Leach",
			},
			{
				Url:     "https://www.brendangregg.com/blog/",
				FeedUrl: "https://www.brendangregg.com/blog/rss.xml",
				Name:    "Brendan Gregg",
			},
			{
				Url:     "https://yosefk.com/blog/",
				FeedUrl: "https://yosefk.com/blog/feed",
				Name:    "Yossi Krenin",
			},
			{
				Url:     "https://www.reddit.com/r/gamedev/comments/wd4qoh/our_machinery_extensible_engine_made_in_c_just/",
				FeedUrl: "https://ourmachinery.com",
				Name:    "Our Machinery",
			},
			{
				Url:     "https://www.factorio.com/blog/",
				FeedUrl: "https://www.factorio.com/blog/rss",
				Name:    "Factorio",
			},
		},
	},
	{
		Name:           "Machine Learning",
		IsRightAligned: true,
		Blogs: []SuggestedBlog{
			{
				Url:     "https://karpathy.github.io",
				FeedUrl: "https://karpathy.github.io/feed.xml",
				Name:    "Andrej Karpathy",
			},
			{
				Url:     "https://distill.pub/",
				FeedUrl: "https://distill.pub/rss.xml",
				Name:    "Distill",
			},
			// TODO: reenable when/if they fix the 403 from cloudflare
			// {
			// 	Url:     "https://openai.com/news/",
			// 	FeedUrl: "https://openai.com/news/rss.xml",
			// 	Name:    "OpenAI",
			// },
			// Unclear if should ever be reenabled, they keep updating posts in the feed
			// (or should that be handled instead?)
			// {
			// 	Url:     "https://deepmind.google/discover/blog/",
			// 	FeedUrl: "https://deepmind.google/blog/rss.xml",
			// 	Name:    "Google DeepMind",
			// },
			{
				Url:     "https://bair.berkeley.edu/blog/",
				FeedUrl: "https://bair.berkeley.edu/blog/feed.xml",
				Name:    "BAIR",
			},
			{
				Url:     "https://jalammar.github.io/",
				FeedUrl: "https://jalammar.github.io/feed.xml",
				Name:    "Jay Alammar",
			},
		},
	},
	{
		Name:           "Rationality",
		IsRightAligned: false,
		Blogs: []SuggestedBlog{
			{
				Url:     "https://www.astralcodexten.com/",
				FeedUrl: "https://www.astralcodexten.com/feed",
				Name:    "Astral Codex Ten",
			},
			{
				Url:     "https://slatestarcodex.com/",
				FeedUrl: "https://slatestarcodex.com/feed/",
				Name:    "Slate Star Codex",
			},
			{
				Url:     "https://www.lesswrong.com/rationality",
				FeedUrl: "https://www.lesswrong.com/rationality",
				Name:    "Sequences",
			},
			{
				Url:     "https://www.overcomingbias.com/",
				FeedUrl: "https://www.overcomingbias.com/feed",
				Name:    "Overcoming Bias",
			},
			{
				Url:     "https://thezvi.substack.com/",
				FeedUrl: "https://thezvi.substack.com/feed",
				Name:    "Don't Worry About the Vase",
			},
			{
				Url:     "https://dynomight.net/",
				FeedUrl: "https://dynomight.net/feed.xml",
				Name:    "DYNOMIGHT",
			},
			{
				Url:     "https://sideways-view.com/",
				FeedUrl: "https://sideways-view.com/feed/",
				Name:    "The sideways view",
			},
			{
				Url:     "https://meltingasphalt.com/",
				FeedUrl: "https://feeds.feedburner.com/MeltingAsphalt",
				Name:    "Melting Asphalt",
			},
		},
	},
}

func init() {
	for _, category := range SuggestedCategories {
		for i := range category.Blogs {
			blog := &category.Blogs[i]
			blog.AddFeedPath = SubscriptionAddFeedPath(blog.FeedUrl)
			SuggestionFeedUrls[blog.FeedUrl] = true
		}
	}
}

func SubscriptionAddFeedPath(feedUrl string) string {
	return fmt.Sprintf("/subscriptions/add/%s", url.PathEscape(feedUrl))
}

//nolint:exhaustruct
var MiscellaneousBlogs = []MiscellaneousBlog{
	{
		Url:     "https://acoup.blog/",
		FeedUrl: "https://acoup.blog/feed/",
		Name:    "A Collection of Unmitigated Pedantry",
		Tag:     "history",
	},
	{
		Url:     "https://pedestrianobservations.com/",
		FeedUrl: "https://pedestrianobservations.com/feed/",
		Name:    "Pedestrian Observations",
		Tag:     "urbanism",
	},
	{
		Url:     "https://paulgraham.com/articles.html",
		FeedUrl: "https://paulgraham.com/articles.html",
		Name:    "Paul Graham",
		Tag:     "entrepreneurship",
	},
	{
		Url:     "https://caseyhandmer.wordpress.com/",
		FeedUrl: "https://caseyhandmer.wordpress.com/feed/",
		Name:    "Casey Handmer",
		Tag:     "space",
	},
	{
		Url:     "https://waitbutwhy.com/archive",
		FeedUrl: "https://waitbutwhy.com/feed",
		Name:    "Wait But Why",
		Tag:     "explainers",
	},
	{
		Url:     "https://www.mrmoneymustache.com/",
		FeedUrl: "https://feeds.feedburner.com/mrmoneymustache",
		Name:    "Mr. Money Mustache",
		Tag:     "personal finance",
	},
	{
		Url:     "https://blog.cryptographyengineering.com/",
		FeedUrl: "https://blog.cryptographyengineering.com/feed/",
		Name:    "Cryptographic Engineering",
		Tag:     "cryptography",
	},
	{
		Url:     "https://www.righto.com/",
		FeedUrl: "https://www.righto.com/feeds/posts/default",
		Name:    "Ken Shirriff",
		Tag:     "hardware",
	},
	{
		Url:     "https://daniellakens.blogspot.com/",
		FeedUrl: "https://daniellakens.blogspot.com/feeds/posts/default",
		Name:    "The 20% Statistician",
		Tag:     "statistics",
	},
}

func init() {
	for i := range MiscellaneousBlogs {
		blog := &MiscellaneousBlogs[i]
		blog.AddFeedPath = SubscriptionAddFeedPath(blog.FeedUrl)
		blog.NonBreakingTag = strings.ReplaceAll(blog.Tag, " ", "\u00A0")
		SuggestionFeedUrls[blog.FeedUrl] = true
	}
}
