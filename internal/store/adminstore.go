package store

import (
	"context"
	"strings"
	"time"

	"github.com/madhava-poojari/dashboard-api/internal/models"
	"gorm.io/gorm"
)

func (s *Store) ApproveUser(ctx context.Context, userID string) error {
	return s.DB.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{"approved": true, "updated_at": time.Now()}).Error
}

func (s *Store) ChangeUserRole(ctx context.Context, userID, role string) error {
	return s.DB.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Update("role", role).Error
}

func (s *Store) AddCoachStudent(ctx context.Context, coachID, studentID, mentorID string) error {
	cs := models.CoachStudent{CoachID: coachID, StudentID: studentID, MentorCoachID: mentorID}
	return s.DB.WithContext(ctx).Create(&cs).Error
}

func (s *Store) RemoveCoachStudent(ctx context.Context, coachID, studentID string) error {
	return s.DB.WithContext(ctx).Where("coach_id = ? AND student_id = ?", coachID, studentID).Delete(&models.CoachStudent{}).Error
}

// ListUnapprovedUsers returns users that are not approved, sorted by newest first
func (s *Store) ListUnapprovedUsers(ctx context.Context) ([]*models.User, error) {
	var res []*models.User
	if err := s.DB.WithContext(ctx).Preload("UserDetails").Where("approved = ?", false).Order("created_at desc").Find(&res).Error; err != nil {
		return nil, err
	}
	return res, nil
}

// StudentWithAssignment represents a student with their coach/mentor assignment info
type StudentWithAssignment struct {
	*models.User
	CoachID       *string    `json:"coach_id,omitempty"`
	MentorCoachID *string    `json:"mentor_coach_id,omitempty"`
	AssignedAt    *time.Time `json:"assigned_at,omitempty"`
}

// ListStudentsWithAssignments returns all students with their assignment info
func (s *Store) ListStudentsWithAssignments(ctx context.Context) ([]*StudentWithAssignment, error) {
	var students []models.User
	if err := s.DB.WithContext(ctx).Preload("UserDetails").Where("role = ?", "student").Find(&students).Error; err != nil {
		return nil, err
	}

	var assignments []struct {
		StudentID     string    `gorm:"column:student_id"`
		CoachID       string    `gorm:"column:coach_id"`
		MentorCoachID string    `gorm:"column:mentor_coach_id"`
		CreatedAt     time.Time `gorm:"column:created_at"`
	}
	if err := s.DB.WithContext(ctx).Table("coach_students").Find(&assignments).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Create a map of student_id -> assignment
	assignmentMap := make(map[string]struct {
		CoachID       string
		MentorCoachID string
		CreatedAt     time.Time
	})
	for _, a := range assignments {
		assignmentMap[a.StudentID] = struct {
			CoachID       string
			MentorCoachID string
			CreatedAt     time.Time
		}{
			CoachID:       a.CoachID,
			MentorCoachID: a.MentorCoachID,
			CreatedAt:     a.CreatedAt,
		}
	}

	result := make([]*StudentWithAssignment, 0, len(students))
	unassigned := make([]*StudentWithAssignment, 0)
	assigned := make([]*StudentWithAssignment, 0)

	for i := range students {
		swa := &StudentWithAssignment{User: &students[i]}
		if a, ok := assignmentMap[students[i].ID]; ok {
			swa.CoachID = &a.CoachID
			if a.MentorCoachID != "" {
				swa.MentorCoachID = &a.MentorCoachID
			}
			swa.AssignedAt = &a.CreatedAt
			assigned = append(assigned, swa)
		} else {
			unassigned = append(unassigned, swa)
		}
	}

	// Sort assigned by assigned_at desc (newest first)
	for i := 0; i < len(assigned)-1; i++ {
		for j := i + 1; j < len(assigned); j++ {
			if assigned[i].AssignedAt != nil && assigned[j].AssignedAt != nil {
				if assigned[i].AssignedAt.Before(*assigned[j].AssignedAt) {
					assigned[i], assigned[j] = assigned[j], assigned[i]
				}
			}
		}
	}

	// Unassigned first, then assigned (sorted by newest assignment)
	result = append(result, unassigned...)
	result = append(result, assigned...)

	return result, nil
}

