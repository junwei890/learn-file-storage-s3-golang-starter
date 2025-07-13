package main

import (
	"os/exec"
	"bytes"
	"errors"
	"fmt"
	"os"
	"encoding/json"
)

func getVideoAspectRatio(filePath string) (string, error) {
	command := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	buffer := &bytes.Buffer{}
	command.Stdout = buffer
	if err := command.Run(); err != nil {
		return "", errors.New("unable to run command")
	}

	var output struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(buffer.Bytes(), &output); err != nil {
		return "", fmt.Errorf("could not parse ffprobe output: %v", err)
	}

	if len(output.Streams) == 0 {
		return "", errors.New("no video streams found")
	}

	width := output.Streams[0].Width
	height := output.Streams[0].Height

	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := fmt.Sprintf("%s.processing", filePath)
	command := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	if err := command.Run(); err != nil {
		return "", errors.New("error encountered processing video")
	}

	fileInfo, err := os.Stat(outputFilePath)
	if err != nil {
		return "", errors.New("error getting metadata for processed file")
	}
	if fileInfo.Size() == 0 {
		return "", errors.New("processed file is empty")
	}
	
	return outputFilePath, nil
}
