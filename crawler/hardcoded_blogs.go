package crawler

import (
	"encoding/xml"
	"feedrewind/log"
	"feedrewind/oops"
	"fmt"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
)

/*
	Blogs here have explicit branches for them in code, there are more in the database
*/

// Hardcoded as fake feeds that point to the DB
const HardcodedOurMachinery = "https://ourmachinery.com"
const HardcodedSequences = "https://www.lesswrong.com/rationality"

const HardcodedDanLuuFeedName = "Dan Luu"

var hardcodedACOUP CanonicalUri
var HardcodedAstralCodexTenFeed CanonicalUri
var hardcodedBenKuhn *Link
var hardcodedBenKuhnArchives CanonicalUri
var hardcodedCaseyHandmer CanonicalUri
var hardcodedCaseyHandmerSpaceMisconceptions *Link
var hardcodedCaseyHandmerMarsTrilogy *Link
var hardcodedCaseyHandmerMarsTrilogyFirstLink *Link
var hardcodedCaseyHandmerFutureOfEnergy *Link
var hardcodedCaseyHandmerBookReviews *Link
var hardcodedCryptographyEngineering CanonicalUri
var hardcodedCryptographyEngineeringAll CanonicalUri
var hardcodedDanLuu CanonicalUri
var HardcodedDanLuuFeed CanonicalUri
var HardcodedDontWorryAboutTheVaseFeed CanonicalUri
var hardcodedFactorio CanonicalUri
var hardcodedJuliaEvans CanonicalUri
var hardcodedKalzumeus CanonicalUri
var hardcodedMrMoneyMustache CanonicalUri
var HardcodedOvercomingBiasFeed CanonicalUri
var hardcodedPaulGraham CanonicalUri
var HardcodedSlateStarCodexFeed string

func init() {
	logger := NewDummyLogger()

	hardcodedACOUP = hardcodedMustParse("https://acoup.blog/")
	HardcodedAstralCodexTenFeed = hardcodedMustParse("https://www.astralcodexten.com/feed")
	benKuhn := "https://www.benkuhn.net/"
	hardcodedBenKuhn, _ = ToCanonicalLink(benKuhn, logger, nil)
	hardcodedBenKuhnArchives = hardcodedMustParse(benKuhn + "all/")
	caseyHandmer := "https://caseyhandmer.wordpress.com/"
	hardcodedCaseyHandmer = hardcodedMustParse(caseyHandmer)
	hardcodedCaseyHandmerSpaceMisconceptions, _ = ToCanonicalLink(
		caseyHandmer+"2019/08/17/blog-series-countering-misconceptions-in-space-journalism/",
		logger, nil,
	)
	hardcodedCaseyHandmerMarsTrilogy, _ = ToCanonicalLink(
		caseyHandmer+"2022/12/13/mars-trilogy-technical-commentary/",
		logger, nil,
	)
	hardcodedCaseyHandmerMarsTrilogyFirstLink, _ = ToCanonicalLink(
		caseyHandmer+"2022/12/13/mars-trilogy-festival-night/",
		logger, nil,
	)
	hardcodedCaseyHandmerFutureOfEnergy, _ = ToCanonicalLink(
		caseyHandmer+"2023/10/19/future-of-energy-reading-list/",
		logger, nil,
	)
	hardcodedCaseyHandmerBookReviews, _ = ToCanonicalLink(
		caseyHandmer+"2020/07/26/book-reviews/",
		logger, nil,
	)
	danLuu := "https://danluu.com"
	hardcodedDanLuu = hardcodedMustParse(danLuu)
	HardcodedDanLuuFeed = hardcodedMustParse(danLuu + "/atom.xml")
	HardcodedDontWorryAboutTheVaseFeed = hardcodedMustParse("https://thezvi.substack.com/feed")
	cryptographyEngineering := "https://blog.cryptographyengineering.com"
	hardcodedCryptographyEngineering = hardcodedMustParse(cryptographyEngineering)
	hardcodedCryptographyEngineeringAll = hardcodedMustParse(cryptographyEngineering + "/all-posts/")
	hardcodedFactorio = hardcodedMustParse("https://www.factorio.com/blog/")
	hardcodedJuliaEvans = hardcodedMustParse("https://jvns.ca")
	hardcodedKalzumeus = hardcodedMustParse("https://www.kalzumeus.com/archive/")
	hardcodedMrMoneyMustache = hardcodedMustParse("https://www.mrmoneymustache.com/blog")
	HardcodedOvercomingBiasFeed = hardcodedMustParse("https://www.overcomingbias.com/feed")
	hardcodedPaulGraham = hardcodedMustParse("https://paulgraham.com/articles.html")
	HardcodedSlateStarCodexFeed = "https://slatestarcodex.com/feed/"
}

