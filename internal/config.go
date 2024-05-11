package internal

import (
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

type Config struct {
	Token    string     `yaml:"token"`
	Chat     string     `yaml:"chat"`
	Profiles []*Profile `yaml:"profiles,omitempty"`
	Filename string     `yaml:"-"`
}

func ConfigFromFile(filename string) (*Config, error) {
	buff, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(buff, &cfg); err != nil {
		return nil, err
	}
	cfg.Filename = filename
	return &cfg, err
}

func (cfg *Config) Update() error {
	buff, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.Filename, buff, 0755)
}

type Profile struct {
	Tag        string    `yaml:"tag"`
	Username   string    `yaml:"username"`
	UserId     string    `yaml:"user_id"`
	Thread     string    `yaml:"thread,omitempty"`
	LastUpload time.Time `yaml:"last_upload,omitempty"`
}

func (profile *Profile) Clone() *Profile {
	return &Profile{
		Tag:        profile.Tag,
		Username:   profile.Username,
		Thread:     profile.Thread,
		LastUpload: profile.LastUpload,
	}
}
