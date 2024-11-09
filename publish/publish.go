package publish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"slices"
	"time"

	"feedrewind.com/config"
	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/routes/rutil"
	"feedrewind.com/util/schedule"
)

const defaultPostsInRss = 30

func InitSubscription(
	tx *pgw.Tx, userId models.UserId, productUserId models.ProductUserId,
	subscriptionId models.SubscriptionId, subscriptionName string, blogBestUrl string,
	deliveryChannel models.DeliveryChannel, shouldPublishRssPosts bool, utcNow schedule.Time,
	localTime schedule.Time, localDate schedule.Date,
) error {
	return initSubscriptionImpl(
		tx, userId, productUserId, subscriptionId, subscriptionName, blogBestUrl, deliveryChannel,
		shouldPublishRssPosts, utcNow, localTime, localDate, defaultPostsInRss,
	)
}

func PublishForUser(
	tx *pgw.Tx, userId models.UserId, productUserId models.ProductUserId,
	deliveryChannel models.DeliveryChannel, utcNow schedule.Time, localTime schedule.Time,
	localDate schedule.Date, scheduledFor string,
) error {
	return publishForUserImpl(
		tx, userId, productUserId, deliveryChannel, utcNow, localTime, localDate, scheduledFor,
		defaultPostsInRss,
	)
}

var EmailInitialItemJob_PerformNowFunc func(
	qu pgw.Queryable, userId models.UserId, subscriptionId models.SubscriptionId, scheduledFor string,
) error

var EmailPostsJob_PerformNowFunc func(
	qu pgw.Queryable, userId models.UserId, date schedule.Date, scheduledFor string,
	finalItemSubscriptionIds []models.SubscriptionId,
) error

