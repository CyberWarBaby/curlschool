package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ═══════════════════════════════════════════════════════════════════
//  CHALLENGES  (original 20 + new 20)
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
	// ── original 20 ─────────────────────────────────────────────
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
	// ── new 20 ──────────────────────────────────────────────────
	{21, "basic_auth", "mid", 25, "GET", "/basic-login", "Use HTTP Basic Auth (-u username:password) to authenticate", "FLAG{basic_auth_old_but_gold}"},
	{22, "wrong_method", "mid", 25, "POST", "/ping", "Trigger a 405 on /ping then fix it with the correct method", "FLAG{method_matters_always}"},
	{23, "redirect", "mid", 25, "GET", "/moved", "Follow the redirect with -L and land on the real page", "FLAG{follow_the_leader}"},
	{24, "resp_header", "mid", 25, "GET", "/secret-header", "Use -i and read the X-Secret-Token response header value", "FLAG{headers_hide_secrets}"},
	{25, "form_submit", "mid", 25, "POST", "/form", "Submit URL-encoded form data: name=YOUR_NAME&course=curl101", "FLAG{forms_are_just_strings}"},
	{26, "query_and_hdr", "mid", 25, "GET", "/combo", "Send both ?mode=strict AND X-Api-Version: 2 together", "FLAG{query_plus_header_combo}"},
	{27, "two_hdrs", "mid", 25, "GET", "/double-check", "Send both X-First: alpha AND X-Second: beta headers", "FLAG{two_headers_one_request}"},
	{28, "json_validation", "mid", 25, "POST", "/validated", "POST JSON with title (string), author (string), year (number)", "FLAG{schema_validation_passed}"},
	{29, "trigger_401", "mid", 25, "GET", "/notes", "Hit /notes without a token (get 401), then fix it", "FLAG{401_then_fixed}"},
	{30, "trigger_400", "mid", 25, "POST", "/notes", "POST /notes with missing title (get 400), then fix it", "FLAG{400_then_fixed}"},
	{31, "cookie_save", "mid", 25, "POST", "/cookie-login", "Login via /cookie-login and save the cookie with -c cookies.txt", "FLAG{cookies_saved_to_jar}"},
	{32, "cookie_reuse", "mid", 25, "GET", "/cookie-protected", "Send the saved cookie to /cookie-protected with -b cookies.txt", "FLAG{session_cookie_rides_again}"},
	{33, "useragent_check", "mid", 25, "GET", "/agent-check", "Send a User-Agent that starts with Mozilla to /agent-check", "FLAG{fake_it_till_you_make_it}"},
	{34, "accept_json", "mid", 25, "GET", "/content-deal", "Hit /content-deal with Accept: application/json to get JSON back", "FLAG{accept_json_get_json}"},
	{35, "put_idempotent", "mid", 25, "PUT", "/notes/1", "PUT the same payload to /notes/1 twice — prove idempotency", "FLAG{idempotency_means_same_result}"},
	{36, "patch_vs_put", "mid", 25, "PATCH", "/notes/1", "PATCH only the body field of note 1 — compare with PUT", "FLAG{patch_is_surgical_put_is_nuclear}"},
	{37, "head_vs_get", "mid", 25, "HEAD", "/notes/1", "HEAD /notes/1 — headers only, no body (compare with GET)", "FLAG{head_saves_bandwidth}"},
	{38, "pagination", "mid", 25, "GET", "/notes", "Hit GET /notes?page=1&limit=2 for paginated results", "FLAG{pagination_keeps_it_clean}"},
	{39, "sorting", "mid", 25, "GET", "/notes", "Hit GET /notes?sort=desc for descending order", "FLAG{sorted_like_a_pro}"},
	{40, "mini_boss", "hard", 50, "GET", "/boss", "One shot: Bearer token + ?level=hard + X-Boss: true + Accept: application/json", "FLAG{boss_defeated_curl_master}"},
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
//  SSE hub
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
	// Tie-break: same points → whoever reached that score FIRST ranks higher
	rows, err := db.Query(`
		SELECT u.username, u.points,
		       (SELECT COUNT(*) FROM solved s WHERE s.username=u.username) as solved
		FROM users u
		ORDER BY u.points DESC, u.last_points_at ASC, u.created_at ASC
		LIMIT 50`)
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

func hashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
}