func hardcodedMustParse(url string) CanonicalUri {
	uri, err := neturl.Parse(url)
	if err != nil {
		panic(err)
	}
	return CanonicalUriFromUri(uri)
}

var hardcodedCryptographyEngineeringCategories []HistoricalBlogPostCategory
var hardcodedKalzumeusCategories []HistoricalBlogPostCategory
var hardcodedMrMoneyMustacheCategories []HistoricalBlogPostCategory
var hardcodedPaulGrahamCategories []HistoricalBlogPostCategory

func init() {
	logger := NewDummyLogger()

	makeTopCategory := func(name string, topUrls []string) []HistoricalBlogPostCategory {
		topLinks := make([]Link, len(topUrls))
		for i, url := range topUrls {
			link, ok := ToCanonicalLink(url, logger, nil)
			if !ok {
				panic(fmt.Errorf("Couldn't parse link: %s", url))
			}
			topLinks[i] = *link
		}
		category := HistoricalBlogPostCategory{
			Name:      name,
			IsTop:     true,
			PostLinks: topLinks,
		}
		return []HistoricalBlogPostCategory{category}
	}

	cryptographyEngineeringTopUrls := []string{
		"https://blog.cryptographyengineering.com/2015/03/03/attack-of-week-freak-or-factoring-nsa/",
		"https://blog.cryptographyengineering.com/2016/03/21/attack-of-week-apple-imessage/",
		"https://blog.cryptographyengineering.com/2014/04/24/attack-of-week-triple-handshakes-3shake/",
		"https://blog.cryptographyengineering.com/2012/10/27/attack-of-week-cross-vm-timing-attacks/",
		"https://blog.cryptographyengineering.com/2013/09/06/on-nsa/",
		"https://blog.cryptographyengineering.com/2015/12/22/on-juniper-backdoor/",
		"https://blog.cryptographyengineering.com/2015/05/22/attack-of-week-logjam/",
		"https://blog.cryptographyengineering.com/2013/12/03/how-does-nsa-break-ssl/",
		"https://blog.cryptographyengineering.com/2014/12/29/on-new-snowden-documents/",
		"https://blog.cryptographyengineering.com/2013/09/18/the-many-flaws-of-dualecdrbg/",
		"https://blog.cryptographyengineering.com/2013/09/20/rsa-warns-developers-against-its-own/",
		"https://blog.cryptographyengineering.com/2013/12/28/a-few-more-notes-on-nsa-random-number/",
		"https://blog.cryptographyengineering.com/2015/01/14/hopefully-last-post-ill-ever-write-on/",
		"https://blog.cryptographyengineering.com/2014/11/27/zero-knowledge-proofs-illustrated-primer/",
		"https://blog.cryptographyengineering.com/2011/09/29/what-is-random-oracle-model-and-why-3/",
		"https://blog.cryptographyengineering.com/2011/10/08/what-is-random-oracle-model-and-why-2/",
		"https://blog.cryptographyengineering.com/2011/10/20/what-is-random-oracle-model-and-why_20/",
		"https://blog.cryptographyengineering.com/2011/11/02/what-is-random-oracle-model-and-why/",
		"https://blog.cryptographyengineering.com/2016/06/15/what-is-differential-privacy/",
		"https://blog.cryptographyengineering.com/2014/02/21/cryptographic-obfuscation-and/",
		"https://blog.cryptographyengineering.com/2013/04/11/wonkery-mailbag-ideal-ciphers/",
		"https://blog.cryptographyengineering.com/2014/10/04/why-cant-apple-decrypt-your-iphone/",
		"https://blog.cryptographyengineering.com/2014/08/13/whats-matter-with-pgp/",
		"https://blog.cryptographyengineering.com/2015/08/16/the-network-is-hostile/",
		"https://blog.cryptographyengineering.com/2012/02/28/how-to-fix-internet/",
		"https://blog.cryptographyengineering.com/2012/02/21/random-number-generation-illustrated/",
		"https://blog.cryptographyengineering.com/2012/03/09/surviving-bad-rng/",
		"https://blog.cryptographyengineering.com/2015/04/02/truecrypt-report/",
		"https://blog.cryptographyengineering.com/2013/10/14/lets-audit-truecrypt/",
	}
	hardcodedCryptographyEngineeringCategories = makeTopCategory("Top posts", cryptographyEngineeringTopUrls)

	kalzumeusTopUrls := []string{
		"https://www.kalzumeus.com/2019/3/18/two-years-at-stripe/",
		"https://www.kalzumeus.com/2018/10/19/japanese-hometown-tax/",
		"https://www.kalzumeus.com/2017/09/09/identity-theft-credit-reports/",
		"https://www.kalzumeus.com/2012/01/23/salary-negotiation/",
		"https://www.kalzumeus.com/2011/10/28/dont-call-yourself-a-programmer/",
		"https://www.kalzumeus.com/2011/03/13/some-perspective-on-the-japan-earthquake/",
		"https://www.kalzumeus.com/2010/08/25/the-hardest-adjustment-to-self-employment/",
		"https://www.kalzumeus.com/2010/06/17/falsehoods-programmers-believe-about-names/",
		"https://www.kalzumeus.com/2010/04/20/building-highly-reliable-websites-for-small-companies/",
		"https://www.kalzumeus.com/2010/03/20/running-a-software-business-on-5-hours-a-week/",
		"https://www.kalzumeus.com/2010/01/24/startup-seo/",
		"https://www.kalzumeus.com/2009/09/05/desktop-aps-versus-web-apps/",
		"https://www.kalzumeus.com/2009/03/07/how-to-successfully-compete-with-open-source-software/",
	}
	hardcodedKalzumeusCategories = makeTopCategory("Most popular", kalzumeusTopUrls)

	mrMoneyMustacheStartHereUrls := []string{
		"https://www.mrmoneymustache.com/2011/04/06/meet-mr-money-mustache/",
		"https://www.mrmoneymustache.com/2012/06/01/raising-a-family-on-under-2000-per-year/",
		"https://www.mrmoneymustache.com/2011/09/15/a-brief-history-of-the-stash-how-we-saved-from-zero-to-retirement-in-ten-years/",
		"https://www.mrmoneymustache.com/2011/05/12/the-coffee-machine-that-can-pay-for-a-university-education/",
		"https://www.mrmoneymustache.com/2012/01/13/the-shockingly-simple-math-behind-early-retirement/",
		"https://www.mrmoneymustache.com/2011/10/02/what-is-stoicism-and-how-can-it-turn-your-life-to-solid-gold/",
		"https://www.mrmoneymustache.com/2012/09/18/is-it-convenient-would-i-enjoy-it-wrong-question/",
		"https://www.mrmoneymustache.com/2012/10/08/how-to-go-from-middle-class-to-kickass/",
		"https://www.mrmoneymustache.com/2012/03/07/frugality-the-new-fanciness/",
		"https://www.mrmoneymustache.com/2012/04/18/news-flash-your-debt-is-an-emergency/",
		"https://www.mrmoneymustache.com/2011/10/06/the-true-cost-of-commuting/",
		"https://www.mrmoneymustache.com/2011/09/28/get-rich-with-moving-to-a-better-place/",
		"https://www.mrmoneymustache.com/2011/11/28/new-cars-and-auto-financing-stupid-or-sensible/",
		"https://www.mrmoneymustache.com/2012/03/19/top-10-cars-for-smart-people/",
		"https://www.mrmoneymustache.com/2011/04/18/get-rich-with-bikes/",
		"https://www.mrmoneymustache.com/2011/05/06/mmm-challenge-cut-your-cash-leaking-umbilical-cord/",
		"https://www.mrmoneymustache.com/2012/03/29/killing-your-1000-grocery-bill/",
		"https://www.mrmoneymustache.com/2011/10/12/avoiding-ivy-league-preschool-syndrome/",
		"https://www.mrmoneymustache.com/2011/12/05/muscle-over-motor/",
		"https://www.mrmoneymustache.com/2012/10/03/the-practical-benefits-of-outrageous-optimism/",
		"https://www.mrmoneymustache.com/2011/05/18/how-to-make-money-in-the-stock-market/",
		"https://www.mrmoneymustache.com/2012/05/29/how-much-do-i-need-for-retirement/",
		"https://www.mrmoneymustache.com/2012/06/07/safety-is-an-expensive-illusion/",
	}
	hardcodedMrMoneyMustacheCategories = makeTopCategory("Start Here", mrMoneyMustacheStartHereUrls)

	paulGrahamTopUrls := []string{
		"https://paulgraham.com/hs.html",
		"https://paulgraham.com/essay.html",
		"https://paulgraham.com/marginal.html",
		"https://paulgraham.com/jessica.html",
		"https://paulgraham.com/lies.html",
		"https://paulgraham.com/wisdom.html",
		"https://paulgraham.com/wealth.html",
		"https://paulgraham.com/re.html",
		"https://paulgraham.com/say.html",
		"https://paulgraham.com/makersschedule.html",
		"https://paulgraham.com/ds.html",
		"https://paulgraham.com/vb.html",
		"https://paulgraham.com/love.html",
		"https://paulgraham.com/growth.html",
		"https://paulgraham.com/startupideas.html",
		"https://paulgraham.com/mean.html",
		"https://paulgraham.com/kids.html",
		"https://paulgraham.com/lesson.html",
		"https://paulgraham.com/hwh.html",
		"https://paulgraham.com/think.html",
		"https://paulgraham.com/worked.html",
		"https://paulgraham.com/heresy.html",
		"https://paulgraham.com/newideas.html",
		"https://paulgraham.com/useful.html",
		"https://paulgraham.com/richnow.html",
		"https://paulgraham.com/cred.html",
		"https://paulgraham.com/own.html",
		"https://paulgraham.com/smart.html",
		"https://paulgraham.com/wtax.html",
		"https://paulgraham.com/conformism.html",
		"https://paulgraham.com/orth.html",
		"https://paulgraham.com/noob.html",
		"https://paulgraham.com/early.html",
		"https://paulgraham.com/ace.html",
		"https://paulgraham.com/simply.html",
		"https://paulgraham.com/fn.html",
		"https://paulgraham.com/earnest.html",
		"https://paulgraham.com/genius.html",
		"https://paulgraham.com/work.html",
		"https://paulgraham.com/before.html",
		"https://paulgraham.com/greatwork.html",
		"https://paulgraham.com/cities.html",
	}

	hardcodedPaulGrahamCategories = makeTopCategory("Top", paulGrahamTopUrls)
}

