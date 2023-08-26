package publish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"fmt"
	"sort"
	"time"
)

const defaultPostsInRss = 30

func InitSubscription(
	tx pgw.Queryable, userId models.UserId, productUserId models.ProductUserId,
	subscriptionId models.SubscriptionId, subscriptionName string, blogBestUrl string,
	deliveryChannel models.DeliveryChannel, shouldPublishRssPosts bool, utcNow time.Time, localTime time.Time,
	localDate util.Date,
) error {
	return initSubscriptionImpl(
		tx, userId, productUserId, subscriptionId, subscriptionName, blogBestUrl, deliveryChannel,
		shouldPublishRssPosts, utcNow, localTime, localDate, defaultPostsInRss,
	)
}

func PublishForUser(
	tx pgw.Queryable, userId models.UserId, productUserId models.ProductUserId,
	deliveryChannel models.DeliveryChannel, utcNow time.Time, localTime time.Time, localDate util.Date,
	scheduledFor string,
) error {
	return publishForUserImpl(
		tx, userId, productUserId, deliveryChannel, utcNow, localTime, localDate, scheduledFor,
		defaultPostsInRss,
	)
}

func initSubscriptionImpl(
	tx pgw.Queryable, userId models.UserId, productUserId models.ProductUserId,
	subscriptionId models.SubscriptionId, subscriptionName string, blogBestUrl string,
	deliveryChannel models.DeliveryChannel, shouldPublishRssPosts bool, utcNow time.Time, localTime time.Time,
	localDate util.Date, postsInRss int,
) error {
	subscriptions, err := models.Subscription_ListSortedToPublish(tx, userId)
	if err != nil {
		return err
	}

	var subscription *models.SubscriptionToPublish
	for i := range subscriptions {
		if subscriptions[i].Id == subscriptionId {
			subscription = &subscriptions[i]
		}
	}

	var publishStatus models.PostPublishStatus
	switch deliveryChannel {
	case models.DeliveryChannelSingleFeed, models.DeliveryChannelMultipleFeeds:
		publishStatus = models.PostPublishStatusRssPublished
		var newPosts []models.SubscriptionBlogPost
		if shouldPublishRssPosts {
			dayOfWeek := util.Schedule_DayOfWeek(localTime)
			dayCount, err := models.Schedule_GetCount(tx, subscriptionId, dayOfWeek)
			if err != nil {
				return err
			}

			newPosts, err = models.SubscriptionPost_GetNextUnpublished(tx, subscriptionId, dayCount)
			if err != nil {
				return err
			}
			for i := range newPosts {
				newPosts[i].PublishedAt = &utcNow
			}
			log.Info().Msgf("Subscription %d: will publish %d new posts", subscriptionId, len(newPosts))

			unpublishedCount, err := models.SubscriptionPost_GetUnpublishedCount(tx, subscriptionId)
			if err != nil {
				return err
			}
			if subscription.FinalItemPublishedAt == nil && unpublishedCount == len(newPosts) {
				// So that publishRssFeeds() knows about the update too
				subscription.FinalItemPublishedAt = &utcNow

				err := models.Subscription_SetFinalItemPublished(tx, subscriptionId, utcNow, publishStatus)
				if err != nil {
					return err
				}
				log.Info().Msgf("Will publish the final item for subscription %d", subscriptionId)

				models.ProductEvent_MustEmit(tx, productUserId, "finish subscription", map[string]any{
					"subscription_id": subscriptionId,
					"blog_url":        blogBestUrl,
				}, nil)
			}
		}

		newPostsBySubscriptionId := map[models.SubscriptionId][]models.SubscriptionBlogPost{
			subscriptionId: newPosts,
		}

		err := publishRssFeeds(tx, userId, subscriptions, newPostsBySubscriptionId, postsInRss)
		if err != nil {
			return err
		}

		for _, post := range newPosts {
			err := models.SubscriptionPost_UpdatePublished(tx, post.Id, utcNow, localDate, publishStatus)
			if err != nil {
				return err
			}
		}
	case models.DeliveryChannelEmail:
		publishStatus = models.PostPublishStatusEmailPending
		scheduledFor, err := util.Schedule_ToUTCStr(utcNow)
		if err != nil {
			return err
		}
		// The job won't be visible until the transaction is committed
		err = jobs.EmailInitialItemJob_PerformNow(tx, userId, subscriptionId, scheduledFor)
		if err != nil {
			return err
		}

		err = createEmptySubscriptionFeed(tx, subscriptionId, subscriptionName)
		if err != nil {
			return err
		}
	default:
		panic(fmt.Errorf("Unknown delivery channel: %s", deliveryChannel))
	}

	err = models.Subscription_SetInitialItemPublishStatus(tx, subscriptionId, publishStatus)
	return err
}

