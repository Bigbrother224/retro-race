# Retro Race — Single Source of Truth

> Everything about the project in one file. Vision, decisions, architecture, glossary, roadmap, risks, and the message to send to the engineer friend.

> **Statut (2026-08-15) :** ce document est la **vision et le plan** du produit. La **MVP réellement implémentée** est la course netplay 2 joueurs (menu arcade → lobby → partie partagée → arbiter → fin dramatique) — voir `docs/adr/0004-mvp-scope-and-done-line.md`. Tout le reste (classements, rating, matchmaking, formats de compétition, comptes, Ghost in-game) est une vision **non implémentée**, pas une réalité du code.

---

## Vision

Bring the retro gaming community back to life by turning local emulation into a modern social experience: launch a game in seconds, find your friends, race a stranger, see where your opponent is in the same level, climb trustworthy leaderboards, and join community events.

The promise is not to remake the games. The promise is:

> **You keep your retro game, but you never play alone again.**

The product must be light like a retro tool, immediate like a console, and deep like a competitive community.

## What Retro Race is (resolved)

Retro Race **is a light emulator frontend**: it embeds mature libretro cores (C libraries) inside a small native app — it is **not** a wrapper that depends on RetroArch being installed. "Not writing an emulator" means: we reuse mature cores instead of writing CPU/APU/graphics emulation from scratch. Retro Race owns the window and the framebuffer. This was the source of a past ambiguity ("layer beside the emulator" vs "embedded frontend"); the embedded frontend is the decision, and a RetroArch adapter (BSV + UDP NCI) is only a fallback backend for games where an embedded core is not satisfactory.

## Positioning

Not a new general-purpose emulator, not a ROM library, not a streaming service. It is a **local-first competition and community layer** on top of compatible emulation runtimes.

The core experience is a **parallel Race**:

1. Each player owns and runs their content locally.
2. The client syncs a shared start state and a countdown.
3. Players play the same segment in parallel, without modifying the game's logic.
4. The opponent's inputs are relayed and replayed in a second local instance.
5. Each player sees the opponent's screen as a small live PiP window, plus a progress gauge.
6. The first valid victory event wins the segment.
7. Results feed history, rating, leaderboards and rivalries.

## Why now

The pieces already exist separately: RetroArch provides netplay, RetroAchievements provides achievements and some ranking data, TASVideos proves input-replay viability, Speedrun.com structures communities, and Fightcade shows a specialized netplay can last. What's missing is a unified, simple, modern experience centered on **live opponent views, segment races, and player discovery**.

### Competitive landscape

| Product / project | Strength | Main gap for Retro Race |
| --- | --- | --- |
| RetroArch | Cores, netplay, spectators | Technical UX, no guided competition with live opponent view |
| RetroAchievements | Achievements, profiles, community, game data | No parallel Race or live opponent view |
| Speedrun.com | Mature community rules and leaderboards | No local launcher or built-in duels |
| Fightcade | Very strong arcade netplay | Narrow scope, no per-segment campaigns or general race layer |
| TASVideos / BizHawk | Input replays and reproducibility | Expert tooling, no casual matchmaking |
| Parsec / Steam Remote Play | Simple remote play | Generic streaming, no race, arbitration or leaderboard |
| Antstream Arcade | Licensed catalog and challenges | Closed cloud model, not local-first |

Differentiation is not "we support more consoles." It is the quality of the loop: **choose, start, see, beat, retry, share**.

## Decisions taken

### 1. It is a parallel Race, not co-op

Everyone plays the same segment locally. Each player sees the opponent's progress via a PiP window and a gauge, without altering the game's logic. The first valid victory event wins.

### 2. The opponent is visible, not injected

During a Race, each player sees a **small live window of the opponent's screen** (picture-in-picture) plus a **progress gauge** (who is ahead). Nothing is injected into the game itself: no sprite, no ghost, no modification of the framebuffer. This is simpler and more universal than an in-game Ghost.

### 3. Local-first: inputs only, never video or ROMs

The network carries inputs, events and metadata — never a persistent video stream and never ROM files.

### 4. The light architecture (the big change)

Replaces the previous AI plan. **No real-time AI, no screen capture, no manual RAM profiling.**

