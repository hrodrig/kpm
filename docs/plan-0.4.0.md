# Plan 0.4.0 — Notifications (thresholds · multi-sink)

<a id="top"></a>

Band plan for **0.4.x Notifications** (SPEC §6 / P3). Roadmap IDs: `#12`–`#15`.
Parent: [ROADMAP.md](./ROADMAP.md). Contract: [SPECIFICATIONS.md](../SPECIFICATIONS.md).

**Depends on:** Core sample daemon (`interval_seconds > 0`) + store records from
each tick. Insights optional (notify does not require recommend/TUI).

**Goal:** Sustained util thresholds → fan-out alerts (gghstats-school sinks)
without mutating the cluster. Core sample path stays unaware of notify.

**Checklist convention:** `[x]` done · `[ ]` remaining — **do not delete** rows
(history + validation).

---

## Band checklist (P3)

### Docs / planning

- [x] `docs/plan-0.4.0.md` (this file)
- [x] ROADMAP Current focus → 0.4.x Notifications
- [x] ROADMAP `#14` wording → multi-sink (slack + webhook presets + smtp)
- [x] SPEC §6.3 multi-sink normative wording in tree
- [x] `configs/kpm.sample.yml` sinks[] examples
- [x] man `kpm.1` notify-test + notifications
- [x] CHANGELOG `[Unreleased]` Notify section

### `#12` Rule engine

- [x] Hold: metric ≥ threshold continuously for `hold_for_seconds` → fire once
- [x] Repeat: while true, re-notify every `repeat_seconds` (`0` = no repeat)
- [x] Resolve: false for `clear_after_seconds` (= hold default); webhook if `notify_on_resolve`
- [x] Dedupe key: `rule + namespace + pod + container + metric`
- [x] In-memory state only (restart = re-hold)
- [x] Evaluate from `sample.Result.Records` after each daemon tick (no 2nd Metrics API)
- [x] Unit tests + fake clock

### `#13` Built-in defaults

- [x] Empty/omitted `rules` + `enabled: true` → CPU + mem util vs request @ **0.80**
- [x] Startup log once (English): built-in defaults message
- [x] Non-empty `rules` → user rules only (no merge with built-ins)
- [x] Defaults: `hold_for_seconds=600`, `repeat_seconds=3600`, `notify_on_resolve=true`

### `#14` Multi-sink (gghstats-school)

- [x] Config `notifications.sinks[]` (replace solo `webhook:` sketch)
- [x] `slack` — Incoming Webhook `webhook_url_env`
- [x] `webhook` — `url_env`, optional `headers_env`, `body`: `generic` | `discord` | `teams`
- [x] `smtp` — host/port/user/password/from/to `*_env`; STARTTLS (587) / `use_tls` (465)
- [x] Fan-out all sinks; continue on per-sink failure; aggregate errors
- [x] Fail-closed: enabled + missing env for any sink → config/start error
- [x] enabled + empty sinks → config error (not silent)
- [x] PagerDuty / n8n: document as `type: webhook` + `body: generic` (no native type)

### `#15` CLI

- [x] Daemon wire in `sample` CLI `onTick` only (`interval_seconds > 0`)
- [x] One-shot sample: **no** notify
- [x] `kpm notify-test` — synthetic firing → FanOut all sinks

### Gates / band exit

- [x] `make lint` + `make cover-check` ≥ 80%
- [ ] SPEC §6 → mark Normative in status banner (draft bump polish)
- [ ] PR `feat/0.4-notify` → `develop` merged
- [ ] Cut `v0.4.0` (separate chore; operador OK)

[↑ Back to top](#top)

---

## Layering

```
internal/notify/  — rules, metric, engine FSM, sinks, FanOut
internal/cli/     — sample onTick wire + notify-test
internal/sample/  — unchanged (no import of notify)
```

| Rule | Why |
|------|-----|
| **Core must not import notify** | Sample→SQLite stays lean |
| **Eval = tick records** | Same capture as SQLite insert |
| **Multi-sink `sinks[]`** | Align gghstats / kwd target contract |

[↑ Back to top](#top)

---

## Merge order

| Order | ID | Deliverable | Status |
|------:|----|-------------|--------|
| 1 | docs | This plan + ROADMAP focus | [x] |
| 2 | `#13`+cfg | `config.Notifications` + validate + sample YAML | [x] |
| 3 | `#12` | Engine hold/repeat/resolve/dedupe | [x] |
| 4 | `#14` | slack + webhook presets + smtp + FanOut | [x] |
| 5 | `#15` | daemon wire + `notify-test` | [x] |
| 6 | docs | SPEC §6 / man / CHANGELOG | [x] |
| 7 | ship | PR → develop; later cut `v0.4.0` | [ ] |

**Suggested PRs:** single `feat/0.4-notify` (plan + code) → develop.

[↑ Back to top](#top)

---

## Sinks (locked)

| Type | Payload |
|------|---------|
| `slack` | `{"text":...}` Incoming Webhook |
| `webhook` | JSON POST; `body` preset `generic` (default) \| `discord` \| `teams` |
| `smtp` | Email via SMTP (`*_env` secrets) |

**Not first-class:** `pagerduty`, native `discord`/`teams`, `loki` (defer).
PagerDuty → generic webhook to Events API / automation.

[↑ Back to top](#top)

---

## Success criteria (band done)

- [x] Daemon + `notifications.enabled` fires/repeats/resolves per SPEC §6.1–6.2
- [x] Built-in 80% rules when `rules` empty + startup log
- [x] Fan-out slack + webhook (discord/teams/generic) + smtp; fail-closed env
- [x] `kpm notify-test` hits all configured sinks
- [x] Core sample works with notify disabled / absent
- [x] Cover ≥ 80%; gocyclo ≤ 14
- [ ] Merged to develop + release cut

## Out of scope

- [ ] Loki sink (later)
- Native pagerduty/discord/teams types (**rejected** — presets/webhook)
- Persist alert state in SQLite
- MCP / server (`0.5`–`0.6`)
- Auto-apply resource patches

[↑ Back to top](#top)
