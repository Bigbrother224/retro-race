# Audit de code — Retro Race (2026-08-15)

Audit honnête, sans complaisance, de l'état du dépôt. Objectif : dire ce qui est faible,
pourquoi, et ce qui mérite correction ou suppression — sans surpromettre. Chaque finding
est sourcé (fichier:ligne). "Vert" = ce qui est réellement solide, "Rouge" = ce qui détruit
la confiance, "Ambre" = ce qui coûte sans valeur claire.

## Verdict court

Le code n'est pas lent (du 8/16-bit, ça tourne). Le problème n'est pas la performance,
c'est la **structure et la crédibilité** : deux implémentations de la même fonctionnalité
principale, des chemins absolus propres à la machine de l'auteur, des saves jetables dans
`/tmp`, et du scope non demandé (Joust, bot vision). Rien ici ne peut être poussé, branché
sur un autre Mac, ou montré sans honte. Ce qui tient tient par itération rapide, pas par
conception.

## Vert — ce qui est réellement bon

- **Netcode delay-based** (`internal/netplay`) : RTT auto-tuning, ping loop, hooks de
  prédiction/rollback, relay. Bien documenté, conçu proprement. Vérifié en exécution :
  host/guest en sync exacte, rollback qui corrige sous latence, framebuffers identiques
  pixel-par-pixel.
- **Rollback câblé dans le chemin produit** (`internal/app/netlobby.go` : `netRb.Commit`,
  `TakeCorrection`), pas seulement dans simulate.
- **Arbiter** (`internal/arbiter`) : machine à états pure (transition→settle), branchée
  (app.go:569), 5 tests ciblés.
- **Replay déterministe** (`internal/replay`) : n'enregistre que les changements d'inputs,
  rejouable via core. Simple, juste.
- **Canal mémoire partagée** (`internal/shm`) : seqlock, layout fixe, producteur/consommateur
  séparés. Correct.
- **Rival headless** (`internal/rival`) : rejoue un run enregistré (déterministe) et publie
  framebuffer/progression via shm. C'est le bon outil pour l'async "Beat this Ghost".

## Rouge — ce qui détruit la confiance

### R1. Deux implémentations netplay parallèles
`NetplayApp` (768 lignes, `internal/app/netplay.go`, lancé par `--fakeopp/--host/--join/--netbot`)
ET l'intégration dans `App` (478 lignes, `internal/app/netlobby.go`, lancée par le menu). La
logique de stepping du core est copiée-collée dans 3 endroits (netlobby.go, netplay.go,
cmd/simulate). Un commentaire dit textuellement "Mirrors NetplayApp" — la duplication est
consciente. Deux chemins, deux façons de diverger, deux bugs pour le prix d'un. Pas de source
de vérité unique sur la fonctionnalité principale.

### R2. 17 chemins absolus `/Users/mac/retro-race` hardcodés
Dont dans le binaire produit (cmd/retrorace/main.go:25-26) et dans les tests
(internal/library/library_test.go:56, internal/aiagent/loop_test.go:26-27). "Produit
cross-platform" qui ne peut tourner que sur la machine de l'auteur. Tout autre clone : tests
cassés, app pointée vers des dossiers inexistants.

### R3. Saves de jeu dans `/tmp`
`g_save_dir = "/tmp"` dans le shim C (internal/engine/libretro_shim.c). Les saves sont
effacées au reboot, non persistées dans les données utilisateur. Contredit la promesse "tu
gardes tes jeux rétro".

### R4. God object `App`
`internal/app/app.go` : struct à 42 champs, 9 méthodes update/draw, 841 lignes, un seul
mutable partagé pour titre/profil/console/sélection/netlobby/jeu/test-manettes. Plus 478
(netlobby.go) + 768 (netplay.go) de couche app. Rien n'est séparé par écran. Ça tient à 800
lignes, ça ne tiendra pas à 3000.

## Ambre — coûte sans valeur claire

### A1. Mode Joust
Deux humains qui se battent pour un seul personnage en swap de port toutes les 4 s
(internal/app/input.go:172, internal/app/race.go:361). Sans rapport avec "comparer qui finit
le segment en premier" (la vision produit). Feature creep non demandé, chemin entier de
complexité.

### A2. Bot vision `aiagent`/`aibot`
Envoie la framebuffer à une API vision toutes les 30 frames (internal/aiagent) pour simuler
un adversaire, alors que `rival.go` fait le même boulot gratuitement avec des inputs
déterministes. Aspirateur à budget API, redondant avec l'existant. Utile uniquement comme
outil CLI de debug de l'équipe, jamais dans le chemin produit ni le budget API produit.

### A3. Shim C naïf
- `g_snapshot[1024*1024*4]` = 4 Mo alloués statiquement par processus, même pour un jeu
  256×240 (internal/engine/libretro_shim.c).
- Framebuffer copiée 2× en C (video_cb → buffer statique, puis rr_snapshot → Go) + conversion
  raw→rgba en Go, à chaque frame. "Assez rapide", pas "optimisé".
- `g_tmp_game_path[64]` : 64 octets pour un chemin temporaire (fragile).

### A4. Double hash par ROM
`internal/library/library.go` calcule md5 **et** sha256 pour chaque jeu. md5 est redondant
et obsolète ; deux passes de hash sur chaque ROM pour rien.

### A5. Sur-documentation
64 Ko de doc vision (RETRO-RACE.md 29 Ko + ROADMAP.md 35 Ko) pour ~8 700 lignes de code.
Théorise la classification IA d'écrans, les Game Profiles, les formats de compétition, le
matchmaking — pendant que le code a à peine une course. Planning theater : la doc est le seul
endroit où le produit est "fini".

### A6. Churn de design dans l'historique git
Tint bleu ajouté puis retiré, fin redesignée, HUD nettoyé 3×, layout duel redessiné. Le
design n'est pas arrêté, il est re-litigé à chaque commit — la cause mécanique du "tourner
en rond".

## Plan de correction priorisé (honnête, pas de promesse en l'air)

Par ordre de valeur, pas par envie :

1. **Supprimer `NetplayApp`** et ne garder qu'un seul chemin netplay (le menu). ~768 lignes
   de duplication en moins, une ambiguïté de moins. Risque faible : les deux font la même
   chose.
2. **Tuer le Joust.** Complexité en moins, hors vision produit.
3. **Remplacer les chemins hardcodés** par les valeurs de `paths.go` (résolution bundle déjà
   faite) — `main.go`, `simulate`, `aibot`, tests. Rend le repo clonable ailleurs.
4. **`save_dir` → données utilisateur** (étendre ce que `paths.go` fait déjà aux saves).
5. **Découper `App`** en un struct par écran. Grosse refactor : à faire **après** avoir
   bouclé et testé la course, pas avant.
6. **`aiagent`** : garder seulement comme outil CLI, hors build produit.
7. **md5** : ne garder que sha256.

## Ce que cet audit ne promet pas

- Pas de "ça va devenir parfait/optimisé". Objectif : que chaque chose ait **une seule**
  façon d'être faite, que le repo soit **portable**, et qu'il n'y ait **pas de code mort
  étiqueté feature**.
