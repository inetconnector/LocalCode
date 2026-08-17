// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  const i18n = window.LocalCodeI18n;
  if (i18n?.dictionaries) {
    Object.assign(i18n.dictionaries.de, {
      'Neuer Ordner':'Neuer Ordner',
      'Ordner umbenennen':'Ordner umbenennen',
      'Leeren Ordner löschen':'Leeren Ordner löschen',
      'Ordner rekursiv löschen…':'Ordner rekursiv löschen…',
      'Neuen Ordner anlegen':'Neuen Ordner anlegen',
      'Legt einen neuen Projektordner direkt unter der aktuellen Projektwurzel an.':'Legt einen neuen Projektordner direkt unter der aktuellen Projektwurzel an.',
      'Ordner anlegen':'Ordner anlegen',
      'Ordner wurde angelegt.':'Ordner wurde angelegt.',
      'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.':'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.',
      'Umbenennen':'Umbenennen',
      'Ordner wurde umbenannt.':'Ordner wurde umbenannt.',
      'Leeren Ordner löschen?':'Leeren Ordner löschen?',
      'Der Ordner wird nur gelöscht, wenn er wirklich leer ist.':'Der Ordner wird nur gelöscht, wenn er wirklich leer ist.',
      'Leerer Ordner wurde gelöscht.':'Leerer Ordner wurde gelöscht.',
      'Ordner rekursiv löschen?':'Ordner rekursiv löschen?',
      'Dauerhaft löschen':'Dauerhaft löschen',
      'Bestätigung stimmt nicht mit dem Ordnernamen überein.':'Bestätigung stimmt nicht mit dem Ordnernamen überein.',
      'Ordner wurde rekursiv gelöscht.':'Ordner wurde rekursiv gelöscht.',
      'Anzeigenamen bearbeiten':'Anzeigenamen bearbeiten',
      'Dateien und Unterordner werden dauerhaft gelöscht. Der lokale Chatverlauf bleibt archiviert erhalten.':'Dateien und Unterordner werden dauerhaft gelöscht. Der lokale Chatverlauf bleibt archiviert erhalten.'
    });
    Object.assign(i18n.dictionaries.en, {
      'Neuer Ordner':'New folder',
      'Ordner umbenennen':'Rename folder',
      'Leeren Ordner löschen':'Delete empty folder',
      'Ordner rekursiv löschen…':'Delete folder recursively…',
      'Neuen Ordner anlegen':'Create new folder',
      'Legt einen neuen Projektordner direkt unter der aktuellen Projektwurzel an.':'Creates a new project folder directly below the current project root.',
      'Ordner anlegen':'Create folder',
      'Ordner wurde angelegt.':'Folder created.',
      'Ändert den tatsächlichen Ordnernamen auf der Festplatte. LocalCode aktualisiert Projekt- und Chatverweise.':'Renames the actual folder on disk. LocalCode updates project and chat references.',
      'Umbenennen':'Rename',
      'Ordner wurde umbenannt.':'Folder renamed.',
      'Leeren Ordner löschen?':'Delete empty folder?',
      'Der Ordner wird nur gelöscht, wenn er wirklich leer ist.':'The folder is deleted only if it is actually empty.',
      'Leerer Ordner wurde gelöscht.':'Empty folder deleted.',
      'Ordner rekursiv löschen?':'Delete folder recursively?',
      'Dauerhaft löschen':'Delete permanently',
      'Bestätigung stimmt nicht mit dem Ordnernamen überein.':'Confirmation does not match the folder name.',
      'Ordner wurde rekursiv gelöscht.':'Folder deleted recursively.',
      'Anzeigenamen bearbeiten':'Edit display name',
      'Dateien und Unterordner werden dauerhaft gelöscht. Der lokale Chatverlauf bleibt archiviert erhalten.':'Files and subfolders are permanently deleted. Local chat history is preserved in the archive.'
    });
  }

  const css = `
    :root{--rightW:280px}
    .sidebar{background:#182126}
    .section-title{display:flex;align-items:center;justify-content:space-between;gap:8px;padding-right:9px}
    .section-title-action{width:27px;height:27px;flex:0 0 27px;border:1px solid #36434a;border-radius:7px;background:#202b30;color:#b8c3c9;display:grid;place-items:center;font-size:18px;line-height:1;padding:0}
    .section-title-action:hover{background:#2b3940;color:#fff;border-color:#4a5a62}
    .project-row.active{background:#263b4d;box-shadow:inset 2px 0 0 var(--accent)}
    .context-menu{background:#1e2529;border-color:#3c494f;box-shadow:0 18px 60px rgba(0,0,0,.6)}
    .context-menu-item:hover,.context-menu-item:focus,.context-menu-item.open{background:#2c383e}
    .context-menu-item.danger{color:#ff8e8e}
    .rightbar{background:#181c1f;border-left-color:#2c3438}
    .right-tabs{height:50px;padding:0 8px}
    .right-tab{height:32px;padding:0 9px}
    .right-body{padding:10px;overflow-x:hidden}
    .output-list,.output-card,.output-card details,.output-card summary,.output-head,.output-message{min-width:0;max-width:100%}
    .output-card{padding:10px;border-radius:9px;overflow:hidden}
    .output-card h4,.output-message,.output-head b{overflow-wrap:anywhere;word-break:break-word}
    .output-card pre{width:100%;max-width:100%;max-height:330px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word;font-size:10.5px}
    .approval-actions{display:flex;flex-wrap:wrap;gap:6px}
    .approval-actions button{max-width:100%}
    .approval-dock{left:50%;bottom:96px;transform:translateX(-50%);width:min(690px,calc(100vw - 56px));max-height:46vh;overflow:auto;background:#1e252a;border:1px solid #3d4d58;border-radius:14px;box-shadow:0 20px 70px rgba(0,0,0,.62),0 0 0 1px rgba(47,129,247,.08);padding:14px 15px;display:block}
    .approval-dock-kicker{color:#69a8ff;font-size:10.5px;letter-spacing:.11em;margin-bottom:4px}
    .approval-dock-message{font-size:14px;font-weight:650;line-height:1.4}
    .approval-dock-meta{margin-top:6px;color:#9aa7ae;font-size:10.5px;overflow-wrap:anywhere}
    .approval-dock-preview{width:100%;max-width:100%;max-height:150px;margin-top:10px;padding:9px 10px;background:#131719;border:1px solid #344149;border-radius:8px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;word-break:break-word}
    .approval-dock-actions{display:flex;align-items:center;justify-content:flex-start;gap:7px;flex-wrap:wrap;margin-top:11px}
    .approval-dock .approve,.approval-dock .reject,.approval-dock .always{height:34px;padding:0 11px;border-radius:7px;font-size:12px;font-weight:650;white-space:normal;line-height:1.15}
    .approval-dock .approve{background:#eef0f1;color:#111}
    .approval-dock .always{background:#285fae;border-color:#3c78ca}
    .approval-dock .reject{background:#382426;border-color:#6d3d41;color:#ffdede}
    body.has-approval .composer-zone{padding-bottom:18px}
    .modal-card{background:#1e2428;border-color:#3b464c}
    .action-modal-danger{background:#b64b4b!important;border-color:#c65a5a!important}
    @media(max-width:1100px){:root{--rightW:250px}.approval-dock{width:min(630px,calc(100vw - 40px));bottom:88px}}
    @media(max-width:760px){.approval-dock{left:12px;right:12px;bottom:82px;transform:none;width:auto;max-height:48vh}.approval-dock-actions{justify-content:stretch}.approval-dock-actions button{flex:1 1 150px}}
  `;

  const style = document.createElement('style');
  style.id = 'localcode-ui-polish';
  style.textContent = css;
  document.head.appendChild(style);

  function tr(text) {
    return window.LocalCodeI18n?.t ? window.LocalCodeI18n.t(text) : text;
  }

  function folderBase(path) {
    const parts = String(path || '').split(/[\\/]/).filter(Boolean);
    return parts[parts.length - 1] || '';
  }

  function installProjectHeaderAction() {
    const section = document.querySelector('#leftPane .section-title');
    if (!section || section.querySelector('#newFolderBtn')) return;
    const label = document.createElement('span');
    label.textContent = tr('Projekte');
    section.textContent = '';
    section.appendChild(label);
    const button = document.createElement('button');
    button.id = 'newFolderBtn';
    button.className = 'section-title-action';
    button.type = 'button';
    button.textContent = '+';
    button.title = tr('Neuer Ordner');
    button.setAttribute('aria-label', tr('Neuer Ordner'));
    button.addEventListener('click', event => {
      event.stopPropagation();
      createProjectFolder();
    });
    section.appendChild(button);
  }

  async function createProjectFolder() {
    const root = state?.status?.root_dir || document.querySelector('#rootPath')?.textContent || '';
    if (!root) {
      toast(tr('Keine Projektordner gefunden.'), true);
      return;
    }
    const name = await openActionModal({
      title: 'Neuen Ordner anlegen',
      help: 'Legt einen neuen Projektordner direkt unter der aktuellen Projektwurzel an.',
      value: '',
      confirm: 'Ordner anlegen'
    });
    if (name === null) return;
    try {
      const result = await projectAction(root, 'create_folder', name);
      toast(tr('Ordner wurde angelegt.'));
      if (result?.project?.path) await selectProject(result.project.path);
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

  async function deleteEmptyProjectFolder(path) {
    const ok = await openActionModal({
      title: 'Leeren Ordner löschen?',
      help: 'Der Ordner wird nur gelöscht, wenn er wirklich leer ist.',
      confirm: 'Leeren Ordner löschen',
      danger: true,
      input: false
    });
    if (!ok) return;
    try {
      await projectAction(path, 'delete_empty');
      toast(tr('Leerer Ordner wurde gelöscht.'));
    } catch (error) {
      toast(error.message, true);
    }
  }

  async function deleteProjectFolderRecursive(path) {
    const name = folderBase(path);
    const typed = await openActionModal({
      title: 'Ordner rekursiv löschen?',
      help: `${tr('Dateien und Unterordner werden dauerhaft gelöscht. Der lokale Chatverlauf bleibt archiviert erhalten.')} ${tr('Bestätigen')}: “${name}”`,
      value: '',
      confirm: 'Dauerhaft löschen',
      danger: true,
      input: true
    });
    if (typed === null) return;
    if (typed.toLocaleLowerCase() !== name.toLocaleLowerCase()) {
      toast(tr('Bestätigung stimmt nicht mit dem Ordnernamen überein.'), true);
      return;
    }
    try {
      await projectAction(path, 'delete_recursive', typed);
      toast(tr('Ordner wurde rekursiv gelöscht.'));
    } catch (error) {
      toast(error.message, true);
    }
  }

  function polishedProjectMenu(path, x, y) {
    const p = state.projectMeta[path] || {path, name: projectDisplay(path), pinned: false};
    showContextMenu([
      {label:'Neue Aufgabe', action:'project-new-task', icon:'＋'},
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
      {label:'Leeren Ordner löschen', action:'project-delete-empty', icon:'⌫', danger:true},
      {label:'Ordner rekursiv löschen…', action:'project-delete-recursive', icon:'⚠', danger:true}
    ], x, y, {kind:'project', path});
  }

  function installFunctionOverrides() {
    const originalContextAction = handleContextAction;
    showProjectMenu = polishedProjectMenu;
    handleContextAction = async function(action, target) {
      switch (action) {
        case 'project-create-folder': return createProjectFolder();
        case 'project-rename-folder': return renameProjectFolder(target.path);
        case 'project-delete-empty': return deleteEmptyProjectFolder(target.path);
        case 'project-delete-recursive': return deleteProjectFolderRecursive(target.path);
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
    const button = document.querySelector('#newFolderBtn');
    if (button) {
      button.title = tr('Neuer Ordner');
      button.setAttribute('aria-label', tr('Neuer Ordner'));
    }
  }

  window.addEventListener('load', () => {
    installProjectHeaderAction();
    installFunctionOverrides();
    migrateRightPanelWidth();
    refreshLocalizedControls();
    document.addEventListener('localcode:language', () => setTimeout(refreshLocalizedControls, 0));
  });
})();