var acoupNonArticleRegex *regexp.Regexp

func init() {
	acoupNonArticleRegex = regexp.MustCompile(`/\d+/\d+/\d+/(gap-week|fireside)-`)
}

func extractACOUPCategories(postLinks []*maybeTitledLink) ([]HistoricalBlogPostCategory, error) {
	var articlesLinks []Link
	for _, link := range postLinks {
		if !acoupNonArticleRegex.MatchString(link.Curi.Path) {
			articlesLinks = append(articlesLinks, link.Link)
		}
	}
	if len(articlesLinks) == 0 {
		return nil, oops.Newf("ACOUP categories not found")
	}
	return []HistoricalBlogPostCategory{{
		Name:      "Articles",
		IsTop:     true,
		PostLinks: articlesLinks,
	}}, nil
}

func ExtractACXCategories(postLink *FeedEntryLink, logger log.Logger) []string {
	path := postLink.Curi.Path
	var categories []string
	if strings.Contains(path, "open-thread") {
		categories = append(categories, "Open Threads")
	}
	if strings.Contains(path, "book-review") {
		categories = append(categories, "Book Reviews")
	}
	if strings.Contains(path, "mantic-monday") {
		categories = append(categories, "Mantic Mondays")
	}
	if strings.Contains(path, "links-for-") {
		categories = append(categories, "Links")
	}
	if strings.Contains(path, "meetup") {
		categories = append(categories, "Meetups")
	}
	if len(categories) == 0 {
		// Everything that can show up in the feed from now on is 2023+
		categories = append(categories, "Articles 2023+")

		categories = append(categories, "Articles")
	}
	if postLink.MaybeDate == nil {
		logger.Error().Msgf("ACX feed entry doesn't have date: %s", postLink.Url)
		return categories
	}
	yearStr := fmt.Sprint(postLink.MaybeDate.Year())
	categories = append(categories, yearStr)
	return categories
}

