LOCALCODE 4.5.0 - SCHNELLSTART

1. Dieses ZIP vollständig in einen neuen Ordner entpacken.
2. BUILD-AND-RUN.bat doppelklicken.
3. Der Build führt Formatierung, Tests, go vet und beide Windows-Builds aus.
4. Danach öffnet sich LocalCode 4.5.0 bevorzugt als desktopartiges Edge-/Chrome-App-Fenster.

WICHTIG
- Ollama muss lokal laufen und mindestens ein Coding-Modell enthalten.
- Standardmodell ist bevorzugt qwen2.5-coder:14b.
- Alle Dateien bleiben lokal; Webzugriffe erfolgen nur nach den Einstellungen.

NEU IN 4.5.0
- vollständige Umbenennung des Produkts, der EXEs, Datenordner und UI auf LocalCode
- verlustfreie Übernahme vorhandener Konfigurationen, Chats und Sicherungen
- Apache-2.0-Lizenz mit LICENSE, NOTICE und THIRD_PARTY_NOTICES.md
- automatische Werkzeugerkennung mit absoluten Pfaden
- Android-SDK-/ADB-Erkennung aus Projekt, Umgebung und Standardpfaden
- vollständige Werkzeugausgaben mit Exitcode, STDOUT und STDERR
- kontrollierter ADB-Reparaturversuch statt Wiederholungsschleifen
- Blockade identischer Aktionen und bereits beantworteter Rückfragen
- automatische Suche offizieller Werkzeugdokumentation, wenn freigegeben
- Werkzeugübersicht und ADB-Diagnose in den Einstellungen
