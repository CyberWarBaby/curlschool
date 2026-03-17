package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ═══════════════════════════════════════════════════════════════════
//  CHALLENGES
// ═══════════════════════════════════════════════════════════════════

type Challenge struct {
	ID     int    `json:"id"`
	Key    string `json:"-"`
	Tier   string `json:"tier"`
	Points int    `json:"points"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Task   string `json:"task"`
	Flag   string `json:"-"`
}

var challenges = []Challenge{
	{1, "ping", "easy", 10, "GET", "/ping", "Wake the server up", "FLAG{alive_and_kicking}"},
	{2, "get_all", "easy", 10, "GET", "/notes", "List every note (token required)", "FLAG{curl_gets_it_all}"},
	{3, "get_one", "easy", 10, "GET", "/notes/1", "Read a single note by ID (token required)", "FLAG{one_note_at_a_time}"},
	{4, "post_note", "easy", 10, "POST", "/notes", "Create a new note with a JSON body (token required)", "FLAG{you_just_posted_like_a_pro}"},
	{5, "delete", "easy", 10, "DELETE", "/notes/2", "Delete note 2 (token required)", "FLAG{gone_forever_rip_note}"},
	{6, "query", "easy", 10, "GET", "/echo", "Send a query parameter: ?student=YOUR_NAME", "FLAG{query_params_are_key_value}"},
	{7, "custom_hdr", "easy", 10, "GET", "/echo", "Send a custom header: X-Student: YOUR_NAME", "FLAG{custom_headers_unlocked}"},
	{8, "useragent", "easy", 10, "GET", "/echo", "Change your User-Agent to: CurlMaster/1.0", "FLAG{curl_is_a_valid_browser}"},
	{9, "put", "easy", 10, "PUT", "/notes/1", "Fully replace note 1 — send both title and body (token required)", "FLAG{full_replace_no_mercy}"},
	{10, "patch", "easy", 10, "PATCH", "/notes/1", "Update only the title of note 1 (token required)", "FLAG{surgical_precision_patch}"},
	{11, "auth_echo", "mid", 25, "GET", "/echo", "Send your Bearer token in the Authorization header to /echo", "FLAG{bearer_tokens_unlock_doors}"},
	{12, "post_multi", "mid", 25, "POST", "/notes", "Create 3 notes total (POST /notes three separate times)", "FLAG{mass_production_unlocked}"},
	{13, "verbose", "mid", 25, "GET", "/notes", "Hit GET /notes with header X-Verbose: true", "FLAG{verbose_mode_engaged}"},
	{14, "accept", "mid", 25, "GET", "/notes", "Hit GET /notes with Accept: application/json header", "FLAG{content_negotiation_basics}"},
	{15, "timing", "mid", 25, "GET", "/slow", "Hit GET /slow and use curl -w to print total time taken", "FLAG{patience_is_a_virtue}"},
	{16, "post_noct", "mid", 25, "POST", "/notes", "POST /notes without a Content-Type header — observe what breaks", "FLAG{content_type_matters}"},
	{17, "patch_body", "hard", 50, "PATCH", "/notes/1", "PATCH note 1 updating both title and body in one request", "FLAG{double_patch_power}"},
	{18, "head", "hard", 50, "HEAD", "/notes", "Send a HEAD request to /notes — check the response headers", "FLAG{head_not_get_big_brain}"},
	{19, "multi_hdr", "hard", 50, "GET", "/echo", "Hit /echo with X-Student, Authorization AND User-Agent all set", "FLAG{triple_header_threat}"},
	{20, "leaderboard", "hard", 50, "GET", "/leaderboard", "Call GET /leaderboard and find your name on it", "FLAG{i_can_see_myself_winning}"},
}

func challengeByKey(key string) *Challenge {
	for i := range challenges {
		if challenges[i].Key == key {
			return &challenges[i]
		}
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════════
//  SSE hub — pushes leaderboard JSON to React frontend
// ═══════════════════════════════════════════════════════════════════

type sseHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

var hub = &sseHub{clients: map[chan string]struct{}{}}

func (h *sseHub) subscribe() chan string {
	ch := make(chan string, 4)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *sseHub) unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *sseHub) broadcast(data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

func broadcastLeaderboard() {
	type Entry struct {
		Rank     int    `json:"rank"`
		Username string `json:"username"`
		Points   int    `json:"points"`
		Solved   int    `json:"solved"`
		Medal    string `json:"medal"`
	}
	rows, err := db.Query(`
		SELECT u.username, u.points,
		       (SELECT COUNT(*) FROM solved s WHERE s.username=u.username) as solved
		FROM users u ORDER BY u.points DESC, solved DESC, u.created_at ASC LIMIT 50`)
	if err != nil {
		return
	}
	defer rows.Close()
	var board []Entry
	rank := 1
	for rows.Next() {
		var e Entry
		rows.Scan(&e.Username, &e.Points, &e.Solved)
		e.Rank = rank
		switch rank {
		case 1:
			e.Medal = "🥇"
		case 2:
			e.Medal = "🥈"
		case 3:
			e.Medal = "🥉"
		default:
			e.Medal = fmt.Sprintf("#%d", rank)
		}
		board = append(board, e)
		rank++
	}
	if board == nil {
		board = []Entry{}
	}
	data, _ := json.Marshal(board)
	hub.broadcast(string(data))
}

// ═══════════════════════════════════════════════════════════════════
//  DB
// ═══════════════════════════════════════════════════════════════════

var db *sql.DB

func initDB(path string) {
	var err error
	db, err = sql.Open("sqlite3", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			username   TEXT    UNIQUE NOT NULL,
			token      TEXT    UNIQUE NOT NULL,
			points     INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS notes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			title      TEXT NOT NULL,
			body       TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS solved (
			username      TEXT NOT NULL,
			challenge_key TEXT NOT NULL,
			solved_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (username, challenge_key)
		);
	`)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func lookupToken(token string) string {
	var u string
	db.QueryRow(`SELECT username FROM users WHERE token=?`, token).Scan(&u)
	return u
}

