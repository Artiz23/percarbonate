package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Config struct {
	Addr     string
	BotToken string
	ChatID   string
}

type Lead struct {
	Name        string `json:"name"`
	Phone       string `json:"phone"`
	Platforms   string `json:"platforms"`
	Category    string `json:"category"`
	City        string `json:"city"`
	DailyVolume string `json:"dailyVolume"`
}

type apiResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

var (
	phoneDigits = regexp.MustCompile(`\D+`)
	phpToken    = regexp.MustCompile(`['"]bot_token['"]\s*=>\s*['"]([^'"]*)['"]`)
	phpChat     = regexp.MustCompile(`['"]chat_id['"]\s*=>\s*['"]([^'"]*)['"]`)
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/lead", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Error: "method"})
			return
		}
		handleLead(w, r, cfg)
	})
	mux.HandleFunc("/", servePublic)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("сайт: http://localhost%s", cfg.Addr)
	if cfg.BotToken == "" || cfg.ChatID == "" {
		log.Print("токен или chat_id пустые — заявки не уйдут. Заполните api/config.php")
	}
	log.Fatal(server.ListenAndServe())
}

func loadConfig() (Config, error) {
	cfg := Config{Addr: ":8080"}
	if v := strings.TrimSpace(os.Getenv("ADDR")); v != "" {
		cfg.Addr = v
	}
	if !strings.HasPrefix(cfg.Addr, ":") && !strings.Contains(cfg.Addr, ":") {
		cfg.Addr = ":" + cfg.Addr
	}

	raw, err := os.ReadFile("api/config.php")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if err == nil {
		if m := phpToken.FindSubmatch(raw); len(m) == 2 {
			cfg.BotToken = strings.TrimSpace(string(m[1]))
		}
		if m := phpChat.FindSubmatch(raw); len(m) == 2 {
			cfg.ChatID = strings.TrimSpace(string(m[1]))
		}
	}

	if v := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); v != "" {
		cfg.BotToken = v
	}
	if v := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")); v != "" {
		cfg.ChatID = v
	}
	return cfg, nil
}

func handleLead(w http.ResponseWriter, r *http.Request, cfg Config) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)

	if cfg.BotToken == "" || cfg.ChatID == "" {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Error: "not_configured"})
		return
	}

	var lead Lead
	if err := json.NewDecoder(r.Body).Decode(&lead); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{Error: "json"})
		return
	}

	lead.Name = clip(strings.TrimSpace(lead.Name), 80)
	lead.Phone = clip(strings.TrimSpace(lead.Phone), 32)
	lead.Platforms = clip(strings.TrimSpace(lead.Platforms), 80)
	lead.Category = clip(strings.TrimSpace(lead.Category), 80)
	lead.City = clip(strings.TrimSpace(lead.City), 80)
	lead.DailyVolume = clip(strings.TrimSpace(lead.DailyVolume), 80)

	digits := phoneDigits.ReplaceAllString(lead.Phone, "")
	if lead.Name == "" || lead.City == "" || len(digits) != 11 || !strings.HasPrefix(digits, "7") {
		writeJSON(w, http.StatusBadRequest, apiResponse{Error: "validation"})
		return
	}
	if lead.Platforms == "" {
		lead.Platforms = "не указано"
	}
	if lead.Category == "" {
		lead.Category = "не указано"
	}
	if lead.DailyVolume == "" {
		lead.DailyVolume = "не указано"
	}

	number, err := nextLeadNumber("api/lead-number.txt")
	if err != nil {
		log.Printf("counter: %v", err)
		number = 1
	}

	if err := sendTelegram(cfg, formatLead(lead, digits, number)); err != nil {
		log.Printf("telegram: %v", err)
		writeJSON(w, http.StatusBadGateway, apiResponse{Error: "telegram"})
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{OK: true})
}

func formatLead(lead Lead, digits string, number int) string {
	pretty := formatPrettyPhone(digits)

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.Local
	}
	when := time.Now().In(loc).Format("02.01.2006 15:04")

	return "👤 <b>" + html.EscapeString(lead.Name) + "</b>\n" +
		"📞 +" + html.EscapeString(digits) + "\n" +
		`<a href="tel:+` + html.EscapeString(digits) + `">` + html.EscapeString(pretty) + "</a>\n" +
		"📦 Объём в день: " + html.EscapeString(lead.DailyVolume) + "\n\n" +
		"<b>Ответы теста</b>\n" +
		"• На каких площадках вы торгуете\n— <b>" + html.EscapeString(lead.Platforms) + "</b>\n" +
		"• Какая у вас категория товара\n— <b>" + html.EscapeString(lead.Category) + "</b>\n" +
		"• Из какого города отгружаете товар\n— <b>" + html.EscapeString(lead.City) + "</b>\n" +
		"• Какой объём заказов в день\n— <b>" + html.EscapeString(lead.DailyVolume) + "</b>\n\n" +
		"🕐 " + html.EscapeString(when) + " · заявка №" + strconv.Itoa(number)
}

func formatPrettyPhone(digits string) string {
	if len(digits) != 11 {
		return "+" + digits
	}
	return fmt.Sprintf("+7 (%s) %s-%s-%s", digits[1:4], digits[4:7], digits[7:9], digits[9:11])
}

func nextLeadNumber(path string) (int, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return 0, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	n++
	if err := f.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	if _, err := fmt.Fprintf(f, "%d", n); err != nil {
		return 0, err
	}
	return n, nil
}

func sendTelegram(cfg Config, text string) error {
	endpoint := "https://api.telegram.org/bot" + cfg.BotToken + "/sendMessage"
	form := url.Values{}
	form.Set("chat_id", cfg.ChatID)
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var tg struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &tg); err != nil {
		return err
	}
	if resp.StatusCode >= 400 || !tg.OK {
		if tg.Description == "" {
			tg.Description = resp.Status
		}
		return errors.New(tg.Description)
	}
	return nil
}

func servePublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	if path == "/" {
		http.ServeFile(w, r, "index.html")
		return
	}

	clean := strings.TrimPrefix(path, "/")
	if !isPublicPath(clean) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, clean)
}

func isPublicPath(path string) bool {
	if strings.Contains(path, "..") || strings.ContainsAny(path, `\\`) {
		return false
	}
	switch {
	case path == "index.html":
		return true
	case strings.HasPrefix(path, "css/"):
		return true
	case strings.HasPrefix(path, "js/"):
		return true
	case strings.HasPrefix(path, "img/"):
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, payload apiResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func clip(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b bytes.Buffer
	for _, r := range s {
		if utf8.RuneCount(b.Bytes()) >= max {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
