# ow-api — Code Audit

Datum: 2026-08-01 · Commit: `21fe44c` · Go 1.24.2 · ~3.100 LOC
Werkzeuge: `go vet` (sauber), `govulncheck` (32 erreichbare Vulns), manuelles Review aller Pakete.

Priorität: **P0** = sofort, **P1** = zeitnah, **P2** = Verbesserung.

---

## Status: umgesetzt

Alle unten aufgeführten Punkte wurden implementiert. Verifikation nach dem Umbau:

| Prüfung | Vorher | Nachher |
|---|---|---|
| `govulncheck ./...` | 32 erreichbare Vulns | **0** |
| `go vet ./...` | sauber | sauber |
| `gofmt -l .` | — | leer |
| `go test -race ./...` | 1 Testdatei (44 Zeilen) | **6 Pakete grün, ~60 Testfälle** |
| Smoke-Test | — | Validierung, Auth, Rate-Limit, Graceful Shutdown verifiziert |

**Nicht committet** — die Änderungen liegen als Arbeitskopie im Working Tree.

### Was sich strukturell geändert hat

- `service/cache.go` gelöscht (reiner Pass-Through-Wrapper ohne Logik)
- `service/validate.go` neu (Input-Validierung)
- `ovrstat.Stats`/`ProfileStats`/`GetUnlockInfo` nehmen jetzt ein `context.Context` als erstes Argument — **Breaking Change für externe Nutzer des Pakets**
- `cache.RedisCache`-Methoden nehmen `ctx` als erstes Argument
- Cache-Werte sind jetzt in `cache.Entry[T]` gewrappt (`{cachedAt, value}`) — **bestehende Cache-Einträge werden als Miss behandelt und neu gescraped**; einmalig `FLUSHDB` oder einfach auslaufen lassen
- `ovrstat.SummaryStats` als eingebettete Struct — JSON-Ausgabe bleibt identisch
- Tests: `ovrstat/player_stats_test.go`, `service/{validate,admin,news,persist}_test.go`, `cache/redis_test.go`

### Was bewusst offen blieb

- **Admin-Passwort weiterhin als Klartext in der Konfiguration.** Es wird intern gehasht und konstantzeitig verglichen, aber ein echtes bcrypt/argon2-Setup würde das Konfigurationsformat brechen. Sinnvoll als nächster Schritt zusammen mit Docker Secrets.
- **Keine HTML-Fixture-Tests für den Parser.** Dafür bräuchte es echte, gespeicherte Profilseiten von Blizzard unter `testdata/`. Die Guards sind getestet, das Zusammenspiel mit echtem Markup nicht.
- **`config.go` weiterhin mit handgeschriebenen `os.Getenv`-Blöcken.** Funktioniert, ist nur redundant; ein Umstieg auf env-Tags wäre reine Kosmetik mit Regressionsrisiko.

---

## 1. Security

### P0-1 — Toolchain & Dependencies mit erreichbaren CVEs
`govulncheck` meldet **32 Schwachstellen, die euer Code tatsächlich erreicht**.

| Modul | Aktuell | Fix |
|---|---|---|
| Go stdlib | 1.24.2 | ≥ 1.25.12 (crypto/tls, crypto/x509, net/http, net/url, os) |
| `golang.org/x/net` | v0.48.0 | v0.55.0 — 5 HTML-Parser-Bugs, direkt über `goquery.NewDocumentFromReader` erreichbar (DoS beim Parsen fremden HTML) |
| `github.com/labstack/echo/v4` | v4.9.0 | v4.15.4 — v4.9.0 ist ~3 Jahre alt |
| `github.com/golang-jwt/jwt` | v3.2.2 | Nie gefixt (GO-2025-3553). Kommt transitiv über Echo-Middleware; entfällt mit Echo-Update |
| `github.com/PuerkitoBio/goquery` | v1.8.0 | v1.12.0 |

Kritisch hier: **GO-2026-5028** (DoS beim Parsen beliebigen HTML) ist über `ProfileStats` → `goquery` von außen ansteuerbar.

```
go get go@1.25.12 golang.org/x/net@latest github.com/labstack/echo/v4@latest github.com/PuerkitoBio/goquery@latest
go mod tidy && govulncheck ./...
```

