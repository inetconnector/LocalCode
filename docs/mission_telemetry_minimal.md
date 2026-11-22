# LocalCode: minimaler Missionsstatus für eine zukünftige Betreiberanbindung

Stand: 22.11.2026. **Nur interner Quellcode, keine Live-Anbindung und keine Ausführungsfreigabe.**

Die internen Go-Funktionen in `src/agent_mission_telemetry_minimal.go` lesen
`AgentMissionDesktopStatus` beziehungsweise den existierenden
`agentMissionDesktopStatusForRun`-Speicher. Der gebundene Readback
`agentMissionMinimalJSONForBoundRun` fordert vor der Projektion
übereinstimmende Run-ID, Mission-ID **und den erwarteten Projektwert**.
Der Projektwert wird nur intern verglichen und niemals serialisiert;
die erwarteten Werte müssen außerhalb dieses Codes vertrauenswürdig
festgelegt sein. Die Funktionen konstruieren **vor jeder
Serialisierung** ein neues, fest typisiertes Statusobjekt. Sie rufen
keine Netzwerk-, HTTP-, IPC- oder Logging-Funktion auf.

Die Ausgabe enthält exakt sieben Top-Level-Felder:

- `schema_version`: `nomamica.localcode.desktop-minimal.v1`
- `mission_id`: maximal 128 Zeichen, internes Kennzeichen
- `execution_run_id`: maximal 128 Zeichen, internes Kennzeichen
- `state`: `running`, `succeeded`, `failed`, `cancelled` oder `budget_exhausted`
- `started_at` und `updated_at`: Zeitstempel in UTC
- `scheduler`: ausschließlich `mission_id`, `queued`, `running`

Die JSON-Ausgabe ist auf 1.024 Byte begrenzt. Bei ungültigen Werten liefert
die Funktion nur `MINIMAL_TELEMETRY_INVALID` und keine Eingabewerte.
Projektpfade, Modellnamen, Zugangsdaten, Tokens, Prompts, Tool-Ausgaben,
Budgetdetails, Task-IDs und sämtliche freien Texte werden nicht gelesen
und nicht serialisiert.

**Wichtige Einschränkung:** Der Quellcode allein kann nicht erkennen, ob
jemand absichtlich Geheimnisse in erlaubte Kennzeichenfelder schreibt.
Eine spätere Betreiberverbindung muss IDs aus einer separaten
vertrauenswürdigen Quelle prüfen, jeden Snapshot gegen eine genehmigte
Session und OS-Prozessidentität binden und den Transport verschlüsseln.
Der öffentliche Browser darf keine IDs frei wählen oder Daten hochladen.

Es gibt aktuell **keine** Transportintegration. Insbesondere nicht
implementiert oder freigegeben sind: ein verschlüsselter und gegenseitig
authentifizierter Kanal, OS-Prozess- und Sandbox-Attestierung,
Replay-Schutz, geheimes Schlüsselmanagement und ein echter Coding-Run.

Die Go-Tests `src/agent_mission_telemetry_minimal_test.go` verwenden
ausschließlich erfundene Canaries und den echten LocalCode-Go-Typ.
Sie testen Feld-Whitelists, ungültige Werte, feste Fehler und den
internen Speicher-Readback. Weder die Tests noch das neue Modul
starten eine Mission.

**Sicherheitsstatus: RELEASE_NO_GO.** Kein Push, kein Deployment,
keine Freigabe autonomer Codeausführung durch diesen Commit.
