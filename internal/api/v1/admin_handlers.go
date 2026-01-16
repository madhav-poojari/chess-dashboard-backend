package v1

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/madhava-poojari/dashboard-api/internal/models"
	"github.com/madhava-poojari/dashboard-api/internal/utils"
)

type AdminHandler struct {
	store serviceStore
}

func NewAdminHandler(store serviceStore) *AdminHandler {
	return &AdminHandler{store: store}
}

func (h *AdminHandler) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email    *string `json:"email,omitempty"`
		Role     *string `json:"role,omitempty"`
		Approved *bool   `json:"approved,omitempty"`
		Active   *bool   `json:"active,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request ", nil, err)
		return
	}
	userUpdates := map[string]interface{}{}
	if payload.Email != nil {
		userUpdates["email"] = *payload.Email
	}
	if payload.Role != nil {
		userUpdates["role"] = *payload.Role
	}
	if payload.Approved != nil {
		userUpdates["approved"] = *payload.Approved
	}
	if payload.Active != nil {
		userUpdates["active"] = *payload.Active
	}
	// Update user status logic here
	err := h.store.UpdateUserFields(r.Context(), chi.URLParam(r, "id"), userUpdates)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "couldnt process the updates ", nil, err)
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "updated", nil, nil)
}

// GetUnapprovedUsers returns list of unapproved users
func (h *AdminHandler) GetUnapprovedUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUnapprovedUsers(r.Context())
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching unapproved users", nil, err)
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "success", users, nil)
}

// GetStudentsWithAssignments returns all students with their assignment info
func (h *AdminHandler) GetStudentsWithAssignments(w http.ResponseWriter, r *http.Request) {
	students, err := h.store.ListStudentsWithAssignments(r.Context())
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching students", nil, err)
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "success", students, nil)
}

// GetCoachesWithAssignments returns all coaches with their assignment info
func (h *AdminHandler) GetCoachesWithAssignments(w http.ResponseWriter, r *http.Request) {
	coaches, err := h.store.ListCoachesWithAssignments(r.Context())
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching coaches", nil, err)
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "success", coaches, nil)
}

// ApproveUser approves a user
func (h *AdminHandler) ApproveUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "missing user id", nil, nil)
		return
	}

	err := h.store.ApproveUser(r.Context(), userID)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error approving user", nil, err)
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "user approved", nil, nil)
}

// AssignStudentToCoach assigns a student to a coach
func (h *AdminHandler) AssignStudentToCoach(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		CoachID   string `json:"coach_id"`
		StudentID string `json:"student_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err)
		return
	}

	if payload.CoachID == "" || payload.StudentID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "coach_id and student_id are required", nil, nil)
		return
	}

	// Check if assignment already exists
	var existing models.CoachStudent
	if err := h.store.Store.DB.WithContext(r.Context()).Where("coach_id = ? AND student_id = ?", payload.CoachID, payload.StudentID).First(&existing).Error; err == nil {
		utils.WriteJSONResponse(w, http.StatusConflict, false, "assignment already exists", nil, nil)
		return
	}

	// Check if this coach has a default mentor assigned (check any existing student assignments)
	var defaultMentor string
	var mentorCheck models.CoachStudent
	if err := h.store.Store.DB.WithContext(r.Context()).
		Where("coach_id = ? AND mentor_coach_id != '' AND mentor_coach_id IS NOT NULL", payload.CoachID).
		First(&mentorCheck).Error; err == nil {
		defaultMentor = mentorCheck.MentorCoachID
	}

	err := h.store.AddCoachStudent(r.Context(), payload.CoachID, payload.StudentID, defaultMentor)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error assigning student", nil, err)
		return
	}

	// Clean up any tracking entries for this coach (they're no longer needed once we have real students)
	// Format: "T-" + last 8 chars of coach_id
	coachIDLen := len(payload.CoachID)
	var trackingStudentID string
	if coachIDLen <= 8 {
		trackingStudentID = "T-" + payload.CoachID
	} else {
		trackingStudentID = "T-" + payload.CoachID[coachIDLen-8:]
	}
	h.store.Store.DB.WithContext(r.Context()).
		Where("coach_id = ? AND student_id = ?", payload.CoachID, trackingStudentID).
		Delete(&models.CoachStudent{})

	utils.WriteJSONResponse(w, http.StatusOK, true, "student assigned to coach", nil, nil)
}