### P0-2 — Admin-Auth: Timing-Angriff + kein Rate-Limit
`service/admin.go:29` — `token != adminPassword` ist ein nicht-konstantzeitiger Vergleich. Zusammen mit **fehlendem Rate-Limiting** ist das Passwort brute-forcebar.

```go
import "crypto/subtle"
if subtle.ConstantTimeCompare([]byte(token), []byte(adminPassword)) != 1 { ... }
```
Zusätzlich: `middleware.RateLimiter` auf die `/admin`-Gruppe legen, und das Passwort als Hash (bcrypt/argon2) statt Klartext ablegen.

### P0-3 — Unvalidierter User-Input in Upstream-URLs
`ovrstat/player_stats.go:231` und `:633` bauen URLs per String-Konkatenation aus dem Pfadparameter `:tag`:

```go
req, err := http.NewRequest("GET", baseURL+"/"+tag, nil)   // :231
apires, err := http.Get(apiURL + tag)                       // :633
```

`tag` kommt ungefiltert aus der Route. Folgen:
- `../` im Tag → Pfad-Traversal auf `overwatch.blizzard.com` (anderer Endpunkt wird abgefragt)
- `?`/`#` im Tag → Query- bzw. Fragment-Injection
- Beliebig lange Tags → unnötige Upstream-Last

Fix: Tag gegen eine Whitelist validieren (BattleTag-Format `^[\p{L}\p{N}_]{3,32}[-#]\d{4,7}$`, Konsolen-Gamertags entsprechend) und mit `url.PathEscape` einsetzen. Ebenso `platform` gegen `pc`/`console` prüfen — aktuell wird der Parameter gar nicht validiert.

### P1-4 — Keine Server-Timeouts (Slowloris)
`service/service.go:103` startet Echo mit Defaults: kein `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `ReadHeaderTimeout`. Ein Angreifer hält Verbindungen beliebig lange offen.

```go
s := &http.Server{Addr: ":"+port, ReadHeaderTimeout: 5*time.Second,
    ReadTimeout: 15*time.Second, WriteTimeout: 60*time.Second, IdleTimeout: 60*time.Second}
