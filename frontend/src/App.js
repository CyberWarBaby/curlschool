import { useEffect, useState, useRef } from "react";

const API = process.env.REACT_APP_API_URL || "";

const TIER_COLORS = { easy: "#00ff88", mid: "#ffcc00", hard: "#ff4466" };
const MAX_POINTS = 600;

function ProgressBar({ value, max, color }) {
  const pct = Math.min(100, Math.round((value / max) * 100));
  return (
    <div style={{ background: "#111", borderRadius: 2, height: 6, width: "100%", overflow: "hidden" }}>
      <div
        style={{
          height: "100%",
          width: `${pct}%`,
          background: color,
          transition: "width 0.6s cubic-bezier(0.4,0,0.2,1)",
          boxShadow: `0 0 8px ${color}`,
        }}
      />
    </div>
  );
}

function Rank({ entry, prev, isNew }) {
  const moved = prev !== null && prev !== entry.rank;
  const up = prev !== null && prev > entry.rank;
  const color =
    entry.rank === 1 ? "#ffd700"
    : entry.rank === 2 ? "#c0c0c0"
    : entry.rank === 3 ? "#cd7f32"
    : "#556";

  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "2.5rem 1fr 6rem 5rem 5rem",
        alignItems: "center",
        gap: "0.5rem",
        padding: "0.75rem 1rem",
        background: isNew ? "rgba(0,255,136,0.07)" : entry.rank <= 3 ? "rgba(255,255,255,0.03)" : "transparent",
        borderLeft: `3px solid ${color}`,
        borderRadius: "0 4px 4px 0",
        marginBottom: 4,
        transition: "background 0.4s",
        animation: isNew ? "flashIn 0.6s ease" : moved ? "slideIn 0.4s ease" : "none",
      }}
    >
      {/* rank */}
      <span style={{ fontFamily: "'JetBrains Mono', monospace", color, fontSize: "1rem", fontWeight: 700 }}>
        {entry.medal}
      </span>

      {/* name */}
      <span style={{ fontFamily: "'JetBrains Mono', monospace", color: "#eee", fontSize: "0.95rem", letterSpacing: 1 }}>
        {entry.username}
        {moved && (
          <span style={{ marginLeft: 8, fontSize: "0.7rem", color: up ? "#00ff88" : "#ff4466" }}>
            {up ? `▲ +${prev - entry.rank}` : `▼ ${entry.rank - prev}`}
          </span>
        )}
      </span>

      {/* solved */}
      <span style={{ fontFamily: "'JetBrains Mono', monospace", color: "#aaa", fontSize: "0.8rem", textAlign: "right" }}>
        {entry.solved}<span style={{ color: "#444" }}>/20</span>
      </span>

      {/* bar */}
      <ProgressBar value={entry.points} max={MAX_POINTS} color={color} />

      {/* points */}
      <span
        style={{
          fontFamily: "'JetBrains Mono', monospace",
          color: "#fff",
          fontSize: "0.95rem",
          fontWeight: 700,
          textAlign: "right",
        }}
      >
        {entry.points}
        <span style={{ color: "#444", fontSize: "0.7rem" }}>pts</span>
      </span>
    </div>
  );
}

function Ticker({ events }) {
  return (
    <div
      style={{
        borderTop: "1px solid #222",
        padding: "0.4rem 1rem",
        height: "2rem",
        overflow: "hidden",
        display: "flex",
        alignItems: "center",
        gap: "2rem",
        background: "#060606",
      }}
    >
      {events.slice(-6).map((e, i) => (
        <span
          key={i}
          style={{
            fontFamily: "'JetBrains Mono', monospace",
            fontSize: "0.7rem",
            color: i === events.length - 1 ? "#00ff88" : "#333",
            whiteSpace: "nowrap",
            transition: "color 1s",
          }}
        >
          {e}
        </span>
      ))}
    </div>
  );
}

