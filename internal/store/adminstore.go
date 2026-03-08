package store

import (
	"context"
	"fmt"
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
	r := models.Relation{CoachID: coachID, UserID: studentID, MentorID: mentorID}
	return s.DB.WithContext(ctx).Create(&r).Error
}

func (s *Store) RemoveCoachStudent(ctx context.Context, coachID, studentID string) error {
	return s.DB.WithContext(ctx).Where("coach_id = ? AND user_id = ?", coachID, studentID).Delete(&models.Relation{}).Error
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

	var relations []models.Relation
	if err := s.DB.WithContext(ctx).Find(&relations).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	// user_id = student; only real assignments (skip tracking rows "T-...")
	assignmentMap := make(map[string]struct {
		CoachID       string
		MentorCoachID string
	})
	for _, r := range relations {
		if r.CoachID != "" && !strings.HasPrefix(r.UserID, "T-") {
			assignmentMap[r.UserID] = struct {
				CoachID       string
				MentorCoachID string
			}{CoachID: r.CoachID, MentorCoachID: r.MentorID}
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
			assigned = append(assigned, swa)
		} else {
			unassigned = append(unassigned, swa)
		}
	}

	// Unassigned first, then assigned
	result = append(result, unassigned...)
	result = append(result, assigned...)

	return result, nil
}

type CoachPickerItem struct {
    ID                string `json:"id"`
    FirstName         string `json:"first_name"`
    LastName          string `json:"last_name"`
    CurrentStudentCount int  `json:"current_student_count"`
}

// ListCoachesForPicker returns all active coaches with their real student count.
func (s *Store) ListCoachesForPicker(ctx context.Context) ([]*CoachPickerItem, error) {
    var coaches []models.User
    if err := s.DB.WithContext(ctx).
        Where("role = ? AND active = ?", "coach", true).
        Find(&coaches).Error; err != nil {
        return nil, err
    }

    var relations []models.Relation
    if err := s.DB.WithContext(ctx).Find(&relations).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }

    studentCountMap := make(map[string]int)
    for _, r := range relations {
        if r.CoachID != "" && !strings.HasPrefix(r.UserID, "T-") {
            studentCountMap[r.CoachID]++
        }
    }

    result := make([]*CoachPickerItem, 0, len(coaches))
    for _, c := range coaches {
        result = append(result, &CoachPickerItem{
            ID:                  c.ID,
            FirstName:           c.FirstName,
            LastName:            c.LastName,
            CurrentStudentCount: studentCountMap[c.ID],
        })
    }
    return result, nil
}

// ListMentorsForPicker returns all active mentors with their real student count.
// A mentor's student count = number of relations where MentorID = mentor's ID (excluding T- students).
func (s *Store) ListMentorsForPicker(ctx context.Context) ([]*CoachPickerItem, error) {
    // Step 1: fetch all active mentors
    var mentors []models.User
    if err := s.DB.WithContext(ctx).
        Where("role = ? AND active = ?", "mentor", true).
        Find(&mentors).Error; err != nil {
        return nil, err
    }

    // Step 2: fetch all relations
    var relations []models.Relation
    if err := s.DB.WithContext(ctx).Find(&relations).Error; err != nil && err != gorm.ErrRecordNotFound {
        return nil, err
    }

    // Step 3: count students per mentor
    // A mentor oversees a student when they appear as MentorID in a relation
    studentCountMap := make(map[string]int)
    for _, r := range relations {
        if r.MentorID != "" && !strings.HasPrefix(r.UserID, "T-") {
            studentCountMap[r.MentorID]++
        }
    }

    // Step 4: assemble result
    result := make([]*CoachPickerItem, 0, len(mentors))
    for _, m := range mentors {
        result = append(result, &CoachPickerItem{
            ID:                  m.ID,
            FirstName:           m.FirstName,
            LastName:            m.LastName,
            CurrentStudentCount: studentCountMap[m.ID],
        })
    }
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

	var relations []models.Relation
	if err := s.DB.WithContext(ctx).Find(&relations).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}

	assignmentMap := make(map[string][]struct {
		StudentID     string
		IsMentor      bool
		MentorCoachID string
	})
	coachMentorMap := make(map[string]string)

	for _, r := range relations {
		if r.CoachID != "" {
			assignmentMap[r.CoachID] = append(assignmentMap[r.CoachID], struct {
				StudentID     string
				IsMentor      bool
				MentorCoachID string
			}{StudentID: r.UserID, IsMentor: false, MentorCoachID: r.MentorID})
			if r.MentorID != "" {
				coachMentorMap[r.CoachID] = r.MentorID
			}
		}
		if r.MentorID != "" {
			assignmentMap[r.MentorID] = append(assignmentMap[r.MentorID], struct {
				StudentID     string
				IsMentor      bool
				MentorCoachID string
			}{StudentID: r.UserID, IsMentor: true, MentorCoachID: ""})
		}
	}

	result := make([]*CoachWithAssignment, 0)
	unassigned := make([]*CoachWithAssignment, 0)
	assigned := make([]*CoachWithAssignment, 0)

	for i := range coaches {
		var real []struct {
			StudentID     string
			IsMentor      bool
			MentorCoachID string
		}
		for _, a := range assignmentMap[coaches[i].ID] {
			if !strings.HasPrefix(a.StudentID, "T-") {
				real = append(real, a)
			}
		}
		defaultMentor := coachMentorMap[coaches[i].ID]

		if len(real) == 0 {
			cwa := &CoachWithAssignment{User: &coaches[i]}
			if defaultMentor != "" {
				cwa.MentorCoachID = &defaultMentor
			}
			unassigned = append(unassigned, cwa)
		} else {
			for _, a := range real {
				cwa := &CoachWithAssignment{
					User:      &coaches[i],
					StudentID: &a.StudentID,
					IsMentor:  a.IsMentor,
				}
				if !a.IsMentor && (a.MentorCoachID != "" || defaultMentor != "") {
					m := a.MentorCoachID
					if m == "" {
						m = defaultMentor
					}
					cwa.MentorCoachID = &m
				}
				assigned = append(assigned, cwa)
			}
		}
	}

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