func bearerToken(r *http.Request) string {
	p := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(p) == 2 && strings.EqualFold(p[0], "bearer") {
		return p[1]
	}
	return ""
}

func hasSolved(username, key string) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM solved WHERE username=? AND challenge_key=?`, username, key).Scan(&n)
	return n > 0
}

func awardFlag(username, key string) (string, int, bool) {
	c := challengeByKey(key)
	if c == nil {
		return "", 0, false
	}
	if hasSolved(username, key) {
		return c.Flag, c.Points, true
	}
	db.Exec(`INSERT OR IGNORE INTO solved (username, challenge_key) VALUES (?,?)`, username, key)
	db.Exec(`UPDATE users SET points=points+? WHERE username=?`, c.Points, username)
	go broadcastLeaderboard()
	return c.Flag, c.Points, false
}

func userPoints(username string) int {
	var p int
	db.QueryRow(`SELECT points FROM users WHERE username=?`, username).Scan(&p)
	return p
}

func solvedCount(username string) int {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM solved WHERE username=?`, username).Scan(&n)
	return n
}

// ═══════════════════════════════════════════════════════════════════
//  NOTES
// ═══════════════════════════════════════════════════════════════════

type Note struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

var mu sync.Mutex

func seedNotes() {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&n)
	if n > 0 {
		return
	}
	db.Exec(`INSERT INTO notes (title, body) VALUES
		('Welcome to CurlSchool!', 'Try GET, PUT, and PATCH on me!'),
		('Delete me!',             'Seriously. DELETE /notes/2. Do it.'),
		('Just vibing',            'Note 3. Nothing to see here.')`)
	log.Println("🌱 seeded 3 starter notes")
}

