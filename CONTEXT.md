# Retro Race Context

Ce contexte définit le vocabulaire du produit de compétition sociale pour jeux rétro. Il sert à éviter de confondre émulation, netplay, ghost et course.

## Expérience de jeu

**Race**:
Une compétition dans laquelle plusieurs joueurs exécutent localement le même segment de jeu depuis un départ commun et cherchent à le terminer avant les autres.
_Avoid_: Co-op, match netplay, partie partagée

**Ghost**:
La représentation in-game d'un autre joueur, produite à partir de ses inputs rejoués ou de son état de course. Le ghost n'agit pas sur la partie locale et ne modifie pas la logique du jeu.
_Avoid_: Avatar réseau, écran partagé, streaming

**Run**:
Une exécution complète et horodatée d'un segment de jeu, incluant son état de départ, ses inputs, ses événements de progression et son résultat.
_Avoid_: Partie, session, replay (un replay est la représentation persistée d'un run)

**Segment**:
Unité de compétition choisie par le produit, généralement un niveau, une course, une zone ou un boss. Un jeu peut exposer plusieurs segments ordonnés.
_Avoid_: Manche (réservé au format d'une compétition), niveau dans tous les cas

**Race Arbiter**:
Composant qui détecte les événements de fin de segment (victoire, écran de fin, Game Over) à partir des événements du Run et d'un rejeu déterministe, sans lire la mémoire du jeu en temps réel.
_Avoid_: Détection RAM, condition codée par jeu

**Game Profile**:
Définition versionnée et spécifique à un jeu qui décrit son identifiant, ses ROM hashes acceptés, son core, son état de départ, ses conditions de victoire, sa calibration de position et la manière d'afficher le ghost.
_Avoid_: Patch, mod, émulateur

**Ghost Profile**:
Sous-ensemble d'un Game Profile décrivant comment localiser et dessiner le personnage du joueur adverse : adresses de position (obtenues par calibration automatique), décalages de frames et règle de composition.
_Avoid_: Skin, overlay statique

**Ghost Extractor**:
Composant qui localise et découpe le personnage adverse à partir de la framebuffer du processus Ghost (région autour de la position calibrée), sans IA temps réel, sans capture d'écran et sans profile RAM écrit à la main.
_Avoid_: Profile RAM manuel, segmentation IA

## Intégrité et compétition

**Casual Run**:
Run jouable et partageable sans garantie forte d'intégrité. Il peut alimenter l'activité sociale mais pas un classement compétitif certifié.
_Avoid_: Run valide

**Verified Run**:
Run qui respecte un Game Profile, un hash de contenu, un core et des paramètres autorisés, et dont le replay peut être rejoué sans divergence connue.
_Avoid_: Run anti-triche (la vérification locale ne rend pas un client compromis trustless)

**Leaderboard**:
Classement d'un jeu et d'une catégorie précise, ordonné par temps en frames, résultat de duel ou rating selon le type de classement.
_Avoid_: Classement global unique

**Rating**:
Estimation de la force d'un joueur dans les duels d'une catégorie, distincte du meilleur temps historique.
_Avoid_: Score, niveau du compte

**Competition**:
Format organisé qui agrège plusieurs Races ou Runs : duel entre amis, matchmaking aléatoire, défi asynchrone, tournoi ou saison.
_Avoid_: Session (terme technique pour une connexion active)

## Social

**Friend**:
Relation réciproque acceptée par deux utilisateurs, permettant les invitations et les défis privés.
_Avoid_: Follower, contact

**Rival**:
Joueur suivi ou fréquemment affronté dont le Ghost et les progrès sont mis en avant.
_Avoid_: Ami (un rival peut être public sans relation réciproque)

**Lobby**:
Espace temporaire de préparation d'une Race, avec participants, jeu, segment, règles et compte à rebours.
_Avoid_: Serveur de jeu (le jeu tourne localement)

**Game Community**:
Espace public centré sur un jeu, ses catégories, ses événements, ses joueurs et ses règles de classement.
_Avoid_: Guilde (réservé à une organisation créée par les utilisateurs)

## Ambiguïtés résolues

- Le produit ne transforme pas les jeux solo en jeux co-op. Il organise des **Races** parallèles avec un **Ghost** visible in-game.
- Le serveur ne fait pas tourner la ROM du joueur et ne reçoit pas sa ROM. Il orchestre, relaie des inputs et conserve des métadonnées et replays autorisés.
- Un classement ne mélange jamais des contenus, cores, régions ou catégories incompatibles.
- "Vérifié" signifie reproductible selon les règles du Game Profile, pas inviolable face à un client compromis.

## Exemple de dialogue

> **Joueur** : Je lance une Race sur le segment 1-1 de mon Game Profile NES.
>
> **Client** : Ton ami est dans le Lobby. Le départ commun aura lieu dans 5 secondes.
>
> **Joueur** : Je vois son Ghost en violet dans mon niveau, mais il ne peut pas toucher mes ennemis.
>
> **Client** : Correct. Vous produisez chacun un Run local. Le premier événement de victoire valide remporte le segment.
>
> **Joueur** : Mon temps apparaît dans le Leaderboard ?
>
> **Client** : Oui si ton Run est Verified Run. Sinon il reste un Casual Run visible sur ton profil et dans l'activité de tes amis.
