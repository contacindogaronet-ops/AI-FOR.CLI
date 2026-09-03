package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"aicli/internal/config"
	"aicli/internal/core"
)

// RunInteractiveMenu adalah fungsi utama yang dipanggil oleh main.go
func RunInteractiveMenu(cfg *config.Config, pool *core.AIClientPool, memory *core.Memory) {
	StartUIWithPool(cfg, pool, memory)
}

func StartUI(cfg *config.Config) {
	pool := core.InitPool(cfg)
	memory := core.LoadMemory()
	StartUIWithPool(cfg, pool, memory)
}

func StartUIWithPool(cfg *config.Config, pool *core.AIClientPool, memory *core.Memory) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Println("\n========================================")
		fmt.Println("      AI MULTI-PROVIDER CLI ENGINE      ")
		fmt.Println("========================================")
		fmt.Printf("Model/Provider Aktif : [%s]\n", cfg.DefaultModel)
		fmt.Printf("Memori Sesi Aktif    : %d pesan tercatat\n", len(memory.History))
		fmt.Println("----------------------------------------")
		fmt.Println("[1] Kirim Prompt / Chat")
		fmt.Println("[2] Kelola API Keys (Tambah / Hapus)")
		fmt.Println("[3] Ganti Provider Aktif")
		fmt.Println("[4] Bersihkan Memori Otak (Reset Context)")
		fmt.Println("[5] Diagnosa Otomatis Sistem HP & Termux")
		fmt.Println("[6] Ping & Cek Status Kesehatan Semua API Key")
		fmt.Println("[q] Keluar")
		fmt.Println("----------------------------------------")
		fmt.Print("Pilih menu [1-6/q]: ")

		if !scanner.Scan() {
			break
		}
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			fmt.Println("\nMasukkan prompt Anda (Mendukung paste kode multi-baris).")
			fmt.Println("-> Tekan Enter lalu Ctrl+D pada baris kosong untuk mengirim eksekusi:")

			var sb strings.Builder
			// Loop ini akan menelan semua baris termasuk breakline (\n) dari hasil paste
			for scanner.Scan() {
				sb.WriteString(scanner.Text() + "\n")
			}

			// [CRITICAL FIX]: Menghapus cache io.EOF dari bufio.Scanner
			// Wajib dilakukan agar iterasi menu berikutnya tetap bisa menerima ketikan keyboard
			scanner = bufio.NewScanner(os.Stdin)

			prompt := strings.TrimSpace(sb.String())
			if prompt != "" {
				fmt.Printf("\n[Mengirim dengan provider: %s]...\n", cfg.DefaultModel)
				err := pool.ExecuteWithFailover(cfg.DefaultModel, prompt, memory)
				if err != nil {
					fmt.Printf("[Error] Gagal mengeksekusi request: %v\n", err)
				}
			}
		case "2":
			fmt.Println("\n[Kelola API Keys]")
			fmt.Print("Masukkan nama provider (gemini/deepseek/openai/claude): ")
			if scanner.Scan() {
				pName := strings.TrimSpace(scanner.Text())
				fmt.Print("Masukkan API Key baru: ")
				if scanner.Scan() {
					apiKey := strings.TrimSpace(scanner.Text())
					if pName != "" && apiKey != "" {
						found := false
						for i := range cfg.Providers {
							if cfg.Providers[i].Name == pName {
								cfg.Providers[i].APIKeys = append(cfg.Providers[i].APIKeys, apiKey)
								found = true
								break
							}
						}
						if !found {
							cfg.Providers = append(cfg.Providers, config.ProviderConfig{
								Name:    pName,
								Type:    pName,
								APIKeys: []string{apiKey},
							})
						}
						_ = config.SaveConfig(cfg)
						pool = core.InitPool(cfg)
						fmt.Println("[Sukses] API Key berhasil ditambahkan dan disimpan.")
					}
				}
			}
		case "3":
			fmt.Print("Masukkan nama provider baru (gemini/deepseek/openai/claude): ")
			if scanner.Scan() {
				newModel := strings.TrimSpace(scanner.Text())
				if newModel != "" {
					cfg.DefaultModel = newModel
					_ = config.SaveConfig(cfg)
					fmt.Printf("[Sukses] Provider aktif diubah ke: %s\n", newModel)
				}
			}
		case "4":
			memory.Clear()
			fmt.Println("[Sukses] Memori otak berhasil dibersihkan.")
		case "5":
			fmt.Println("\n[DIAGNOSA OTOMATIS SISTEM HP & TERMUX]")
			ramInfo := core.RunSystemDiagnostics("free -h")
			diskInfo := core.RunSystemDiagnostics("df -h")
			procInfo := core.RunSystemDiagnostics("ps aux | head -n 15")

			diagnosticSummary := fmt.Sprintf("--- RAM INFO ---\n%s\n\n--- DISK STORAGE ---\n%s\n\n--- TOP PROCESSES ---\n%s", ramInfo, diskInfo, procInfo)
			fmt.Println(diagnosticSummary)

			prompt := "Analisis kondisi sistem HP dan Termux saya berdasarkan data diagnostik ini, berikan rekomendasi optimasi:\n" + diagnosticSummary
			fmt.Printf("\n[Mengirim data diagnosa ke AI: %s]...\n", cfg.DefaultModel)
			err := pool.ExecuteWithFailover(cfg.DefaultModel, prompt, memory)
			if err != nil {
				fmt.Printf("[Error] Gagal menganalisis sistem: %v\n", err)
			}
		case "6":
			fmt.Println("\n[PULSE CHECK / PING KESEHATAN SELURUH API KEY]...")
			healthStatuses := pool.PingAllProviders()
			fmt.Println("\n========================================================")
			fmt.Println(" PROVIDER   | KEY INDEX | KEY MASK   | STATUS   | DETAIL")
			fmt.Println("--------------------------------------------------------")
			for _, h := range healthStatuses {
				statusColored := h.Status
				if h.Status == "ONLINE" {
					statusColored = "[ONLINE]"
				} else {
					statusColored = "[OFFLINE]"
				}
				fmt.Printf(" %-10s | [%-9d] | %-10s | %-8s | %s\n", h.Provider, h.KeyIndex, h.KeyMask, statusColored, h.Reason)
			}
			fmt.Println("========================================================")
		case "q":
			fmt.Println("Keluar dari program. Sampai jumpa!")
			return
		default:
			fmt.Println("[Error] Pilihan tidak valid.")
		}
	}
}