- **Embed libretro cores** in a mini-frontend (the cores are mature C libraries — this is not writing an emulator). Each player process runs its own core locally; a silent second process can replay inputs deterministically when needed (replay, verification).
- **The framebuffer is the UI**: the libretro video callback exposes the raw framebuffer — that is what the player sees and what is shared (small, low-fps copy) for the picture-in-picture window. Zero segmentation, zero sprite sheets.
- **Victory/end detection by screen change**: a big persistent framebuffer change (level clear, game over) signals the end of a segment — no per-game memory addresses needed.

Real cost:

- one embedded 8/16-bit core per player ≈ a few % CPU on Apple Silicon;
- a small PiP frame copy per second ≈ negligible bandwidth;
- a lighter app than a full RetroArch, no Electron.

No per-game calibration required: position-based ghost rendering is gone. End detection uses screen changes, which works for every game out of the box.

### 5. Cross-platform: macOS + Windows from day one

Development happens on macOS; both are supported from the start, Linux later.

### 6. Games: 8/16-bit first

NES, SNES, Game Boy, Game Boy Color, Genesis, and some deterministic GBA titles. PS1 and heavier systems come later; PS2 is out of the roadmap.

### 7. How the opponent is shown (simplified, wow first)

- **Level 1 (main)**: picture-in-picture live window of the opponent's screen + progress gauge. Simple, universal, works for every game.
- **Level 2 (upgrade)**: the PiP window gets a "replay the last 10 seconds" button and the end-of-race slow-motion dramatization (slow-mo + name + time + side-by-side replay of the last seconds).
- **Level 3 (later, optional)**: any in-game representation (ghost, trajectory) — explicitly NOT part of the MVP; only revisit if PiP proves not exciting enough.

### 8. ROMs and BIOS stay local

The client scans a local file, computes its hash and maps it to a canonical identifier. The file, BIOS and game assets are never provided to or sent to the server. This is not a legal opinion on its own: specialist review is required before public distribution.

### 9. Honest leaderboards and integrity

Separate leaderboards: **Fastest** (best frame time), **Head-to-head** (wins, losses, rating), **Consistency**, **Community** (participation), **Global profile** (playful progression, never presented as an objective skill measure across games).

Integrity levels: **Casual** (shareable, no guarantees), **Replayed** (replayed without divergence), **Verified** (respects Game Profile rules, recognized content/core), **Reviewed** (community/procedure-checked for important results). A server that does not hold the ROM cannot promise cryptographic inviolability — keep the wording honest.

Pragmatic anti-cheat: lock speed, pause, rewind, frame-advance and savestates for strict categories; content/core/version hashes in every Run; signed local replay with block checksums; divergence and input-gap detection; no silent deletion — contested results use a `contested` state.

### 10. Competition formats

- **Live 1v1 duel** (flagship): simultaneous start, live PiP of the opponent + progress gauge, first valid end event wins.
- **Async challenge** (retention): challenge an existing time, the opponent's run is replayed (deterministic) to compare, publish even if the rival is offline.
- **Segment races**: ordered segments; one point per win; `first to 3`, `full run`, `instant rematch`.
- **Random matchmaking**: a queue per game/category and a `Surprise Me` queue limited to locally owned compatible content; to avoid infinite waits, propose a list of three compatible games instead of forcing an unexpected one.
- **Tournaments & seasons**: weekly cup on one game, monthly multi-game event, season with divisions, community challenge against a reference run, anniversary race around a console or series. Seasons reward participation and progression, not only top players.

### 11. Social, not voice

MVP has no built-in voice. Invitations, short reactions, blocking, and optional Discord integration are enough to start. Voice adds cost, moderation and sensitive data without improving the Ghost itself.

### 12. Business model

Options: free client with a limited community service, subscription for advanced events/storage, or a self-hostable service. Provisional recommendation: do not lock the local core or friends play; monetize later the social cloud, organized seasons or creator tools — never a paid competitive advantage.

## Architecture

### Client (native desktop)

- lightweight native launcher (Swift/SwiftUI), macOS and Windows;
- **embedded libretro mini-frontend** (cores are mature C libraries);
- one process per player running the core, rendering the framebuffer via the video callback;
- a small live PiP window of the opponent's screen (low-fps framebuffer copy) + a progress gauge;
- unified input manager (keyboard + gamepad, merged into one state);
- recorder/replayer reusing native core formats first;
- Game Profile manager (per-game: hashes, core, start state, end-of-segment rules);
- local cache of metadata and replays;
- signed client and core updates;
- **alternate backend**: RetroArch adapter (BSV + UDP Network Control Interface) for games where an embedded core is not satisfactory.

