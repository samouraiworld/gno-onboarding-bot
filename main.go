package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/handlers"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
	"onboardingbot/internal/valoper"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config.yaml")
	templatesPath := flag.String("templates", "templates.yaml", "path to templates.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	tpl, err := templates.Load(*templatesPath)
	if err != nil {
		log.Fatalf("load templates: %v", err)
	}

	sheetsClient, err := sheet.NewGoogleSheetsClient(context.Background(), cfg.GoogleCredentialsFile)
	if err != nil {
		log.Fatalf("create sheets client: %v", err)
	}
	if err := sheet.Ensure(context.Background(), sheetsClient, cfg.SheetID, cfg.SheetName); err != nil {
		log.Fatalf("ensure sheet tab/headers: %v", err)
	}
	if err := sheet.EnsureApprovedView(context.Background(), sheetsClient, cfg.SheetID, cfg.SheetName); err != nil {
		log.Fatalf("ensure approved-view tab: %v", err)
	}
	if err := sheet.EnsureStatusDropdown(context.Background(), sheetsClient, cfg.SheetID, cfg.SheetName); err != nil {
		log.Fatalf("ensure status dropdown: %v", err)
	}
	if err := sheet.EnsureStatusColors(context.Background(), sheetsClient, cfg.SheetID, cfg.SheetName); err != nil {
		log.Fatalf("ensure status colors: %v", err)
	}
	// Color the -approved tab by status too, so "GovDAO submitted" and "GovDAO
	// pending" rows are visually separated (replaces the old divider row).
	if err := sheet.EnsureStatusColors(context.Background(), sheetsClient, cfg.SheetID, sheet.ApprovedTabName(cfg.SheetName)); err != nil {
		log.Fatalf("ensure status colors (approved): %v", err)
	}
	if err := sheet.EnsureFrozenHeader(context.Background(), sheetsClient, cfg.SheetID, cfg.SheetName); err != nil {
		log.Fatalf("ensure frozen header (source): %v", err)
	}
	if err := sheet.EnsureFrozenHeader(context.Background(), sheetsClient, cfg.SheetID, sheet.ApprovedTabName(cfg.SheetName)); err != nil {
		log.Fatalf("ensure frozen header (approved): %v", err)
	}
	// Harvest assessment layer: N-Y columns + criterion checkboxes, the -evidence tab.
	if err := sheet.EnsureHarvestLayout(context.Background(), sheetsClient, cfg.SheetID, cfg.SheetName); err != nil {
		log.Fatalf("ensure harvest layout: %v", err)
	}

	s, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("create discord session: %v", err)
	}
	// GuildMessages + MessageContent (privileged) let /harvest read channel history.
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	ready := make(chan struct{})
	s.AddHandlerOnce(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})

	if err := s.Open(); err != nil {
		log.Fatalf("open discord session: %v", err)
	}
	defer s.Close()
	<-ready

	registrations := []func(*discordgo.Session, *config.Config, sheet.API, *templates.Templates) error{
		handlers.RegisterCandidate,
		handlers.RegisterRequestMissingInfo,
		handlers.RegisterDecline,
		handlers.RegisterEscalateToCall,
		handlers.RegisterApprove,
		handlers.RegisterHarvest,
	}
	for _, register := range registrations {
		if err := register(s, cfg, sheetsClient, tpl); err != nil {
			log.Fatalf("register command: %v", err)
		}
	}

	renderer := valoper.NewClient(cfg.GnoRPCEndpoint)
	if err := handlers.RegisterSubmit(s, cfg, sheetsClient, tpl, renderer); err != nil {
		log.Fatalf("register submit command: %v", err)
	}

	log.Println("bot is running, press Ctrl+C to exit")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}
