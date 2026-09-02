package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Message struct {
	Role    string `json:"role"` // "user" atau "model"
	Content string `json:"content"`
}

type Memory struct {
	History []Message `json:"history"`
}

func getMemoryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aicli", "memory.json")
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
	// Batasi memori maksimal 20 riwayat terakhir agar tidak bloat
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
