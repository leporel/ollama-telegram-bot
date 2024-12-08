package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"slices"
	"sync"
)

type UserType string

const (
	UserTypeSystem UserType = "system"
	UserTypeAI     UserType = "assistant"
	UserTypeUser   UserType = "user"
)

type Message struct {
	UserType UserType `json:"user_type"`
	Message  string   `json:"message"`
}

type ChatContext struct {
	Chat    int64        `json:"-"`
	History *BoundedList `json:"history"`
	Memory  *Memory     `json:"memory"`
	filename string		 `json:"-"`
}

type Memory struct {
	mu       sync.Mutex `json:"-"`
	Data     []string   `json:"data"`
}

func (m *Memory) GetList() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	rs := ""
	for i, v := range m.Data {
		rs = fmt.Sprintf("%d. %s\n", i+1, v)
	}
	return rs
}

func (m *Memory) Add(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Data = append(m.Data, message)
}

// Index from 0 to len-1
func (m *Memory) Remove(index int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.Data) {
		return "", false
	}

	removedString := m.Data[index]
	m.Data = slices.Delete(m.Data, index, index+1)

	return removedString, true
}

type BoundedList struct {
	mu       sync.Mutex 	`json:"-"`
	Data     []Message 		`json:"data"`
	limit    int			`json:"-"`
}

func NewChatContext(chatID int64, limit int, filename string, load bool) *ChatContext {
	bm := &BoundedList{
		Data:     make([]Message, 0),
		limit:    limit,
	}

	ctxChat := &ChatContext{
		Chat:    chatID,
		History: bm,
		Memory:  &Memory{sync.Mutex{}, []string{}},
		filename: filename,
	}

	if !load {
		return ctxChat
	}

	if err := ctxChat.loadFromFile(); err != nil {
		log.Println("Error loading from file:", err)
	}

	return ctxChat
}

func (bm *BoundedList) Add(value Message) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.Data) >= bm.limit {
		// Remove the oldest element
		bm.Data = bm.Data[1:]
	}

	bm.Data = append(bm.Data, value)
}

func (bm *BoundedList) GetAll() []Message {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	elements := make([]Message, len(bm.Data))
	copy(elements, bm.Data)
	return elements
}

// Clear BoundedList
func (bm *BoundedList) Clear() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	clear(bm.Data)
}

// Save BoundedList to json file
func (cc *ChatContext) SaveToFile() error {
	file, err := os.Create(cc.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Convert the data to JSON format
	jsonData, err := json.MarshalIndent(cc, "", "  ")
	if err != nil {
		return err
	}

	// Write the data to the file
	_, err = file.Write(jsonData)
	if err != nil {
		return err
	}

	log.Printf("History [%d messages] saved to file: %s", len(cc.History.Data), cc.filename)

	return nil
}

// Load BoundedList from file
func (cc *ChatContext) loadFromFile() error {
	// Check if file exists before attempting to load
	if _, err := os.Stat(cc.filename); os.IsNotExist(err) {
		log.Println("History file does not found.")
		return nil
	}

	file, err := os.Open(cc.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the data from the file
	jsonData, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	var newCc ChatContext
	// Unmarshal JSON data into a slice of Message structs
	err = json.Unmarshal(jsonData, &newCc)
	if err != nil {
		return err
	}

	cc.Memory = newCc.Memory
	for _, v := range newCc.History.Data {
		cc.History.Add(v)
	}

	log.Printf("History [%d messages] loaded", len(cc.History.Data))

	return nil
}
