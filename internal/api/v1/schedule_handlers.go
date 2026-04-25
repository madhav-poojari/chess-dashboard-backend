package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/madhava-poojari/dashboard-api/internal/auth"
	"github.com/madhava-poojari/dashboard-api/internal/models"
	"github.com/madhava-poojari/dashboard-api/internal/store"
	"github.com/madhava-poojari/dashboard-api/internal/utils"
)

// normalizeTime ensures time string is in HH:MM format.
func normalizeTime(t string) string {
	// Strip seconds if present: "HH:MM:SS" -> "HH:MM"
	if len(t) >= 5 {
		return t[:5]
	}
	return t
}

type ScheduleHandler struct {
	store *store.Store
}

func NewScheduleHandler(s serviceStore) *ScheduleHandler {
	return &ScheduleHandler{store: s.Store}
}

// ---- Validation helpers ----

// validateScheduleCreate validates the required fields for creating a schedule.
// Returns an error message string (empty if valid).
func validateScheduleCreate(studentID string, dayOfWeek *int, startTime, timezone string) string {
	if studentID == "" {
		return "student_id is required"
	}
	if dayOfWeek == nil {
		return "day_of_week is required"
	}
	if *dayOfWeek < 0 || *dayOfWeek > 6 {
		return "day_of_week must be 0-6"
	}
	if startTime == "" {
		return "start_time is required"
	}
	if timezone == "" {
		return "timezone is required"
	}
	return ""
}

// validateScheduleUpdate validates the partial update fields for a schedule.
// Returns the updates map and an error message string (empty if valid).
func validateScheduleUpdate(dayOfWeek *int, startTime *string, timezone *string) (map[string]interface{}, string) {
	updates := map[string]interface{}{}

	if dayOfWeek != nil {
		if *dayOfWeek < 0 || *dayOfWeek > 6 {
			return nil, "day_of_week must be 0-6"
		}
		updates["day_of_week"] = *dayOfWeek
	}
	if startTime != nil {
		updates["start_time"] = normalizeTime(*startTime)
	}
	if timezone != nil {
		if *timezone == "" {
			return nil, "timezone cannot be empty"
		}
		updates["timezone"] = *timezone
	}

	if len(updates) == 0 {
		return nil, "no updates provided"
	}
	return updates, ""
}

// ---- Handlers ----

// POST /schedules
func (h *ScheduleHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentID string `json:"student_id"`
		DayOfWeek *int   `json:"day_of_week"`
		StartTime string `json:"start_time"`
		Timezone  string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err.Error())
		return
	}

	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	// Validate
	if errMsg := validateScheduleCreate(req.StudentID, req.DayOfWeek, req.StartTime, req.Timezone); errMsg != "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, errMsg, nil, nil)
		return
	}

	// Permission check
	if !CanAccessStudentData(ctx, h.store, current, req.StudentID) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	cs := &models.ClassSchedule{
		StudentID: req.StudentID,
		DayOfWeek: *req.DayOfWeek,
		StartTime: normalizeTime(req.StartTime),
		Timezone:  req.Timezone,
	}

	if err := h.store.CreateSchedule(ctx, cs); err != nil {
		if strings.Contains(err.Error(), "overlaps") {
			utils.WriteJSONResponse(w, http.StatusBadRequest, false, err.Error(), nil, nil)
			return
		}
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "create schedule failed", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusCreated, true, "created", cs, nil)
}

// GET /schedules
func (h *ScheduleHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	var students []*models.User
	var err error

	switch current.Role {
	case models.RoleAdmin:
		students, err = h.store.ListActiveStudents(ctx, true)
	case models.RoleCoach, models.RoleMentor:
		students, err = h.store.ListStudentsForCoachOrMentor(ctx, current.ID)
	default:
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching students", nil, err.Error())
		return
	}

	// Extract student IDs
	studentIDs := make([]string, len(students))
	for i, s := range students {
		studentIDs[i] = s.ID
	}

	out, err := h.store.ListSchedulesForStudents(ctx, studentIDs)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching schedules", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "ok", out, nil)
}

// PATCH /schedules/{id}
func (h *ScheduleHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	idStr := chi.URLParam(r, "id")
	idU64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid id", nil, err.Error())
		return
	}

	// Fetch the slot to know which student it belongs to
	existing, err := h.store.GetScheduleByID(ctx, uint(idU64))
	if err != nil {
		if store.IsNotFound(err) {
			utils.WriteJSONResponse(w, http.StatusNotFound, false, "not found", nil, nil)
			return
		}
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error", nil, err.Error())
		return
	}

	// Permission check
	if !CanAccessStudentData(ctx, h.store, current, existing.StudentID) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	var req struct {
		DayOfWeek *int    `json:"day_of_week"`
		StartTime *string `json:"start_time"`
		Timezone  *string `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request", nil, err.Error())
		return
	}

	// Validate
	updates, errMsg := validateScheduleUpdate(req.DayOfWeek, req.StartTime, req.Timezone)
	if errMsg != "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, errMsg, nil, nil)
		return
	}

	updated, err := h.store.UpdateScheduleByID(ctx, uint(idU64), updates)
	if err != nil {
		if strings.Contains(err.Error(), "overlaps") {
			utils.WriteJSONResponse(w, http.StatusBadRequest, false, err.Error(), nil, nil)
			return
		}
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "update failed", nil, err.Error())
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "updated", updated, nil)
}

// DELETE /schedules/{id}
func (h *ScheduleHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	idStr := chi.URLParam(r, "id")
	idU64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid id", nil, err.Error())
		return
	}

	existing, err := h.store.GetScheduleByID(ctx, uint(idU64))
	if err != nil {
		if store.IsNotFound(err) {
			utils.WriteJSONResponse(w, http.StatusNotFound, false, "not found", nil, nil)
			return
		}
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error", nil, err.Error())
		return
	}

	if !CanAccessStudentData(ctx, h.store, current, existing.StudentID) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	if err := h.store.DeleteScheduleByID(ctx, uint(idU64)); err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "delete failed", nil, err.Error())
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "deleted", nil, nil)
}

// dummy-use fmt to satisfy import if needed elsewhere in package
var _ = fmt.Sprint
