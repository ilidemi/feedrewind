package jobs

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"feedrewind.com/config"
	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"

	"github.com/goccy/go-json"
)

func init() {
	registerJobNameFunc(
		"NotifySlackJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 1 {
				return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
			}

			text, ok := args[0].(string)
			if !ok {
				return oops.Newf("Failed to parse text (expected string): %v", args[0])
			}

			return NotifySlackJob_Perform(ctx, pool, text)
		},
	)
}

func NotifySlackJob_PerformNow(qu pgw.Queryable, text string) error {
	return performNow(qu, "NotifySlackJob", defaultQueue, strToYaml(text))
}

func NotifySlackJob_Perform(ctx context.Context, pool *pgw.Pool, text string) error {
	logger := pool.Logger()
	webhookUrl := config.Cfg.SlackWebhook
	if webhookUrl == config.DemoValue {
		logger.Info().Msg("Skipping Slack notifications in demo mode")
		return nil
	}

	body, err := json.Marshal(map[string]string{
		"text": text,
	})
	if err != nil {
		return oops.Wrap(err)
	}
	reader := bytes.NewReader(body)

	req, err := http.NewRequest(http.MethodPost, webhookUrl, reader)
	if err != nil {
		return oops.Wrap(err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oops.Wrap(err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error().Err(err).Msg("Error while reading Slack response body")
		}
		return oops.Newf("Slack webhook failed: %d %s", resp.StatusCode, string(respBody))
	}

	if config.Cfg.Env.IsDevOrTest() {
		maybeSlackDump, err := models.TestSingleton_GetValue(pool, "slack_dump")
		if err != nil {
			return err
		}
		if maybeSlackDump != nil && *maybeSlackDump == "yes" {
			err = models.TestSingleton_SetValue(pool, "slack_last_message", text)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func NotifySlackJob_Escape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