// CoachWithAssignment represents a coach with their student assignment info
type CoachWithAssignment struct {
	*models.User
	StudentID     *string    `json:"student_id,omitempty"`
	IsMentor      bool       `json:"is_mentor"`
	MentorCoachID *string    `json:"mentor_coach_id,omitempty"` // Mentor assigned to this coach
	AssignedAt    *time.Time `json:"assigned_at,omitempty"`
}

// ListCoachesWithAssignments returns all coaches with their assignment info
func (s *Store) ListCoachesWithAssignments(ctx context.Context) ([]*CoachWithAssignment, error) {
	var coaches []models.User
	if err := s.DB.WithContext(ctx).Preload("UserDetails").Where("role IN ?", []string{"coach", "mentor"}).Find(&coaches).Error; err != nil {
		return nil, err
	}

	var assignments []struct {
		CoachID       string    `gorm:"column:coach_id"`
		MentorCoachID string    `gorm:"column:mentor_coach_id"`
		StudentID     string    `gorm:"column:student_id"`
		CreatedAt     time.Time `gorm:"column:created_at"`
	}
	if err := s.DB.WithContext(ctx).Table("coach_students").Find(&assignments).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// Create a map of coach_id -> assignments (can have multiple)
	assignmentMap := make(map[string][]struct {
		StudentID     string
		IsMentor      bool
		MentorCoachID string
		CreatedAt     time.Time
	})
	// Track default mentors for coaches (coaches with mentor but no students shown yet)
	coachMentorMap := make(map[string]string)

	for _, a := range assignments {
		// Add as coach assignment
		if a.CoachID != "" {
			assignmentMap[a.CoachID] = append(assignmentMap[a.CoachID], struct {
				StudentID     string
				IsMentor      bool
				MentorCoachID string
				CreatedAt     time.Time
			}{
				StudentID:     a.StudentID,
				IsMentor:      false,
				MentorCoachID: a.MentorCoachID,
				CreatedAt:     a.CreatedAt,
			})
			// Track mentor for this coach (use the most recent one if multiple)
			if a.MentorCoachID != "" {
				coachMentorMap[a.CoachID] = a.MentorCoachID
			}
		}
		// Add as mentor assignment
		if a.MentorCoachID != "" {
			assignmentMap[a.MentorCoachID] = append(assignmentMap[a.MentorCoachID], struct {
				StudentID     string
				IsMentor      bool
				MentorCoachID string
				CreatedAt     time.Time
			}{
				StudentID:     a.StudentID,
				IsMentor:      true,
				MentorCoachID: "",
				CreatedAt:     a.CreatedAt,
			})
		}
	}

	result := make([]*CoachWithAssignment, 0)
	unassigned := make([]*CoachWithAssignment, 0)
	assigned := make([]*CoachWithAssignment, 0)

	for i := range coaches {
		assignments := assignmentMap[coaches[i].ID]
		// Check if this coach has a default mentor (even without students)
		defaultMentor := coachMentorMap[coaches[i].ID]

		// Filter out tracking entries (they're just for storing mentor info, not real assignments)
		realAssignments := make([]struct {
			StudentID     string
			IsMentor      bool
			MentorCoachID string
			CreatedAt     time.Time
		}, 0)
		for _, a := range assignments {
			// Skip tracking entries (student_id starts with "T-")
			if !strings.HasPrefix(a.StudentID, "T-") {
				realAssignments = append(realAssignments, a)
			}
		}

		if len(realAssignments) == 0 {
			// Coach has no real student assignments, but might have a mentor
			cwa := &CoachWithAssignment{User: &coaches[i]}
			if defaultMentor != "" {
				cwa.MentorCoachID = &defaultMentor
			}
			unassigned = append(unassigned, cwa)
		} else {
			// Create one entry per assignment, sorted by newest first
			for _, a := range realAssignments {
				cwa := &CoachWithAssignment{
					User:       &coaches[i],
					StudentID:  &a.StudentID,
					IsMentor:   a.IsMentor,
					AssignedAt: &a.CreatedAt,
				}
				// Include mentor info if this is a coach assignment (not mentor assignment)
				if !a.IsMentor {
					// Use mentor from this specific assignment, or fall back to default mentor
					if a.MentorCoachID != "" {
						cwa.MentorCoachID = &a.MentorCoachID
					} else if defaultMentor != "" {
						// Coach has a default mentor (from tracking entry or other assignments)
						cwa.MentorCoachID = &defaultMentor
					}
				}
				assigned = append(assigned, cwa)
			}
		}
	}

	// Sort assigned by assigned_at desc (newest first)
	for i := 0; i < len(assigned)-1; i++ {
		for j := i + 1; j < len(assigned); j++ {
			if assigned[i].AssignedAt != nil && assigned[j].AssignedAt != nil {
				if assigned[i].AssignedAt.Before(*assigned[j].AssignedAt) {
					assigned[i], assigned[j] = assigned[j], assigned[i]
				}
			}
		}
	}

	// Unassigned first, then assigned (sorted by newest assignment)
	result = append(result, unassigned...)
	result = append(result, assigned...)

	return result, nil
}

