package internal

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"log/slog"
	"time"
)

type Manager struct {
	dscrd  *discordgo.Session
	Config *Config
}

func NewManagerFromFile(filename string, level slog.Level) (*Manager, error) {
	config, err := ConfigFromFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	bot, err := discordgo.New("Bot " + config.Token)
	if err != nil {
		slog.Error("error creating Discord session,", "err", err)
		return nil, err
	}

	return &Manager{
		dscrd:  bot,
		Config: config,
	}, nil
}

func (manager *Manager) Start(pollRate time.Duration) {
	slog.Info("Starting poll", "pollRate", pollRate)
	ticker := time.NewTicker(pollRate)
	for ; true; _ = <-ticker.C {
		for _, profile := range manager.Config.Profiles {
			if err := manager.Profile(profile); err != nil {
				slog.Error("failed to update profile", "profile", profile.Tag, "err", err)
			}
		}
	}
}
