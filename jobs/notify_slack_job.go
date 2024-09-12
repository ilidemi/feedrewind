package jobs

import (
	"bytes"
	"context"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"io"
	"net/http"
	"strings"

	"github.com/goccy/go-json"
)

func init() {
	registerJobNameFunc(
		"NotifySlackJob",
		false,
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 1 {
				return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
			}

			text, ok := args[0].(string)
			if !ok {
				return oops.Newf("Failed to parse text (expected string): %v", args[0])
			}

			return NotifySlackJob_Perform(ctx, conn, text)
		},
	)
}

func NotifySlackJob_PerformNow(tx pgw.Queryable, text string) error {
	return performNow(tx, "NotifySlackJob", defaultQueue, strToYaml(text))
}

func NotifySlackJob_Perform(ctx context.Context, conn *pgw.Conn, text string) error {
	logger := conn.Logger()
	webhookUrl := config.Cfg.SlackWebhook

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
		maybeSlackDump, err := models.TestSingleton_GetValue(conn, "slack_dump")
		if err != nil {
			return err
		}
		if maybeSlackDump != nil && *maybeSlackDump == "yes" {
			err = models.TestSingleton_SetValue(conn, "slack_last_message", text)
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