func trackingUserIDForCoach(coachID string) string {
	if len(coachID) <= 8 {
		return "T-" + coachID
	}
	return "T-" + coachID[len(coachID)-8:]
}

func getDefaultMentorForCoachTx(tx *gorm.DB, coachID string) (string, error) {
	var r models.Relation
	err := tx.Where("coach_id = ? AND mentor_id != '' AND mentor_id IS NOT NULL", coachID).First(&r).Error
	if err == nil {
		return r.MentorID, nil
	}
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	return "", err
}

// SetStudentCoachAssignment assigns/moves/unassigns a student from a coach.
func (s *Store) SetStudentCoachAssignment(ctx context.Context, studentID, coachID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.Relation
		err := tx.Where("user_id = ?", studentID).First(&existing).Error
		found := (err == nil)
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if coachID == "" {
			if found {
				return tx.Where("coach_id = ? AND user_id = ?", existing.CoachID, studentID).Delete(&models.Relation{}).Error
			}
			return nil
		}
		defaultMentor, err := getDefaultMentorForCoachTx(tx, coachID)
		if err != nil {
			return err
		}
		if found && existing.CoachID == coachID {
			return tx.Model(&models.Relation{}).Where("coach_id = ? AND user_id = ?", coachID, studentID).Update("mentor_id", defaultMentor).Error
		}
		if found {
			if err := tx.Where("coach_id = ? AND user_id = ?", existing.CoachID, studentID).Delete(&models.Relation{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&models.Relation{CoachID: coachID, UserID: studentID, MentorID: defaultMentor}).Error; err != nil {
			return err
		}
		trackingID := trackingUserIDForCoach(coachID)
		return tx.Where("coach_id = ? AND user_id = ?", coachID, trackingID).Delete(&models.Relation{}).Error
	})
}


func (s *Store) SetStudentMentor(ctx context.Context, studentID, mentorID string) error {
    return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        var existing models.Relation
        err := tx.Where("user_id = ?", studentID).First(&existing).Error

        if err != nil && err != gorm.ErrRecordNotFound {
            return err
        }

        // No existing row — need coach_id to create, can't assign mentor alone
        if err == gorm.ErrRecordNotFound {
            return fmt.Errorf("student has no coach assigned yet")
        }

        // Update mentor (empty string = unassign)
        return tx.Model(&models.Relation{}).
            Where("user_id = ?", studentID).
            Update("mentor_id", mentorID).Error
    })
}

// SetCoachMentorAssignment assigns/removes a mentor for a coach.
func (s *Store) SetCoachMentorAssignment(ctx context.Context, coachID, mentorCoachID string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Relation{}).Where("coach_id = ?", coachID).Update("mentor_id", mentorCoachID).Error; err != nil {
			return err
		}
		trackingID := trackingUserIDForCoach(coachID)
		if mentorCoachID == "" {
			return tx.Where("coach_id = ? AND user_id = ?", coachID, trackingID).Delete(&models.Relation{}).Error
		}
		var n int64
		if err := tx.Model(&models.Relation{}).Where("coach_id = ? AND user_id NOT LIKE 'T-%'", coachID).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return tx.Where("coach_id = ? AND user_id = ?", coachID, trackingID).Delete(&models.Relation{}).Error
		}
		var r models.Relation
		err := tx.Where("coach_id = ? AND user_id = ?", coachID, trackingID).First(&r).Error
		if err == nil {
			return tx.Model(&models.Relation{}).Where("coach_id = ? AND user_id = ?", coachID, trackingID).Update("mentor_id", mentorCoachID).Error
		}
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&models.Relation{CoachID: coachID, UserID: trackingID, MentorID: mentorCoachID}).Error
		}
		return err
	})
}
