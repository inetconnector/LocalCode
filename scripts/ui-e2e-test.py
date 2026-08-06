import json
from pathlib import Path
from playwright.sync_api import sync_playwright

static=Path(__file__).resolve().parents[1]/'src'/'static'
threads=[{'id':'t1','project':'C:\\Users\\frede\\Projekte\\Geschichten','title':'Analyse','model':'qwen2.5-coder:14b','updated_at':'2026-08-05T00:00:00Z'}]
projects=[{'path':'C:\\Users\\frede\\Projekte\\Geschichten','name':'Geschichten','pinned':False},{'path':'C:\\Users\\frede\\Projekte\\FritzShare','name':'FritzShare','pinned':True}]
mcp_servers=[
 {'name':'filesystem','display_name':'Filesystem','description':'Sichere Datei- und Verzeichnisoperationen innerhalb des aktiven Projekts.','enabled':True,'transport':'builtin','preset':'filesystem','auto_install':False},
 {'name':'powershell','display_name':'PowerShell','description':'PowerShell-Befehle, Cmdlet-Erkennung und Hilfetexte mit LocalCode-Genehmigungen.','enabled':True,'transport':'builtin','preset':'powershell','auto_install':True},
 {'name':'git','display_name':'Git','description':'Git-Status, Diff, Historie, Branches, Staging und Commits mit sicherer Argumentübergabe.','enabled':True,'transport':'builtin','preset':'git','auto_install':True},
 {'name':'fetch','display_name':'Fetch','description':'Offizieller MCP-Referenzserver zum Abrufen und Umwandeln von Webseiten.','enabled':True,'transport':'stdio','preset':'fetch','auto_install':True},
 {'name':'github','display_name':'GitHub','description':'Offizieller GitHub MCP Server für Repositories, Issues, Pull Requests und Actions.','enabled':False,'transport':'streamable-http','preset':'github','auto_install':False},
 {'name':'playwright','display_name':'Playwright Browser','description':'Offizieller Microsoft Playwright MCP Server für Browserautomation; ersetzt den archivierten Puppeteer-Server.','enabled':True,'transport':'stdio','preset':'playwright','auto_install':True}
]
current='t1'; selected=projects[0]['path']; posts=[]

