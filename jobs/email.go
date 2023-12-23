package jobs

import (
	"errors"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mrz1836/postmark"
)

func GetPostmarkClientAndMaybeMetadata(tx pgw.Queryable) (*postmark.Client, *string) {
	var maybeTestMetadata *string
	var apiToken string
	if config.Cfg.Env.IsDevOrTest() {
		var err error
		maybeTestMetadata, err = models.TestSingleton_GetValue(tx, "email_metadata")
		if errors.Is(err, pgx.ErrNoRows) {
			apiToken = config.Cfg.PostmarkApiSandboxToken
		} else if err == nil {
			apiToken = config.Cfg.PostmarkApiToken
		} else {
			panic(err)
		}
	} else {
		apiToken = config.Cfg.PostmarkApiToken
	}

	client := postmark.NewClient(apiToken, "")
	return client, maybeTestMetadata
}

func newInitialEmail(
	userId models.UserId, userEmail string, subscriptionId models.SubscriptionId, subscriptionName string,
	maybeTestMetadata *string, scheduledFor string,
) postmark.Email {

	type TemplateData struct {
		Url string
	}
	templateData := TemplateData{
		Url: rutil.SubscriptionUrl(subscriptionId),
	}
	htmlBody := templates.MustFormat("email/initial", templateData)
	textBody := templates.MustFormat("email/initial.txt", templateData)

	metadata := map[string]string{
		"user_id":         fmt.Sprint(userId),
		"subscription_id": fmt.Sprint(subscriptionId),
	}
	if maybeTestMetadata != nil {
		metadata["test"] = *maybeTestMetadata
		metadata["server_timestamp"] = scheduledFor
	}

	return postmark.Email{ //nolint:exhaustruct
		From:          "feedrewind@feedrewind.com",
		To:            userEmail,
		ReplyTo:       "support@feedrewind.com",
		Subject:       fmt.Sprintf("%s added to FeedRewind", subscriptionName),
		Tag:           "subscription_initial",
		HTMLBody:      htmlBody,
		TextBody:      textBody,
		Metadata:      metadata,
		MessageStream: "outbound",
	}
}

func newFinalEmail(
	userId models.UserId, userEmail string, subscriptionId models.SubscriptionId, subscriptionName string,
	maybeTestMetadata *string, scheduledFor string,
) postmark.Email {
	type TemplateData struct {
		AddUrl string
	}
	htmlBody := templates.MustFormat("email/final", &TemplateData{
		AddUrl: rutil.SubscriptionAddUrl,
	})
	textBody := templates.MustFormat("email/final.txt", &TemplateData{
		AddUrl: rutil.SubscriptionAddUrl,
	})

	metadata := map[string]string{
		"user_id":         fmt.Sprint(userId),
		"subscription_id": fmt.Sprint(subscriptionId),
	}
	if maybeTestMetadata != nil {
		metadata["test"] = *maybeTestMetadata
		metadata["server_timestamp"] = scheduledFor
	}
	return postmark.Email{ //nolint:exhaustruct
		From:          "feedrewind@feedrewind.com",
		To:            userEmail,
		ReplyTo:       "support@feedrewind.com",
		Subject:       fmt.Sprintf("You're all caught up with %s", subscriptionName),
		Tag:           "subscription_final",
		HTMLBody:      htmlBody,
		TextBody:      textBody,
		Metadata:      metadata,
		MessageStream: "outbound",
	}
}

func newPostEmail(
	userId models.UserId, userEmail string, subscriptionId models.SubscriptionId, subscriptionName string,
	postId models.SubscriptionPostId, postTitle string, postRandomId models.SubscriptionPostRandomId,
	maybeTestMetadata *string, scheduledFor string,
) postmark.Email {
	type TemplateData struct {
		SubscriptionName string
		SubscriptionUrl  string
		PostTitle        string
		PostUrl          string
	}
	templateData := TemplateData{
		SubscriptionName: subscriptionName,
		SubscriptionUrl:  rutil.SubscriptionUrl(subscriptionId),
		PostTitle:        postTitle,
		PostUrl:          rutil.SubscriptionPostUrl(postTitle, postRandomId),
	}
	htmlBody := templates.MustFormat("email/post", templateData)
	textBody := templates.MustFormat("email/post.txt", templateData)

	metadata := map[string]string{
		"user_id":              fmt.Sprint(userId),
		"subscription_id":      fmt.Sprint(subscriptionId),
		"subscription_post_id": fmt.Sprint(postId),
	}
	if maybeTestMetadata != nil {
		metadata["test"] = *maybeTestMetadata
		metadata["server_timestamp"] = scheduledFor
	}
	return postmark.Email{ //nolint:exhaustruct
		From:          "feedrewind@feedrewind.com",
		To:            userEmail,
		ReplyTo:       "support@feedrewind.com",
		Subject:       postTitle,
		Tag:           "subscription_post",
		HTMLBody:      htmlBody,
		TextBody:      textBody,
		Metadata:      metadata,
		MessageStream: "outbound",
	}
}