export default function App() {
  const [board, setBoard] = useState([]);
  const [prevBoard, setPrevBoard] = useState({});
  const [newEntries, setNewEntries] = useState({});
  const [connected, setConnected] = useState(false);
  const [events, setEvents] = useState(["Connecting..."]);
  const [lastUpdate, setLastUpdate] = useState(null);
  const prevRef = useRef({});

  useEffect(() => {
    const es = new EventSource(`${API}/leaderboard/stream`);

    es.onopen = () => {
      setConnected(true);
      setEvents(e => [...e, "🟢 connected"]);
    };

    es.onmessage = (ev) => {
      try {
        const incoming = JSON.parse(ev.data);
        setLastUpdate(new Date());

        // detect rank moves and new entries
        const oldMap = prevRef.current;
        const newMap = {};
        const fresh = {};
        incoming.forEach(entry => {
          newMap[entry.username] = entry.rank;
          if (!(entry.username in oldMap)) {
            fresh[entry.username] = true;
          }
        });

        setPrevBoard({ ...oldMap });
        setNewEntries(fresh);
        setBoard(incoming);
        prevRef.current = newMap;

        // event ticker
        if (incoming.length > 0) {
          const top = incoming[0];
          setEvents(e => [
            ...e.slice(-20),
            `${top.medal} ${top.username} leads with ${top.points}pts`,
          ]);
        }

        setTimeout(() => setNewEntries({}), 2000);
      } catch (_) {}
    };

    es.onerror = () => {
      setConnected(false);
      setEvents(e => [...e, "🔴 reconnecting..."]);
    };

    return () => es.close();
  }, []);

  const totalStudents = board.length;
  const topScore = board[0]?.points ?? 0;

  return (
    <div
      style={{
        minHeight: "100vh",
        background: "#050505",
        color: "#eee",
        fontFamily: "'JetBrains Mono', monospace",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <style>{`
        @import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&family=Bebas+Neue&display=swap');
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { background: #050505; }
        @keyframes flashIn {
          0%   { background: rgba(0,255,136,0.25); }
          100% { background: rgba(0,255,136,0.07); }
        }
        @keyframes slideIn {
          0%   { transform: translateX(-8px); opacity: 0.5; }
          100% { transform: translateX(0); opacity: 1; }
        }
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50%       { opacity: 0.4; }
        }
        ::-webkit-scrollbar { width: 4px; }
        ::-webkit-scrollbar-track { background: #111; }
        ::-webkit-scrollbar-thumb { background: #333; }
      `}</style>

      {/* ── Header ── */}
      <div
        style={{
          padding: "1.5rem 2rem 1rem",
          borderBottom: "1px solid #1a1a1a",
          display: "flex",
          alignItems: "baseline",
          justifyContent: "space-between",
          gap: "1rem",
        }}
      >
        <div>
          <h1
            style={{
              fontFamily: "'Bebas Neue', sans-serif",
              fontSize: "clamp(2rem, 5vw, 3.5rem)",
              letterSpacing: 4,
              color: "#fff",
              lineHeight: 1,
            }}
          >
            🚩 CURLSCHOOL
          </h1>
          <p style={{ color: "#444", fontSize: "0.75rem", letterSpacing: 2, marginTop: 4 }}>
            LIVE LEADERBOARD · 20 CHALLENGES · 600 PTS MAX
          </p>
        </div>

        <div style={{ textAlign: "right" }}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              justifyContent: "flex-end",
              marginBottom: 4,
            }}
          >
            <span
              style={{
                width: 8,
                height: 8,
                borderRadius: "50%",
                background: connected ? "#00ff88" : "#ff4466",
                animation: connected ? "none" : "pulse 1s infinite",
                display: "inline-block",
              }}
            />
            <span style={{ fontSize: "0.7rem", color: connected ? "#00ff88" : "#ff4466" }}>
              {connected ? "LIVE" : "RECONNECTING"}
            </span>
          </div>
          {lastUpdate && (
            <div style={{ fontSize: "0.65rem", color: "#333" }}>
              updated {lastUpdate.toLocaleTimeString()}
            </div>
          )}
        </div>
      </div>

      {/* ── Stats bar ── */}
      <div
        style={{
          display: "flex",
          gap: "2rem",
          padding: "0.75rem 2rem",
          borderBottom: "1px solid #111",
          background: "#070707",
        }}
      >
        {[
          { label: "PLAYERS", value: totalStudents },
          { label: "TOP SCORE", value: `${topScore}pts` },
          { label: "MAX POSSIBLE", value: "600pts" },
          { label: "CHALLENGES", value: "20" },
        ].map(s => (
          <div key={s.label}>
            <div style={{ fontSize: "0.6rem", color: "#444", letterSpacing: 2 }}>{s.label}</div>
            <div style={{ fontSize: "1.1rem", fontWeight: 700, color: "#fff" }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* ── Tier legend ── */}
      <div
        style={{
          display: "flex",
          gap: "1.5rem",
          padding: "0.5rem 2rem",
          borderBottom: "1px solid #111",
        }}
      >
        {Object.entries(TIER_COLORS).map(([tier, color]) => (
          <div key={tier} style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <div style={{ width: 8, height: 8, borderRadius: 1, background: color }} />
            <span style={{ fontSize: "0.65rem", color: "#555", textTransform: "uppercase", letterSpacing: 1 }}>
              {tier} · {tier === "easy" ? 10 : tier === "mid" ? 25 : 50}pts
            </span>
          </div>
        ))}
      </div>

      {/* ── Board ── */}
      <div style={{ flex: 1, overflowY: "auto", padding: "1rem 2rem" }}>
        {/* Column headers */}
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "2.5rem 1fr 6rem 5rem 5rem",
            gap: "0.5rem",
            padding: "0 1rem 0.5rem",
            fontSize: "0.6rem",
            color: "#333",
            letterSpacing: 2,
            textTransform: "uppercase",
          }}
        >
          <span>#</span>
          <span>NAME</span>
          <span style={{ textAlign: "right" }}>SOLVED</span>
          <span style={{ textAlign: "right" }}>SCORE</span>
          <span style={{ textAlign: "right" }}>PTS</span>
        </div>

        {board.length === 0 ? (
          <div
            style={{
              textAlign: "center",
              color: "#333",
              padding: "4rem 0",
              fontSize: "0.9rem",
              letterSpacing: 2,
            }}
          >
            {connected ? "WAITING FOR PLAYERS..." : "CONNECTING TO SERVER..."}
          </div>
        ) : (
          board.map(entry => (
            <Rank
              key={entry.username}
              entry={entry}
              prev={prevBoard[entry.username] ?? null}
              isNew={!!newEntries[entry.username]}
            />
          ))
        )}
      </div>

      {/* ── Event ticker ── */}
      <Ticker events={events} />
    </div>
  );
}
