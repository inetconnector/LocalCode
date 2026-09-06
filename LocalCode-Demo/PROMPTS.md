# LocalCode Prompts & Technical Architecture: Pac-Man Arcade Demo

[Deutsch](#deutsch) · [English](#english) · [Zurück zur Demo-Übersicht](README.md)

---

## Deutsch

### 📜 Übersicht: Wie LocalCode das Spiel gebaut hat

Das Spiel wurde über die **Android Mobile Remote API** (`/remote/api/chat` und `/remote/api/snapshot`) gesteuert. Das lokale Modell (`qwen2.5-coder:14b` über Ollama) erhielt modulare Aufgabenstellungen zur Erstellung der einzelnen Architektur-Komponenten.

Die mobile App empfing die generierten Dateiänderungen in Echtzeit als Event-Stream und bot im Genehmigungsdialog die direkte Freigabe für Datei-Schreibvorgänge (`write_file`).

---

### 1. Initialer Gesamtauftrag (Holistischer Prompt)

```text
Baue einen vollständigen, sofort spielbaren und detailgetreuen Pac-Man Arcade-Klon (originalgetreue Arcade-Physik und -Logik wie Namco/Konami) mit eigenem Windows-Launcher und Installer in diesem Projektverzeichnis:

Erstelle mit write_file folgende Dateien:

1. `index.html`:
   - Arcade-Cabinet-Optik mit schwarzem Hintergrund, authentischer Retro-Schriftart (Press Start 2P / Arcade Canvas), Highscore-Anzeige, Score, Lives (Pacman Icons), Fruit Indicators, Start/Pause Button, Mute Button und Touch-D-Pad Steuerung für Mobile.
   - Canvas mit nativer Arcade-Auflösung 448x496 (28x31 Tiles à 16px).

2. `style.css`:
   - Elegantes Retro-Arcade Design, zentriertes Cabinet, CRT-Scanline-Filter (optional zuschaltbar), responsive Touch-Controls für Mobile/Touchscreens.

3. `audio.js`:
   - Reiner Web-Audio-API 8-Bit Retro Sound-Synthesizer (keine externen MP3-Abhängigkeiten):
     - Waka-Waka Kausound (präzise alternierende Tonhöhe)
     - Geister-Sirene Hintergrund-Loop
     - Power-Pellet Energizer Wub-Wub Sound
     - Geist-Essen Sound (aufsteigender Arpeggio-Jingle)
     - Pacman-Sterbeanimation Sound (fallende Tonleiter mit Retro-Pitch)
     - Frucht-Essen Bonus Sound
     - Level-Start Intro-Melodie

4. `maze.js`:
   - Exakte 28x31 Arcade-Geometrie mit allen Wänden, 240 Pellets (Punkte = 10), 4 Energizern (Punkte = 50), Geisterhaus-Tür, seitlichen Warp-Tunneln (Zeile 14 mit wrap-around).

5. `pacman.js`:
   - Pac-Man Spieler: Position, Geschwindigkeit, aktuelle/nächste Richtung (Grid-based Cornering), Mundanimation (öffnen/schließen im Kreis), 3 Leben, Kollisionserkennung mit Pellets/Energizern/Früchten, Todesanimation.

6. `ghost.js`:
   - Die 4 originalen Geister mit exakten Farben und KI-Logik:
     - Blinky (Rot / Shadow): Direkte Verfolgung von Pac-Man
     - Pinky (Rosa / Speedy): Zielt 4 Felder vor Pac-Man
     - Inky (Cyan / Bashful): Komplexes Vektor-Targeting basierend auf Blinky und Pac-Man
     - Clyde (Orange / Pokey): Jagd bei Distanz > 8 Tiles, flieht in die untere linke Ecke bei Nähe
   - Geister-Zustände: Scatter-Modus, Chase-Modus, Frightened-Modus (Blaue Geister, blinken vor Ende, Punkte 200/400/800/1600), Eaten-Modus (Augen laufen zurück ins Geisterhaus).

7. `game.js`:
   - Komplette Arcade-Spielschleife (60 FPS requestAnimationFrame), Highscore-Verwaltung mit localStorage, Level-Fortschritt, Früchte-Spawns (Kirsche, Erdbeere, Apfel etc.), Game-Over- und Sieg-Bildschirm, Tastatur- (Pfeiltasten, WASD) und Touch-Steuerung.

8. `start-pacman.bat`:
   - Ein-Klick-Starter für Windows, der das Spiel im Standardbrowser öffnet oder über einen lokalen Micro-Webserver startet.

9. `build-installer.ps1` & `INSTALL.bat`:
   - Ein vollständiges Windows-Setup- und Installationsskript, das das Spiel sauber nach `%LOCALAPPDATA%\Programs\PacmanArcade` installiert, Desktop- und Startmenü-Verknüpfungen anlegt und einen Uninstaller bereitstellt.

Führe nach dem Erstellen aller Dateien eine Überprüfung durch, damit alle Dateien vollständig und lauffähig sind.
```

---

### 2. Modulare Prompts (Schrittweise Modell-Generierung)

#### Schritt 1: `maze.js` (Labyrinth & Kachelmatrix)
```text
Schreibe mit der Aktion write_file die Datei `maze.js` mit dem vollständigen Labyrinth-Code:

Erstelle `maze.js` mit:
1. `const TILE_SIZE = 16; const COLS = 28; const ROWS = 31;`
2. Dem vollständigen 28x31 Gitter-Array (1=Wand, 2=Pellet, 3=Energizer, 0=Leer/Tunnel, 4=Geisterhaus, 5=Geisterhaustür).
3. Einer Klasse `Maze` mit Methoden:
   - `constructor()`: Kopiert das Ursprungsgitter.
   - `isWall(col, row)`: Prüft auf 1 oder 5.
   - `getTile(col, row)`: Gibt den Kacheltyp zurück.
   - `eatPellet(col, row)`: Wenn Pellet (2) oder Energizer (3), setzt auf 0 und gibt { points: 10/50, isEnergizer: bool } zurück.
   - `remainingPellets()`: Zählt alle 2 und 3.
   - `draw(ctx, tick)`: Zeichnet Wände in Blau (#2121DE), Pellets (gelb-weiß r=2), Energizer (r=5, blinkend wenn tick%30 < 15), Geisterhaustür (#FFB8FF).
   - `reset()`: Setzt alle Kacheln auf Anfang.

Führe `write_file` für `maze.js` aus.
```

#### Schritt 2: `pacman.js` (Spieler-Mechanik & Cornering)
```text
Schreibe mit der Aktion write_file die Datei `pacman.js`:

Erstelle `pacman.js` mit der Klasse `Pacman`:
- Startwerte: x = 13.5 * 16, y = 23 * 16, dir = 'LEFT', nextDir = null, speed = 2, radius = 7, mouthAngle = 0.2, mouthDir = 1, lives = 3, isDead = false, deathAngle = 0.
- Methoden:
  - `setDirection(dir)`: Setzt nextDir.
  - `update(maze)`:
    - Wenn Pac-Man an der Kachelmitte ist ((x%16 < speed) und (y%16 < speed)), prüft ob nextDir frei ist und wechselt ggf. die Richtung.
    - Bewegt x, y in Richtung dir, falls nächstes Feld keine Wand ist.
    - Warp-Tunnel in Zeile 14: wenn x < -8 -> x = 28*16; wenn x > 28*16 -> x = -8.
    - Mund-Animation: Öffnet und schließt zyklisch zwischen 0 und 0.35.
  - `draw(ctx)`:
    - Zeichnet Pac-Man (#FFFF00) als Kreis mit Mundaussparung in Blickrichtung (RIGHT=0, DOWN=0.5*PI, LEFT=PI, UP=1.5*PI).
    - Bei Tod (isDead): schrumpft und rotiert der Kreis bis er verschwindet.
  - `reset()`: Position x=13.5*16, y=23*16, dir='LEFT', nextDir=null, isDead=false.

Führe `write_file` für `pacman.js` aus.
```

#### Schritt 3: `ghost.js` (Geister-KI & Modi)
```text
Schreibe mit der Aktion write_file die vollständige Datei `ghost.js`:

Erstelle `ghost.js` mit der Klasse `Ghost` und `GhostManager`:
- Geister: Blinky (Rot #FF0000, Start 13.5*16, 11*16, Scatter 25, 0), Pinky (Rosa #FFB8FF, Start 13.5*16, 14*16, Scatter 2, 0), Inky (Cyan #00FFFF, Start 11.5*16, 14*16, Scatter 27, 31), Clyde (Orange #FFB852, Start 15.5*16, 14*16, Scatter 0, 31).
- Modi: 'SCATTER', 'CHASE', 'FRIGHTENED', 'EATEN'.
- Methoden:
  - `update(maze, pacman, blinky)`: An Kachelkreuzungen wählt der Geist die legale Richtung (keine Wand, keine Umkehr), die dem Ziel-Tile am nächsten liegt (Blinky: Pac-Man Kachel; Pinky: 4 Kacheln vor Pac-Man; Inky: 2*(Pacman+2)-Blinky; Clyde: Pacman wenn Distanz>8 sonst Scatter).
  - Bei Frightened: Zufällige Richtungswahl, halbe Geschwindigkeit.
  - Bei Eaten: Ziel ist Geisterhaus (13.5, 11), doppelte Geschwindigkeit.
  - `draw(ctx)`: Runder Kopf, Wellensaum unten, weiße Augen mit blauen Pupillen in Blickrichtung. Frightened: Dunkelblau mit Zick-Zack-Mund (blinkt weiß vor Ablauf). Eaten: Nur Augen.
  - `frighten()`, `eat()`, `reset()`.
- `GhostManager`: verwaltet alle 4 Geister und bietet `update(maze, pacman)`, `draw(ctx)`, `frightenAll()`, `reset()`.

Führe `write_file` für `ghost.js` aus.
```

#### Schritt 4: `game.js` (60 FPS Gameloop & Kollisionen)
```text
Schreibe mit der Aktion write_file die Datei `game.js`:

Erstelle `game.js` mit:
1. Initialisierung: Canvas (448x496), `const maze = new Maze()`, `const pacman = new Pacman()`, `const ghostManager = new GhostManager()`.
2. Variablen: score = 0, highscore = parseInt(localStorage.getItem('pacman_highscore')||'0'), lives = 3, state = 'START' ('START', 'READY', 'PLAYING', 'PACMAN_DYING', 'GAME_OVER', 'VICTORY'), modeTimer = 0.
3. Kollisionserkennung:
   - Pacman Kachel: `maze.eatPellet(col, row)` -> Punkte addieren, Highscore aktualisieren, Waka-Sound/Energizer-Sound. Wenn Energizer: `ghostManager.frightenAll()`.
   - Pacman <-> Geister Distanz: Wenn Geist Frightened -> `ghost.eat()`, +200..1600 Punkte. Wenn Geist Chase/Scatter -> `pacman.isDead = true`, Leben -1, Todes-Sound.
   - Wenn alle Pellets gegessen (`maze.remainingPellets() === 0`) -> 'VICTORY'.
4. Event-Listener:
   - Keyboard: ArrowUp, ArrowDown, ArrowLeft, ArrowRight, W, A, S, D, Spacebar.
   - Onscreen Buttons: `#up`, `#down`, `#left`, `#right`, `#start-pause`, `#mute`.
5. Gameloop (`requestAnimationFrame`):
   - Clear Canvas, Update State, `maze.draw(ctx)`, `pacman.draw(ctx)`, `ghostManager.draw(ctx)`, Score-HUD aktualisieren.
   - Overlay-Texte für 'READY!', 'PAUSED', 'GAME OVER', 'YOU WIN!'.
6. Start-Aufruf: `requestAnimationFrame(gameLoop)`.

Führe `write_file` für `game.js` aus.
```

#### Schritt 5: Launcher & Windows Installer
```text
Schreibe mit write_file die Windows-Launcher und Installations-Dateien:

1. `start-pacman.bat`:
@echo off
start "" "%~dp0index.html"

2. `build-installer.ps1`:
$ErrorActionPreference = 'Stop'
$targetDir = Join-Path $env:LOCALAPPDATA 'Programs\PacmanArcade'
New-Item -ItemType Directory -Force -Path $targetDir | Out-Null
Copy-Item -Path "$PSScriptRoot\*" -Destination $targetDir -Recurse -Force
$wsh = New-Object -ComObject WScript.Shell
$desktopShortcut = $wsh.CreateShortcut((Join-Path ([Environment]::GetFolderPath('Desktop')) 'Pac-Man Arcade.lnk'))
$desktopShortcut.TargetPath = (Join-Path $targetDir 'start-pacman.bat')
$desktopShortcut.WorkingDirectory = $targetDir
$desktopShortcut.Save()
$startMenu = Join-Path ([Environment]::GetFolderPath('StartMenu')) 'Programs'
$startShortcut = $wsh.CreateShortcut((Join-Path $startMenu 'Pac-Man Arcade.lnk'))
$startShortcut.TargetPath = (Join-Path $targetDir 'start-pacman.bat')
$startShortcut.WorkingDirectory = $targetDir
$startShortcut.Save()
Write-Host "Pac-Man Arcade erfolgreich installiert nach: $targetDir"

3. `INSTALL.bat`:
@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0build-installer.ps1"
pause

Schreibe alle 3 Dateien mit write_file.
```

---

## English

### 📜 Overview: How LocalCode Built This Project

The game was developed via the **LocalCode Android Mobile Remote API** (`/remote/api/chat` and `/remote/api/snapshot`). The local model (`qwen2.5-coder:14b` running via Ollama) processed modular prompts to construct each component of the game engine.

The Android Companion app received live SSE updates and handled tool approval requests for `write_file` with one-tap confirmations.

All prompts above are shown in their original dispatched format.
