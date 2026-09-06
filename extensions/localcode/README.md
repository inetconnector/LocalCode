# LocalCode for VS Code and Antigravity IDE

[Deutsch](#deutsch) · [English](#english)

## English

Use your LocalCode agent beside your code: a dedicated Activity Bar sidebar and an editor panel, persistent tasks, live events, local model selection, explicit tool approvals and native Git diffs. LocalCode is independent of Microsoft, Google and Anthropic.

### Install and use

1. Install and start [LocalCode Desktop](https://github.com/inetconnector/LocalCode/releases). Priority follow-ups and task-bound stopping require the backend shipped with the IDE extension release (capabilities `steering-v1` and `stop-task-v1`). Older backends support chat but reject these newer controls with an update message.
2. Download `localcode-0.1.0.vsix` from the release. In VS Code or Antigravity IDE, open Extensions, then **… → Install from VSIX…** and select the file.
3. Open a **trusted local folder** inside the project root configured in LocalCode Desktop. Click the LocalCode Activity Bar icon or press **Ctrl+Alt+L** (macOS: **Cmd+Alt+L**).
4. Choose a saved task or create one, select a model and send your request. Use **Add file or selection** to attach bounded editor context; no file content is sent automatically.
5. While LocalCode is working, keep typing and send another prompt. The new instructions take priority at the next safe boundary. Stop remains a separate button. Tool approvals display the action, command/path and preview before **Approve once** or **Reject**.

The companion backend runs separately. **Start LocalCode** launches the installed Windows application on explicit request; closing the IDE does not kill it. Configure `localcode.executablePath` if the program is installed elsewhere. The default connection is `http://127.0.0.1:32145`; adjust the application-level `localcode.serverUrl` if necessary. There is no automatic executable download or model installation by this extension.

### Features

- Sidebar and editor chat with theme colors, keyboard navigation, accessible labels, safe Markdown/code blocks and copy controls.
- Persistent backend task history filtered to the chosen workspace; explicit project/task identity with every prompt.
- SSE live events and periodic snapshot reconciliation; reconnect and stale-task detection.
- Follow-up prompts interrupt stale **model inference**, retain the active task, and enter a bounded FIFO mailbox. Newer user instructions resolve conflicts. Started tools finish first. Follow-ups never grant new permissions or reset run budgets.
- Editor selection/file context (64 KiB combined), explicit error/warning diagnostics, and protection against sending edits over unsaved workspace files.
- One-time tool approvals; no silent project/global permission grants.
- Native Git review from HEAD to the current file, including staged/current changes and unsaved editor contents. This is a working-tree review, not an atomic rollback operation. LocalCode's own engine backups remain in Desktop.
- German and English, with synchronized catalog keys. Automatic language follows LocalCode's Windows display-language report; disconnected fallback is the IDE language. Manual setting overrides it.
- Network traffic goes through the extension host to a validated loopback address. The webview cannot fetch arbitrary URLs, execute shell commands or invoke arbitrary IDE commands.

### Requirements and limits

VS Code API 1.85+ or a compatible Antigravity IDE, a running LocalCode backend and a configured model are required. The backend remains Windows-first. Remote SSH/WSL/Codespaces and virtual workspaces are deliberately disabled in this version. Multi-root local workspaces require choosing one folder. Git review requires the built-in Git extension and a repository rooted at that folder.

Desktop and IDE share **one active agent run**. Independent IDE windows may inspect different saved tasks, but cannot run them concurrently on the same backend. Use separate backend instances/profiles for independent execution. The extension sends explicit task/run IDs; a stale Stop action cannot cancel another run on a current backend.

Follow-ups are text (including explicitly attached text context), up to 32 KiB each, 16 waiting messages, 64 IDs and 128 KiB total per run. Retries reuse the same ID. They are accepted only while the ordinary agent loop is active; Mission recovery, initialization and finalization do not acquire new steering authority. The mailbox is transient: process crashes discard unapplied input, and there is no automatic replay. Applied messages enter existing chat history; pending messages abandoned by ordinary termination receive an explicit warning. A queued acknowledgement is not proof of model compliance.

This extension integrates the existing LocalCode tools and model. It does not claim identical reasoning quality, proprietary cloud services, inline autocomplete, background multi-agent sessions, or full feature parity with Claude Code or Google's Antigravity. Image/voice input and advanced engine/MCP/recovery settings remain available in LocalCode Desktop.

### Development, verification and publishing

```sh
cd extensions/localcode
npm ci
npm run check
npm test
python scripts/test-webview.py
node scripts/test-host.js
npm run package
```

Node.js 22+ and npm are development requirements; the installed extension has no third-party runtime dependencies. The webview test uses Python Playwright and installed Edge. `test-host.js` downloads official stable VS Code into `.vscode-test`; set `LOCALCODE_TEST_IDE` to an existing IDE executable to test Antigravity instead. Tests use isolated user data, a temporary workspace and a fixture HTTP backend, not personal LocalCode tasks or live inference. For backend checks, follow the repository quality gates in `../../docs/QUALITY-GATES.md`.

`npm run package` produces the installable VSIX. Marketplace publication requires a publisher account named `inetconnector` and its registry-specific credential: `VSCE_PAT` for Visual Studio Marketplace and `OVSX_PAT` for Open VSX. Never commit tokens. `npm run publish:vsce` and `npm run publish:ovsx` are explicit release commands. A GitHub release/VSIX download is distinct from a searchable Marketplace listing.

Architecture: `src/extension.js` owns the IDE bridge and lifecycle; `src/client.js` owns bounded HTTP/SSE transport; `src/safety.js` validates messages and canonical paths; `media/` contains the sandboxed webview and language catalogs. `state.md` records exact verification and release status. Contributions should include focused behavior tests, both languages and updated documentation. Licensed under **Apache-2.0**; see [LICENSE](LICENSE).

## Deutsch

LocalCode arbeitet direkt neben deinem Code: mit eigener Seitenleiste und Editoransicht, gespeicherten Aufgaben, Live-Ausgabe, Modellwahl, Werkzeugfreigaben und nativen Git-Diffs. Das Projekt ist unabhängig von Microsoft, Google und Anthropic.

### Installation und Verwendung

1. [LocalCode Desktop](https://github.com/inetconnector/LocalCode/releases) installieren und starten. Vorrangige Folgeprompts und aufgabengebundener Abbruch benötigen das Backend des IDE-Erweiterungsreleases (`steering-v1`, `stop-task-v1`). Ältere Backends unterstützen den Chat; neue Steuerungen melden den Aktualisierungsbedarf.
2. `localcode-0.1.0.vsix` vom Release herunterladen. In VS Code oder Antigravity IDE unter Erweiterungen **… → Aus VSIX installieren…** wählen.
3. Einen **vertrauenswürdigen lokalen Ordner** innerhalb der in LocalCode Desktop eingestellten Projektwurzel öffnen. Das LocalCode-Symbol anklicken oder **Strg+Alt+L** drücken.
4. Aufgabe auswählen oder erstellen, Modell wählen und Auftrag senden. **Datei oder Auswahl hinzufügen** hängt Editor-Kontext ausdrücklich an; Dateien werden nicht automatisch übertragen.
5. Während der Ausführung weitere Hinweise senden. Neue Anweisungen werden beim nächsten sicheren Schritt vorrangig berücksichtigt. Abbruch bleibt eine eigene Schaltfläche. Freigaben zeigen Aktion, Befehl/Pfad und Vorschau vor **Einmal genehmigen** oder **Ablehnen**.

**LocalCode starten** öffnet ausdrücklich die vorhandene Windows-Installation. Beim Schließen der IDE läuft Desktop weiter. Ein anderer Programmpfad wird über `localcode.executablePath`, eine andere Loopback-Adresse über `localcode.serverUrl` eingestellt; Standard ist `http://127.0.0.1:32145`. Die Erweiterung lädt keine Programme oder Modelle automatisch herunter.

### Funktionen und Grenzen

Enthalten sind sichere Markdown-/Code-Darstellung, Kopieren, native Theme-Farben, Tastaturbedienung, Verlauf je Arbeitsordner, SSE mit Snapshot-Abgleich, Dateiauswahl, Editor-Diagnosen und Schutz ungespeicherter Dateien beim Start eines Auftrags. Angehängter Textkontext ist auf 64 KiB begrenzt. Git-Review vergleicht HEAD mit aktuellen Dateien einschließlich Editor-Inhalt; es ist keine atomare Wiederherstellung. Engine-Backups bleiben in Desktop verfügbar.

Neue Folgeprompts unterbrechen veraltete Modellinferenz; bereits gestartete Werkzeuge beenden ihren Schritt kontrolliert. Die Warteschlange bleibt an Aufgabe und Lauf gebunden. Neuere Benutzeranweisungen lösen Widersprüche auf, ohne Berechtigungen oder Laufbudgets zu erweitern. Grenzen: 32 KiB je Hinweis, 16 wartende Hinweise, 64 IDs und 128 KiB insgesamt je Lauf. Wiederholte Übertragungen verwenden dieselbe ID. Initialisierung, Abschluss und Mission-Recovery unterstützen diese Steuerung nicht. Die Warteschlange ist flüchtig; bei einem Prozessabsturz gehen unverarbeitete Hinweise verloren und werden nicht automatisch wiederholt. Übernommene Hinweise stehen im Chatverlauf; beim normalen Laufende nicht mehr verarbeitete Hinweise werden ausdrücklich gemeldet. Eingereiht bedeutet noch nicht vom Modell befolgt.

Deutsch und Englisch besitzen identische Katalogschlüssel. Automatisch gilt die von LocalCode gemeldete Windows-Anzeigesprache, ohne Verbindung ersatzweise die IDE-Sprache. Die manuelle Sprachwahl hat Vorrang.

Benötigt werden VS Code API 1.85+ beziehungsweise eine kompatible Antigravity IDE, das laufende LocalCode-Backend und ein Modell. Das Backend bleibt Windows-first. Remote SSH/WSL/Codespaces und virtuelle Arbeitsordner sind deaktiviert. Bei mehreren lokalen Arbeitsordnern wird einer gewählt. Git-Review benötigt die integrierte Git-Erweiterung und ein Repository direkt im gewählten Ordner.

Desktop und IDE teilen **einen aktiven Agentenlauf**. Mehrere IDE-Fenster können verschiedene Aufgaben ansehen, aber nicht gleichzeitig auf demselben Backend ausführen. Explizite Aufgaben-/Lauf-IDs verhindern mit aktuellem Backend, dass ein veralteter Stop-Klick einen anderen Lauf beendet.

Die Erweiterung verwendet die vorhandenen LocalCode-Werkzeuge und das gewählte Modell. Gleiche Denkqualität, proprietäre Cloud-Dienste, Inline-Autovervollständigung und vollständige Gleichwertigkeit mit Claude Code oder Antigravity sind nicht nachgewiesen. Bild-/Spracheingabe und erweiterte Engine-/MCP-/Recovery-Einstellungen bleiben in LocalCode Desktop.

### Entwicklung und Veröffentlichung

Die obigen Befehle prüfen Syntax, Sprachkataloge, Transport/Sicherheitsverhalten, die sichtbare Oberfläche und einen echten Extension Host. Entwicklung benötigt Node.js 22+, npm sowie Python Playwright/Edge für den Oberflächentest; die installierte Erweiterung besitzt keine externen Laufzeitpakete. `LOCALCODE_TEST_IDE` kann auf die installierte Antigravity-Programmdatei zeigen. Tests verwenden isolierte Profile und ein simuliertes Backend.

`npm run package` baut die VSIX. Marketplace-Publishing benötigt den Publisher `inetconnector` und jeweils `VSCE_PAT` beziehungsweise `OVSX_PAT`, niemals Tokens im Repository. Die expliziten Befehle sind `npm run publish:vsce` und `npm run publish:ovsx`. Ein GitHub-Download ist keine Marketplace-Listung. Änderungen müssen Verhaltenstests, beide Sprachen und Dokumentation pflegen. Architektur: IDE-Brücke in `src/extension.js`, Transport in `src/client.js`, Pfad-/Nachrichtenprüfung in `src/safety.js`, Webview und Sprachkataloge unter `media/`. Exakter Stand in `state.md`. Lizenz: **Apache-2.0**.
