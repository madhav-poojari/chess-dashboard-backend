package v1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/madhava-poojari/dashboard-api/internal/auth"
	"github.com/madhava-poojari/dashboard-api/internal/service"
	"github.com/madhava-poojari/dashboard-api/internal/store"
	"github.com/madhava-poojari/dashboard-api/internal/utils"
)

type RatingHandler struct {
	store *store.Store
}

func NewRatingHandler(s serviceStore) *RatingHandler {
	return &RatingHandler{store: s.Store}
}

// GET /ratings/{studentId}/{platform}
// Returns rating history for a specific student on a specific platform.
// Allowed platforms: chesscom, lichess, fide, uscf
func (h *RatingHandler) GetStudentPlatformRatings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	current := auth.GetUserFromCtx(ctx)
	if current == nil {
		utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "unauthorized", nil, nil)
		return
	}

	studentID := chi.URLParam(r, "studentId")
	platform := chi.URLParam(r, "platform")

	if studentID == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "missing studentId", nil, nil)
		return
	}

	// Validate platform
	validPlatforms := map[string]bool{"chesscom": true, "lichess": true, "fide": true, "uscf": true}
	if !validPlatforms[platform] {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid platform, must be one of: chesscom, lichess, fide, uscf", nil, nil)
		return
	}

	// Access check: student viewing self, coach/mentor of student, or admin
	if !CanAccessStudentData(ctx, h.store, current, studentID) {
		utils.WriteJSONResponse(w, http.StatusForbidden, false, "forbidden", nil, nil)
		return
	}

	records, err := h.store.GetRatingHistory(ctx, studentID, platform)
	if err != nil {
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "error fetching ratings", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "ok", records, nil)
}

// POST /ratings/trigger-scrape
// Admin-only endpoint to manually trigger a full scrape cycle.
func (h *RatingHandler) TriggerScrape(w http.ResponseWriter, r *http.Request) {
	// Run scrapes in background goroutines
	go func() {
		service.RunWeeklyRatingScrape(h.store)
		service.RunMonthlyFIDEScrape(h.store)
	}()

	utils.WriteJSONResponse(w, http.StatusOK, true, "scrape triggered in background", nil, nil)
}
