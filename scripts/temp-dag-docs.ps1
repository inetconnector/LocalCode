$ErrorActionPreference = 'Stop'
$branch = 'feat/native-agent-task-dag'
git fetch origin $branch
if ($LASTEXITCODE -ne 0) { throw 'Failed to fetch feature branch' }
git checkout -B $branch "origin/$branch"
if ($LASTEXITCODE -ne 0) { throw 'Failed to checkout feature branch' }

$readmeMarker = '## Native Agent Task DAG / Native-Agent-Aufgabengraph'
$architectureMarker = '## Native Agent Task DAG (Phase 4)'
$securityMarker = '## Native Agent Task DAG security boundary / Sicherheitsgrenze'

$readme = [IO.File]::ReadAllText('README.md')
if (-not $readme.Contains($readmeMarker)) {
  $append = @'

## Native Agent Task DAG / Native-Agent-Aufgabengraph

**DE:** LocalCode Native kann Planner-Vorschläge jetzt als deterministischen, maschinenlesbaren Task-DAG modellieren. Jede vorgeschlagene Aufgabe besitzt eine stabile ID; doppelte/fehlende/zyklische Abhängigkeiten werden verworfen. Dynamische Rollenbezeichnungen und angeforderte Capabilities sind ausschließlich Planungsdaten und gewähren weder Ausführungsrechte noch neue Tools. Dieser Schritt plant und bewertet Readiness; er startet noch keine mutierenden Builder, Worktrees oder parallelen Scheduler-Jobs.

**EN:** LocalCode Native can now model Planner proposals as a deterministic machine-readable task DAG. Every proposed task has a stable ID; duplicate, missing, self-referential, or cyclic dependencies are rejected. Dynamic role labels and requested capabilities are planning data only and never grant execution authority or new tools. This phase models planning/readiness only; it does not yet start mutating Builders, worktrees, or parallel scheduler jobs.
'@
  [IO.File]::WriteAllText('README.md', $readme.TrimEnd() + $append + "`n", [Text.UTF8Encoding]::new($false))
}

$architecture = [IO.File]::ReadAllText('docs/ARCHITECTURE.md')
if (-not $architecture.Contains($architectureMarker)) {
  $append = @'

## Native Agent Task DAG (Phase 4)

Planner `suggested_tasks` are converted into `AgentTaskGraph` data with stable task IDs, mission/parent metadata, explicit dependencies, requested capabilities, and deterministic task states. Graph construction validates identifiers, duplicate or missing dependencies, self-dependencies, cycles, and state/mission consistency before the graph is exposed to the parent. Dependency reconciliation derives ready/blocked state deterministically and propagates failed/cancelled dependencies without executing a task.

Dynamic role labels remain data. Executable Native child roles are still restricted by the existing runtime role/capability validation. `RequestedCapabilities` records what a plan asks for; it is deliberately separate from granted `Capabilities`. Task-DAG construction therefore cannot escalate authority. Scheduler/resource queues, durable missions, mutation-capable Builders, worktrees, Integrator/Test-Agent execution, and OS/QEMU missions remain later layers above this contract.

Deutsch: Der Task-DAG ist eine validierte Planungsschicht, keine neue Berechtigungsschicht. Rollenbezeichnungen und angeforderte Fähigkeiten in Planner-Vorschlägen bleiben Daten; tatsächliche Ausführungsrechte werden weiterhin ausschließlich durch die LocalCode-Governance vergeben.
'@
  [IO.File]::WriteAllText('docs/ARCHITECTURE.md', $architecture.TrimEnd() + $append + "`n", [Text.UTF8Encoding]::new($false))
}

$security = [IO.File]::ReadAllText('docs/SECURITY.md')
if (-not $security.Contains($securityMarker)) {
  $append = @'

## Native Agent Task DAG security boundary / Sicherheitsgrenze

Task-DAG validation does not widen the child-agent trust boundary. Planner proposals may name dynamic roles and request capabilities, but these values are inert until a later governance layer explicitly maps them to an allowed executable role and grants capabilities. Graph construction copies requests into `RequestedCapabilities` and leaves granted `Capabilities` empty. Invalid IDs, missing/duplicate/self dependencies, cycles, inconsistent mission/parent metadata, and invalid task states fail closed.

Deutsch: Der DAG vergibt keine Rechte. Dynamische Rollen und angeforderte Capabilities aus einem Plan werden nicht automatisch ausführbar. Bestehende Sandbox-, Approval-, SHA-/Precondition-, Atomic-Write-, Mobile- und Prozessgrenzen bleiben unverändert; insbesondere führt dieser Phase-4-Slice keine mutierenden Child-Agents, Worktrees oder parallelen Schreibzugriffe ein.
'@
  [IO.File]::WriteAllText('docs/SECURITY.md', $security.TrimEnd() + $append + "`n", [Text.UTF8Encoding]::new($false))
}

git diff --check
if ($LASTEXITCODE -ne 0) { throw 'Documentation diff check failed' }

git config user.name 'LocalCode CI'
git config user.email 'localcode-ci@users.noreply.github.com'
git add -- README.md docs/ARCHITECTURE.md docs/SECURITY.md
if ((git diff --cached --name-only).Count -eq 0) {
  Write-Host 'DAG documentation already present; no branch mutation needed.'
  exit 0
}
git diff --cached --check
if ($LASTEXITCODE -ne 0) { throw 'Staged documentation diff check failed' }
git commit -m 'docs: document native agent task DAG boundary'
if ($LASTEXITCODE -ne 0) { throw 'Failed to commit DAG documentation' }
git push origin HEAD:$branch
if ($LASTEXITCODE -ne 0) { throw 'Failed to push DAG documentation' }