func publishForUserImpl(
	tx pgw.Queryable, userId models.UserId, productUserId models.ProductUserId,
	deliveryChannel models.DeliveryChannel, utcNow time.Time, localTime time.Time, localDate util.Date,
	scheduledFor string, postsInRss int,
) error {
	subscriptions, err := models.Subscription_ListSortedToPublish(tx, userId)
	if err != nil {
		return err
	}
	log.Info().Msgf("%d subscriptions", len(subscriptions))

	newPostsBySubscriptionId := make(map[models.SubscriptionId][]models.SubscriptionBlogPost)
	var finalItemSubscriptionIds []models.SubscriptionId
	for i := range subscriptions {
		subscription := &subscriptions[i]

		var newPosts []models.SubscriptionBlogPost
		if !(subscription.IsPaused != nil && *subscription.IsPaused) {
			dayOfWeek := util.Schedule_DayOfWeek(localTime)
			dayCount, err := models.Schedule_GetCount(tx, subscription.Id, dayOfWeek)
			if err != nil {
				return err
			}

			newPosts, err = models.SubscriptionPost_GetNextUnpublished(tx, subscription.Id, dayCount)
			if err != nil {
				return err
			}
			for i := range newPosts {
				newPosts[i].PublishedAt = &utcNow
			}
			log.Info().Msgf("Subscription %d: will publish %d new posts", subscription.Id, len(newPosts))
		} else {
			log.Info().Msgf("Skipping subscription %d", subscription.Id)
		}
		newPostsBySubscriptionId[subscription.Id] = newPosts

		unpublishedCount, err := models.SubscriptionPost_GetUnpublishedCount(tx, subscription.Id)
		if err != nil {
			return err
		}
		if subscription.FinalItemPublishedAt == nil && unpublishedCount == len(newPosts) {
			// So that publishRssFeeds() knows about the update too
			subscription.FinalItemPublishedAt = &utcNow
			finalItemSubscriptionIds = append(finalItemSubscriptionIds, subscription.Id)
			log.Info().Msgf("Will publish the final item for subscription %d", subscription.Id)

			blogBestUrl, err := models.Blog_GetBestUrl(tx, subscription.BlogId)
			if err != nil {
				return err
			}
			models.ProductEvent_MustEmit(tx, productUserId, "finish subscription", map[string]any{
				"subscription_id": subscription.Id,
				"blog_url":        blogBestUrl,
			}, nil)
		}
	}

	var publishStatus models.PostPublishStatus
	switch deliveryChannel {
	case models.DeliveryChannelSingleFeed, models.DeliveryChannelMultipleFeeds:
		err := publishRssFeeds(tx, userId, subscriptions, newPostsBySubscriptionId, postsInRss)
		if err != nil {
			return err
		}
		publishStatus = models.PostPublishStatusRssPublished
	case models.DeliveryChannelEmail:
		// The job won't be visible until the transaction is committed
		err := jobs.EmailPostsJob_PerformNow(tx, userId, localDate, scheduledFor, finalItemSubscriptionIds)
		if err != nil {
			return err
		}
		publishStatus = models.PostPublishStatusEmailPending
	default:
		panic(fmt.Errorf("Unknown delivery channel for user %d: %s", userId, deliveryChannel))
	}

	for _, newPosts := range newPostsBySubscriptionId {
		for _, post := range newPosts {
			err := models.SubscriptionPost_UpdatePublished(tx, post.Id, utcNow, localDate, publishStatus)
			if err != nil {
				return err
			}
		}
	}
	for _, subscriptionId := range finalItemSubscriptionIds {
		err := models.Subscription_SetFinalItemPublished(tx, subscriptionId, utcNow, publishStatus)
		if err != nil {
			return err
		}
	}

	return nil
}

