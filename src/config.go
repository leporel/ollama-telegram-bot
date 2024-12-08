package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Config struct {
	BotToken          string   `json:"botToken"`
	Model             string   `json:"model"`
	ServerURL         string   `json:"serverUrl"`
	EnableLog         bool     `json:"enableLog"`
	ChatGroupID       int64    `json:"chatGroupId"`
	SystemPrompt      string   `json:"systemPrompt"`
	Temperature       float64  `json:"temperature"`
	NumCtx            int      `json:"numCtx"`
	GreetingMessage   string   `json:"greetingMessage"`
	GoodbyeMessage    string   `json:"goodbyeMessage"`
	TriggerWords      []string `json:"triggerWords"`
	RemoveFromReplay  string   `json:"removeFromReplay"`
	HistorySize       int      `json:"historySize"`
	EnableSaveHistory bool     `json:"enableSaveHistory"`
}

func loadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Config open error: %v", err)
	}
	defer file.Close()

	config := &Config{}
	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Config read error: %v", err)
	}

	err = json.Unmarshal(bytes, config)
	if err != nil {
		return nil, fmt.Errorf("Config parse error: %v", err)
	}

	return config, nil
}
