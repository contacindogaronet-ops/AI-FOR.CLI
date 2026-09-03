// internal/core/memory.go
package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"aicli/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Memory struct {
	History []Message `json:"history"`
}

func getMemoryPath() string {
	return filepath.Join(config.GetTermuxHome(), ".config", "aicli", "memory.json")
}

func LoadMemory() *Memory {
	path := getMemoryPath()
	file, err := os.Open(path)
	if err != nil {
		return &Memory{History: []Message{}}
	}
	defer file.Close()

	var mem Memory
	_ = json.NewDecoder(file).Decode(&mem)
	return &mem
}

func (m *Memory) Save(role, content string) {
	m.History = append(m.History, Message{Role: role, Content: content})
	if len(m.History) > 20 {
		m.History = m.History[len(m.History)-20:]
	}

	path := getMemoryPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(path, data, 0644)
}

func (m *Memory) Clear() {
	m.History = []Message{}
	path := getMemoryPath()
	_ = os.Remove(path)
}