func extractBenKuhnCategories(mainPage *htmlPage, logger Logger) ([]HistoricalBlogPostCategory, error) {
	essaysElements := htmlquery.Find(mainPage.Document, "/html/body/div[1]/div[1]/div[*]/h2/a")
	essaysLinks := make([]Link, len(essaysElements))
	for i, element := range essaysElements {
		href := findAttr(element, "href")
		link, ok := ToCanonicalLink(href, logger, mainPage.FetchUri)
		if !ok {
			return nil, oops.Newf("Ben Kuhn categories bad link: %s", href)
		}
		essaysLinks[i] = *link
	}

	return []HistoricalBlogPostCategory{{
		Name:      "Essays",
		IsTop:     true,
		PostLinks: essaysLinks,
	}}, nil
}

func extractCaseyHandmerCategories(
	spaceMisconceptionsPage, marsTrilogyPage, futureOfEnergyPage *htmlPage, links []*maybeTitledLink,
	curiEqCfg *CanonicalEqualityConfig, logger Logger,
) ([]HistoricalBlogPostCategory, error) {
	logger.Info("Extracting Casey Handmer categories")

	spaceMisconceptionsElements := htmlquery.Find(
		spaceMisconceptionsPage.Document,
		"/html/body/div[1]/div/div/div[1]/main/article/div[1]/ul[*]/li[*]/a",
	)
	var spaceMisconceptionsLinks []Link
	for _, element := range spaceMisconceptionsElements {
		href := findAttr(element, "href")
		if strings.HasSuffix(href, ".pdf") {
			continue
		}
		link, ok := ToCanonicalLink(href, logger, spaceMisconceptionsPage.FetchUri)
		if !ok {
			return nil, oops.Newf("Casey Handmer categories bad link: %q", href)
		}
		spaceMisconceptionsLinks = append(spaceMisconceptionsLinks, *link)
	}
	if len(spaceMisconceptionsLinks) == 0 {
		return nil, oops.Newf("Casey Handmer space misconceptions category not found")
	}

	marsTrilogyElements := htmlquery.Find(
		marsTrilogyPage.Document,
		"/html/body/div[1]/div/div/div[1]/main/article/div[1]/p[*]/a",
	)
	var marsTrilogyLinks []Link
	seenFirstLink := false
	for _, element := range marsTrilogyElements {
		href := findAttr(element, "href")
		if strings.HasSuffix(href, ".pdf") {
			continue
		}
		link, ok := ToCanonicalLink(href, logger, marsTrilogyPage.FetchUri)
		if !ok {
			return nil, oops.Newf("Casey Handmer categories bad link: %q", href)
		}
		if CanonicalUriEqual(link.Curi, hardcodedCaseyHandmerMarsTrilogyFirstLink.Curi, curiEqCfg) {
			seenFirstLink = true
		}
		if seenFirstLink {
			marsTrilogyLinks = append(marsTrilogyLinks, *link)
		}
	}
	if len(marsTrilogyLinks) == 0 {
		return nil, oops.Newf("Casey Handmer mars trilogy category not found")
	}

	futureOfEnergyElements := htmlquery.Find(
		futureOfEnergyPage.Document,
		"/html/body/div[1]/div/div/div[1]/main/article/div[1]//a",
	)
	var futureOfEnergyLinks []Link
	for _, element := range futureOfEnergyElements {
		href := findAttr(element, "href")
		if strings.HasSuffix(href, ".pdf") {
			continue
		}
		link, ok := ToCanonicalLink(href, logger, futureOfEnergyPage.FetchUri)
		if !ok {
			return nil, oops.Newf("Casey Handmer categories bad link: %q", href)
		}
		if link.Curi.Host == hardcodedCaseyHandmer.Host &&
			!CanonicalUriEqual(link.Curi, hardcodedCaseyHandmerBookReviews.Curi, curiEqCfg) {

			futureOfEnergyLinks = append(futureOfEnergyLinks, *link)
		}
	}
	if len(futureOfEnergyLinks) == 0 {
		return nil, oops.Newf("Casey Handmer future of energy category not found")
	}

	categorizedLinksLists := [][]Link{spaceMisconceptionsLinks, marsTrilogyLinks, futureOfEnergyLinks}
	categorizedLinksSet := NewCanonicalUriSet(nil, curiEqCfg)
	for _, categorizedLinks := range categorizedLinksLists {
		for _, link := range categorizedLinks {
			categorizedLinksSet.add(link.Curi)
		}
	}
	var uncategorizedLinks []Link
	for _, link := range links {
		if !categorizedLinksSet.Contains(link.Curi) {
			uncategorizedLinks = append(uncategorizedLinks, link.Link)
		}
	}

	return []HistoricalBlogPostCategory{
		{
			Name:      "Top",
			IsTop:     true,
			PostLinks: spaceMisconceptionsLinks,
		},
		{
			Name:      "Countering misconceptions in space journalism",
			IsTop:     false,
			PostLinks: spaceMisconceptionsLinks,
		},
		{
			Name:      "Mars Trilogy Technical Commentary",
			IsTop:     false,
			PostLinks: marsTrilogyLinks,
		},
		{
			Name:      "Future of Energy Reading List",
			IsTop:     false,
			PostLinks: futureOfEnergyLinks,
		},
		{
			Name:      "Uncategorized",
			IsTop:     false,
			PostLinks: uncategorizedLinks,
		},
	}, nil
}

