LOCALCODE 4.7.0 - SCHNELLSTART

1. Dieses ZIP vollständig in einen neuen Ordner entpacken.
2. BUILD-AND-RUN.bat doppelklicken.
3. Der Build führt Formatierung, Tests, go vet und beide Windows-Builds aus.
4. Danach öffnet sich LocalCode 4.7.0 bevorzugt als desktopartiges Edge-/Chrome-App-Fenster.

WICHTIG
- Ollama muss lokal laufen und mindestens ein Coding-Modell enthalten.
- Standardmodell ist bevorzugt qwen2.5-coder:14b.
- Alle Dateien bleiben lokal; Webzugriffe erfolgen nur nach den Einstellungen.

NEU IN 4.7.0
- Git, ADB, Java, .NET und MSBuild werden über Projekt, PATH, Android SDK, Visual Studio und bekannte Windows-Pfade gesucht
- fehlendes Git und Android Platform-Tools können nach separater Genehmigung benutzerlokal installiert werden
- .NET SDK und Visual Studio Build Tools besitzen offizielle, verifizierte Installationsabläufe
- nach Installation wird exakt die ursprüngliche Aktion automatisch wiederholt
- project_info, build_project und deploy_android vermeiden geratenes Build- und Deployment-Verhalten
- Android-Deployment baut, sucht die APK, prüft das Gerät und installiert mit adb install -r
- neue Aufgaben werden nicht mehr durch alte Git-/ADB-Rückfragen gekapert
- leere Websuchanfragen werden aus der aktuellen Aufgabe ergänzt
- Agent Supervisor verhindert Git-Fixierung und Dateiänderungen bei reiner Analyse
- bestätigte Git-Rückfragen werden direkt ausgeführt und verifiziert
- Websuche besitzt eine nicht leere Query-Garantie und einen zweiten Suchanbieter als Fallback
- langer Verlauf wird bei 68 Prozent kontrolliert komprimiert und mit erhaltenem Arbeitszustand fortgesetzt
- Komprimierungsschwelle und aktuelle Nachrichten sind in den Einstellungen konfigurierbar
- Git-ready: .gitignore, .gitattributes, .editorconfig und COMMIT_MESSAGE.txt