func publishRssFeeds(
	tx pgw.Queryable, userId models.UserId, subscriptions []models.SubscriptionToPublish,
	newPostsBySubscriptionId map[models.SubscriptionId][]models.SubscriptionBlogPost, postsInRss int,
) error {
	type userDateItem struct {
		PublishedAt time.Time
		Item        item
	}
	var userDatesItems []userDateItem
	for _, subscription := range subscriptions {
		log.Info().Msgf("Generating RSS for subscription %d", subscription.Id)
		newPosts := newPostsBySubscriptionId[subscription.Id]
		remainingPostsCount := postsInRss - len(newPosts)
		if subscription.FinalItemPublishedAt != nil {
			remainingPostsCount--
		}
		remainingPosts, err := models.SubscriptionPost_GetLastPublishedDesc(
			tx, subscription.Id, remainingPostsCount,
		)
		if err != nil {
			return err
		}
		reversePosts(remainingPosts)

		subscriptionUrl := rutil.SubscriptionUrl(subscription.Id)
		var subscriptionItems []item
		if subscription.FinalItemPublishedAt != nil {
			log.Info().Msgf("Generating final item for subscription %d", subscription.Id)
			finalItem := item{
				Title: fmt.Sprintf("You're all caught up with %s", subscription.Name),
				Link:  subscriptionUrl,
				Guid: guid{
					Guid:        makeGuid(fmt.Sprintf("%d-final", subscription.Id)),
					IsPermalink: false,
				},
				Description: `<a href="https://feedrewind.com/subscriptions/add">Want to read something else?</a>`,
				PubDate:     subscription.FinalItemPublishedAt.Format(time.RFC1123Z),
			}
			subscriptionItems = append(subscriptionItems, finalItem)
			userDatesItems = append(userDatesItems, userDateItem{
				PublishedAt: *subscription.FinalItemPublishedAt,
				Item:        finalItem,
			})
		}

		subscriptionPosts := make([]models.SubscriptionBlogPost, 0, len(newPosts)+len(remainingPosts))
		subscriptionPosts = append(subscriptionPosts, remainingPosts...)
		subscriptionPosts = append(subscriptionPosts, newPosts...)
		reversePosts(subscriptionPosts)

		for _, post := range subscriptionPosts {
			link := rutil.SubscriptionPostUrl(post.Title, post.RandomId)
			guidValue := makeGuid(fmt.Sprint(post.Id))
			pubDate := post.PublishedAt.Format(time.RFC1123Z)

			subscriptionItem := item{
				Title: post.Title,
				Link:  link,
				Guid: guid{
					Guid:        guidValue,
					IsPermalink: false,
				},
				Description: fmt.Sprintf(`<a href="%s">Manage</a>`, subscriptionUrl),
				PubDate:     pubDate,
			}
			subscriptionItems = append(subscriptionItems, subscriptionItem)

			userItemDescription := fmt.Sprintf(
				`from %s<br><br><a href="%s">Manage</a>`, subscription.Name, subscriptionUrl,
			)
			userItem := item{
				Title: post.Title,
				Link:  link,
				Guid: guid{
					Guid:        guidValue,
					IsPermalink: false,
				},
				Description: userItemDescription,
				PubDate:     pubDate,
			}
			userDatesItems = append(userDatesItems, userDateItem{
				PublishedAt: *post.PublishedAt,
				Item:        userItem,
			})
		}

		if len(subscriptionItems) < postsInRss {
			log.Info().Msgf("Generating initial item for subscription %d", subscription.Id)
			initialItem := item{
				Title: fmt.Sprintf("%s added to FeedRewind", subscription.Name),
				Link:  subscriptionUrl,
				Guid: guid{
					Guid:        makeGuid(fmt.Sprintf("%d-welcome", subscription.Id)),
					IsPermalink: false,
				},
				Description: fmt.Sprintf(`<a href="%s">Manage</a>`, subscriptionUrl),
				PubDate:     subscription.FinishedSetupAt.Format(time.RFC1123Z),
			}
			subscriptionItems = append(subscriptionItems, initialItem)
			userDatesItems = append(userDatesItems, userDateItem{
				PublishedAt: subscription.FinishedSetupAt,
				Item:        initialItem,
			})
		}

		subscriptionRssText, err := generateSubscriptionRss(
			subscription.Name, subscriptionUrl, subscriptionItems,
		)
		if err != nil {
			return err
		}
		log.Info().Msgf("Total items for subscription %d: %d", subscription.Id, len(subscriptionItems))

		err = models.SubscriptionRss_Upsert(tx, subscription.Id, subscriptionRssText)
		if err != nil {
			return err
		}
		log.Info().Msgf("Saved RSS for subscription %d", subscription.Id)
	}

	// Date desc, index asc (= publish date desc, sub date desc, post index desc)
	sort.SliceStable(userDatesItems, func(i, j int) bool {
		return userDatesItems[i].PublishedAt.After(userDatesItems[j].PublishedAt)
	})
	userItems := make([]item, 0, postsInRss)
	for i, userDateItem := range userDatesItems {
		if i >= postsInRss {
			break
		}
		userItems = append(userItems, userDateItem.Item)
	}

	newUserItemsCount := 0
	for _, newPosts := range newPostsBySubscriptionId {
		newUserItemsCount += len(newPosts)
	}
	log.Info().Msgf("Total items for user %d: %d (%d new)", userId, len(userItems), newUserItemsCount)
	userRssText, err := generateUserRss(userItems)
	if err != nil {
		return err
	}

	err = models.UserRss_Upsert(tx, userId, userRssText)
	if err != nil {
		return err
	}
	log.Info().Msgf("Saved RSS for user %d", userId)

	return nil
}

