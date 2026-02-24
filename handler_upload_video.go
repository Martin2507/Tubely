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

	// Limit the request body size to 1 GB to prevent memory exhaustion
	const maxMemory = 1 << 30

	r.Body = http.MaxBytesReader(w, r.Body, maxMemory)

	// Extract and validate the video ID from the URL path parameters
	videoIDString := r.PathValue("videoID")

	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Authenticate the user by checking for a Bearer token in the headers
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Validate the JWT and retrieve the user's ID
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// Fetch the video metadata from the database to ensure it exists
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access to vidoe", err)
		return
	}

	// Ensure the user attempting the upload owns the video record
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access to video", nil)
		return
	}

	// Parse the multipart form to retrieve the uploaded video file
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}

	defer file.Close()

	// Validate the Content-Type of the uploaded file
	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type", errors.New("from file header missing Content-Type"))
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

	// Create a temporary file on disk to store the video for processing
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create a temp file", err)
		return
	}

	defer os.Remove(tempFile.Name())

	defer tempFile.Close()

	// Copy the uploaded file data into the temporary disk file
	_, copyErr := io.Copy(tempFile, file)
	if copyErr != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy video to a new file", copyErr)
		return
	}

	// Reset the read pointer to the beginning of the file before processing/uploading
	tempFile.Seek(0, io.SeekStart)

	// Generate a random 32-byte hex string to use as the unique S3 filename
	randomByte := make([]byte, 32)

	if _, err := rand.Read(randomByte); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unexpected error has occured while trying to generate a random number", err)
		return
	}

	fileName := hex.EncodeToString(randomByte)

	// Determine the aspect ratio to decide the S3 prefix (directory)
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

	// Construct the final S3 key using the prefix and random filename
	keyValue := path.Join(prefixString, fileName+".mp4")

	// Reset seek again before S3 upload
	tempFile.Seek(0, io.SeekStart)

	// Upload the video file to the S3 bucket
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

	// Update the database with the new S3 URL
	newVideoUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, keyValue)

	video.VideoURL = &newVideoUrl

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update the vidoe", err)
		return
	}

	// Return the updated video metadata to the client
	respondWithJSON(w, http.StatusOK, video)

}
