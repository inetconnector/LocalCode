# Tool discovery and execution / Werkzeugerkennung und Ausführung

## Deutsch

LocalCode bezeichnet ein Programm erst nach strukturierter Suche als fehlend. Die Suchreihenfolge umfasst konfigurierte Overrides, Projekt-Wrapper, `PATH`, Umgebungsvariablen, Android SDK, Visual Studio und `vswhere`, bekannte Installationsorte und den LocalCode-Werkzeugordner.

Jeder Aufruf dokumentiert soweit verfügbar:

- aufgelösten absoluten Pfad
- Arbeitsverzeichnis
- Argumente
- Exitcode
- Dauer
- STDOUT und STDERR
- Timeout oder Abbruch

Unterstützte fehlende Werkzeuge lösen eine sichtbare Installationsgenehmigung aus. Nach Installation wird der Pfad verifiziert und die ursprüngliche Aktion erneut ausgeführt. Unbekannte Bedienungsfehler dürfen eine Recherche in offizieller Dokumentation auslösen, wenn Netzwerkzugriff aktiviert und genehmigt ist.

## English

LocalCode declares a program missing only after structured discovery. Search locations include configured overrides, project wrappers, `PATH`, environment variables, Android SDK installations, Visual Studio and `vswhere`, known install locations, and the LocalCode tools directory.

Each invocation records, when available:

- resolved absolute path
- working directory
- arguments
- exit code
- duration
- STDOUT and STDERR
- timeout or cancellation

Supported missing tools trigger a visible installation approval. After installation, the path is verified and the original action is retried. Unknown usage failures may trigger research in official documentation when network access is enabled and approved.
