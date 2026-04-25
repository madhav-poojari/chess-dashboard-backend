package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/madhava-poojari/dashboard-api/internal/models"
)

// addOneHour adds 1 hour to a "HH:MM" or "HH:MM:SS" time string and returns "HH:MM".
func addOneHour(t string) string {
	parts := strings.Split(t, ":")
	h, _ := strconv.Atoi(parts[0])
	m := 0
	if len(parts) > 1 {
		m, _ = strconv.Atoi(parts[1])
	}
	h += 1
	if h >= 24 {
		h -= 24
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

// CreateSchedule inserts a new class schedule slot after validating no overlap.
func (s *Store) CreateSchedule(ctx context.Context, cs *models.ClassSchedule) error {
	endTime := addOneHour(cs.StartTime)
	overlap, err := s.CheckScheduleOverlap(ctx, cs.StudentID, cs.DayOfWeek, cs.StartTime, endTime, 0)
	if err != nil {
		return err
	}
	if overlap {
		return fmt.Errorf("time slot overlaps with an existing slot for this student")
	}
	return s.DB.WithContext(ctx).Create(cs).Error
}

// GetScheduleByID fetches a single schedule slot (used to extract student_id for permission checks).
func (s *Store) GetScheduleByID(ctx context.Context, id uint) (*models.ClassSchedule, error) {
	var cs models.ClassSchedule
	if err := s.DB.WithContext(ctx).Preload("Student").First(&cs, id).Error; err != nil {
		return nil, err
	}
	return &cs, nil
}

// UpdateScheduleByID applies partial updates to a schedule slot after validating no overlap.
func (s *Store) UpdateScheduleByID(ctx context.Context, id uint, updates map[string]interface{}) (*models.ClassSchedule, error) {
	// Fetch existing to merge fields for overlap check
	existing, err := s.GetScheduleByID(ctx, id)
	if err != nil {
		return nil, err
	}

	dayOfWeek := existing.DayOfWeek
	startTime := existing.StartTime

	if v, ok := updates["day_of_week"]; ok {
		dayOfWeek = v.(int)
	}
	if v, ok := updates["start_time"]; ok {
		startTime = v.(string)
	}

	endTime := addOneHour(startTime)

	overlap, err := s.CheckScheduleOverlap(ctx, existing.StudentID, dayOfWeek, startTime, endTime, id)
	if err != nil {
		return nil, err
	}
	if overlap {
		return nil, fmt.Errorf("time slot overlaps with an existing slot for this student")
	}

	if err := s.DB.WithContext(ctx).Model(&models.ClassSchedule{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetScheduleByID(ctx, id)
}

// DeleteScheduleByID deletes a schedule slot.
func (s *Store) DeleteScheduleByID(ctx context.Context, id uint) error {
	return s.DB.WithContext(ctx).Where("id = ?", id).Delete(&models.ClassSchedule{}).Error
}

// ListSchedulesForStudents returns all schedule slots for the given list of student IDs.
// Preloads the Student relation. If studentIDs is empty, returns nil.
func (s *Store) ListSchedulesForStudents(ctx context.Context, studentIDs []string) ([]*models.ClassSchedule, error) {
	if len(studentIDs) == 0 {
		return nil, nil
	}
	var out []*models.ClassSchedule
	err := s.DB.WithContext(ctx).
		Preload("Student").
		Where("student_id IN ?", studentIDs).
		Order("day_of_week, start_time").
		Find(&out).Error
	return out, err
}

// CheckScheduleOverlap returns true if a conflicting slot exists for the same student, same day, overlapping time.
func (s *Store) CheckScheduleOverlap(ctx context.Context, studentID string, dayOfWeek int, startTime, endTime string, excludeID uint) (bool, error) {
	var count int64

	// Cleaner approach: use raw SQL for the overlap check
	q := s.DB.WithContext(ctx).Model(&models.ClassSchedule{}).
		Where("student_id = ? AND day_of_week = ?", studentID, dayOfWeek).
		Where("start_time < ?", endTime).
		Where("start_time > ?", subtractOneHour(startTime))

	if excludeID > 0 {
		q = q.Where("id != ?", excludeID)
	}

	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// subtractOneHour subtracts 1 hour from a "HH:MM" time string.
// Used for overlap check: existing_start + 1hr > new_start <==> existing_start > new_start - 1hr
func subtractOneHour(t string) string {
	parts := strings.Split(t, ":")
	h, _ := strconv.Atoi(parts[0])
	m := 0
	if len(parts) > 1 {
		m, _ = strconv.Atoi(parts[1])
	}
	h -= 1
	if h < 0 {
		h += 24
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}