func initDB(path string) {
	var err error
	db, err = sql.Open("sqlite3", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			username       TEXT    UNIQUE NOT NULL,
			password_hash  TEXT    NOT NULL DEFAULT '',
			token          TEXT    UNIQUE NOT NULL,
			session_token  TEXT    UNIQUE,
			points         INTEGER DEFAULT 0,
			last_points_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
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
	// Safe migrations for existing DBs that lack the new columns
	db.Exec(`ALTER TABLE users ADD COLUMN password_hash  TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE users ADD COLUMN session_token  TEXT UNIQUE`)
	db.Exec(`ALTER TABLE users ADD COLUMN last_points_at DATETIME DEFAULT CURRENT_TIMESTAMP`)
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

func lookupSessionToken(token string) string {
	var u string
	db.QueryRow(`SELECT username FROM users WHERE session_token=?`, token).Scan(&u)
	return u
}

func bearerToken(r *http.Request) string {
	p := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(p) == 2 && strings.EqualFold(p[0], "bearer") {
		return p[1]
	}
	return ""
}

func cookieToken(r *http.Request) string {
	c, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return c.Value
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
	// Update points AND the timestamp used for tie-breaking
	db.Exec(`UPDATE users SET points=points+?, last_points_at=CURRENT_TIMESTAMP WHERE username=?`, c.Points, username)
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
//  NOTES helpers
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

func getAllNotes(page, limit int, sortDir string) []Note {
	if limit <= 0 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	order := "ASC"
	if strings.ToLower(sortDir) == "desc" {
		order = "DESC"
	}
	offset := (page - 1) * limit
	q := fmt.Sprintf(`SELECT id, title, body, created_at FROM notes ORDER BY id %s LIMIT ? OFFSET ?`, order)
	rows, _ := db.Query(q, limit, offset)
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
//  Response helpers
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
		"challenges_done": fmt.Sprintf("%d/%d", solvedCount(username), len(challenges)),
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
			"hint":  `POST /login first, then add: -H "Authorization: Bearer YOUR_TOKEN"`,
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

func requireCookieAuth(w http.ResponseWriter, r *http.Request) string {
	st := cookieToken(r)
	if st == "" {
		writeJSON(w, 401, map[string]any{
			"error": "Session cookie required.",
			"hint":  "POST /cookie-login with -c cookies.txt first, then use -b cookies.txt here.",
		})
		return ""
	}
	username := lookupSessionToken(st)
	if username == "" {
		writeJSON(w, 401, map[string]any{
			"error": "Session cookie not recognised or expired.",
			"hint":  "POST /cookie-login again to get a fresh cookie.",
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
		writeJSON(w, 404, map[string]any{"error": fmt.Sprintf("No route: %s %s", r.Method, r.URL.Path)})
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
	maxScore := 0
	for _, c := range challenges {
		maxScore += c.Points
	}
	writeJSON(w, 200, map[string]any{
		"welcome":       fmt.Sprintf("🚩 CurlSchool — %d challenges, %dpts max", len(challenges), maxScore),
		"start":         `POST /login with {"username":"...","password":"..."} to get a token`,
		"scoring":       map[string]any{"easy": "10pts", "mid": "25pts", "hard": "50pts"},
		"easy_1_to_10":  easy,
		"mid_11_to_39":  mid,
		"hard_17_20_40": hard,
	})
}

// ═══════════════════════════════════════════════════════════════════
//  POST /login  — now requires password; username is UNIQUE
// ═══════════════════════════════════════════════════════════════════

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": fmt.Sprintf("Method %s not allowed.", r.Method)})
		return
	}
	ct := r.Header.Get("Content-Type")
	raw, _ := io.ReadAll(r.Body)
	if !strings.Contains(ct, "application/json") {
		writeJSON(w, 400, map[string]any{"error": "Expected Content-Type: application/json.", "got": ct})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "Could not parse JSON body.", "details": err.Error()})
		return
	}
	username := strings.TrimSpace(body.Username)
	password := strings.TrimSpace(body.Password)
	if username == "" {
		writeJSON(w, 400, map[string]any{"error": "Field 'username' is required."})
		return
	}
	if password == "" {
		writeJSON(w, 400, map[string]any{
			"error": "Field 'password' is required.",
			"hint":  "Choose a password — you will need it to log in again.",
		})
		return
	}

	pwHash := hashPassword(password)

	// Check whether username already exists
	var existingHash, existingToken string
	err := db.QueryRow(`SELECT password_hash, token FROM users WHERE username=?`, username).
		Scan(&existingHash, &existingToken)

	if err == nil {
		// Username taken — verify password
		if existingHash != pwHash {
			writeJSON(w, 401, map[string]any{
				"error": "Wrong password for this username.",
				"hint":  "Usernames are unique. Use your original password, or pick a different username.",
			})
			return
		}
		// Correct password → return existing token
		writeJSON(w, 200, map[string]any{
			"message":         fmt.Sprintf("👋 Welcome back %s!", username),
			"token":           existingToken,
			"points":          userPoints(username),
			"challenges_done": fmt.Sprintf("%d/%d", solvedCount(username), len(challenges)),
		})
		return
	}

	// New user → register
	token := generateToken()
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, token) VALUES (?,?,?)`, username, pwHash, token); err != nil {
		writeJSON(w, 500, map[string]any{"error": "DB error — username may already be taken."})
		return
	}
	writeJSON(w, 201, map[string]any{
		"message":  fmt.Sprintf("🎉 Welcome %s! Save your token AND your password.", username),
		"username": username,
		"token":    token,
		"warning":  "Usernames are unique. Don't lose your password — you need it to log in again.",
	})
}

// ═══════════════════════════════════════════════════════════════════
//  GET /ping — Challenge 1  (POST returns 405 → C22 teaching moment)
// ═══════════════════════════════════════════════════════════════════

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]any{
			"error": fmt.Sprintf("Method %s not allowed on /ping.", r.Method),
			"hint":  "Challenge 22: you found the 405! Now fix it — use GET /ping.",
		})
		return
	}
	res := map[string]any{
		"status":  "🟢 alive",
		"time":    time.Now().Format(time.RFC3339),
		"message": "pong!",
	}
	username := lookupToken(bearerToken(r))
	if username != "" {
		for k, v := range flagResp(username, "ping", nil) {
			res[k] = v
		}
		// C22: award once they used the right method
		res["flag_22"] = flagResp(username, "wrong_method", map[string]any{
			"note": "You fixed the 405 by switching to the correct GET method!",
		})
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
		"mirror": map[string]any{"method": r.Method, "params": params, "headers": headers},
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
	if xStudent != "" && token != "" && isCurlMaster && username != "" {
		res["flag_19"] = flagResp(username, "multi_hdr", nil)
	}
	writeJSON(w, 200, res)
}

// ═══════════════════════════════════════════════════════════════════
//  GET /leaderboard — Challenge 20 + SSE stream
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
		FROM users u
		ORDER BY u.points DESC, u.last_points_at ASC, u.created_at ASC
		LIMIT 50`)
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
	maxScore := 0
	for _, c := range challenges {
		maxScore += c.Points
	}
	res := map[string]any{
		"leaderboard": board,
		"total":       rank - 1,
		"max_score":   maxScore,
		"tiebreaker":  "Equal points → first to reach that score ranks higher",
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
	go broadcastLeaderboard()
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
		"challenges_done": fmt.Sprintf("%d/%d", len(solved), len(challenges)),
		"solved":          solved,
		"remaining":       remaining,
	})
}

