package service

import (
	"context"
	"log"
	"time"

	"github.com/madhava-poojari/dashboard-api/internal/store"
	"github.com/robfig/cron/v3"
)

// StartRatingCrons creates and starts the cron scheduler for rating scraping.
// Weekly: Chess.com + Lichess (Monday 3 AM UTC)
// Monthly: FIDE (1st of month 4 AM UTC)
func StartRatingCrons(s *store.Store) {
	c := cron.New(cron.WithLocation(time.UTC))

	// Weekly: every Monday at 3:00 AM UTC
	_, err := c.AddFunc("0 3 * * 1", func() {
		log.Println("[RatingCron] Starting weekly Chess.com + Lichess scrape...")
		RunWeeklyRatingScrape(s)
		log.Println("[RatingCron] Weekly scrape completed")
	})
	if err != nil {
		log.Printf("[RatingCron] Failed to schedule weekly scrape: %v", err)
	}

	// Monthly: 1st of every month at 4:00 AM UTC
	_, err = c.AddFunc("0 4 1 * *", func() {
		log.Println("[RatingCron] Starting monthly FIDE scrape...")
		RunMonthlyFIDEScrape(s)
		log.Println("[RatingCron] Monthly FIDE scrape completed")
	})
	if err != nil {
		log.Printf("[RatingCron] Failed to schedule monthly scrape: %v", err)
	}

	c.Start()
	log.Println("[RatingCron] Cron scheduler started (weekly: Mon 3AM UTC, monthly: 1st 4AM UTC)")
}

// RunWeeklyRatingScrape processes all active students for Chess.com and Lichess.
func RunWeeklyRatingScrape(s *store.Store) {
	ctx := context.Background()

	// ---- Chess.com ----
	chesscomStudents, err := s.GetStudentsWithPlatformUsername(ctx, "chesscom")
	if err != nil {
		log.Printf("[RatingCron] Error fetching chesscom students: %v", err)
	} else {
		for _, student := range chesscomStudents {
			processChesscomStudent(ctx, s, student)
		}
	}

	// ---- Lichess ----
	lichessStudents, err := s.GetStudentsWithPlatformUsername(ctx, "lichess")
	if err != nil {
		log.Printf("[RatingCron] Error fetching lichess students: %v", err)
	} else {
		for _, student := range lichessStudents {
			processLichessStudent(ctx, s, student)
		}
	}
}

// RunMonthlyFIDEScrape processes all active students for FIDE.
func RunMonthlyFIDEScrape(s *store.Store) {
	ctx := context.Background()

	fideStudents, err := s.GetStudentsWithPlatformUsername(ctx, "fide")
	if err != nil {
		log.Printf("[RatingCron] Error fetching FIDE students: %v", err)
		return
	}

	for _, student := range fideStudents {
		processFIDEStudent(ctx, s, student)
	}
}

func processChesscomStudent(ctx context.Context, s *store.Store, student store.StudentPlatformInfo) {
	has, err := s.HasRatingHistory(ctx, student.UserID, "chesscom")
	if err != nil {
		log.Printf("[RatingCron] Error checking chesscom history for %s: %v", student.UserID, err)
		return
	}

	if !has {
		// Backfill: fetch all historical rapid games
		log.Printf("[RatingCron] Backfilling chesscom for user %s (username: %s)", student.UserID, student.Username)
		records, err := ScrapeChesscomHistory(student.Username)
		if err != nil {
			log.Printf("[RatingCron] Error backfilling chesscom for %s: %v", student.UserID, err)
			return
		}
		if len(records) == 0 {
			log.Printf("[RatingCron] No chesscom rapid games found for %s", student.UserID)
			return
		}
		// Set user IDs
		for i := range records {
			records[i].UserID = student.UserID
		}
		if err := s.BulkCreateRatingRecords(ctx, records); err != nil {
			log.Printf("[RatingCron] Error bulk-inserting chesscom records for %s: %v", student.UserID, err)
			return
		}
		log.Printf("[RatingCron] Backfilled %d chesscom records for user %s", len(records), student.UserID)
	} else {
		// Incremental: fetch current rating
		record, err := ScrapeChesscomCurrent(student.Username)
		if err != nil {
			log.Printf("[RatingCron] Error fetching current chesscom for %s: %v", student.UserID, err)
			return
		}
		record.UserID = student.UserID
		if err := s.CreateRatingRecord(ctx, record); err != nil {
			log.Printf("[RatingCron] Error inserting chesscom record for %s: %v", student.UserID, err)
			return
		}
		log.Printf("[RatingCron] Added chesscom rating %d for user %s", record.Rating, student.UserID)
	}
}

