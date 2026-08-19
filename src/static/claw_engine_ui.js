// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  function ensureClawOption() {
    const select = document.querySelector('#setEditingEngine');
    if (!select || select.querySelector('option[value="claw"]')) return;
    const option = document.createElement('option');
    option.value = 'claw';
    option.textContent = window.LocalCodeI18n?.t?.('Claw Code (experimentell)') || 'Claw Code';
    select.insertBefore(option, select.querySelector('option[value="native"]') || null);
  }

  function patchEngineFunctions() {
    ensureClawOption();

    if (typeof window.selectedEngineName === 'function' && !window.selectedEngineName.__localCodeClawWrapped) {
      const original = window.selectedEngineName;
      const wrapped = function() {
        if (document.querySelector('#setEditingEngine')?.value === 'claw') return 'Claw Code';
        return original();
      };
      wrapped.__localCodeClawWrapped = true;
      window.selectedEngineName = wrapped;
    }

    if (typeof window.updateEngineSettingsVisibility === 'function' && !window.updateEngineSettingsVisibility.__localCodeClawWrapped) {
      const original = window.updateEngineSettingsVisibility;
      const wrapped = function() {
        original();
        const engine = document.querySelector('#setEditingEngine')?.value || 'aider';
        if (engine !== 'claw') return;
        document.querySelector('#aiderEngineSettings')?.classList.add('hidden');
        document.querySelector('#claudeEngineSettings')?.classList.add('hidden');
        document.querySelector('#openCodeEngineSettings')?.classList.add('hidden');
        document.querySelector('#engineLoginBtn')?.classList.add('hidden');
      };
      wrapped.__localCodeClawWrapped = true;
      window.updateEngineSettingsVisibility = wrapped;
    }

    window.updateEngineSettingsVisibility?.();
  }

  window.addEventListener('load', patchEngineFunctions, {once: true});
})();
