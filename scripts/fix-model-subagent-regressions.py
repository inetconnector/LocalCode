from pathlib import Path

agent_path = Path("src/agent.go")
agent = agent_path.read_text(encoding="utf-8")
verbose = '- Nutze subagent_analyze(task), wenn eine getrennte Exploration oder Testlog-Analyse sinnvoll ist. LocalCode startet dafür einen isolierten read-only Modell-Worker mit eigenem Kontext und hartem Schrittbudget. Der Child darf ausschließlich list/read/search/LSP/finish nutzen, niemals Dateien ändern, Shell/Git/MCP/Netzwerk ausführen, Installationen starten oder weitere Subagenten erzeugen; bei Modellfehler wird deterministisch auf statische Repository-Analyse zurückgefallen.\\n'
compact = '- Nutze subagent_analyze(task) für isolierte read-only Exploration; der Child darf nur lesen/suchen/LSP nutzen und nie mutieren.\\n'
if verbose in agent:
    agent = agent.replace(verbose, compact, 1)
agent_path.write_text(agent, encoding="utf-8", newline="\n")

sub_path = Path("src/subagent_model.go")
sub = sub_path.read_text(encoding="utf-8")
old = 's.AddEvent(UIEvent{Type: "agent_step", Message: "Subagent: " + action.Message, Action: "subagent:" + action.Action, Path: action.Path})'
new = 's.AddEvent(UIEvent{Type: "agent_step", Message: localizeConfigText(cfg, "Subagent: ", "Subagent: ") + action.Message, Action: "subagent:" + action.Action, Path: action.Path})'
if old in sub:
    sub = sub.replace(old, new, 1)
sub_path.write_text(sub, encoding="utf-8", newline="\n")
