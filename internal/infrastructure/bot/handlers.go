package bot

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"spot-assistant/internal/common/collections"
	stringsHelper "spot-assistant/internal/common/strings"
	"spot-assistant/internal/core/dto/book"
	"spot-assistant/internal/core/dto/summary"
)

/*
*
System events that are initialized by Discord.
*/

func (b *Bot) GuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	b.log.Debug("GuildCreate")

	defer b.eventHandler.OnGuildCreate(MapGuild(g.Guild))
}

func (b *Bot) Ready(s *discordgo.Session, r *discordgo.Ready) {
	b.log.Debug("Ready")

	b.StartTicking()

	defer b.eventHandler.OnReady()
}

// InteractionCreate this is the entry point when a slash command is invoked.
func (b *Bot) InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.log.Debug("InteractionCreate")
	tStart := time.Now()

	b.handleCommand(i)

	b.log.With("duration", time.Since(tStart)).Debug("interaction handled")
}

// Service events
// Events that are sent by tickers or our custom integrations,
// such as commands.

func (b *Bot) Tick() {
	b.log.Debug("Tick")

	defer b.eventHandler.OnTick()
}

func (b *Bot) Book(i *discordgo.InteractionCreate) error {
	b.log.Info("Book")
	interaction := i.Interaction
	tNow := time.Now()
	gID, err := stringsHelper.StrToInt64(i.GuildID)
	if err != nil {
		return err
	}
	dcSession := b.mgr.SessionForGuild(gID)

	// Flag parsing
	overbook := false
	switch len(i.ApplicationCommandData().Options) {
	case 4:
		if i.ApplicationCommandData().Options[3].StringValue() == "true" {
			overbook = true
		}
	case 3:
		break
	default:
		return errors.New("book command requires 3 arguments")
	}

	startAt, err := time.Parse(stringsHelper.DcTimeFormat, i.ApplicationCommandData().Options[1].StringValue())
	if err != nil {
		return err
	}
	startAt = time.Date(
		tNow.Year(), tNow.Month(), tNow.Day(), startAt.Hour(), startAt.Minute(), 0, 0, tNow.Location())

	endAt, err := time.Parse(stringsHelper.DcTimeFormat, i.ApplicationCommandData().Options[2].StringValue())
	if err != nil {
		return err
	}
	endAt = time.Date(
		tNow.Year(), tNow.Month(), tNow.Day(), endAt.Hour(), endAt.Minute(), 0, 0, tNow.Location())

	if startAt.Before(tNow) {
		startAt = startAt.Add(24 * time.Hour)
		endAt = endAt.Add(24 * time.Hour)
	}

	if startAt.After(endAt) {
		endAt = endAt.Add(24 * time.Hour)
	}

	guild, err := b.GetGuild(gID)
	if err != nil {
		return err
	}

	member := MapMember(i.Member)
	request := book.BookRequest{
		Member:   member,
		Guild:    guild,
		Spot:     i.ApplicationCommandData().Options[0].StringValue(),
		StartAt:  startAt,
		EndAt:    endAt,
		Overbook: overbook,
	}

	tStart := time.Now()
	response, err := b.eventHandler.OnBook(request)
	bookLog := b.log.With("duration", time.Since(tStart), "error", err)
	var message string
	if err != nil {
		bookLog.Warn("book request handled")
		message = b.formatter.FormatBookError(response, err)
	} else {
		bookLog.Info("book request handled")
		message = b.formatter.FormatBookResponse(response)
	}

	bookLog.Info("followup message sending")

	_, err = dcSession.FollowupMessageCreate(interaction, false, &discordgo.WebhookParams{
		Content: message,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
		},
	})
	return err
}

func (b *Bot) BookAutocomplete(i *discordgo.InteractionCreate) error {
	selectedOption, index := collections.PoorMansFind(i.ApplicationCommandData().Options,
		func(o *discordgo.ApplicationCommandInteractionDataOption) bool {
			return o.Focused
		})
	if index == -1 {
		return errors.New("none of the options were selected for autocompletion")
	}

	response, err := b.eventHandler.OnBookAutocomplete(book.BookAutocompleteRequest{
		Field: book.BookAutocompleteFocus(index),
		Value: selectedOption.StringValue(),
	})
	if err != nil {
		return err
	}

	responseData := &discordgo.InteractionResponseData{
		Choices: MapStringArrToChoice(response),
	}
	return b.interactionRespond(i, responseData, discordgo.InteractionApplicationCommandAutocompleteResult)
}