The architecture keeps a clear boundary so the embedded core can be swapped for RetroArch, BizHawk or Mesen without changing the Race model, and the end detection can be swapped without touching the rest.

### Runtime shape during a Race

```text
Player process A (you)
   ├─ libretro core A: your game, visible, your controller
   ├─ libretro video callback → framebuffer A (no screen capture)
   └─ shows framebuffer B (opponent) as a small live PiP window + gauge

Player process B (opponent)
   ├─ libretro core B: same ROM, same core, same start state
   ├─ runs B's inputs (live or replayed deterministically)
   └─ sends a low-fps copy of its framebuffer for the PiP window
```

The gameplay of A never depends on B's state; the PiP window can lag or drop frames without making the game unplayable.

### Determinism requirements

- same content hash;
- same region and revision;
- same core and version;
- same deterministic options;
- same start state;
- same logical cadence;
- no detected divergence.

### Backend services

- identity, pseudonym, session;
- friends, invitations, blocking, minimal presence;
- matchmaking and lobbies;
- regional relay of small input packets;
- start clock and ephemeral Race state;
- storage of replays, results, published Ghosts and metadata;
- leaderboards and ratings;
- Game Profile catalog;
- events, seasons, notifications;
- reporting, moderation, audit.

The server is never the required path for game rendering. If it goes down, local play and local replays keep working.

### Data retained

User id and pseudonym; friendships, blocks, minimal presence; game hashes and identifiers (never ROMs); Run/replay/result/verification status; rendering and control preferences; messages and reports per a clear retention policy.

## Lightness and performance targets

To validate on a recent low/mid-range machine:

- first interactive screen in under 3 seconds after launch;
- in-game in under 10 seconds after selecting already-recognized content;
- no remote video required for a Race;
- duel network limited to inputs and events, typically tens of KB per minute (excluding presence and chat);
- at most two emulation processes per Ghost;
- clean Ghost shutdown if the machine is too slow, with `Echo` or deferred Ghost as fallback;
- stable rendering at the core's native cadence without slowing the main game.

Exact budgets are measured at the spike. Do not promise uniform PS1/PS2 support before real measurement.

## Legal and security

### ROMs and BIOS

The product never downloads, stores, redistributes or links to any protected ROM or BIOS. The server receives only a hash and metadata. The client must clearly state that the user must hold the necessary rights under their jurisdiction. This architecture reduces exposure but does not guarantee legal immunity. Have a competent lawyer review the Terms of Use, the import flow, the core licenses and the distribution rules.

### User content

Replays, names, avatars, comments and images are UGC. Plan from beta: blocking, reporting, removal, moderation log, anti-spam, invitation control, account deletion.

### Client security

- run cores without elevated privileges;
- isolate the emulation process and limit its network/file access;
- sign binaries and updates;
- refuse arbitrary cores or plugins in Verified mode;
- treat every ROM, replay and external profile as untrusted;
- never run a profile script outside a strict sandbox.

### Privacy

Presence, current game, relationships, possible voice and history are personal data. MVP avoids voice and open private messages. Start with pseudonym, accepted friends, blocking, configurable visibility, data export/deletion.

## Game Profiles and the Ghost

A **Game Profile** is a versioned, testable, separately-publishable artifact. For the MVP it must stay as small as possible. It contains:

- game id and accepted hashes;
- regions and revisions;
- compatible core and version;
- start state and reset method;
- segments and order;
- start/checkpoint/victory detection;
- time limits and allowed pauses;
- Ghost Profile;
- HUD masks and composition rules;
- leaderboard rules;
- known limitations and reproducibility tests.

The **Ghost Profile** is the part describing how to locate and draw the opponent: position addresses (from automatic calibration), frame offsets, and composition rule.

The community can propose profiles, but only maintained and tested profiles feed Verified leaderboards.

## Community strategy

### Retention loop

```text
I play → I see a Ghost → I barely lose → I ask for a rematch
→ I beat the Ghost → I publish the Run → another player challenges me
```

Every screen must offer a next action: rematch, beat the Ghost, follow the rival, join the event, or share the result.

### Community mechanics

- rival of the day;
- Ghost of the week;
- win streaks;
- rating divisions;
- progression badges, no pay-to-win;
- clubs by console, series or country;
- scheduled events;
- player discovery with close times, not just top players;
- light spectator mode and shareable replays;
- votes to propose the next event game;
- game page with rules and Game Profile contributors.