func reversePosts(posts []models.SubscriptionBlogPost) {
	for i, j := 0, len(posts)-1; i < j; i, j = i+1, j-1 {
		posts[i], posts[j] = posts[j], posts[i]
	}
}

func makeGuid(value string) string {
	hashBytes := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hashBytes[:])
}

func createEmptySubscriptionFeed(
	tx pgw.Queryable, subscriptionId models.SubscriptionId, subscriptionName string,
) error {
	subscriptionUrl := rutil.SubscriptionUrl(subscriptionId)
	subscriptionRssText, err := generateSubscriptionRss(subscriptionName, subscriptionUrl, nil)
	if err != nil {
		return err
	}
	err = models.SubscriptionRss_Upsert(tx, subscriptionId, subscriptionRssText)
	if err != nil {
		return err
	}
	log.Info().Msgf("Created empty RSS for subscription %d", subscriptionId)
	return nil
}

func CreateEmptyUserFeed(tx pgw.Queryable, userId models.UserId) error {
	userRssText, err := generateUserRss(nil)
	if err != nil {
		return err
	}
	err = models.UserRss_Upsert(tx, userId, userRssText)
	if err != nil {
		return err
	}
	log.Info().Msg("Created empty user RSS")
	return nil
}

type rss struct {
	Version          string  `xml:"version,attr"`
	ContentNamespace string  `xml:"xmlns:content,attr"`
	Channel          channel `xml:"channel"`
}

type channel struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
	Items []item `xml:"item"`
}

type item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Guid        guid   `xml:"guid"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type guid struct {
	Guid        string `xml:",chardata"`
	IsPermalink bool   `xml:"isPermaLink,attr"`
}

func generateSubscriptionRss(subscriptionName string, subscriptionUrl string, items []item) (string, error) {
	title := fmt.Sprintf("%s · FeedRewind", subscriptionName)
	return generateRss(title, subscriptionUrl, items)
}

func generateUserRss(items []item) (string, error) {
	return generateRss("FeedRewind", "https://feedrewind.com", items)
}

func generateRss(title string, url string, items []item) (string, error) {
	var buf bytes.Buffer
	_, _ = fmt.Fprint(&buf, xml.Header)
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	err := encoder.Encode(rss{
		Version:          "2.0",
		ContentNamespace: "http://purl.org/rss/1.0/modules/content/",
		Channel: channel{
			Title: title,
			Link:  url,
			Items: items,
		},
	})
	if err != nil {
		return "", oops.Wrap(err)
	}

	return buf.String(), nil
}