func (b *Bot) Unbook(i *discordgo.InteractionCreate) error {
	if len(i.ApplicationCommandData().Options) < 1 {
		return errors.New("you must select a reservation to unbook")
	}

	reservationId, err := stringsHelper.StrToInt64(i.ApplicationCommandData().Options[0].StringValue())
	if err != nil {
		return fmt.Errorf("could not parse reservation id: %v", reservationId)
	}

	gID, err := stringsHelper.StrToInt64(i.GuildID)
	if err != nil {
		return fmt.Errorf("could not parse guild id: %v", i.GuildID)
	}

	guild, err := b.GetGuild(gID)
	if err != nil {
		return err
	}

	res, err := b.eventHandler.OnUnbook(book.UnbookRequest{
		Member:        MapMember(i.Member),
		Guild:         guild,
		ReservationID: reservationId,
	})
	if err != nil {
		return err
	}

	_, err = b.mgr.SessionForGuild(gID).FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: b.formatter.FormatUnbookResponse(res),
	})
	return err
}

func (b *Bot) UnbookAutocomplete(i *discordgo.InteractionCreate) error {
	selectedOption, index := collections.PoorMansFind(i.ApplicationCommandData().Options,
		func(o *discordgo.ApplicationCommandInteractionDataOption) bool {
			return o.Focused
		})
	if index == -1 {
		return fmt.Errorf("none of the options were selected for autocompletion")
	}

	gID, err := stringsHelper.StrToInt64(i.GuildID)
	if err != nil {
		return err
	}

	guild, err := b.GetGuild(gID)
	if err != nil {
		return err
	}

	request := book.UnbookAutocompleteRequest{
		Guild:  guild,
		Member: MapMember(i.Member),
		Value:  selectedOption.StringValue(),
	}

	response, err := b.eventHandler.OnUnbookAutocomplete(request)
	if err != nil {
		return err
	}

	responseData := &discordgo.InteractionResponseData{
		Choices: MapReservationWithSpotArrToChoice(response.Choices),
	}

	return b.interactionRespond(i, responseData, discordgo.InteractionApplicationCommandAutocompleteResult)
}

func (b *Bot) PrivateSummary(i *discordgo.InteractionCreate) error {
	b.log.Info("PrivateSummary")

	gID, err := stringsHelper.StrToInt64(i.GuildID)
	if err != nil {
		return err
	}

	uID, err := stringsHelper.StrToInt64(i.Member.User.ID)
	if err != nil {
		return err
	}

	isSummaryWithoutRespawns := len(i.ApplicationCommandData().Options) < 1
	if isSummaryWithoutRespawns {
		err = b.eventHandler.OnPrivateSummary(summary.PrivateSummaryRequest{
			GuildID: gID,
			UserID:  uID,
		})
		if err != nil {
			return err
		}
	} else {

		spotNames := strings.Split(i.ApplicationCommandData().Options[0].StringValue(), ", ")

		err = b.eventHandler.OnPrivateSummary(summary.PrivateSummaryRequest{
			GuildID:   gID,
			UserID:    uID,
			SpotNames: spotNames,
		})
		if err != nil {
			return err
		}
	}

	_, err = b.mgr.SessionForGuild(gID).FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{Content: "Check your DM!"})
	return err

}

func (b *Bot) PrivateSummaryWithSpotNamesAutocomplete(i *discordgo.InteractionCreate) error {

	b.log.Info("PrivateSummaryWithSpotNamesAutocomplete")

	userInput, index := collections.PoorMansFind(i.ApplicationCommandData().Options,
		func(o *discordgo.ApplicationCommandInteractionDataOption) bool {
			return o.Focused
		})
	if index == -1 {
		return errors.New("none of the options were selected for autocompletion")
	}

	lastEntry := strings.Split(userInput.StringValue(), ",")[len(strings.Split(userInput.StringValue(), ","))-1]

	responseFromAutocomplete, err := b.eventHandler.OnBookAutocomplete(book.BookAutocompleteRequest{
		Field: book.BookAutocompleteFocus(index),
		Value: strings.TrimLeft(lastEntry, " "),
	})
	if err != nil {
		return err
	}

	lastCommaIndex := strings.LastIndex(userInput.StringValue(), ",")
	if lastCommaIndex > 0 {
		for i := 0; i < len(responseFromAutocomplete); i++ {
			responseFromAutocomplete[i] = userInput.StringValue()[:lastCommaIndex] + ", " + responseFromAutocomplete[i]
		}
	}

	responseData := &discordgo.InteractionResponseData{
		Choices: MapStringArrToChoice(responseFromAutocomplete),
	}

	return b.interactionRespond(i, responseData, discordgo.InteractionApplicationCommandAutocompleteResult)

}