func ExtractDontWorryAboutTheVaseCategories(postLink *FeedEntryLink, logger log.Logger) []string {
	path := postLink.Curi.Path
	// Everything that can show up in the feed from now on is 2023+
	// Also assuming covid is over
	categories := []string{"2023+", "Non-Covid"}
	dontWorryAboutTheVaseAIRegex := regexp.MustCompile("^/p/ai-[0-9]")
	if dontWorryAboutTheVaseAIRegex.MatchString(path) {
		categories = append(categories, "AI Series")
	}
	if postLink.MaybeDate == nil {
		logger.Error().Msgf("Don't Worry About The Vase feed entry doesn't have date: %s", postLink.Url)
		return categories
	}
	yearStr := fmt.Sprint(postLink.MaybeDate.Year())
	categories = append(categories, yearStr)
	return categories
}

func extractFactorioCategories(postLinks []*maybeTitledLink) ([]HistoricalBlogPostCategory, error) {
	var fffLinks []Link
	for _, link := range postLinks {
		if strings.HasPrefix(link.Curi.Path, "/blog/post/fff-") {
			fffLinks = append(fffLinks, link.Link)
		}
	}
	if len(fffLinks) == 0 {
		return nil, oops.Newf("Factorio categories not found")
	}
	return []HistoricalBlogPostCategory{{
		Name:      "Friday Facts",
		IsTop:     true,
		PostLinks: fffLinks,
	}}, nil
}

