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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/jeromeberg/ollama-telegram-bot/src/ollama"

	// TODO v4 ?
	"gopkg.in/telebot.v3"
)

var botMentionKey = "self_mention"

// Регулярное выражение для поиска всех форматов URL: http://, https://, www.
var regexLinks = regexp.MustCompile(`(?mi)\b(?:http|https|www)\S+`)

var regexGif = regexp.MustCompile(`(?mi)\[(?:gif|гиф)\s*-\s*(.*?)\]`)


func handlers(tgBot *telebot.Bot, bot *bot) {
	tgBot.Handle(telebot.OnText, bot.botMiddleware(bot.handleMessage))
	tgBot.Handle(telebot.OnMedia, bot.botMiddleware(bot.handleMessage))
}


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

		if c.Message() == nil {
			return nil
		}

		if len(c.Message().Entities) > 0 &&
			c.Message().Entities[0].Type == telebot.EntityMention &&
			c.Message().Entities[0].Offset == 0 &&
			strings.Contains(c.Text(), "@"+c.Bot().Me.Username) {

			c.Set(botMentionKey, true)
		}

		return next(c)
	}
}

// Check message for trigger LLM
func (b *bot) isNeedProcessAnswer(message string, c telebot.Context) bool {

	if c.Bot().Me.ID == c.Message().Sender.ID {
		return false
	}

	if b.containsTriggerWord(strings.ToLower(message)) {
		return true
	}

	if c.Message().IsReply() && int(c.Message().ReplyTo.Sender.ID) == int(c.Bot().Me.ID) {
		return true
	}

	if c.Get(botMentionKey) != nil {
		return true
	}

	return false
}

func fetchPreview(link string) string {

	// check url for valid
	u, err := url.Parse(link)
	if err != nil || u.Scheme == "" || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	// check if url not localhost or local ip's
	if u.Host == "localhost" || strings.HasPrefix(u.Host, "127.0.0.") || strings.HasPrefix(u.Host, "192.168.") {
		return ""
	}

	previewText := ""

	// Fetch preview text from URL
	resp, err := http.Get(u.String())
	if err != nil {
		log.Printf("Error fetching preview: %v", err)
		return ""
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error fetching preview: %v", err)
		return ""
	}

	// Parse the HTML content and extract text from <header title> and <header description>
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("Error parsing HTML: %v", err)
		return ""
	}

	title := doc.Find("title").Text()
	description := doc.Find("meta[name=description]").AttrOr("content", "")

	if title != "" {
		previewText += fmt.Sprintf("*%s*\n", title)
	}
	if description != "" {
		previewText += fmt.Sprintf(" - %s\n", description)
	}

	return previewText
}

func (b *bot) processInputMessage(c telebot.Context) string {
	message := c.Text()

	message = strings.TrimSpace(removeLinks(message))
	message = strings.TrimSuffix(message, "@"+c.Bot().Me.Username)
	message = strings.TrimSpace(message)

	if c.Message().PreviewOptions != nil && !c.Message().PreviewOptions.Disabled {
		if c.Message().PreviewOptions.URL != "" {

			previewText := fetchPreview(c.Message().PreviewOptions.URL)

			if previewText != "" {
				message = fmt.Sprintf("User send link with text: %s", previewText)
			}
		}
	}

	if message == "" {
		return ""
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
				// extraMessage = extraMessage + "User replay to:" + c.Message().ReplyTo.Text
				extraMessage = fmt.Sprintf("%s User replay to:\"%s\"", extraMessage, c.Message().ReplyTo.Text)
				break
			case c.Message().ReplyTo.Caption != "":
				extraMessage = fmt.Sprintf("%s User replay to:\"%s\"", extraMessage, c.Message().ReplyTo.Caption)
				break
			}
		}

		if c.Message().Quote != nil && c.Message().Quote.Text != "" {
			extraMessage = fmt.Sprintf("%s User replay to:\"%s\"", extraMessage, c.Message().Quote.Text)
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

		message = strings.TrimSpace(extraMessage) + " " + message
	}

	return message
}

