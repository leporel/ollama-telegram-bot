package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jeromeberg/ollama-telegram-bot/src/ollama"
	"github.com/peterhellberg/giphy"
	"gopkg.in/telebot.v3"
)

type bot struct {
	tgBot        *telebot.Bot
	config       *Config
	chatContexts *ChatContext
	llmChan      chan *data
	startTime    time.Time
	giphy 		*giphy.Client
}

type data struct {
	request  *ollama.ChatRequest
	ctx      telebot.Context
	response chan string
}

func main() {

	configFile := ""

	// Get config file from command line argument -c or use default
	if len(os.Args) > 1 {
		for i := range os.Args {
			if os.Args[i] == "-c" && i+1 < len(os.Args) {
				configFile = os.Args[i+1]
				break
			}
		}
	}

	// Load config from file or use default if not found
	if configFile == "" {
		configFile = "config.json"
	}

	log.Printf("Config file: %v\n", configFile)

	// Config
	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Logs
	if config.EnableLog {
		/* 		logFile, err := os.OpenFile("log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		   		if err != nil {
		   			log.Fatalf("Error log file: %v", err)
		   		}
		   		defer logFile.Close()
		   		multiWriter := io.MultiWriter(os.Stdout, logFile)
		   		log.SetOutput(multiWriter) */
		log.SetFlags(log.Ldate | log.Ltime)
	}

	if config.EnableLog {
		log.Printf("Config loaded:\n%s\n", spew.Sprintf("%+v", *config))
	}

	// Telegram bot
	tgBot, err := telebot.NewBot(telebot.Settings{
		Token: config.BotToken,
		Poller: &telebot.LongPoller{
			Timeout:      10 * time.Second,
			LastUpdateID: 0,
		},
	})

	if err != nil {
		log.Println(err)
		return
	}

	chatContexts := NewChatContext(config.ChatGroupID, config.HistorySize, fmt.Sprintf("./%d_history.json", config.ChatGroupID), config.EnableSaveHistory)

	chatBot := &bot{
		tgBot:        tgBot,
		config:       config,
		chatContexts: chatContexts,
		llmChan:      make(chan *data, 1),
		startTime:    time.Now(),
		giphy: 		  giphy.NewClient(giphy.APIKey(config.GiphyAPIKey), giphy.Rating("r")),
	}

	// Handlers
	handlers(tgBot, chatBot)

	log.Println("ollama-telegram-bot running...")
	go chatBot.processOllama()

	// Send hello to chat group
	err = chatBot.SendMessageToChatGroup(config.ChatGroupID, config.GreetingMessage)
	if err != nil {
		log.Println(err)
	}

	// Handle interapt signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go func() {
		s := <-sig
		log.Println("Received interrupt signal:", s)
		log.Println("Stopping bot...")

		// Send goodbye to chat group
		err = chatBot.SendMessageToChatGroup(config.ChatGroupID, config.GoodbyeMessage)
		if err != nil {
			log.Println(err)
		}

		if config.EnableSaveHistory {
			if err = chatContexts.SaveToFile(); err != nil {
				log.Println(err)
			}
		}

		tgBot.Stop()
		log.Println("Bot stopped")
		os.Exit(0)
	}()

	tgBot.Start()
}
