package api

import (
	"spot-assistant/internal/common/test/mocks"
	"spot-assistant/internal/core/dto/discord"
	"spot-assistant/internal/core/dto/reservation"
	"spot-assistant/internal/core/dto/summary"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdateGuildSummary(t *testing.T) {
	// given
	assert := assert.New(t)
	guild := &discord.Guild{
		ID:   "test-guild-id",
		Name: "test-guild-name",
	}
	summary := &summary.Summary{
		Title: "summary",
	}
	summaryCh := &discord.Channel{ID: "test-channel-id", Name: "letter-summary"}
	reservations := []*reservation.ReservationWithSpot{
		{
			Spot: reservation.Spot{
				ID:   1,
				Name: "test-spot-name",
			},
			Reservation: reservation.Reservation{
				ID:      1,
				SpotID:  1,
				StartAt: time.Now(),
				EndAt:   time.Now().Add(2 * time.Hour),
			},
		},
	}
	mockBot := new(mocks.MockBot)
	mockBot.On("SendLetterMessage", guild, summaryCh, summary).Return(nil)
	mockBot.On("FindChannelByName", guild, "letter-summary").Return(summaryCh, nil)
	mockReservationRepo := new(mocks.MockReservationRepo)
	mockReservationRepo.On("SelectUpcomingReservationsWithSpot", mocks.ContextMock, guild.ID).Return(reservations, nil)
	mockSummarySrv := new(mocks.MockSummaryService)
	mockSummarySrv.On("PrepareSummary", reservations).Return(summary, nil)
	mockBookingSrv := new(mocks.MockBookingService)
	adapter := NewApplication(mockReservationRepo, mockSummarySrv, mockBookingSrv)

	// when
	err := adapter.UpdateGuildSummary(mockBot, guild)

	// assert
	assert.Nil(err)
	mockReservationRepo.AssertExpectations(t)
	mockSummarySrv.AssertExpectations(t)
	mockBot.AssertExpectations(t)
	mockBookingSrv.AssertExpectations(t)
}

func TestOnPrivateSummaryWhenRequestContainsSpotNames(t *testing.T) {
	// given
	assert := assert.New(t)
	privateSummaryRequest := summary.PrivateSummaryRequest{
		UserID:    23,
		GuildID:   34,
		SpotNames: []string{"test-spot-name"},
	}

	var guild *discord.Guild = nil

	dcDmChannel := &discord.Channel{ID: "test-channel-id", Name: "letter-summary"}

	dcMember := &discord.Member{
		ID: strconv.FormatInt(privateSummaryRequest.UserID, 10),
	}

	summary := &summary.Summary{
		Title: "summary",
	}

	reservations := []*reservation.ReservationWithSpot{
		{
			Spot: reservation.Spot{
				ID:   1,
				Name: "test-spot-name",
			},
			Reservation: reservation.Reservation{
				ID:      1,
				SpotID:  1,
				StartAt: time.Now(),
				EndAt:   time.Now().Add(2 * time.Hour),
			},
		},
	}
	mockBot := new(mocks.MockBot)
	mockBot.On("OpenDM", dcMember).Return(dcDmChannel, nil)
	mockBot.On("SendLetterMessage", guild, dcDmChannel, summary).Return(nil)
	mockReservationRepo := new(mocks.MockReservationRepo)
	mockReservationRepo.On("SelectAllReservationsWithSpotsBySpotNames", mocks.ContextMock, strconv.FormatInt(privateSummaryRequest.GuildID, 10), privateSummaryRequest.SpotNames).Return(reservations, nil)
	mockSummarySrv := new(mocks.MockSummaryService)
	mockSummarySrv.On("PrepareSummary", reservations).Return(summary, nil)
	mockBookingSrv := new(mocks.MockBookingService)
	adapter := NewApplication(mockReservationRepo, mockSummarySrv, mockBookingSrv)

	// when
	err := adapter.OnPrivateSummary(mockBot, privateSummaryRequest)

	// assert
	assert.Nil(err)
	mockReservationRepo.AssertExpectations(t)
	mockSummarySrv.AssertExpectations(t)
	mockBot.AssertExpectations(t)
	mockBookingSrv.AssertExpectations(t)
}

func TestOnPrivateSummaryWhenRequestDoesntContainsSpotNames(t *testing.T) {
	// given
	assert := assert.New(t)
	privateSummaryRequest := summary.PrivateSummaryRequest{
		UserID:  23,
		GuildID: 34,
	}

	var guild *discord.Guild = nil

	dcDmChannel := &discord.Channel{ID: "test-channel-id", Name: "letter-summary"}

	dcMember := &discord.Member{
		ID: strconv.FormatInt(privateSummaryRequest.UserID, 10),
	}

	summary := &summary.Summary{
		Title: "summary",
	}

	reservations := []*reservation.ReservationWithSpot{
		{
			Spot: reservation.Spot{
				ID:   1,
				Name: "test-spot-name",
			},
			Reservation: reservation.Reservation{
				ID:      1,
				SpotID:  1,
				StartAt: time.Now(),
				EndAt:   time.Now().Add(2 * time.Hour),
			},
		},
	}
	mockBot := new(mocks.MockBot)
	mockBot.On("OpenDM", dcMember).Return(dcDmChannel, nil)
	mockBot.On("SendLetterMessage", guild, dcDmChannel, summary).Return(nil)
	mockReservationRepo := new(mocks.MockReservationRepo)
	mockReservationRepo.On("SelectUpcomingReservationsWithSpot", mocks.ContextMock, strconv.FormatInt(privateSummaryRequest.GuildID, 10)).Return(reservations, nil)
	mockSummarySrv := new(mocks.MockSummaryService)
	mockSummarySrv.On("PrepareSummary", reservations).Return(summary, nil)
	mockBookingSrv := new(mocks.MockBookingService)
	adapter := NewApplication(mockReservationRepo, mockSummarySrv, mockBookingSrv)

	// when
	err := adapter.OnPrivateSummary(mockBot, privateSummaryRequest)

	// assert
	assert.Nil(err)
	mockReservationRepo.AssertExpectations(t)
	mockSummarySrv.AssertExpectations(t)
	mockBot.AssertExpectations(t)
	mockBookingSrv.AssertExpectations(t)
}