The feed should favor playable actions (`Beat this Ghost`) over passive likes.

### What can make the product wow

- arrival in the level with your friend's Ghost already visible;
- rival colors, name and status readable without hiding the game;
- discrete time splits and end-of-race dramatization;
- automatic replay of the last ten seconds with both Ghosts;
- shareable result image;
- instant rematch without reconfiguring the game;
- matchmaking that finds a comparable opponent;
- historical Ghosts: best friend, community record, personal record;
- weekly themed events;
- **post-race AI coach** (deferred analysis, no runtime cost): explains in one sentence where you lost time ("you lose 0.8 s on the first jump because you jump 3 frames late").

## Roadmap

Durations are orders of magnitude for a small experienced team. They are not a schedule promise; the exit condition is the real criterion.

### Phase 0 — Cross-platform feasibility spike, 1–2 weeks

Goal: prove the Ghost works on the team's Mac before building the social layer.

Deliverables:

- a legally distributable demo/homebrew;
- one libretro core embedded in a minimal macOS frontend (video callback → framebuffer);
- two processes, one silent Ghost (video callback NULL);
- identical start state;
- determinism measurement over hundreds of thousands of frames;
- automatic calibration prototype (save-state diff → character position);
- transparent click-through overlay aligned on the game window;
- CPU, memory and latency measurement on a modest machine.

Go/no-go: main game stays stable, Ghost stays readable, calibration yields a reliable position. If calibration fails, test the core's sprite planes or the difference mask (`Echo`), then only consider an external RetroArch adapter. Do not write an emulator.

### Phase 1 — Local Ghost Lab, 3–6 weeks

Goal: a complete loop with no account and no network.

Deliverables:

- macOS + Windows desktop launcher with embedded libretro mini-frontend;
- silent Ghost process and shared memory for position + sprite;
- local library and hash import;
- automatic per-game calibration (versioned JSON);
- simulated local Race with two emulation processes;
- Ghost with color, opacity, name and trail;
- pause, reset, forfeit, segment end (Run events + replay);
- exportable and replayable replay;
- `Echo` fallback (difference mask) to test coverage.

Exit: someone unfamiliar with the architecture understands the race in under 30 seconds and can relaunch it without a technical menu.

### Phase 2 — Private friend Race, 3–6 weeks

Goal: actually play with a friend remotely.

Deliverables: minimal pseudonym account; code invitation; private lobby; hash/backend/options compatibility; input relay; common countdown and start frame; reconnection and clean forfeit; result and rematch; private Race history.

Exit: two players on different networks finish a Race with no PIP, no local slowdown, and a readable Ghost.

### Phase 3 — Async challenges and profiles, 3–5 weeks

Goal: make the product interesting even when no friend is online.

Deliverables: publish a Ghost/Run; `Beat this Ghost`; personal best; rivalry history; profiles and friends; minimal notifications; result sharing; 3–5 Game Profiles total.

Exit: a player spontaneously returns to beat a Run without a prior appointment.

### Phase 4 — Reliable leaderboards, 4–8 weeks

Goal: build credible competition without claiming absolute anti-cheat.

Deliverables: leaderboards per game/segment/category; Casual, Replayed and Verified statuses; visible rules before playing; replay validation and divergence detection; head-to-head ratings; contestation, review and correction history; public Game Community page.

Exit: a player understands why a result is ranked, and the community can flag a contested result without breaking overall trust.

### Phase 5 — Random matchmaking and first season, 4–8 weeks

Goal: move from a friends' tool to a living community.

Deliverables: queue per game and category; `Surprise Me` limited to compatible local content; rating and search range; abandonment protection and reconnection; season, divisions and weekly events; activity, rivals and recommendations; full user moderation.

Exit: a user can launch the app alone and find playable activity without Discord or prior knowledge.

### Phase 6 — Controlled expansion

Goal: increase depth without sacrificing lightness.

Recommended order:

1. more NES/SNES/Genesis/GB profiles;
2. second emulator backend if the first doesn't cover a console;
3. selected deterministic GBA titles;
4. deferred/Echo Ghost on PS1;
5. macOS/Linux support;
6. audited community profiles;
7. only then, heavier embedded renderers or heavier systems.

Do not add a console because it is popular. Add it when its cost, reproducibility and Ghost quality are mastered.

## Success metrics

### Product