def fulfill(route,obj,status=200): route.fulfill(status=status,content_type='application/json',body=json.dumps(obj))
def handler(route):
    global current,selected,threads,projects
    req=route.request; path=req.url.split('/api/',1)[1].split('?',1)[0]; method=req.method
    data={}
    if req.post_data:
        try:data=json.loads(req.post_data)
        except:pass
    posts.append((path,method,data))
    if path=='status': return fulfill(route,{'version':'6.4.1','resolved_language':'de','system_language':'de','project':selected,'selected_model':'qwen2.5-coder:14b','models':[{'name':'qwen2.5-coder:14b'}],'root_dir':'C:\\Users\\frede\\Projekte','ollama_online':True,'ollama_url':'http://127.0.0.1:11434','editing_engine':'aider','engine_installed':True,'engine_authenticated':True,'engine_version':'aider 0.86.2','engine_executable':'aider.exe','aider_installed':True,'aider_version':'aider 0.86.2'})
    if path=='projects': return fulfill(route,{'root':'C:\\Users\\frede\\Projekte','projects':projects,'hidden_projects':[]})
    if path=='threads': return fulfill(route,{'threads':threads,'current':current})
    if path=='snapshot':
        tid=req.url.split('thread_id=',1)[1] if 'thread_id=' in req.url else current
        t=next((x for x in threads if x['id']==tid),None)
        return fulfill(route,{'events':[],'project':t['project'] if t else selected,'model':'qwen2.5-coder:14b','running':False,'current_thread':tid,'run_phase':'idle'})
    if path=='settings':
        if method=='GET': return fulfill(route,{'schema_version':9,'editing_engine':'aider','aider_enabled':True,'aider_auto_install':True,'aider_version':'0.86.2','aider_edit_format':'diff','aider_editor_edit_format':'editor-diff','aider_map_tokens':4096,'aider_max_chat_history_tokens':8192,'aider_auto_lint':True,'aider_auto_test':True,'aider_use_git':True,'claude_code_enabled':True,'claude_code_auto_install':True,'claude_code_channel':'stable','claude_code_model':'sonnet','claude_code_permission_mode':'acceptEdits','claude_code_max_turns':24,'opencode_enabled':True,'opencode_auto_install':True,'opencode_version':'latest','opencode_model':'ollama/qwen2.5-coder:14b','opencode_agent':'build','opencode_auto_approve':True,'approval_mode':'strict','sandbox_mode':'project','language':'auto','shortcuts':{},'approval_rules':[],'ui_left_width':296,'ui_right_width':340,'ui_terminal_height':260,'show_bottom_bar':True,'mcp_servers':mcp_servers})
        return fulfill(route,{'ok':True})
    if path=='new-chat':
        selected=data.get('project',selected); current='t'+str(len(threads)+1); new={'id':current,'project':selected,'title':'Neuer Chat','model':'qwen2.5-coder:14b','updated_at':'2026-08-05T00:01:00Z'}; threads.insert(0,new); return fulfill(route,{'ok':True,'thread':new})
    if path=='select-project': selected=data['path']; return fulfill(route,{'ok':True,'project':selected})
    if path=='select-chat': current=data['id']; return fulfill(route,{'ok':True})
    if path=='project-action':
        for p in list(projects):
            if p['path']==data['path']:
                if data['action']=='rename': p['name']=data['value']
                if data['action']=='pin': p['pinned']=True
                if data['action']=='unpin': p['pinned']=False
                if data['action']=='remove': projects.remove(p)
                return fulfill(route,{'ok':True,'project':p})
    if path=='rename-chat':
        for t in threads:
            if t['id']==data['id']: t['title']=data['title']
        return fulfill(route,{'ok':True})
    if path=='duplicate-chat':
        src=next(t for t in threads if t['id']==data['id']); dup=dict(src,id='td',title=src['title']+' – Kopie'); threads.insert(0,dup); return fulfill(route,{'ok':True,'thread':dup})
    if path=='archive-chat':
        for t in threads:
            if t['id']==data['id']: t['archived']=data.get('archived',True)
        return fulfill(route,{'ok':True})
    if path=='delete-chat':
        threads[:]=[t for t in threads if t['id']!=data['id']]; return fulfill(route,{'ok':True})
    if path in ('open-project','open-terminal','open-chat-window','approve','stop','force-stop','chat'): return fulfill(route,{'ok':True})
    if path=='engines/status':
        engine=(req.url.split('engine=',1)[1].split('&',1)[0] if 'engine=' in req.url else 'aider')
        values={
          'aider':{'engine':'aider','display_name':'Aider','enabled':True,'installed':True,'authenticated':True,'executable':'aider.exe','version':'aider 0.86.2','expected_version':'0.86.2','installation_root':'C:\\Users\\frede\\AppData\\Local\\LocalCode\\tools\\aider'},
          'claude':{'engine':'claude','display_name':'Claude Code','enabled':True,'installed':True,'authenticated':True,'executable':'claude.exe','version':'2.1.211 (Claude Code)','expected_version':'stable','installation_root':'C:\\Users\\frede\\.local\\bin'},
          'opencode':{'engine':'opencode','display_name':'OpenCode','enabled':True,'installed':True,'authenticated':True,'executable':'opencode.cmd','version':'1.2.3','expected_version':'latest','installation_root':'C:\\Users\\frede\\AppData\\Local\\LocalCode\\tools\\opencode'},
          'native':{'engine':'native','display_name':'LocalCode nativ','enabled':True,'installed':True,'authenticated':True,'executable':'embedded','version':'6.4.1'}
        }
        return fulfill(route,{'selected':engine,'status':values[engine],'engines':list(values.values())})
    if path=='engines/setup':
        engine=data.get('engine','aider'); action=data.get('action','test'); names={'aider':'Aider','claude':'Claude Code','opencode':'OpenCode','native':'LocalCode nativ'}
        return fulfill(route,{'ok':True,'status':{'engine':engine,'display_name':names[engine],'installed':True,'authenticated':True,'version':'test-version','executable':engine+'.exe'},'detail':action+' successful'})
    if path=='engines/undo': return fulfill(route,{'ok':True,'detail':'Restored files: 1'})
    # Compatibility routes remain available for older front ends.
    if path=='aider/status': return fulfill(route,{'enabled':True,'installed':True,'executable':'aider.exe','version':'aider 0.86.2','expected_version':'0.86.2'})
    if path=='aider/setup': return fulfill(route,{'ok':True,'status':{'installed':True,'version':'aider 0.86.2','expected_version':'0.86.2','executable':'aider.exe'},'detail':'Repository map: sample'})
    if path=='aider/undo': return fulfill(route,{'ok':True,'detail':'Restored files: 1'})
    if path=='mcp/status': return fulfill(route,{'servers':[
      {'name':'filesystem','display_name':'Filesystem','preset':'filesystem','enabled':True,'installed':True,'connected':True,'tool_count':4,'tools':['list_directory','read_text_file','write_file','search_files']},
      {'name':'powershell','display_name':'PowerShell','preset':'powershell','enabled':True,'installed':True,'connected':True,'tool_count':3,'tools':['powershell_run','powershell_get_command','powershell_get_help']},
      {'name':'git','display_name':'Git','preset':'git','enabled':True,'installed':True,'connected':True,'tool_count':4,'tools':['git_status','git_diff','git_commit','git_push']},
      {'name':'fetch','display_name':'Fetch','preset':'fetch','enabled':True,'installed':False,'connected':False,'error':'uvx fehlt'},
      {'name':'github','display_name':'GitHub','preset':'github','enabled':False,'installed':True,'connected':False,'auth_required':True,'error':'GitHub-Anmeldung erforderlich'},
      {'name':'playwright','display_name':'Playwright Browser','preset':'playwright','enabled':True,'installed':False,'connected':False,'error':'npx fehlt'}
    ]})
    if path=='mcp/setup': return fulfill(route,{'ok':True,'detail':'done','settings':{'mcp_servers':mcp_servers},'status':{'name':data.get('name'),'enabled':True,'installed':True,'connected':False}})
    if path=='mcp/test': return fulfill(route,{'results':{'filesystem':{'ok':True}}})
    if path=='tools': return fulfill(route,{'tools':[]})
    if path=='git-overview': return fulfill(route,{'branch':'main','status':'','diff':'','log':'','errors':[]})
    return fulfill(route,{'ok':True})