func extractJuliaEvansCategories(page *htmlPage, logger Logger) ([]HistoricalBlogPostCategory, error) {
	logger.Info("Extracting Julia Evans categories")

	headings := htmlquery.Find(page.Document, "//article/a/h3")
	if len(headings) == 0 {
		return nil, oops.Newf("Julia Evans categories not found")
	}

	categories := make([]HistoricalBlogPostCategory, 1+len(headings))
	for categoryIdx, heading := range headings {
		categoryName := innerText(heading)
		postLinkElements := htmlquery.Find(heading.Parent.NextSibling, ".//a")
		postLinks := make([]Link, len(postLinkElements))
		for i, element := range postLinkElements {
			href := findAttr(element, "href")
			link, ok := ToCanonicalLink(href, logger, page.FetchUri)
			if !ok {
				return nil, oops.Newf("Julia Evans categories bad link: %q", href)
			}
			postLinks[i] = *link
		}

		categories[1+categoryIdx] = HistoricalBlogPostCategory{
			Name:      categoryName,
			IsTop:     false,
			PostLinks: postLinks,
		}
	}

	var postLinksExceptRC []Link
	for _, category := range categories {
		if strings.Contains(category.Name, "Recurse center") || category.Name == "Conferences" {
			continue
		}
		postLinksExceptRC = append(postLinksExceptRC, category.PostLinks...)
	}
	categories[0] = HistoricalBlogPostCategory{
		Name:      "Blog posts",
		IsTop:     true,
		PostLinks: postLinksExceptRC,
	}

	return categories, nil
}

