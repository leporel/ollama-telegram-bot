package main

import (
	"sync"
)

type UserType string

const (
	UserTypeSystem UserType = "system"
	UserTypeAI     UserType = "assistant"
	UserTypeUser   UserType = "user"
)

type Message struct {
	UserType UserType 
	Message  string   
}

type ChatContext struct {
	Chat    int64
	History *BoundedList 
}

type BoundedList struct {
	mu      sync.Mutex
	data    []Message
	limit   int
}

func NewBoundedList(limit int) *BoundedList {
	return &BoundedList{
		data:  make([]Message, 0),
		limit: limit,
	}
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