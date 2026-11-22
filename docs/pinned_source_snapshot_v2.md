# Commit-gepinnter LocalCode-Quell-Snapshot – Version 2

Stand: 22.11.2026. Der Quell-Snapshot-Prüfer
`scripts/New-LocalCodePinnedSnapshot-v2.ps1` erstellt **keinen**
Go-Testlauf, startet keine Mission und installiert keinen Bridge-Task.
Er prüft ausschließlich einen bereits vorhandenen, vollständig
benannten Git-Commit und kann dessen versionierte `src`-Dateien in
ein **neues** lokales Verzeichnis extrahieren.

## Warum Version 2?

Der vorhandene generische TrustedRunner hat eine isolierte
LocalCode-Python-Prüfung mit NO_GO gestoppt, weil ein ignorierter
Go-Werkzeugpfad nicht zu seiner Snapshot-Richtlinie passte.
Die Schutzrichtlinie darf deshalb nicht abgeschwächt und der
ignorierte Ordner weder gelöscht noch in den Test übernommen werden.

Die andere Quelle `scripts/Test-LocalCodePinnedGo.ps1` wurde
während paralleler Bearbeitung zeitweise beschädigt und inzwischen
in Commit `e85f91d` gesondert eingecheckt. Diese V2-Variante bleibt
davon bewusst unabhängig: **nur Vorschau und Quell-Snapshot**,
keine Docker-Ausführung. Vor Verwendung einer der beiden Varianten
müssen Quellcode und exakte Dateiprüfsumme separat geprüft werden.

## Vorschau ohne Änderungen

Aus einer normalen PowerShell:

```powershell
$p = '.\scripts\New-LocalCodePinnedSnapshot-v2.ps1'
& $p -ExpectedCommit '703eb84632b8174b9c7f6a295e6ea62ac32b09ea'
```

Voraussetzung ist, dass das Skript selbst bereits in LocalCode
eingecheckt ist und mit dem dort gespeicherten Git-Blob
übereinstimmt. Der ausgewählte Commit muss im Repository vorhanden
und Vorfahr des aktuell ausgecheckten Feature-Branches sein.
**Der aktuelle HEAD muss nicht dem Testcommit entsprechen.**

Die Vorschau prüft Git-Typen, Dateizahlen, Dateigrößen, Dateinamen,
Verzeichnisgrenzen, Reparse-/Symlink-Schutz und eine begrenzte
Whitelist von Quell-Dateitypen. Sie benötigt keinen Go-Compiler,
keine Credentials und keine Netzwerkverbindung.

Erwartet: `PINNED_SNAPSHOT_PREVIEW_PASS`, `mutations=0`.

## Snapshot erzeugen – nur nach unabhängiger Hash-Prüfung

Vor dem Ausführen die **genaue SHA-256-Prüfsumme des eingecheckten
Skripts unabhängig prüfen** (nicht einen vom Skript selbst
behaupteten Wert übernehmen). Den bestätigten Hash an zwei Stellen
verwenden:

```powershell
$p = '.\scripts\New-LocalCodePinnedSnapshot-v2.ps1'
$approved = '<extern-gepruefter-64-stelliger-SHA256>'
if ((Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash.ToLowerInvariant() -ne $approved) {
    throw 'Skriptprüfung fehlgeschlagen'
}
& $p -ExpectedCommit '703eb84632b8174b9c7f6a295e6ea62ac32b09ea' -CreateSnapshot -ApprovedScriptSha256 $approved
```

Der Prüfer führt ausschließlich `git archive` auf `src`
des erwarteten Commits aus. Ignorierte oder unversionierte Dateien
(`.tools`, private Daten, Benutzerprofile, Credential-Stores),
die lokale Git-Datenbank und veränderte Workspace-Dateien werden
**nicht** eingepackt. Das Archiv wird vor der Extraktion auf
sichere, im Commit aufgeführte Dateien geprüft. Nach der Extraktion
wird **jede Datei über ihren Git-Blob-Hash** mit dem Commit
verglichen. Für den neuen Snapshot wird eine zufällige Kennung
verwendet; bestehende Snapshots werden nicht überschrieben oder
gelöscht. Das temporäre ZIP wird im Abschlussblock entfernt.

Erwartet: `PINNED_SNAPSHOT_PASS` sowie
`NO_TEST_RAN; NO_NETWORK_REQUEST; NO_RELEASE_APPROVAL`.
Der Snapshot entsteht ausschließlich unter dem lokalen
`_workspace/LocalCodePinnedSnapshots`-Verzeichnis neben dem
LocalCode-Checkout. Der Pfad selbst wird nicht in Receipts,
Logs oder Browser-Telemetrie übertragen.

## Was noch nicht getestet oder freigegeben ist

Dieser Baustein führt **weder `go test` noch `go vet`** aus.
Dafür fehlt weiterhin ein gesondert genehmigter, lokal
vorhandener, digest-gepinnter Go-Testcontainer mit Go 1.26.6
und den offline verfügbaren Abhängigkeiten. Ein nächster
Operator-Prüfschritt muss ohne Netzwerk, ohne Credential-Mounts,
ohne unverschlüsselte Datenübertragung und mit unabhängig
kontrollierten Prozess-/Containergrenzen stattfinden.

Das Git-Archiv beweist, dass keine ungetrackten Dateien
einfließen. Es beweist **nicht**, dass jeder jemals eingecheckte
Quellinhalt frei von vertraulichen Daten ist. Vor einer
externen Übermittlung oder einem neuen Container-Mount
ist daher eine eigenständige Quellen-/Geheimnisprüfung erforderlich.

Der bestehende `LocalCode`-Branch, seine ignorierten Dateien
und die NomaMica-/NOMA-Sync-Dateien werden durch diesen
Prüfer nicht verändert. Kein Push, keine Registry-Mutation,
kein Release, keine autonome Entwicklungsausführung.
**RELEASE_NO_GO.**