func ExtractOvercomingBiasCategories(postLink *FeedEntryLink, logger log.Logger) []string {
	// Everything that can show up in the feed from now on is 2023+
	categories := []string{"2023+"}
	if postLink.MaybeDate == nil {
		logger.Error().Msgf("Don't Worry About The Vase feed entry doesn't have date: %s", postLink.Url)
		return categories
	}
	yearStr := fmt.Sprint(postLink.MaybeDate.Year())
	categories = append(categories, yearStr)
	return categories
}

var lockXPath *xpath.Expr

func init() {
	// Will also match parts of a word, but should be good enough and otherwise the test will catch it
	lockXPath = xpath.MustCompile(`//*[contains(@class, "lock")]`)
}

func extractSubstackCategories(
	htmlLinks []*maybeTitledHtmlLink, distanceToTopParent int,
) []HistoricalBlogPostCategory {
	publicCategory := HistoricalBlogPostCategory{
		Name:      "Public",
		IsTop:     true,
		PostLinks: []Link{},
	}
	for _, htmlLink := range htmlLinks {
		topParent := htmlLink.Element
		for i := 0; i < distanceToTopParent; i++ {
			topParent = topParent.Parent
		}
		lockNode := htmlquery.QuerySelector(topParent, lockXPath)
		if lockNode == nil {
			publicCategory.PostLinks = append(publicCategory.PostLinks, htmlLink.Link)
		}
	}
	return []HistoricalBlogPostCategory{publicCategory}
}

