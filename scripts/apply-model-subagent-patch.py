from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected anchor once, found {count}")
    return text.replace(old, new, 1)


agent_path = Path("src/agent.go")
text = agent_path.read_text(encoding="utf-8")
if 'runReadOnlyModelSubagent(ctx, project, cfg, a.Task)' not in text:
    text = replace_once(
        text,
        '- Nutze subagent_analyze(task), wenn eine getrennte, unverändernde Exploration oder Testlog-Analyse sinnvoll ist. Diese Aktion liest nur Projektkontext, ausdrücklich erwähnte Dateien und Suchtreffer und liefert einen strukturierten Handoff; sie darf keine Dateien ändern, keine Befehle ausführen, keine Netzwerkzugriffe starten und keine MCP-Tools aufrufen.\n',
        '- Nutze subagent_analyze(task), wenn eine getrennte Exploration oder Testlog-Analyse sinnvoll ist. LocalCode startet dafür einen isolierten read-only Modell-Worker mit eigenem Kontext und hartem Schrittbudget. Der Child darf ausschließlich list/read/search/LSP/finish nutzen, niemals Dateien ändern, Shell/Git/MCP/Netzwerk ausführen, Installationen starten oder weitere Subagenten erzeugen; bei Modellfehler wird deterministisch auf statische Repository-Analyse zurückgefallen.\n',
        "subagent prompt",
    )
    text = replace_once(
        text,
        '\tcase "subagent_analyze":\n\t\tresult, err = runReadOnlySubagent(project, cfg, a.Task)',
        '\tcase "subagent_analyze":\n\t\tresult, err = s.runReadOnlyModelSubagent(ctx, project, cfg, a.Task)',
        "subagent dispatch",
    )
    agent_path.write_text(text, encoding="utf-8", newline="\n")

supervisor_path = Path("src/agent_supervisor.go")
supervisor = supervisor_path.read_text(encoding="utf-8")
if '"project_info", "subagent_analyze", "tool_inventory"' not in supervisor:
    supervisor = replace_once(
        supervisor,
        'case "project_info", "tool_inventory", "discover_tool",',
        'case "project_info", "subagent_analyze", "tool_inventory", "discover_tool",',
        "analysis supervisor subagent",
    )
    supervisor_path.write_text(supervisor, encoding="utf-8", newline="\n")
