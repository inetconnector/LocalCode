# LocalCode 4.3.0

Diese Version behebt die konkret gemeldeten Laufzeitprobleme.

- Keine kurz aufblinkenden Konsolenfenster mehr bei Hintergrundbefehlen unter Windows.
- Werkzeugausgaben, Befehlsausgaben, Diffs und Fehler erscheinen vollständig im Chat und im Ausgabenbereich.
- Rückfragen werden als Fortsetzung desselben Agentenlaufs behandelt; eine Antwort wie „ja“ startet nicht wieder von vorn.
- Dauerhaft sichtbare Abbruchsteuerung im Composer und in der Kopfzeile.
- Modellaufrufe besitzen ein konfigurierbares Zeitlimit.
- Kontrollierter Abbruch mit automatischer Notfallfreigabe der Oberfläche.
- SSE-Verbindung und Status werden regelmäßig mit dem Server abgeglichen.
- Einstellungen werden vorwärtskompatibel gespeichert und dürfen auch während eines Agentenlaufs geändert werden.
- Speicherfehler zeigen eine genaue Servermeldung statt nur „Bad Request“.

Die Standardwerte sind 240 Sekunden je Modellaufruf und 300 Sekunden je Befehl.
