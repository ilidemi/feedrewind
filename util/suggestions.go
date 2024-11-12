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
	IsPlayful           bool
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
				Url:     "https://github.com/ruby0x1/machinery_blog_archive",
				FeedUrl: "https://ourmachinery.com",
				Name:    "Our Machinery",
			},
			{
				Url:     "http://antirez.com",
				FeedUrl: "http://antirez.com/rss",
				Name:    "Antirez",
			},
			{
				Url:     "https://www.factorio.com/blog/",
				FeedUrl: "https://www.factorio.com/blog/rss",
				Name:    "Factorio",
			},
		},
	},
	{

		Name:           "Rationality",
		IsRightAligned: true,
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
			{
				Url:     "https://gwern.net",
				FeedUrl: "https://gwern.net",
				Name:    "Gwern",
			},
		},
	},
	{
		Name:           "AI",
		IsRightAligned: false,
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
			{
				Url:     "https://transformer-circuits.pub",
				FeedUrl: "https://transformer-circuits.pub",
				Name:    "Transformer Circuits",
			},
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

type HmnCategory struct {
	Name  string
	IsBig bool
	Blogs []HmnBlog
}

type HmnBlog struct {
	IconName        string // empty if missing
	IconPath        string
	Url             string
	FeedUrl         string
	AddFeedPath     string
	Name            string
	Tags            []string
	NonBreakingTags []string
	LibraryMentions []HmnLibraryMention
}

type HmnLibraryMention struct {
	Author      string
	Message     string
	MessageHtml template.HTML
	Posts       []HmnLibraryMentionPost
}

type HmnLibraryMentionPost struct {
	Title string
	Url   string
}

//nolint:exhaustruct
var HmnCategories = []HmnCategory{
	{
		Name:  "Most shared on Handmade Network",
		IsBig: true,
		Blogs: []HmnBlog{
			{
				IconName: "ryg.jpg",
				Url:      "https://fgiesen.wordpress.com",
				FeedUrl:  "https://fgiesen.wordpress.com/feed/",
				Name:     "The ryg blog",
				Tags:     []string{"graphics", "compression", "bit tricks", "math"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Keeba",
						Posts: []HmnLibraryMentionPost{{
							Title: "A whirlwind introduction to dataflow graphs",
							Url:   "https://fgiesen.wordpress.com/2018/03/05/a-whirlwind-introduction-to-dataflow-graphs/",
						}},
					},
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "Reading bits in far too many ways (part 3)",
							Url:   "https://fgiesen.wordpress.com/2018/09/27/reading-bits-in-far-too-many-ways-part-3",
						}},
					},
					{
						Author: "Chen",
						Posts: []HmnLibraryMentionPost{{
							Title: "Optimizing the basic rasterizer",
							Url:   "https://fgiesen.wordpress.com/2013/02/10/optimizing-the-basic-rasterizer/",
						}},
					},
					{
						Author: "ryanfleury",
						Posts: []HmnLibraryMentionPost{{
							Title: "A trip through the Graphics Pipeline 2011: Index",
							Url:   "https://fgiesen.wordpress.com/2011/07/09/a-trip-through-the-graphics-pipeline-2011-index/",
						}},
					},
				},
			},
			{
				IconName: "digitalgrove.png",
				Url:      "https://www.rfleury.com/",
				FeedUrl:  "https://www.rfleury.com/feed",
				Name:     "Digital Grove",
				Tags:     []string{"ui", "memory management", "software craftsmanship"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "The Sandvich Maker",
						Message: "I imagine many people here are aware of Ryan's substack, but for those that aren't:\nAn article outlining the idea behind linear allocator based, or stack based, or arena based allocation and how to use it to (greatly) simplify memory allocation patterns",
						Posts: []HmnLibraryMentionPost{{
							Title: "Untangling Lifetimes: The Arena Allocator",
							Url:   "https://www.rfleury.com/p/untangling-lifetimes-the-arena-allocator",
						}},
					},
					{
						Author: "NicknEma",
						Posts: []HmnLibraryMentionPost{{
							Title: "The Easiest Way To Handle Errors Is To Not Have Them",
							Url:   "https://www.rfleury.com/p/the-easiest-way-to-handle-errors",
						}},
					},
				},
			},
			{
				IconName: "fabiensanglard.png",
				Url:      "https://fabiensanglard.net/",
				FeedUrl:  "http://fabiensanglard.net/rss.xml",
				Name:     "Fabien Sanglard",
				Tags:     []string{"retro gamedev"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "mofelia",
						Posts: []HmnLibraryMentionPost{{
							Title: "Game Engine Black Book: Wolfenstein 3D",
							Url:   "http://fabiensanglard.net/Game_Engine_Black_Book_Release_Date/index.php",
						}},
					},
					{
						Author: "Jim0_o",
						Posts: []HmnLibraryMentionPost{{
							Title: "How DOOM fire was done",
							Url:   "https://fabiensanglard.net/doom_fire_psx/",
						}},
					},
					{
						Author: "ryza",
						Posts: []HmnLibraryMentionPost{{
							Title: "\"Another World\" Code Review",
							Url:   "http://fabiensanglard.net/anotherWorld_code_review/index.php",
						}},
					},
					{
						Author:  "Ben Visness",
						Message: "Both of Fabien Sanglard's Game Engine Black Books, on Wolfenstein and DOOM, are available on his website as both free PDFs and high-quality physical versions sold at cost:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Game Engine Black Books Update",
							Url:   "http://fabiensanglard.net/gebb/index.html",
						}},
					},
					{
						Author:  "ReasonableCoder",
						Message: "Not sure if these have been shared before... Great technical books/write-ups on DOOM and Wolfenstein 3D. PDFs freely available online:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Game Engine Black Books Update",
							Url:   "http://fabiensanglard.net/gebb/index.html",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "The polygons of DOOM: PSX",
							Url:   "http://fabiensanglard.net/doom_psx/index.html",
						}},
					},
					{
						Author:  "ktr_synchronizer",
						Message: "Virtual-machine based game and the adventures in porting it to other platforms.",
						Posts: []HmnLibraryMentionPost{{
							Title: "\"Another World\" Code Review",
							Url:   "https://fabiensanglard.net/anotherWorld_code_review/",
						}},
					},
					{
						Author:  "Cirdan",
						Message: "STREET FIGHTER II, PAPER TRAILS - sprite sheets",
						Posts: []HmnLibraryMentionPost{{
							Title: "Street Fighter II, paper trails",
							Url:   "https://fabiensanglard.net/sf2_sheets/index.html",
						}},
					},
					{
						Author:  "Uneven Prankster",
						Message: "Driving Compilers: 5-page walk through the process of the creation of an executable by Fabien Sanglard.",
						Posts: []HmnLibraryMentionPost{{
							Title: "Driving Compilers",
							Url:   "https://fabiensanglard.net/dc/index.php",
						}},
					},
				},
			},
		},
	},
	{
		Name:  "Graphics",
		IsBig: false,
		Blogs: []HmnBlog{
			{
				Url:     "https://floooh.github.io/",
				FeedUrl: "https://floooh.github.io/feed.xml",
				Name:    "The Brain Dump (floooh)",
				Tags:    []string{"sokol", "gamedev"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "The Sandvich Maker",
						Message: "Handles are the better pointers (about handle-based resource management)",
						Posts: []HmnLibraryMentionPost{{
							Title: "Handles are the better pointers",
							Url:   "https://floooh.github.io/2018/06/17/handles-vs-pointers.html",
						}},
					},
					{
						Author:  "Martins",
						Message: "Blog entry about Z80 cycle-stepped emulator:",
						Posts: []HmnLibraryMentionPost{{
							Title: "A new cycle-stepped Z80 emulator",
							Url:   "https://floooh.github.io/2021/12/17/cycle-stepped-z80.html",
						}},
					},
				},
			},
			{
				Url:     "https://raphlinus.github.io/",
				FeedUrl: "https://raphlinus.github.io/feed.xml",
				Name:    "Raph Levien’s blog",
				Tags:    []string{"vector graphics", "font technology", "rust gui"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Swapchains and frame pacing",
							Url:   "https://raphlinus.github.io/ui/graphics/gpu/2021/10/22/swapchain-frame-pacing.html",
						}},
					},
					{
						Author: "aolo2 (Alex)",
						Posts: []HmnLibraryMentionPost{{
							Title: "Swapchains and frame pacing",
							Url:   "https://raphlinus.github.io/ui/graphics/gpu/2021/10/22/swapchain-frame-pacing.html",
						}},
					},
					{
						Author:  "Jacob\U0001F41D",
						Message: "summary of concerns for a text layout engine",
						Posts: []HmnLibraryMentionPost{{
							Title: "Text layout is a loose hierarchy of segmentation",
							Url:   "https://raphlinus.github.io/text/2020/10/26/text-layout.html",
						}},
					},
					{
						Author: "ninerdelta",
						Posts: []HmnLibraryMentionPost{{
							Title: "The compositor is evil",
							Url:   "https://raphlinus.github.io/ui/graphics/2020/09/13/compositor-is-evil.html",
						}},
					},
				},
			},
			{
				IconName: "aras-p.png",
				Url:      "https://aras-p.info/",
				FeedUrl:  "https://aras-p.info/atom.xml",
				Name:     "Aras' website",
				Tags:     []string{"compression", "software optimization"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Shazan",
						Posts: []HmnLibraryMentionPost{{
							Title: "Best unknown MSVC flag: d2cgsummary",
							Url:   "http://aras-p.info/blog/2017/10/23/Best-unknown-MSVC-flag-d2cgsummary/",
						}},
					},
					{
						Author: "Shazan",
						Posts: []HmnLibraryMentionPost{{
							Title: "Slow to Compile Table Initializers",
							Url:   "http://aras-p.info/blog/2017/10/24/Slow-to-Compile-Table-Initializers/",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Texture Compression in 2020",
							Url:   "https://aras-p.info/blog/2020/12/08/Texture-Compression-in-2020/",
						}},
					},
					{
						Author:  "Lucas",
						Message: "Nice post about how Aras (one of the OG Unity guys) optimized blender’s obj file exporter.",
						Posts: []HmnLibraryMentionPost{{
							Title: "Speeding up Blender .obj export",
							Url:   "https://aras-p.info/blog/2022/02/03/Speeding-up-Blender-.obj-export/",
						}},
					},
					{
						Author:  "Martins",
						Message: "reasons to not trust your C runtime library (or why \"zero cost abstractions\" are not zero cost)",
						Posts: []HmnLibraryMentionPost{{
							Title: "Curious lack of sprintf scaling",
							Url:   "https://aras-p.info/blog/2022/02/25/Curious-lack-of-sprintf-scaling/",
						}},
					},
				},
			},
			{
				Url:     "http://the-witness.net/news/",
				FeedUrl: "http://the-witness.net/news/feed/",
				Name:    "The Witness",
				Tags:    []string{"gamedev"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{
							{
								Title: "Graphics Tech: Multi-lightmaps",
								Url:   "http://the-witness.net/news/2010/04/graphics-tech-multi-lightmaps/",
							},
							{
								Title: "Hemicube Rendering and Integration",
								Url:   "http://the-witness.net/news/2010/09/hemicube-rendering-and-integration/",
							},
							{
								Title: "Lightmap Parameterization",
								Url:   "http://the-witness.net/news/2010/03/graphics-tech-texture-parameterization/",
							},
							{
								Title: "Irradiance Caching – Part 1",
								Url:   "http://the-witness.net/news/2011/07/irradiance-caching-part-1/",
							},
						},
					},
					{
						Author:  "Shazan",
						Message: "about floating point numbers and how it loses precision over time when numbers are large\nAnd how shaders use floating point to store time and issues related and how to fix it",
						Posts: []HmnLibraryMentionPost{{
							Title: "A Shader Trick",
							Url:   "http://the-witness.net/news/2022/02/a-shader-trick/",
						}},
					},
				},
			},
			{
				IconName: "adriancourreges.png",
				Url:      "http://www.adriancourreges.com/",
				FeedUrl:  "http://www.adriancourreges.com/atom.xml",
				Name:     "Adrian Courrèges",
				Tags:     []string{"AAA frame study"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "oliver3355",
						Posts: []HmnLibraryMentionPost{{
							Title: "Beware of Transparent Pixels",
							Url:   "http://www.adriancourreges.com/blog/2017/05/09/beware-of-transparent-pixels/",
						}},
					},
					{
						Author:  "Deleted User",
						Message: "A collection of those \"how XXX renders a frame\" articles.",
						Posts: []HmnLibraryMentionPost{{
							Title: "Graphics Studies Compilation",
							Url:   "https://www.adriancourreges.com/blog/2020/12/29/graphics-studies-compilation/",
						}},
					},
					{
						Author:  "Red Tearstone Ring",
						Message: "meta list of frame capture breakdowns",
						Posts: []HmnLibraryMentionPost{{
							Title: "Graphics Studies Compilation",
							Url:   "https://www.adriancourreges.com/blog/2020/12/29/graphics-studies-compilation/",
						}},
					},
					{
						Author:  "Jakub",
						Message: "Doom 2016 renderering pipeline breakdown",
						Posts: []HmnLibraryMentionPost{{
							Title: "DOOM (2016) - Graphics Study",
							Url:   "http://www.adriancourreges.com/blog/2016/09/09/doom-2016-graphics-study/",
						}},
					},
					{
						Author:  "Steіn",
						Message: "Compilation of graphics studies",
						Posts: []HmnLibraryMentionPost{{
							Title: "Adrian Courrèges",
							Url:   "https://www.adriancourreges.com/blog/",
						}},
					},
				},
			},
			{
				Url:     "https://zeux.io/",
				FeedUrl: "https://zeux.io/feed/",
				Name:    "zeux.io",
				Tags:    nil,
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Oskar",
						Posts: []HmnLibraryMentionPost{{
							Title: "Writing an efficient Vulkan renderer",
							Url:   "https://zeux.io/2020/02/27/writing-an-efficient-vulkan-renderer/",
						}},
					},
					{
						Author:  "Phillip Trudeau \U0001F1E8\U0001F1E6",
						Message: "Rerelease of good old article about compiler speed",
						Posts: []HmnLibraryMentionPost{{
							Title: "On Proebsting's Law",
							Url:   "https://zeux.io/2022/01/08/on-proebstings-law/",
						}},
					},
				},
			},
			{
				IconName: "3dgep.png",
				Url:      "https://www.3dgep.com/",
				FeedUrl:  "https://www.3dgep.com/feed/",
				Name:     "3D Game Engine Programming",
				Tags:     []string{"gamedev"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "platin21",
						Posts: []HmnLibraryMentionPost{{
							Title: "DirectX Archives",
							Url:   "https://www.3dgep.com/category/graphics-programming/directx/",
						}},
					},
				},
			},
			{
				IconName: "therealmjp.png",
				Url:      "https://therealmjp.github.io/",
				FeedUrl:  "https://therealmjp.github.io/posts/index.xml",
				Name:     "The Danger Zone",
				Tags:     nil,
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "Breaking Down Barriers – Part 5: Back To The Real World",
							Url:   "https://therealmjp.github.io/posts/breaking-down-barriers-part-5-back-to-the-real-world/",
						}},
					},
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{{
							Title: "SG Series Part 1: A Brief (and Incomplete) History of Baked Lighting Representations",
							Url:   "https://therealmjp.github.io/posts/sg-series-part-1-a-brief-and-incomplete-history-of-baked-lighting-representations/",
						}},
					},
					{
						Author: "Chen",
						Posts: []HmnLibraryMentionPost{{
							Title: "A Sampling of Shadow Techniques",
							Url:   "https://therealmjp.github.io/posts/shadow-maps/",
						}},
					},
					{
						Author:  "wataru",
						Message: "This is an older article, but it's an interesting writeup about shadow techniques. If anyone's got a more recent and better article, feel free to share!",
						Posts: []HmnLibraryMentionPost{{
							Title: "A Sampling of Shadow Techniques",
							Url:   "https://therealmjp.github.io/posts/shadow-maps/",
						}},
					},
					{
						Author:  "BlarghamelJones",
						Message: "This post is about D3D12, but Vulkan is similar",
						Posts: []HmnLibraryMentionPost{{
							Title: "GPU Memory Pools in D3D12",
							Url:   "https://therealmjp.github.io/posts/gpu-memory-pool/",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Shader Printf in HLSL and DX12",
							Url:   "https://therealmjp.github.io/posts/hlsl-printf/",
						}},
					},
				},
			},
			{
				Url:     "http://www.ludicon.com/castano/blog/",
				FeedUrl: "http://www.ludicon.com/castano/blog/feed/",
				Name:    "Ignacio Castaño",
				Tags:    nil,
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{
							{
								Title: "Irradiance Caching – Continued",
								Url:   "http://www.ludicon.com/castano/blog/2014/07/irradiance-caching-continued/",
							},
							{
								Title: "The Witness: Lightmap optimizations for iOS",
								Url:   "http://www.ludicon.com/castano/blog/2017/10/lightmap-optimizations-ios/",
							},
							{
								Title: "Lightmap Compression in The Witness",
								Url:   "http://www.ludicon.com/castano/blog/2016/09/lightmap-compression-in-the-witness/",
							},
						},
					},
				},
			},
			{
				Url:     "https://bartwronski.com/",
				FeedUrl: "https://bartwronski.com/feed/",
				Name:    "Bart Wronski",
				Tags:    []string{"image processing"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Cirdan",
						Posts: []HmnLibraryMentionPost{{
							Title: "Separate your filters! Separability, SVD and low-rank approximation of...",
							Url:   "https://bartwronski.com/2020/02/03/separate-your-filters-svd-and-low-rank-approximation-of-image-filters/",
						}},
					},
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{{
							Title: "Is this a branch?",
							Url:   "https://bartwronski.com/2021/01/18/is-this-a-branch/",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Bilinear down/upsampling, aligning pixel grids, and that infamous GPU half pixel offset",
							Url:   "https://bartwronski.com/2021/02/15/bilinear-down-upsampling-pixel-grids-and-that-half-pixel-offset/",
						}},
					},
				},
			},
			{
				IconName: "filmicworlds.png",
				Url:      "http://filmicworlds.com/",
				FeedUrl:  "http://filmicworlds.com/feed.xml",
				Name:     "Filmic Worlds",
				Tags:     []string{"photorealistic"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{{
							Title: "Filmic Tonemapping with Piecewise Power Curves",
							Url:   "http://filmicworlds.com/blog/filmic-tonemapping-with-piecewise-power-curves/",
						}},
					},
					{
						Author: "Kknewkles",
						Posts: []HmnLibraryMentionPost{{
							Title: "Linear-Space Lighting (i.e. Gamma)",
							Url:   "http://filmicworlds.com/blog/linear-space-lighting-i-e-gamma/",
						}},
					},
					{
						Author:  "Oskar",
						Message: "Simple explanation of sRGB/gamma vs linear space lighting:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Linear-Space Lighting (i.e. Gamma)",
							Url:   "http://filmicworlds.com/blog/linear-space-lighting-i-e-gamma/",
						}},
					},
				},
			},
			{
				Url:     "https://interplayoflight.wordpress.com/",
				FeedUrl: "https://interplayoflight.wordpress.com/feed/",
				Name:    "Interplay of Light",
				Tags:    []string{"raytracing"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Order independent transparency, part 1",
							Url:   "https://interplayoflight.wordpress.com/2022/06/25/order-independent-transparency-part-1/",
						}},
					},
					{
						Author:  "Martins",
						Message: "Followup on part 1 earlier:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Order independent transparency, part 2",
							Url:   "https://interplayoflight.wordpress.com/2022/07/02/order-independent-transparency-part-2/",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Low-level thinking in high-level shading languages 2023",
							Url:   "https://interplayoflight.wordpress.com/2023/12/29/low-level-thinking-in-high-level-shading-languages-2023/",
						}},
					},
				},
			},
		},
	},
	{
		Name:  "Systems Programming",
		IsBig: false,
		Blogs: []HmnBlog{
			{
				IconName: "gb.png",
				Url:      "https://www.gingerbill.org/",
				FeedUrl:  "https://www.gingerbill.org/article/index.xml",
				Name:     "Articles on gingerBill",
				Tags:     []string{"memory allocation", "odin", "language dev"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Br",
						Posts: []HmnLibraryMentionPost{{
							Title: "Memory Allocation Strategies - Part 1",
							Url:   "http://www.gingerbill.org/article/2019/02/01/memory-allocation-strategies-001/",
						}},
					},
				},
			},
			{
				IconName: "nullprogram.png",
				Url:      "https://nullprogram.com",
				FeedUrl:  "https://nullprogram.com/feed/",
				Name:     "null program",
				Tags:     nil,
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "Deleted User",
						Message: "UTF-8 for Windows: libwinsane",
						Posts: []HmnLibraryMentionPost{{
							Title: "Some sanity for C and C++ development on Windows",
							Url:   "https://nullprogram.com/blog/2021/12/30/",
						}},
					},
					{
						Author: "progfix",
						Posts: []HmnLibraryMentionPost{{
							Title: "SDL2 common mistakes and how to avoid them",
							Url:   "https://nullprogram.com/blog/2021/12/30/",
						}},
					},
					{
						Author: "aolo2 (Alex)",
						Posts: []HmnLibraryMentionPost{{
							Title: "Practical libc-free threading on Linux",
							Url:   "https://nullprogram.com/blog/2023/03/23/",
						}},
					},
					{
						Author:  "Ali A.",
						Message: "a simple guide for CRT-free programs on windows, it has tips for most of gotchas that you encounter\nand it is updated to 2023",
						Posts: []HmnLibraryMentionPost{{
							Title: "CRT-free in 2023: tips and tricks",
							Url:   "https://nullprogram.com/blog/2023/02/15/",
						}},
					},
				},
			},
			{
				IconName: "computerenhance.png",
				Url:      "https://computerenhance.com",
				FeedUrl:  "https://computerenhance.com/feed",
				Name:     "Computer, Enhance!",
				Tags:     []string{"performance-aware programming"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "feynon",
						Message: "Recent Microsoft linkers accumulate large amounts of outdated data in PDBs, even when all incremental build options are disabled, leading to potential debugger bugs and bloated PDB sizes; deleting PDBs before building can help manage this issue:",
						Posts: []HmnLibraryMentionPost{{
							Title: "MSVC PDBs Are Filled With Stale Debug Info",
							Url:   "https://www.computerenhance.com/p/msvc-pdbs-are-filled-with-stale-debug",
						}},
					},
					{
						Author:  "Oscar",
						Message: "Casey Muratori's article discussing higher than expected performance of a microbenchmark on Golden Cove / Alder Lake P-cores, and speculation on the scarcely documented changes that may lead to these results:",
						Posts: []HmnLibraryMentionPost{{
							Title: "The Case of the Missing Increment",
							Url:   "https://www.computerenhance.com/p/the-case-of-the-missing-increment",
						}},
					},
				},
			},
			{
				IconName: "randomascii.png",
				Url:      "https://randomascii.wordpress.com/",
				FeedUrl:  "https://randomascii.wordpress.com/feed/",
				Name:     "Random ASCII - tech blog of Bruce Dawson",
				Tags:     []string{"performance investigations"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "Exercises in Emulation: Xbox 360’s FMA Instruction",
							Url:   "https://randomascii.wordpress.com/2019/03/20/exercises-in-emulation-xbox-360s-fma-instruction/",
						}},
					},
					{
						Author:  "Reikling",
						Message: "A bunch of high quality blog posts about windows, performance, chromium, and more",
						Posts: []HmnLibraryMentionPost{{
							Title: "Random ASCII - tech blog of Bruce Dawson",
							Url:   "https://randomascii.wordpress.com/",
						}},
					},
				},
			},
			{
				IconName: "preshing.png",
				Url:      "https://preshing.com/",
				FeedUrl:  "https://preshing.com/feed",
				Name:     "Preshing on Programming",
				Tags:     []string{"c++", "algorithms"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{{
							Title: "Hash Collision Probabilities",
							Url:   "https://preshing.com/20110504/hash-collision-probabilities/",
						}},
					},
					{
						Author: "ninerdelta",
						Posts: []HmnLibraryMentionPost{{
							Title: "How to Write Your Own C++ Game Engine",
							Url:   "https://preshing.com/20171218/how-to-write-your-own-cpp-game-engine/",
						}},
					},
				},
			},
			{
				Url:     "https://probablydance.com/",
				FeedUrl: "https://probablydance.com/feed/",
				Name:    "Probably Dance",
				Tags:    []string{"c++", "multithreading", "data structures"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "A Programmers Take on “Six Memos for the Next Millennium”",
							Url:   "https://probablydance.com/2019/03/09/a-programmers-take-on-six-memos-for-the-next-millenium/",
						}},
					},
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "A New Algorithm for Controlled Randomness",
							Url:   "https://probablydance.com/2019/08/28/a-new-algorithm-for-controlled-randomness/",
						}},
					},
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "Measuring Mutexes, Spinlocks and how Bad the Linux Scheduler Really is",
							Url:   "https://probablydance.com/2019/12/30/measuring-mutexes-spinlocks-and-how-bad-the-linux-scheduler-really-is/",
						}},
					},
					{
						Author:  "Martins",
						Message: "Min-Max Heap (faster and more efficient Binary Heap):",
						Posts: []HmnLibraryMentionPost{{
							Title: "On Modern Hardware the Min-Max Heap beats a Binary Heap",
							Url:   "https://probablydance.com/2020/08/31/on-modern-hardware-the-min-max-heap-beats-a-binary-heap/",
						}},
					},
					{
						Author:  "ratchetfreak",
						Message: "a post hash operation for getting better hash table distributions:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Fibonacci Hashing: The Optimization that the World Forgot (or: a Be...",
							Url:   "https://probablydance.com/2018/06/16/fibonacci-hashing-the-optimization-that-the-world-forgot-or-a-better-alternative-to-integer-modulo/",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Beautiful Branchless Binary Search",
							Url:   "https://probablydance.com/2023/04/27/beautiful-branchless-binary-search/",
						}},
					},
					{
						Author:  "Boostibot",
						Message: "Great article linked by @aolo2 (Alex) about weird behaviour of different (spin)lock implementations under different thread schedulers",
						Posts: []HmnLibraryMentionPost{{
							Title: "Measuring Mutexes, Spinlocks and how Bad the Linux Scheduler Really is",
							Url:   "https://probablydance.com/2019/12/30/measuring-mutexes-spinlocks-and-how-bad-the-linux-scheduler-really-is/",
						}},
					},
				},
			},
			{
				IconName: "lemire.jpg",
				Url:      "https://lemire.me/",
				FeedUrl:  "https://lemire.me/blog/feed/",
				Name:     "Daniel Lemire's blog",
				Tags:     []string{"simd", "algorithms", "data structures"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Ridiculously fast unicode (UTF-8) validation",
							Url:   "https://lemire.me/blog/2020/10/20/ridiculously-fast-unicode-utf-8-validation/",
						}},
					},
					{
						Author:  "Martins",
						Message: "Number Parsing at a Gigabyte per Second:\n> With disks and networks providing gigabytes per second, parsing decimal numbers from strings becomes a bottleneck. We consider the problem of parsing decimal numbers to the nearest binary floating-point value. The general problem requires variable-precision arithmetic. However, we need at most 17 digits to represent 64-bit standard floating-point numbers (IEEE 754). Thus we can represent the decimal significand with a single 64-bit word. By combining the significand and precomputed tables, we can compute the nearest floating-point number using as few as one or two 64-bit multiplications. Our implementation can be several times faster than conventional functions present in standard C libraries on modern 64-bit systems (Intel, AMD, ARM and POWER9). Our work is available as open source software used by major systems such as Apache Arrow and Yandex ClickHouse. The Go standard library has adopted a version of our approach.\nImplementation in C++:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Fast float parsing in practice",
							Url:   "https://lemire.me/blog/2020/03/10/fast-float-parsing-in-practice",
						}},
					},
				},
			},
			{
				IconName: "travisdowns.png",
				Url:      "https://travisdowns.github.io/",
				FeedUrl:  "https://travisdowns.github.io/feed.xml",
				Name:     "Performance Matters",
				Tags:     []string{"cpu internals"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Justas",
						Posts: []HmnLibraryMentionPost{{
							Title: "Performance speed limits",
							Url:   "https://travisdowns.github.io/blog/2019/06/11/speed-limits.html",
						}},
					},
					{
						Author:  "x13pixels",
						Message: "Just including the STD header algorithm (not even using anything from it) can slow down your code",
						Posts: []HmnLibraryMentionPost{{
							Title: "Clang format tanks performance",
							Url:   "https://travisdowns.github.io/blog/2019/11/19/toupper.html",
						}},
					},
					{
						Author: "ratchetfreak",
						Posts: []HmnLibraryMentionPost{{
							Title: "Gathering Intel on Intel AVX-512 Transitions",
							Url:   "https://travisdowns.github.io/blog/2020/01/17/avxfreq1.html",
						}},
					},
					{
						Author:  "Ali A.",
						Message: "radix sort (for integers) beats the C/C++ standard library implementations of std::sort / qsort",
						Posts: []HmnLibraryMentionPost{{
							Title: "Beating Up on Qsort",
							Url:   "https://travisdowns.github.io/blog/2019/05/22/sorting.html",
						}},
					},
				},
			},
			{
				IconName: "thume.png",
				Url:      "https://thume.ca/",
				FeedUrl:  "https://thume.ca/atom.xml",
				Name:     "Tristan Hume",
				Tags:     []string{"performance", "hardware"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "Deleted User",
						Message: "Blog post on styles of optimizing. E.g. focusing on Big O vs real-time constraints etc",
						Posts: []HmnLibraryMentionPost{{
							Title: "Two Performance Aesthetics: Never Miss a Frame and Do Almost Nothing",
							Url:   "https://thume.ca/2019/07/27/two-performance-aesthetics/",
						}},
					},
				},
			},
			{
				Url:     "https://blog.molecular-matters.com/",
				FeedUrl: "https://blog.molecular-matters.com/feed/",
				Name:    "Molecular Musings",
				Tags:    []string{"live++", "memory allocation", "software architecture"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Andrew (azmr)",
						Posts: []HmnLibraryMentionPost{{
							Title: "Job System 2.0: Lock-Free Work Stealing – Part 1: Basics",
							Url:   "https://blog.molecular-matters.com/2015/08/24/job-system-2-0-lock-free-work-stealing-part-1-basics/",
						}},
					},
				},
			},
			{
				IconName:        "brendangregg.png",
				Url:             "https://brendangregg.com/",
				FeedUrl:         "https://brendangregg.com/blog/rss.xml",
				Name:            "Brendan Gregg's Blog",
				Tags:            []string{"performance", "ebpf", "flamegraphs"},
				LibraryMentions: nil,
			},
			{
				IconName: "marc-b-reynolds.png",
				Url:      "https://marc-b-reynolds.github.io/",
				FeedUrl:  "https://marc-b-reynolds.github.io/feed.xml",
				Name:     "MBR",
				Tags:     []string{"math", "bit tricks"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "Martins",
						Message: "Good example of how to use sollya to implement very accurate (max error 1ulp) full-range sinpi/cospi.",
						Posts: []HmnLibraryMentionPost{{
							Title: "A design and implementation of sincospi in binary32",
							Url:   "http://marc-b-reynolds.github.io/math/2020/03/11/SinCosPi.html",
						}},
					},
					{
						Author:  "Blatnik",
						Message: "Fast and simple normal distribution approximations:",
						Posts: []HmnLibraryMentionPost{{
							Title: "A cheap normal distribution approximation",
							Url:   "https://marc-b-reynolds.github.io/distribution/2021/03/18/CheapGaussianApprox.html",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Popcount walks: next, previous, toward and nearest",
							Url:   "http://marc-b-reynolds.github.io/math/2023/11/09/PopNextPrev.html",
						}},
					},
				},
			},
			{
				Url:     "https://unixism.net/",
				FeedUrl: "https://unixism.net/feed/",
				Name:    "Unixism",
				Tags:    []string{"linux", "open source", "performance"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "Martins",
						Message: "ZeroHTTPd: A web server to teach Linux performance, with benchmarks",
						Posts: []HmnLibraryMentionPost{{
							Title: "Linux Applications Performance: Introduction",
							Url:   "https://unixism.net/2019/04/linux-applications-performance-introduction/",
						}},
					},
					{
						Author:  "Martins",
						Message: "io_uring example (new, high-performance interface for asynchronous I/O):",
						Posts: []HmnLibraryMentionPost{{
							Title: "io_uring By Example: An Article Series",
							Url:   "https://unixism.net/2020/04/io-uring-by-example-article-series/",
						}},
					},
				},
			},
			{
				Url:     "https://maskray.me/",
				FeedUrl: "https://maskray.me/blog/atom.xml",
				Name:    "MaskRay",
				Tags:    []string{"compilers"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "Martins",
						Message: "Subtitle: How to make my /usr/bin executables 8% smaller?",
						Posts: []HmnLibraryMentionPost{{
							Title: "Relative relocations and RELR",
							Url:   "https://maskray.me/blog/2021-10-31-relative-relocations-and-relr",
						}},
					},
					{
						Author:  "Martins",
						Message: ".init, .ctors, and .init_array - how gcc/linux runtime initializes/destructs global C & C++ things:",
						Posts: []HmnLibraryMentionPost{{
							Title: ".init, .ctors, and .init_array",
							Url:   "https://maskray.me/blog/2021-11-07-init-ctors-init-array",
						}},
					},
					{
						Author:  "Ali A.",
						Message: "Somewhat an introduction to object file format coff Mach-o and elf",
						Posts: []HmnLibraryMentionPost{{
							Title: "Exploring object file formats",
							Url:   "https://maskray.me/blog/2024-01-14-exploring-object-file-formats",
						}},
					},
				},
			},
			{
				Url:     "https://nigeltao.github.io/",
				FeedUrl: "https://nigeltao.github.io/feed.xml",
				Name:    "Nigel Tao's blog",
				Tags:    []string{"compression"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "Martins",
						Message: "series of blog articles explaining how zstd works:",
						Posts: []HmnLibraryMentionPost{{
							Title: "Zstandard Worked Example Part 1: Concepts",
							Url:   "https://nigeltao.github.io/blog/2022/zstandard-part-1-concepts.html",
						}},
					},
					{
						Author:  "Martins",
						Message: "Number Parsing at a Gigabyte per Second:\n> With disks and networks providing gigabytes per second, parsing decimal numbers from strings becomes a bottleneck. We consider the problem of parsing decimal numbers to the nearest binary floating-point value. The general problem requires variable-precision arithmetic. However, we need at most 17 digits to represent 64-bit standard floating-point numbers (IEEE 754). Thus we can represent the decimal significand with a single 64-bit word. By combining the significand and precomputed tables, we can compute the nearest floating-point number using as few as one or two 64-bit multiplications. Our implementation can be several times faster than conventional functions present in standard C libraries on modern 64-bit systems (Intel, AMD, ARM and POWER9). Our work is available as open source software used by major systems such as Apache Arrow and Yandex ClickHouse. The Go standard library has adopted a version of our approach.\nImplementation in C:",
						Posts: []HmnLibraryMentionPost{{
							Title: "The Eisel-Lemire ParseNumberF64 Algorithm",
							Url:   "https://nigeltao.github.io/blog/2020/eisel-lemire.html",
						}},
					},
					{
						Author:  "Martins",
						Message: "jsonptr is a new, sandboxed command-line tool that formats JSON and speaks the JSON Pointer query syntax. Wuffs standard library’s JSON decoder can run in O(1) memory, even with arbitrarily long input (containing arbitrarily long strings) because it uses multiple tokens to represent each JSON string. Processing the JSON Pointer query during (instead of after) parsing can dramatically impact performance. jsonptr can be faster, tighter (use less memory) and safer than alternatives such as jq, serde_json and simdjson.\n\nInteresting article on how jsonptr uses wuff's coroutines & SECCOMP for safe json parsing",
						Posts: []HmnLibraryMentionPost{{
							Title: "Jsonptr: Using Wuffs’ Memory-Safe, Zero-Allocation JSON Decoder",
							Url:   "https://nigeltao.github.io/blog/2020/jsonptr.html",
						}},
					},
				},
			},
			{
				Url:     "http://www.corsix.org/",
				FeedUrl: "http://www.corsix.org/rss.xml",
				Name:    "corsix.org",
				Tags:    []string{"lua", "jit", "bit tricks", "cpu architectures"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Faster CRC32-C on x86",
							Url:   "https://www.corsix.org/content/fast-crc32c-4k",
						}},
					},
					{
						Author: "Martins",
						Posts: []HmnLibraryMentionPost{{
							Title: "Galois field instructions on 2021 CPUs",
							Url:   "http://www.corsix.org/content/galois-field-instructions-2021-cpus",
						}},
					},
					{
						Author:  "Martins",
						Message: "> Given the current register state for a thread, and read-only access to memory, what would the register state hypothetically become if the current function was to immediately return and execution was to resume in its caller?\n> [...]\n> This operation is typically called unwinding (as in \"unwinding a call frame\" or \"unwinding the stack\").",
						Posts: []HmnLibraryMentionPost{{
							Title: "Linux/ELF .eh_frame from the bottom up",
							Url:   "https://www.corsix.org/content/elf-eh-frame",
						}},
					},
				},
			},
		},
	},
	{
		Name:  "General Programming",
		IsBig: false,
		Blogs: []HmnBlog{
			{
				Url:     "https://danluu.com/",
				FeedUrl: "https://danluu.com/atom.xml",
				Name:    "Dan Luu",
				Tags:    []string{"big tech", "computer architecture", "critical analysis"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "notnullnotvoid",
						Posts: []HmnLibraryMentionPost{{
							Title: "Files are fraught with peril",
							Url:   "https://danluu.com/deconstruct-files/",
						}},
					},
				},
			},
			{
				IconName:        "drewdevault.png",
				Url:             "https://drewdevault.com/",
				FeedUrl:         "https://drewdevault.com/blog/index.xml",
				Name:            "Drew DeVault's blog",
				Tags:            []string{"open source"},
				LibraryMentions: nil,
			},
			{
				IconName: "ciechanowski.png",
				Url:      "https://ciechanow.ski/",
				FeedUrl:  "https://ciechanow.ski/atom.xml",
				Name:     "Bartosz Ciechanowski",
				Tags:     []string{"interactive explainers"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{{
							Title: "Alpha Compositing",
							Url:   "https://ciechanow.ski/alpha-compositing/",
						}},
					},
					{
						Author: "Shazan",
						Posts: []HmnLibraryMentionPost{{
							Title: "Lights and Shadows",
							Url:   "https://ciechanow.ski/lights-and-shadows/",
						}},
					},
					{
						Author:  "Br",
						Message: "I'm sure this has been posted before, but what a fantastic article on color, filtering and compositing",
						Posts: []HmnLibraryMentionPost{{
							Title: "Alpha Compositing",
							Url:   "https://ciechanow.ski/alpha-compositing/",
						}},
					},
					{
						Author:  "pablo-dgr",
						Message: "An amazing fully interactive and visual explanation of Bezier curves and sufaces",
						Posts: []HmnLibraryMentionPost{{
							Title: "Curves and Surfaces",
							Url:   "https://ciechanow.ski/curves-and-surfaces/",
						}},
					},
					{
						Author:  "ratchetfreak",
						Message: "how GPS works:",
						Posts: []HmnLibraryMentionPost{{
							Title: "GPS",
							Url:   "https://ciechanow.ski/gps/",
						}},
					},
					{
						Author:  "Deleted User",
						Message: "A well-written, interactive article about Alpha Compositing.",
						Posts: []HmnLibraryMentionPost{{
							Title: "Alpha Compositing",
							Url:   "https://ciechanow.ski/alpha-compositing/",
						}},
					},
				},
			},
			{
				Url:             "https://bvisness.me/",
				FeedUrl:         "https://bvisness.me/index.xml",
				Name:            "Ben Visness",
				Tags:            []string{"dear leader"},
				LibraryMentions: nil,
			},
			{
				IconName: "acko.png",
				Url:      "https://acko.net/",
				FeedUrl:  "https://acko.net/atom.xml",
				Name:     "Acko.net",
				Tags:     []string{"webdev", "graphics"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author:  "b_sharp_add9",
						Message: "Cool visualization of the fourier transform",
						Posts: []HmnLibraryMentionPost{{
							Title: "Fourier Analysis",
							Url:   "https://acko.net/files/gltalks/toolsforthought/#28",
						}},
					},
					{
						Author:  "Guntha",
						Message: "Frame study of Teardown",
						Posts: []HmnLibraryMentionPost{{
							Title: "Teardown Frame Teardown",
							Url:   "https://acko.net/blog/teardown-frame-teardown/",
						}},
					},
				},
			},
			{
				IconName:        "hmn.png",
				Url:             "https://handmade.network/fishbowl",
				FeedUrl:         "https://handmade.network/fishbowl",
				Name:            "Fishbowls | Handmade Network",
				Tags:            nil,
				LibraryMentions: nil,
			},
			{
				IconName: "inkandswitch.png",
				Url:      "https://www.inkandswitch.com/",
				FeedUrl:  "https://www.inkandswitch.com/index.xml",
				Name:     "Ink & Switch",
				Tags:     []string{"local-first software", "tools for thought"},
				LibraryMentions: []HmnLibraryMention{{
					Author:  "Lucas",
					Message: "Some awesome work from the Ink&Switch guys:",
					Posts: []HmnLibraryMentionPost{{
						Title: "Inkbase: Programmable Ink",
						Url:   "https://www.inkandswitch.com/inkbase/",
					}},
				}},
			},
			{
				IconName: "demofox.jpg",
				Url:      "https://blog.demofox.org/",
				FeedUrl:  "https://blog.demofox.org/feed/",
				Name:     "The blog at the bottom of the sea",
				Tags:     []string{"math", "algorithms"},
				LibraryMentions: []HmnLibraryMention{
					{
						Author: "Solwer",
						Posts: []HmnLibraryMentionPost{{
							Title: "Demystifying Floating Point Precision",
							Url:   "https://blog.demofox.org/2017/11/21/floating-point-precision/",
						}},
					},
					{
						Author: "Oskar",
						Posts: []HmnLibraryMentionPost{{
							Title: "Using Low Discrepancy Sequences & Blue Noise in Loot Drop Tables for Games",
							Url:   "https://blog.demofox.org/2020/03/01/using-low-discrepancy-sequences-blue-noise-in-loot-drop-tables-for-games/",
						}},
					},
					{
						Author: "Deleted User",
						Posts: []HmnLibraryMentionPost{{
							Title: "Fast Voronoi Diagrams and Distance Field Textures on the GPU With the Jump Flooding Algorithm",
							Url:   "https://blog.demofox.org/2016/02/29/fast-voronoi-diagrams-and-distance-dield-textures-on-the-gpu-with-the-jump-flooding-algorithm/",
						}},
					},
				},
			},
		},
	},
}

func init() {
	for i := range HmnCategories {
		category := &HmnCategories[i]
		for j := range category.Blogs {
			blog := &category.Blogs[j]
			if blog.IconName != "" {
				blog.IconPath = "hmn/" + blog.IconName
			}
			blog.AddFeedPath = SubscriptionAddFeedPath(blog.FeedUrl)
			for k := range blog.LibraryMentions {
				libraryMention := &blog.LibraryMentions[k]
				if libraryMention.Message != "" {
					messageHtml := strings.ReplaceAll(libraryMention.Message, "\n", "<br>")
					libraryMention.MessageHtml = template.HTML(messageHtml)
				}
			}
			for _, tag := range blog.Tags {
				nonBreakingTag := strings.ReplaceAll(tag, " ", "\u00A0")
				blog.NonBreakingTags = append(blog.NonBreakingTags, nonBreakingTag)
			}
			SuggestionFeedUrls[blog.FeedUrl] = true
		}
	}
}
