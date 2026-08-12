# Ghost via cores libretro embarqués + calibration automatique par diff d'état

L'extraction du Ghost repose sur deux mécanismes légers, sans IA temps réel et sans capture d'écran :

1. **Apparence** : l'instance ghost rend sa propre framebuffer via le vidéo callback libretro. Le client copie la région autour de la position du personnage, la teinte (couleur de course, contour, opacité) et la compose dans la fenêtre principale. Zéro segmentation, zéro sprite sheet, zéro adresse mémoire pour l'apparence.
2. **Position** : déterminée par une calibration automatique une fois par jeu. On lance deux instances du même core avec des inputs légèrement différents, on sérialise l'état (save state) à la même frame, et on compare les tableaux d'octets. Les octets qui diffèrent correspondent à l'état du joueur (position, vie, score). Le client ne lit ensuite que 4 à 8 octets par frame — un coût négligeable. La base de données de RetroAchievements peut semer cette recherche.

Le coût runtime est délibérément minimal : deux cores 8/16-bit embarqués (quelques % de CPU sur Apple Silicon), pas de rendu vidéo côté ghost (vidéo callback vers NULL), et une mémoire partagée de quelques octets par frame.

## Options rejetées

- L'IA de segmentation temps réel (MobileSAM/YOLO-seg) : lourde, superflue puisque la framebuffer du ghost fournit déjà l'apparence et que la calibration donne la position.
- La capture d'écran système (ScreenCaptureKit/Windows Graphics Capture) : inutile car le vidéo callback libretro expose directement la framebuffer.
- Lancer RetroArch comme application externe : deux instances complètes sont plus lourdes que deux cores embarqués, et ne donnent pas d'accès direct à la framebuffer.
- Les profils RAM écrits à la main : remplacés par la calibration automatique (quelques secondes par jeu), seedée par RetroAchievements.
- Les plans de rendu des émulateurs : non disponibles de façon uniforme sur tous les cores.
- La différence de frames (`Echo`) : conservée comme fallback pour les jeux dont la calibration échoue, mais elle confond ennemis et HUD avec le personnage.

## Conséquences

- Le client est un mini-frontend libretro (pas un émulateur écrit maison) : le core est une bibliothèque C mature, compilée depuis les sources officielles.
- L'instance ghost tourne dans un processus séparé (l'API libretro est single-instance) et communique via mémoire partagée : quelques octets par frame.
- Chaque jeu supporté demande une calibration de quelques secondes, versionnée dans un petit JSON ; les jeux dont la calibration échoue restent supportés en `Echo` ou sont exclus.
- L'IA n'est plus dans la boucle temps réel. Elle reste envisageable en analyse différée (coach post-course, anti-triche), où son coût est sans impact.
- Le déterminisme (même ROM, même core, même état de départ, même cadence) reste la condition de base, inchangée.
