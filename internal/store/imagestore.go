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
// If isOwner is false, private images are excluded.
func (s *Store) ListGalleryImages(ctx context.Context, userID string, isOwner bool) ([]models.Image, error) {
	var images []models.Image
	q := s.DB.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC")
	if !isOwner {
		q = q.Where("is_private = ?", false)
	}
	err := q.Find(&images).Error
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

// DeleteImage hard-deletes an image record by ID.
func (s *Store) DeleteImage(ctx context.Context, imageID uint) error {
	return s.DB.WithContext(ctx).Unscoped().Delete(&models.Image{}, imageID).Error
}

// UpdateImageMetadata updates the editable metadata fields of an image.
func (s *Store) UpdateImageMetadata(ctx context.Context, imageID uint, fields map[string]interface{}) error {
	return s.DB.WithContext(ctx).Model(&models.Image{}).Where("id = ?", imageID).Updates(fields).Error
}
