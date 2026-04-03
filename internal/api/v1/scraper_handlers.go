package v1

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/madhava-poojari/dashboard-api/internal/config"
	"github.com/madhava-poojari/dashboard-api/internal/models"
	"github.com/madhava-poojari/dashboard-api/internal/utils"
)

type ScraperHandler struct {
	ss  serviceStore
	cfg *config.Config
}

func NewScraperHandler(ss serviceStore, cfg *config.Config) *ScraperHandler {
	return &ScraperHandler{ss: ss, cfg: cfg}
}

// APIKeyAuth middleware validates the X-Scraper-Key header against SCRAPER_API_KEY.
func (h *ScraperHandler) APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Scraper-Key")
		if key == "" || key != h.cfg.ScraperAPIKey {
			utils.WriteJSONResponse(w, http.StatusUnauthorized, false, "invalid or missing scraper API key", nil, nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetZipcodes syncs zipcode_distances from user_details, then returns
// (zipcode, distance) pairs that need scraping.
func (h *ScraperHandler) GetZipcodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.ss.SyncZipcodesFromUserDetails(ctx); err != nil {
		log.Printf("[scraper] sync zipcodes error: %v", err)
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to sync zipcodes", nil, err.Error())
		return
	}

	zds, err := h.ss.GetZipcodesForScraping(ctx)
	if err != nil {
		log.Printf("[scraper] get zipcodes error: %v", err)
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to get zipcodes", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "", zds, nil)
}

// submitTournamentsRequest is the expected JSON body for POST /scraper/tournaments.
type submitTournamentsRequest struct {
	Zipcode     string              `json:"zipcode"`
	Distance    int                 `json:"distance"`
	Tournaments []tournamentPayload `json:"tournaments"`
}

type tournamentPayload struct {
	Title       string `json:"title"`
	URLPath     string `json:"url_path"`
	City        string `json:"city"`
	State       string `json:"state"`
	Dates       string `json:"dates"`
	StartDate   string `json:"start_date"` // ISO date string "2026-03-11"
	Organizer   string `json:"organizer"`
	Description string `json:"description"`
}

// SubmitTournaments accepts scraped tournament data for a (zipcode, distance) and persists it.
func (h *ScraperHandler) SubmitTournaments(w http.ResponseWriter, r *http.Request) {
	var req submitTournamentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "invalid request body", nil, err.Error())
		return
	}

	req.Zipcode = strings.TrimSpace(req.Zipcode)
	if req.Zipcode == "" {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "zipcode is required", nil, nil)
		return
	}
	if req.Distance <= 0 {
		utils.WriteJSONResponse(w, http.StatusBadRequest, false, "distance is required", nil, nil)
		return
	}

	// Convert payloads to models
	tournaments := make([]models.Tournament, 0, len(req.Tournaments))
	for _, t := range req.Tournaments {
		mt := models.Tournament{
			Title:       t.Title,
			URLPath:     t.URLPath,
			City:        t.City,
			State:       t.State,
			Dates:       t.Dates,
			Organizer:   t.Organizer,
			Description: strings.TrimSpace(t.Description),
		}
		if t.StartDate != "" {
			if parsed, err := time.Parse("2006-01-02", t.StartDate); err == nil {
				mt.StartDate = &parsed
			}
		}
		tournaments = append(tournaments, mt)
	}

	if err := h.ss.UpsertTournaments(r.Context(), req.Zipcode, req.Distance, tournaments); err != nil {
		log.Printf("[scraper] upsert tournaments error (zip=%s, dist=%d): %v", req.Zipcode, req.Distance, err)
		utils.WriteJSONResponse(w, http.StatusInternalServerError, false, "failed to save tournaments", nil, err.Error())
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, true, "tournaments saved", map[string]interface{}{
		"zipcode":  req.Zipcode,
		"distance": req.Distance,
		"count":    len(tournaments),
	}, nil)
}
