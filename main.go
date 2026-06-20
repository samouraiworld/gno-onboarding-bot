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

	s, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("create discord session: %v", err)
	}
	s.Identify.Intents = discordgo.IntentsGuilds

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
		handlers.RegisterAskToRetry,
		handlers.RegisterEscalateToCall,
		handlers.RegisterApprove,
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
