package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/acd19ml/hermes-go/internal/config"
)

const pageSize = 20

// runConfigure runs the interactive configuration wizard that:
//  1. Prompts for Base URL and API Key.
//  2. Tests the connection by fetching the model list.
//  3. Shows LLM models in pages and lets the user pick one.
//  4. Saves the result to ~/.hermes-go/config.json.
//
// Returns 0 on success, 1 on error.
func runConfigure(stdout, stderr io.Writer) int {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "  hermes-go configuration")
	fmt.Fprintln(stdout, "  ════════════════════════")
	fmt.Fprintln(stdout)

	// ── 1. Base URL ───────────────────────────────────────────────────────
	const defaultBase = "https://aiping.cn/api/v1"
	fmt.Fprintf(stdout, "  Base URL [%s]: ", defaultBase)
	baseURL := readLine(reader)
	if baseURL == "" {
		baseURL = defaultBase
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// ── 2. API Key ────────────────────────────────────────────────────────
	fmt.Fprint(stdout, "  API Key: ")
	apiKey := readLine(reader)
	if apiKey == "" {
		fmt.Fprintln(stderr, "error: API key cannot be empty")
		return 1
	}

	// ── 3. Test connection ────────────────────────────────────────────────
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, "  Testing connection...")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	allModels, err := config.FetchModels(ctx, baseURL, apiKey)
	if err != nil {
		fmt.Fprintln(stdout) // newline after "Testing..."
		fmt.Fprintf(stderr, "\nerror: connection failed: %v\n", err)
		return 1
	}
	llms := config.LLMOnly(allModels)
	fmt.Fprintf(stdout, " ✓  %d models available (%d LLM)\n", len(allModels), len(llms))

	if len(llms) == 0 {
		fmt.Fprintln(stderr, "error: no LLM models found at this endpoint")
		return 1
	}

	// ── 4. Model selection ────────────────────────────────────────────────
	selected := selectModel(stdout, stderr, reader, llms)
	if selected == "" {
		fmt.Fprintln(stderr, "no model selected; configuration not saved")
		return 1
	}

	// ── 5. Save ───────────────────────────────────────────────────────────
	cfgPath, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	cfg := config.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   selected,
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "  ✓ Configuration saved to "+cfgPath)
	fmt.Fprintf(stdout, "    Base URL : %s\n", baseURL)
	fmt.Fprintf(stdout, "    Model    : %s\n", selected)
	fmt.Fprintln(stdout)
	return 0
}

// selectModel displays LLM models in pages and returns the chosen model ID.
// Returns "" if the user quits without selecting.
func selectModel(stdout, stderr io.Writer, reader *bufio.Reader, llms []config.ModelInfo) string {
	totalPages := (len(llms) + pageSize - 1) / pageSize
	page := 0

	for {
		start := page * pageSize
		end := start + pageSize
		if end > len(llms) {
			end = len(llms)
		}
		pageModels := llms[start:end]

		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "  LLM Models  (page %d/%d)  — ¥/M tokens\n", page+1, totalPages)
		fmt.Fprintln(stdout, "  "+strings.Repeat("─", 72))
		fmt.Fprintf(stdout, "  %-4s  %-44s  %-7s  %-10s  %-10s\n",
			"#", "Model", "ctx", "¥/M in", "¥/M out")
		fmt.Fprintln(stdout, "  "+strings.Repeat("─", 72))

		for i, m := range pageModels {
			globalIdx := start + i + 1 // 1-based

			ctx := "?"
			if k := m.MaxContextK(); k > 0 {
				ctx = fmt.Sprintf("%dK", k)
			}

			inp := priceStr(m.Price.Input)
			out := priceStr(m.Price.Output)

			fmt.Fprintf(stdout, "  %-4d  %-44s  %-7s  %-10s  %-10s\n",
				globalIdx, m.ID, ctx, inp, out)
		}

		fmt.Fprintln(stdout, "  "+strings.Repeat("─", 72))

		// Navigation hint
		hints := []string{}
		if page > 0 {
			hints = append(hints, "[p] prev")
		}
		if page < totalPages-1 {
			hints = append(hints, "[n] next")
		}
		hints = append(hints, "[q] quit")
		fmt.Fprintf(stdout, "  %s\n", strings.Join(hints, "  "))
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, "  Select model number: ")

		line := strings.TrimSpace(readLine(reader))
		switch strings.ToLower(line) {
		case "n":
			if page < totalPages-1 {
				page++
			}
			continue
		case "p":
			if page > 0 {
				page--
			}
			continue
		case "q", "":
			return ""
		}

		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(llms) {
			fmt.Fprintf(stderr, "  invalid selection %q — enter a number between 1 and %d\n", line, len(llms))
			continue
		}
		return llms[n-1].ID
	}
}

// priceStr formats a price range slice as "min-max" or "0.00" when both ends
// are zero (free / unknown).
func priceStr(r []float64) string {
	if len(r) < 2 {
		return "?"
	}
	if r[0] == 0 && r[1] == 0 {
		return "0.00"
	}
	if r[0] == r[1] {
		return fmt.Sprintf("%.2f", r[0])
	}
	return fmt.Sprintf("%.2f-%.2f", r[0], r[1])
}

// readLine reads one line from reader, trimming the trailing newline.
func readLine(r *bufio.Reader) string {
	line, _ := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}