// AssignCoachAsMentor assigns a mentor coach to a coach
// If student_id is provided, assigns mentor to that specific student-coach relationship
// If student_id is empty but coach_id is provided, assigns mentor to all existing and future students of that coach
func (h *AdminHandler) AssignCoachAsMentor(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		MentorCoachID string `json:"mentor_coach_id"`
		StudentID     string `json:"student_id"` // Optional: if empty, applies to all students of the coach
		CoachID       string `json:"coach_id"`   // Required: the coach to assign mentor to
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err)
		return
	}

	if payload.MentorCoachID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "mentor_coach_id is required", nil, nil)
		return
	}

	if payload.CoachID == "" && payload.StudentID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "either coach_id or student_id is required", nil, nil)
		return
	}

	// If student_id is provided, handle specific student-coach relationship
	if payload.StudentID != "" {
		// Find existing coach-student relationship for this student
		var existing models.CoachStudent
		err := h.store.Store.DB.WithContext(r.Context()).
			Where("student_id = ?", payload.StudentID).
			First(&existing).Error

		if err == nil {
			// Update existing relationship to add/update mentor
			// Use payload.CoachID if provided, otherwise use existing.CoachID
			coachIDToUse := payload.CoachID
			if coachIDToUse == "" {
				coachIDToUse = existing.CoachID
			}
			updateData := map[string]interface{}{"mentor_coach_id": payload.MentorCoachID}
			err = h.store.Store.DB.WithContext(r.Context()).Model(&models.CoachStudent{}).
				Where("coach_id = ? AND student_id = ?", coachIDToUse, payload.StudentID).
				Updates(updateData).Error
			if err != nil {
				utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error assigning mentor", nil, err)
				return
			}
		} else {
			// No existing relationship - if coach_id is provided, create with that coach and mentor
			if payload.CoachID != "" {
				err = h.store.AddCoachStudent(r.Context(), payload.CoachID, payload.StudentID, payload.MentorCoachID)
			} else {
				utils.WriteJSONResponse(w, http.StatusBadRequest, false, "coach_id is required when creating new student-coach relationship", nil, nil)
				return
			}
			if err != nil {
				utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error assigning mentor", nil, err)
				return
			}
		}
	} else {
		// No student_id provided - assign mentor to all existing students of this coach
		// Update all existing coach_students entries for this coach
		updateData := map[string]interface{}{"mentor_coach_id": payload.MentorCoachID}
		result := h.store.Store.DB.WithContext(r.Context()).Model(&models.CoachStudent{}).
			Where("coach_id = ?", payload.CoachID).
			Updates(updateData)
		if result.Error != nil {
			utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error assigning mentor to coach", nil, result.Error)
			return
		}

		// If coach has no existing students, create a tracking entry to store the mentor assignment
		// We'll use a special student_id format to track this (e.g., "T-{coach_id_last_8_chars}")
		// This allows us to query for coaches with mentors even when they have no students
		// Note: student_id is limited to 10 chars, so we use "T-" prefix (2 chars) + last 8 chars of coach_id
		if result.RowsAffected == 0 {
			// Create a tracking entry - this will be replaced when real students are assigned
			// We use a special format for student_id that we can identify later
			// Format: "T-" + last 8 chars of coach_id (total max 10 chars)
			coachIDLen := len(payload.CoachID)
			var trackingStudentID string
			if coachIDLen <= 8 {
				trackingStudentID = "T-" + payload.CoachID
			} else {
				trackingStudentID = "T-" + payload.CoachID[coachIDLen-8:]
			}
			// Check if tracking entry already exists
			var existingTracking models.CoachStudent
			if err := h.store.Store.DB.WithContext(r.Context()).
				Where("coach_id = ? AND student_id = ?", payload.CoachID, trackingStudentID).
				First(&existingTracking).Error; err != nil {
				// Create new tracking entry
				err = h.store.AddCoachStudent(r.Context(), payload.CoachID, trackingStudentID, payload.MentorCoachID)
				if err != nil {
					utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error creating mentor tracking entry", nil, err)
					return
				}
			} else {
				// Update existing tracking entry
				err = h.store.Store.DB.WithContext(r.Context()).Model(&models.CoachStudent{}).
					Where("coach_id = ? AND student_id = ?", payload.CoachID, trackingStudentID).
					Updates(updateData).Error
				if err != nil {
					utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error updating mentor tracking entry", nil, err)
					return
				}
			}
		}
		// Note: Future student assignments will automatically get this mentor
		// This is handled in AssignStudentToCoach handler
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "coach assigned as mentor", nil, nil)
}

