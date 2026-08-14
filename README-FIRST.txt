LOCALCODE 6.4.4 - SCHNELLSTART / QUICK START
=============================================

DEUTSCH
-------
1. ZIP vollständig in einen neuen Ordner entpacken.
2. START.bat doppelklicken. Beim ersten Start erzwingt die Datei
   dist\REBUILD-NATIVE.txt einen vollständigen nativen Windows-Build. Danach
   wird nur bei fehlendem oder neuerem Quellcode erneut gebaut.
3. Fehlt eine unterstützte Go-Version, lädt LocalCode die aktuelle stabile
   Windows-Ausgabe von go.dev und prüft den offiziellen SHA-256-Wert.
4. Vor der Hauptoberfläche zeigt ein kompaktes Startfenster die laufende
   Prüfung, Installation und den Fortschritt von Modelldownloads. Eine
   deaktivierte Websuche blockiert die automatische Einrichtung nicht mehr.
5. Danach werden Ollama, das Standardmodell qwen2.5-coder:14b und die in den
   Einstellungen ausgewählte Coding-Agent-Engine geprüft. Fehlende unterstützte
   Komponenten werden automatisch benutzerlokal installiert und verifiziert.
6. Unter Einstellungen -> Konfiguration -> Coding-Agent-Engine kann zwischen
   Aider, Claude Code und OpenCode umgeschaltet werden. LocalCode nativ bleibt
   zusätzlich als interne Werkzeugschleife verfügbar.
7. Aider und OpenCode können lokale Ollama-Modelle verwenden. Claude Code
   benötigt eine Anmeldung bei Anthropic oder eine unterstützte Provider-
   Konfiguration. OpenCode kann alternativ über opencode auth login mit einem
   Cloud-Provider verbunden werden.
8. Große Modell-Downloads können beim ersten Start dauern. Details stehen in
   %LOCALAPPDATA%\LocalCode\localcode.log.
9. Für die Handy-Fernsteuerung im selben Netzwerk: LocalCode starten, im Menü
   Hilfe -> Remote koppeln wählen, die angezeigte LAN-Adresse auf dem Handy
   öffnen und den Pairing-Code eingeben.

Für einen vollständigen Neuaufbau: BUILD-AND-RUN.bat
Für Diagnoseausgabe: DIAGNOSE.bat
Engine-Dokumentation: docs\CODING-ENGINES.md

ENGLISH
-------
1. Extract the ZIP completely into a new directory.
2. Double-click START.bat. On first launch, dist\REBUILD-NATIVE.txt forces a
   complete native Windows build. Later rebuilds run only when the executable
   is missing or source/build inputs are newer.
3. If no supported Go version is available, LocalCode downloads the current
   stable Windows release from go.dev and verifies the official SHA-256 value.
4. Before the main UI, a compact startup window shows checks, installation,
   and model-download progress. Disabling web research no longer blocks setup.
5. Ollama, the default qwen2.5-coder:14b model, and the coding-agent engine
   selected in Settings are then verified. Missing supported components are
   installed for the current user automatically and verified afterwards.
6. Settings -> Configuration -> Coding-agent engine switches between Aider,
   Claude Code, and OpenCode. LocalCode native remains available as the
   internal tool loop.
7. Aider and OpenCode can use local Ollama models. Claude Code requires an
   Anthropic sign-in or a supported provider configuration. OpenCode can also
   connect to a cloud provider through opencode auth login.
8. Large model downloads can take time on first startup. Details are written to
   %LOCALAPPDATA%\LocalCode\localcode.log.
9. For phone remote control on the same network: start LocalCode, choose Help
   -> Pair Remote, open the shown LAN address on the phone, and enter the
   pairing code.

For a complete rebuild: BUILD-AND-RUN.bat
For diagnostics: DIAGNOSE.bat
Engine documentation: docs\CODING-ENGINES.md
