# Lore UI Design Spec

`lore ui` starts a local web server (default port `7890`, `localhost` only) that is the primary interface for humans. Agents use the CLI; humans use the UI.

---

## Tech Stack

- **Go `net/http` + `html/template`** — server-side rendering, all assets embedded in the binary
- **HTMX** — partial page updates, SSE streaming, form submissions without full reloads
- **Tailwind CSS** — utility-first styling
- No npm, no build step, no separate server process

---

## Themes & Colour Palette

A theme toggle (sun/moon icon) lives in the top-right corner of the sidebar. The preference is persisted in `localStorage`.

### Dark Theme

| Role | Value |
|---|---|
| Page background | `#0f0f11` |
| Sidebar background | `#16161a` |
| Card / surface | `#1c1c21` |
| Border | `#2a2a32` |
| Primary text | `#e8e8f0` |
| Muted text | `#6b6b80` |
| Accent (indigo) | `#6366f1` |
| Accent hover | `#818cf8` |
| Open (blue) | `#3b82f6` |
| Working (amber) | `#f59e0b` |
| Blocked (red) | `#ef4444` |
| Ready for review (purple) | `#a855f7` |
| Done (green) | `#22c55e` |

### Light Theme

The light theme uses a warm off-white base — not pure white — to reduce eye strain while keeping the interface clean and professional.

| Role | Value |
|---|---|
| Page background | `#f5f5f7` |
| Sidebar background | `#ffffff` |
| Card / surface | `#ffffff` |
| Border | `#e2e2e8` |
| Primary text | `#18181b` |
| Muted text | `#71717a` |
| Accent (indigo) | `#4f46e5` |
| Accent hover | `#4338ca` |
| Open (blue) | `#2563eb` |
| Working (amber) | `#d97706` |
| Blocked (red) | `#dc2626` |
| Ready for review (purple) | `#9333ea` |
| Done (green) | `#16a34a` |

Status badges and accent colours shift slightly between themes to maintain contrast ratios. All interactive elements meet WCAG AA contrast in both themes.

---

## Typography

- **UI font**: system font stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`)
- **Monospace**: `"JetBrains Mono", "Fira Code", ui-monospace, monospace` — used for ticket IDs, event log, code snippets
- Base size: `14px`. Thread entry body: `15px`. Ticket ID headings: `18px` monospace.

---

## Global Chrome

Narrow left sidebar, always visible:

- Lore wordmark at the top (indigo accent on the "L")
- Nav links: **Dashboard**, **Tickets**, **New Ticket**, **Search**, **Event Log**
- Active link has an indigo left border and slightly brighter text
- Bottom of sidebar: repo name + current branch in muted monospace
- Top-right corner of sidebar: light/dark theme toggle (sun/moon icon)

---

## Dashboard (landing page)

The default landing page. Three vertical columns side by side, each with a coloured accent header.

### Needs Answer (amber)

Each card shows:
- Ticket ID in monospace + truncated description
- The question text in a callout block with an amber left border
- Author chip + relative timestamp ("asked 4 min ago")
- A one-line text input + **Answer** button inline at the bottom of the card — no page navigation, HTMX swaps the card with a confirmation state

### Ready for Review (purple)

Each card shows:
- Ticket ID + description
- Agent who submitted + how long it has been waiting
- **Approve** (green) and **Reject** (red) buttons
- Clicking Reject expands an inline reason field before submitting

### Blocked (red)

Each card shows:
- Ticket ID + description
- Block reason in a muted callout
- **Unblock** button with an optional note field that expands inline

Each column shows an empty state with a faint dashed border and "Nothing here" when there is no work — not blank space.

---

## Ticket List

Full-width table with status filter tabs across the top:

**All · Open · Working · Blocked · Ready · Done**

Clicking a tab does an HTMX partial swap — no full page reload.

Each row:
- Status badge (coloured pill)
- Ticket ID in monospace
- Description (truncated to one line)
- Assigned agent (or "—")
- Age (relative: "2h ago")

Clicking anywhere on a row navigates to the ticket detail view.

---

## Ticket Detail

### Header Block

- Ticket ID in large monospace, status badge, agent chip, created timestamp
- Action buttons top-right, context-sensitive:
  - `ready-for-review` → **Approve** + **Reject**
  - `blocked` → **Unblock**
  - Always: **Add Update**, **Attach Image**

### Thread

Chronological list of entries. Each kind is visually distinct:

| Kind | Style |
|---|---|
| `update` | Plain left-aligned text block. Author + timestamp in muted text above. |
| `question` | Amber left border, slightly inset, question mark icon. **OPEN** or **ANSWERED** badge top-right. |
| `answer` | Green left border, visually indented under the question it answers. |
| `comment` | Same as update but with a speech bubble icon. |
| `image` | Full-width inline image render. Caption below in muted italic. |
| Approval | Full-width green banner: "Approved by `<reviewer>`" + timestamp. |
| Rejection | Full-width red banner: "Rejected by `<reviewer>`: `<reason>`" + timestamp. |

### Action Bar

Pinned to the bottom of the viewport. Tab strip:

**Add Update · Ask Question · Attach Image**

Switching tabs morphs the input area. Submit posts via HTMX — new entry animates into the thread without a page reload.

---

## New Ticket

Centred single-column form:

- Large auto-growing textarea for the free-form description
- Drag-and-drop image attachment zone below
- **Create Ticket** button
- On success, redirects to the new ticket's detail page

---

## Event Log

Terminal aesthetic. Monospace font. Events stream in via SSE, newest at the bottom with auto-scroll. Each line syntax-highlighted by type:

| Event group | Colour |
|---|---|
| `ticket.*` | Indigo |
| `question.*` | Amber |
| `lore.*` (system) | Muted gray |

A **Pause** toggle stops auto-scroll while you read. Resuming jumps back to the latest event.

---

## Search

Search bar centred and prominent. Results appear as you type (debounced, 300ms). Each result is a card:

- Ticket ID + status badge
- Matched snippet with the query term highlighted
- Click → ticket detail

---

## Routes

| Route | Description |
|---|---|
| `GET /` | Dashboard |
| `GET /tickets` | Ticket list (`?status=` filter) |
| `GET /tickets/{id}` | Ticket detail |
| `GET /tickets/new` | New ticket form |
| `POST /tickets/new` | Create ticket |
| `POST /tickets/{id}/approve` | Approve ticket |
| `POST /tickets/{id}/reject` | Reject ticket |
| `POST /tickets/{id}/unblock` | Unblock ticket |
| `POST /tickets/{id}/answer` | Answer a question |
| `POST /tickets/{id}/update` | Add update or comment |
| `POST /tickets/{id}/image` | Attach image to thread |
| `GET /search?q=` | Search results |
| `GET /events` | SSE event stream |