// GetPendingApprovals is an alias for ListUnapprovedUsers (for backward compatibility)
func (s *Store) GetPendingApprovals(ctx context.Context) ([]*models.User, error) {
	return s.ListUnapprovedUsers(ctx)
}

// GetUsersByRole fetches users filtered by a specific role
func (s *Store) GetUsersByRole(ctx context.Context, role models.Role) ([]*models.User, error) {
	var users []*models.User
	if err := s.DB.WithContext(ctx).
		Preload("UserDetails").
		Where("role = ?", role).
		Order("created_at DESC").
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// GetAllStudents fetches all users with role student
func (s *Store) GetAllStudents(ctx context.Context) ([]*models.User, error) {
	return s.GetUsersByRole(ctx, models.RoleStudent)
}

// GetAllCoaches fetches users with role coach
func (s *Store) GetAllCoaches(ctx context.Context) ([]*models.User, error) {
	return s.GetUsersByRole(ctx, models.RoleCoach)
}

// GetAllMentorCoaches fetches users with role mentor
func (s *Store) GetAllMentorCoaches(ctx context.Context) ([]*models.User, error) {
	return s.GetUsersByRole(ctx, models.RoleMentor)
}

// GetAllUsersGrouped returns all users grouped by role for admin view
func (s *Store) GetAllUsersGrouped(ctx context.Context) (map[string][]*models.User, error) {
	result := make(map[string][]*models.User)

	students, err := s.GetAllStudents(ctx)
	if err != nil {
		return nil, err
	}
	result["students"] = students

	coaches, err := s.GetAllCoaches(ctx)
	if err != nil {
		return nil, err
	}
	result["coaches"] = coaches

	mentors, err := s.GetAllMentorCoaches(ctx)
	if err != nil {
		return nil, err
	}
	result["mentors"] = mentors

	return result, nil
}

func trackingStudentIDForCoach(coachID string) string {
	// Format: "T-" + last 8 chars of coach_id (max 10 chars total)
	coachIDLen := len(coachID)
	if coachIDLen <= 8 {
		return "T-" + coachID
	}
	return "T-" + coachID[coachIDLen-8:]
}

func getDefaultMentorForCoachTx(tx *gorm.DB, coachID string) (string, error) {
	var cs models.CoachStudent
	err := tx.
		Where("coach_id = ? AND mentor_coach_id != '' AND mentor_coach_id IS NOT NULL", coachID).
		First(&cs).Error
	if err == nil {
		return cs.MentorCoachID, nil
	}
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	return "", err
}

// SetStudentCoachAssignment assigns/moves/unassigns a student from a coach.
// If coachID == "", the assignment (if any) is removed.
// If coachID != "", the assignment is created or moved to the given coach.
//
// This also applies the coach's default mentor (if any) to the student's assignment
// and cleans up mentor tracking rows for the coach once a real student exists.
func (s *Store) SetStudentCoachAssignment(ctx context.Context, studentID, coachID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.CoachStudent
		err := tx.Where("student_id = ?", studentID).First(&existing).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		// Remove assignment
		if coachID == "" {
			if err == nil {
				return tx.Where("coach_id = ? AND student_id = ?", existing.CoachID, studentID).
					Delete(&models.CoachStudent{}).Error
			}
			return nil
		}

		defaultMentor, err := getDefaultMentorForCoachTx(tx, coachID)
		if err != nil {
			return err
		}

		// If already assigned to this coach, just ensure mentor follows default mentor (if any).
		if err == nil && existing.CoachID == coachID {
			return tx.Model(&models.CoachStudent{}).
				Where("coach_id = ? AND student_id = ?", coachID, studentID).
				Update("mentor_coach_id", defaultMentor).Error
		}

		// Move: delete old assignment if it exists
		if err == nil {
			if err := tx.Where("coach_id = ? AND student_id = ?", existing.CoachID, studentID).
				Delete(&models.CoachStudent{}).Error; err != nil {
				return err
			}
		}

		// Create or update the (coach_id, student_id) row
		var target models.CoachStudent
		targetErr := tx.Where("coach_id = ? AND student_id = ?", coachID, studentID).First(&target).Error
		if targetErr == nil {
			if err := tx.Model(&models.CoachStudent{}).
				Where("coach_id = ? AND student_id = ?", coachID, studentID).
				Update("mentor_coach_id", defaultMentor).Error; err != nil {
				return err
			}
		} else if targetErr == gorm.ErrRecordNotFound {
			if err := tx.Create(&models.CoachStudent{
				CoachID:       coachID,
				StudentID:     studentID,
				MentorCoachID: defaultMentor,
			}).Error; err != nil {
				return err
			}
		} else {
			return targetErr
		}

		// If there is a mentor tracking row for this coach, it's no longer needed once we have real students.
		trackingStudentID := trackingStudentIDForCoach(coachID)
		return tx.Where("coach_id = ? AND student_id = ?", coachID, trackingStudentID).
			Delete(&models.CoachStudent{}).Error
	})
}

