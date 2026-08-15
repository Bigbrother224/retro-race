# Retro Race

> **Statut (2026-08-15) :** ce document est le **plan** produit. La **MVP implémentée** se limite à la course netplay 2 joueurs (menu → lobby → partie partagée → arbiter → fin dramatique), voir `docs/adr/0004-mvp-scope-and-done-line.md`. La vision large (classements, rating, matchmaking, défi asynchrone, formats, comptes, Ghost in-game) est un plan, pas du code livré. Ne pas la confondre avec l'état réel du dépôt.

## Vision

Redonner vie à la communauté des jeux rétro en transformant l'émulation locale en expérience sociale moderne : lancer un jeu en quelques secondes, retrouver ses amis, affronter un inconnu, voir son adversaire comme un Ghost dans le même niveau, progresser dans des classements fiables et participer à des événements communautaires.

La promesse n'est pas de refaire les jeux. La promesse est :

> **Tu gardes ton jeu rétro, mais tu ne joues plus jamais seul.**

Le produit doit être léger comme un outil rétro, immédiat comme une console, et profond comme une communauté compétitive.

## Positionnement

Le projet n'est ni un nouvel émulateur généraliste, ni une bibliothèque de ROMs, ni un service de streaming. C'est une **couche locale-first de compétition et de communauté** au-dessus de runtimes d'émulation compatibles.

Le cœur de l'expérience est une **Race parallèle** :

1. Chaque joueur possède et exécute localement son contenu.
2. Le client synchronise un état de départ et un compte à rebours.
3. Les joueurs jouent le même segment en parallèle, sans modifier la logique du jeu.
4. Les inputs de l'adversaire sont relayés et rejoués dans une seconde instance locale.
5. Le Ghost de l'adversaire est composé dans le framebuffer du jeu.
6. Le premier événement de victoire valide gagne le segment.
7. Les résultats alimentent l'historique, le rating, les classements et les rivalités.

## Pourquoi maintenant

Les briques existent séparément : RetroArch fournit le netplay, RetroAchievements les succès et certaines données de classement, TASVideos prouve la viabilité des replays d'inputs, Speedrun.com structure des communautés et Fightcade montre qu'un netplay spécialisé peut durer. Il manque une expérience unifiée, simple et moderne centrée sur le Ghost live, les courses par segment et la découverte de joueurs.

Les concurrents proches ne couvrent pas la même combinaison :

| Produit ou projet | Force | Manque principal pour Retro Race |
| --- | --- | --- |
| RetroArch | Cores, netplay, spectateurs | UX technique, pas de Ghost live propre ni de compétition guidée |
| RetroAchievements | Succès, profils, communauté, données de jeu | Pas de Race parallèle ni de Ghost live |
| Speedrun.com | Règles et classements communautaires matures | Pas de lancement local ni de duel intégré |
| Fightcade | Netplay très fort pour l'arcade | Périmètre limité, pas de campagne par segment ni Ghost général |
| TASVideos / BizHawk | Replays d'inputs et reproductibilité | Expérience experte, pas de matchmaking casual |
| Parsec / Steam Remote Play | Jeu distant simple | Streaming générique, pas de Ghost, arbitrage ni classement |
| Antstream Arcade | Catalogue sous licence et défis | Modèle cloud fermé, pas local-first |

La différenciation ne doit donc pas être "nous supportons plus de consoles". Elle doit être la qualité de la boucle : **choisir, partir, voir, battre, recommencer, partager**.

## Principes non négociables

### Local-first

Le jeu et le rendu principal tournent sur le PC du joueur. Le réseau transporte des inputs, des événements et des métadonnées, pas un flux vidéo permanent. Cela réduit la latence, la bande passante et le coût.

### Light by default

- macOS et Windows dès le départ (les cores libretro sont natifs sur les deux), Linux ensuite.
- Client natif ou shell très léger avec cœur natif ; éviter une application Electron lourde.
- Pas de streaming vidéo obligatoire.
- Deux processus du core seulement quand le Ghost l'exige, dont un silencieux sans rendu vidéo.
- 8/16-bit d'abord ; PS1 ensuite ; PS2 hors roadmap initiale.
- Démarrage rapide, configuration automatique, réglages avancés masqués.

### Réutiliser avant de réécrire

