package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
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