// SetCoachMentorAssignment assigns/removes a mentor coach for a coach.
// If mentorCoachID == "", the mentor assignment is removed.
//
// If a coach has no real student rows yet, a tracking row (student_id="T-...") is used
// to remember the mentor assignment for future student assignments.
func (s *Store) SetCoachMentorAssignment(ctx context.Context, coachID, mentorCoachID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Apply mentor to all rows for this coach (includes tracking rows if present).
		if err := tx.Model(&models.CoachStudent{}).
			Where("coach_id = ?", coachID).
			Update("mentor_coach_id", mentorCoachID).Error; err != nil {
			return err
		}

		trackingStudentID := trackingStudentIDForCoach(coachID)

		// Removing mentor: also remove tracking entry.
		if mentorCoachID == "" {
			return tx.Where("coach_id = ? AND student_id = ?", coachID, trackingStudentID).
				Delete(&models.CoachStudent{}).Error
		}

		// Determine if coach has any real students (exclude tracking rows).
		var realCount int64
		if err := tx.Model(&models.CoachStudent{}).
			Where("coach_id = ? AND student_id NOT LIKE 'T-%'", coachID).
			Count(&realCount).Error; err != nil {
			return err
		}

		// If there are real students, tracking row is not needed; clean it up if it exists.
		if realCount > 0 {
			return tx.Where("coach_id = ? AND student_id = ?", coachID, trackingStudentID).
				Delete(&models.CoachStudent{}).Error
		}

		// No real students: upsert tracking row to persist mentor assignment.
		var existing models.CoachStudent
		err := tx.Where("coach_id = ? AND student_id = ?", coachID, trackingStudentID).First(&existing).Error
		if err == nil {
			return tx.Model(&models.CoachStudent{}).
				Where("coach_id = ? AND student_id = ?", coachID, trackingStudentID).
				Update("mentor_coach_id", mentorCoachID).Error
		}
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&models.CoachStudent{
				CoachID:       coachID,
				StudentID:     trackingStudentID,
				MentorCoachID: mentorCoachID,
			}).Error
		}
		return err
	})
}
