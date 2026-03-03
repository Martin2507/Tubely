package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {

	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	decoder := json.NewDecoder(&buffer)
	ffprobesParams := ffprobeOutput{}
	jsonErr := decoder.Decode(&ffprobesParams)
	if jsonErr != nil {
		return "", jsonErr
	}

	if len(ffprobesParams.Streams) == 0 {
		return "", errors.New("no stream found")
	}

	ratio := float64(ffprobesParams.Streams[0].Width) / float64(ffprobesParams.Streams[0].Height)

	if math.Abs(ratio-(16.0/9.0)) < 0.02 {
		return "16:9", nil
	}

	if math.Abs(ratio-(9.0/16.0)) < 0.02 {
		return "9:16", nil
	}

	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {

	outputFile := filePath + ".processing"

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFile)
	var buffer bytes.Buffer
	cmd.Stderr = &buffer

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("ffmpeg %s: %w", buffer.String(), err)
	}

	return outputFile, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {

	presignClient := s3.NewPresignClient(s3Client)

	newUrlString, err := presignClient.PresignGetObject(
		context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
		s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	return newUrlString.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil {
		return video, nil
	}

	splitVideoUrl := strings.Split(*video.VideoURL, ",")
	if len(splitVideoUrl) != 2 {
		return video, nil
	}

	presignClientString, err := generatePresignedURL(cfg.s3Client, splitVideoUrl[0], splitVideoUrl[1], time.Hour)
	if err != nil {
		return video, err
	}

	video.VideoURL = &presignClientString

	return video, nil
}
