// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  const i18n = window.LocalCodeI18n;
  if (i18n?.dictionaries) {
    Object.assign(i18n.dictionaries.de, {
      'Neues Projekt':'Neues Projekt',
      'Neuer Ordner':'Neuer Ordner',
      'Papierkorb':'Papierkorb',
      'Neues Projekt anlegen':'Neues Projekt anlegen',
      'Erstellt ein LocalCode-Projekt mit README.md, AGENTS.md und STATE.md.':'Erstellt ein LocalCode-Projekt mit README.md, AGENTS.md und STATE.md.',
      'Projekt anlegen':'Projekt anlegen',
      'Projekt wurde angelegt.':'Projekt wurde angelegt.',
      'Neuen Ordner anlegen':'Neuen Ordner anlegen',
      'Legt einen absichtlich leeren Ordner direkt unter der aktuellen Projektwurzel an.':'Legt einen absichtlich leeren Ordner direkt unter der aktuellen Projektwurzel an.',
      'Ordner anlegen':'Ordner anlegen',
      'Ordner wurde leer angelegt.':'Ordner wurde leer angelegt.',
      'Ordner umbenennen':'Ordner umbenennen',
      'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.':'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.',
      'Umbenennen':'Umbenennen',
      'Ordner wurde umbenannt.':'Ordner wurde umbenannt.',
      'Löschen…':'Löschen…',
      'Leeren Ordner löschen?':'Leeren Ordner löschen?',
      'Der Ordner ist leer und wird nach dieser Bestätigung direkt gelöscht.':'Der Ordner ist leer und wird nach dieser Bestätigung direkt gelöscht.',
      'Leeren Ordner löschen':'Leeren Ordner löschen',
      'Leerer Ordner wurde gelöscht.':'Leerer Ordner wurde gelöscht.',
      'Projekt in den Papierkorb verschieben?':'Projekt in den Papierkorb verschieben?',
      'Dieser Projektordner enthält {files} Dateien, {dirs} Unterordner und {bytes} Bytes. Er wird in den LocalCode-Papierkorb verschoben und kann wiederhergestellt werden.':'Dieser Projektordner enthält {files} Dateien, {dirs} Unterordner und {bytes} Bytes. Er wird in den LocalCode-Papierkorb verschoben und kann wiederhergestellt werden.',
      'In Papierkorb verschieben':'In Papierkorb verschieben',
      'Bestätigung stimmt nicht exakt mit dem Projektnamen überein.':'Bestätigung stimmt nicht exakt mit dem Projektnamen überein.',
      'Projekt wurde in den Papierkorb verschoben und kann wiederhergestellt werden.':'Projekt wurde in den Papierkorb verschoben und kann wiederhergestellt werden.',
      'Papierkorb ist leer.':'Papierkorb ist leer.',
      'Wiederherstellen':'Wiederherstellen',
      'Dauerhaft löschen…':'Dauerhaft löschen…',
      'Dauerhaftes Löschen kann nicht rückgängig gemacht werden. Gib exakt “PURGE {project}” ein.':'Dauerhaftes Löschen kann nicht rückgängig gemacht werden. Gib exakt “PURGE {project}” ein.',
      'Projekt wurde wiederhergestellt.':'Projekt wurde wiederhergestellt.',
      'Projekt wurde dauerhaft aus dem Papierkorb gelöscht.':'Projekt wurde dauerhaft aus dem Papierkorb gelöscht.',
      'Dateien: {files} · Ordner: {dirs} · Bytes: {bytes}':'Dateien: {files} · Ordner: {dirs} · Bytes: {bytes}',
      'Schließen':'Schließen',
      'Anzeigenamen bearbeiten':'Anzeigenamen bearbeiten',
      'Claw Code (experimentell)':'Claw Code (experimentell)',
      'Extras':'Extras',
      'Remote koppeln':'Remote koppeln',
      'Scanne den QR-Code mit der Smartphone-Kamera oder der LocalCode Remote App.':'Scanne den QR-Code mit der Smartphone-Kamera oder der LocalCode Remote App.',
      'Pairing-Code':'Pairing-Code',
      'Kopieren':'Kopieren',
      'Code in die Zwischenablage kopiert.':'Code in die Zwischenablage kopiert.',
      'Fertig':'Fertig',
      'Hallo, wobei kann ich Ihnen helfen?':'Hallo, wobei kann ich Ihnen helfen?',
      'Beschreibe eine Aufgabe oder nutze einen der Starter-Vorschläge für dein Projekt.':'Beschreibe eine Aufgabe oder nutze einen der Starter-Vorschläge für dein Projekt.',
      'Architektur analysieren':'Architektur analysieren',
      'Analysiere die Projektarchitektur und liste Kernkomponenten auf.':'Analysiere die Projektarchitektur und liste Kernkomponenten auf.',
      'Tests ausführen':'Tests ausführen',
      'Führe alle Tests und die Codequalitätsprüfung aus.':'Führe alle Tests und die Codequalitätsprüfung aus.',
      'Git-Review':'Git-Review',
      'Prüfe den aktuellen Git-Status, geänderte Dateien und Diffs.':'Prüfe den aktuellen Git-Status, geänderte Dateien und Diffs.',
      'Release bauen':'Release bauen',
      'Prüfe die Release-Bereitschaft und erstelle das Release-Paket.':'Prüfe die Release-Bereitschaft und erstelle das Release-Paket.',
      'Pacman Arcade':'Pacman Arcade',
      'Starte das Pacman Arcade Demo und prüfe die Steuerung.':'Starte das Pacman Arcade Demo und prüfe die Steuerung.',
      '+ Nachricht an LocalCode senden...':'+ Nachricht an LocalCode senden...',
      'Kein Git-Repository':'Kein Git-Repository',
      'Keine Git-Informationen verfügbar.':'Keine Git-Informationen verfügbar.',
      'Projekt auswählen':'Projekt auswählen',
      'Kein Git-Repository in diesem Projekt initialisiert.':'Kein Git-Repository in diesem Projekt initialisiert.',
      'Plan genehmigen & ausführen':'Plan genehmigen & ausführen',
      'Plan im Editor öffnen':'Plan im Editor öffnen',
      'Implementierungsplan':'Implementierungsplan',
      'Genehmigt, bitte gemäß Plan ausführen.':'Genehmigt, bitte gemäß Plan ausführen.'
    });
    Object.assign(i18n.dictionaries.en, {
      'Neues Projekt':'New project',
      'Neuer Ordner':'New folder',
      'Papierkorb':'Trash',
      'Neues Projekt anlegen':'Create new project',
      'Erstellt ein LocalCode-Projekt mit README.md, AGENTS.md und STATE.md.':'Creates a LocalCode project with README.md, AGENTS.md and STATE.md.',
      'Projekt anlegen':'Create project',
      'Projekt wurde angelegt.':'Project created.',
      'Neuen Ordner anlegen':'Create new folder',
      'Legt einen absichtlich leeren Ordner direkt unter der aktuellen Projektwurzel an.':'Creates an intentionally empty folder directly below the current project root.',
      'Ordner anlegen':'Create folder',
      'Ordner wurde leer angelegt.':'Empty folder created.',
      'Ordner umbenennen':'Rename folder',
      'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.':'Renames the actual folder on disk. LocalCode updates project and chat references.',
      'Umbenennen':'Rename',
      'Ordner wurde umbenannt.':'Folder renamed.',
      'Löschen…':'Delete…',
      'Leeren Ordner löschen?':'Delete empty folder?',
      'Der Ordner ist leer und wird nach dieser Bestätigung direkt gelöscht.':'The folder is empty and will be deleted directly after this confirmation.',
      'Leeren Ordner löschen':'Delete empty folder',
      'Leerer Ordner wurde gelöscht.':'Empty folder deleted.',
      'Projekt in den Papierkorb verschieben?':'Move project to Trash?',
      'Dieser Projektordner enthält {files} Dateien, {dirs} Unterordner und {bytes} Bytes. Er wird in den LocalCode-Papierkorb verschoben und kann wiederhergestellt werden.':'This project folder contains {files} files, {dirs} subfolders and {bytes} bytes. It will be moved to LocalCode Trash and can be restored.',
      'In Papierkorb verschieben':'Move to Trash',
      'Bestätigung stimmt nicht exakt mit dem Projektnamen überein.':'Confirmation does not exactly match the project name.',
      'Projekt wurde in den Papierkorb verschoben und kann wiederhergestellt werden.':'Project moved to Trash and can be restored.',
      'Papierkorb ist leer.':'Trash is empty.',
      'Wiederherstellen':'Restore',
      'Dauerhaft löschen…':'Permanently purge…',
      'Dauerhaftes Löschen kann nicht rückgängig gemacht werden. Gib exakt “PURGE {project}” ein.':'Permanent purge cannot be undone. Type exactly “PURGE {project}”.',
      'Projekt wurde wiederhergestellt.':'Project restored.',
      'Projekt wurde dauerhaft aus dem Papierkorb gelöscht.':'Project permanently purged from Trash.',
      'Dateien: {files} · Ordner: {dirs} · Bytes: {bytes}':'Files: {files} · Folders: {dirs} · Bytes: {bytes}',
      'Schließen':'Close',
      'Anzeigenamen bearbeiten':'Edit display name',
      'Claw Code (experimentell)':'Claw Code (experimental)',
      'Extras':'Tools',
      'Remote koppeln':'Pair Remote',
      'Scanne den QR-Code mit der Smartphone-Kamera oder der LocalCode Remote App.':'Scan the QR code with your smartphone camera or the LocalCode Remote app.',
      'Pairing-Code':'Pairing code',
      'Kopieren':'Copy',
      'Code in die Zwischenablage kopiert.':'Code copied to clipboard.',
      'Fertig':'Done',
      'Hallo, wobei kann ich Ihnen helfen?':'Hello, how can I help you today?',
      'Beschreibe eine Aufgabe oder nutze einen der Starter-Vorschläge für dein Projekt.':'Describe a task or pick one of the starter suggestions for your project.',
      'Architektur analysieren':'Analyze architecture',
      'Analysiere die Projektarchitektur und liste Kernkomponenten auf.':'Analyze the project architecture and summarize core components.',
      'Tests ausführen':'Run tests',
      'Führe alle Tests und die Codequalitätsprüfung aus.':'Run all tests and code quality checks.',
      'Git-Review':'Git review',
      'Prüfe den aktuellen Git-Status, geänderte Dateien und Diffs.':'Check current Git status, modified files and diffs.',
      'Release bauen':'Build release',
      'Prüfe die Release-Bereitschaft und erstelle das Release-Paket.':'Check release readiness and build the release package.',
      'Pacman Arcade':'Pacman Arcade',
      'Starte das Pacman Arcade Demo und prüfe die Steuerung.':'Launch the Pacman Arcade demo and test controls.',
      '+ Nachricht an LocalCode senden...':'+ Message LocalCode...',
      'Kein Git-Repository':'No Git repository',
      'Keine Git-Informationen verfügbar.':'No Git information available.',
      'Projekt auswählen':'Select project',
      'Kein Git-Repository in diesem Projekt initialisiert.':'No Git repository initialized in this project.',
      'Plan genehmigen & ausführen':'Approve & execute plan',
      'Plan im Editor öffnen':'Open plan in editor',
      'Implementierungsplan':'Implementation Plan',
      'Genehmigt, bitte gemäß Plan ausführen.':'Approved, please proceed according to the plan.'
    });
  }

  const css = `
    :root{--rightW:280px}
    .sidebar{background:#121519}
    .project-tree{flex:1 1 auto!important;min-height:0!important;overflow-y:auto!important}
    .section-title{display:flex;align-items:center;justify-content:space-between;gap:6px;padding-right:8px}
    .section-title-action{height:24px;flex:0 0 auto;border:1px solid rgba(255,255,255,0.08);border-radius:9999px;background:rgba(255,255,255,0.04);color:#c0c0c0;display:grid;place-items:center;font-size:12px;padding:0 6px;transition:all .15s}
    .section-title-action.text{padding:0 8px;font-size:10.5px;font-weight:600;white-space:nowrap}
    .section-title-action:hover{background:rgba(255,255,255,0.08);color:#fff;border-color:rgba(255,255,255,0.18)}
    .project-header-actions{display:flex;align-items:center;gap:4px;margin-left:auto}
    .project-row.active{background:rgba(255,255,255,0.09);box-shadow:inset 2px 0 0 var(--accent)}
    .context-menu{background:#1c2026;border-color:rgba(255,255,255,0.12);box-shadow:0 14px 44px rgba(0,0,0,.65);border-radius:9px}
    .context-menu-item:hover,.context-menu-item:focus,.context-menu-item.open{background:rgba(255,255,255,0.08);border-radius:5px}
    .context-menu-item.danger{color:#ff8e8e}
    .rightbar{background:#121519;border-left-color:rgba(255,255,255,0.06)}
    .right-tabs{height:42px;padding:0 8px}
    .right-tab{height:26px;padding:0 10px;border-radius:9999px;font-size:11.5px}
    .right-tab.active{background:rgba(255,255,255,0.09);color:#fff}
    .right-body{padding:10px;overflow-x:hidden}
    .output-list,.output-card,.output-card details,.output-card summary,.output-head,.output-message{min-width:0;max-width:100%}
    .output-card{padding:9px 10px;border-radius:9px;background:#16191e;border:1px solid rgba(255,255,255,0.06);overflow:hidden;font-size:11.5px}
    .output-card h4,.output-message,.output-head b{overflow-wrap:anywhere;word-break:break-word}
    .output-card pre{width:100%;max-width:100%;max-height:300px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;font-size:10.5px}
    .approval-actions{display:flex;flex-wrap:wrap;gap:6px}
    .approval-actions button{max-width:100%}
    .approval-dock{left:50%;bottom:80px;transform:translateX(-50%);width:min(640px,calc(100vw - 40px));max-height:46vh;overflow:auto;background:#1a1d22;border:1px solid rgba(255,255,255,0.14);border-radius:12px;box-shadow:0 16px 60px rgba(0,0,0,.65),0 0 0 1px rgba(47,129,247,.12);padding:12px 14px;display:block}
    .approval-dock-kicker{color:#69a8ff;font-size:10px;letter-spacing:.1em;margin-bottom:3px}
    .approval-dock-message{font-size:13px;font-weight:650;line-height:1.4}
    .approval-dock-meta{margin-top:4px;color:#8b949e;font-size:10.5px;overflow-wrap:anywhere}
    .approval-dock-preview{width:100%;max-width:100%;max-height:130px;margin-top:8px;padding:8px;background:#0c0e11;border:1px solid rgba(255,255,255,0.07);border-radius:8px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;font-size:10.5px}
    .approval-dock-actions{display:flex;align-items:center;justify-content:flex-start;gap:6px;flex-wrap:wrap;margin-top:10px}
    .approval-dock .approve,.approval-dock .reject,.approval-dock .always{height:30px;padding:0 12px;border-radius:9999px;font-size:11.5px;font-weight:650;white-space:normal;line-height:1.15}
    .approval-dock .approve{background:#ffffff;color:#111}
    .approval-dock .always{background:#285fae;border-color:#3c78ca;color:#fff}
    .approval-dock .reject{background:#30221e;border-color:#5a3328;color:#ffdede}
    body.has-approval .composer-zone{padding-bottom:14px}
    .modal-card{background:#1a1d22;border-color:rgba(255,255,255,0.12);border-radius:14px}
    .action-modal-danger{background:#b64b4b!important;border-color:#c65a5a!important}
    .project-trash-overlay{position:fixed;inset:0;z-index:650;background:rgba(0,0,0,.65);display:grid;place-items:center;padding:20px;backdrop-filter:blur(6px)}
    .project-trash-dialog{width:min(640px,calc(100vw - 32px));max-height:min(680px,calc(100vh - 40px));overflow:auto;background:#1a1d22;border:1px solid rgba(255,255,255,0.12);border-radius:14px;box-shadow:0 20px 80px rgba(0,0,0,.7);padding:16px}
    .project-trash-head{display:flex;align-items:center;gap:8px;margin-bottom:12px}.project-trash-head h3{margin:0;flex:1;font-size:15px}.project-trash-close{border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);border-radius:9999px;padding:4px 10px;color:#d0d0d0;font-size:11.5px}
    .project-trash-row{border:1px solid rgba(255,255,255,0.06);background:#121519;border-radius:9px;padding:9px 10px;margin:6px 0}.project-trash-name{font-weight:700;font-size:12.5px}.project-trash-meta{color:#8b949e;font-size:10.5px;margin:3px 0 8px;overflow-wrap:anywhere}.project-trash-actions{display:flex;gap:6px;flex-wrap:wrap}.project-trash-actions button{border:1px solid rgba(255,255,255,0.08);background:rgba(255,255,255,0.04);border-radius:9999px;padding:4px 10px;font-size:11px}.project-trash-actions .project-trash-purge{margin-left:auto;background:#30221e;border-color:#5a3328;color:#ffdede}
    #newChatBtn{margin:4px 8px 8px;background:rgba(255,255,255,0.05);border:1px solid rgba(255,255,255,0.08);border-radius:9999px;height:32px;padding:0 12px;font-weight:600;font-size:12px;color:#fff;display:flex;align-items:center;gap:8px;box-shadow:0 2px 6px rgba(0,0,0,0.2);transition:all .15s ease}
    #newChatBtn:hover{background:rgba(255,255,255,0.09);border-color:rgba(255,255,255,0.18);transform:translateY(-1px);box-shadow:0 4px 10px rgba(0,0,0,0.3)}
    #newChatBtn .icon{width:14px;height:14px;color:#d0d7de}
    #newChatBtn .hint{font-size:9.5px;color:#7d8590;background:rgba(255,255,255,0.06);padding:1px 5px;border-radius:3px}
    .empty-state{height:100%;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:16px}
    .hero-container{max-width:640px;display:flex;flex-direction:column;align-items:center}
    .hero-title{font-size:22px;font-weight:600;letter-spacing:-.015em;color:#f0f3f6;margin:0 0 6px}
    .hero-subtitle{font-size:12.5px;color:#8b949e;line-height:1.5;margin:0 0 18px;max-width:520px}
    .hero-chips{display:flex;flex-wrap:wrap;justify-content:center;gap:8px}
    .hero-chip{background:rgba(255,255,255,0.04);border:1px solid rgba(255,255,255,.08);border-radius:9999px;padding:6px 14px;font-size:12px;font-weight:500;color:#c9d1d9;cursor:pointer;transition:all .15s ease;box-shadow:0 2px 6px rgba(0,0,0,.15)}
    .hero-chip:hover{background:rgba(255,255,255,0.09);border-color:rgba(255,255,255,.18);color:#fff;transform:translateY(-1px);box-shadow:0 4px 12px rgba(0,0,0,.3)}
    .composer{background:#171a1f!important;border:1px solid rgba(255,255,255,.09)!important;border-radius:16px!important;box-shadow:0 8px 30px rgba(0,0,0,.45)!important}
    .composer:focus-within{border-color:rgba(255,255,255,.2)!important;box-shadow:0 10px 36px rgba(0,0,0,.55),0 0 0 1px rgba(255,255,255,.08)!important}
    @media(max-width:1100px){:root{--rightW:250px}.approval-dock{width:min(600px,calc(100vw - 32px));bottom:72px}.section-title-action.text{font-size:9.5px;padding:0 4px}}
    @media(max-width:760px){.approval-dock{left:10px;right:10px;bottom:70px;transform:none;width:auto;max-height:48vh}.approval-dock-actions{justify-content:stretch}.approval-dock-actions button{flex:1 1 140px}}
  `;

  const style = document.createElement('style');
  style.id = 'localcode-ui-polish';
  style.textContent = css;
  document.head.appendChild(style);

  function tr(text) {
    return window.LocalCodeI18n?.t ? window.LocalCodeI18n.t(text) : text;
  }

  function trf(text, values = {}) {
    let result = tr(text);
    for (const [key, value] of Object.entries(values)) result = result.replaceAll(`{${key}}`, String(value));
    return result;
  }

  function folderBase(path) {
    const parts = String(path || '').split(/[\\/]/).filter(Boolean);
    return parts[parts.length - 1] || '';
  }

  function installClawEngineUI() {
    const select = document.querySelector('#setEditingEngine');
    if (select && !select.querySelector('option[value="claw"]')) {
      const option = document.createElement('option');
      option.value = 'claw';
      option.textContent = tr('Claw Code (experimentell)');
      const native = select.querySelector('option[value="native"]');
      select.insertBefore(option, native || null);
    }
    if (window.__localCodeClawEngineUIInstalled) return;
    window.__localCodeClawEngineUIInstalled = true;
    if (typeof window.selectedEngineName === 'function') {
      const originalSelectedEngineName = window.selectedEngineName;
      window.selectedEngineName = function() {
        if (document.querySelector('#setEditingEngine')?.value === 'claw') return 'Claw Code';
        return originalSelectedEngineName();
      };
    }
    if (typeof window.updateEngineSettingsVisibility === 'function') {
      const originalUpdateEngineSettingsVisibility = window.updateEngineSettingsVisibility;
      window.updateEngineSettingsVisibility = function() {
        originalUpdateEngineSettingsVisibility();
        const engine = document.querySelector('#setEditingEngine')?.value || 'aider';
        if (engine !== 'claw') return;
        document.querySelector('#aiderEngineSettings')?.classList.add('hidden');
        document.querySelector('#claudeEngineSettings')?.classList.add('hidden');
        document.querySelector('#openCodeEngineSettings')?.classList.add('hidden');
        document.querySelector('#engineLoginBtn')?.classList.add('hidden');
      };
    }
  }

  function installProjectHeaderAction() {
    const section = document.querySelector('#leftPane .section-title');
    if (!section || section.querySelector('#newProjectBtn')) return;
    const label = document.createElement('span');
    label.textContent = tr('Projekte');
    section.textContent = '';
    section.appendChild(label);
    const actions = document.createElement('span');
    actions.className = 'project-header-actions';
    const addButton = (id, labelKey, handler, compact = false) => {
      const button = document.createElement('button');
      button.id = id;
      button.className = 'section-title-action' + (compact ? '' : ' text');
      button.type = 'button';
      button.dataset.projectHeaderLabel = labelKey;
      button.textContent = compact ? '♲' : `+ ${tr(labelKey)}`;
      button.title = tr(labelKey);
      button.setAttribute('aria-label', tr(labelKey));
      button.addEventListener('click', event => {
        event.stopPropagation();
        handler();
      });
      actions.appendChild(button);
    };
    addButton('newProjectBtn', 'Neues Projekt', createLocalCodeProject);
    addButton('newFolderBtn', 'Neuer Ordner', createProjectFolder);
    addButton('projectTrashBtn', 'Papierkorb', openProjectTrash, true);
    section.appendChild(actions);
  }

  async function createLocalCodeProject() {
    const root = state?.status?.root_dir || document.querySelector('#rootPath')?.textContent || '';
    if (!root) {
      toast(tr('Keine Projektordner gefunden.'), true);
      return;
    }
    const name = await openActionModal({
      title: 'Neues Projekt anlegen',
      help: 'Erstellt ein LocalCode-Projekt mit README.md, AGENTS.md und STATE.md.',
      value: '',
      confirm: 'Projekt anlegen'
    });
    if (name === null || !name.trim()) return;
    try {
      const result = await projectAction(root, 'create_project', name.trim());
      toast(tr('Projekt wurde angelegt.'));
      if (result?.project?.path) await selectProject(result.project.path);
    } catch (error) {
      toast(error.message, true);
    }
  }

  async function createProjectFolder() {
    const root = state?.status?.root_dir || document.querySelector('#rootPath')?.textContent || '';
    if (!root) {
      toast(tr('Keine Projektordner gefunden.'), true);
      return;
    }
    const name = await openActionModal({
      title: 'Neuen Ordner anlegen',
      help: 'Legt einen absichtlich leeren Ordner direkt unter der aktuellen Projektwurzel an.',
      value: '',
      confirm: 'Ordner anlegen'
    });
    if (name === null || !name.trim()) return;
    try {
      await projectAction(root, 'create_folder', name.trim());
      toast(tr('Ordner wurde leer angelegt.'));
    } catch (error) {
      toast(error.message, true);
    }
  }

  async function renameProjectFolder(path) {
    const current = folderBase(path);
    const name = await openActionModal({
      title: 'Ordner umbenennen',
      help: 'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.',
      value: current,
      confirm: 'Umbenennen'
    });
    if (name === null || name === current) return;
    try {
      await projectAction(path, 'rename_folder', name);
      toast(tr('Ordner wurde umbenannt.'));
    } catch (error) {
      toast(error.message, true);
    }
  }

  async function deleteProjectFolder(path) {
    try {
      const preview = await api(`/api/project-delete-preview?path=${encodeURIComponent(path)}`);
      if (preview.empty) {
        const ok = await openActionModal({
          title: 'Leeren Ordner löschen?',
          help: 'Der Ordner ist leer und wird nach dieser Bestätigung direkt gelöscht.',
          confirm: 'Leeren Ordner löschen',
          danger: true,
          input: false
        });
        if (!ok) return;
        await projectAction(path, 'delete_empty');
        toast(tr('Leerer Ordner wurde gelöscht.'));
        return;
      }
      const help = `${trf('Dieser Projektordner enthält {files} Dateien, {dirs} Unterordner und {bytes} Bytes. Er wird in den LocalCode-Papierkorb verschoben und kann wiederhergestellt werden.', {files: preview.files, dirs: preview.directories, bytes: preview.bytes})} ${tr('Bestätigen')}: “${preview.confirmation}”`;
      const typed = await openActionModal({
        title: 'Projekt in den Papierkorb verschieben?',
        help,
        value: '',
        confirm: 'In Papierkorb verschieben',
        danger: true,
        input: true
      });
      if (typed === null) return;
      if (typed.trim() !== String(preview.confirmation)) {
        toast(tr('Bestätigung stimmt nicht exakt mit dem Projektnamen überein.'), true);
        return;
      }
      await projectAction(path, 'delete_recursive', typed.trim());
      toast(tr('Projekt wurde in den Papierkorb verschoben und kann wiederhergestellt werden.'));
    } catch (error) {
      toast(error.message, true);
    }
  }

  function ensureProjectTrashOverlay() {
    let overlay = document.querySelector('#projectTrashOverlay');
    if (overlay) return overlay;
    overlay = document.createElement('div');
    overlay.id = 'projectTrashOverlay';
    overlay.className = 'project-trash-overlay hidden';
    const dialog = document.createElement('div');
    dialog.className = 'project-trash-dialog';
    const head = document.createElement('div');
    head.className = 'project-trash-head';
    const title = document.createElement('h3');
    title.dataset.trashTitle = '1';
    title.textContent = tr('Papierkorb');
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'project-trash-close';
    close.textContent = tr('Schließen');
    close.onclick = () => overlay.classList.add('hidden');
    head.append(title, close);
    const list = document.createElement('div');
    list.dataset.trashList = '1';
    dialog.append(head, list);
    overlay.appendChild(dialog);
    overlay.addEventListener('click', event => {
      if (event.target === overlay) overlay.classList.add('hidden');
    });
    document.body.appendChild(overlay);
    return overlay;
  }

  async function openProjectTrash() {
    const overlay = ensureProjectTrashOverlay();
    const list = overlay.querySelector('[data-trash-list]');
    overlay.querySelector('[data-trash-title]').textContent = tr('Papierkorb');
    overlay.querySelector('.project-trash-close').textContent = tr('Schließen');
    list.textContent = '';
    overlay.classList.remove('hidden');
    try {
      const result = await api('/api/project-quarantine');
      const entries = result?.quarantine || [];
      if (!entries.length) {
        const empty = document.createElement('div');
        empty.className = 'muted';
        empty.textContent = tr('Papierkorb ist leer.');
        list.appendChild(empty);
        return;
      }
      for (const entry of entries) {
        const row = document.createElement('div');
        row.className = 'project-trash-row';
        const name = document.createElement('div');
        name.className = 'project-trash-name';
        name.textContent = entry.name;
        const meta = document.createElement('div');
        meta.className = 'project-trash-meta';
        meta.textContent = trf('Dateien: {files} · Ordner: {dirs} · Bytes: {bytes}', {files: entry.files, dirs: entry.directories, bytes: entry.bytes});
        const actions = document.createElement('div');
        actions.className = 'project-trash-actions';
        const restore = document.createElement('button');
        restore.type = 'button';
        restore.textContent = tr('Wiederherstellen');
        restore.onclick = async () => {
          try {
            await api('/api/project-quarantine-action', {method: 'POST', body: JSON.stringify({action: 'restore', id: entry.id})});
            await loadProjects();
            await loadThreads();
            await loadSnapshot();
            renderAll();
            toast(tr('Projekt wurde wiederhergestellt.'));
            await openProjectTrash();
          } catch (error) {
            toast(error.message, true);
          }
        };
        const purge = document.createElement('button');
        purge.type = 'button';
        purge.className = 'project-trash-purge';
        purge.textContent = tr('Dauerhaft löschen…');
        purge.onclick = async () => {
          const expected = `PURGE ${entry.name}`;
          const typed = window.prompt(trf('Dauerhaftes Löschen kann nicht rückgängig gemacht werden. Gib exakt “PURGE {project}” ein.', {project: entry.name}), '');
          if (typed === null) return;
          if (typed !== expected) {
            toast(tr('Bestätigung stimmt nicht exakt mit dem Projektnamen überein.'), true);
            return;
          }
          try {
            await api('/api/project-quarantine-action', {method: 'POST', body: JSON.stringify({action: 'purge', id: entry.id, confirmation: typed})});
            toast(tr('Projekt wurde dauerhaft aus dem Papierkorb gelöscht.'));
            await openProjectTrash();
          } catch (error) {
            toast(error.message, true);
          }
        };
        actions.append(restore, purge);
        row.append(name, meta, actions);
        list.appendChild(row);
      }
    } catch (error) {
      list.textContent = error.message;
    }
  }

  function polishedProjectMenu(path, x, y) {
    const p = state.projectMeta[path] || {path, name: projectDisplay(path), pinned: false};
    showContextMenu([
      {label:'Neue Aufgabe', action:'project-new-task', icon:'＋'},
      {label:'Neues Projekt', action:'project-create-project', icon:'P+'},
      {label:'Neuer Ordner', action:'project-create-folder', icon:'▣+'},
      {label:'Ordner umbenennen', action:'project-rename-folder', icon:'✎'},
      {label:'Anzeigenamen bearbeiten', action:'project-rename', icon:'Aa'},
      {separator:true},
      {label:'Öffnen in', action:'noop', icon:'↗', submenu:[
        {label:'Standardeditor', action:'project-open-default'},
        {label:'Visual Studio', action:'project-open-visualstudio'},
        {label:'Visual Studio Code', action:'project-open-vscode'}
      ]},
      {label:'Im integrierten Terminal öffnen', action:'project-open-integrated-terminal', icon:'›_'},
      {label:'Im Datei-Explorer öffnen', action:'project-open-explorer', icon:'▣'},
      {separator:true},
      {label:p.pinned?'Projekt lösen':'Projekt anheften', action:p.pinned?'project-unpin':'project-pin', icon:'⌖'},
      {label:'Projekt entfernen', action:'project-remove', icon:'−'},
      {separator:true},
      {label:'Papierkorb', action:'project-trash', icon:'♲'},
      {label:'Löschen…', action:'project-delete', icon:'⌫', danger:true}
    ], x, y, {kind:'project', path});
  }

  function installFunctionOverrides() {
    const originalContextAction = handleContextAction;
    showProjectMenu = polishedProjectMenu;
    handleContextAction = async function(action, target) {
      switch (action) {
        case 'project-create-project': return createLocalCodeProject();
        case 'project-create-folder': return createProjectFolder();
        case 'project-rename-folder': return renameProjectFolder(target.path);
        case 'project-trash': return openProjectTrash();
        case 'project-delete': return deleteProjectFolder(target.path);
        default: return originalContextAction(action, target);
      }
    };

    const originalMenuAction = handleMenuAction;
    handleMenuAction = function(action) {
      if (action === 'reset-layout') {
        closeMenus();
        state.settings.ui_left_width = 296;
        state.settings.ui_right_width = 280;
        state.settings.ui_terminal_height = 260;
        state.leftHidden = state.rightHidden = false;
        applyAppearance();
        setMainGrid();
        saveSettings(true);
        return;
      }
      return originalMenuAction(action);
    };
  }

  function migrateRightPanelWidth() {
    let attempts = 0;
    const timer = setInterval(() => {
      attempts++;
      if (typeof state !== 'undefined' && state.settings) {
        const current = Number(state.settings.ui_right_width || 340);
        let next = current;
        if (!current || current === 340) next = 280;
        else if (current > 420) next = 420;
        if (next !== current) {
          state.settings.ui_right_width = next;
          document.documentElement.style.setProperty('--rightW', `${next}px`);
          saveSettings(true);
        } else {
          document.documentElement.style.setProperty('--rightW', `${current}px`);
        }
        clearInterval(timer);
      } else if (attempts > 80) {
        clearInterval(timer);
      }
    }, 100);

    const splitter = document.querySelector('#rightSplitter');
    splitter?.addEventListener('pointerup', () => {
      requestAnimationFrame(() => {
        if (!state?.settings) return;
        const current = Number(state.settings.ui_right_width || 280);
        const clamped = Math.max(240, Math.min(420, current));
        if (clamped !== current) {
          state.settings.ui_right_width = clamped;
          document.documentElement.style.setProperty('--rightW', `${clamped}px`);
          saveSettings(true);
        }
      });
    });
  }

  function refreshLocalizedControls() {
    const section = document.querySelector('#leftPane .section-title > span');
    if (section) section.textContent = tr('Projekte');
    document.querySelectorAll('[data-project-header-label]').forEach(button => {
      const key = button.dataset.projectHeaderLabel;
      const compact = button.id === 'projectTrashBtn';
      button.textContent = compact ? '♲' : `+ ${tr(key)}`;
      button.title = tr(key);
      button.setAttribute('aria-label', tr(key));
    });
    const trash = document.querySelector('#projectTrashOverlay');
    if (trash && !trash.classList.contains('hidden')) openProjectTrash();
    const claw = document.querySelector('#setEditingEngine option[value="claw"]');
    if (claw) claw.textContent = tr('Claw Code (experimentell)');
  }

  installClawEngineUI();

  window.addEventListener('load', () => {
    installClawEngineUI();
    installProjectHeaderAction();
    installFunctionOverrides();
    migrateRightPanelWidth();
    refreshLocalizedControls();
    document.addEventListener('localcode:language', () => setTimeout(refreshLocalizedControls, 0));
  });
})();