package main

import (
	"fmt"
	"net/http"
	"io"
	"path/filepath"
	"os"
	"strings"
	"crypto/rand"
	"encoding/base64"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to parse request body", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to return file from form key", err)
		return
	}

	mediaType := fileHeader.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Media type not specified", nil)
		return
	}
	if mediaType != "image/png" && mediaType != "image/jpeg" {
		respondWithError(w, http.StatusBadRequest, "Invalid file types", nil)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video does not exist", err)
		return
	}

	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "User ID mismatch", err)
		return
	}

	byteSlice := make([]byte, 32)
	if _, err := rand.Read(byteSlice); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to fill slice of bytes", err)
	}
	randomName := base64.RawURLEncoding.EncodeToString(byteSlice)

	fileName := fmt.Sprintf("%s.%s", randomName, strings.Split(mediaType, "/")[1])
	filePath := filepath.Join(cfg.assetsRoot, fileName)
	createdFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file in file system", err)
		return
	}
	defer createdFile.Close()

	if _, err := io.Copy(createdFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write to file", err)
		return
	}

	newThumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)
	videoMetadata.ThumbnailURL = &newThumbnailURL
	if err := cfg.db.UpdateVideo(videoMetadata); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetadata)
}
