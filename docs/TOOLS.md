# Werkzeugauflösung und Ausführung

## Ziel

LocalCode darf aus einem fehlgeschlagenen Shell-Aufruf nicht vorschnell schließen, dass ein Programm nicht installiert ist. Vor einer solchen Aussage wird das Werkzeug strukturiert gesucht, mit absolutem Pfad ausgeführt und anhand von Exitcode, Standardausgabe und Standardfehler diagnostiziert.

## Suchreihenfolge

1. Fester Pfad aus `tool_overrides`
2. Projektlokale Werkzeuge und Wrapper
3. Projektspezifische Konfiguration, zum Beispiel `local.properties`
4. Relevante Umgebungsvariablen
5. Prozess-PATH
6. Bekannte installationsspezifische Pfade

Projektlokal geprüft werden insbesondere:

- Projektwurzel
- `bin`
- `tools`
- `.tools`
- `scripts`
- `node_modules/.bin`

## Android-SDK

Android-Werkzeuge werden aus folgenden SDK-Wurzeln gesucht:

- `sdk.dir` in `local.properties`
- `ANDROID_HOME`
- `ANDROID_SDK_ROOT`
- `%LOCALAPPDATA%\Android\Sdk`

Relevante Unterordner:

- `platform-tools` für `adb` und `fastboot`
- `cmdline-tools\<Version>\bin` für `sdkmanager`
- `emulator` für den Android Emulator
- `cmake\<Version>\bin` für `ninja`

## Ausführung

`run_tool` startet das aufgelöste Programm direkt. Batch- und CMD-Werkzeuge werden unter Windows korrekt über `cmd.exe` aufgerufen. Jeder Lauf liefert:

- Werkzeugname
- absoluter Pfad
- Argumentliste
- Arbeitsordner
- Exitcode
- Laufzeit
- STDOUT
- STDERR
- Diagnose

Hintergrundprozesse werden ohne sichtbares Konsolenfenster gestartet. Bei Abbruch oder Timeout wird unter Windows der gesamte Prozessbaum beendet.

## ADB-Diagnose

Für `adb devices -l` werden folgende Zustände getrennt behandelt:

- `device`: Gerät erreichbar
- `unauthorized`: RSA-Freigabe am entsperrten Gerät fehlt
- `offline`: ADB-Verbindung besteht, Gerät antwortet aber nicht korrekt
- leere Liste: ADB funktioniert, aber kein Gerät wird aufgelistet
- Start-/Daemonfehler: ADB-Serverproblem

LocalCode führt höchstens einen automatischen Reparaturzyklus aus. Endlosschleifen und wiederholte identische Nutzerfragen sind blockiert.

## Automatische Dokumentationssuche

Wenn `auto_research_tool_help` aktiviert ist und Netzwerkzugriff erlaubt wurde, ergänzt LocalCode Fehlerdiagnosen um Suchergebnisse aus offizieller Dokumentation. Die Suche ersetzt niemals die lokale Ausgabe; Pfad, Exitcode, STDOUT und STDERR bleiben die primäre Evidenz.

## Grenzen

Es gibt keine sichere Methode, jedes weltweit existierende Programm ohne Kontext und ohne vollständigen Datenträgerscan zu erkennen. LocalCode unterstützt bekannte Entwicklungswerkzeuge sowie beliebige projektlokale oder über PATH beziehungsweise feste Pfade erreichbare Programme. Neue Werkzeuge können ohne Codeänderung über `tool_overrides` eindeutig registriert werden.
