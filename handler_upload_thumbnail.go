package main

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {

	// Extract the videoID from the URL path parameters.
	videoIDString := r.PathValue("videoID")

	// Convert the raw string from the URL into a structured UUID.
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Extract the Bearer Token from the Authorization header.
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	// Validate the JWT and extract the userID.
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// Parse the incoming request as "multipart/form-data".
	const maxMemory = 10 << 20
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "couldn't parse multipart form", err)
		return
	}

	// Access the specific file labeled "thumbnail" from the form.
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}

	// Ensure the file handle is closed when the function finishes to prevent memory leaks.
	defer file.Close()

	// Retrieve the Media Type (e.g., "image/png").
	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type", errors.New("form file header missing Content-Type"))
		return
	}

	// Read the entire file content into a byte slice (imgData).
	// imgData, err := io.ReadAll(file)
	// if err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "Unable to read thumbnail data", err)
	// 	return
	// }

	// Look up the video in our database.
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to find video", err)
		return
	}

	// Authorization check.
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized access to video", nil)
		return
	}

	// Encode the thumbnail
	// encoded := base64.StdEncoding.EncodeToString(imgData)
	// if encoded == "" {
	// 	respondWithError(w, http.StatusInternalServerError, "Unable to encode the thumbnail", nil)
	// 	return
	// }

	imageExtension := strings.Split(mediaType, "/")

	str, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to parse mediaType to variable", err)
		return
	}

	if str != "image/jpeg" && str != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid file format", nil)
		return
	}

	filePath := filepath.Join(cfg.assetsRoot, video.ID.String()+"."+imageExtension[1])

	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create a new file", err)
		return
	}

	// Ensure the file handle is closed when the function finishes to prevent memory leaks.
	defer newFile.Close()

	_, newErr := io.Copy(newFile, file)
	if newErr != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy image to a new file", newErr)
		return
	}

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, videoID, imageExtension[1])

	video.ThumbnailURL = &thumbnailURL

	// Persist the changes to the database.
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
