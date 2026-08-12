# IPC ghost → player via POSIX shared memory (seqlock), pas de serialise/restore réseau

Le processus Ghost (rejoue les inputs de l'adversaire) publie chaque frame un **slot** en mémoire partagée : position calibrée (x, y) + sprite tile RGBA (32×32) autour de la position, avec un compteur de frame. Le processus joueur mappe la même région, lit le slot via une discipline seqlock (lecture du compteur avant/après, réessai en cas de lecture torn) et compose le sprite teinté dans son overlay. C'est le remplacement du prototype Phase 0 qui sérialisait l'état à la main entre les runs.

## Décision

- `shm_open` + `mmap(MAP_SHARED)` sur macOS, layout fixe versionné (`rr_shm_slot`, magic/version), producteur = ghost-live, consommateur = player/launcher.
- Le producteur écrit le slot complet puis publie `frame` avec un release store ; le lecteur boucle sur `frame` avant/après copie (seqlock).
- Le producteur dimensionne la région (`ftruncate`) ; un consommateur ne fait jamais `O_CREAT` ni `ftruncate` — il ne fait que mappe ce qui existe.
- La tile est copiée via des accesseurs C (`rr_shm_slot_set_tile` / `rr_shm_slot_tile_copy`) plutôt que d'exposer le tableau C 4096 octets directement à Swift (un tel tableau est importé comme un tuple géant et devient inutilisable).

## Options rejetées

- Sérialiser/restaurer l'état entre processus (proto Phase 0) : coûteux (13 758 octets/frame), et croise le déterminisme avec du transport ; la position+sprite suffisent au rendu.
- Sockets / pipes par frame : latence et sérialisation inutiles pour un canal mono-consommateur intra-machine ; la mémoire partagée donne quelques octets/frame sans copie.
- Mach shared memory ou XPC : plus lourd à mettre en place, `shm_open` est portable vers Windows via un équivalent à choisir plus tard (mapped file).
- Un seul processus avec deux instances du core en threads : l'API libretro est single-instance ; deux processus restent le modèle de référence.

## Conséquences

- Le canal est symétrique à l'architecture documentée (ADR 0001/0002) : position + sprite via mémoire partagée, pas de vidéo réseau.
- `ghost-live` tourne aussi vite que possible (~1300 fps sur Apple Silicon) ; le joueur lit le dernier slot publié. La latence perçue reste < 1 frame du point de vue du joueur.
- Le déterminisme reste inchangé : mêmes ROM/core/état de départ/inputs rejoués ; le shm ne transporte pas d'état de jeu, seulement position + apparence.
- Un slot non publié (ghost en pause, segment fini) est visible via `state` (`IDLE/RACING/DONE/ABORTED`), ce qui permettra pause/abandon sans réseau.
- L'overlay (launcher) spawn désormais `ghost-live` comme enfant, lit le shm et compose le ghost teinté — la Phase 1 « Ghost Lab local » avance sans compte ni réseau.