def project_menu(page):
    page.locator('.project-row').first.click(button='right'); page.wait_for_selector('#contextMenu:not(.hidden)')
def thread_menu(page):
    page.locator('.thread-row').first.click(button='right'); page.wait_for_selector('#contextMenu:not(.hidden)')

with sync_playwright() as p:
    browser=p.chromium.launch(headless=True,executable_path='/usr/bin/chromium',args=['--no-sandbox'])
    page=browser.new_page(viewport={'width':1440,'height':900}); page.set_default_timeout(10000); errors=[]
    page.on('pageerror',lambda e: errors.append(str(e))); page.route('http://localcode.test/api/**',handler)
    html=(static/'index.html').read_text(); i18n=(static/'i18n.js').read_text()
    html=html.replace('<head>','<head><base href="http://localcode.test/">',1).replace('<script src="/i18n.js"></script>','<script>'+i18n+'</script>').replace('setMainGrid();connectEvents();startHealthMonitor();','setMainGrid();startHealthMonitor();')
    page.set_content(html,wait_until='load'); page.wait_for_selector('.project-group')
    assert page.locator('.new-task-row').first.is_visible(); page.locator('.new-task-row').first.click(); page.wait_for_timeout(150)
    assert any(p[0]=='new-chat' for p in posts)
    project_menu(page); labels=page.locator('#contextMenu > .context-menu-item .menu-label').all_text_contents()
    for x in ['Neue Aufgabe','Name bearbeiten','Öffnen in','Im integrierten Terminal öffnen','Im Datei-Explorer öffnen','Projekt anheften','Projekt entfernen']: assert x in labels
    # Rename
    page.locator('#contextMenu > .context-menu-item',has_text='Name bearbeiten').click(); page.locator('#actionModalInput').fill('Geschichten Neu'); page.locator('#actionModalConfirm').click(); page.wait_for_timeout(150)
    assert any(x[0]=='project-action' and x[2].get('action')=='rename' for x in posts)
    # Open in Visual Studio submenu
    project_menu(page); parent=page.locator('#contextMenu > .context-menu-item',has_text='Öffnen in'); parent.hover(); page.get_by_role('menuitem',name='Visual Studio',exact=True).click(); page.wait_for_timeout(100)
    assert any(x[0]=='open-project' and x[2].get('target')=='visualstudio' for x in posts)
    # Integrated terminal
    project_menu(page); page.locator('#contextMenu > .context-menu-item',has_text='Im integrierten Terminal öffnen').click(); assert page.locator('#terminalPanel').is_visible(); page.locator('#closeTerminalBtn').click()
    # Explorer
    project_menu(page); page.locator('#contextMenu > .context-menu-item',has_text='Im Datei-Explorer öffnen').click(); page.wait_for_timeout(100)
    assert any(x[0]=='open-project' and x[2].get('target')=='explorer' for x in posts)
    # Pin
    project_menu(page); page.locator('#contextMenu > .context-menu-item',has_text='Projekt anheften').click(); page.wait_for_timeout(150)
    assert any(x[0]=='project-action' and x[2].get('action')=='pin' for x in posts)
    # Task actions
    thread_menu(page); labels=page.locator('#contextMenu > .context-menu-item .menu-label').all_text_contents()
    for x in ['Aufgabe umbenennen','In neuem Fenster öffnen','Aufgabe duplizieren','Aufgabe archivieren','Aufgabe löschen']: assert x in labels
    page.locator('#contextMenu > .context-menu-item',has_text='In neuem Fenster öffnen').click(); page.wait_for_timeout(100)
    assert any(x[0]=='open-chat-window' for x in posts)
    thread_menu(page); page.locator('#contextMenu > .context-menu-item',has_text='Aufgabe umbenennen').click(); page.locator('#actionModalInput').fill('Neu benannt'); page.locator('#actionModalConfirm').click(); page.wait_for_timeout(150)
    assert any(x[0]=='rename-chat' for x in posts)
    thread_menu(page); page.locator('#contextMenu > .context-menu-item',has_text='Aufgabe duplizieren').click(); page.wait_for_timeout(150)
    assert any(x[0]=='duplicate-chat' for x in posts)
    # Keyboard navigation and no invalid nested buttons.
    project_menu(page); page.wait_for_timeout(80); items=page.locator('#contextMenu > .context-menu-item'); items.first.focus(); assert page.evaluate("document.activeElement === document.querySelectorAll('#contextMenu > .context-menu-item')[0]"); page.keyboard.press('ArrowDown'); page.wait_for_timeout(80); assert page.evaluate("document.activeElement === document.querySelectorAll('#contextMenu > .context-menu-item')[1]")
    assert page.locator('#contextMenu button button').count()==0
    page.keyboard.press('Escape')
    assert page.locator('#approvalAlwaysBtn').count()==1 and page.locator('#approvalGlobalBtn').count()==1
    # Managed MCP cards are real controls, not placeholders.
    page.evaluate("openSettings('general')")
    page.wait_for_timeout(100)
    page.locator('.settings-nav-btn[data-settings="configuration"]').click()
    page.wait_for_selector('.setting-panel[data-panel="configuration"].active')
    assert page.locator('#setEditingEngine').input_value()=='aider'
    assert page.locator('#aiderEngineSettings').is_visible()
    assert not page.locator('#claudeEngineSettings').is_visible()
    assert not page.locator('#openCodeEngineSettings').is_visible()
    page.locator('#engineStatusBtn').click(); page.wait_for_timeout(150)
    assert 'Aider' in page.locator('#engineResult').inner_text() and '0.86.2' in page.locator('#engineResult').inner_text()
    assert any(x[0]=='engines/status' and 'engine=aider' in x[0]+'?'+page.url for x in posts) or any(x[0]=='engines/status' for x in posts)
    # Every external engine can be selected and uses the same status/setup API.
    page.locator('#setEditingEngine').select_option('claude'); page.wait_for_timeout(50)
    assert page.locator('#claudeEngineSettings').is_visible() and not page.locator('#aiderEngineSettings').is_visible()
    page.locator('#engineStatusBtn').click(); page.wait_for_timeout(150)
    assert 'Claude Code' in page.locator('#engineResult').inner_text()
    page.locator('#engineTestBtn').click(); page.wait_for_timeout(150)
    assert any(x[0]=='engines/setup' and x[2].get('engine')=='claude' and x[2].get('action')=='test' for x in posts)
    page.locator('#setEditingEngine').select_option('opencode'); page.wait_for_timeout(50)
    assert page.locator('#openCodeEngineSettings').is_visible() and not page.locator('#claudeEngineSettings').is_visible()
    page.locator('#engineStatusBtn').click(); page.wait_for_timeout(150)
    assert 'OpenCode' in page.locator('#engineResult').inner_text()
    page.locator('#engineTestBtn').click(); page.wait_for_timeout(150)
    assert any(x[0]=='engines/setup' and x[2].get('engine')=='opencode' and x[2].get('action')=='test' for x in posts)
    page.locator('#setEditingEngine').select_option('aider'); page.wait_for_timeout(50)
    page.locator('.settings-nav-btn[data-settings="plugins"]').click()
    page.wait_for_selector('.setting-panel[data-panel="plugins"].active')
    assert page.locator('#mcpCards .mcp-managed-card').count()==6
    assert page.locator('#mcpCards').get_by_text('Filesystem',exact=True).count()==1
    assert page.locator('#mcpCards').get_by_text('PowerShell',exact=True).count()==1
    assert page.locator('#mcpCards').get_by_text('Git',exact=True).count()==1
    assert page.locator('#mcpCards').get_by_text('GitHub',exact=True).count()==1
    assert page.locator('#mcpCards').get_by_text('Playwright Browser',exact=True).count()==1
    page.locator('[data-mcp="fetch"][data-mcp-action="install"]').click(); page.wait_for_timeout(100)
    page.locator('[data-mcp="github"][data-mcp-action="authenticate"]').click(); page.wait_for_timeout(100)
    page.locator('[data-mcp="playwright"][data-mcp-action="reset"]').click(); page.wait_for_timeout(100)
    assert any(x[0]=='mcp/setup' and x[2].get('action')=='install' and x[2].get('name')=='fetch' for x in posts)
    assert any(x[0]=='mcp/setup' and x[2].get('action')=='authenticate' and x[2].get('name')=='github' for x in posts)
    assert any(x[0]=='mcp/setup' and x[2].get('action')=='reset' and x[2].get('name')=='playwright' for x in posts)
    assert not errors,errors
    browser.close()
print('FULL UI E2E OK',len(posts),'requests')
