package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
)

type UserType string

const (
	UserTypeSystem UserType = "system"
	UserTypeAI     UserType = "assistant"
	UserTypeUser   UserType = "user"
)

type Message struct {
	UserType UserType  `json:"user_type"`
	Message  string    `json:"message"`
}

type ChatContext struct {
	Chat    int64
	History *BoundedList 
}

type BoundedList struct {
	mu      sync.Mutex
	data    []Message
	limit   int
	filename string
}

func NewBoundedList(limit int, filename string, load bool) *BoundedList {
	bm := &BoundedList{
		data:  make([]Message, 0),
		limit: limit,
		filename: filename,
	}

	if !load {
		return bm
	}

	if err := bm.loadFromFile(); err != nil{
		log.Println("Error loading from file:", err)
	}
	return bm
}

func (bm *BoundedList) Add(value Message) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.data) >= bm.limit {
		// Remove the oldest element
		bm.data = bm.data[1:]
	}

	bm.data = append(bm.data, value)
}

func (bm *BoundedList) GetAll() []Message {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	elements := make([]Message, len(bm.data))
	copy(elements, bm.data)
	return elements
}

// Clear BoundedList
func (bm *BoundedList) Clear() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.data = []Message{}
}


// Save BoundedList to json file
func (bm *BoundedList) SaveToFile() error {
	file, err := os.Create(bm.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Convert the data to JSON format
	jsonData, err := json.MarshalIndent(bm.data, "", "  ")
	if err != nil {
		return err
	}

	// Write the data to the file
	_, err = file.Write(jsonData)
	if err != nil {
		return err
	}

	log.Printf("History [%d messages] saved to file: %s", len(bm.data), bm.filename)

	return nil
}

// Load BoundedList from file
func (bm *BoundedList) loadFromFile() error {
	// Check if file exists before attempting to load
	if _, err := os.Stat(bm.filename); os.IsNotExist(err) {
		log.Println("History file does not found.")
		return nil 
	}
	
	file, err := os.Open(bm.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the data from the file
	jsonData, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Unmarshal JSON data into a slice of Message structs
	err = json.Unmarshal(jsonData, &bm.data)
	if err != nil {
		return err
	}

	log.Printf("History [%d messages] loaded", len(bm.data))

	return nil
}