func (b *bot) processOutputMessage(msg string) string {

	replayMesage := msg

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

	isRemoveFromReplay := strings.Index(replayMesage, b.config.RemoveFromReplay)
	replayMesage = strings.ReplaceAll(replayMesage, b.config.RemoveFromReplay, "")
	// TODO remove (fix when message contains different substr (exmaple - "Assistant:" got - "Ассистант:" ))
	if isRemoveFromReplay == -1 {
		index := strings.Index(replayMesage, ":")
		if (index != -1 && index != 0) && index < 20 {
			replayMesage = replayMesage[index+1:]
		}
	}

	return replayMesage
}



func (b *bot) processCommands(c telebot.Context) (string, bool) {
	isMentioned, ok := c.Get(botMentionKey).(bool)
	if !ok {
		return "", false
	}

	if !isMentioned {
		return "", false
	}

	text := removeLinks(c.Text())
	rs := ""

	// Trim mention
	text = strings.TrimPrefix(text, "@")

	cmds := strings.SplitN(text, " ", 2)
	if len(cmds) < 2 {
		return "", false
	}
	if cmds[0] != c.Bot().Me.Username {
		return "", false
	}

	text = strings.TrimSpace(strings.TrimSpace(cmds[1]))

	switch {
	case strings.HasPrefix(text, "что ты помнишь"):
		if len(b.chatContexts.Memory.Data) == 0 {
			return "", false
		}

		rs = fmt.Sprintf("Меня просили запомнить:\n%s", b.chatContexts.Memory.GetList())
		return rs, true

	case strings.HasPrefix(text, "запомни"):

		cmdText := strings.SplitN(text, " ", 2)
		if len(cmdText) < 2 {
			return "", false
		}

		b.chatContexts.Memory.Add(cmdText[1])
		rs = fmt.Sprintf("Теперь я помню: %s", cmdText[1])
		return rs, true

	case strings.HasPrefix(text, "забудь"):

		cmdText := strings.SplitN(text, " ", 2)
		if len(cmdText) < 2 {
			return "", false
		}

		index, err := strconv.Atoi(cmdText[1])
		if err != nil {
			return "", false
		}

		old, ok := b.chatContexts.Memory.Remove(index - 1)
		if !ok {
			return "", false
		}

		rs = fmt.Sprintf("Забыл: %s", old)
		return rs, true

	}

	return "", false
}


