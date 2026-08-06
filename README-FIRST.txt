LOCALCODE 6.1.0 - SCHNELLSTART / QUICK START
=============================================

DEUTSCH
-------
1. ZIP vollständig in einen neuen Ordner entpacken.
2. START.bat doppelklicken. Fehlt der Build oder ist der Quellcode neuer,
   wird automatisch BUILD.bat ausgeführt.
3. Fehlt eine unterstützte Go-Version, lädt LocalCode die aktuelle stabile
   Windows-Ausgabe von go.dev und prüft den offiziellen SHA-256-Wert.
4. Beim ersten Programmstart werden Ollama, das Standardmodell
   qwen2.5-coder:14b und Aider 0.86.2 geprüft. Fehlende Komponenten werden
   automatisch benutzerlokal installiert und danach verifiziert.
5. Große Modell-Downloads können beim ersten Start dauern. Details stehen in
   %LOCALAPPDATA%\LocalCode\localcode.log.

Für einen vollständigen Neuaufbau: BUILD-AND-RUN.bat
Für Diagnoseausgabe: DIAGNOSE.bat

ENGLISH
-------
1. Extract the ZIP completely into a new directory.
2. Double-click START.bat. If the build is missing or source files are newer,
   BUILD.bat runs automatically.
3. If no supported Go version is available, LocalCode downloads the current
   stable Windows release from go.dev and verifies the official SHA-256 value.
4. On first application startup, Ollama, the default qwen2.5-coder:14b model,
   and Aider 0.86.2 are verified. Missing components are installed for the
   current user automatically and verified afterwards.
5. Large model downloads can take time on first startup. Details are written to
   %LOCALAPPDATA%\LocalCode\localcode.log.

For a complete rebuild: BUILD-AND-RUN.bat
For diagnostics: DIAGNOSE.bat
