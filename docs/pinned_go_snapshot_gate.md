# Sauberer, commit-gepinnter LocalCode-Go-Test-Snapshot

Stand 2026-11-22. **Quelltest, keine Live- oder Release-Freigabe.**

Der versionierte Operator-Prüfer
\`scripts/Test-LocalCodePinnedGo.ps1\` erzeugt bei ausdrücklicher
Anforderung einen separaten Git-Archiv-Snapshot. Er nimmt nur
versionierte \`src\`-Dateien eines ausdrücklich angegebenen, bereits
geprüften 40-stelligen Commit-SHA auf. Ungetrackte/ignorierte
\`.tools\`-Dateien, persönliche Dateien, Git-Metadaten und
uncommittete Änderungen werden niemals kopiert.

## Schreiberfreie Vorschau

In normaler PowerShell im lokalen LocalCode-Checkout:

\`\`\`powershell
& .\scripts\Test-LocalCodePinnedGo.ps1 -ExpectedCommit '<gepruefter-40-stelliger-commit-sha>'
\`\`\`

Erwartet: \`PINNED_GO_PREVIEW_PASS ... mutations=0\`.
Die Vorschau prüft die lokale Branch- und Git-Historie, ob der
vorgegebene Commit ein Vorfahre des aktuellen Feature-HEAD ist,
und ob das Prüfer-Skript genau dem eigenen committed Git-Blob
entspricht. Auch alle Dateimodi, eindeutige Pfade, Dateitypen,
Dateigrößen und eine starke Geheimnis-Signaturprüfung werden
kontrolliert. Fehlende oder gefährliche Eingaben werden ohne
Wiedergabe von Datei-Inhalten abgelehnt.

## Separaten Quell-Snapshot anlegen

Nur nach **unabhängiger** SHA256-Prüfung des vollständigen,
versionierten Prüfers:

\`\`\`powershell
& .\scripts\Test-LocalCodePinnedGo.ps1 -ExpectedCommit '<gepruefter-40-stelliger-commit-sha>' -CreateSnapshot -ApprovedScriptSha256 '<unabhaengig-gepruefter-64-stelliger-sha256>'
\`\`\`

Erwartet: \`PINNED_GO_SNAPSHOT_PASS\` und
\`NO_TEST_RAN; NO_RELEASE_APPROVAL; NO_NETWORK_REQUEST\`.

Der Snapshot wird unter \`_workspace/LocalCodePinnedSnapshots\`
mit einer neuen eindeutigen Kennung erzeugt. Er verändert keine
Branch-Referenzen, keinen Index, keinen laufenden Quell-Checkout
und keinen vorhandenen Snapshot. Die ZIP-Dateiliste wird mit
der zuvor geprüften Git-Manifest-Liste verglichen und danach
jeder extrahierte Blob mit Git erneut gegen den Commit geprüft.
Der Prüfer ersetzt **nicht** die TrustedRunner-Sicherheitsrichtlinie.

**Wichtig:** Der aktuelle Feature-HEAD darf später durch
neue Commits weiterlaufen; der vorgegebene ältere Commit
bleibt unverändert. Nur bei fehlender Vorfahren-Beziehung
oder geänderter Prüferquelle lautet das Ergebnis BLOCKED.

## Optionale Go-Tests im abgeschotteten Docker-Container

Das Test-Gate besitzt **keine** Berechtigung, ein Image
herunterzuladen. \`-RunTests\` erfordert ein separat geprüftes,
bereits vorhandenes, digest-gepinntes Linux-Go-Image mit
Go 1.26.6 und seinen Offline-Abhängigkeiten:

\`\`\`powershell
& .\scripts\Test-LocalCodePinnedGo.ps1 -ExpectedCommit '<gepruefter-40-stelliger-commit-sha>' -RunTests -ApprovedScriptSha256 '<unabhaengig-gepruefter-sha256>' -ApprovedGoImage '<lokal-vorhandenes-geprueftes-image@sha256:64-hex>'
\`\`\`

Der Docker-Container erhält nur \`src\` schreibgeschützt,
kein Host-Home, keine lokalen Credential-Verzeichnisse,
keine Secret-Variablen, kein Docker-Socket und kein Netzwerk
(\`--network=none\`, \`--pull=never\`, minimierte Rechte,
festgelegte CPU-/RAM-/PID-Grenzen). Roh-Testausgaben
werden unterdrückt. Die Meldungen
\`PINNED_GO_FOCUSED_PASS\` bzw. \`PINNED_GO_FULL_PASS\`
erscheinen **nur** nach tatsächlich erfolgreichem Test-Exit.

Ohne ein geprüftes Image bleibt der vollständige isolierte
Go-Test offen. Die Git-Diff-Prüfung und die separate
NomaMica-Kompatibilitätssuite sind **kein** Ersatz.

Das Quellscan-Gate entdeckt einige bekannte Secret-Muster;
es garantiert nicht, dass alle möglichen Geheimnisse im
versionierten Quelltext erkannt werden. Auch ein Docker-Test
ist kein Nachweis einer vollständigen OS-Attestierung oder
authentifizierten, verschlüsselten Live-Telemetrie.

**RELEASE_NO_GO. Kein produktiver Start, kein Git-Push,
keine neue TrustedBridge-Taskregistrierung.**