func (b *bot) handleMessage(c telebot.Context) error {
	cmdResult, cmdOk := b.processCommands(c)

	if cmdOk {
		err := b.send(cmdResult, c)
		if err != nil {
			return err
		}
		return nil
	}

	message := b.processInputMessage(c)

	if message == "" {
		return nil
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

	// Skip old message when receive missing updates
	if c.Message().Time().Before(b.startTime) {
		return nil
	}

	if !b.isNeedProcessAnswer(message, c) {
		return nil
	}

	response := make(chan string)
	defer close(response)

	payload := b.makeChatRequest(newMessage)
	b.llmChan <- &data{payload, c, response}

	replayMesage := <-response

	if replayMesage == "" {
		log.Println("WARNING: Empty response from ollama")
		return nil
	}

	replayMesage = b.processOutputMessage(replayMesage)

	replay, storeReplay := b.makeReplay(replayMesage)

	err := b.send(replay, c)
	if err != nil {
		return err
	}

	if storeReplay {
		b.chatContexts.History.Add(Message{UserType: UserTypeAI, Message: replayMesage})
	}

	return nil
}

func (b *bot) makeReplay(replayMesage string) (any, bool) {
	var replay any
	var storeReplay bool

	switch {
	case regexGif.MatchString(replayMesage):
		match := regexGif.FindStringSubmatch(replayMesage)
		if len(match) > 1 {
			res, err := b.giphy.Translate(strings.Split(match[1], " "))
			if err != nil {
				log.Printf("Error: %v", err)
				return nil, false
			}

			replay = &telebot.Animation{File: telebot.File{FileURL: res.Data.MediaURL()}, Caption: fmt.Sprintf("_%s_\n`%s`", escapeMarkdownV2(match[1]), "Powered by GIPHY")}
		}

	default:
		replay = replayMesage
		storeReplay = true
	}

	return replay, storeReplay
}

func (b *bot) send(replay any, c telebot.Context) error {
	replayOpts := &telebot.SendOptions{
		ParseMode: telebot.ModeMarkdownV2,
		ReplyTo:   c.Message(),
	}

	rpls := []any{}

	switch v := replay.(type) {
	case string:
		v = escapeMarkdownV2(v)

		if utf8.RuneCountInString(v) > 4000 {
			runes := []rune(v)
			for i := 0; i < len(runes); i += 4000 {
				end := i + 4000
				if end > len(runes) {
					end = len(runes)
				}
				rpls = append(rpls, string(runes[i:end]))
			}
		} else {
			rpls = append(rpls, v)
		}

	case *telebot.Animation:
		rpls = append(rpls, v)
	}

	for _, r := range rpls {
		err := c.Send(r, replayOpts)
		if err != nil {
			log.Printf("Send replay error: %v, replay string: %v\n", err, replay)
			return err
		}
	}

	return nil
}

func (b *bot) SendMessageToChatGroup(chatID int64, msg string) error {
	if chatID != 0 {
		if _, err := b.tgBot.Send(&telebot.Chat{ID: chatID}, msg, telebot.Silent); err != nil {
			return fmt.Errorf("Cant send message: %v", err)
		}
		if b.config.EnableLog {
			log.Printf("Message sent to chat group: %v msg: %v \n", chatID, msg)
		}
	}
	b.chatContexts.History.Add(Message{UserType: UserTypeAI, Message: msg})
	return nil
}

func (b *bot) makeChatRequest(newMsg Message) *ollama.ChatRequest {
	systemMessage := ollama.MakeMessage(string(UserTypeSystem), b.config.SystemPrompt)

	if len(b.chatContexts.Memory.Data) > 0 {
		systemMessage.Content = fmt.Sprintf("%s\nТебя просили запомнить:\n%s", systemMessage.Content, b.chatContexts.Memory.GetList())
	}

	messages := []ollama.Message{systemMessage}

	for _, msg := range b.chatContexts.History.GetAll() {
		messages = append(messages, ollama.MakeMessage(string(msg.UserType), msg.Message))
	}

	messages = append(messages, ollama.MakeMessage(string(newMsg.UserType), newMsg.Message))

	payload := &ollama.ChatRequest{
		Model:    b.config.Model,
		Messages: messages,
		AdvancedParams: ollama.AdvancedParams{
			Options: &ollama.Options{
				Temperature: b.config.Temperature,
				NumCtx:      b.config.NumCtx,
			},
			Stream: false,
			// Format: "json",
		},
	}

	return payload
}

func (b *bot) processOllama() {
	// Listen channel for new requests
	for data := range b.llmChan {
		log.Println("Process ollama request")

		err := data.ctx.Notify(telebot.Typing)
		if err != nil {
			log.Printf("Send Notify error: %v\n", err)
		}

		resp, err := b.sendRequestOllama(data.request)

		if err != nil {
			log.Printf("Error: %v\n", err)
			data.response <- ""
			continue
		}

		data.response <- resp

		time.Sleep(5 * time.Second)
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

			part = strings.ReplaceAll(part, "\\", "\\\\")
			part = strings.ReplaceAll(part, ")", "\\)")

			link = strings.Replace(link, match[1], part, 1)
			linkMap[placeholder] = link
		}
	}

	for placeholder, block := range codeBlockMap {
		block = strings.TrimPrefix(block, "```")
		block = strings.TrimSuffix(block, "```")

		block = strings.ReplaceAll(block, "\\", "\\\\")
		block = strings.ReplaceAll(block, "`", "\\`")

		codeBlockMap[placeholder] = "```" + block + "```"
	}

	for placeholder, code := range inlineCodeMap {
		code = strings.TrimPrefix(code, "`")
		code = strings.TrimSuffix(code, "`")

		code = strings.ReplaceAll(code, "\\", "\\\\")
		code = strings.ReplaceAll(code, "`", "\\`")

		inlineCodeMap[placeholder] = "`" + code + "`"
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