func getAllNotes() []Note {
	rows, _ := db.Query(`SELECT id, title, body, created_at FROM notes ORDER BY id`)
	if rows == nil {
		return []Note{}
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		var n Note
		rows.Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt)
		out = append(out, n)
	}
	if out == nil {
		return []Note{}
	}
	return out
}

func getNoteByID(id int) (*Note, bool) {
	var n Note
	err := db.QueryRow(`SELECT id, title, body, created_at FROM notes WHERE id=?`, id).
		Scan(&n.ID, &n.Title, &n.Body, &n.CreatedAt)
	return &n, err == nil
}

// ═══════════════════════════════════════════════════════════════════
//  RESPONSE HELPERS
// ═══════════════════════════════════════════════════════════════════

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

var hypeEasy = []string{
	"LET'S GO %s!! 🔥 +%d points!",
	"Ayy %s, easy money! +%d pts 🎉",
	"Look at you %s, curling already! +%d 🚀",
	"Clean work %s. +%d pts, keep going! ✅",
	"Boom %s! +%d pts 💥",
}
var hypeMid = []string{
	"OKAY %s! Mid cleared! +%d pts — leaderboard is shaking 📈",
	"Getting serious now %s! +%d pts 🧠",
	"%s not playing. +%d pts 💪",
	"Mid-tier down %s. +%d pts. You're cooking 🔥",
}
var hypeHard = []string{
	"SHEESH %s!! HARD CLEARED! +%d PTS! BUILT DIFFERENT 🏆",
	"%s just went full DevOps mode. +%d pts. Leaderboard FEARS you 👑",
	"BIG BRAIN %s. +%d pts. Someone call the instructor 😂",
	"%s eating the hard challenges like snacks. +%d pts 🍖",
}

func congrats(username, tier string, pts int) string {
	idx := int(time.Now().UnixNano() % 1000)
	switch tier {
	case "easy":
		return fmt.Sprintf(hypeEasy[idx%len(hypeEasy)], username, pts)
	case "mid":
		return fmt.Sprintf(hypeMid[idx%len(hypeMid)], username, pts)
	case "hard":
		return fmt.Sprintf(hypeHard[idx%len(hypeHard)], username, pts)
	}
	return fmt.Sprintf("Nice %s! +%d", username, pts)
}

func flagResp(username, key string, extra map[string]any) map[string]any {
	flag, pts, already := awardFlag(username, key)
	c := challengeByKey(key)
	res := map[string]any{
		"flag":            flag,
		"points_earned":   pts,
		"your_total":      userPoints(username),
		"challenges_done": fmt.Sprintf("%d/20", solvedCount(username)),
	}
	if already {
		res["note"] = "Already solved — no extra points."
	} else {
		res["congrats"] = congrats(username, c.Tier, pts)
	}
	for k, v := range extra {
		res[k] = v
	}
	return res
}

func requireAuth(w http.ResponseWriter, r *http.Request) string {
	token := bearerToken(r)
	if token == "" {
		writeJSON(w, 401, map[string]any{
			"error": "Token required.",
			"hint":  "POST /login first, then add: -H \"Authorization: Bearer YOUR_TOKEN\"",
		})
		return ""
	}
	username := lookupToken(token)
	if username == "" {
		writeJSON(w, 401, map[string]any{
			"error": "Token not recognized.",
			"hint":  "Check you copied it correctly, or POST /login again.",
		})
		return ""
	}
	return username
}

