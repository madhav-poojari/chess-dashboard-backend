package store

import (
	"context"

	"github.com/madhava-poojari/dashboard-api/internal/models"
)

// CreateImage inserts a new gallery image record.
func (s *Store) CreateImage(ctx context.Context, img *models.Image) error {
	return s.DB.WithContext(ctx).Create(img).Error
}

// ListGalleryImages returns all gallery images for a user.
// All images (including private) are returned; is_private only affects the academy gallery.
func (s *Store) ListGalleryImages(ctx context.Context, userID string) ([]models.Image, error) {
	var images []models.Image
	err := s.DB.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&images).Error
	return images, err
}

// GetImageByID returns a single image by its primary key.
func (s *Store) GetImageByID(ctx context.Context, imageID uint) (*models.Image, error) {
	var img models.Image
	if err := s.DB.WithContext(ctx).First(&img, imageID).Error; err != nil {
		return nil, err
	}
	return &img, nil
}

// DeleteImage soft-deletes an image record by ID (sets deleted_at).
func (s *Store) DeleteImage(ctx context.Context, imageID uint) error {
	return s.DB.WithContext(ctx).Delete(&models.Image{}, imageID).Error
}

// UpdateImageMetadata updates the editable metadata fields of an image.
// Scoped to user_id so that only images belonging to the given user are updated.
func (s *Store) UpdateImageMetadata(ctx context.Context, userID string, imageID uint, fields map[string]interface{}) error {
	return s.DB.WithContext(ctx).Model(&models.Image{}).Where("id = ? AND user_id = ?", imageID, userID).Updates(fields).Error
}

// ListAllPublicImages returns non-private images across all users, with pagination.
// Returns images and total count.
func (s *Store) ListAllPublicImages(ctx context.Context, page, pageSize int) ([]models.Image, int64, error) {
	var total int64
	if err := s.DB.WithContext(ctx).Model(&models.Image{}).Where("is_private = ?", false).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var images []models.Image
	offset := (page - 1) * pageSize
	err := s.DB.WithContext(ctx).
		Where("is_private = ?", false).
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&images).Error
	return images, total, err
}
