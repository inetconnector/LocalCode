# LocalCode 6.1.1 – Windows test isolation and native first-build distribution

## Deutsch

Diese Wartungsversion behebt die unter Windows reproduzierten Build-Abbrüche aus 6.1.0.

### Korrekturen

- Der Windows-ADB-Test erzeugt jetzt eine echte tabulatorgetrennte `adb devices -l`-Ausgabe. Das simulierte Gerät wird dadurch korrekt erkannt.
- Der persistente MCP-STDIO-Test beendet und reapet den Hilfsprozess, bevor Windows die temporären Arbeitsordner löscht.
- Eine zusätzliche Regressionprüfung löscht den MCP-Projektordner unmittelbar nach `Close()` und erkennt verbliebene Dateihandles direkt.
- Build-Tests verwenden eigene temporäre Konfigurations-, Cache- und Benutzerverzeichnisse und greifen nicht auf das reale LocalCode-Profil zu.
- Die simulierte Ollama-Modellinstallation schreibt keine irreführende Pull-Meldung mehr in den normalen Testlauf.
- Das Distributionsarchiv enthält wieder Windows-Binärdateien im Ordner `dist`. Eine Markierungsdatei erzwingt vor dem ersten Start einen vollständigen nativen Neuaufbau auf dem Zielsystem.

### Automatische Laufzeiteinrichtung

Die in 6.1.0 eingeführte automatische Einrichtung bleibt enthalten:

- fehlendes Ollama unter Windows installieren und starten,
- konfigurierte Ollama-Modelle automatisch laden,
- `uv`, Python und die festgelegte Aider-Version automatisch einrichten,
- vorhandene Installationen erkennen und weiterverwenden.

## English

This maintenance release fixes the Windows build failures reproduced in 6.1.0.

### Fixes

- The Windows ADB test now generates a real tab-delimited `adb devices -l` response, allowing the simulated device to be detected correctly.
- The persistent MCP STDIO test now terminates and reaps its helper process before Windows removes temporary working directories.
- An additional regression check removes the MCP project directory immediately after `Close()` and detects leaked file handles directly.
- Build tests use isolated temporary configuration, cache, and user directories instead of the real LocalCode profile.
- The simulated Ollama model installation no longer prints a misleading pull message during the normal test run.
- The distribution archive once again includes Windows executables in `dist`. A marker forces a complete native rebuild on the target system before first launch.

### Automatic runtime setup

The automatic setup introduced in 6.1.0 remains included:

- install and start Ollama on Windows when missing,
- automatically download configured Ollama models,
- automatically provision `uv`, Python, and the pinned Aider version,
- detect and reuse existing installations.