func generatePgFeed(
	rootLink *Link, page *htmlPage, crawlCtx *CrawlContext, curiEqCfg *CanonicalEqualityConfig,
	logger Logger,
) DiscoverFeedsResult {
	pageAllLinks := extractLinks(
		page.Document, page.FetchUri, nil, crawlCtx.Redirects, logger, includeXPathOnly,
	)
	fakeFeedEntryUrls := []string{
		"https://paulgraham.com/icad.html",
		"https://paulgraham.com/power.html",
		"https://paulgraham.com/fix.html",
	}
	fakeFeedEntryTitles := []string{
		"Revenge of the Nerds",
		"Succinctness is Power",
		"What Languages Fix",
	}
	var linkBuckets [][]FeedEntryLink
	feedEntryCurisTitlesMap := NewCanonicalUriMap[*LinkTitle](curiEqCfg)
	for i, entryUrl := range fakeFeedEntryUrls {
		entryLink, _ := ToCanonicalLink(entryUrl, logger, page.FetchUri)
		linkBuckets = append(linkBuckets, []FeedEntryLink{
			FeedEntryLink{
				maybeTitledLink: maybeTitledLink{
					Link:       *entryLink,
					MaybeTitle: nil,
				},
				MaybeDate: nil,
			},
		})
		title := NewLinkTitle(fakeFeedEntryTitles[i], LinkTitleSourceFeed, nil)
		feedEntryCurisTitlesMap.Add(*entryLink, &title)
	}
	feedEntryLinks := FeedEntryLinks{
		LinkBuckets:    linkBuckets,
		Length:         3,
		IsOrderCertain: true,
	}
	extractionsByStarCount := getExtractionsByStarCount(
		pageAllLinks, "", &feedEntryLinks, &feedEntryCurisTitlesMap, curiEqCfg, 0, logger,
	)
	if len(extractionsByStarCount[0].Extractions) != 1 {
		logger.Error(
			"Couldn't parse PG essays: %d extractions", len(extractionsByStarCount[0].Extractions),
		)
		return &DiscoverFeedsErrorBadFeed{}
	}
	entryLinks := extractionsByStarCount[0].Extractions[0].LinksExtraction.Links
	feedTitle := "Paul Graham: Essays"
	var feedSb strings.Builder
	fmt.Fprintln(&feedSb, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(&feedSb, `<rss xmlns:atom="http://www.w3.org/2005/Atom" xmlns:content="http://purl.org/rss/1.0/modules/content/" version="2.0">`)
	fmt.Fprintln(&feedSb, `  <channel>`)
	fmt.Fprintln(&feedSb, `    <atom:link href="http://www.paulgraham.com/articles.html" rel="self" type="application/rss+xml"/>`)
	fmt.Fprintf(&feedSb, "    <title>%s</title>\n", feedTitle)
	fmt.Fprintf(&feedSb, "    <link>%s</link>\n", rootLink)
	for _, link := range entryLinks {
		fmt.Fprint(&feedSb, "    <item>\n")
		fmt.Fprint(&feedSb, "      <title>")
		_ = xml.EscapeText(&feedSb, []byte(link.MaybeTitle.Value))
		fmt.Fprint(&feedSb, "</title>\n")
		fmt.Fprint(&feedSb, "      <link>")
		_ = xml.EscapeText(&feedSb, []byte(link.Url))
		fmt.Fprint(&feedSb, "</link>\n")
		fmt.Fprint(&feedSb, "    </item>\n")
	}
	fmt.Fprintln(&feedSb, `  </channel>`)
	fmt.Fprintln(&feedSb, `</rss>`)
	feedContent := feedSb.String()
	parsedFeed, err := ParseFeed(feedContent, rootLink.Uri, logger)
	if err != nil {
		logger.Error("Couldn't parse PG essays feed we just created: %v", err)
		return &DiscoverFeedsErrorBadFeed{}
	}

	return &DiscoveredSingleFeed{
		MaybeStartPage: &DiscoveredStartPage{
			Url:      rootLink.Url,
			FinalUrl: rootLink.Url,
			Content:  page.Content,
		},
		Feed: DiscoveredFetchedFeed{
			Title:      feedTitle,
			Url:        rootLink.Url,
			FinalUrl:   rootLink.Url,
			Content:    feedContent,
			ParsedFeed: parsedFeed,
		},
	}
}
