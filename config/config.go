package config

import (
	"io"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Nick     string   `toml:"nick"`
	Username string   `toml:"username"`
	Realname string   `toml:"realname"`
	Servers  []Server `toml:"server"`
}

type Server struct {
	Name     string   `toml:"name"`
	Host     string   `toml:"host"`
	Port     int      `toml:"port"`
	UseTls   bool     `toml:"tls"`
	Autojoin []string `toml:"join-channels"`
}

func LoadConfig(r io.Reader) (*Config, error) {
	var cfg Config
	_, err := toml.DecodeReader(r, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