// ═══════════════════════════════════════════════════════════════════
//  /notes — Challenges 2,4,12,13,14,16,18,29,30,38,39
// ═══════════════════════════════════════════════════════════════════

func handleNotes(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	switch r.Method {

	case http.MethodGet:
		pageStr := r.URL.Query().Get("page")
		limitStr := r.URL.Query().Get("limit")
		sortDir := r.URL.Query().Get("sort")
		page, _ := strconv.Atoi(pageStr)
		limit, _ := strconv.Atoi(limitStr)
		mu.Lock()
		all := getAllNotes(page, limit, sortDir)
		mu.Unlock()
		res := flagResp(username, "get_all", map[string]any{"count": len(all), "notes": all})
		// C29: they successfully authenticated after seeing a 401 — award here
		res["flag_29"] = flagResp(username, "trigger_401", map[string]any{
			"note": "You fixed the 401! Token auth working correctly.",
		})
		if pageStr != "" && limitStr != "" {
			res["flag_38"] = flagResp(username, "pagination", map[string]any{"page": page, "limit": limit})
		}
		if sortDir != "" {
			res["flag_39"] = flagResp(username, "sorting", map[string]any{"sort": sortDir})
		}
		if r.Header.Get("X-Verbose") == "true" {
			res["flag_13"] = flagResp(username, "verbose", map[string]any{
				"server_info": map[string]any{"db": "sqlite3", "challenges": len(challenges)},
			})
		}
		if r.Header.Get("Accept") == "application/json" {
			res["flag_14"] = flagResp(username, "accept", nil)
		}
		writeJSON(w, 200, res)

	case http.MethodPost:
		ct := r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(ct, "application/json") {
			writeJSON(w, 400, map[string]any{
				"error":    "Content-Type header missing or incorrect.",
				"received": ct,
				"flag_16":  flagResp(username, "post_noct", nil),
			})
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
			// C30: trigger the 400 and award it
			writeJSON(w, 400, map[string]any{
				"error":   "Field 'title' is required.",
				"hint":    "Add 'title' to your JSON body and try again.",
				"flag_30": flagResp(username, "trigger_400", map[string]any{"note": "You triggered a 400! Now fix it: add the 'title' field."}),
			})
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
//  /notes/:id — Challenges 3,5,9,10,17,35,36,37
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
		res := flagResp(username, "put", map[string]any{"note": updated})
		// C35: idempotency — award every time they PUT (flag awarded on first, "already solved" on repeats)
		res["flag_35"] = flagResp(username, "put_idempotent", map[string]any{
			"note": "PUT the same payload twice — result is identical. That is idempotency!",
		})
		writeJSON(w, 200, res)

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
		// C36: PATCH only the body field
		if body.Body != nil && body.Title == nil {
			res["flag_36"] = flagResp(username, "patch_vs_put", map[string]any{
				"note": "PATCH only changed 'body'. PUT would have replaced the entire note.",
			})
		}
		writeJSON(w, 200, res)

	case http.MethodHead:
		// C37: HEAD on /notes/:id
		flag, pts, already := awardFlag(username, "head_vs_get")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("X-Note-Id", fmt.Sprintf("%d", note.ID))
		w.Header().Set("X-Note-Title", note.Title)
		w.Header().Set("X-Flag", flag)
		w.Header().Set("X-Points", fmt.Sprintf("%d", pts))
		if already {
			w.Header().Set("X-Already-Solved", "true")
		} else {
			w.Header().Set("X-Congrats", congrats(username, "mid", pts))
		}
		w.WriteHeader(200)

	case http.MethodDelete:
		mu.Lock()
		db.Exec(`DELETE FROM notes WHERE id=?`, id)
		mu.Unlock()
		writeJSON(w, 200, flagResp(username, "delete", map[string]any{"deleted": note}))

	default:
		writeJSON(w, 405, map[string]any{
			"error":     fmt.Sprintf("Method %s not allowed.", r.Method),
			"supported": []string{"GET", "PUT", "PATCH", "DELETE", "HEAD"},
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

// ═══════════════════════════════════════════════════════════════════
//  NEW CHALLENGE ROUTES (21-40)
// ═══════════════════════════════════════════════════════════════════

// C21: GET /basic-login
func handleBasicLogin(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok || username == "" || password == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="CurlSchool"`)
		writeJSON(w, 401, map[string]any{
			"error": "HTTP Basic Auth required.",
			"hint":  "Use: curl -u username:password http://localhost:8080/basic-login",
		})
		return
	}
	var pwHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username=?`, username).Scan(&pwHash); err != nil || pwHash != hashPassword(password) {
		w.Header().Set("WWW-Authenticate", `Basic realm="CurlSchool"`)
		writeJSON(w, 401, map[string]any{
			"error": "Invalid username or password.",
			"hint":  "Use the same credentials you registered with at POST /login.",
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "basic_auth", map[string]any{
		"method": "HTTP Basic Auth",
		"note":   "curl -u encodes username:password as base64 in the Authorization header.",
	}))
}

// C23: GET /moved — 301 redirect
func handleMoved(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/ping", http.StatusMovedPermanently)
}

// C24: GET /secret-header — flag lives in a response header
func handleSecretHeader(w http.ResponseWriter, r *http.Request) {
	username := lookupToken(bearerToken(r))
	if username == "" {
		writeJSON(w, 401, map[string]any{
			"error": "Token required.",
			"hint":  `Login first, then add: -H "Authorization: Bearer YOUR_TOKEN"`,
		})
		return
	}
	flag, pts, already := awardFlag(username, "resp_header")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Secret-Token", flag)
	w.Header().Set("X-Points", fmt.Sprintf("%d", pts))
	if already {
		w.Header().Set("X-Already-Solved", "true")
	}
	writeJSON(w, 200, map[string]any{
		"message":       "The flag is hiding in the X-Secret-Token response header!",
		"hint":          "Run with: curl -i ... to see all response headers.",
		"points_earned": pts,
		"your_total":    userPoints(username),
	})
}

// C25: POST /form — URL-encoded form data
func handleForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "POST only."})
		return
	}
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		writeJSON(w, 400, map[string]any{
			"error": "Expected Content-Type: application/x-www-form-urlencoded",
			"hint":  `curl -d "name=alice&course=curl101" -H "Content-Type: application/x-www-form-urlencoded" ...`,
		})
		return
	}
	raw, _ := io.ReadAll(r.Body)
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "Could not parse form data."})
		return
	}
	name := strings.TrimSpace(values.Get("name"))
	course := strings.TrimSpace(values.Get("course"))
	if name == "" || course == "" {
		writeJSON(w, 400, map[string]any{
			"error":    "Both 'name' and 'course' fields are required.",
			"received": values,
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "form_submit", map[string]any{
		"received": map[string]string{"name": name, "course": course},
		"note":     "URL-encoded: key=value&key2=value2 — same format as query strings, but in the body.",
	}))
}

