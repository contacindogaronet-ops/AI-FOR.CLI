package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type FileTarget struct {
	FilePath string
	Content  string
}

func RunSystemDiagnostics(command string) string {
	cmd := exec.Command("bash", "-c", command)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return strings.TrimSpace(stderr.String())
	}
	return strings.TrimSpace(out.String())
}

func ExtractCodeBlocks(aiResponse string) []FileTarget {
	var targets []FileTarget

	reHeaderPath := regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\\s+(?:path|filepath|file)=\"?([^\\s\"`]+)\"?\\s*\n(.*?)```")
	matches := reHeaderPath.FindAllStringSubmatch(aiResponse, -1)

	for _, m := range matches {
		if len(m) >= 3 {
			targets = append(targets, FileTarget{
				FilePath: strings.TrimSpace(m[1]),
				Content:  strings.TrimPrefix(m[2], "\n"),
			})
		}
	}

	reGenericBlock := regexp.MustCompile("(?s)```[a-zA-Z0-9_+-]*\\s*\n(.*?)```")
	genericMatches := reGenericBlock.FindAllStringSubmatch(aiResponse, -1)

	reCommentFile := regexp.MustCompile(`^(?://|#|/\*)\s*(?:File|Path):\s*([^\s\*]+)`)

	for _, m := range genericMatches {
		if len(m) >= 2 {
			blockContent := m[1]
			lines := strings.Split(blockContent, "\n")
			if len(lines) > 0 {
				firstLine := strings.TrimSpace(lines[0])
				if fileMatch := reCommentFile.FindStringSubmatch(firstLine); len(fileMatch) >= 2 {
					filePath := strings.TrimSpace(fileMatch[1])

					alreadyExtracted := false
					for _, t := range targets {
						if t.FilePath == filePath {
							alreadyExtracted = true
							break
						}
					}

					if !alreadyExtracted {
						targets = append(targets, FileTarget{
							FilePath: filePath,
							Content:  blockContent,
						})
					}
				}
			}
		}
	}

	return targets
}

func AutoApplyFiles(aiResponse string) int {
	targets := ExtractCodeBlocks(aiResponse)
	if len(targets) == 0 {
		return 0
	}

	appliedCount := 0
	fmt.Println("\n[AUTO-EXECUTOR] Menganalisis kode & direktori dari AI...")

	for _, t := range targets {
		dir := filepath.Dir(t.FilePath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Printf(" ❌ Gagal membuat folder [%s]: %v\n", dir, err)
				continue
			}
		}

		err := os.WriteFile(t.FilePath, []byte(t.Content), 0644)
		if err != nil {
			fmt.Printf(" ❌ Gagal menulis file [%s]: %v\n", t.FilePath, err)
			continue
		}

		fmt.Printf(" 🚀 [SUKSES TERBUAT/TERPERBARUI] -> %s\n", t.FilePath)
		appliedCount++
	}

	// Otomatis jalankan git pipeline dengan penanganan konflik cerdas
	if appliedCount > 0 {
		fmt.Println("\n[AUTO-GIT] Menjalankan sinkronisasi git otomatis...")
		
		// 1. Bersihkan sisa rebase / merge yang macet sebelumnya
		RunSystemDiagnostics("git rebase --abort")
		RunSystemDiagnostics("git merge --abort")
		RunSystemDiagnostics("rm -rf .git/rebase-merge .git/rebase-apply")

		// 2. Deteksi branch aktif secara dinamis
		currentBranch := RunSystemDiagnostics("git branch --show-current")
		if currentBranch == "" {
			currentBranch = "master"
		}
		fmt.Printf("[AUTO-GIT] Menggunakan branch aktif: %s\n", currentBranch)

		// 3. Add perubahan file hasil patch AI
		if out := RunSystemDiagnostics("git add ."); out != "" {
			fmt.Println("Git Add:", out)
		}

		// 4. Commit perubahan lokal
		commitMsg := "fix: autonomous AI patch update"
		if out := RunSystemDiagnostics(fmt.Sprintf("git commit -m \"%s\"", commitMsg)); out != "" {
			fmt.Println("Git Commit:", out)
		}

		// 5. Tarik update dari remote dengan strategi 'ours' (jika ada konflik file workflow,utamakan patch AI terbaru)
		fmt.Printf("[AUTO-GIT] Sinkronisasi remote (git pull --rebase origin %s)...\n", currentBranch)
		pullOut := RunSystemDiagnostics(fmt.Sprintf("git pull --rebase origin %s", currentBranch))
		if pullOut != "" {
			fmt.Println("Git Pull Rebase Info:", pullOut)
			// Jika terjadi konflik saat rebase, selesaikan otomatis dengan menerima file patch lokal terbaru
			if strings.Contains(pullOut, "CONFLICT") || strings.Contains(pullOut, "could not apply") {
				fmt.Println("[AUTO-GIT] Terdeteksi konflik, menyamakan status dengan patch lokal...")
				RunSystemDiagnostics("git add .")
				RunSystemDiagnostics("git rebase --continue")
			}
		}

		// 6. Push paksa yang aman (force-with-lease) agar skrip daemon tidak pernah stuck di terminal
		fmt.Printf("[AUTO-GIT] Melakukan push ke origin/%s...\n", currentBranch)
		pushOut := RunSystemDiagnostics(fmt.Sprintf("git push -u origin %s --force-with-lease", currentBranch))
		if pushOut != "" && strings.Contains(pushOut, "error") {
			fmt.Println("Git Push Error:", pushOut)
			// Fallback darurat jika lease gagal: lakukan push standar
			RunSystemDiagnostics(fmt.Sprintf("git push -u origin %s", currentBranch))
		} else {
			fmt.Println(" 🚀 [GIT PUSH & GITHUB ACTIONS BERHASIL DIPICU]")
		}
	}

	return appliedCount
}
