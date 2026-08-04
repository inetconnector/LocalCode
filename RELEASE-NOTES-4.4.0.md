# LocalCode 4.4.0 – Werkzeugauflösung und belastbare Diagnose

Diese Version behebt den gemeldeten Fehler, bei dem ein vorhandenes Android-Gerät nicht zuverlässig über ADB erkannt wurde und der Agent dieselbe Rückfrage wiederholt hat.

## Werkzeugauflösung

- Neue strukturierte Agentenaktionen `discover_tool`, `tool_inventory` und `run_tool`.
- Bekannte Programme werden nicht mehr ausschließlich über den Prozess-PATH gesucht.
- Auflösung erfolgt in dieser Reihenfolge: feste Einstellung, projektlokale Wrapper/Binaries, projektspezifische Konfiguration, Umgebungsvariablen, PATH und bekannte Installationspfade.
- Gefundene Programme werden mit absolutem Pfad ausgeführt.
- Projektlokale Verzeichnisse `bin`, `tools`, `.tools`, `scripts` und `node_modules/.bin` werden berücksichtigt.
- Für unbekannte Werkzeugnamen funktioniert die Auflösung ebenfalls über Projektpfade und PATH.
- Einstellbare feste Werkzeugpfade erlauben eine eindeutige Zuordnung, ohne globale Systemänderungen.

## Android und ADB

- Android-SDK-Erkennung über `local.properties`, `ANDROID_HOME`, `ANDROID_SDK_ROOT` und den üblichen Windows-SDK-Pfad.
- ADB wird bevorzugt direkt aus `platform-tools` ausgeführt.
- `adb devices -l` unterscheidet `device`, `unauthorized`, `offline` und keine gelisteten Geräte.
- Bei Server-/Verbindungsproblemen erfolgt genau ein kontrollierter Reparaturversuch mit `adb start-server`; bei leerer Geräteliste zusätzlich `adb reconnect` und eine erneute Abfrage.
- Vollständiger Pfad, Argumente, Arbeitsordner, Exitcode, Laufzeit, STDOUT und STDERR bleiben sichtbar.
- Ein vorhandenes, aber nicht autorisiertes oder offline gemeldetes Gerät wird nicht fälschlich als fehlende ADB-Installation behandelt.

## Agentenkontrolle

- Identische unmittelbar aufeinanderfolgende Werkzeugaktionen werden blockiert.
- Bereits beantwortete Rückfragen dürfen nicht erneut gestellt werden.
- Nach einem Werkzeugfehler muss der Agent Pfad, Exitcode, STDOUT und STDERR auswerten und eine andere Diagnose wählen.
- Tool- und Shellprozesse übernehmen den Laufkontext; Abbruch und Timeout beenden unter Windows den gesamten Kindprozessbaum.

## Offizielle Hilfe

- Wenn ein Werkzeug nicht gefunden wird oder fehlschlägt, kann LocalCode automatisch nach offizieller Herstellerdokumentation suchen.
- Netzwerkzugriff und Suchanbieter bleiben über die Einstellungen steuerbar.
- Direkte offizielle Dokumentationslinks sind in den Werkzeugprofilen hinterlegt.

## Einstellungen und Diagnose

Unter **Einstellungen > Computernutzung** befinden sich:

- automatische Werkzeugerkennung
- automatische Recherche offizieller Werkzeughilfe
- feste Werkzeugpfade als JSON
- vollständige Werkzeugprüfung
- gezielte ADB-Diagnose

`LocalCode-Debug.exe --diagnose` listet zusätzlich alle erkannten Werkzeuge, deren absolute Pfade, Versionen und ADB-Gerätestatus auf.
