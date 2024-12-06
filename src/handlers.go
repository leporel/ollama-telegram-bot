package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jeromeberg/ollama-telegram-bot/src/ollama"

	// TODO v4 ?
	"gopkg.in/telebot.v3"
)

// Регулярное выражение для поиска всех форматов URL: http://, https://, www.
var regexLinks = regexp.MustCompile(`\b(?:http|https|www)\S+`)

// Deny messages from not witelisted users and chats
func validateChat(config *Config, c telebot.Context) bool {
	chatID := c.Chat().ID

	if chatID == config.ChatGroupID {
		return true
	}

	return false
}

func removeLinks(text string) string {
	cleanedText := regexLinks.ReplaceAllString(text, "")

	return cleanedText
}

func (b *bot) containsTriggerWord(message string) bool {
	for _, word := range b.config.TriggerWords {
		if strings.Contains(strings.ToLower(message), strings.ToLower(word)) {
			return true
		}
	}
	return false
}

// Middleware for bot context
func (b *bot) botMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc {
	return func(c telebot.Context) error {

		if b.config.EnableLog {
			log.Printf("[%d] %d: %s", c.Chat().ID, c.Sender().ID, c.Message().Text)
		}

		if !validateChat(b.config, c) {
			if b.config.EnableLog {
				log.Printf("ACCESS DENY - chat_id:%v user_id:%v \n", c.Chat().ID, c.Sender().ID)
			}
			return nil
		}

		return next(c)
	}
}

// Check message for trigger LLM
func (b *bot) isNeedProcess(message string, c telebot.Context) bool {
	if c.Bot().Me.ID == c.Message().Sender.ID {
		return false
	}

	if b.containsTriggerWord(strings.ToLower(message)) {
		return true
	}

	if c.Message().IsReply() && int(c.Message().ReplyTo.Sender.ID) == int(c.Bot().Me.ID) {
		return true
	}

	return false
}

func handlers(tgBot *telebot.Bot, bot *bot) {
	// TODO Skip updates
	// TODO Make persist storage for last messages?
	tgBot.Handle(telebot.OnText, bot.botMiddleware(bot.handleMessage))
	tgBot.Handle(telebot.OnMedia, bot.botMiddleware(bot.handleMessage))
}


func (b *bot) handleMessage(c telebot.Context) error {
	message := c.Text()

	message = strings.TrimSpace(removeLinks(message))
	message = strings.TrimSpace(message)

	if message == "" {
		return nil
	}

	sender := ""
	if c.Sender().Username != "" {
		sender = fmt.Sprintf("@%s", c.Sender().Username)
	} 
	if c.Sender().FirstName != "" {
		sender = fmt.Sprintf("%s (%s)", sender, c.Sender().FirstName)
	}
	message = sender + ": " + message

	// проверить что сообщение содержит полный его контест (реплаи, форварды и т.д.) и записать его текст в текущее сообщение
	if c.Message().IsForwarded() || c.Message().IsReply() {

		extraMessage := ""

		if c.Message().ReplyTo != nil {
			switch {
				case c.Message().ReplyTo.Text != "":
					extraMessage = extraMessage + "User replay to:" + c.Message().ReplyTo.Text
					break
				case c.Message().ReplyTo.Caption != "":
					extraMessage = extraMessage + "User replay to:" + c.Message().ReplyTo.Caption
					break
			}
		}

		if c.Message().Quote != nil && c.Message().Quote.Text != "" {
			extraMessage = extraMessage + "User replay to:" + c.Message().Quote.Text
		}

		if c.Message().IsForwarded() {
				switch {
				case c.Message().OriginalSenderName != "":
					extraMessage = fmt.Sprintf(`%s User forward this message from:"%s"`, extraMessage, c.Message().OriginalSenderName)
					break
				case c.Message().OriginalChat != nil && c.Message().OriginalChat.Type == telebot.ChatChannel:
					extraMessage = fmt.Sprintf(`%s User forward this message from:"%s"`, extraMessage, c.Message().OriginalChat.Title)
					break
				}
		}

		message = extraMessage + " " + message
	}


	var userRole UserType

	switch c.Sender().ID {
	case c.Bot().Me.ID:
		userRole = UserTypeAI
	default:
		userRole = UserTypeUser
	}

	// make new context record
	newMessage := Message{
		UserType: userRole,
		Message:  message,
	}

	b.chatContexts.History.Add(newMessage)

	if !b.isNeedProcess(message, c) {
		return nil
	}

	systemMessage := ollama.MakeMessage(string(UserTypeSystem), b.config.SystemPrompt)

	messages := []ollama.Message{systemMessage}

	for _, msg := range b.chatContexts.History.GetAll() {
		messages = append(messages, ollama.MakeMessage(string(msg.UserType), msg.Message))
	}

	payload := &ollama.ChatRequest{
		Model:    b.config.Model,
		Messages: messages,
		AdvancedParams: ollama.AdvancedParams{
			Options: ollama.Options{
				Temperature: b.config.Temperature,
				NumCtx:      b.config.NumCtx,
			},
			Stream: false,
			// Format: "json",
		},
	}

	response := make(chan string)
	defer close(response)

	b.llmChan <- &data{payload, c, response}

	err := c.Notify(telebot.Typing)
	if err != nil {
		log.Printf("Send Notify error: %v\n", err)
	}

	replayMesage := <-response

	if replayMesage == "" {
		log.Println("WARNING: Empty response from ollama")
		return nil
	}

	// Если ответ пришел в ввиде json'на
	var tmp map[string]interface{}
	if err := json.Unmarshal([]byte(replayMesage), &tmp); err == nil {
		for _, v := range tmp {
			msg, ok := v.(string)
			if !ok {
				log.Println("ERROR: When cast string")
			} else {
				replayMesage = msg
			}
		}
	}

	b.chatContexts.History.Add(Message{UserType: UserTypeAI, Message: replayMesage})

	isRemoveFromReplay := strings.Index(replayMesage, b.config.RemoveFromReplay)
	replayMesage = strings.ReplaceAll(replayMesage, b.config.RemoveFromReplay, "")
	// TODO remove (fix when message contains different substr (exmaple - "Assistant:" got - "Ассистант:" ))
	if isRemoveFromReplay < -1 {
		index := strings.Index(replayMesage, ":")
		if (index != -1 && index != 0) && index < 20 {
			replayMesage = replayMesage[index+1:]
		}
	}

	replayOpts := &telebot.SendOptions{
		ParseMode: telebot.ModeMarkdownV2,
		ReplyTo:   c.Message(),
	}

	replayMesage = escapeMarkdownV2(replayMesage)
	
	err = c.Send(replayMesage, replayOpts)
	if err != nil {
			log.Printf("Send replay error: %v, replay string: %v\n", err, replayMesage)
	}

	return err
}