// C26: GET /combo — query param + custom header required together
func handleCombo(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	mode := r.URL.Query().Get("mode")
	apiVer := r.Header.Get("X-Api-Version")
	if mode != "strict" || apiVer != "2" {
		writeJSON(w, 400, map[string]any{
			"error":    "Both ?mode=strict AND header X-Api-Version: 2 are required.",
			"got_mode": mode,
			"got_ver":  apiVer,
			"hint":     `curl "http://localhost:8080/combo?mode=strict" -H "X-Api-Version: 2" -H "Authorization: Bearer TOKEN"`,
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "query_and_hdr", map[string]any{
		"note": "You sent a query param and a custom header in the same request!",
	}))
}

// C27: GET /double-check — two specific custom headers required
func handleDoubleCheck(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	first := r.Header.Get("X-First")
	second := r.Header.Get("X-Second")
	if first != "alpha" || second != "beta" {
		writeJSON(w, 400, map[string]any{
			"error":      "Requires: X-First: alpha AND X-Second: beta.",
			"got_first":  first,
			"got_second": second,
			"hint":       `Add: -H "X-First: alpha" -H "X-Second: beta"`,
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "two_hdrs", map[string]any{
		"note": "Multiple -H flags stack — each becomes its own request header.",
	}))
}

// C28: POST /validated — JSON schema with type validation
func handleValidated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "POST only."})
		return
	}
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		writeJSON(w, 400, map[string]any{"error": "Expected Content-Type: application/json."})
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "JSON parse error."})
		return
	}
	var errs []string
	if titleStr, ok := body["title"].(string); !ok || strings.TrimSpace(titleStr) == "" {
		errs = append(errs, "'title' (string) is required")
	}
	if authorStr, ok := body["author"].(string); !ok || strings.TrimSpace(authorStr) == "" {
		errs = append(errs, "'author' (string) is required")
	}
	if _, ok := body["year"].(float64); !ok {
		errs = append(errs, "'year' (number) is required")
	}
	if len(errs) > 0 {
		writeJSON(w, 400, map[string]any{
			"error":  "Validation failed.",
			"issues": errs,
			"hint":   `Send: {"title":"My Book","author":"Alice","year":2024}`,
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "json_validation", map[string]any{
		"received": body,
		"note":     "Schema validation: server enforces field types, not just presence.",
	}))
}

