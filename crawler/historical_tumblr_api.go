package crawler

import (
	"feedrewind/oops"
	"fmt"
	"math"
	neturl "net/url"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"
)

func getTumblrApiHistorical(
	hostname string, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Get Tumblr historical start")
	apiKey := "REDACTED_TUMBLR_API_KEY"

	var links []*maybeTitledLink
	var timestamps []int64
	url := fmt.Sprintf("https://api.tumblr.com/v2/blog/%s/posts?api_key=%s", hostname, apiKey)
	var blogLink *Link
	var blogTitle string
	var expectedCount int
	categoriesByName := make(map[string]*HistoricalBlogPostCategory)
	for {
		uri, err := neturl.Parse(url)
		if err != nil {
			return nil, oops.Wrap(err)
		}

		requestStart := time.Now()
		resp, err := crawlCtx.HttpClient.Request(uri, true, crawlCtx.RobotsClient, logger)
		if err != nil {
			return nil, err
		}
		requestMs := time.Since(requestStart).Milliseconds()
		crawlCtx.RequestsMade++
		progressLogger.LogHtml()
		logger.Info("%s %dms %s", resp.Code, requestMs, url)

		if resp.Code != "200" {
			return nil, oops.New("Tumblr error")
		}

		type TumblrResponse struct {
			Response struct {
				Blog struct {
					Url   string
					Title *string
				}
				Posts []struct {
					Post_Url  string
					Title     *string
					Summary   string
					Timestamp int64
					Tags      []string
				}
				Total_Posts *int
				Links       struct {
					Next struct {
						Href string
					}
				} `json:"_links"`
			}
		}

		var tumblrResp TumblrResponse
		err = json.Unmarshal(resp.Body, &tumblrResp)
		if err != nil {
			return nil, oops.Wrap(err)
		}

		if len(tumblrResp.Response.Posts) == 0 {
			return nil, oops.New("No posts in Tumblr response")
		}

		if blogLink == nil {
			blogUrl := tumblrResp.Response.Blog.Url
			if blogUrl == "" {
				return nil, oops.New("No blog url in Tumblr response")
			}

			var ok bool
			blogLink, ok = ToCanonicalLink(blogUrl, logger, nil)
			if !ok {
				return nil, oops.Newf("Couldn't parse Tumblr blog link: %s", blogUrl)
			}

			maybeTitle := tumblrResp.Response.Blog.Title
			if maybeTitle == nil {
				return nil, oops.New("No blog title in Tumblr response")
			}
			blogTitle = *maybeTitle

			totalPosts := tumblrResp.Response.Total_Posts
			if totalPosts == nil {
				return nil, oops.New("No posts count in Tumblr response")
			}
			expectedCount = *totalPosts
		}

		for _, post := range tumblrResp.Response.Posts {
			postUrl := post.Post_Url
			postTitle := post.Summary
			if post.Title != nil {
				postTitle = *post.Title
			}
			normalizedPostTitle := normalizeTitle(postTitle)
			if normalizedPostTitle == "" {
				normalizedPostTitle = normalizeTitle(blogTitle)
			}
			postLink, ok := ToCanonicalLink(postUrl, logger, nil)
			if !ok {
				return nil, oops.Newf("Couldn't parse Tumble post link: %s", postUrl)
			}
			linkTitle := NewLinkTitle(normalizedPostTitle, LinkTitleSourceTumblr, nil)
			links = append(links, &maybeTitledLink{
				Link:       *postLink,
				MaybeTitle: &linkTitle,
			})
			timestamps = append(timestamps, post.Timestamp)
			for _, tag := range post.Tags {
				tagLower := strings.ToLower(tag)
				if _, ok := categoriesByName[tagLower]; !ok {
					categoriesByName[tagLower] = &HistoricalBlogPostCategory{
						Name:      tag,
						IsTop:     false,
						PostLinks: nil,
					}
				}
				categoriesByName[tagLower].PostLinks = append(categoriesByName[tagLower].PostLinks, *postLink)
			}
			if len(post.Tags) == 0 {
				uncategorizedLower := strings.ToLower(uncategorized)
				if _, ok := categoriesByName[uncategorizedLower]; !ok {
					categoriesByName[uncategorizedLower] = &HistoricalBlogPostCategory{
						Name:      uncategorized,
						IsTop:     false,
						PostLinks: nil,
					}
				}
				categoriesByName[uncategorizedLower].PostLinks =
					append(categoriesByName[uncategorizedLower].PostLinks, *postLink)
			}
		}

		requestsRemaining := int(math.Ceil(float64(expectedCount-len(links)) / 20))
		progressLogger.LogAndSavePostprocessingCounts(len(links), requestsRemaining)

		nextUrl := tumblrResp.Response.Links.Next.Href
		if nextUrl == "" {
			break
		}

		url = fmt.Sprintf("https://api.tumblr.com%s&api_key=%s", nextUrl, apiKey)
	}

	areTimestampsSorted := true
	for i := 0; i < len(timestamps)-1; i++ {
		if timestamps[i] <= timestamps[i+1] {
			areTimestampsSorted = false
			break
		}
	}
	if !areTimestampsSorted {
		return nil, oops.Newf("Tumblr posts are not sorted: %v", timestamps)
	}

	categories := make([]HistoricalBlogPostCategory, 0, len(categoriesByName))
	for _, category := range categoriesByName {
		categories = append(categories, *category)
	}
	slices.SortFunc(categories, func(a, b HistoricalBlogPostCategory) int {
		count1 := len(a.PostLinks)
		count2 := len(b.PostLinks)
		if count1 != count2 {
			return count2 - count1 // descending
		}
		return strings.Compare(a.Name, b.Name)
	})
	logger.Info("Categories: %s", categoryCountsString(categories))

	logger.Info("Get Tumblr historical finish")
	return &postprocessedResult{
		MainLnk:                 *blogLink,
		Pattern:                 "tumblr",
		Links:                   links,
		IsMatchingFeed:          true,
		PostCategories:          categories,
		Extra:                   nil,
		MaybePartialPagedResult: nil,
	}, nil
}
