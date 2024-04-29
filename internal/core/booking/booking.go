package booking

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"spot-assistant/internal/common/collections"
	stringsHelper "spot-assistant/internal/common/strings"
	"spot-assistant/internal/core/dto/discord"
	"spot-assistant/internal/core/dto/reservation"
	"spot-assistant/internal/core/dto/spot"
)

var HourRegex = regexp.MustCompile(`(\d{2}:\d{2})`)

// Returns spots filtered by filter, if non-zero length.
func (a *Adapter) FindAvailableSpots(filter string) ([]string, error) {
	spots, err := a.spotRepo.SelectAllSpots(context.Background())
	if err != nil {
		return []string{}, fmt.Errorf("could not fetch spots matching your query: %w", err)
	}

	if len(filter) > 0 {
		spots = collections.PoorMansFilter(spots, func(spot *spot.Spot) bool {
			return strings.Contains(strings.ToLower(spot.Name), strings.ToLower(filter))
		})
	}

	spots = collections.Truncate(spots, 15)

	return collections.PoorMansMap(spots, func(s *spot.Spot) string {
		return s.Name
	}), nil
}

// Returns suggested hours based on requested time. If filter is non-zero length,
// it will return filtered results.
func (a *Adapter) GetSuggestedHours(baseTime time.Time, filter string) []string {
	suggestedHours := make([]time.Time, 0)
	validatedFilter := HourRegex.FindString(filter)

	roundedMinutes := baseTime.Minute()
	roundedHour := baseTime.Hour()
	if roundedMinutes >= 30 {
		roundedMinutes = 0
		roundedHour += 1
	} else {
		roundedMinutes = 30
	}

	baseTimeRounded := time.Date(baseTime.Year(), baseTime.Month(), baseTime.Day(), roundedHour, roundedMinutes, 0, 0, baseTime.Location())

	suggestedHours = append(suggestedHours, baseTimeRounded)
	for x := 1; x <= 7; x++ {
		suggestedHours = append(suggestedHours, suggestedHours[x-1].Add(30*time.Minute))
	}

	suggestedOptions := collections.PoorMansMap(suggestedHours, func(hour time.Time) string {
		return hour.Format(stringsHelper.DcTimeFormat)
	})

	if len(validatedFilter) > 0 {
		suggestedOptions = collections.PoorMansFilter(suggestedOptions, func(t string) bool {
			return strings.Contains(strings.ToLower(t), strings.ToLower(validatedFilter))
		})

		// Add user input, if it's valid
		if !collections.PoorMansContains(suggestedOptions, validatedFilter) {
			suggestedOptions = append(suggestedOptions, validatedFilter)
		}
	}

	return suggestedOptions
}

func (a *Adapter) Book(member *discord.Member, guild *discord.Guild, spotName string, startAt time.Time, endAt time.Time, overbook bool, hasPermissions bool) ([]*reservation.ClippedOrRemovedReservation, error) {
	a.log.With(
		"spot", spotName,
		"member.id", member.ID,
		"member.name", member.Nick,
		"member.username", member.Username,
		"hasPermissions", hasPermissions,
		"overbook", overbook,
		"startAt", startAt,
		"endAt", endAt,
	).Info("booking request")

	spots, err := a.spotRepo.SelectAllSpots(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not fetch spots: %w", err)
	}

	spot, _ := collections.PoorMansFind(spots, func(s *spot.Spot) bool {
		return s.Name == spotName
	})
	if spot == nil {
		return nil, fmt.Errorf("could not find spot called %s", spotName)
	}

	if err = validateHuntLength(endAt.Sub(startAt)); err != nil {
		return nil, err
	}

	upcomingAuthorReservations, err := a.reservationRepo.SelectUpcomingMemberReservationsWithSpots(context.Background(), guild, member)
	if err != nil {
		return nil, fmt.Errorf("could not select upcoming member reservations: %w", err)
	}

	if err = validateHuntLengthForMultiFloorRespawns(spotName, upcomingAuthorReservations, startAt, endAt); err != nil {
		return nil, err
	}

	conflictingReservations, err := a.reservationRepo.SelectOverlappingReservations(context.Background(), spotName, startAt, endAt, guild.ID)
	if err != nil {
		return nil, fmt.Errorf("could not select overlapping reservations: %w", err)
	}

	if len(conflictingReservations) > 0 {
		if overbook {
			err = validateNoSelfOverbook(member, conflictingReservations)
			if err != nil {
				return nil, err
			}
		}

		if !canOverbook(overbook, hasPermissions, conflictingReservations) {
			return collections.PoorMansMap(conflictingReservations, func(r *reservation.Reservation) *reservation.ClippedOrRemovedReservation {
				return &reservation.ClippedOrRemovedReservation{
					Original: r,
					New:      []*reservation.Reservation{r},
				}
			}), InsufficientPermissionsError

		}
	}

	res, err := a.reservationRepo.CreateAndDeleteConflicting(context.Background(), member, guild, conflictingReservations, spot.ID, startAt, endAt)
	if err != nil {
		return nil, fmt.Errorf("could not create the reservation: %w", err)
	}

	return res, nil
}

func (a *Adapter) UnbookAutocomplete(g *discord.Guild, m *discord.Member, filter string) ([]*reservation.ReservationWithSpot, error) {
	// Get reservations with end_date >= time.Now()
	// a.reservationRepo.SelectUpcomingReservationsWithSpot(context.Background(), g.ID)
	reservations, err := a.reservationRepo.SelectUpcomingMemberReservationsWithSpots(context.Background(), g, m)
	if err != nil {
		return []*reservation.ReservationWithSpot{}, err
	}

	// If any input value is passed, try to match it with startAt, endAt and spot name
	if len(filter) > 0 {
		reservations = collections.PoorMansFilter(reservations, func(r *reservation.ReservationWithSpot) bool {
			searchableString := strings.Join([]string{
				r.StartAt.Format(stringsHelper.DcLongTimeFormat),
				r.StartAt.Format(stringsHelper.DcLongTimeFormat),
				r.Spot.Name}, "")
			containsFilterWord := strings.Contains(strings.ToLower(searchableString), strings.ToLower(filter))
			return containsFilterWord
		})
	}

	return reservations, nil
}

func (a *Adapter) Unbook(g *discord.Guild, m *discord.Member, reservationId int64) (*reservation.ReservationWithSpot, error) {

	// Get non-expired reservation for guild + member + reservation
	// Remove it
	// Return removed reservation and an error
	res, err := a.reservationRepo.FindReservationWithSpot(context.Background(), reservationId, g.ID, m.ID)
	if err != nil {
		return nil, err
	}

	err = a.reservationRepo.DeletePresentMemberReservation(context.Background(), g, m, res.Reservation.ID)
	if err != nil {
		return res, err
	}

	return res, nil
}