// GetMentorCoaches returns all mentor coaches (coaches with role "mentor")
func (h *AdminHandler) GetMentorCoaches(w http.ResponseWriter, r *http.Request) {
	coaches, err := h.store.ListCoachesWithAssignments(r.Context())
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to fetch mentor coaches", nil, err)
		return
	}
	// Filter to only mentor coaches
	mentors := make([]interface{}, 0)
	for _, coach := range coaches {
		if coach.Role == "mentor" {
			mentors = append(mentors, coach)
		}
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "mentor coaches fetched", mentors, nil)
}

// GetAllCoaches returns all coaches (for assignment dropdown)
func (h *AdminHandler) GetAllCoaches(w http.ResponseWriter, r *http.Request) {
	coaches, err := h.store.ListCoachesWithAssignments(r.Context())
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to fetch coaches", nil, err)
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "coaches fetched", coaches, nil)
}

// GetAdminDashboard returns all data needed for the admin dashboard in one call
func (h *AdminHandler) GetAdminDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get pending approvals
	pendingUsers, err := h.store.ListUnapprovedUsers(ctx)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to fetch pending approvals", nil, err)
		return
	}

	// Get students with assignments
	students, err := h.store.ListStudentsWithAssignments(ctx)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to fetch students", nil, err)
		return
	}

	// Get coaches with assignments
	coaches, err := h.store.ListCoachesWithAssignments(ctx)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to fetch coaches", nil, err)
		return
	}

	// Filter mentor coaches
	mentors := make([]interface{}, 0)
	for _, coach := range coaches {
		if coach.Role == "mentor" {
			mentors = append(mentors, coach)
		}
	}

	data := map[string]interface{}{
		"pending_approvals": pendingUsers,
		"students":          students,
		"coaches":           coaches,
		"mentor_coaches":    mentors,
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "admin dashboard data fetched", data, nil)
}

// UpdateStudentAssignment updates a student's coach assignment
func (h *AdminHandler) UpdateStudentAssignment(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		StudentID string `json:"student_id"`
		CoachID   string `json:"coach_id"` // Empty string to remove assignment
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err)
		return
	}

	if payload.StudentID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "student_id is required", nil, nil)
		return
	}

	ctx := r.Context()

	// Find existing assignment
	var existing models.CoachStudent
	err := h.store.Store.DB.WithContext(ctx).Where("student_id = ?", payload.StudentID).First(&existing).Error

	if payload.CoachID == "" {
		// Remove assignment
		if err == nil {
			err = h.store.RemoveCoachStudent(ctx, existing.CoachID, payload.StudentID)
			if err != nil {
				utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error removing assignment", nil, err)
				return
			}
		}
		utils.WriteJSONResponse(w, http.StatusOK, true, "assignment removed", nil, nil)
		return
	}

	// Update or create assignment
	if err == nil {
		// Update existing assignment
		// Check if coach has a default mentor
		var defaultMentor string
		var mentorCheck models.CoachStudent
		if err := h.store.Store.DB.WithContext(ctx).
			Where("coach_id = ? AND mentor_coach_id != '' AND mentor_coach_id IS NOT NULL", payload.CoachID).
			First(&mentorCheck).Error; err == nil {
			defaultMentor = mentorCheck.MentorCoachID
		}

		updateData := map[string]interface{}{
			"coach_id":        payload.CoachID,
			"mentor_coach_id": defaultMentor,
		}
		err = h.store.Store.DB.WithContext(ctx).Model(&models.CoachStudent{}).
			Where("student_id = ?", payload.StudentID).
			Updates(updateData).Error
		if err != nil {
			utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error updating assignment", nil, err)
			return
		}
	} else {
		// Create new assignment
		var defaultMentor string
		var mentorCheck models.CoachStudent
		if err := h.store.Store.DB.WithContext(ctx).
			Where("coach_id = ? AND mentor_coach_id != '' AND mentor_coach_id IS NOT NULL", payload.CoachID).
			First(&mentorCheck).Error; err == nil {
			defaultMentor = mentorCheck.MentorCoachID
		}

		err = h.store.AddCoachStudent(ctx, payload.CoachID, payload.StudentID, defaultMentor)
		if err != nil {
			utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error creating assignment", nil, err)
			return
		}
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "assignment updated", nil, nil)
}

