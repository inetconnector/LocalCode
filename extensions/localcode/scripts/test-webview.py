"""Exercise the real webview assets with the narrow VS Code bridge mocked."""
import json
import os
from pathlib import Path
from playwright.sync_api import sync_playwright

root = Path(__file__).resolve().parents[1]
out = root.parents[1] / 'logs' / 'ide-ui'
out.mkdir(parents=True, exist_ok=True)
with sync_playwright() as p:
    browser = p.chromium.launch(channel='msedge', headless=True)
    for language in ['en', 'de']:
        page = browser.new_page(viewport={'width': 470, 'height': 950})
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.add_init_script("""window.requests=[]; window.saved={}; window.acquireVsCodeApi=()=>({postMessage:m=>window.requests.push(m),getState:()=>window.saved,setState:s=>window.saved=s});""")
        def route_handler(route):
            name = route.request.url.split('http://localcode.test/', 1)[1]
            if name in ['', 'index.html']:
                content = (root/'media/index.html').read_text(encoding='utf-8').replace('{{csp}}', 'http://localcode.test').replace('{{nonce}}', 'testnonce').replace('{{style}}', 'http://localcode.test/style.css').replace('{{script}}', 'http://localcode.test/app.js')
                route.fulfill(body=content, content_type='text/html')
            elif name in ['style.css', 'app.js']:
                route.fulfill(body=(root/'media'/name).read_text(encoding='utf-8'), content_type='text/css' if name.endswith('.css') else 'application/javascript')
            else:
                route.fulfill(status=404)
        page.route('http://localcode.test/**', route_handler)
        page.goto('http://localcode.test/')
        strings = json.loads((root/'media'/f'{language}.json').read_text(encoding='utf-8'))
        state = {'type':'state','language':language,'strings':strings,'connected':True,'project':r'C:\Projects\demo','thread':'t1','events':[],'models':[{'name':'qwen2.5-coder:14b'}],'model':'qwen2.5-coder:14b','engine':'native','running':False,'busy':False,'attachments':[]}
        def update():
            page.evaluate('(data)=>window.dispatchEvent(new MessageEvent("message",{data}))', state)
        update()
        page.get_by_role('button', name=strings['analyze'], exact=True).click()
        assert page.locator('#prompt').input_value() == strings['analyze']
        page.locator('#send').click()
        assert page.evaluate('window.requests.at(-1).type') == 'send'
        page.evaluate('(prompt)=>window.dispatchEvent(new MessageEvent("message",{data:{type:"sent",prompt}}))', strings['analyze'])
        assert page.locator('#prompt').input_value() == ''
        state['running'] = True
        state['events'] = [{'id':'e1','type':'user','message':'Build a small project dashboard.'},{'id':'e2','type':'assistant','message':'I will inspect **the workspace** and run the tests.\n```js\nconst local = true;\n```'},{'id':'e3','type':'tool_result','action':'read_file','message':'src/app.js','detail':'<img src=x onerror="window.injected=true">'}]
        update()
        page.locator('#prompt').fill('Use German. Keep the original requirements.')
        page.locator('#send').click()
        assert page.evaluate('window.requests.at(-1).message') == 'Use German. Keep the original requirements.'
        assert page.locator('#stop').is_visible()
        assert not page.evaluate('Boolean(window.injected)')
        page.locator('.event details summary').click()
        assert page.locator('.event details pre').is_visible()
        state['pending'] = {'id':'approval-1','message':'Update src/app.js','action':'replace_text','preview':'@@ -1 +1 @@\n-old\n+new'}
        state['attachments'] = ['src/app.js:12 (editor context)']
        update()
        page.locator('#approve').click()
        assert page.evaluate('window.requests.at(-1)') == {'type':'approve','id':'approval-1','decision':'once'}
        page.locator('#reject').click()
        assert page.evaluate('window.requests.at(-1).decision') == 'reject'
        for action in ['refresh', 'start', 'desktop', 'settings']:
            page.locator('#menu summary').click()
            page.locator(f'[data-action="{action}"]').click()
            assert page.evaluate('window.requests.at(-1).type') == action
        for action in ['chooseTask','chooseWorkspace','addContext','addDiagnostics','clearContext','review','stop','newTask']:
            page.locator(f'[data-action="{action}"]').click()
            assert page.evaluate('window.requests.at(-1).type') == action
        assert not errors, errors
        page.screenshot(path=str(out/f'localcode-{language}.png'), full_page=True)
        page.set_viewport_size({'width': 280, 'height': 700})
        assert page.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'horizontal overflow'
        page.close()
    browser.close()
print('WEBVIEW E2E PASSED: DE/EN, prompts during runs, approval, every visible menu, context, code blocks, XSS, narrow layout')
