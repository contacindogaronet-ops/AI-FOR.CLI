package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"aicli/internal/config"
	"aicli/internal/core"
)

func RunInteractiveMenu(cfg *config.Config, pool *core.AIClientPool, memory *core.Memory) {
	reader := bufio.NewReader(os.Stdin)
	currentModel := cfg.DefaultModel

	for {
		fmt.Println("\n=========================================")
		fmt.Println("       AI MULTI-PROVIDER CLI ENGINE      ")
		fmt.Println("=========================================")
		fmt.Printf("Model/Provider Aktif : [%s]\n", currentModel)
		fmt.Printf("Memori Sesi Aktif    : %d pesan tercatat\n", len(memory.History))
		fmt.Println("-----------------------------------------")
		fmt.Println("[1] Kirim Prompt / Chat")
		fmt.Println("[2] Kelola API Keys (Tambah / Hapus)")
		fmt.Println("[3] Ganti Provider Aktif")
		fmt.Println("[4] Bersihkan Memori Otak (Reset Context)")
		fmt.Println("[q] Keluar")
		fmt.Println("-----------------------------------------")
		fmt.Print("Pilih menu [1-4/q]: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			fmt.Print("\nMasukkan prompt kamu: ")
			pInput, _ := reader.ReadString('\n')
			prompt := strings.TrimSpace(pInput)
			if prompt == "" {
				fmt.Println("Prompt tidak boleh kosong!")
				continue
			}

			fmt.Printf("\n[Mengirim dengan provider: %s]...\n", currentModel)
			err = pool.ExecuteWithFailover(currentModel, prompt, memory)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}

		case "2":
			manageAPIKeysMenu(cfg, pool, reader)

		case "3":
			fmt.Println("\nDaftar Provider Tersedia:")
			for idx, p := range cfg.Providers {
				fmt.Printf("[%d] %s (%d API Keys terdaftar)\n", idx+1, p.Name, len(p.APIKeys))
			}
			fmt.Print("Pilih nama provider: ")
			mInput, _ := reader.ReadString('\n')
			target := strings.TrimSpace(mInput)
			
			found := false
			for _, p := range cfg.Providers {
				if p.Name == target {
					currentModel = p.Name
					found = true
					fmt.Printf("-> Berhasil beralih ke provider: %s\n", currentModel)
					break
				}
			}
			if !found {
				fmt.Println("-> Provider tidak valid!")
			}

		case "4":
			memory.Clear()
			fmt.Println("-> Memori otak berhasil dibersihkan (Reset sesi baru).")

		case "q", "exit":
			fmt.Println("Keluar dari program...")
			return

		default:
			fmt.Println("Pilihan tidak valid.")
		}
	}
}

func manageAPIKeysMenu(cfg *config.Config, pool *core.AIClientPool, reader *bufio.Reader) {
	for {
		fmt.Println("\n--- KELOLA API KEYS ---")
		for idx, p := range cfg.Providers {
			fmt.Printf("[%d] Provider: %s (Total Key: %d)\n", idx+1, p.Name, len(p.APIKeys))
			for kidx, k := range p.APIKeys {
				masked := k
				if len(k) > 10 {
					masked = k[:6] + "..." + k[len(k)-4:]
				}
				fmt.Printf("    - [%d.%d] %s\n", idx+1, kidx+1, masked)
			}
		}
		fmt.Println("-----------------------------------------")
		fmt.Println("[1] Tambah API Key")
		fmt.Println("[2] Hapus API Key")
		fmt.Println("[b] Kembali ke Menu Utama")
		fmt.Print("Pilih [1/2/b]: ")

		input, _ := reader.ReadString('\n')
		opt := strings.TrimSpace(input)

		if opt == "b" || opt == "B" {
			break
		}

		if opt == "1" {
			fmt.Print("Masukkan nama provider (gemini/deepseek/claude/gpt): ")
			pName, _ := reader.ReadString('\n')
			pName = strings.TrimSpace(pName)

			fmt.Print("Masukkan string API Key baru: ")
			newKey, _ := reader.ReadString('\n')
			newKey = strings.TrimSpace(newKey)

			if newKey == "" {
				fmt.Println("API Key kosong!")
				continue
			}

			updated := false
			for i := range cfg.Providers {
				if cfg.Providers[i].Name == pName {
					cfg.Providers[i].APIKeys = append(cfg.Providers[i].APIKeys, newKey)
					updated = true
					break
				}
			}

			if !updated {
				cfg.Providers = append(cfg.Providers, config.ProviderConfig{
					Name:    pName,
					Type:    pName,
					APIKeys: []string{newKey},
				})
			}

			_ = config.SaveConfig(cfg)
			*pool = *core.InitPool(cfg)
			fmt.Println("-> API Key berhasil ditambahkan dan disimpan!")

		} else if opt == "2" {
			fmt.Print("Masukkan nama provider yang ingin dihapus key-nya: ")
			pName, _ := reader.ReadString('\n')
			pName = strings.TrimSpace(pName)

			for i := range cfg.Providers {
				if cfg.Providers[i].Name == pName {
					if len(cfg.Providers[i].APIKeys) == 0 {
						fmt.Println("Tidak ada key di provider ini.")
						break
					}
					for kidx, k := range cfg.Providers[i].APIKeys {
						fmt.Printf("[%d] %s\n", kidx+1, k)
					}
					fmt.Print("Pilih nomor index key yang akan dihapus: ")
					var kidx int
					_, err := fmt.Scanf("%d", &kidx)
					_, _ = reader.ReadString('\n')

					if err == nil && kidx >= 1 && kidx <= len(cfg.Providers[i].APIKeys) {
						idxToRemove := kidx - 1
						cfg.Providers[i].APIKeys = append(cfg.Providers[i].APIKeys[:idxToRemove], cfg.Providers[i].APIKeys[idxToRemove+1:]...)
						_ = config.SaveConfig(cfg)
						*pool = *core.InitPool(cfg)
						fmt.Println("-> API Key berhasil dihapus!")
					} else {
						fmt.Println("Index key tidak valid!")
					}
					break
				}
			}
		}
	}
}
