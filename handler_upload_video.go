package main

import (
	"net/http"
	"mime"
	"os"
	"fmt"
	"strings"
	"io"
	"crypto/rand"
	"encoding/base64"
	"github.com/google/uuid"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	jwtToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unable to parse JWT token from header", err)
		return
	}
	userID, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unable to validate JWT token", err)
		return
	}

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video ID from metadata", err)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video ID not found", err)
		return
	}
	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "This video does not belong to you", err)
		return
	}

	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)
	file, _, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse request to multipart form", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType("video/mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}
	
	tempFileName := fmt.Sprintf("%s.%s", videoIDString, strings.Split(mediaType, "/")[1])
	tempFile, err := os.CreateTemp("", tempFileName)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create a temp file", err)
		return
	}
	defer os.Remove(tempFileName)
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write to temp file", err)
		return
	}
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to reset pointer to start of temp file", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get aspect ratio of video", err)
		return
	}
	var prefix string
	switch aspectRatio {
	case "16:9":
		prefix = "landscape"
	case "9:16":
		prefix = "portrait"
	default:
		prefix = "other"
	}

	processedVideoFileName, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to process video", err)
		return
	}
	processedVideo, err := os.Open(processedVideoFileName)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to open processed file", err)
		return
	}
	defer os.Remove(processedVideoFileName)
	defer processedVideo.Close()

	byteSlice := make([]byte, 32)
	if _, err := rand.Read(byteSlice); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to fill slice of bytes", err)
		return
	}
	randName := base64.RawURLEncoding.EncodeToString(byteSlice)
	fileKey := fmt.Sprintf("%s/%s.%s", prefix, randName, strings.Split(mediaType, "/")[1])

	putObjectParams := &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &fileKey,
		Body: processedVideo,
		ContentType: &mediaType,
	}
	if _, err := cfg.s3Client.PutObject(r.Context(), putObjectParams); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to store object in S3 bucket", err)
		return
	}

	videoURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, fileKey)
	videoMetadata.VideoURL = &videoURL
	if err := cfg.db.UpdateVideo(videoMetadata); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetadata)
}
