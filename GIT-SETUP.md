# Git setup / Git-Einrichtung

[Deutsch](#deutsch) · [English](#english)

## Deutsch

Dieses Quellpaket ist vollständig für Git und das Öffnen als Ordner in Visual Studio vorbereitet.

### Repository initialisieren

Öffne PowerShell im entpackten Quellordner und führe aus:

```powershell
git init --initial-branch=main
git add .
git status
git commit -F COMMIT_MESSAGE.txt
```

`COMMIT_MESSAGE.txt` enthält eine zum Release passende Commit-Meldung. Prüfe vor jedem Commit immer `git status` und `git diff --cached`.

### Optional: Remote hinzufügen und pushen

```powershell
git remote add origin <HTTPS-ODER-SSH-URL>
git push -u origin main
```

Zugangsdaten gehören niemals in den Quellcode. Verwende den Git Credential Manager, SSH-Schlüssel oder die Authentifizierung des Hosting-Anbieters.

### Visual Studio

1. Visual Studio öffnen.
2. **Datei → Öffnen → Ordner** wählen.
3. Den entpackten LocalCode-Quellordner auswählen.
4. Im Fenster **Git-Änderungen** zuerst die geänderten Dateien und anschließend den Diff prüfen.
5. Zum Bauen `BUILD.bat` oder `BUILD-AND-RUN.bat` verwenden. Für das Go-Projekt ist keine generierte `.sln` erforderlich.

### Enthaltene Git-Dateien

- `.gitignore`: ignoriert `.vs/`, Benutzerdateien, Visual-Studio-Caches, Go-Buildausgaben, `dist/`, portable Werkzeuge, Logs, lokale Laufzeitdaten und typische Geheimnisse.
- `.gitattributes`: legt reproduzierbare Zeilenenden fest; Go- und Dokumentationsdateien verwenden LF, Windows-Skripte CRLF.
- `.editorconfig`: gemeinsame Formatierungsregeln für Visual Studio und andere Editoren.
- `COMMIT_MESSAGE.txt`: geprüfte Commit-Meldung für dieses Release.
- `STATE.md`: aktueller technischer Projektstatus; der markierte LocalCode-Bereich darf automatisch aktualisiert werden.

### Vor einem Push prüfen

```powershell
cd src
go fmt ./...
go test -race -count=1 ./...
go vet ./...
cd ..
git status
git diff --check
git diff --cached
```

Binärdateien in `dist/`, lokale Modell- oder Werkzeugdownloads sowie Zugangsdaten werden absichtlich nicht versioniert.

---

## English

This source package is fully prepared for Git and for opening the directory directly in Visual Studio.

### Initialize the repository

Open PowerShell in the extracted source directory and run:

```powershell
git init --initial-branch=main
git add .
git status
git commit -F COMMIT_MESSAGE.txt
```

`COMMIT_MESSAGE.txt` contains a release-appropriate commit message. Always review `git status` and `git diff --cached` before committing.

### Optional: add a remote and push

```powershell
git remote add origin <HTTPS-OR-SSH-URL>
git push -u origin main
```

Never place credentials in source files. Use Git Credential Manager, SSH keys, or the authentication mechanism provided by the hosting service.

### Visual Studio

1. Open Visual Studio.
2. Choose **File → Open → Folder**.
3. Select the extracted LocalCode source directory.
4. Review the changed files and their diffs in the **Git Changes** window before committing.
5. Use `BUILD.bat` or `BUILD-AND-RUN.bat` to build the application. A generated `.sln` file is not required for this Go project.

### Included Git files

- `.gitignore`: excludes `.vs/`, user files, Visual Studio caches, Go build output, `dist/`, portable tools, logs, local runtime data, and common secrets.
- `.gitattributes`: defines reproducible line endings; Go and documentation files use LF, Windows scripts use CRLF.
- `.editorconfig`: shared formatting rules for Visual Studio and other editors.
- `COMMIT_MESSAGE.txt`: reviewed commit message for this release.
- `STATE.md`: current technical project state; the marked LocalCode section may be maintained automatically.

### Verify before pushing

```powershell
cd src
go fmt ./...
go test -race -count=1 ./...
go vet ./...
cd ..
git status
git diff --check
git diff --cached
```

Binaries in `dist/`, local model or tool downloads, and credentials are intentionally not tracked.
