package crawl

import (
	"errors"
	"fmt"
	"html"
	"os"
	"slices"
	"strings"
	"time"

	"feedrewind.com/crawler"
)

func outputReport(filename string, results []Result, expectedTotal int) error {
	successCount := 0
	failureCount := 0
	badFailureCount := 0

	type EvaluatedResult struct {
		StartLinkId int
		Values      []any
		Statuses    []crawler.Status
		Error       error
		Key         string
	}
	evaluatedResults := make([]EvaluatedResult, 0, len(results))

	statusKeys := map[crawler.Status]string{
		crawler.StatusSuccess: "a",
		crawler.StatusNeutral: "b",
		crawler.StatusFailure: "c",
	}
	for _, result := range results {
		if result.Result != nil {
			columnStatuses := result.Result.ColumnStatuses()
			hasFailure := false
			for _, status := range columnStatuses {
				if status == crawler.StatusFailure {
					hasFailure = true
					break
				}
			}
			if hasFailure {
				failureCount++
			} else {
				successCount++
			}

			var b strings.Builder
			b.WriteString("a|")
			statuses := result.Result.ColumnStatuses()
			for _, status := range statuses {
				b.WriteString(statusKeys[status])
			}
			b.WriteString("|")
			if result.Error != nil {
				b.WriteString(result.Error.Error())
			}
			b.WriteString("|")
			b.WriteString(fmt.Sprint(result.StartLinkId))
			evaluatedResults = append(evaluatedResults, EvaluatedResult{
				StartLinkId: result.StartLinkId,
				Values:      result.Result.ColumnValues(),
				Statuses:    statuses,
				Error:       result.Error,
				Key:         b.String(),
			})
		} else {
			badFailureCount++
			key := "b|"
			if result.Error != nil {
				key += result.Error.Error()
			}
			evaluatedResults = append(evaluatedResults, EvaluatedResult{
				StartLinkId: result.StartLinkId,
				Values:      nil,
				Statuses:    nil,
				Error:       result.Error,
				Key:         key,
			})
		}
	}

	slices.SortFunc(evaluatedResults, func(a, b EvaluatedResult) int {
		return strings.Compare(a.Key, b.Key)
	})

	type Counter struct {
		SuccessCount int
		NeutralCount int
		FailureCount int
	}
	var columnCounters []Counter
	for i := 0; i < len(GuidedCrawlingColumnNames); i++ {
		statusCounts := make(map[crawler.Status]int)
		for _, result := range evaluatedResults {
			if result.Statuses == nil {
				statusCounts[crawler.StatusFailure]++
			} else {
				statusCounts[result.Statuses[i]]++
			}
		}
		columnCounters = append(columnCounters, Counter{
			SuccessCount: statusCounts[crawler.StatusSuccess],
			NeutralCount: statusCounts[crawler.StatusNeutral],
			FailureCount: statusCounts[crawler.StatusFailure],
		})
	}

	styles := map[crawler.Status]string{
		crawler.StatusNeutral: "",
		crawler.StatusSuccess: ` style="background: lightgreen;"`,
		crawler.StatusFailure: ` style="background: lightcoral;"`,
	}

	var b strings.Builder

	fmt.Fprintf(&b, `<html>
<head>
	<title>Report</title>
	<style>table, th, td { border: 1px solid black; border-collapse: collapse; }</style>
	<style>body, table { font-size: small; }</style>
</head>
<body>
`)
	fmt.Fprintf(&b, "Processed: %d/%d\n", len(evaluatedResults), expectedTotal)
	fmt.Fprintf(&b, "<br>\n")

	fmt.Fprintf(
		&b, "Success: %d Failure: %d Bad failure: %d\n", successCount, failureCount, badFailureCount,
	)
	fmt.Fprintf(&b, "<br>\n")

	weissmanScore := "N/A"
	if len(evaluatedResults) > 0 {
		weissmanScore = fmt.Sprint(successCount * 100 / len(evaluatedResults))
	}
	fmt.Fprintf(&b, "Weissman score: %s%%\n", weissmanScore)
	fmt.Fprintf(&b, "<br>\n")

	fmt.Fprintf(&b, "<table>\n")
	fmt.Fprintf(&b, "<tr><th>id</th>\n")
	for i, columnName := range GuidedCrawlingColumnNames {
		counters := columnCounters[i]
		counterStr := ""
		if counters.SuccessCount > 0 || counters.FailureCount > 0 {
			counterStr = `<br>`
			counterStr += fmt.Sprintf(`<span style="color: green;">%d</span>`, counters.SuccessCount)
			if counters.NeutralCount > 0 {
				counterStr += fmt.Sprintf(` / %d / `, counters.NeutralCount)
			} else {
				counterStr += ` / `
			}
			counterStr += fmt.Sprintf(`<span style="color: red;">%d</span>`, counters.FailureCount)
		}
		fmt.Fprintf(&b, "<th>%s%s</th>", columnName, counterStr)
	}
	fmt.Fprintf(&b, "<th>error</th></tr>\n")

	for _, result := range evaluatedResults {
		fmt.Fprintf(&b, "<tr>\n")
		fmt.Fprintf(&b, "<td>id%04d</td>", result.StartLinkId)

		if result.Values == nil {
			for range GuidedCrawlingColumnNames {
				fmt.Fprintf(&b, "<td></td>")
			}
		} else {
			for i, value := range result.Values {
				var valueStr string
				if GuidedCrawlingColumnNames[i] == "extra" {
					valueStr = strings.Join(value.([]string), "<br>")
				} else {
					valueStr = fmt.Sprint(value)
				}
				fmt.Fprintf(&b, "<td%s>%s</td>", styles[result.Statuses[i]], valueStr)
			}
		}

		if result.Error == nil {
			fmt.Fprintf(&b, "<td></td>")
		} else {
			errorHtml := strings.ReplaceAll(html.EscapeString(result.Error.Error()), "\n", "<br>")
			fmt.Fprintf(&b, "<td%s>%s</td>", styles[crawler.StatusFailure], errorHtml)
		}
		fmt.Fprintf(&b, "</tr>\n")
	}
	fmt.Fprintf(&b, "</table>\n")
	fmt.Fprintf(&b, "</body>\n")
	fmt.Fprintf(&b, "</html>\n")

	tempFilename := fmt.Sprintf("%s/feedrewind_report.html", os.TempDir())
	tempFile, err := os.Create(tempFilename)
	if err != nil {
		return err
	}
	_, err = tempFile.WriteString(b.String())
	if err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	writeAttempts := 0
	for {
		writeAttempts++
		_, err := os.Stat(filename)
		if err == nil {
			removeErr := os.Remove(filename)
			if removeErr != nil {
				return removeErr
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		err = os.Rename(tempFilename, filename)
		if err != nil {
			if writeAttempts >= 20 {
				fmt.Println("Couldn't write report")
				return err
			} else {
				fmt.Println("Coulnd't write report, retrying in 100ms")
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		break
	}

	return nil
}
