# LocalCode 4.6.0 – Werkzeugreparatur, Build-Automation und zuverlässige Fortsetzungen

## Behoben

- Git wird in Visual Studio, Standardpfaden, `PATH`, Projektpfaden und LocalCode-Werkzeugordnern gesucht.
- Fehlendes Git kann nach Genehmigung als offizielle portable MinGit-Ausgabe installiert und sofort verwendet werden.
- ADB und Fastboot werden in allen bekannten Android-SDK-Pfaden gesucht; fehlende Platform-Tools können nach Genehmigung aus der offiziellen Google-Quelle installiert werden.
- ADB prüft alternative Installationen und Windows Plug-and-Play, statt denselben erfolglosen Befehl zu wiederholen.
- Antworten auf Rückfragen setzen nur bei tatsächlichem semantischem Bezug fort. Neue Aufgaben beseitigen alte Fortsetzungen.
- Leere Websuchanfragen werden aus der aktuellen Aufgabe ergänzt.
- Unnötige Fragen nach Git-Initialisierung oder manueller Werkzeugsuche werden blockiert, wenn LocalCode die Diagnose selbst ausführen kann.

## Neu

- Visual-Studio-Erkennung über `vswhere.exe` und installierte Instanzverzeichnisse.
- Genehmigte automatische Installation mit Verifikation und deterministischem Wiederholen der ursprünglichen Aktion.
- `project_info` für reproduzierbare Projekterkennung.
- `build_project` für Android/Gradle, Go, Rust, Node, .NET/MSBuild, CMake und Python.
- `deploy_android` für Build, APK-Auswahl, Geräteprüfung und `adb install -r`.
- Neue Tests für Fortsetzungslogik, Werkzeug-Metadaten, sichere ZIP-Extraktion, Projektplanerkennung und ADB-Geräteparser.

## Sicherheit

- Installationen benötigen eine eigene Genehmigung.
- Downloads werden nur aus fest codierten offiziellen Quellen beziehungsweise über WinGet bezogen.
- ZIP-Archive werden gegen Pfadtraversierung geprüft.
- Installierte Programme werden vor dem erneuten Originalaufruf verifiziert.
