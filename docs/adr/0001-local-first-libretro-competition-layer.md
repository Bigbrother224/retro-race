# Embedded libretro cores first, local-first competition layer

Le produit est une application desktop légère et cross-platform (macOS + Windows) qui embarque des cores libretro (bibliothèques C matures, pas un émulateur écrit maison), lance deux processus locaux — le jeu du joueur et un Ghost silencieux — et ajoute une couche de course, ghost et communauté. Le vidéo callback de l'API libretro expose la framebuffer brute, ce qui supprime la capture d'écran système. La position du personnage est obtenue par une calibration automatique (diff de save states), seedée par la base de RetroAchievements. Un adaptateur RetroArch externe (BSV + interface réseau UDP) reste disponible comme backend alternatif pour les jeux dont le core embarqué n'est pas satisfaisant.

L'application ne distribuera ni ROM ni BIOS et n'enverra pas les fichiers de jeu au serveur : le service ne recevra que les identifiants, hashes, inputs, états de course et métadonnées nécessaires. Cette forme conserve la faible latence du jeu local, réutilise des outils matures et limite le risque de construire un émulateur avant d'avoir prouvé la communauté.

## Options rejetées

- Orchestrer RetroArch comme application externe en prérequis : deux instances complètes sont plus lourdes que deux cores embarqués, et n'offrent pas d'accès direct à la framebuffer. L'interface NCI UDP reste utile comme backend alternatif.
- La capture d'écran système (ScreenCaptureKit / Windows Graphics Capture) : superflue, le vidéo callback libretro fournit directement la framebuffer.
- Un modèle de vision IA pour extraire le personnage : lourd et superflu, la framebuffer du Ghost fournit déjà l'apparence et la calibration fournit la position.
- Les profils mémoire écrits à la main : remplacés par la calibration automatique, seedée par RetroAchievements.
- Un backend Windows-only comme BizHawk : fonctionnel mais inutilisable sur macOS, qui est la plateforme de développement de l'équipe.
- Un service de cloud gaming : plus simple pour contrôler l'environnement, mais incompatible avec l'objectif low-spec, plus coûteux et juridiquement plus exposé.
- Une plateforme qui héberge les ROMs : exclue pour des raisons de droits, de risque opérationnel et de positionnement.

## Conséquences

- Le client est un mini-frontend libretro : un processus principal (jeu du joueur) et un processus Ghost silencieux (vidéo callback vers NULL, position + région sprite publiées via mémoire partagée, quelques octets par frame).
- Chaque jeu supporté nécessite une calibration automatique de quelques secondes, versionnée dans un petit JSON ; les jeux dont la calibration échoue restent supportés en `Echo` ou trajectoire abstraite.
- Le support initial sera limité aux consoles et jeux dont la reproductibilité peut être démontrée.
- Le mode "jeu avec un ami" sera une Race parallèle, pas du netplay de la logique du jeu.
- L'IA, si elle est un jour utilisée, reste hors boucle temps réel (analyse différée post-course).