func (b *bot) processOllama() {
	// Listen channel for new requests
	for data := range b.llmChan {
		time.Sleep(3 * time.Second)
		log.Println("Process ollama request")

		resp, err := b.sendRequestOllama(data.request)

		if err != nil {
			log.Printf("Error: %v\n", err)
			data.response <- ""
			continue
		}

		data.response <- resp
	}
}

func (b *bot) sendRequestOllama(payload *ollama.ChatRequest) (string, error) {

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", nil
	}

	if b.config.EnableLog {
		log.Printf("Request: %s\n", string(jsonData))
	}

	// make url request /api/chat
	serverURL, err := url.JoinPath(b.config.ServerURL, "/api/chat")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", serverURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", nil
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var response ollama.ChatResponse

	if b.config.EnableLog {
		log.Printf("Responce: %s\n", string(body))
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}

	res := response.Message.Content

	// Log
	if b.config.EnableLog {
		log.Printf("[responce message] %s\n", res)
	}

	return res, nil
}

// Регулярное выражение для поиска части (...) встроенных ссылок и кастомных эмодзи
// TODO FIX Не ловит строку типа [text](lnk://c.c/pa=)_\), думая что первая скобка закрывающая
var inlineLinkRegex = regexp.MustCompile(`\[.*?\]\((.*?)\)`) 

// Регулярное выражение для поиска блоков кода
var codeBlockRegex = regexp.MustCompile("(?s)```.*?```")
var lineCodeRegex = regexp.MustCompile("`[^`\n]+`")


func escapeMarkdownV2(text string) string {
	// Список символов, которые нужно экранировать
	escapeChars := `_*[]()~` + "`" + `>#+-=|{}.!`

	linkMap := make(map[string]string)
	codeBlockMap := make(map[string]string)
	inlineCodeMap := make(map[string]string)
	
	// Замена оригинальных блоков на заполнители
	var linkIndex int
	text = inlineLinkRegex.ReplaceAllStringFunc(text, func(link string) string {
		linkIndex++
		placeholder := fmt.Sprintf("$$LINK$%d$$", linkIndex)
		linkMap[placeholder] = link
		return placeholder
	})

	var codeBlockIndex int
	text = codeBlockRegex.ReplaceAllStringFunc(text, func(block string) string {
		codeBlockIndex++
		placeholder := fmt.Sprintf("$$BLOCKCODE$%d$$", linkIndex)
		codeBlockMap[placeholder] = block
		return placeholder
	})

	var inlineCodeIndex int
	text = lineCodeRegex.ReplaceAllStringFunc(text, func(code string) string {
		inlineCodeIndex++
		placeholder := fmt.Sprintf("$$INLINECODE$%d$$", linkIndex)
		inlineCodeMap[placeholder] = code
		return placeholder
	})


	// Экранирование символов внутри заполнителей
	for placeholder, link := range linkMap {
		match := inlineLinkRegex.FindStringSubmatch(link)
		if len(match) > 1 {
			part := match[1]

			part = strings.ReplaceAll(part, ")", "\\)")
			part = strings.ReplaceAll(part, "\\", "\\\\")

			link = strings.Replace(link, match[1], part, 1)
			linkMap[placeholder] = link
		}
	}

	for placeholder, block := range codeBlockMap {
		block = strings.TrimPrefix(block, "```")
		block = strings.TrimSuffix(block, "```")

		block = strings.ReplaceAll(block, "`", "\\`")
		block = strings.ReplaceAll(block, "\\", "\\\\")
		codeBlockMap[placeholder] = "```"+block+"```"	
	}

	for placeholder, code := range inlineCodeMap {
		code = strings.TrimPrefix(code, "`")
		code = strings.TrimSuffix(code, "`")

		code = strings.ReplaceAll(code, "`", "\\`")
		code = strings.ReplaceAll(code, "\\", "\\\\")
		inlineCodeMap[placeholder] = "`"+code+"`"	
	}

	text = strings.ReplaceAll(text, "\\", `\\`)

	// Заменяем каждый символ на экранированный
	for _, char := range escapeChars {
		text = strings.ReplaceAll(text, string(char), "\\"+string(char))
	}

	// Возращаем блоки
	for placeholder, link := range linkMap {
		text = strings.ReplaceAll(text, placeholder, link)
	}

	for placeholder, block := range codeBlockMap {
		text = strings.ReplaceAll(text, placeholder, block)
	}

	for placeholder, code := range inlineCodeMap {
		text = strings.ReplaceAll(text, placeholder, code)
	}

	return text
}