func initSubscriptionImpl(
	tx *pgw.Tx, userId models.UserId, productUserId models.ProductUserId,
	subscriptionId models.SubscriptionId, subscriptionName string, blogBestUrl string,
	deliveryChannel models.DeliveryChannel, shouldPublishRssPosts bool, utcNow schedule.Time,
	localTime schedule.Time, localDate schedule.Date, postsInRss int,
) error {
	logger := tx.Logger()
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
		var newPosts []models.PublishedSubscriptionBlogPost
		if shouldPublishRssPosts {
			dayOfWeek := localTime.DayOfWeek()
			dayCount, err := models.Schedule_GetCount(tx, subscriptionId, dayOfWeek)
			if err != nil {
				return err
			}

			unpublishedNewPosts, err :=
				models.SubscriptionPost_GetNextUnpublished(tx, subscriptionId, dayCount)
			if err != nil {
				return err
			}
			newPosts = make([]models.PublishedSubscriptionBlogPost, len(unpublishedNewPosts))
			for i, post := range unpublishedNewPosts {
				newPosts[i] = models.PublishedSubscriptionBlogPost{
					Id:          post.Id,
					Title:       post.Title,
					RandomId:    post.RandomId,
					PublishedAt: utcNow,
				}
			}
			logger.Info().Msgf("Subscription %d: will publish %d new posts", subscriptionId, len(newPosts))

			unpublishedCount, err := models.SubscriptionPost_GetUnpublishedCount(tx, subscriptionId)
			if err != nil {
				return err
			}
			if subscription.MaybeFinalItemPublishedAt == nil && unpublishedCount == len(newPosts) {
				// So that publishRssFeeds() knows about the update too
				subscription.MaybeFinalItemPublishedAt = &utcNow

				_, err := tx.Exec(`
					update subscriptions_without_discarded
					set final_item_published_at = $1, final_item_publish_status = $2
					where id = $3
				`, utcNow, publishStatus, subscriptionId)
				if err != nil {
					return err
				}
				logger.Info().Msgf("Will publish the final item for subscription %d", subscriptionId)

				models.ProductEvent_MustEmit(tx, productUserId, "finish subscription", map[string]any{
					"subscription_id": subscriptionId,
					"blog_url":        blogBestUrl,
				}, nil)
			}
		}

		newPostsBySubscriptionId := map[models.SubscriptionId][]models.PublishedSubscriptionBlogPost{
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
		scheduledFor := utcNow.MustUTCString()
		// The job won't be visible until the transaction is committed
		err = EmailInitialItemJob_PerformNowFunc(tx, userId, subscriptionId, scheduledFor)
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

	_, err = tx.Exec(`
		update subscriptions_without_discarded set initial_item_publish_status = $1 where id = $2
	`, publishStatus, subscriptionId)
	return err
}

func publishForUserImpl(
	tx *pgw.Tx, userId models.UserId, productUserId models.ProductUserId,
	deliveryChannel models.DeliveryChannel, utcNow schedule.Time, localTime schedule.Time,
	localDate schedule.Date, scheduledFor string, postsInRss int,
) error {
	logger := tx.Logger()
	subscriptions, err := models.Subscription_ListSortedToPublish(tx, userId)
	if err != nil {
		return err
	}
	logger.Info().Msgf("%d subscriptions", len(subscriptions))

	newPostsBySubscriptionId := make(map[models.SubscriptionId][]models.PublishedSubscriptionBlogPost)
	var finalItemSubscriptionIds []models.SubscriptionId
	for i := range subscriptions {
		subscription := &subscriptions[i]

		var newPosts []models.PublishedSubscriptionBlogPost
		if !subscription.IsPaused {
			dayOfWeek := localTime.DayOfWeek()
			dayCount, err := models.Schedule_GetCount(tx, subscription.Id, dayOfWeek)
			if err != nil {
				return err
			}

			unpublishedNewPosts, err :=
				models.SubscriptionPost_GetNextUnpublished(tx, subscription.Id, dayCount)
			if err != nil {
				return err
			}
			newPosts = make([]models.PublishedSubscriptionBlogPost, len(unpublishedNewPosts))
			for i, post := range unpublishedNewPosts {
				newPosts[i] = models.PublishedSubscriptionBlogPost{
					Id:          post.Id,
					Title:       post.Title,
					RandomId:    post.RandomId,
					PublishedAt: utcNow,
				}
			}
			logger.Info().Msgf("Subscription %d: will publish %d new posts", subscription.Id, len(newPosts))
		} else {
			logger.Info().Msgf("Skipping subscription %d", subscription.Id)
		}
		newPostsBySubscriptionId[subscription.Id] = newPosts

		unpublishedCount, err := models.SubscriptionPost_GetUnpublishedCount(tx, subscription.Id)
		if err != nil {
			return err
		}
		if subscription.MaybeFinalItemPublishedAt == nil && unpublishedCount == len(newPosts) {
			// So that publishRssFeeds() knows about the update too
			subscription.MaybeFinalItemPublishedAt = &utcNow
			finalItemSubscriptionIds = append(finalItemSubscriptionIds, subscription.Id)
			logger.Info().Msgf("Will publish the final item for subscription %d", subscription.Id)

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
		err := EmailPostsJob_PerformNowFunc(tx, userId, localDate, scheduledFor, finalItemSubscriptionIds)
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
		_, err := tx.Exec(`
			update subscriptions_without_discarded
			set final_item_published_at = $1, final_item_publish_status = $2
			where id = $3
		`, utcNow, publishStatus, subscriptionId)
		if err != nil {
			return err
		}
	}

	return nil
}

func publishRssFeeds(
	tx *pgw.Tx, userId models.UserId, subscriptions []models.SubscriptionToPublish,
	newPostsBySubscriptionId map[models.SubscriptionId][]models.PublishedSubscriptionBlogPost, postsInRss int,
) error {
	logger := tx.Logger()
	type UserDateItem struct {
		PublishedAt schedule.Time
		Item        item
	}
	var userDatesItems []UserDateItem
	for _, subscription := range subscriptions {
		logger.Info().Msgf("Generating RSS for subscription %d", subscription.Id)
		newPosts := newPostsBySubscriptionId[subscription.Id]
		remainingPostsCount := postsInRss - len(newPosts)
		if subscription.MaybeFinalItemPublishedAt != nil {
			remainingPostsCount--
		}
		remainingPosts, err := models.SubscriptionPost_GetLastPublishedDesc(
			tx, subscription.Id, remainingPostsCount,
		)
		if err != nil {
			return err
		}
		slices.Reverse(remainingPosts)

		subscriptionUrl := rutil.SubscriptionUrl(subscription.Id)
		var subscriptionItems []item
		if subscription.MaybeFinalItemPublishedAt != nil {
			logger.Info().Msgf("Generating final item for subscription %d", subscription.Id)
			finalItem := item{
				Title: fmt.Sprintf("You're all caught up with %s", subscription.Name),
				Link:  subscriptionUrl,
				Guid: guid{
					Guid:        makeGuid(fmt.Sprintf("%d-final", subscription.Id)),
					IsPermalink: false,
				},
				Description: fmt.Sprintf(
					`<a href="%s">Want to read something else?</a>`, rutil.SubscriptionAddUrl(),
				),
				PubDate: subscription.MaybeFinalItemPublishedAt.Format(time.RFC1123Z),
			}
			subscriptionItems = append(subscriptionItems, finalItem)
			userDatesItems = append(userDatesItems, UserDateItem{
				PublishedAt: *subscription.MaybeFinalItemPublishedAt,
				Item:        finalItem,
			})
		}

		subscriptionPosts := slices.Concat(remainingPosts, newPosts)
		slices.Reverse(subscriptionPosts)

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
			userDatesItems = append(userDatesItems, UserDateItem{
				PublishedAt: post.PublishedAt,
				Item:        userItem,
			})
		}

		if len(subscriptionItems) < postsInRss {
			logger.Info().Msgf("Generating initial item for subscription %d", subscription.Id)
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
			userDatesItems = append(userDatesItems, UserDateItem{
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
		logger.Info().Msgf("Total items for subscription %d: %d", subscription.Id, len(subscriptionItems))

		err = models.SubscriptionRss_Upsert(tx, subscription.Id, subscriptionRssText)
		if err != nil {
			return err
		}
		logger.Info().Msgf("Saved RSS for subscription %d", subscription.Id)
	}

	// Date desc, index asc (= publish date desc, sub date desc, post index desc)
	slices.SortStableFunc(userDatesItems, func(a, b UserDateItem) int {
		return b.PublishedAt.Compare(a.PublishedAt)
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
	logger.Info().Msgf("Total items for user %d: %d (%d new)", userId, len(userItems), newUserItemsCount)
	userRssText, err := generateUserRss(userItems)
	if err != nil {
		return err
	}

	err = models.UserRss_Upsert(tx, userId, userRssText)
	if err != nil {
		return err
	}
	logger.Info().Msgf("Saved RSS for user %d", userId)

	return nil
}

func makeGuid(value string) string {
	hashBytes := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hashBytes[:])
}

func createEmptySubscriptionFeed(
	tx *pgw.Tx, subscriptionId models.SubscriptionId, subscriptionName string,
) error {
	logger := tx.Logger()
	subscriptionUrl := rutil.SubscriptionUrl(subscriptionId)
	subscriptionRssText, err := generateSubscriptionRss(subscriptionName, subscriptionUrl, nil)
	if err != nil {
		return err
	}
	err = models.SubscriptionRss_Upsert(tx, subscriptionId, subscriptionRssText)
	if err != nil {
		return err
	}
	logger.Info().Msgf("Created empty RSS for subscription %d", subscriptionId)
	return nil
}

func CreateEmptyUserFeed(tx *pgw.Tx, userId models.UserId) error {
	logger := tx.Logger()
	userRssText, err := generateUserRss(nil)
	if err != nil {
		return err
	}
	err = models.UserRss_Upsert(tx, userId, userRssText)
	if err != nil {
		return err
	}
	logger.Info().Msg("Created empty user RSS")
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
	return generateRss("FeedRewind", config.Cfg.RootUrl, items)
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
