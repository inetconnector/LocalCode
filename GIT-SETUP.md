# Git setup / Git-Einrichtung

[Deutsch](#deutsch) · [English](#english)

## Deutsch

Das Quellpaket ist für Git und Visual Studio vorbereitet.

### Erstes Repository und erster Commit

```powershell
git init --initial-branch=main
git add .
git commit -F COMMIT_MESSAGE.txt
```

### Enthaltene Repository-Dateien

- `.gitignore`: Visual Studio, Go, Build-Ausgaben, lokale Laufzeitdaten, Logs, Caches und Geheimnisse.
- `.gitattributes`: konsistente Zeilenenden für Go, Dokumentation und Windows-Skripte.
- `.editorconfig`: Formatierungsregeln für Visual Studio und andere Editoren.
- `COMMIT_MESSAGE.txt`: geprüfte Commit-Meldung für dieses Release.

`dist/`, portable Werkzeuge und lokale LocalCode-Daten werden absichtlich nicht versioniert. Baue die EXE mit `BUILD.bat` oder `BUILD-AND-RUN.bat` neu.

### Visual Studio

Öffne den Quellordner direkt über **Datei → Ordner öffnen**. Für das Go-Projekt ist keine erzeugte `.sln` erforderlich. Visual-Studio-spezifische Benutzerdateien und Cacheordner werden ignoriert.

## English

The source package is prepared for Git and Visual Studio.

### Initialize the repository and create the first commit

```powershell
git init --initial-branch=main
git add .
git commit -F COMMIT_MESSAGE.txt
```

### Included repository files

- `.gitignore`: excludes Visual Studio state, Go build output, runtime data, logs, caches, and secrets.
- `.gitattributes`: defines consistent line endings for Go, documentation, and Windows scripts.
- `.editorconfig`: formatting rules understood by Visual Studio and other editors.
- `COMMIT_MESSAGE.txt`: reviewed commit message for this release.

`dist/`, portable tools, and machine-local LocalCode data are intentionally excluded. Rebuild executables with `BUILD.bat` or `BUILD-AND-RUN.bat`.

### Visual Studio

Open the source directory with **File → Open → Folder**. A generated `.sln` file is not required for this Go project. Visual Studio user state and cache directories are ignored.
