# Périmètre MVP : une course netplay 2 joueurs complète, pas un prototype étroit

Le projet a dérivé : netplay, relay, rollback, un bot vision et un simulateur ont été
construits en parallèle, sans rien boucler ni commiter. On tourne en rond faute d'une
ligne de fin claire. Ce document fige ce que la MVP est, ce qui est reporté, et la
définition de fin ("done") qui la valide.

La MVP doit être **fun**, pas techniquement étroite. Un MVP qui montre juste deux écrans
côte à côte est une mauvaise MVP : l'excitation vient de la course contre un vrai rival,
pas de la preuve de faisabilité technique.

## Décision — la MVP est la boucle de course netplay complète

Définition de fin ("done") :

> Je lance un jeu depuis le menu arcade, je crée ou je rejoins un salon par code, et je
> race un vrai adversaire distant en simultané : je vois son écran en PiP et la jauge,
> le premier événement de fin valide gagne, et j'obtiens la fin dramatique + revanche.
> Je veux recommencer.

**Inclus (MVP) :**

- Menu arcade + lancement NES/SNES (fait).
- Support 2 ports manette dans le moteur (fait) : `rr_set_button_port`, comptage des
  ports du core, port 1 configuré en joypad.
- Gate d'input 2 joueurs (+ mode Joust) : un seul endroit transforme des sources
  physiques (clavier, manettes, inputs relayés distants) en états de boutons par port.
- Netplay **delay-based** + transport `relay` (contournement NAT par code de salon).
- Vérification de divergence par hash d'état (les deux machines tournent le même jeu).
- Lobby hôte/invité par code, handshake par hash de ROM/core (aucun contenu transféré).
- PiP framebuffer + jauge de course.
- Arbiter : détection de fin de segment par changement d'écran.
- Fin dramatique : ralenti, nom + temps, replay des dernières secondes côte à côte, revanche.
- Résolution de chemins bundle-aware (`paths.go`) pour tourner comme `.app` macOS.

**Outils de dev (gardés, hors produit) :**

- `cmd/aibot` + `internal/aiagent` : bot vision qui joue en regardant l'écran. **Coûteux
  en API (40 appels/partie simulée)** — outil de debug/simulation de l'équipe, jamais
  dans la boucle produit, jamais dans le budget API du produit.
- `cmd/simulate` : simulateur de course.
- `scripts/build-app.sh`.

**Reporté (v2) :**

- Leaderboards, rating, matchmaking, défi asynchrone, formats de compétition, comptes.
- Généralisation à d'autres consoles/core.

## Options rejetées

- MVP sans netplay (PiP + jauge + opposant local seulement) : ennuyeux, ne valide pas la
  promesse "tu ne joues plus jamais seul". Le fun *est* le produit.
- Geler/retirer le bot vision : il reste utile à l'équipe pour simuler et débugger.
  On le garde, mais hors périmètre produit et hors budget API du produit.
- Park rollback derrière un build-tag : rollback est **déjà intégré** au netplay (corrections
  prédictives dans la boucle partagée) ; le déconnecter casserait la course fluide.
  Il fait partie du netplay MVP, pas un bloc séparé.

## Conséquences

- Toute nouvelle fonctionnalité se place **derrière la ligne de fin** : on ne valide la MVP
  que par la phrase "je veux recommencer", pas par le nombre de paquets ou de cores.
- Le bot vision ne doit jamais être branché dans le chemin produit ; son budget API est
  déconnecté de celui du produit.
- Le WIP en cours (une seule feature : netplay 2 joueurs) se commit en unités logiques qui
  compilent et passent les tests chacune : moteur (ports), netplay produit, outillage dev.
- Un commit ne mélange jamais la passe moteur (shim C) et la passe UI.
