package gateway

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIdleReservationOnceNotifiesOnlyOwnerAndCompletes(t *testing.T) {
	db := newIdleReservationTestDB(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	created, err := createIdleReservation(db, 7, IdleReservationInput{
		Name:       "large model run",
		Filters:    IdleReservationFilters{MinFreeVRAMGB: 40, ProcessPolicy: "emptyOnly", IdleDurationMinutes: 10},
		NotifyMode: "once", ExpiresInHours: 24,
	}, now)
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	if created.Status != IdleReservationActive || created.Filters.ProcessPolicy != "emptyOnly" {
		t.Fatalf("unexpected created reservation: %+v", created)
	}
	if _, err := evaluateIdleReservation(db, 8, created.ID, []string{"3:0"}, now); !errors.Is(err, ErrIdleReservationNotFound) {
		t.Fatalf("other user should not access reservation: %v", err)
	}
	first, err := evaluateIdleReservation(db, 7, created.ID, []string{"3:0"}, now)
	if err != nil {
		t.Fatalf("evaluate reservation: %v", err)
	}
	if first.Reservation.Status != IdleReservationCompleted || len(first.NewMatchKeys) != 1 || first.NewMatchKeys[0] != "3:0" {
		t.Fatalf("first match should notify and complete once: %+v", first)
	}
	second, err := evaluateIdleReservation(db, 7, created.ID, []string{"3:0"}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("evaluate completed reservation: %v", err)
	}
	if second.Reservation.Status != IdleReservationCompleted || len(second.NewMatchKeys) != 0 {
		t.Fatalf("completed reminder must not notify again: %+v", second)
	}
}

func TestIdleReservationContinuousNotifiesOnNewMatchEdges(t *testing.T) {
	db := newIdleReservationTestDB(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	created, err := createIdleReservation(db, 11, IdleReservationInput{
		Name:       "any free gpu",
		Filters:    IdleReservationFilters{ProcessPolicy: "any"},
		NotifyMode: "continuous", ExpiresInHours: 0,
	}, now)
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	first, err := evaluateIdleReservation(db, 11, created.ID, []string{"1:0"}, now)
	if err != nil || len(first.NewMatchKeys) != 1 {
		t.Fatalf("first match should notify: result=%+v err=%v", first, err)
	}
	stillMatched, err := evaluateIdleReservation(db, 11, created.ID, []string{"1:0"}, now.Add(time.Minute))
	if err != nil || len(stillMatched.NewMatchKeys) != 0 {
		t.Fatalf("stable match should not repeat: result=%+v err=%v", stillMatched, err)
	}
	_, err = evaluateIdleReservation(db, 11, created.ID, []string{}, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("clear match: %v", err)
	}
	reappeared, err := evaluateIdleReservation(db, 11, created.ID, []string{"1:0"}, now.Add(3*time.Minute))
	if err != nil || len(reappeared.NewMatchKeys) != 1 {
		t.Fatalf("new match edge should notify again: result=%+v err=%v", reappeared, err)
	}
}

func TestIdleReservationExpiresAndRejectsInvalidFilters(t *testing.T) {
	db := newIdleReservationTestDB(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	_, err := createIdleReservation(db, 3, IdleReservationInput{
		Name: "short wait", Filters: IdleReservationFilters{ProcessPolicy: "emptyOnly"}, NotifyMode: "once", ExpiresInHours: 1,
	}, now)
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}
	items, err := listIdleReservations(db, 3, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("list expired reservation: %v", err)
	}
	if len(items) != 1 || items[0].Status != IdleReservationExpired || len(items[0].CurrentMatchKeys) != 0 {
		t.Fatalf("reservation should expire and clear matches: %+v", items)
	}
	_, err = createIdleReservation(db, 3, IdleReservationInput{
		Name: "bad", Filters: IdleReservationFilters{ProcessPolicy: "unknown"}, NotifyMode: "once", ExpiresInHours: 1,
	}, now)
	if !errors.Is(err, ErrInvalidIdleReservation) {
		t.Fatalf("expected invalid filter error, got %v", err)
	}
}

func newIdleReservationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&GPUReservation{}); err != nil {
		t.Fatalf("migrate reservations: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
