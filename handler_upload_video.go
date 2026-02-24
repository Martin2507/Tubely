package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	// Allocate 1 GB of space of memeory for the uploded video
	const maxMemory = 1 << 30

	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	// Access the video ID from the URL and parse it as UUID
	videoIDString := r.PathValue("videoID")

	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Extract the Bearer Token from the Authorization header
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Validate the JWT and extract the userID
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access to vidoe", err)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}

	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type", errors.New("from file header missing Content-Type"))
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access to video", nil)
		return
	}

	str, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to parse mediaType to variable", err)
		return
	}

	if str != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file format", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create a temp file", err)
		return
	}

	defer os.Remove(tempFile.Name())

	defer tempFile.Close()

	_, copyErr := io.Copy(tempFile, file)
	if copyErr != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy video to a new file", copyErr)
		return
	}

	tempFile.Seek(0, io.SeekStart)

	randomByte := make([]byte, 32)

	if _, err := rand.Read(randomByte); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unexpected error has occured while trying to generate a random number", err)
		return
	}

	fileName := hex.EncodeToString(randomByte)

	prefixString := "other"

	prefix, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "No file with such a name exists", err)
		return
	}

	if prefix == "16:9" {
		prefixString = "landscape"
	}
	if prefix == "9:16" {
		prefixString = "portrait"
	}

	keyValue := path.Join(prefixString, fileName+".mp4")

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(keyValue),
		Body:        tempFile,
		ContentType: aws.String(mediaType),
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create a new s3 bucket resource", err)
		return
	}

	newVideoUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, keyValue)

	video.VideoURL = &newVideoUrl

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update the vidoe", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)

}