// ═══════════════════════════════════════════════════════════════════
//  GET /
// ═══════════════════════════════════════════════════════════════════

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeJSON(w, 404, map[string]any{
			"error": fmt.Sprintf("No route: %s %s", r.Method, r.URL.Path),
		})
		return
	}
	type CI struct {
		ID     int    `json:"id"`
		Tier   string `json:"tier"`
		Points int    `json:"points"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Task   string `json:"task"`
	}
	var easy, mid, hard []CI
	for _, c := range challenges {
		ci := CI{c.ID, c.Tier, c.Points, c.Method, c.Path, c.Task}
		switch c.Tier {
		case "easy":
			easy = append(easy, ci)
		case "mid":
			mid = append(mid, ci)
		case "hard":
			hard = append(hard, ci)
		}
	}
	writeJSON(w, 200, map[string]any{
		"welcome":       "🚩 CurlSchool — 20 challenges, 600pts max",
		"start":         "POST /login with your name to get a token",
		"scoring":       map[string]any{"easy": "10pts", "mid": "25pts", "hard": "50pts"},
		"easy_1_to_10":  easy,
		"mid_11_to_16":  mid,
		"hard_17_to_20": hard,
	})
}

// ═══════════════════════════════════════════════════════════════════
//  POST /login
// ═══════════════════════════════════════════════════════════════════

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{
			"error": fmt.Sprintf("Method %s not allowed.", r.Method),
		})
		return
	}

	ct := r.Header.Get("Content-Type")
	raw, _ := io.ReadAll(r.Body)

	if !strings.Contains(ct, "application/json") {
		writeJSON(w, 400, map[string]any{
			"error": "Expected Content-Type: application/json.",
			"got":   ct,
		})
		return
	}

	var body struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, 400, map[string]any{
			"error":   "Could not parse JSON body.",
			"details": err.Error(),
		})
		return
	}

	username := strings.TrimSpace(body.Username)
	if username == "" {
		writeJSON(w, 400, map[string]any{"error": "Field 'username' is required."})
		return
	}

	var existing string
	if err := db.QueryRow(`SELECT token FROM users WHERE username=?`, username).Scan(&existing); err == nil {
		writeJSON(w, 200, map[string]any{
			"message":         fmt.Sprintf("👋 Welcome back %s!", username),
			"token":           existing,
			"points":          userPoints(username),
			"challenges_done": fmt.Sprintf("%d/20", solvedCount(username)),
		})
		return
	}

	token := generateToken()
	if _, err := db.Exec(`INSERT INTO users (username, token) VALUES (?,?)`, username, token); err != nil {
		writeJSON(w, 500, map[string]any{"error": "DB error."})
		return
	}

	writeJSON(w, 201, map[string]any{
		"message":  fmt.Sprintf("🎉 Welcome %s! Save your token.", username),
		"username": username,
		"token":    token,
		"warning":  "Don't lose this. POST /login again with same username if you do.",
	})
}

// ═══════════════════════════════════════════════════════════════════
//  GET /ping — Challenge 1
// ═══════════════════════════════════════════════════════════════════

func handlePing(w http.ResponseWriter, r *http.Request) {
	res := map[string]any{
		"status":  "🟢 alive",
		"time":    time.Now().Format(time.RFC3339),
		"message": "pong!",
	}
	if username := lookupToken(bearerToken(r)); username != "" {
		for k, v := range flagResp(username, "ping", nil) {
			res[k] = v
		}
	}
	writeJSON(w, 200, res)
}

// ═══════════════════════════════════════════════════════════════════
//  GET /slow — Challenge 15
// ═══════════════════════════════════════════════════════════════════

func handleSlow(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	time.Sleep(2 * time.Second)
	writeJSON(w, 200, flagResp(username, "timing", map[string]any{"took": "~2s"}))
}

// ═══════════════════════════════════════════════════════════════════
//  GET /echo — Challenges 6, 7, 8, 11, 19
// ═══════════════════════════════════════════════════════════════════

func handleEcho(w http.ResponseWriter, r *http.Request) {
	headers := map[string]string{}
	for k, v := range r.Header {
		headers[k] = strings.Join(v, ", ")
	}
	params := map[string]string{}
	for k, v := range r.URL.Query() {
		params[k] = strings.Join(v, ", ")
	}

	token := bearerToken(r)
	username := lookupToken(token)

	res := map[string]any{
		"mirror": map[string]any{
			"method":  r.Method,
			"params":  params,
			"headers": headers,
		},
	}

	studentParam := r.URL.Query().Get("student")
	xStudent := r.Header.Get("X-Student")
	isCurlMaster := strings.Contains(r.Header.Get("User-Agent"), "CurlMaster")

	if studentParam != "" && username != "" {
		res["flag_6"] = flagResp(username, "query", nil)
	}
	if xStudent != "" && username != "" {
		res["flag_7"] = flagResp(username, "custom_hdr", nil)
	}
	if isCurlMaster && username != "" {
		res["flag_8"] = flagResp(username, "useragent", nil)
	}
	if token != "" && username != "" {
		res["flag_11"] = flagResp(username, "auth_echo", nil)
	}
	// Challenge 19: all three simultaneously
	if xStudent != "" && token != "" && isCurlMaster && username != "" {
		res["flag_19"] = flagResp(username, "multi_hdr", nil)
	}

	writeJSON(w, 200, res)
}

// ═══════════════════════════════════════════════════════════════════
//  GET /leaderboard — Challenge 20  +  SSE stream
// ═══════════════════════════════════════════════════════════════════

func handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	type Entry struct {
		Rank     int    `json:"rank"`
		Username string `json:"username"`
		Points   int    `json:"points"`
		Solved   int    `json:"solved"`
		Medal    string `json:"medal"`
	}
	rows, _ := db.Query(`
		SELECT u.username, u.points,
		       (SELECT COUNT(*) FROM solved s WHERE s.username=u.username) as solved
		FROM users u ORDER BY u.points DESC, solved DESC, u.created_at ASC LIMIT 50`)
	var board []Entry
	rank := 1
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var e Entry
			rows.Scan(&e.Username, &e.Points, &e.Solved)
			e.Rank = rank
			switch rank {
			case 1:
				e.Medal = "🥇"
			case 2:
				e.Medal = "🥈"
			case 3:
				e.Medal = "🥉"
			default:
				e.Medal = fmt.Sprintf("#%d", rank)
			}
			board = append(board, e)
			rank++
		}
	}
	if board == nil {
		board = []Entry{}
	}

	res := map[string]any{
		"leaderboard": board,
		"total":       rank - 1,
		"max_score":   600,
	}
	if username := lookupToken(bearerToken(r)); username != "" {
		res["your_flag"] = flagResp(username, "leaderboard", nil)
	}
	writeJSON(w, 200, res)
}

func handleLeaderboardStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}

	ch := hub.subscribe()
	defer hub.unsubscribe(ch)

	go broadcastLeaderboard() // push current state on connect

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ═══════════════════════════════════════════════════════════════════
//  GET /me
// ═══════════════════════════════════════════════════════════════════

func handleMe(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	rows, _ := db.Query(`SELECT challenge_key, solved_at FROM solved WHERE username=? ORDER BY solved_at`, username)
	type S struct {
		Key string `json:"challenge"`
		At  string `json:"solved_at"`
	}
	var solved []S
	solvedMap := map[string]bool{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var s S
			rows.Scan(&s.Key, &s.At)
			solved = append(solved, s)
			solvedMap[s.Key] = true
		}
	}
	var remaining []string
	for _, c := range challenges {
		if !solvedMap[c.Key] {
			remaining = append(remaining, fmt.Sprintf("[%d] %s %s — %s (%dpts)", c.ID, c.Method, c.Path, c.Task, c.Points))
		}
	}
	writeJSON(w, 200, map[string]any{
		"username":        username,
		"points":          userPoints(username),
		"challenges_done": fmt.Sprintf("%d/20", len(solved)),
		"solved":          solved,
		"remaining":       remaining,
	})
}

// ═══════════════════════════════════════════════════════════════════
//  /notes collection — Challenges 2, 4, 12, 13, 14, 16, 18
// ═══════════════════════════════════════════════════════════════════

func handleNotes(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}

	switch r.Method {

	case http.MethodGet:
		mu.Lock()
		all := getAllNotes()
		mu.Unlock()
		res := flagResp(username, "get_all", map[string]any{"count": len(all), "notes": all})
		if r.Header.Get("X-Verbose") == "true" {
			res["flag_13"] = flagResp(username, "verbose", map[string]any{
				"server_info": map[string]any{"db": "sqlite3", "challenges": 20},
			})
		}
		if r.Header.Get("Accept") == "application/json" {
			res["flag_14"] = flagResp(username, "accept", nil)
		}
		writeJSON(w, 200, res)

	case http.MethodPost:
		ct := r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)

		// Challenge 16: POST without correct Content-Type — award flag and explain
		if !strings.Contains(ct, "application/json") {
			res := map[string]any{
				"error":    "Content-Type header missing or incorrect.",
				"received": ct,
				"flag_16":  flagResp(username, "post_noct", nil),
			}
			writeJSON(w, 400, res)
			return
		}

		var body struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			writeJSON(w, 400, map[string]any{"error": "JSON parse error.", "details": err.Error()})
			return
		}
		if strings.TrimSpace(body.Title) == "" {
			writeJSON(w, 400, map[string]any{"error": "Field 'title' is required."})
			return
		}

		mu.Lock()
		result, err := db.Exec(`INSERT INTO notes (title, body) VALUES (?,?)`, body.Title, body.Body)
		mu.Unlock()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": "DB error."})
			return
		}
		id, _ := result.LastInsertId()
		n, _ := getNoteByID(int(id))
		res := flagResp(username, "post_note", map[string]any{"note": n})

		var userPosts int
		db.QueryRow(`SELECT COUNT(*) FROM notes WHERE id > 3`).Scan(&userPosts)
		if userPosts >= 3 {
			res["flag_12"] = flagResp(username, "post_multi", nil)
		} else {
			res["progress_12"] = fmt.Sprintf("%d/3 notes created", userPosts)
		}
		writeJSON(w, 201, res)

	case http.MethodHead:
		// Challenge 18 — HEAD: respond with headers, no body
		flag, pts, already := awardFlag(username, "head")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Flag", flag)
		w.Header().Set("X-Points", fmt.Sprintf("%d", pts))
		if already {
			w.Header().Set("X-Already-Solved", "true")
		} else {
			w.Header().Set("X-Congrats", congrats(username, "hard", pts))
		}
		w.WriteHeader(200)

	default:
		writeJSON(w, 405, map[string]any{
			"error":     fmt.Sprintf("Method %s not allowed.", r.Method),
			"supported": []string{"GET", "POST", "HEAD"},
		})
	}
}

// ═══════════════════════════════════════════════════════════════════
//  /notes/:id — Challenges 3, 5, 9, 10, 17
// ═══════════════════════════════════════════════════════════════════

func handleNote(w http.ResponseWriter, r *http.Request, id int) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	mu.Lock()
	note, ok := getNoteByID(id)
	mu.Unlock()
	if !ok {
		writeJSON(w, 404, map[string]any{
			"error": fmt.Sprintf("Note %d not found.", id),
			"hint":  "GET /notes to see available IDs.",
		})
		return
	}

	switch r.Method {

	case http.MethodGet:
		writeJSON(w, 200, flagResp(username, "get_one", map[string]any{"note": note}))

	case http.MethodPut:
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 400, map[string]any{"error": "Expected Content-Type: application/json."})
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.Unmarshal(raw, &body); err != nil || strings.TrimSpace(body.Title) == "" {
			writeJSON(w, 400, map[string]any{"error": "PUT requires both 'title' and 'body'."})
			return
		}
		mu.Lock()
		db.Exec(`UPDATE notes SET title=?, body=? WHERE id=?`, body.Title, body.Body, id)
		updated, _ := getNoteByID(id)
		mu.Unlock()
		writeJSON(w, 200, flagResp(username, "put", map[string]any{"note": updated}))

	case http.MethodPatch:
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			writeJSON(w, 400, map[string]any{"error": "Expected Content-Type: application/json."})
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Title *string `json:"title"`
			Body  *string `json:"body"`
		}
		if err := json.Unmarshal(raw, &body); err != nil || (body.Title == nil && body.Body == nil) {
			writeJSON(w, 400, map[string]any{"error": "PATCH requires at least one of: 'title', 'body'."})
			return
		}
		mu.Lock()
		if body.Title != nil {
			db.Exec(`UPDATE notes SET title=? WHERE id=?`, *body.Title, id)
		}
		if body.Body != nil {
			db.Exec(`UPDATE notes SET body=? WHERE id=?`, *body.Body, id)
		}
		updated, _ := getNoteByID(id)
		mu.Unlock()
		res := flagResp(username, "patch", map[string]any{"note": updated})
		if body.Title != nil && body.Body != nil {
			res["flag_17"] = flagResp(username, "patch_body", nil)
		}
		writeJSON(w, 200, res)

	case http.MethodDelete:
		mu.Lock()
		db.Exec(`DELETE FROM notes WHERE id=?`, id)
		mu.Unlock()
		writeJSON(w, 200, flagResp(username, "delete", map[string]any{"deleted": note}))

	default:
		writeJSON(w, 405, map[string]any{
			"error":     fmt.Sprintf("Method %s not allowed.", r.Method),
			"supported": []string{"GET", "PUT", "PATCH", "DELETE"},
		})
	}
}

func notesRouter(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/notes" || r.URL.Path == "/notes/" {
		handleNotes(w, r)
		return
	}
	var id int
	if _, err := fmt.Sscanf(r.URL.Path, "/notes/%d", &id); err != nil {
		writeJSON(w, 400, map[string]any{"error": "Note ID must be a number. Example: /notes/1"})
		return
	}
	handleNote(w, r, id)
}

func logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("▶  %-7s %s", r.Method, r.URL.Path)
		next(w, r)
		log.Printf("   ↳ %v", time.Since(start))
	}
}

// ═══════════════════════════════════════════════════════════════════
//  MAIN
// ═══════════════════════════════════════════════════════════════════

func main() {
	port := flag.String("port", "8080", "port to listen on")
	host := flag.String("host", "0.0.0.0", "host/interface to bind")
	dbPath := flag.String("db", "/data/curlschool.db", "SQLite database path")
	seed := flag.Bool("seed", true, "seed starter notes on first run")
	verbose := flag.Bool("verbose", true, "log every request")
	flag.Parse()

	initDB(*dbPath)
	if *seed {
		seedNotes()
	}

	mux := http.NewServeMux()
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		if *verbose {
			return logger(h)
		}
		return h
	}

	mux.HandleFunc("/", wrap(handleRoot))
	mux.HandleFunc("/ping", wrap(handlePing))
	mux.HandleFunc("/login", wrap(handleLogin))
	mux.HandleFunc("/echo", wrap(handleEcho))
	mux.HandleFunc("/slow", wrap(handleSlow))
	mux.HandleFunc("/leaderboard", wrap(handleLeaderboard))
	mux.HandleFunc("/leaderboard/stream", handleLeaderboardStream)
	mux.HandleFunc("/me", wrap(handleMe))
	mux.HandleFunc("/notes", wrap(notesRouter))
	mux.HandleFunc("/notes/", wrap(notesRouter))

	addr := fmt.Sprintf("%s:%s", *host, *port)
	fmt.Printf("\n  🚩  CurlSchool — 20 Challenges · 600pts max\n")
	fmt.Printf("  Listening:  http://%s\n", addr)
	fmt.Printf("  DB:         %s\n\n", *dbPath)
	log.Fatal(http.ListenAndServe(addr, mux))
}