e.StartServer(s)
```

### P1-5 — CORS komplett offen
`service/service.go:127` — `middleware.CORS()` erlaubt jeden Origin. Für eine öffentliche Read-API vertretbar, aber die `/admin`-Routen sollten davon ausgenommen bzw. auf eine Origin-Allowlist eingeschränkt werden.

### P1-6 — Kein Body-Limit
`admin.POST /news` und `/season-resets` binden JSON ohne `middleware.BodyLimit("64K")`. `req.Resets []int` ist unbegrenzt — ein 100-MB-Array landet in Memory und wird auf Platte geschrieben.

### P1-7 — Kein Längen-/Inhaltslimit auf News-Content
`service/admin.go:120` prüft nur auf leer. Es gibt kein Maximum. Die Datei wächst unbegrenzt, `/news` liefert sie komplett aus. Empfehlung: max. 500 Zeichen, max. N Items.

### P2-8 — Middleware-Reihenfolge
`e.Pre(middleware.Secure())` (Zeile 124) steht *nach* `e.Use(...)`-Aufrufen im Code, wird durch `Pre` aber vorgezogen — funktioniert, ist aber verwirrend. Außerdem fehlt `middleware.RequestID` und ein `RecoverWithConfig` mit sinnvollem Log.

### P2-9 — Admin-Passwort im Klartext im Log-Pfad
Kein Leak im Log selbst (gut), aber `docker-compose.yml` reicht `ADMIN_PASSWORD` als Env durch — sichtbar in `docker inspect`. Empfehlung: Docker Secret / `_FILE`-Suffix-Pattern.

---

## 2. Bugs & Korrektheit

### P0-10 — Garantierte Panics beim Parsen
Vier Stellen indizieren Regex-/Node-Ergebnisse ohne Prüfung. Ändert Blizzard das Markup — was bei einem Scraper der Normalfall ist — panict der Handler:

| Ort | Code |
|---|---|
| `player_stats.go:661` | `endorsementRegexp.FindStringSubmatch(ps.EndorsementIcon)[1]` |
| `player_stats.go:736` | dito |
| `player_stats.go:368`, `:498` | `filterRegexp.FindStringSubmatch(id)[1]` |
| `player_stats.go:690, 694, 703` | `sel.Find(...).Nodes[0]` |

`middleware.Recover()` fängt das im HTTP-Pfad ab → 500 statt Absturz. **Aber**: in `logging.go:29/54` (`triggerScraperUpdate`) und in `cmd/scraper/main.go` laufen dieselben Funktionen in eigenen Goroutines **ohne** Recover → **der ganze Prozess stirbt**.

Fix: überall `m := re.FindStringSubmatch(x); if len(m) > 1 { ... }` bzw. `if len(sel.Nodes) > 0`. Zusätzlich Recover in beiden Goroutine-Startpunkten.

### P0-11 — Goroutine-Leak bei Timeout
`service/stats.go:20–68`: `statsWithTimeout` kehrt bei `ctx.Done()` zurück, die gestartete Goroutine läuft aber weiter (der HTTP-Client hat 15 s eigenen Timeout, der Kontext wird nie durchgereicht). Bei Timeout-Last stapeln sich Goroutines **und** parallele Requests gegen Blizzard → Risiko IP-Ban.

Fix: `ctx` bis in `ovrstat.Stats` durchreichen (`http.NewRequestWithContext`) und die Wrapper-Funktionen entfallen lassen. Das behebt gleichzeitig P1-14.

### P0-12 — Fehlererkennung per String-Vergleich
`stats.go:88` — `if err.Error() == "request timeout"`. Extrem brüchig; jede Umformulierung bricht den kompletten Cache-Fallback-Pfad still. Ersetzen durch eine Sentinel `var ErrTimeout = errors.New(...)` + `errors.Is`.
Analog `stats.go:110` — `if err == ovrstat.ErrPlayerNotFound` bricht, sobald irgendwo `errors.Wrap` dazwischenkommt → `errors.Is`.

### P1-13 — Unbegrenztes Spawnen von Hintergrund-Scrapes
`logging.go:29`/`:54`: jeder Timeout-Request startet eine Goroutine, die Blizzard anfragt. Kein Dedupe, kein Limit. 1.000 gleichzeitige Anfragen für denselben Spieler = 1.000 Upstream-Requests. Fix: `singleflight.Group` + Worker-Pool oder Redis-Set als Queue.

### P1-14 — Ignorierte Fehler
`stats.go:130` und `:210`: `redisCache.Set(...)` ohne Fehlerprüfung im Private-Profile-Zweig (an den anderen Stellen wird geprüft).
`player_stats.go:404`, `:719`, `:774`, `:923–944`: dutzendweise `_ = strconv.Atoi(...)` — bei Parse-Fehler steht still eine `0` im JSON. Für einen Stats-Dienst sind stille Nullen schlimmer als ein Fehler.

### P1-15 — Context in Struct gespeichert
`cache/redis.go:19` hält ein `ctx context.Context` als Feld. Go-Antipattern: keine Cancellation, kein Timeout pro Operation, Redis-Aufrufe können unbegrenzt hängen. Fix: `ctx` als erstes Argument jeder Methode.

### P1-16 — Kein Graceful Shutdown (API)
`service.go:103` — `e.Logger.Fatal(e.Start(...))`. Kein SIGTERM-Handling; laufende Requests und Datei-Writes (`writeFileReplacing`) brechen mitten drin ab. Der Scraper (`cmd/scraper`) macht es richtig — die API nicht.

### P2-17 — `strings.Title` ist deprecated
`player_stats.go:1077` — seit Go 1.18 deprecated und bei Unicode fehlerhaft. Ersetzen durch `golang.org/x/text/cases.Title`.

### P2-18 — Nicht-atomarer Write unter Windows
`service/persist.go:16` löscht die Zieldatei vor dem Rename. Crash im Fenster dazwischen = Datenverlust. Auf Windows `os.Rename` direkt versuchen und nur bei Fehler auf Delete+Rename zurückfallen.

### P2-19 — Redis-Key-Kollision
`cache/redis.go:47` — `fmt.Sprintf("ow:stats:%s:%s", platform, tag)`. Ein Tag mit `:` erzeugt kollidierende Keys, und `cmd/scraper/main.go:88` splittet den Key wieder per `strings.Split(key, ":")` und geht von genau 4–5 Teilen aus. Tags mit `:` brechen den Scraper. Fix: Tag beim Key-Bau escapen (z. B. `url.QueryEscape`) oder Hash verwenden.

### P2-20 — Toter/duplizierter Code
`service/cache.go` ist ein reiner Pass-Through-Wrapper ohne Zusatzlogik (4 Methoden, die nur delegieren) — kann entfallen.
`service/logging.go:75` `getClientIP(c interface{})` mit Type-Assertion auf ein Ad-hoc-Interface, wird nirgends aufgerufen (`c.RealIP()` wird direkt genutzt). Löschen.
`statsComplete` und `statsProfile` sind zu ~90 % identisch (je 80 Zeilen) — Generics-Kandidat.

---

## 3. Performance

### P0-21 — Der Cache wird faktisch nie genutzt
Das ist die wichtigste Performance-Erkenntnis. In `stats.go:82` wird **bei jedem Request live gescraped**; Redis wird *nur* im Timeout-Zweig gelesen. Konsequenz:

- Jeder API-Aufruf = 2–4 HTTP-Roundtrips zu Blizzard (`resolveCareerID` → ggf. Redirect-Kette → Profilseite → ggf. Unlocks-API) + HTML-Parsing von mehreren hundert KB.
- Der 24-h-TTL-Cache bringt praktisch null Latenzgewinn.
- Upstream-Last skaliert linear mit Traffic → Rate-Limit/Ban durch Blizzard.

Fix: **Cache-first.** Cache lesen → Hit mit frischem Eintrag sofort ausliefern → bei abgelaufenem/fehlendem Eintrag scrapen (mit `singleflight` gegen Thundering Herd) → stale-while-revalidate für Einträge zwischen Soft- und Hard-TTL. Realistisch: 5 s p99 → <10 ms für Cache-Hits.

### P1-22 — `KEYS` statt `SCAN`
`cache/redis.go:120` nutzt `client.Keys(pattern)`. `KEYS` ist O(N) und **blockiert den kompletten Redis-Server**. Aufgerufen von `adminCacheStats`, `adminTriggerScraper` und `scrapeAll`. Bei 100 k Keys ist Redis für alle Clients für Sekunden tot. Ersetzen durch `client.Scan` mit Cursor-Iteration.

Nebenbei: `adminCacheStats` gibt **alle Keys** im JSON zurück — bei großem Cache eine mehrstellige MB-Response.

### P1-23 — Regex-Kompilierung im Hot Path
`player_stats.go:1069` — `regexp.MustCompile(...)` in `cleanJSONKey`, das pro Stat-Key aufgerufen wird (hunderte Male je Profil). Als Paket-Level-`var` herausziehen; die anderen vier Regexes sind bereits korrekt platziert.

Ebenda `:1071–1072`: `FindStringSubmatch` wird zweimal für dasselbe Ergebnis aufgerufen.

### P1-24 — `primeOWSession` bei jedem Namecard-Lookup
`GetUnlockInfo` (`:137`) ruft `primeOWSession` bei **jedem** Aufruf auf — ein zusätzlicher HTTP-Request pro Profil, obwohl der `CookieJar` prozessweit geteilt wird. Session einmalig primen (`sync.Once` + Refresh bei 401/403).

### P1-25 — Eigener Client ohne Wiederverwendung
`player_stats.go:222` legt in `resolveCareerID` bei **jedem Aufruf** einen neuen `http.Client` + `cookiejar` an → keine Connection-Wiederverwendung, neuer TLS-Handshake pro Request. Einen Paket-Client mit konfiguriertem `Transport` (`MaxIdleConnsPerHost`) verwenden.

Ebenso `retrievePlayers:633` — `http.Get` nutzt `http.DefaultClient`: **kein Timeout**, kann unbegrenzt hängen.

### P2-26 — Data Race auf `owClient`
`getOWClient()` (`:46`) prüft und schreibt `owClient` ohne Synchronisation. Wird aus mehreren Goroutines aufgerufen (`triggerScraperUpdate`, parallele Requests) → Race, mehrere Clients, potenziell verlorene Cookie-Jars. `sync.Once` verwenden. (`go vet` findet das nicht — mit `go test -race` reproduzierbar.)

Gleiches gilt für die Paket-Globals `adminPassword`, `redisCache`, `apiTimeout`, `debugLogging`, `Debug`, `newsService`, `seasonResetsService`: alle werden beim Start geschrieben und danach gelesen — beim aktuellen Start-Ablauf unkritisch, aber fragil. Sauberer wäre eine `Service`-Struct statt globaler Variablen.

### P2-27 — Ineffiziente News-Operationen
`news.go:104` sortiert bei **jedem** `GET /news` neu, obwohl die Liste sich selten ändert. `DeleteNews` (`:135`) ist O(n) mit Slice-Rebuild. Bei kleinen Listen egal — nur erwähnt für den Fall, dass News wächst.

---

## 4. Modernisierung

| Thema | Status | Empfehlung |
|---|---|---|
| `github.com/pkg/errors` | Archiviert seit 2021 | stdlib `errors` + `fmt.Errorf("%w")` |
| `interface{}` | 23 Vorkommen | `any` (Go 1.18+) |
| `strings.Replace(x, y, z, -1)` | mehrfach | `strings.ReplaceAll` |
| `strings.Title` | `:1078` | `x/text/cases` |
| `sort.Slice` | `news.go:119` | `slices.SortFunc` |
| Handler-Duplikation | `stats.go` | Generics über `PlayerStats`/`PlayerStatsProfile` |
| Logging | `log.Printf` überall | `log/slog` mit strukturierten Feldern |
| Konfiguration | 20× handgeschriebene `os.Getenv`-Blöcke in `config.go` | `env`-Tags + Reflection, oder Viper/koanf |
| `fmt.Sscanf` für ints | `config.go:116, 125` | `strconv.Atoi` — `Sscanf` schluckt hier Fehler |
| Deutsche Kommentare + Emojis | `player_stats.go:104, 251, 278` | Projekt-Konvention ist Englisch |

**Tests:** Es existiert genau eine Testdatei (`seasonmap/seasonmap_test.go`, 44 Zeilen) für ~3.100 LOC. Es gibt **keinen einzigen Test** für den Parser (`player_stats.go`, 1.084 Zeilen) — ausgerechnet der fragilste Teil. Empfehlung: HTML-Fixtures unter `testdata/` ablegen und `parseGeneralInfo`/`parseHeroStats` dagegen testen. Das hätte alle Panics aus P0-10 gefunden.

**CI (`.github/workflows/go.yml`):**
- Kein `go test`, kein `go vet`, kein Linter, kein `govulncheck`
- `actions/setup-go@v4` → v5
- Kein Modul-Cache → jeder Build lädt alle Deps neu
- Der Scraper (`./cmd/scraper`) wird gar nicht gebaut

**Docker:**
- `version: '3.8'` in `docker-compose.yml` ist obsolet (Compose V2 warnt)
- `alpine:3.19` ist EOL → `alpine:3.22`
- Redis-Port 6379 ist nach außen gemappt, ohne Passwort — im selben Compose-Netz unnötig. `ports` streichen.
- `USER nobody`, Multi-Stage, `-ldflags="-s -w"`, Healthchecks: alles korrekt gemacht 👍
- Keine Buildx-Cache-Mounts (`--mount=type=cache`) → langsame Rebuilds

---

## 5. Was gut ist

- Atomares Schreiben via Temp-Datei + Rename (`persist.go`)
- Sauberes Mutex-Handling in `NewsService`/`SeasonResetsService`, inkl. Rollback bei Save-Fehler
- Kopien statt Slices nach außen (`GetNews`, `Get`) — kein Aliasing-Race
- XSS-Escaping im Admin-Frontend (`escapeHtml`) korrekt umgesetzt
- Admin-Passwort wird nie im Browser gespeichert (kein localStorage)
- Legacy-Migration von `./news.json` nach `data/` sauber gelöst
- Graceful Shutdown im Scraper, Rate-Limiting (2 s Delay) gegen Blizzard
- `go vet` läuft komplett sauber durch

---

## Empfohlene Reihenfolge

1. **P0-1** Dependency-/Toolchain-Update (`go get`, ~15 Min, 32 CVEs weg)
2. **P0-10** Panic-Guards in `player_stats.go` + Recover in Goroutines
3. **P0-21** Cache-first umbauen — größter Performance-Hebel
4. **P0-2/3** Constant-Time-Vergleich, Rate-Limit, Input-Validierung
5. **P0-11/12** Context-Propagierung statt Goroutine-Wrapper, `errors.Is`
6. **P1** Server-Timeouts, `SCAN` statt `KEYS`, Body-Limits, Graceful Shutdown
7. **P2** Refactoring, Tests, CI-Ausbau

Punkte 1–3 sind unabhängig voneinander und liefern zusammen den größten Effekt.