// C31: POST /cookie-login
func handleCookieLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"error": "POST only."})
		return
	}
	ct := r.Header.Get("Content-Type")
	raw, _ := io.ReadAll(r.Body)
	if !strings.Contains(ct, "application/json") {
		writeJSON(w, 400, map[string]any{"error": "Expected Content-Type: application/json."})
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "JSON parse error."})
		return
	}
	username := strings.TrimSpace(body.Username)
	password := strings.TrimSpace(body.Password)
	if username == "" || password == "" {
		writeJSON(w, 400, map[string]any{"error": "Both 'username' and 'password' are required."})
		return
	}
	var pwHash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE username=?`, username).Scan(&pwHash); err != nil || pwHash != hashPassword(password) {
		writeJSON(w, 401, map[string]any{"error": "Invalid username or password."})
		return
	}
	sessionToken := generateToken()
	db.Exec(`UPDATE users SET session_token=? WHERE username=?`, sessionToken, username)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400,
	})
	flag, pts, already := awardFlag(username, "cookie_save")
	writeJSON(w, 200, map[string]any{
		"message":        fmt.Sprintf("🍪 Session started for %s!", username),
		"flag":           flag,
		"points_earned":  pts,
		"already_solved": already,
		"next":           "Now run: curl -b cookies.txt http://localhost:8080/cookie-protected",
		"tip":            "Re-run this request with -c cookies.txt to save the cookie to a file.",
	})
}

// C32: GET /cookie-protected
func handleCookieProtected(w http.ResponseWriter, r *http.Request) {
	username := requireCookieAuth(w, r)
	if username == "" {
		return
	}
	writeJSON(w, 200, flagResp(username, "cookie_reuse", map[string]any{
		"note":    "curl -b sent the session cookie automatically — just like a browser!",
		"session": "Cookie auth: server recognises you by your session ID, no manual header needed.",
	}))
}

// C33: GET /agent-check — User-Agent must start with Mozilla
func handleAgentCheck(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	ua := r.Header.Get("User-Agent")
	if !strings.HasPrefix(ua, "Mozilla") {
		writeJSON(w, 400, map[string]any{
			"error": "User-Agent must start with 'Mozilla'.",
			"got":   ua,
			"hint":  `curl -H "User-Agent: Mozilla/5.0 (compatible; CurlStudent)" ...`,
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "useragent_check", map[string]any{
		"your_ua": ua,
		"note":    "Servers often inspect User-Agent to detect browsers vs bots.",
	}))
}

// C34: GET /content-deal — response format depends on Accept header
func handleContentDeal(w http.ResponseWriter, r *http.Request) {
	username := lookupToken(bearerToken(r))
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		if username != "" {
			writeJSON(w, 200, flagResp(username, "accept_json", map[string]any{
				"format": "json",
				"note":   "You asked for JSON (Accept: application/json) and got it!",
			}))
		} else {
			writeJSON(w, 200, map[string]any{
				"format":  "json",
				"message": "JSON delivered. Add a Bearer token to earn the flag.",
			})
		}
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)
	fmt.Fprintln(w, "Plain text response. Set Accept: application/json to get JSON (and the flag).")
}

// C40: GET /boss — mini boss
func handleBoss(w http.ResponseWriter, r *http.Request) {
	username := requireAuth(w, r)
	if username == "" {
		return
	}
	level := r.URL.Query().Get("level")
	bossHdr := r.Header.Get("X-Boss")
	accept := r.Header.Get("Accept")
	var missing []string
	if level != "hard" {
		missing = append(missing, "?level=hard query param")
	}
	if bossHdr != "true" {
		missing = append(missing, "X-Boss: true header")
	}
	if accept != "application/json" {
		missing = append(missing, "Accept: application/json header")
	}
	if len(missing) > 0 {
		writeJSON(w, 400, map[string]any{
			"error":   "Boss challenge incomplete!",
			"missing": missing,
			"hint":    `curl "http://localhost:8080/boss?level=hard" -H "Authorization: Bearer TOKEN" -H "X-Boss: true" -H "Accept: application/json"`,
		})
		return
	}
	writeJSON(w, 200, flagResp(username, "mini_boss", map[string]any{
		"note": "MINI BOSS DEFEATED! You combined auth + query param + 2 custom headers in one shot!",
	}))
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

	// original routes
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
	// new routes
	mux.HandleFunc("/basic-login", wrap(handleBasicLogin))
	mux.HandleFunc("/moved", wrap(handleMoved))
	mux.HandleFunc("/secret-header", wrap(handleSecretHeader))
	mux.HandleFunc("/form", wrap(handleForm))
	mux.HandleFunc("/combo", wrap(handleCombo))
	mux.HandleFunc("/double-check", wrap(handleDoubleCheck))
	mux.HandleFunc("/validated", wrap(handleValidated))
	mux.HandleFunc("/cookie-login", wrap(handleCookieLogin))
	mux.HandleFunc("/cookie-protected", wrap(handleCookieProtected))
	mux.HandleFunc("/agent-check", wrap(handleAgentCheck))
	mux.HandleFunc("/content-deal", wrap(handleContentDeal))
	mux.HandleFunc("/boss", wrap(handleBoss))

	maxScore := 0
	for _, c := range challenges {
		maxScore += c.Points
	}
	addr := fmt.Sprintf("%s:%s", *host, *port)
	fmt.Printf("\n  🚩  CurlSchool — %d Challenges · %dpts max\n", len(challenges), maxScore)
	fmt.Printf("  Listening:  http://%s\n", addr)
	fmt.Printf("  DB:         %s\n\n", *dbPath)
	log.Fatal(http.ListenAndServe(addr, mux))
}