- time to first Race;
- detection and launch success rate;
- readable-Ghost rate on supported profiles;
- rematch rate after a Race;
- share of async Runs replayed;
- weekly active players per Game Community;
- number of rival/friend relationships created.

### Technical

- replay divergences per core/profile;
- network drops;
- Ghost CPU overhead;
- idle and in-Race memory;
- relay input latency;
- crash rate per core.

### Community

- % of new players finishing a first Race;
- recurring event participants;
- challenges received/resolved ratio;
- report handling time;
- share of activity from top players vs newcomers.

North-star metric: **Races replayed per active player per week**, complemented by rematch rate. A leaderboard without rematch is just a static page.

## Risks and responses

| Risk | Severity | Response |
| --- | --- | --- |
| Ghost impossible to render generically | Very high | Versioned profiles, `Echo` fallback, small initial catalog |
| Desynchronization | Very high | Common start state, strict hashes, determinism tests, Ghost decoupled from gameplay |
| Compromised client and fake records | High | Trust levels, replays, community review, honest wording |
| Empty catalog at launch | High | 3–5 heavily worked games, async challenges, regular events |
| Configuration too technical | High | Local scan, controller detection, plain-language diagnostics |
| ROM/BIOS copyright risk | Very high | No hosting/distribution, local import, legal review, clear documentation |
| Client too heavy | High | Native, targeted cores, no video, budgets measured from Phase 0 |
| Matchmaking without players | High | Async challenges, reference Ghosts, unranked bots only if legally/technically clean |
| Toxicity and spam | High | Pseudonyms, blocking, reporting, private invitations by default |
| Too many consoles too early | High | Support sheet based on quality, not quantity |

## Open decisions to settle before product build

### Platform

Recommendation: Windows first, with an architecture that doesn't close off macOS/Linux. The PC market and emulation tools are accessible there, and it reduces packaging and support costs.

### First game

Recommendation: choose a deterministic 8/16-bit game with short segments and a clear victory state. The internal prototype can use a homebrew ROM; the first public profile depends on the user's local content and legal review.

### Live vs async

Recommendation: build replay infrastructure from the start, but launch with the private live duel. Async becomes the community safety net once replay is stable.

### Voice and chat

Recommendation: no built-in voice in the MVP. Invitations, short reactions, blocking and optional Discord integration suffice initially.

### Business model

Options: free client with a limited community service, subscription for advanced events/storage, or a self-hostable service. Provisional recommendation: don't lock the local core or friends play; monetize later the social cloud, organized seasons or creator tools, without paid competitive advantage.

## Non-goals of the MVP

- distributing ROMs or BIOS;
- supporting every console;
- PS2;
- mobile play;
- mandatory remote video;
- co-op that changes the game's rules;
- unmoderated public voice;
- a global leaderboard mixing all games;
- allegedly perfect anti-cheat;
- auto-generated profiles without tests.

## Product truth criterion

The project is not validated because it launches an emulator. It is validated when a player says:

> "I lost by two seconds, I saw exactly where my friend passed me, and I immediately relaunched to beat his Ghost."

If this loop isn't fun with three games, adding fifty consoles won't solve anything.

## Glossary (domain language)

### Gameplay

- **Race** — a competition where several players locally run the same segment from a common start and try to finish first. *Avoid: co-op, netplay match, shared session.*
- **Écran partagé (PiP)** — a small live window of the opponent's screen shown during a Race; nothing is injected into the local game. *Avoid: ghost, in-game sprite, framebuffer modification.*
- **Jauge de course** — the progress gauge comparing both players' position in the segment (who is ahead). *Avoid: opponent sprite, in-game position.*
- **Run** — a complete, timestamped execution of a segment, including its start state, inputs, progression events and result. *Avoid: game, session (a replay is the persisted representation of a run).*
- **Segment** — the competition unit chosen by the product, usually a level, race, zone or boss. A game can expose several ordered segments. *Avoid: round (reserved for competition format), level in every case.*

### Systems

- **Race Arbiter** — the component that detects segment-end events (victory, end screen, Game Over) by screen-change detection (a big persistent framebuffer change) and Run events. *Avoid: RAM detection, hardcoded per-game condition, real-time AI.*
- **Game Profile** — a versioned, game-specific definition of accepted ROM hashes, core, start state, victory/end rules and race settings. *Avoid: patch, mod, emulator.*

### Integrity and competition