// UpdateCoachMentorAssignment updates a coach's mentor assignment
func (h *AdminHandler) UpdateCoachMentorAssignment(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		CoachID       string `json:"coach_id"`
		MentorCoachID string `json:"mentor_coach_id"` // Empty string to remove assignment
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err)
		return
	}

	if payload.CoachID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "coach_id is required", nil, nil)
		return
	}

	ctx := r.Context()

	if payload.MentorCoachID == "" {
		// Remove mentor assignment from all students of this coach
		updateData := map[string]interface{}{"mentor_coach_id": ""}
		err := h.store.Store.DB.WithContext(ctx).Model(&models.CoachStudent{}).
			Where("coach_id = ?", payload.CoachID).
			Updates(updateData).Error
		if err != nil {
			utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error removing mentor assignment", nil, err)
			return
		}

		// Also remove from tracking entries
		coachIDLen := len(payload.CoachID)
		var trackingStudentID string
		if coachIDLen <= 8 {
			trackingStudentID = "T-" + payload.CoachID
		} else {
			trackingStudentID = "T-" + payload.CoachID[coachIDLen-8:]
		}
		h.store.Store.DB.WithContext(ctx).
			Where("coach_id = ? AND student_id = ?", payload.CoachID, trackingStudentID).
			Delete(&models.CoachStudent{})

		utils.WriteJSONResponse(w, http.StatusOK, true, "mentor assignment removed", nil, nil)
		return
	}

	// Update mentor for all students of this coach
	updateData := map[string]interface{}{"mentor_coach_id": payload.MentorCoachID}
	result := h.store.Store.DB.WithContext(ctx).Model(&models.CoachStudent{}).
		Where("coach_id = ?", payload.CoachID).
		Updates(updateData)

	if result.Error != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error updating mentor assignment", nil, result.Error)
		return
	}

	// If coach has no existing students, create a tracking entry
	if result.RowsAffected == 0 {
		coachIDLen := len(payload.CoachID)
		var trackingStudentID string
		if coachIDLen <= 8 {
			trackingStudentID = "T-" + payload.CoachID
		} else {
			trackingStudentID = "T-" + payload.CoachID[coachIDLen-8:]
		}

		var existingTracking models.CoachStudent
		if err := h.store.Store.DB.WithContext(ctx).
			Where("coach_id = ? AND student_id = ?", payload.CoachID, trackingStudentID).
			First(&existingTracking).Error; err != nil {
			// Create new tracking entry
			err = h.store.AddCoachStudent(ctx, payload.CoachID, trackingStudentID, payload.MentorCoachID)
			if err != nil {
				utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error creating mentor tracking entry", nil, err)
				return
			}
		} else {
			// Update existing tracking entry
			err = h.store.Store.DB.WithContext(ctx).Model(&models.CoachStudent{}).
				Where("coach_id = ? AND student_id = ?", payload.CoachID, trackingStudentID).
				Updates(updateData).Error
			if err != nil {
				utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error updating mentor tracking entry", nil, err)
				return
			}
		}
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "mentor assignment updated", nil, nil)
}

// ResetUserPassword resets a user's password (admin only)
func (h *AdminHandler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID   string `json:"user_id"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err)
		return
	}

	if payload.UserID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "user_id is required", nil, nil)
		return
	}

	if payload.Password == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "password is required", nil, nil)
		return
	}

	// Hash the new password
	hash, err := utils.HashPassword(payload.Password)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error hashing password", nil, err)
		return
	}

	// Update user's password
	err = h.store.UpdateUserFields(r.Context(), payload.UserID, map[string]interface{}{
		"password_hash": hash,
	})
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error updating password", nil, err)
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "password reset successfully", nil, nil)
}
