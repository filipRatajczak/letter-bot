package api

import (
	"context"
	"fmt"
	"spot-assistant/internal/core/dto/reservation"
	"strconv"

	"spot-assistant/internal/common/errors"
	"spot-assistant/internal/core/dto/discord"
	"spot-assistant/internal/core/dto/summary"
)

// UpdateGuild makes a full-fledged guild update including summary re-generation.
func (a *Application) UpdateGuildSummary(guild *discord.Guild) error {
	log := a.log.With("guild.ID", guild.ID, "guild.Name", guild.Name, "event", "UpdateGuildSummary")

	// For each guild
	reservations, err := a.db.SelectUpcomingReservationsWithSpot(
		context.Background(), guild.ID,
	)
	if err != nil {
		return err
	}

	if len(reservations) == 0 {
		log.Warn("no reservations for guild, skipping")

		return nil
	}

	summ, err := a.summarySrv.PrepareSummary(reservations)
	if err != nil {
		return err
	}

	return a.commSrv.SendGuildSummary(guild, summ)
}

func (a *Application) UpdateGuildSummaryAndLogError(guild *discord.Guild) {
	errors.LogError(a.log, a.UpdateGuildSummary(guild))
}

func (a *Application) OnPrivateSummary(request summary.PrivateSummaryRequest) error {
	log := a.log.With("user.ID", request.UserID, "guild.ID", request.GuildID)
	log.Debug("OnPrivateSummary")

	res, err := a.db.SelectUpcomingReservationsWithSpot(context.Background(), strconv.FormatInt(request.GuildID, 10))
	if err != nil {
		return err
	}

	if len(res) == 0 {
		log.Warn("no reservations to display in DM; skipping")

		return nil
	}

	summ, err := a.summarySrv.PrepareSummary(res)
	if err != nil {
		return err
	}

	return a.commSrv.SendPrivateSummary(request, summ)
}

func (a *Application) fetchUpcomingReservationsWithSpot(request summary.PrivateSummaryRequest) ([]*reservation.ReservationWithSpot, error) {
	var res []*reservation.ReservationWithSpot
	var err error

	if request.SpotNames != nil {
		res, err = a.db.SelectAllReservationsWithSpotsBySpotNames(context.Background(), strconv.FormatInt(request.GuildID, 10), request.SpotNames)
		if err != nil {
			return nil, fmt.Errorf("could not fetch upcoming reservations: %v", err)
		}
	} else {
		res, err = a.db.SelectUpcomingReservationsWithSpot(context.Background(), strconv.FormatInt(request.GuildID, 10))
		if err != nil {
			return nil, fmt.Errorf("could not fetch upcoming reservations: %v", err)
		}
	}

	return res, nil
}