- **Casual Run** — playable and shareable without strong integrity guarantees; can feed social activity but not a certified competitive leaderboard. *Avoid: valid run.*
- **Verified Run** — a Run respecting a Game Profile, content hash, core and allowed parameters, whose replay replays without known divergence. *Avoid: anti-cheat run (local verification does not make a compromised client trustless).*
- **Leaderboard** — a ranking for a precise game and category, ordered by frame time, duel result or rating depending on type. *Avoid: single global leaderboard.*
- **Rating** — an estimate of a player's strength in duels of a category, distinct from historical best time. *Avoid: score, account level.*
- **Competition** — an organized format aggregating Races or Runs: friends duel, random matchmaking, async challenge, tournament or season. *Avoid: session (technical term for an active connection).*

### Social

- **Friend** — a reciprocal relationship accepted by both users, enabling private invitations and challenges. *Avoid: follower, contact.*
- **Rival** — a followed or frequently-faced player whose progress and runs are highlighted. *Avoid: friend (a rival can be public without reciprocity).*
- **Lobby** — a temporary Race preparation space with participants, game, segment, rules and countdown. *Avoid: game server (the game runs locally).*
- **Game Community** — a public space centered on a game, its categories, events, players and ranking rules. *Avoid: guild (reserved for user-created organizations).*

### Resolved ambiguities

- The product does not turn solo games into co-op. It organizes parallel **Races** where each player sees the other via a **PiP window** and a **progress gauge** — never by injecting anything into the game.
- The server never runs the player's ROM and never receives it. It orchestrates, relays inputs and stores authorized metadata and replays.
- A leaderboard never mixes incompatible content, cores, regions or categories.
- "Verified" means reproducible per Game Profile rules, not inviolable against a compromised client.

### Example dialog

> **Player**: I start a Race on segment 1-1 of my NES Game Profile.
>
> **Client**: Your friend is in the Lobby. The common start happens in 5 seconds.
>
> **Player**: I see my friend's screen in the corner, and the gauge shows I'm slightly ahead.
>
> **Client**: Correct. You each produce a local Run. The first valid end event wins the segment. At the end, both last seconds are replayed side by side.
>
> **Player**: Does my time appear on the Leaderboard?
>
> **Client**: Yes if your Run is a Verified Run. Otherwise it stays a Casual Run visible on your profile and in your friends' activity.

## Message to send to the engineer friend

Salut, je réfléchis à une application desktop cross-platform (je développe sur Mac) pour redonner une vie sociale aux jeux rétro : on choisit sa console puis son jeu, et si un ami est en ligne on joue à ce jeu-là avec lui. Pendant la course, chacun voit une petite fenêtre live de l'écran de l'autre (picture-in-picture) plus une jauge de progression qui montre qui est en tête, et à la fin un replay ralenti des dernières secondes côte à côte. C'est un mini-frontend libretro embarqué (les cores sont des bibliothèques C matures, pas un émulateur écrit maison) : le vidéo callback expose la framebuffer sans capture d'écran, l'input est unifié clavier + manette. On a validé localement le rendu (NES + SNES, 60 fps fluide). Le réseau arrivera après, en ne transportant que des inputs et de petits extraits de framebuffer pour le PiP, pas de la vidéo complète. Est-ce que tu vois une architecture plus simple pour le mode course (voir l'écran de l'autre + jauge + replay de fin), et quel serait le prototype minimal à valider ?

## Key sources to watch

- [RetroArch platforms (macOS/Windows)](https://www.retroarch.com/?page=platforms)
- [Developing libretro cores](https://docs.libretro.com/development/cores/developing-cores/)
- [Libretro Network Control Interface](https://docs.libretro.com/development/retroarch/network-control-interface/)
- [Libretro Netplay documentation](https://docs.libretro.com/development/retroarch/netplay/)
- [RetroArch BSV replays](https://docs.libretro.com/guides/record-play-replay/)
- [RetroAchievements documentation](https://docs.retroachievements.org/)
- [RetroAchievements rcheevos](https://github.com/RetroAchievements/rcheevos)
- [Speedrun.com](https://www.speedrun.com/about)
- [TASVideos emulator resources](https://tasvideos.org/EmulatorResources)
- [Fightcade](https://www.fightcade.com/)
- [MAME legal information](https://www.mamedev.org/legal.html)
- [17 U.S.C. §117](https://www.law.cornell.edu/uscode/text/17/117)
- [17 U.S.C. §1201](https://www.law.cornell.edu/uscode/text/17/1201)