func processLichessStudent(ctx context.Context, s *store.Store, student store.StudentPlatformInfo) {
	has, err := s.HasRatingHistory(ctx, student.UserID, "lichess")
	if err != nil {
		log.Printf("[RatingCron] Error checking lichess history for %s: %v", student.UserID, err)
		return
	}

	if !has {
		// Backfill: fetch full rating history
		log.Printf("[RatingCron] Backfilling lichess for user %s (username: %s)", student.UserID, student.Username)
		records, err := ScrapeLichessHistory(student.Username)
		if err != nil {
			log.Printf("[RatingCron] Error backfilling lichess for %s: %v", student.UserID, err)
			return
		}
		if len(records) == 0 {
			log.Printf("[RatingCron] No lichess rapid history found for %s", student.UserID)
			return
		}
		for i := range records {
			records[i].UserID = student.UserID
		}
		if err := s.BulkCreateRatingRecords(ctx, records); err != nil {
			log.Printf("[RatingCron] Error bulk-inserting lichess records for %s: %v", student.UserID, err)
			return
		}
		log.Printf("[RatingCron] Backfilled %d lichess records for user %s", len(records), student.UserID)
	} else {
		// Incremental: fetch current rating
		record, err := ScrapeLichessCurrent(student.Username)
		if err != nil {
			log.Printf("[RatingCron] Error fetching current lichess for %s: %v", student.UserID, err)
			return
		}
		record.UserID = student.UserID
		if err := s.CreateRatingRecord(ctx, record); err != nil {
			log.Printf("[RatingCron] Error inserting lichess record for %s: %v", student.UserID, err)
			return
		}
		log.Printf("[RatingCron] Added lichess rating %d for user %s", record.Rating, student.UserID)
	}
}

func processFIDEStudent(ctx context.Context, s *store.Store, student store.StudentPlatformInfo) {
	has, err := s.HasRatingHistory(ctx, student.UserID, "fide")
	if err != nil {
		log.Printf("[RatingCron] Error checking FIDE history for %s: %v", student.UserID, err)
		return
	}

	if !has {
		// Backfill: fetch all historical monthly ratings
		log.Printf("[RatingCron] Backfilling FIDE for user %s (FIDE ID: %s)", student.UserID, student.Username)
		records, err := ScrapeFIDEHistory(student.Username)
		if err != nil {
			log.Printf("[RatingCron] Error backfilling FIDE for %s: %v", student.UserID, err)
			return
		}
		if len(records) == 0 {
			log.Printf("[RatingCron] No FIDE rating data found for %s", student.UserID)
			return
		}
		for i := range records {
			records[i].UserID = student.UserID
		}
		if err := s.BulkCreateRatingRecords(ctx, records); err != nil {
			log.Printf("[RatingCron] Error bulk-inserting FIDE records for %s: %v", student.UserID, err)
			return
		}
		log.Printf("[RatingCron] Backfilled %d FIDE records for user %s", len(records), student.UserID)
	} else {
		// Incremental: fetch latest month's rating
		record, err := ScrapeFIDECurrent(student.Username)
		if err != nil {
			log.Printf("[RatingCron] Error fetching current FIDE for %s: %v", student.UserID, err)
			return
		}

		// Check if we already have this month's record
		latest, latestErr := s.GetLatestRating(ctx, student.UserID, "fide")
		if latestErr == nil && latest != nil {
			latestYear, latestMonth, _ := latest.RecordedAt.Date()
			recordYear, recordMonth, _ := record.RecordedAt.Date()
			if latestYear == recordYear && latestMonth == recordMonth {
				log.Printf("[RatingCron] FIDE rating for %s already up to date (%d-%s)", student.UserID, recordYear, recordMonth)
				return
			}
		}

		record.UserID = student.UserID
		if err := s.CreateRatingRecord(ctx, record); err != nil {
			log.Printf("[RatingCron] Error inserting FIDE record for %s: %v", student.UserID, err)
			return
		}
		log.Printf("[RatingCron] Added FIDE rating %d for user %s", record.Rating, student.UserID)
	}
}