Le produit doit exploiter les capacités déjà disponibles avant de construire une technologie propriétaire :

- **Les cores libretro embarqués** comme backend principal : bibliothèques C matures (NES, SNES, GB/GBC, Genesis, GBA), vidéo callback qui expose la framebuffer sans capture d'écran, et mêmes cores sur macOS et Windows ;
- **RetroAchievements (rcheevos)** pour semer la calibration de position et réutiliser les données de succès existantes ;
- **RetroArch complet** comme backend alternatif (BSV + interface réseau UDP) si un core embarqué n'est pas satisfaisant pour un jeu ;
- **BizHawk** comme backend Windows uniquement, en cas de besoin de fonctions spécifiques (Lua, plans de rendu) ;
- **Mesen** ou **mGBA** comme backends spécialisés si leur scripting donne une meilleure qualité sur une famille de consoles ;
- **LiveSplit** pour comparer les temps et tester la logique de course avant de réimplémenter un système de timing ;
- **OBS** uniquement pour prototyper l'esthétique, enregistrer et inspecter les frames, pas comme moteur de jeu final.

La première version de Retro Race est donc un **mini-frontend libretro cross-platform**, pas un nouvel émulateur.

### La ROM reste locale

Le client scanne un fichier local, calcule son hash et l'associe à un identifiant canonique. Le fichier, le BIOS et les assets du jeu ne sont ni fournis ni envoyés au serveur. Ce principe ne constitue pas à lui seul un avis juridique : il faut une revue spécialisée avant distribution publique.

### La communauté est conçue dès le départ

Les amis, les rivalités, l'activité et la modération ne sont pas des ajouts après le moteur de jeu. Sans eux, on a un outil technique ; avec eux, on a un lieu où revenir.

### Les classements disent la vérité sur leur niveau de confiance

Un meilleur temps local ne doit pas être présenté comme une preuve irréfutable. Le produit sépare Casual, Replayed, Verified et éventuellement Reviewed, avec des règles lisibles par catégorie.

## Expérience cible

### 1. Premier lancement

- Installer l'application sans compte obligatoire pour jouer localement.
- Détecter manettes, clavier et dossiers de contenu.
- Afficher immédiatement les jeux locaux reconnus.
- Proposer un test Ghost sur une démo/homebrew fournie légalement ou un jeu déjà présent localement.
- Créer un pseudonyme au moment où l'utilisateur veut ajouter des amis ou publier un résultat.

Objectif : atteindre une première Race en moins de cinq minutes, sans connaître les cores, BIOS, ports ou réglages vidéo.

### 2. Race avec un ami

- Depuis un jeu : `Jouer avec un ami`.
- Choisir un segment supporté ou `défi libre`.
- Inviter par lien court, code ou liste d'amis.
- Vérifier automatiquement jeu, hash, région, core, version et paramètres.
- Prévisualiser la couleur et le nom du Ghost de chacun.
- Compte à rebours commun.
- Ghost en jeu, décalage réseau affiché discrètement si nécessaire.
- Fin automatique, écran de résultat, replay et bouton `Revanche`.

### 3. Match aléatoire

Le joueur choisit :

- un jeu précis ;
- une console ;
- une catégorie de difficulté ;
- ou `surprends-moi` parmi les jeux qu'il possède et les Game Profiles compatibles.

Le matchmaking ne lance jamais une partie sur un jeu que le joueur n'a pas localement. La file indique clairement le temps d'attente et la compatibilité.

### 4. Défi asynchrone

Un joueur publie un Run et lance `Bats ce Ghost`. L'autre peut jouer plus tard, sans présence simultanée. C'est essentiel pour remplir la communauté quand le nombre de joueurs en ligne est faible.

### 5. Profil et rivalité

Chaque profil montre :

- jeux favoris ;
- meilleurs temps par catégorie ;
- taux de victoire et rating par jeu ;
- rivalités récentes ;
- Ghosts publics ;
- badges et séries ;
- événements terminés.

Un `Rival` doit être visible et actionnable : `son dernier temps`, `mon retard`, `le battre`, `revanche`.

### 6. Communauté de jeu

Chaque jeu supporté possède une page avec :

- classements par segment, région, version et catégorie ;
- meilleurs Ghosts ;
- règles de la catégorie ;
- joueurs actifs ;
- événements à venir ;
- guides de configuration ;
- signalement et modération.

