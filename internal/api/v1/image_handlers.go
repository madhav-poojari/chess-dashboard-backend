package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/madhava-poojari/dashboard-api/internal/auth"
	"github.com/madhava-poojari/dashboard-api/internal/config"
	"github.com/madhava-poojari/dashboard-api/internal/models"
	"github.com/madhava-poojari/dashboard-api/internal/utils"
	"gorm.io/datatypes"
)

type ImageHandler struct {
	store        serviceStore
	imageStorage *utils.R2Storage
	cfg          *config.Config
}

func NewImageHandler(store serviceStore, cfg *config.Config) *ImageHandler {
	return &ImageHandler{
		store:        store,
		imageStorage: utils.NewR2Storage(cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Endpoint, cfg.R2BucketName),
		cfg:          cfg,
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

	if !CanAccessStudentData(ctx, h.store.Store, current, id) {
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
		_ = h.imageStorage.DeleteFile(user.UserDetails.ProfilePictureURL)
	}

	// Save new file
	urlSuffix, err := h.imageStorage.SaveFile("profile-pictures", header.Filename, file)
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

	fullURL, _ := h.imageStorage.PresignGetObject(urlSuffix, 1*time.Hour)
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

	if !CanAccessStudentData(ctx, h.store.Store, current, id) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	user, err := h.store.GetUserByID(ctx, id)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusNotFound, false, "user not found", nil, nil)
		return
	}

	if user.UserDetails.ProfilePictureURL != "" {
		_ = h.imageStorage.DeleteFile(user.UserDetails.ProfilePictureURL)
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

	if !CanAccessStudentData(ctx, h.store.Store, current, id) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	images, err := h.store.ListGalleryImages(ctx, id)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to list gallery", nil, err.Error())
		return
	}

	// Build presigned URLs
	type imageResp struct {
		models.Image
		URL string `json:"url"`
	}
	resp := make([]imageResp, len(images))
	for i, img := range images {
		presignedURL, _ := h.imageStorage.PresignGetObject(img.URLSuffix, 1*time.Hour)
		resp[i] = imageResp{
			Image: img,
			URL:   presignedURL,
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

	// Coach, mentor, admin, or the student themselves can upload
	if !CanAccessStudentData(ctx, h.store.Store, current, id) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
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
	urlSuffix, err := h.imageStorage.SaveFile(subDir, header.Filename, file)
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

	// Parse tags from JSON string (e.g. '["tag1","tag2"]')
	var tags datatypes.JSON
	tagsStr := r.FormValue("tags")
	if tagsStr != "" {
		tags = datatypes.JSON(tagsStr)
	} else {
		tags = datatypes.JSON("[]")
	}

	isPrivate := r.FormValue("is_private") == "true"

	img := &models.Image{
		UserID:    id,
		URLSuffix: urlSuffix,
		Title:     title,
		Tags:      tags,
		IsPrivate: isPrivate,
	}

	if err := h.store.CreateImage(ctx, img); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to save image record", nil, err.Error())
		return
	}

	fullURL, _ := h.imageStorage.PresignGetObject(urlSuffix, 1*time.Hour)
	utils.WriteJSONResponse(w, http.StatusCreated, true, "gallery image uploaded", map[string]interface{}{
		"id":         img.ID,
		"url_suffix": urlSuffix,
		"url":        fullURL,
	}, nil)
}

// DELETE /users/{id}/gallery/{imageId}
// Admin-only: only admins can delete gallery images from cloud storage.
func (h *ImageHandler) DeleteGalleryImage(w http.ResponseWriter, r *http.Request) {
	imageIdStr := chi.URLParam(r, "imageId")
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	// Only admin can delete gallery images
	if current.Role != models.RoleAdmin {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "only admins can delete gallery images", nil, nil)
		return
	}

	imageId, err := strconv.ParseUint(imageIdStr, 10, 32)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid image id", nil, nil)
		return
	}

	_, err = h.store.GetImageByID(ctx, uint(imageId))
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusNotFound, false, "image not found", nil, nil)
		return
	}

	// Soft delete: do NOT delete file from R2, just mark as deleted in DB

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

	// Coach, mentor, admin, or the student themselves can edit
	if !CanAccessStudentData(ctx, h.store.Store, current, id) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	imageId, err := strconv.ParseUint(imageIdStr, 10, 32)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid image id", nil, nil)
		return
	}

	// Parse JSON body
	var body struct {
		Title     *string          `json:"title"`
		Tags      *json.RawMessage `json:"tags"`
		IsPrivate *bool            `json:"is_private"`
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
	if body.Tags != nil {
		fields["tags"] = datatypes.JSON(*body.Tags)
	}
	if body.IsPrivate != nil {
		fields["is_private"] = *body.IsPrivate
	}

	if len(fields) == 0 {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "no fields to update", nil, nil)
		return
	}

	if err := h.store.UpdateImageMetadata(ctx, id, uint(imageId), fields); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to update image", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "image metadata updated", nil, nil)
}

// GET /images/academy-gallery?page=1&page_size=12
// Returns all non-private images with pagination, presigned URLs, and uploader names.
func (h *ImageHandler) ListAcademyGallery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	// Parse pagination params
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")

	page := 1
	pageSize := 12
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 50 {
		pageSize = ps
	}

	images, total, err := h.store.ListAllPublicImages(ctx, page, pageSize)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to list gallery", nil, err.Error())
		return
	}

	// Collect unique user IDs to fetch names
	userIDs := map[string]bool{}
	for _, img := range images {
		userIDs[img.UserID] = true
	}

	// Fetch user names
	type userInfo struct {
		FirstName string
		LastName  string
	}
	userMap := map[string]userInfo{}
	for uid := range userIDs {
		u, err := h.store.GetUserByID(ctx, uid)
		if err == nil {
			userMap[uid] = userInfo{FirstName: u.FirstName, LastName: u.LastName}
		}
	}

	// Build response
	type imageResp struct {
		models.Image
		URL           string `json:"url"`
		UserFirstName string `json:"user_first_name"`
		UserLastName  string `json:"user_last_name"`
	}
	resp := make([]imageResp, len(images))
	for i, img := range images {
		presignedURL, _ := h.imageStorage.PresignGetObject(img.URLSuffix, 1*time.Hour)
		ui := userMap[img.UserID]
		resp[i] = imageResp{
			Image:         img,
			URL:           presignedURL,
			UserFirstName: ui.FirstName,
			UserLastName:  ui.LastName,
		}
	}

	totalPages := (int(total) + pageSize - 1) / pageSize

	utils.WriteJSONResponse(w, http.StatusOK, true, "success", map[string]interface{}{
		"images":      resp,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
	}, nil)
}
