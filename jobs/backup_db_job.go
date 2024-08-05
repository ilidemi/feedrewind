package jobs

import (
	"bytes"
	"context"
	"feedrewind/config"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"io"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/rs/zerolog"
)

func init() {
	registerJobNameFunc(
		"BackupDbJob",
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return BackupDbJob_Perform(ctx, conn)
		},
	)
	migrations.BackupDbJob_PerformAtFunc = BackupDbJob_PerformAt
}

func BackupDbJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "BackupDbJob", defaultQueue)
}

func BackupDbJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()
	if config.Cfg.Env.IsDevOrTest() {
		logger.Info().Msg("No db backup in dev")
	} else {
		var cmd *exec.Cmd
		if config.Cfg.Env.IsDevOrTest() {
			cmd = exec.Command("wsl", "heroku", "pg:backups", "-a", "feedrewind")
		} else {
			cmd = exec.Command("heroku", "pg:backups", "-a", "feedrewind")
		}

		cmdOutput, err := cmd.Output()
		if err != nil {
			return oops.Wrap(err)
		}
		cmdOutputStr := string(cmdOutput)

		backupsStart := strings.Index(cmdOutputStr, "=== Backups")
		if backupsStart == -1 {
			return oops.New("Backups section not found")
		}
		backupsLength := strings.Index(cmdOutputStr[backupsStart:], "\n\n=== Restores")
		if backupsLength == -1 {
			return oops.New("End of backups section not found")
		}
		backupsSection := cmdOutputStr[backupsStart : backupsStart+backupsLength]
		backupsSectionLines := strings.Split(backupsSection, "\n")
		linesLog := zerolog.Arr()
		for _, line := range backupsSectionLines {
			linesLog.Str(line)
		}
		logger.Info().Array("lines", linesLog).Msg("Queried pg:backups")

		idsToUpload := map[string]bool{}
		headerEnded := false
		for _, line := range backupsSectionLines {
			if strings.HasPrefix(line, "────") {
				headerEnded = true
				continue
			} else if !headerEnded {
				continue
			}

			isCompleted := strings.Contains(line, "Completed")
			isRunning := strings.Contains(line, "Running")
			if isRunning {
				logger.Warn().Msgf("Backup is still running: %s", line)
			} else if isCompleted {
				spaceIdx := strings.Index(line, " ")
				if spaceIdx == -1 {
					return oops.Newf("Couldn't find space: %s", line)
				}
				backupId := line[:spaceIdx]
				idsToUpload[backupId] = true
			} else {
				logger.Error().Msgf("Expected a backup to be running or completed: %s", line)
			}
		}

		creds := credentials.NewStaticCredentialsProvider(
			config.Cfg.AwsAccessKey, config.Cfg.AwsSecretAccessKey, "",
		)
		awsCfg, err := awsconfig.LoadDefaultConfig(
			ctx, awsconfig.WithCredentialsProvider(creds), awsconfig.WithRegion("us-west-2"),
		)
		if err != nil {
			return oops.Wrap(err)
		}
		s3Client := s3.NewFromConfig(awsCfg)
		bucket := "feedrewind-db-backup-dev"
		if config.Cfg.Env == config.EnvProduction {
			bucket = "feedrewind-db-backup-prod"
		}
		//nolint:exhaustruct
		incompleteUploads, err := s3Client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			return oops.Wrap(err)
		}
		if incompleteUploads.IsTruncated == nil || *incompleteUploads.IsTruncated {
			return oops.Newf("S3 incomplete uploads list was truncated at %d", len(incompleteUploads.Uploads))
		}
		if len(incompleteUploads.Uploads) == 0 {
			logger.Info().Msg("No incomplete uploads")
		} else {
			logger.Info().Msgf("Aborting %d incomplete uploads", len(incompleteUploads.Uploads))
			for _, incompleteUpload := range incompleteUploads.Uploads {
				//nolint:exhaustruct
				_, err := s3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
					Bucket:   incompleteUploads.Bucket,
					Key:      incompleteUpload.Key,
					UploadId: incompleteUpload.UploadId,
				})
				if err != nil {
					return oops.Wrap(err)
				}
			}
			logger.Info().Msg("Aborting incomplete uploads done")
		}

		//nolint:exhaustruct
		listOutput, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			return oops.Wrap(err)
		}
		if listOutput.IsTruncated == nil || *listOutput.IsTruncated {
			return oops.Newf("S3 list output was truncated at %d", *listOutput.KeyCount)
		}
		logger.Info().Msgf("S3 has %d backups", *listOutput.KeyCount)
		lastUploadTime := time.Unix(0, 0)
		for _, object := range listOutput.Contents {
			if object.Key == nil {
				logger.Warn().Msg("S3 object key is null")
				continue
			}
			if idsToUpload[*object.Key] {
				delete(idsToUpload, *object.Key)
			}
			if object.LastModified == nil {
				logger.Warn().Msg("S3 last modified time is null")
				continue
			}
			if object.LastModified.After(lastUploadTime) {
				lastUploadTime = *object.LastModified
			}
		}

		if len(idsToUpload) == 0 {
			logger.Info().Msg("No backups to upload")
			if time.Since(lastUploadTime) >= 48*time.Hour {
				logger.Warn().Msgf("Latest uploaded backup is pretty old: %s", lastUploadTime)
			}
		} else {
			logger.Info().Msgf("Uploading %d backups", len(idsToUpload))
			for backupId := range idsToUpload {
				var cmd *exec.Cmd
				if config.Cfg.Env.IsDevOrTest() {
					cmd = exec.Command("wsl", "heroku", "pg:backups:url", backupId, "-a", "feedrewind")
				} else {
					cmd = exec.Command("heroku", "pg:backups:url", backupId, "-a", "feedrewind")
				}
				cmdOutput, err := cmd.Output()
				if err != nil {
					return oops.Wrap(err)
				}
				backupUrl := strings.TrimSpace(string(cmdOutput))
				logger.Info().Msgf("Backup %s: got url", backupId)

				resp, err := http.Get(backupUrl)
				if err != nil {
					return oops.Wrap(err)
				}

				//nolint:exhaustruct
				uploadOutput, err := s3Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
					Bucket:      aws.String(bucket),
					Key:         aws.String(backupId),
					ContentType: aws.String("application/octet-stream"),
				})
				if err != nil {
					return oops.Wrap(err)
				}

				const maxPartSize int64 = 50 * 1024 * 1024
				buf := make([]byte, maxPartSize+1024*1024)
				remaining := resp.ContentLength
				var curr, partLength int64
				var completedParts []types.CompletedPart
				var partNumber int32 = 1
				for curr = 0; remaining != 0; curr += partLength {
					if remaining < maxPartSize {
						partLength = remaining
					} else {
						partLength = maxPartSize
					}

					actualPartLength, err := io.ReadAtLeast(resp.Body, buf, int(partLength))
					if err != nil {
						return oops.Wrap(err)
					}
					partLength = int64(actualPartLength)

					//nolint:exhaustruct
					uploadResult, err := s3Client.UploadPart(ctx, &s3.UploadPartInput{
						Body:       bytes.NewReader(buf[:partLength]),
						Bucket:     uploadOutput.Bucket,
						Key:        uploadOutput.Key,
						PartNumber: aws.Int32(partNumber),
						UploadId:   uploadOutput.UploadId,
					})
					if err != nil {
						return oops.Wrap(err)
					}

					//nolint:exhaustruct
					completedParts = append(completedParts, types.CompletedPart{
						ETag:       uploadResult.ETag,
						PartNumber: aws.Int32(partNumber),
					})
					remaining -= partLength
					partNumber++
					logger.Info().Msgf("Backup %s: uploaded %d parts", backupId, len(completedParts))
				}

				//nolint:exhaustruct
				_, err = s3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
					Bucket:   uploadOutput.Bucket,
					Key:      uploadOutput.Key,
					UploadId: uploadOutput.UploadId,
					MultipartUpload: &types.CompletedMultipartUpload{
						Parts: completedParts,
					},
				})
				if err != nil {
					return oops.Wrap(err)
				}

				err = resp.Body.Close()
				if err != nil {
					return oops.Wrap(err)
				}
				logger.Info().Msgf("Backup %s: upload done", backupId)
			}
			logger.Info().Msg("Uploading backups done")
		}

		//nolint:exhaustruct
		newListOutput, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			return oops.Wrap(err)
		}
		if newListOutput.IsTruncated == nil || *newListOutput.IsTruncated {
			return oops.Newf("New S3 list output was truncated at %d", *newListOutput.KeyCount)
		}
		objects := newListOutput.Contents
		for _, object := range objects {
			if object.LastModified == nil {
				return oops.New("S3 last modified time is null")
			}
		}
		slices.SortFunc(objects, func(a, b types.Object) int {
			return b.LastModified.Compare(*a.LastModified) // descending
		})
		const objectsToKeep = 30
		if len(objects) <= objectsToKeep {
			logger.Info().Msgf("No old backups to delete (total: %d)", len(objects))
		} else {
			var objectsToDelete []types.ObjectIdentifier
			for _, object := range objects[objectsToKeep:] {
				//nolint:exhaustruct
				objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
					Key: object.Key,
				})
			}
			//nolint:exhaustruct
			deleteResult, err := s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &types.Delete{
					Objects: objectsToDelete,
				},
			})
			if err != nil {
				return oops.Wrap(err)
			}
			logger.Info().Msgf("Deleted %d old backups", len(deleteResult.Deleted))
			for _, err := range deleteResult.Errors {
				logger.Error().Msgf("Object deletion error: %#v", err)
			}
			if len(deleteResult.Errors) > 0 {
				return oops.Newf("Couldn't delete %d objects", len(deleteResult.Errors))
			}
		}
	}

	utcNow := schedule.UTCNow()
	runAt := utcNow.BeginningOfDayIn(time.UTC).Add(17 * time.Hour).UTC()
	if runAt.Sub(utcNow) < 0 {
		runAt = runAt.AddDate(0, 0, 1)
	}
	err := BackupDbJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
