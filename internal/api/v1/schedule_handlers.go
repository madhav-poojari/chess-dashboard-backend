package v1

import (
	"encoding/json"
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

	// Validate required fields
	if req.StudentID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "student_id is required", nil, nil)
		return
	}
	if req.DayOfWeek == nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "day_of_week is required", nil, nil)
		return
	}
	if *req.DayOfWeek < 0 || *req.DayOfWeek > 6 {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "day_of_week must be 0-6", nil, nil)
		return
	}
	if req.StartTime == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "start_time is required", nil, nil)
		return
	}
	if req.Timezone == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "timezone is required", nil, nil)
		return
	}

	// Permission check: can the current user manage this student?
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

	var out []*models.ClassSchedule
	var err error

	switch current.Role {
	case models.RoleAdmin:
		out, err = h.store.ListAllSchedules(ctx)
	case models.RoleCoach:
		out, err = h.store.ListSchedulesForCoach(ctx, current.ID)
	case models.RoleMentor:
		out, err = h.store.ListSchedulesForMentor(ctx, current.ID)
	default:
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

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

	// Permission check via CanAccessStudentData
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

	updates := map[string]interface{}{}
	if req.DayOfWeek != nil {
		if *req.DayOfWeek < 0 || *req.DayOfWeek > 6 {
			utils.WriteJSONResponse(w, http.StatusBadRequest, false, "day_of_week must be 0-6", nil, nil)
			return
		}
		updates["day_of_week"] = *req.DayOfWeek
	}
	if req.StartTime != nil {
		updates["start_time"] = normalizeTime(*req.StartTime)
	}
	if req.Timezone != nil {
		if *req.Timezone == "" {
			utils.WriteJSONResponse(w, http.StatusBadRequest, false, "timezone cannot be empty", nil, nil)
			return
		}
		updates["timezone"] = *req.Timezone
	}

	if len(updates) == 0 {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "no updates provided", nil, nil)
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

// GET /schedules/student/{studentId}
func (h *ScheduleHandler) GetStudentSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	studentID := chi.URLParam(r, "studentId")
	if studentID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "missing studentId", nil, nil)
		return
	}

	// Any authenticated user can view if they have access (student viewing self, coach/mentor/admin)
	if !CanAccessStudentData(ctx, h.store, current, studentID) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	out, err := h.store.ListSchedulesByStudent(ctx, studentID)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching schedule", nil, err.Error())
		return
	}
	utils.WriteJSONResponse(w, http.StatusOK, true, "ok", out, nil)
}
