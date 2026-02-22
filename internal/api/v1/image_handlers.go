package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/madhava-poojari/dashboard-api/internal/auth"
	"github.com/madhava-poojari/dashboard-api/internal/config"
	"github.com/madhava-poojari/dashboard-api/internal/models"
	"github.com/madhava-poojari/dashboard-api/internal/utils"
)

type ImageHandler struct {
	store   serviceStore
	storage *utils.FileStorage
	cfg     *config.Config
}

func NewImageHandler(store serviceStore, cfg *config.Config) *ImageHandler {
	return &ImageHandler{
		store:   store,
		storage: utils.NewFileStorage(cfg.UploadDir),
		cfg:     cfg,
	}
}

// POST /users/{id}/profile-picture
func (h *ImageHandler) UploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	coachId, mentorId, _ := h.store.GetCoachesByStudentID(ctx, id)
	if !CanAccessStudentData(current, id, coachId, mentorId) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	// Max 5MB
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "file too large or invalid form", nil, err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "missing file field", nil, err.Error())
		return
	}
	defer file.Close()

	// Delete old profile picture file if exists
	user, err := h.store.GetUserByID(ctx, id)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusNotFound, false, "user not found", nil, nil)
		return
	}
	if user.UserDetails.ProfilePictureURL != "" {
		_ = h.storage.DeleteFile(user.UserDetails.ProfilePictureURL)
	}

	// Save new file
	urlSuffix, err := h.storage.SaveFile("profile-pictures", header.Filename, file)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to save file", nil, err.Error())
		return
	}

	// Update user_details.profile_picture_url
	if err := h.store.UpdateUserDetailsFields(ctx, id, map[string]interface{}{
		"profile_picture_url": urlSuffix,
	}); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to update profile", nil, err.Error())
		return
	}

	fullURL := fmt.Sprintf("%s/uploads/%s", h.cfg.UploadBaseURL, urlSuffix)
	utils.WriteJSONResponse(w, http.StatusOK, true, "profile picture uploaded", map[string]string{
		"url_suffix": urlSuffix,
		"url":        fullURL,
	}, nil)
}

// DELETE /users/{id}/profile-picture
func (h *ImageHandler) DeleteProfilePicture(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	coachId, mentorId, _ := h.store.GetCoachesByStudentID(ctx, id)
	if !CanAccessStudentData(current, id, coachId, mentorId) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	user, err := h.store.GetUserByID(ctx, id)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusNotFound, false, "user not found", nil, nil)
		return
	}

	if user.UserDetails.ProfilePictureURL != "" {
		_ = h.storage.DeleteFile(user.UserDetails.ProfilePictureURL)
	}

	if err := h.store.UpdateUserDetailsFields(ctx, id, map[string]interface{}{
		"profile_picture_url": "",
	}); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to update profile", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "profile picture deleted", nil, nil)
}

// GET /users/{id}/gallery
func (h *ImageHandler) ListGallery(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	coachId, mentorId, _ := h.store.GetCoachesByStudentID(ctx, id)
	if !CanAccessStudentData(current, id, coachId, mentorId) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	isOwner := current.ID == id
	images, err := h.store.ListGalleryImages(ctx, id, isOwner)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to list gallery", nil, err.Error())
		return
	}

	// Build full URLs
	type imageResp struct {
		models.Image
		URL string `json:"url"`
	}
	resp := make([]imageResp, len(images))
	for i, img := range images {
		resp[i] = imageResp{
			Image: img,
			URL:   fmt.Sprintf("%s/uploads/%s", h.cfg.UploadBaseURL, img.URLSuffix),
		}
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "success", resp, nil)
}

// POST /users/{id}/gallery
func (h *ImageHandler) UploadGalleryImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	// Only the owner can upload to their own gallery
	if current.ID != id {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "only the owner can upload gallery images", nil, nil)
		return
	}

	// Max 10MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "file too large or invalid form", nil, err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "missing file field", nil, err.Error())
		return
	}
	defer file.Close()

	subDir := fmt.Sprintf("gallery/%s", id)
	urlSuffix, err := h.storage.SaveFile(subDir, header.Filename, file)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to save file", nil, err.Error())
		return
	}

	// Read form fields
	title := r.FormValue("title")
	if title == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "title is required", nil, nil)
		return
	}
	positionInTournament := r.FormValue("position_in_tournament")
	isPrivate := r.FormValue("is_private") == "true"

	img := &models.Image{
		UserID:               id,
		URLSuffix:            urlSuffix,
		Filename:             header.Filename,
		Title:                title,
		PositionInTournament: positionInTournament,
		IsPrivate:            isPrivate,
	}

	if err := h.store.CreateImage(ctx, img); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to save image record", nil, err.Error())
		return
	}

	fullURL := fmt.Sprintf("%s/uploads/%s", h.cfg.UploadBaseURL, urlSuffix)
	utils.WriteJSONResponse(w, http.StatusCreated, true, "gallery image uploaded", map[string]interface{}{
		"id":         img.ID,
		"url_suffix": urlSuffix,
		"url":        fullURL,
	}, nil)
}

// DELETE /users/{id}/gallery/{imageId}
func (h *ImageHandler) DeleteGalleryImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	imageIdStr := chi.URLParam(r, "imageId")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	// Only the owner can delete their gallery images
	if current.ID != id {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "only the owner can delete gallery images", nil, nil)
		return
	}

	imageId, err := strconv.ParseUint(imageIdStr, 10, 32)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid image id", nil, nil)
		return
	}

	img, err := h.store.GetImageByID(ctx, uint(imageId))
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusNotFound, false, "image not found", nil, nil)
		return
	}

	// Verify the image belongs to this user
	if img.UserID != id {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	// Delete file from disk
	_ = h.storage.DeleteFile(img.URLSuffix)

	// Delete DB record
	if err := h.store.DeleteImage(ctx, uint(imageId)); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to delete image", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "gallery image deleted", nil, nil)
}

// PATCH /users/{id}/gallery/{imageId}
func (h *ImageHandler) UpdateGalleryImageMetadata(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	imageIdStr := chi.URLParam(r, "imageId")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	// Only the owner can edit their gallery images
	if current.ID != id {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "only the owner can edit gallery images", nil, nil)
		return
	}

	imageId, err := strconv.ParseUint(imageIdStr, 10, 32)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid image id", nil, nil)
		return
	}

	img, err := h.store.GetImageByID(ctx, uint(imageId))
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusNotFound, false, "image not found", nil, nil)
		return
	}

	if img.UserID != id {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	// Parse JSON body
	var body struct {
		Title                *string `json:"title"`
		PositionInTournament *string `json:"position_in_tournament"`
		IsPrivate            *bool   `json:"is_private"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request body", nil, err.Error())
		return
	}

	fields := map[string]interface{}{}
	if body.Title != nil {
		if *body.Title == "" {
			utils.WriteJSONResponse(w, http.StatusBadRequest, false, "title cannot be empty", nil, nil)
			return
		}
		fields["title"] = *body.Title
	}
	if body.PositionInTournament != nil {
		fields["position_in_tournament"] = *body.PositionInTournament
	}
	if body.IsPrivate != nil {
		fields["is_private"] = *body.IsPrivate
	}

	if len(fields) == 0 {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "no fields to update", nil, nil)
		return
	}

	if err := h.store.UpdateImageMetadata(ctx, uint(imageId), fields); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to update image", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "image metadata updated", nil, nil)
}
