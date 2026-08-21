// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  const i18n = window.LocalCodeI18n;
  if (i18n?.dictionaries) {
    Object.assign(i18n.dictionaries.de, {
      'Mission':'Mission',
      'Mission-Status':'Mission-Status',
      'Status':'Status',
      'Grund':'Grund',
      'Warteschlange':'Warteschlange',
      'Laufend':'Laufend',
      'Ressourcen':'Ressourcen',
      'Budget':'Budget',
      'Aufgaben':'Aufgaben',
      'Modellaufrufe':'Modellaufrufe',
      'Werkzeugaufrufe':'Werkzeugaufrufe',
      'Geschätzte Tokens':'Geschätzte Tokens',
      'Zeit':'Zeit',
      'verbraucht':'verbraucht',
      'unbegrenzt':'unbegrenzt',
      'erschöpft':'erschöpft',
      'Queue':'Queue',
      'blockiert':'blockiert',
      'running':'läuft',
      'succeeded':'erfolgreich',
      'failed':'fehlgeschlagen',
      'cancelled':'abgebrochen',
      'budget_exhausted':'Budget erschöpft',
      'completed':'abgeschlossen',
      'runtime_error':'Laufzeitfehler',
      'task_failed':'Task fehlgeschlagen',
      'task_budget_exhausted':'Task-Budget erschöpft',
      'mission_budget_exhausted':'Mission-Budget erschöpft',
      'incomplete':'unvollständig',
      'model-inference':'Modell-Inferenz',
      'read-cpu':'Read-CPU',
      'build':'Build',
      'exclusive-integration':'Exklusive Integration'
    });
    Object.assign(i18n.dictionaries.en, {
      'Mission':'Mission',
      'Mission-Status':'Mission status',
      'Status':'Status',
      'Grund':'Reason',
      'Warteschlange':'Queued',
      'Laufend':'Running',
      'Ressourcen':'Resources',
      'Budget':'Budget',
      'Aufgaben':'Tasks',
      'Modellaufrufe':'Model calls',
      'Werkzeugaufrufe':'Tool calls',
      'Geschätzte Tokens':'Estimated tokens',
      'Zeit':'Time',
      'verbraucht':'used',
      'unbegrenzt':'unlimited',
      'erschöpft':'exhausted',
      'Queue':'Queue',
      'blockiert':'blocked',
      'running':'running',
      'succeeded':'succeeded',
      'failed':'failed',
      'cancelled':'cancelled',
      'budget_exhausted':'budget exhausted',
      'completed':'completed',
      'runtime_error':'runtime error',
      'task_failed':'task failed',
      'task_budget_exhausted':'task budget exhausted',
      'mission_budget_exhausted':'mission budget exhausted',
      'incomplete':'incomplete',
      'model-inference':'model inference',
      'read-cpu':'read CPU',
      'build':'build',
      'exclusive-integration':'exclusive integration'
    });
  }

  const style = document.createElement('style');
  style.id = 'localcode-mission-status-style';
  style.textContent = `
    .mission-status-card{border-color:#40536a;background:#1b232a}
    .mission-status-head{display:flex;gap:8px;align-items:flex-start;margin-bottom:8px}
    .mission-status-title{font-weight:750;min-width:0;flex:1;overflow-wrap:anywhere}
    .mission-status-state{font-size:10px;border:1px solid #4b617a;border-radius:99px;padding:2px 7px;white-space:nowrap;color:#bcd7f5}
    .mission-status-state.succeeded{border-color:#386b49;color:#8ee2a7}.mission-status-state.failed,.mission-status-state.budget_exhausted{border-color:#7b4b4b;color:#ffaaaa}.mission-status-state.cancelled{border-color:#756543;color:#e3c980}
    .mission-status-meta,.mission-budget-grid,.mission-resource-grid{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 10px;font-size:10.5px;margin-top:7px}
    .mission-status-meta span:nth-child(odd),.mission-budget-grid span:nth-child(odd),.mission-resource-grid span:nth-child(odd){color:#94a1a8}
    .mission-status-section{margin-top:10px;padding-top:9px;border-top:1px solid #313c43}.mission-status-section-title{font-size:10px;text-transform:uppercase;letter-spacing:.08em;color:#92a0a7;font-weight:700;margin-bottom:6px}
    .mission-task{border:1px solid #313c43;background:#171d21;border-radius:7px;padding:7px 8px;margin-top:5px}.mission-task-head{display:flex;gap:7px;align-items:center}.mission-task-id{min-width:0;flex:1;font:10.5px var(--code-font);overflow-wrap:anywhere}.mission-task-state{font-size:9.5px;color:#b5c4cc}.mission-task-sub{font-size:9.5px;color:#839198;margin-top:3px;overflow-wrap:anywhere}
  `;
  document.head.appendChild(style);

  const tr = text => window.LocalCodeI18n?.t ? window.LocalCodeI18n.t(text) : text;
  const int = value => Number.isFinite(Number(value)) ? Number(value) : 0;

  function stateLabel(value) {
    return tr(String(value || ''));
  }

  function budgetRows(snapshot) {
    const limit = snapshot?.limit || {};
    const usage = snapshot?.usage || {};
    const entries = [
      ['Modellaufrufe', int(usage.model_calls), int(limit.model_calls)],
      ['Werkzeugaufrufe', int(usage.tool_calls), int(limit.tool_calls)],
      ['Geschätzte Tokens', int(usage.estimated_tokens), int(limit.estimated_token_budget)],
      ['Zeit', Math.ceil(int(usage.elapsed_millis) / 1000), int(limit.time_seconds)]
    ];
    return entries.map(([label, used, max]) => [label, max > 0 ? `${used}/${max} ${tr('verbraucht')}` : `${used} · ${tr('unbegrenzt')}`]);
  }

  function appendGrid(parent, className, rows) {
    const grid = document.createElement('div');
    grid.className = className;
    for (const [label, value] of rows) {
      const key = document.createElement('span');
      key.textContent = tr(label);
      const val = document.createElement('span');
      val.textContent = String(value ?? '');
      grid.append(key, val);
    }
    parent.appendChild(grid);
  }

  function section(parent, title) {
    const wrap = document.createElement('div');
    wrap.className = 'mission-status-section';
    const heading = document.createElement('div');
    heading.className = 'mission-status-section-title';
    heading.textContent = tr(title);
    wrap.appendChild(heading);
    parent.appendChild(wrap);
    return wrap;
  }

  function buildMissionCard(mission) {
    const card = document.createElement('div');
    card.className = 'output-card mission-status-card';
    card.dataset.missionStatus = '1';

    const head = document.createElement('div');
    head.className = 'mission-status-head';
    const title = document.createElement('div');
    title.className = 'mission-status-title';
    title.textContent = `${tr('Mission')}: ${mission.mission_id || ''}`;
    const stateChip = document.createElement('div');
    stateChip.className = `mission-status-state ${mission.state || ''}`;
    stateChip.textContent = stateLabel(mission.state);
    head.append(title, stateChip);
    card.appendChild(head);

    const scheduler = mission.scheduler || {};
    const meta = [
      ['Status', stateLabel(mission.state)],
      ['Grund', mission.reason ? stateLabel(mission.reason) : '—'],
      ['Warteschlange', int(scheduler.queued)],
      ['Laufend', int(scheduler.running)]
    ];
    appendGrid(card, 'mission-status-meta', meta);

    const budget = section(card, 'Budget');
    appendGrid(budget, 'mission-budget-grid', budgetRows(mission.budget));
    if (mission.budget?.exhausted) {
      const exhausted = document.createElement('div');
      exhausted.className = 'mission-task-sub';
      exhausted.textContent = `${tr('erschöpft')}: ${mission.budget.exhausted_by || mission.budget_exhausted_by || '—'}`;
      budget.appendChild(exhausted);
    }

    const resources = section(card, 'Ressourcen');
    appendGrid(resources, 'mission-resource-grid', (scheduler.resources || []).map(resource => [
      stateLabel(resource.class), `${int(resource.in_use)}/${int(resource.limit)}`
    ]));

    const tasks = section(card, 'Aufgaben');
    for (const task of scheduler.tasks || []) {
      const row = document.createElement('div');
      row.className = 'mission-task';
      const rowHead = document.createElement('div');
      rowHead.className = 'mission-task-head';
      const id = document.createElement('div');
      id.className = 'mission-task-id';
      id.textContent = task.task_id || '';
      const taskState = document.createElement('div');
      taskState.className = 'mission-task-state';
      taskState.textContent = stateLabel(task.state);
      rowHead.append(id, taskState);
      row.appendChild(rowHead);
      const details = [];
      if (task.resource_class) details.push(stateLabel(task.resource_class));
      if (int(task.queue_position) > 0) details.push(`${tr('Queue')} #${int(task.queue_position)}`);
      if (task.admission_blocked_reason) details.push(`${tr('blockiert')}: ${task.admission_blocked_reason}`);
      if (task.budget?.exhausted) details.push(`${tr('Budget')}: ${tr('erschöpft')} (${task.budget.exhausted_by || '—'})`);
      if (details.length) {
        const sub = document.createElement('div');
        sub.className = 'mission-task-sub';
        sub.textContent = details.join(' · ');
        row.appendChild(sub);
      }
      tasks.appendChild(row);
    }
    return card;
  }

  const originalRenderInspector = renderInspector;
  renderInspector = function() {
    originalRenderInspector();
    if (state.rightTab !== 'outputs') return;
    const mission = state.status?.mission;
    if (!mission) return;
    const body = document.querySelector('#rightBody');
    const list = body?.querySelector('.output-list');
    if (!body) return;
    body.querySelector('[data-mission-status]')?.remove();
    const card = buildMissionCard(mission);
    if (list) list.prepend(card);
    else body.prepend(card);
  };

  let sawRunningMission = false;
  let refreshInFlight = false;
  async function refreshMissionStatus() {
    if (refreshInFlight) return;
    const phase = state.runPhase || state.status?.run_phase || '';
    if (phase !== 'mission-read-only' && !sawRunningMission) return;
    refreshInFlight = true;
    try {
      const status = await api('/api/status');
      state.status = status;
      const mission = status?.mission || null;
      sawRunningMission = !!mission && mission.state === 'running';
      renderInspector();
    } catch (_) {
      // The normal health monitor owns connectivity errors. Mission telemetry is optional UI observation.
    } finally {
      refreshInFlight = false;
    }
  }

  window.addEventListener('load', () => {
    const mission = state.status?.mission;
    sawRunningMission = !!mission && mission.state === 'running';
    renderInspector();
    setInterval(refreshMissionStatus, 1500);
    document.addEventListener('localcode:language', () => setTimeout(renderInspector, 0));
  });
})();
