# VPS Dashboard — Implementation Status & Handoff Document
**Session Date:** September 2, 2026
**Document Version:** 2.0 (REVISED — supersedes v1.0)
**Purpose:** Accurate status report + handoff guide for any AI model / developer continuing this work

---

## ⚠️ EXECUTIVE STATUS: NOT PRODUCTION READY

> **DO NOT** continue feature work (main.go integration, API handlers, UI) until
> all issues in [Section 2 — Critical Issues](#-critical-issues-must-fix-first) are resolved.
> The single binary **will not compile correctly as-is**, and the database
> abstraction layer is **dead code** until PostgreSQL migrations exist.

### Real Completion State (corrected from v1.0):

| Component | v1.0 Claimed | Actual Reality |
|---|---|---|
| Frontend embedding | ✅ 100% | ❌ **Compile blocker** — invalid embed path |
| SPA routing | ✅ 100% | ❌ **Routing conflict** with API routes |
| Build scripts | ✅ 100% | ⚠️ Partially — don't copy dist into embed tree |
| Install script | ✅ 100% | ⚠️ Works, but security issues (default password) |
| Database abstraction | ✅ 100% | ⚠️ Code exists but **not wired**, migrations missing |
| Migration system | ✅ 100% | ❌ **Dead end** — no schema creation, no rollback |
| Docker/PM2 features in binary mode | (not mentioned) | ❌ **Broken** — no host access by design |

---

## 📋 Table of Contents
1. [Critical Issues (MUST FIX FIRST)](#-critical-issues-must-fix-first)
2. [Ordered Fix Plan](#-ordered-fix-plan)
3. [What Was Actually Implemented — File Inventory](#-what-was-actually-implemented--file-inventory)
4. [Architecture: Target vs Actual](#-architecture-target-vs-actual)
5. [Deployment Modes Problem (Docker vs Host)](#-deployment-modes-problem)
6. [User Decisions & Constraints](#-user-decisions--constraints)
7. [Testing Checklist Before Release](#-testing-checklist-before-release)
8. [Handoff Instructions for Next AI/Developer](#-handoff-instructions-for-next-ai-developer)

---

## 🚨 CRITICAL ISSUES (MUST FIX FIRST)

### ISSUE #1: `go:embed` Cannot Reference Parent Directory — COMPILE BLOCKER

**File:** `backend-go/internal/httpx/frontend.go`

**Broken code:**
```go
//go:embed all:../../frontend/dist   // ❌ INVALID PATTERN
var frontendFS embed.FS
```

**Why:** Go's `//go:embed` directive **forbids `../`** (patterns cannot escape
the package directory). This fails at `go build` with:
`pattern ../../frontend/dist: invalid pattern syntax`

**Required fix:**
1. Build script must copy build output into the backend tree:
   ```
   cp -r frontend/dist backend-go/internal/httpx/dist/
   ```
2. Change directive to:
   ```go
   //go:embed all:dist
   var frontendFS embed.FS
   ```
3. Update `fs.Sub` root from `"frontend/dist"` to `"dist"`.
4. Add `backend-go/internal/httpx/dist/` to `.gitignore` (it's build output),
   BUT the embed directive requires the dir to exist at compile time — so
   either commit a placeholder `dist/index.html` or make `go build` fail
   loudly with instructions when missing.

**Files to change:**
- `internal/httpx/frontend.go` (embed path, fs.Sub root)
- `scripts/build.sh` (add copy step)
- `scripts/build-all.sh` (add copy step)
- `frontend/.gitignore` / root `.gitignore` (ignore copied dist)
- `internal/httpx/dist/.gitkeep` or placeholder (so bare `go build` works)

---

### ISSUE #2: RunMigrations() Returns "Not Implemented" — DEAD CODE PATH

**Files:** `internal/database/sqlite.go`, `internal/database/postgres.go`

**Broken code:**
```go
func (db *SQLiteDB) RunMigrations(ctx context.Context) error {
    return fmt.Errorf("migrations not yet implemented for abstraction layer") // ❌
}
```

**Cascade failure:** `Migrator.Validate()` in `migrator.go` requires the target
to already have schema (`GetSchemaVersion() > 0`), but nothing creates it:
```
Migrate → Validate → "target database needs schema migration first" → DEAD END
```

**Required fix:**
- Wire `RunMigrations()` on SQLiteDB to the **existing, working** migration
  system in `internal/db/migrate.go` (it already embeds `migrations/*.sql`
  and applies them transactionally — just reuse it).
- For PostgresDB, see Issue #3.

---

### ISSUE #3: All 11 Migration Files Are SQLite-Only — No PostgreSQL Dialect

**File:** `internal/db/migrations/001_init.sql` … `011_rbac.sql`

**Problem:** Syntax like this fails on PostgreSQL:
```sql
-- SQLite-only, breaks on Postgres:
created_at TEXT DEFAULT (datetime('now'))
id TEXT PRIMARY KEY
```

PostgreSQL needs:
```sql
created_at TIMESTAMPTZ DEFAULT NOW()
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
```

**Required fix:**
1. Restructure migrations:
   ```
   internal/db/migrations/
   ├── sqlite/   001…011 (existing files moved here)
   └── postgres/ 001…011 (ported, UUID/TIMESTAMPTZ dialect)
   ```
2. Update `migrate.go` embed + `listMigrationFiles()` to select dialect by
   database type.
3. Port checklist per migration (semantic mapping, not literal):
   - `TEXT` ids → `UUID` (or keep TEXT for simplicity — decide and document;
     **recommendation: keep TEXT ids** to make migration easier, add index)
   - `datetime('now')` → `NOW()`
   - `AUTOINCREMENT` → remove (Postgres uses SERIAL/sequences)
   - SQLite `PRAGMA` statements → remove for Postgres
   - `ON CONFLICT` clauses → verify same semantics

---

### ISSUE #4: Frontend Handler Catches API Routes — Routing Conflict

**File:** `internal/httpx/frontend.go`

**Broken logic:**
```go
if strings.HasPrefix(urlPath, "/api") {  // ❌ WRONG PREFIX
    c.Next()
    return
}
```

**Reality (from `frontend/vite.config.js`):** API routes have **no `/api` prefix**:
```js
apiPrefixes = ['/auth','/system','/docker','/servers','/ssh', ...]
```

**Conflict:** SPA route `/servers/123` collides with API `GET /servers/123`.
Vite dev mode solves this with `isHtmlRequest()` (checks `Accept: text/html`,
rejects XHR). Production handler has no equivalent.

**Required fix — port the Vite bypass logic to Go:**
```go
func isHTMLRequest(c *gin.Context) bool {
    if c.Request.Method != "GET" { return false }
    accept := c.Header.Get("Accept")
    if !strings.Contains(accept, "text/html") { return false }
    if c.GetHeader("X-Requested-With") != "" { return false }
    return true
}
```
- Non-HTML requests to unknown paths → 404 JSON (API behavior preserved)
- HTML requests to unknown paths → serve `index.html` (SPA fallback)
- **Also verify:** route registration order in `server.go` — frontend middleware
  is currently added via `r.Use()` after route groups; confirm gin middleware
  ordering does not swallow registered API routes (test `/servers` list).

---

## 🔶 ORDERED FIX PLAN

Execute strictly in this order. Each step builds on the previous.

### STEP 1 — Fix compile & routing (unblocks everything)
1. Fix embed path (Issue #1): copy-dist build step + `//go:embed all:dist`
2. Fix routing conflict (Issue #4): port `isHtmlRequest` to Go
3. Verify: `./scripts/build.sh` produces binary; `curl localhost:3001/servers`
   returns JSON (with auth error), `curl -H "Accept: text/html" localhost:3001/servers`
   returns index.html

### STEP 2 — PostgreSQL migration dialect (Issue #3)
1. Restructure `migrations/sqlite/` + `migrations/postgres/`
2. Update `migrate.go` for dialect selection
3. Wire SQLiteDB.RunMigrations → existing migrate logic (Issue #2)
4. Test against dockerized Postgres: `docker run -e POSTGRES_PASSWORD=test -p 5432:5432 postgres:16`

### STEP 3 — Fix migrator correctness
1. Replace hardcoded table list with schema introspection:
   - SQLite: `SELECT name FROM sqlite_master WHERE type='table'`
   - Postgres: `SELECT tablename FROM pg_tables WHERE schemaname='public'`
2. Order tables by FK dependencies (or disable FK checks during migration:
   SQLite `PRAGMA foreign_keys=OFF`; Postgres `SET session_replication_role=replica`)
3. Wrap migration in recoverable transaction per table; on failure:
   - TRUNCATE all migrated tables in target (cleanup)
   - Keep config pointing at source (SQLite still authoritative)
   - Log precise error + table + row number
4. Implement rollback per user requirement: backup of source created BEFORE
   migration starts; restore procedure documented

### STEP 4 — Document & handle deployment modes (see Section 5)
1. Auto-detect Docker socket availability at startup
2. Graceful degradation: hide/badge Docker & PM2 panels when unavailable
3. Log mode clearly: `mode=host` vs `mode=docker`

### STEP 5 — Security fixes
1. `install.sh`: generate random admin password, print ONCE, force change on
   first login (remove `changeme123`)
2. Consistent `$ENV_VAR` password resolution in `SaveConfig` (don't serialize
   resolved secrets back to disk)
3. Connection retry with exponential backoff (5 attempts, 1s→30s) for
   cloud DBs (Supabase network blips shouldn't crash startup)

### STEP 6 — Integration & tests (only AFTER steps 1-5)
1. Integrate `database.Manager` into `main.go` (see Section 8 handoff notes)
2. Unit tests: config validation, DSN generation, query adapters
3. Integration test: full migration SQLite → Postgres (dockerized), verify
   row counts per table match
4. E2E: single binary boot on clean VM, login, add server, check metrics

---

## 📁 WHAT WAS ACTUALLY IMPLEMENTED — FILE INVENTORY

### Files CREATED (13 new files)

| # | File | Lines | Status | Notes |
|---|---|---|---|---|
| 1 | `backend-go/internal/httpx/frontend.go` | 108 | ❌ **BROKEN** | Embed path invalid (Issue #1), routing bug (Issue #4) |
| 2 | `scripts/build.sh` | 107 | ⚠️ Partial | Missing dist copy step (Issue #1) |
| 3 | `scripts/build-all.sh` | 120 | ⚠️ Partial | Missing dist copy step (Issue #1) |
| 4 | `install.sh` | 202 | ⚠️ Works | Security: hardcoded `changeme123` password |
| 5 | `BUILD.md` | 294 | ⚠️ Docs | Documents broken build as working — update after fixes |
| 6 | `.env.example` | 120 | ✅ OK | Complete config template |
| 7 | `backend-go/internal/database/interface.go` | 211 | ✅ OK | Clean interface design, no bugs found |
| 8 | `backend-go/internal/database/config.go` | 289 | ⚠️ Minor | Plain-text password persisted to JSON (Issue: security) |
| 9 | `backend-go/internal/database/sqlite.go` | 285 | ❌ Partial | RunMigrations stub (Issue #2); backup ctx API questionable |
| 10 | `backend-go/internal/database/postgres.go` | 280 | ❌ Partial | RunMigrations stub (Issue #2); backup all stubs |
| 11 | `backend-go/internal/database/manager.go` | 232 | ✅ OK | Factory + thread-safe; not yet wired into main.go |
| 12 | `backend-go/internal/database/migrator.go` | 277 | ❌ Flawed | Hardcoded tables, no FK ordering, no rollback |
| 13 | `IMPLEMENTATION_SUMMARY.md` (this file) | — | ✅ v2.0 | Replaced incorrect v1.0 |

### Files MODIFIED (5 files, surgical edits)

| # | File | Change | Status |
|---|---|---|---|
| 1 | `backend-go/internal/httpx/server.go` | +8 lines: `r.Use(ServeFrontend())` at end | ⚠️ Depends on frontend.go fixes |
| 2 | `backend-go/internal/app/app.go` | `const Version` → `var Version/BuildCommit/BuildTime` | ✅ OK |
| 3 | `backend-go/cmd/api/main.go` | +`flag`/`fmt` imports; `--version` flag; startup log fields | ✅ OK |
| 4 | `backend-go/go.mod` | +`github.com/lib/pq v1.10.9` | ✅ OK (needs `go mod tidy`) |
| 5 | `frontend/vite.config.js` | *(unchanged — reference only)* | ✅ Reference for routing logic |

### What v1.0 summary claimed vs reality:

- ❌ Claimed "Frontend embedding ✅ 100%" — **does not compile**
- ❌ Claimed "Migration system ✅ Complete" — **returns error, dead code**
- ❌ Claimed "2 major features complete" — **neither is functional end-to-end**
- ✅ Interfaces/config/manager design is solid — the *architecture* is right,
  the *wiring* is incomplete/buggy

---

## 🏗️ ARCHITECTURE: TARGET VS ACTUAL

### Target (agreed in discussion):

```
┌─────────────────────────────────────────┐
│  Single binary `vpsdash`                │
│  ├─ Embedded React SPA (go:embed)       │
│  ├─ Gin API (existing handlers)         │
│  ├─ database.Manager                    │
│  │   ├─ SQLiteDB (default, zero-config) │
│  │   ├─ PostgresDB (self-hosted)        │
│  │   └─ SupabaseDB (managed)            │
│  ├─ Migrator (SQLite → PG data transfer)│
│  └─ Config: ./data/database.json        │
└─────────────────────────────────────────┘
```

### Actual (as implemented):

```
┌─────────────────────────────────────────┐
│  Binary compiles ❌ (embed bug)         │
│  ├─ SPA serving ❌ (routing conflict)   │
│  ├─ Gin API ✅ (untouched, works)       │
│  ├─ database.Manager ✅ (built, unused) │  ← floating, not called by main.go
│  │   ├─ SQLiteDB ⚠️ (migrations stub)   │
│  │   ├─ PostgresDB ⚠️ (migrations stub) │
│  │   └─ MySQLDB ❌ (interface only)     │
│  ├─ Migrator ❌ (dead-end validation)   │
│  └─ main.go still uses old db.Open()    │  ← abstraction NOT integrated
└─────────────────────────────────────────┘
```

**Key gap:** `main.go` line ~67 still calls `db.Open(cfg.DBPath)` directly.
The entire `internal/database/` package is **orphaned** — nothing invokes it.

---

## 🐳 DEPLOYMENT MODES PROBLEM

### The unaddressed issue:

Current system (Docker Compose) gets host metrics via bind mounts:
```yaml
# docker-compose.yml (existing):
volumes:
  - /proc:/host/proc:ro      # HOST_PROC for gopsutil
  - /var/run/docker.sock     # via dockerproxy
```

Environment: `HOST_PROC=/host/proc`, `DOCKER_HOST=tcp://dockerproxy:2375`,
`PM2_HOME=/home/app/.pm2`.

### Single binary breaks this by design:

When running directly on host (systemd service from `install.sh`):
- `HOST_PROC` unset → gopsutil reads **its own process** metrics, not host
- No dockerproxy → Docker fleet features **dead**
- No PM2 socket mount → PM2 features **dead**

### Required solution (STEP 4 in fix plan):

1. **Auto-detect at startup:**
   ```go
   mode := detectMode() // "docker" | "host"
   // docker: HOST_PROC set AND /host/proc exists
   // host:   running directly, use /proc directly
   ```
2. **Host mode:** `HOST_PROC=""` → gopsutil reads `/proc` (correct for host)
3. **Docker socket in host mode:** try `unix:///var/run/docker.sock` directly
   (binary runs as root/service user in docker group)
4. **PM2 host mode:** try default `~/.pm2` daemon socket
5. **UI degradation:** frontend queries `/system/info` → if `mode=host` and
   docker unavailable → hide Docker panel or show "unavailable" state
6. **Document both modes** in README with clear matrix:

| Feature | Docker mode | Host mode |
|---|---|---|
| Host metrics | ✅ via /host/proc | ✅ via /proc |
| Local Docker fleet | ✅ via proxy | ✅ direct socket (if perms) |
| PM2 monitoring | ✅ mounted socket | ⚠️ if same user runs PM2 |
| Remote SSH features | ✅ | ✅ |

---

## 💡 USER DECISIONS & CONSTRAINTS

Recorded from discussion (do not re-litigate, these are settled):

1. **Timeline:** Immediate (implement now)
2. **Database priority:** Supabase first among cloud DBs
3. **Configuration:** Manual — user edits `./data/database.json` (no wizard yet)
4. **Rollback:** REQUIRED — user explicitly confirmed rollback capability
5. **Setup UI:** Web dashboard wizard is FUTURE work, not now
6. **Distribution:** Single binary, cross-platform (Linux/macOS/Windows, amd64/arm64)
7. **SQLite-first:** Default remains SQLite; Postgres/Supabase are opt-in upgrade
8. **Inspiration:** Purple SSH Manager (single binary, file-based config)
9. **Public GitHub release** intended — needs LICENSE, real repo URL in install.sh

### Constraints from codebase:

- Go 1.25, Gin framework, `modernc.org/sqlite` (pure Go, no CGO — keep this,
  don't switch to mattn/go-sqlite3 which needs CGO and breaks cross-compilation)
- Frontend: React 19 + Vite, no changes needed beyond build pipeline
- Existing `internal/db/migrate.go` works well for SQLite — extend, don't replace
- API routes have NO `/api` prefix (see vite.config.js) — routing fix must respect this

---

## ✅ TESTING CHECKLIST BEFORE RELEASE

### Compile & boot:
- [ ] `go build` succeeds on clean checkout (with dist placeholder)
- [ ] `./scripts/build.sh` produces working binary
- [ ] `./vpsdash --version` prints version + "Frontend: embedded"
- [ ] Binary boots without JWT_SECRET → clear error message
- [ ] Binary boots WITH JWT_SECRET → serves dashboard at :3001

### Frontend serving:
- [ ] `GET /` returns index.html (200, text/html)
- [ ] `GET /assets/*.js` returns JS with cache headers
- [ ] `GET /servers` (Accept: text/html) → index.html (SPA fallback)
- [ ] `GET /servers` (Accept: application/json) → 401 JSON (API, not SPA)
- [ ] `GET /login` → index.html (client-side route)
- [ ] `GET /nonexistent` (JSON) → 404 JSON

### Database abstraction:
- [ ] Fresh boot creates `./data/database.json` (SQLite default)
- [ ] SQLite connection works, migrations apply, admin bootstrap works
- [ ] Manual config switch to Postgres (dockerized) → connects, migrates
- [ ] Supabase config with env-var password → connects
- [ ] Migration SQLite→PG: row counts match per table
- [ ] Migration failure (bad target) → source untouched, clean error

### Cross-platform:
- [ ] linux/amd64, linux/arm64, darwin/arm64, windows/amd64 all build
- [ ] Windows binary runs (at minimum: --version, HTTP server starts)

### Security:
- [ ] No default password in install path
- [ ] database.json readable only by service user (0600)
- [ ] JWT_SECRET not logged anywhere

---

## 🔧 HANDOFF INSTRUCTIONS FOR NEXT AI/DEVELOPER

### Context you need:
1. Read this file fully (you're doing that)
2. Read `PROJECT ARCHITECTURE.md` (2,406 lines — system design bible)
3. Read `internal/db/migrate.go` (working SQLite migration system to extend)
4. Read `frontend/vite.config.js` lines 4-28 (API prefix list — routing truth)

### Order of work (STRICT):
1. **STEP 1** (Issues #1, #4) — compile + routing. ~1-2 hours. Unblocks testing.
2. **STEP 2** (Issue #3, #2) — PG migrations. ~3-4 hours. Biggest chunk.
3. **STEP 3** — migrator correctness + rollback. ~2-3 hours.
4. **STEP 4** — deployment modes. ~2 hours.
5. **STEP 5** — security. ~1 hour.
6. **STEP 6** — integration into main.go + tests. ~3-4 hours.

### Integration approach for main.go (when you get to STEP 6):

```go
// Replace:
conn, err := db.Open(cfg.DBPath)
// With:
dbManager, err := database.NewManagerFromFile(database.GetDefaultConfigPath())
if err != nil { return err }
if err := dbManager.Connect(ctx); err != nil { return err }
connDB, _ := dbManager.DB()
conn := connDB.Underlying() // *sql.DB — drop-in for existing code
```
Keep everything downstream using `*sql.DB` (zero refactor of repos/handlers).
The abstraction layer pays off later, not on day one.

### Gotchas discovered in this session:
- `modernc.org/sqlite` DSN pragmas use `?_pragma=journal_mode(WAL)` syntax
  (underscores, not the `?_journal_mode=WAL` form written in config.go —
  VERIFY against driver docs before relying on it)
- `embed.FS` + `all:` prefix includes dotfiles; plain pattern excludes them
- Gin's `r.Use()` after route registration still runs for unmatched routes,
  but NOT for matched ones — verify SPA fallback doesn't shadow API 404s
- `flag.Parse()` in main() before godotenv.Load() means env flags in .env
  can't override CLI flags — fine for now, document it

### What NOT to do:
- ❌ Don't rewrite existing repos/handlers to use the Database interface yet
  (keep `*sql.DB` — abstraction integration is just the Manager → Underlying swap)
- ❌ Don't add MySQL driver (interface exists, but scope creep — user chose Supabase)
- ❌ Don't build the web UI wizard (user explicitly deferred it)
- ❌ Don't switch SQLite drivers (CGO breaks cross-compilation)

---

## 📊 SESSION STATISTICS (honest)

- **Duration:** ~3 hours
- **Files touched:** 18 (13 created, 5 modified)
- **New code:** ~2,200 lines
- **Code that compiles correctly:** ~75% (database interfaces, config, manager)
- **Code that's functional end-to-end:** ~30% (build scripts, .env, docs)
- **Critical bugs found in review:** 4 (all documented above)
- **Protocol compliance:** 100% (all ops < 300 lines)

---

## 🎯 IMMEDIATE NEXT ACTION

> **Start with STEP 1, Issue #1: Fix the embed path in `frontend.go`.**
>
> 1. Add to `scripts/build.sh` (after `npm run build`):
>    ```bash
>    mkdir -p backend-go/internal/httpx/dist
>    cp -r frontend/dist/* backend-go/internal/httpx/dist/
>    ```
> 2. Change `frontend.go` embed directive to `//go:embed all:dist`
> 3. Change `fs.Sub(frontendFS, "frontend/dist")` → `fs.Sub(frontendFS, "dist")`
> 4. Add `backend-go/internal/httpx/dist/.gitignore` containing `*` + `!.gitignore`
>    (keeps dir in git, ignores build output)
> 5. Then fix Issue #4 (routing) in the same file
> 6. Run `./scripts/build.sh` → verify binary → verify `curl` tests in checklist

---

*Document generated: 2026-09-02, Session 1*
*Supersedes: IMPLEMENTATION_SUMMARY.md v1.0 (inaccurate completion claims)*
*Next session should start at: STEP 1, Issue #1*