## La course

### La stratégie la plus simple

Le premier prototype embarque le core libretro (pas RetroArch complet) : le vidéo callback de l'API libretro expose la framebuffer brute, sans capture d'écran. Chaque joueur a sa propre instance du core :

```text
Processus joueur A
   ├─ core libretro A : jeu réel, visible, manette du joueur
   ├─ vidéo callback libretro → framebuffer A (pas de capture d'écran)
   └─ affiche une petite fenêtre live (PiP) de la framebuffer de B + la jauge

Processus joueur B
   ├─ core libretro B : même ROM, même core, même état de départ
   ├─ vidéo callback libretro → framebuffer B
   └─ partage de petits extraits basse fréquence de sa framebuffer pour le PiP de A
```

Le processus adversaire ne reçoit jamais une vidéo réseau complète : il partage de petits extraits de sa framebuffer (quelques frames par seconde) pour la fenêtre PiP. Le gameplay de A ne dépend jamais de l'état de B ; la fenêtre PiP peut laguer ou sauter des frames sans rendre la partie injouable.

### La couche course, simplifiée : voir l'écran de l'autre + jauge, sans ghost

On abandonne le Ghost in-game : **rien n'est injecté dans le jeu**. À la place, pendant une Race :

1. **Fenêtre live (PiP)** : chaque joueur voit une petite fenêtre de l'écran de l'autre (extrait framebuffer basse fréquence, ~quelques fps). On voit où en est l'adversaire à l'œil.
2. **Jauge de course** : une barre comparée (toi vs l'autre) indique qui est en tête, basée sur le temps écoulé ou la progression détectée.
3. **Fin dramatique** : ralenti de la dernière seconde, nom du vainqueur + temps, puis replay des dernières secondes des deux écrans côte à côte.

Coût réel :

- un core 8/16-bit par joueur = quelques % de CPU sur Apple Silicon ;
- extraits framebuffer pour le PiP = bande passante négligeable (petites images, basse fréquence) ;
- app plus légère qu'un RetroArch complet, sans Electron.

Limites à connaître :

- la jauge de progression par temps est approximative pour les jeux non linéaires (elle mesure qui joue depuis le moins longtemps) ;
- la détection de fin de segment par changement d'écran peut se tromper sur les cinématiques ; elle sera affinée par Game Profile (règles par jeu).

### L'arbitrage de course

La fin de segment est détectée par **changement d'écran** et par événements du Run, pas par un modèle de vision temps réel :

- un gros changement persistant de la framebuffer (écran de fin, Game Over) signale une fin de segment — détection générique, tous jeux confondus ;
- un modèle de classification d'écrans de fin (Level Clear / Game Over) reste une option asynchrone en secours, hors boucle temps réel, donc sans coût runtime ;
- la vérification par rejeu déterministe reste nécessaire pour les classements sérieux.

### Ordre de réutilisation des ressources existantes

1. Embarquer le core libretro (framebuffer via vidéo callback) : la voie par défaut, la plus légère au runtime.
2. Détection de fin par changement d'écran + PiP framebuffer : générique, tous jeux.
3. Utiliser les formats natifs des cores pour le replay (BSV / savestate) avant d'inventer un format.
4. Ajouter des règles par jeu (Game Profile) seulement pour les jeux populaires qui exigent une détection précise.
5. Lancer RetroArch complet via NCI UDP + BSV seulement comme backend alternatif pour les jeux dont le core embarqué n'est pas satisfaisant.

### Version minimale réellement recommandée

Pour éviter de construire une plateforme avant d'avoir validé l'idée, le premier prototype doit être volontairement étroit :

- macOS et Windows, développé sur le Mac de l'équipe ;
- un seul backend : core libretro embarqué (validé : NES + SNES) ;
- menu arcade : choisir console → choisir jeu ;
- un joueur + un opposant local simulé (2e instance) pour tester la course ;
- framebuffer via vidéo callback, rendu fluide (vsync) ;
- input unifié clavier + manette ;
- course : PiP live de l'écran de l'autre + jauge de progression + fin dramatique (ralenti + replay des dernières secondes côte à côte) ;
- aucun compte, classement ou matchmaking dans le premier test ;
- deux manettes et une revanche locale simulée avant tout réseau.

Le réseau arrive seulement après validation de cette phrase :

> "Je vois suffisamment bien l'écran de mon ami pour comprendre où il me dépasse et vouloir recommencer."

**La nouveauté de l'approche : rien n'est injecté dans le jeu. La framebuffer du joueur (via le vidéo callback) sert à la fois d'affichage et d'extrait partagé pour le PiP ; la jauge et la fin dramatique s'appuient sur le temps et la détection de changement d'écran — pas d'IA temps réel, pas de capture d'écran, pas de profile RAM écrit à la main.**

### Architecture de base

Pendant une Race, chaque client montre une petite fenêtre live de la framebuffer de l'adversaire (extraits basse fréquence) et une jauge de progression. Les inputs du run restent le moyen de vérification/rejeu déterministe.

```text
Core local joueur A ───────── framebuffer A ── écran A (principal)
                                        │
                                        └── extrait basse fréquence ──► PiP chez B

Core joueur B ───────────────── framebuffer B ── écran B (principal)
                                        │
                                        └── extrait basse fréquence ──► PiP chez A

Jauge de course : temps/progression comparés, affichée chez les deux.
```

Le gameplay de A n'est jamais dépendant de l'état de B. Le PiP peut laguer de quelques frames sans rendre la partie injouable.

### Trois niveaux d'affichage de l'adversaire

#### Niveau 1 : PiP + jauge (recommandé, MVP)

Une petite fenêtre live de l'écran de l'adversaire + une jauge comparée. Simple, universel, fonctionne pour tous les jeux sans profil.

#### Niveau 2 : fin dramatique + replay

Ralenti de la dernière seconde, nom du vainqueur + temps, replay des dernières secondes des deux écrans côte à côte. Même base technique que le Niveau 1 (framebuffer), plus l'habillage.

#### Niveau 3 : représentation in-game (optionnelle, plus tard)

Toute représentation dans le jeu (ghost, trajectoire) est explicitement hors MVP. On ne la reconsidère que si le PiP ne suffit pas à l'excitation.

### Déterminisme

Le rejeu et la vérification d'un Run exigent au minimum :

- même hash de contenu ;
- même région et révision ;
- même core et version ;
- mêmes options déterministes ;
- même état de départ ;
- même cadence logique ;
- absence de divergence détectée.

Le support initial recommandé : NES, SNES, Game Boy, Game Boy Color, Genesis et quelques jeux GBA déterministes. PS1 et plus lourds viennent plus tard ; PS2 est exclue du MVP.

### Game Profiles

Un Game Profile est un artefact versionné, testable et publiable séparément du client. Pour le MVP, il doit rester aussi petit que possible. Il contient :

- identifiant du jeu et hashes acceptés ;
- régions et révisions ;
- core et version compatibles ;
- état de départ et méthode de reset ;
- segments et ordre ;
- détection de début, checkpoint et victoire (par changement d'écran, affinée par jeu) ;
- limites de temps et pauses autorisées ;
- règles de classement ;
- limitations connues et tests de reproductibilité.

La communauté peut proposer des profiles, mais seuls les profiles maintenus et testés alimentent les classements Verified.

## Formats de compétition

### Duel live 1v1

Le mode emblématique. Départ simultané, PiP live de l'écran adverse + jauge, victoire au premier événement de fin valide.

### Défi asynchrone

Le mode de rétention. Le joueur défie un temps existant, le run de l'adversaire est rejoué (déterministe) pour comparer, et le résultat peut être publié même si le rival est hors ligne.

### Course de segments

Une série de segments ordonnés. Chaque victoire donne un point ; un abandon ne détruit pas tout l'historique. Supporter `premier à 3`, `course complète` et `revanche immédiate`.

### Matchmaking aléatoire

Une file par jeu/catégorie et une file `surprise`. Pour éviter l'attente infinie, le système peut proposer une liste de trois jeux compatibles au lieu de forcer un jeu inattendu.

### Tournois et saisons

- coupe hebdomadaire sur un jeu ;
- événement mensuel multi-jeux avec profils possédés ;
- saison avec divisions ;
- défi communautaire contre un Ghost de référence ;
- course anniversaire autour d'une console ou d'une série.

La saison doit récompenser la participation et la progression, pas uniquement les joueurs déjà experts.

## Classements et intégrité

### Classements à séparer

1. **Fastest** : meilleur temps en frames sur un segment.
2. **Head-to-head** : victoires, défaites et rating.
3. **Consistency** : régularité sur une série de segments.
4. **Community** : participation, événements, défis proposés et résolus.
5. **Global profile** : progression ludique, jamais présenté comme une mesure objective de talent entre jeux différents.

Chaque classement est filtré par jeu, segment, version, région, catégorie et statut de vérification.

### Niveaux de confiance

- **Casual** : résultat local partageable, aucune garantie.
- **Replayed** : replay rejoué sans divergence par le client.
- **Verified** : règles du Game Profile respectées, contenu et core reconnus.
- **Reviewed** : résultat important contrôlé par la communauté ou une procédure dédiée.

Un serveur qui ne possède pas la ROM ne peut pas promettre une vérification cryptographiquement inviolable. Le wording produit doit rester honnête.

### Anti-triche pragmatique

- verrouillage vitesse, pause, rewind, frame advance et savestates pour les catégories strictes ;
- hash du contenu, core, version et options dans chaque Run ;
- replay signé localement et checksum par blocs ;
- détection des divergences et des trous d'input ;
- impossibilité de modifier un résultat publié ; correction par nouvelle version et journal public ;
- revue manuelle des records et preuves vidéo facultatives ;
- signalement communautaire avec état `contesté`, jamais suppression silencieuse.

L'anti-triche parfait ne doit pas bloquer le Casual. Le produit doit privilégier une séparation claire des catégories plutôt qu'une fausse promesse de sécurité absolue.

## Architecture cible

### Client desktop et adaptateurs

- launcher natif léger (Swift/SwiftUI ou Rust), macOS et Windows ;
- mini-frontend libretro embarqué (les cores sont des bibliothèques C matures, pas un émulateur écrit maison) ;
- processus Ghost séparé, silencieux, rejouant les inputs et publiant position + région sprite ;
- calibration automatique par diff de save states, seedée par la base de données de RetroAchievements ;
- fenêtre overlay transparente click-through pour le Ghost ;
- renderer GPU pour l'overlay et la composition ;
- input manager unifié ;
- recorder/replayer qui réutilise d'abord les formats natifs des cores ;
- gestionnaire de Game Profiles ;
- cache local de métadonnées et de replays ;
- mise à jour signée du client et des cores ;
- backend alternatif : adaptateur RetroArch (NCI UDP + BSV) pour les jeux dont le core embarqué n'est pas satisfaisant.

L'architecture doit conserver une frontière claire afin de remplacer le core embarqué par RetroArch, BizHawk ou Mesen sans changer le modèle de Race, et de permuter la calibration sans toucher au reste.

### Services backend

- identité, pseudonyme et session ;
- amis, invitations, blocage et présence ;
- matchmaking et lobbies ;
- relay régional de petits paquets d'inputs ;
- horloge de départ et état éphémère de Race ;
- stockage de replays, résultats, Ghosts publiés et métadonnées ;
- leaderboards et ratings ;
- catalogue de Game Profiles ;
- événements, saisons et notifications ;
- signalement, modération et audit.

Le serveur ne doit pas être le chemin obligatoire du rendu du jeu. En cas de panne, le jeu local et les replays locaux continuent de fonctionner.

### Transport

Pour le prototype : WebSocket ou QUIC selon les bibliothèques disponibles. Pour la production : relay régional avec paquets input horodatés, reconnexion, accusé de réception et fenêtre de retard. Les flux audio/vidéo restent hors du cœur de produit.

### Données conservées

- identifiant utilisateur et pseudonyme ;
- amitiés, blocages, présence minimale ;
- hashes et identifiants de jeux, jamais les ROMs ;
- Run, replay, résultat, statut de vérification ;
- préférences de rendu et de contrôle ;
- messages et signalements selon une politique de rétention claire.

## Légèreté et performance

Objectifs à valider sur une machine basse/moyenne de gamme récente :

- premier écran interactif en moins de 3 secondes après lancement ;
- entrée en jeu en moins de 10 secondes après sélection d'un contenu déjà reconnu ;
- aucune vidéo distante requise pour une Race ;
- réseau d'un duel limité aux inputs et événements, typiquement de l'ordre de quelques dizaines de kilooctets par minute, hors présence et chat ;
- pas plus de deux instances d'émulation pour un Ghost ;
- arrêt propre du Ghost si la machine est trop lente, avec Echo ou Ghost différé comme fallback ;
- rendu stable à la cadence native du core, sans ralentir le jeu principal.

Les budgets exacts seront mesurés au spike. Il ne faut pas promettre un support uniforme de PS1 ou PS2 avant mesure réelle.

## Sécurité, juridique et distribution

### ROMs et BIOS

Le produit ne télécharge, ne stocke, ne redistribue et ne lie vers aucun ROM ou BIOS protégé. Le serveur ne reçoit qu'un hash et des métadonnées. Le client doit indiquer clairement que l'utilisateur doit disposer des droits nécessaires selon sa juridiction.

Cette architecture réduit l'exposition mais ne garantit pas une immunité juridique. Faire relire les conditions d'utilisation, le parcours d'import, les licences des cores et les règles de distribution par un avocat compétent.

### Contenu utilisateur

Les replays, noms, avatars, commentaires et images sont de l'UGC. Prévoir dès la bêta : blocage, signalement, retrait, journal de modération, anti-spam, contrôle des invitations et suppression de compte.

### Sécurité du client

- exécuter les cores sans privilèges élevés ;
- isoler le processus d'émulation et limiter ses accès réseau/fichiers ;
- signer les binaires et les mises à jour ;
- refuser les cores ou plugins arbitraires en mode Verified ;
- traiter tout ROM, replay et profile externe comme non fiable ;
- ne jamais exécuter un script de profile sans sandbox stricte.

### Vie privée

La présence, le jeu actuel, les relations, la voix éventuelle et l'historique sont des données personnelles. Le MVP devrait éviter la voix et les messages privés ouverts. Commencer avec pseudonyme, amis acceptés, blocage, visibilité configurable et export/suppression des données.

## Stratégie communautaire

### Boucle de retour

```text
Je joue → je vois un Ghost → je perds de peu → je demande une revanche
→ je bats le Ghost → je publie le Run → un autre joueur me défie
```

Chaque écran doit proposer une prochaine action : revanche, battre le Ghost, suivre le rival, rejoindre l'événement ou partager le résultat.

### Mécaniques de vie communautaire

- rival du jour ;
- Ghost de la semaine ;
- séries de victoires ;
- divisions par rating ;
- badges de progression, sans pay-to-win ;
- clubs par console, série ou pays ;
- événements à horaire fixe ;
- découverte des joueurs avec temps proche, pas uniquement des meilleurs ;
- spectateur en mode léger et replay partageable ;
- votes pour proposer le prochain jeu d'un événement ;
- page de jeu avec règles et contributeurs du Game Profile.

Le feed doit privilégier les actions jouables (`Bats ce Ghost`) plutôt que des likes passifs.

### Ce qui peut rendre le produit wow

- arrivée dans la partie avec l'écran de l'ami déjà visible en PiP ;
- nom et statut du rival lisibles sans masquer le jeu ;
- split de temps discret et dramatisation à la fin ;
- replay automatique des dix dernières secondes avec les deux écrans côte à côte ;
- photo de résultat partageable ;
- revanche instantanée sans reconfigurer le jeu ;
- matchmaking qui trouve un joueur de niveau comparable ;
- runs historiques : meilleur ami, record communautaire, record personnel ;
- événements thématiques qui donnent une raison de revenir chaque semaine ;
- **coach IA post-course** : une analyse différée compare ton run et celui du rival et explique en une phrase où tu as perdu du temps ("tu perds 0,8 s sur le premier saut parce que tu sautes 3 frames trop tard"), hors boucle temps réel donc sans coût runtime ;

## Roadmap

Les durées ci-dessous sont des ordres de grandeur pour une petite équipe expérimentée. Elles ne valent pas promesse de calendrier ; le critère est l'exit condition de chaque phase.

### Phase 0 — Spike de faisabilité cross-platform, 1 à 2 semaines ✅

Objectif : vérifier que l'émulation embarquée et la course sont possibles sur le Mac de l'équipe, avant de construire le social.

Livrables (faits) :

- une démo/homebrew légalement distribuable (Alter Ego) + un jeu propriétaire local (Super Mario World, SNES) ;
- core libretro embarqué dans un mini-frontend macOS (vidéo callback → framebuffer) : FCEUmm (NES) et Snes9x (SNES) ;
- rendu fluide 60 fps (CVDisplayLink, buffer réutilisé) ;
- input unifié clavier + manette ;
- détection de fin par changement d'écran ;
- mesure de déterminisme sur des centaines de milliers de frames.

Go/no-go : le jeu reste stable et fluide, les couleurs et dimensions sont correctes. Ne pas écrire un émulateur.

### Phase 1 — Launcher arcade + course locale, 3 à 6 semaines

Objectif : une boucle complète sans compte ni réseau.

Livrables :

- launcher desktop macOS + Windows avec mini-frontend libretro embarqué ;
- menu arcade : choisir console → choisir jeu ;
- bibliothèque locale et import par hash ;
- course locale simulée avec deux instances d'émulation ;
- PiP live de l'écran de l'autre + jauge de progression ;
- fin dramatique (ralenti + nom + temps + replay des dernières secondes côte à côte) ;
- pause, reset, abandon, fin de segment (détection par changement d'écran + événements du Run) ;
- replay exportable et rejouable.

Exit : une personne qui ne connaît pas l'architecture comprend la course en moins de 30 secondes et peut la relancer sans menu technique.

### Phase 2 — Friend Race privée, 3 à 6 semaines

Objectif : jouer réellement avec un ami à distance.

Livrables :

- compte pseudonyme minimal ;
- invitation par code ;
- lobby privé ;
- compatibilité hash/backend/options ;
- relay d'inputs + extraits framebuffer basse fréquence pour le PiP ;
- compte à rebours et frame de départ communs ;
- reconnexion et abandon propre ;
- résultat et revanche ;
- historique privé des Races.

Exit : deux joueurs sur des réseaux différents terminent une Race avec le PiP de l'autre visible, sans ralentissement local.

### Phase 3 — Défis asynchrones et profils, 3 à 5 semaines

Objectif : rendre le produit intéressant même quand aucun ami n'est en ligne.

Livrables :

- publication d'un Run ;
- `Bats ce Run` (rejeu déterministe du run de l'adversaire) ;
- meilleur temps personnel ;
- historique des rivalités ;
- profils et amis ;
- notifications minimales ;
- partage de résultat ;
- 3 à 5 Game Profiles au total.

Exit : un joueur revient spontanément pour battre un Run sans rendez-vous préalable.

### Phase 4 — Classements fiables, 4 à 8 semaines

Objectif : créer une compétition crédible sans prétendre à l'anti-triche absolue.

Livrables :

- leaderboards par jeu/segment/catégorie ;
- statuts Casual, Replayed et Verified ;
- règles visibles avant de jouer ;
- validation du replay et détection de divergence ;
- ratings head-to-head ;
- contestation, revue et historique des corrections ;
- page publique de Game Community.

Exit : un joueur comprend pourquoi un résultat est classé, et la communauté peut identifier un résultat contesté sans casser la confiance globale.

### Phase 5 — Matchmaking aléatoire et première saison, 4 à 8 semaines

Objectif : passer d'un outil entre amis à une communauté vivante.

Livrables :

- file par jeu et catégorie ;
- `Surprise Me` limité aux contenus locaux compatibles ;
- rating et fourchette de recherche ;
- protection contre abandon et reconnexion ;
- saison, divisions et événements hebdomadaires ;
- activité, rivaux et recommandations ;
- modération utilisateur complète.

Exit : l'utilisateur peut lancer l'app seul et trouver une activité jouable sans Discord ni connaissance préalable.

### Phase 6 — Extension maîtrisée

Objectif : augmenter la profondeur sans sacrifier la légèreté.

Ordre recommandé :

1. plus de Visual Adapters NES/SNES/Genesis/GB ;
2. second backend émulateur si le premier ne couvre pas la console ;
3. GBA déterministes sélectionnés ;
4. Ghost différé/Echo sur PS1 ;
5. support macOS/Linux ;
6. profils communautaires audités ;
7. seulement ensuite, core embarqué ou exploration de systèmes plus lourds.

Ne pas ajouter une console parce qu'elle est populaire. L'ajouter quand son coût, sa reproductibilité et sa qualité de Ghost sont maîtrisés.

## Métriques de réussite

### Produit

- temps jusqu'à la première Race ;
- taux de réussite de détection et lancement ;
- taux de Ghost lisible sur les profiles supportés ;
- taux de revanche après une Race ;
- part des Runs asynchrones rejoués ;
- joueurs actifs hebdomadaires par Game Community ;
- nombre de relations rival/ami créées.

### Technique

- divergences de replay par core/profile ;
- abandon dû au réseau ;
- surcharge CPU du Ghost ;
- mémoire au repos et en Race ;
- latence input relay ;
- taux de crash par core.

### Communauté

- pourcentage de nouveaux joueurs qui terminent une première Race ;
- participants récurrents à un événement ;
- ratio défis reçus/résolus ;
- temps de traitement des signalements ;
- part de l'activité qui vient des meilleurs joueurs versus nouveaux joueurs.

La métrique nord-star proposée est : **Races rejouées par joueur actif et par semaine**, complétée par le taux de revanche. Un classement sans revanche n'est qu'une page statique.

## Risques et réponses

| Risque | Gravité | Réponse |
| --- | --- | --- |
| Ghost impossible à rendre génériquement | Très haute | Profiles versionnés, Echo de secours, petit catalogue initial |
| Désynchronisation | Très haute | état de départ commun, hashes stricts, tests de déterminisme, Ghost découplé du gameplay |
| Client compromis et faux records | Haute | niveaux de confiance, replays, revue communautaire, wording honnête |
| Catalogue vide au lancement | Haute | 3 à 5 jeux très travaillés, défis asynchrones, événements réguliers |
| Configuration trop technique | Haute | scan local, détection manettes, diagnostics en langage simple |
| Risque copyright ROM/BIOS | Très haute | aucun hébergement/distribution, import local, revue juridique, documentation claire |
| Client trop lourd | Haute | natif, cores ciblés, pas de vidéo, budgets mesurés dès Phase 0 |
| Matchmaking sans joueurs | Haute | défis asynchrones, Ghosts de référence, bots non classés uniquement si légalement et techniquement propre |
| Toxicité et spam | Haute | pseudonymes, blocage, signalement, invitations privées par défaut |
| Trop de consoles trop tôt | Haute | feuille de support basée sur la qualité, pas sur le nombre |

## Décisions ouvertes à résoudre avant le build produit

### Plateforme

Recommandation : Windows d'abord, avec une architecture qui ne ferme pas macOS/Linux. Le marché PC et les outils d'émulation y sont accessibles, et cela réduit les coûts de packaging et de support.

### Premier jeu

Recommandation : choisir un jeu 8-bit ou 16-bit déterministe avec segments courts et état de victoire clair. Le prototype interne peut utiliser une ROM homebrew ; le premier profile public doit dépendre du contenu local de l'utilisateur et de la revue juridique.

### Live versus asynchrone

Recommandation : construire l'infrastructure de replay dès le début, mais lancer l'expérience avec le duel live privé. L'asynchrone devient le filet de sécurité communautaire dès que le replay est stable.

### Voix et chat

Recommandation : pas de voix intégrée dans le MVP. Les invitations, réactions courtes, blocage et intégration optionnelle avec Discord suffisent au départ. La voix augmente coûts, modération et données sensibles sans améliorer le Ghost lui-même.

### Modèle économique

Options possibles : client gratuit avec service communautaire limité, abonnement pour événements/stockage avancé, ou service auto-hébergeable. Recommandation provisoire : ne pas verrouiller le core local ni le jeu entre amis ; monétiser plus tard le cloud social, les saisons organisées ou les outils créateurs, sans avantage compétitif payant.

## Non-objectifs du MVP

- distribuer des ROMs ou BIOS ;
- supporter toutes les consoles ;
- PS2 ;
- jeu mobile ;
- vidéo distante obligatoire ;
- co-op qui modifie les règles du jeu ;
- voix publique non modérée ;
- classement global mélangeant tous les jeux ;
- anti-triche prétendument parfaite ;
- profiles générés automatiquement sans tests.

## Critère de vérité produit

Le projet ne sera pas validé parce qu'il lance un émulateur. Il sera validé quand un joueur dira :

> "J'ai perdu de deux secondes, j'ai vu exactement où mon ami m'a dépassé, et j'ai immédiatement relancé pour battre son Ghost."

Si cette boucle n'est pas amusante avec trois jeux, ajouter cinquante consoles ne résoudra rien.

## Sources à surveiller

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
