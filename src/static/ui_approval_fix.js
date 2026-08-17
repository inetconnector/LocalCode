// SPDX-License-Identifier: Apache-2.0
(() => {
  'use strict';

  const css = `
    :root{--rightW:260px}
    html,body,#app,.main-screen{max-width:100vw;overflow-x:hidden}
    .rightbar{min-width:0;max-width:300px;overflow:hidden}
    .right-body{min-width:0;max-width:100%;overflow-x:hidden!important}
    .output-list,.output-card,.output-card details,.output-card summary,.output-head,.output-message{min-width:0;max-width:100%}
    .output-card{overflow:hidden}
    .output-card h4,.output-message,.output-head b,.output-card pre{overflow-wrap:anywhere!important;word-break:break-word!important}
    .output-card pre{width:100%!important;max-width:100%!important;overflow-x:hidden!important;overflow-y:auto!important;white-space:pre-wrap!important}
    #rightBody .output-card.approval{display:none!important}

    .approval-dock{
      position:fixed!important;
      z-index:2200!important;
      left:50%!important;
      top:50%!important;
      right:auto!important;
      bottom:auto!important;
      transform:translate(-50%,-50%)!important;
      width:min(760px,calc(100vw - 48px))!important;
      max-width:760px!important;
      min-width:0!important;
      max-height:min(72vh,640px)!important;
      padding:0!important;
      margin:0!important;
      display:flex!important;
      flex-direction:column!important;
      align-items:stretch!important;
      gap:0!important;
      overflow:hidden!important;
      background:#1c2226!important;
      border:1px solid #46545d!important;
      border-radius:16px!important;
      box-shadow:0 0 0 100vmax rgba(0,0,0,.48),0 24px 90px rgba(0,0,0,.72)!important;
    }
    .approval-dock.hidden{display:none!important}
    .approval-dock-copy{
      display:block!important;
      width:100%!important;
      min-width:0!important;
      max-width:100%!important;
      padding:18px 20px 14px!important;
      overflow:auto!important;
    }
    .approval-dock-kicker{
      margin:0 0 6px!important;
      color:#74adff!important;
      font-size:11px!important;
      font-weight:750!important;
      line-height:1.2!important;
      letter-spacing:.09em!important;
      white-space:normal!important;
      overflow-wrap:normal!important;
      word-break:normal!important;
    }
    .approval-dock-message{
      width:100%!important;
      min-width:0!important;
      margin:0!important;
      font-size:16px!important;
      font-weight:680!important;
      line-height:1.38!important;
      white-space:normal!important;
      overflow-wrap:anywhere!important;
      word-break:normal!important;
    }
    .approval-dock-meta{
      display:flex!important;
      width:100%!important;
      min-width:0!important;
      gap:7px 12px!important;
      flex-wrap:wrap!important;
      margin-top:8px!important;
      color:#9eabb2!important;
      font-size:11px!important;
      line-height:1.4!important;
      white-space:normal!important;
      overflow-wrap:anywhere!important;
      word-break:normal!important;
    }
    .approval-dock-preview{
      display:block;
      width:100%!important;
      min-width:0!important;
      max-width:100%!important;
      max-height:280px!important;
      margin:12px 0 0!important;
      padding:11px 12px!important;
      overflow-x:hidden!important;
      overflow-y:auto!important;
      background:#111618!important;
      border:1px solid #35434b!important;
      border-radius:10px!important;
      white-space:pre-wrap!important;
      overflow-wrap:anywhere!important;
      word-break:break-word!important;
      font-size:11px!important;
      line-height:1.48!important;
    }
    .approval-dock-preview.hidden{display:none!important}
    .approval-dock-actions{
      display:flex!important;
      width:100%!important;
      min-width:0!important;
      align-items:center!important;
      justify-content:flex-end!important;
      gap:8px!important;
      flex-wrap:wrap!important;
      margin:0!important;
      padding:14px 20px 18px!important;
      border-top:1px solid #303b41!important;
      background:#192024!important;
    }
    .approval-dock .approve,.approval-dock .reject,.approval-dock .always{
      flex:0 1 auto!important;
      min-width:0!important;
      max-width:100%!important;
      min-height:36px!important;
      height:auto!important;
      padding:8px 12px!important;
      border-radius:8px!important;
      font-size:12px!important;
      font-weight:650!important;
      line-height:1.25!important;
      white-space:normal!important;
      overflow-wrap:anywhere!important;
      word-break:normal!important;
    }
    body.has-approval .composer-zone{padding-bottom:18px!important}

    @media(max-width:980px){
      :root{--rightW:240px}
      .rightbar{max-width:280px}
      .approval-dock{width:min(700px,calc(100vw - 32px))!important;max-height:76vh!important}
    }
    @media(max-width:720px){
      .approval-dock{width:calc(100vw - 20px)!important;max-height:82vh!important}
      .approval-dock-copy{padding:15px 15px 12px!important}
      .approval-dock-actions{padding:12px 15px 15px!important;justify-content:stretch!important}
      .approval-dock-actions button{flex:1 1 210px!important}
    }
  `;

  const style = document.createElement('style');
  style.id = 'localcode-approval-layout-fix';
  style.textContent = css;
  document.head.appendChild(style);

  function installSingleApprovalSurface() {
    if (window.__localCodeSingleApprovalSurfaceInstalled) return true;
    if (typeof renderInspector !== 'function' || typeof state === 'undefined') return false;

    const originalRenderInspector = renderInspector;
    renderInspector = function() {
      const pending = state.pending;
      if (!pending) return originalRenderInspector();
      state.pending = null;
      try {
        return originalRenderInspector();
      } finally {
        state.pending = pending;
      }
    };
    window.__localCodeSingleApprovalSurfaceInstalled = true;
    return true;
  }

  function clampRightPanelWidth() {
    if (typeof state === 'undefined' || !state.settings) return false;
    const current = Number(state.settings.ui_right_width || 260);
    const next = Math.max(240, Math.min(300, current === 340 ? 260 : current));
    state.settings.ui_right_width = next;
    document.documentElement.style.setProperty('--rightW', `${next}px`);
    return true;
  }

  function installWhenReady(attempt = 0) {
    const installed = installSingleApprovalSurface();
    const clamped = clampRightPanelWidth();
    if ((!installed || !clamped) && attempt < 120) {
      setTimeout(() => installWhenReady(attempt + 1), 25);
    }
  }

  setTimeout(() => installWhenReady(), 0);
  window.addEventListener('load', () => {
    installWhenReady();
    const splitter = document.querySelector('#rightSplitter');
    splitter?.addEventListener('pointerup', () => {
      requestAnimationFrame(() => {
        if (!clampRightPanelWidth()) return;
        if (typeof saveSettings === 'function') saveSettings(true);
      });
    });
  });
})();
