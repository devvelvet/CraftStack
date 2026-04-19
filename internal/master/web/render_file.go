package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

func buildFileManagerHTML(data map[string]interface{}) string {
	instI, _ := data["Instance"]
	inst, _ := instI.(*store.Instance)
	instID := ""
	if inst != nil {
		instID = inst.ID
	}

	return fmt.Sprintf(`
<style>
.fm-wrap{display:flex;height:calc(100vh - 6rem);background:#1e1e1e;border-radius:.5rem;overflow:hidden;color:#ccc;font-family:'D2Coding','Malgun Gothic','Microsoft YaHei','Yu Gothic',monospace;font-size:13px}
.fm-side{width:260px;min-width:160px;display:flex;flex-direction:column;background:#252526;border-right:1px solid #333}
.fm-side-hd{padding:6px 10px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.05em;color:#888;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid #333}
.fm-side-hd .fm-acts{display:flex;gap:2px}
.fm-side-hd .fm-acts button,.fm-side-hd .fm-acts label{background:none;border:none;color:#888;cursor:pointer;padding:2px 5px;border-radius:3px;font-size:14px;line-height:1}
.fm-side-hd .fm-acts button:hover,.fm-side-hd .fm-acts label:hover{background:#3c3c3c;color:#ddd}
.fm-tree{flex:1;overflow:auto;padding:2px 0}
.fm-tree::-webkit-scrollbar{width:6px}
.fm-tree::-webkit-scrollbar-thumb{background:#555;border-radius:3px}
.fm-ti{display:flex;align-items:center;height:22px;padding-right:8px;cursor:pointer;white-space:nowrap;user-select:none}
.fm-ti:hover{background:#2a2d2e}
.fm-ti.sel{background:#094771}
.fm-chev{width:16px;text-align:center;font-size:9px;color:#888;flex-shrink:0;transition:transform .12s}
.fm-chev.open{transform:rotate(90deg)}
.fm-ico{width:16px;text-align:center;flex-shrink:0;margin-right:4px;font-size:13px}
.fm-nm{overflow:hidden;text-overflow:ellipsis}
.fm-main{flex:1;display:flex;flex-direction:column;min-width:0}
.fm-tabs{display:flex;background:#252526;border-bottom:1px solid #333;min-height:35px;overflow-x:auto;flex-shrink:0}
.fm-tabs::-webkit-scrollbar{height:3px}
.fm-tb{display:flex;align-items:center;gap:6px;padding:0 12px;height:35px;font-size:13px;cursor:pointer;border-right:1px solid #252526;color:#888;position:relative;white-space:nowrap}
.fm-tb:hover{background:#2d2d2d}
.fm-tb.act{background:#1e1e1e;color:#fff}
.fm-tb.act::after{content:'';position:absolute;bottom:0;left:0;right:0;height:2px;background:#007acc}
.fm-tb .x{font-size:12px;padding:1px 3px;border-radius:3px;opacity:.6}
.fm-tb .x:hover{background:#555;opacity:1}
.fm-tb .dot{width:6px;height:6px;border-radius:50%%;background:#007acc;flex-shrink:0}
.fm-editor{flex:1;position:relative;overflow:hidden}
.fm-welcome{display:flex;align-items:center;justify-content:center;height:100%%;color:#555;flex-direction:column;gap:8px}
.fm-welcome svg{opacity:.3}
.fm-bar{display:flex;align-items:center;justify-content:space-between;padding:0 10px;height:22px;font-size:11px;background:#007acc;color:#fff;flex-shrink:0}
.fm-resize{width:4px;cursor:col-resize;flex-shrink:0}
.fm-resize:hover{background:#007acc80}
.fm-ctx{position:fixed;z-index:200;background:#252526;border:1px solid #454545;border-radius:4px;box-shadow:0 4px 12px #0008;padding:4px 0;min-width:160px}
.fm-ctx-i{padding:4px 12px;cursor:pointer;font-size:13px}
.fm-ctx-i:hover{background:#094771}
.fm-ctx-i.red{color:#f48771}
.fm-ctx-sep{height:1px;background:#454545;margin:4px 0}
.fm-drop{position:fixed;inset:0;z-index:300;display:flex;align-items:center;justify-content:center;background:#007acc22;border:2px dashed #007acc}
.fm-upload{padding:6px 10px;border-top:1px solid #333;font-size:11px}
.fm-upload progress{width:100%%;height:3px;margin-top:4px}
.fm-mobile-toggle{display:none;position:absolute;top:6px;left:6px;z-index:10;background:#333;border:1px solid #555;color:#ccc;border-radius:4px;padding:4px 8px;font-size:18px;cursor:pointer;line-height:1}
.fm-mobile-toggle:hover{background:#444}
@media(max-width:767px){
  .fm-wrap{flex-direction:column;height:calc(100vh - 5rem)}
  .fm-side{width:100%% !important;max-height:0;overflow:hidden;border-right:none;border-bottom:1px solid #333;transition:max-height .2s}
  .fm-side.fm-side-open{max-height:50vh}
  .fm-resize{display:none}
  .fm-mobile-toggle{display:block}
  .fm-tabs{min-height:30px}
  .fm-tb{padding:0 8px;height:30px;font-size:12px}
  .fm-bar{height:20px;font-size:10px}
}
</style>

<div id="fm-root" style="height:calc(100vh - 6rem);">
    <div class="fm-wrap" id="fm-wrap">
        <!-- drop overlay (hidden) -->
        <div class="fm-drop" id="fm-drop" style="display:none">
            <div style="text-align:center;color:#007acc">
                <div style="font-size:36px;margin-bottom:8px">&#8681;</div>
                <div style="font-size:16px;font-weight:600">here file drop</div>
            </div>
        </div>
        <!-- mobile sidebar toggle -->
        <button class="fm-mobile-toggle" onclick="var s=document.getElementById('fm-side');s.classList.toggle('fm-side-open');">&#9776;</button>
        <!-- sidebar -->
        <div class="fm-side" id="fm-side">
            <div class="fm-side-hd">
                <span>navigate</span>
                <div class="fm-acts">
                    <button onclick="FM.newFile()" title="new file">&#43;</button>
                    <button onclick="FM.newDir()" title="new folder">&#128193;</button>
                    <label title="upload" style="cursor:pointer">&#8593;<input type="file" style="display:none" onchange="FM.upload(this.files);this.value=''" multiple></label>
                    <button onclick="FM.refresh()" title="refresh">&#8635;</button>
                </div>
            </div>
            <div class="fm-tree" id="fm-tree"></div>
            <div class="fm-upload" id="fm-upload" style="display:none">
                <div id="fm-upload-text"></div>
                <progress id="fm-upload-bar" value="0" max="100"></progress>
            </div>
        </div>
        <!-- resize handle -->
        <div class="fm-resize" id="fm-resize"></div>
        <!-- main -->
        <div class="fm-main">
            <div class="fm-tabs" id="fm-tabs"></div>
            <div class="fm-editor" id="fm-editor">
                <div class="fm-welcome" id="fm-welcome">
                    <svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
                    <div style="font-size:14px">file optional editplease</div>
                    <div style="font-size:11px;opacity:.5">Ctrl+S save &middot; Ctrl+/ comment </div>
                </div>
            </div>
            <div class="fm-bar" id="fm-bar">
                <span id="fm-bar-path">file none</span>
                <span style="display:flex;align-items:center;gap:6px;">
                    <button id="fm-mobile-save" style="display:none;background:#0e7;border:none;color:#000;padding:1px 10px;border-radius:3px;font-size:11px;cursor:pointer;font-weight:600" onclick="FM.save()">save</button>
                    <button id="fm-mobile-del" style="display:none;background:#f55;border:none;color:#fff;padding:1px 10px;border-radius:3px;font-size:11px;cursor:pointer" onclick="FM.deleteActive()">delete</button>
                    <span id="fm-bar-lang"></span> &nbsp; <span id="fm-bar-pos"></span>
                </span>
            </div>
        </div>
    </div>
    <!-- context menu -->
    <div class="fm-ctx" id="fm-ctx" style="display:none"></div>
</div>

<script>
(function(){
const INST = '%s';
const API = '/api/instances/' + INST;

// ── state ──
const state = {
    tree: [],          // [{name,path,is_dir,expanded,loaded,children}]
    sel: '',           // selected path in tree
    tabs: [],          // [{path,name,lang,model,origContent,modified,truncated}]
    active: '',        // active tab path
    editor: null,
    monaco: null,
    sideW: 260,
};

// ── helpers ──
function esc(s){return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;')}
function isBin(n){
    const x=(n||'').split('.').pop().toLowerCase();
    return ['jar','zip','tar','gz','7z','rar','png','jpg','jpeg','gif','bmp','ico','webp','mp3','mp4','wav','avi','mkv','exe','dll','so','class','dat','db','sqlite','nbt','mca','mcr'].includes(x);
}
function fIcon(n){
    const x=(n||'').split('.').pop().toLowerCase();
    const m={java:'\u2615',jar:'\uD83D\uDCE6',yml:'\u2699',yaml:'\u2699',json:'{}',xml:'\uD83D\uDCC4',properties:'\u2699',js:'JS',ts:'TS',py:'\uD83D\uDC0D',sh:'>_',bat:'>_',md:'M\u2193',txt:'\uD83D\uDCC4',log:'\uD83D\uDCDC',sql:'\uD83D\uDDC3',html:'\uD83C\uDF10',css:'\uD83C\uDFA8',toml:'\u2699',ini:'\u2699',cfg:'\u2699',conf:'\u2699'};
    return m[x]||'\uD83D\uDCC4';
}
function sortEntries(arr){
    return arr.sort((a,b)=>{if(a.is_dir!==b.is_dir)return a.is_dir?-1:1;return a.name.localeCompare(b.name)});
}

// ── API ──
async function apiList(path){
    const r=await fetch(API+'/files?path='+encodeURIComponent(path||''));
    const d=await r.json(); return sortEntries(d.entries||[]);
}
async function apiRead(path){
    const r=await fetch(API+'/files/read?path='+encodeURIComponent(path));
    return await r.json();
}
async function apiWrite(path,content,createDirs){
    const r=await fetch(API+'/files',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({path,content:btoa(unescape(encodeURIComponent(content))),create_dirs:!!createDirs})});
    return await r.json();
}
async function apiDelete(path,recursive){
    const r=await fetch(API+'/files',{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({path,recursive})});
    return await r.json();
}
async function apiMkdir(path){
    const r=await fetch(API+'/files/mkdir',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path})});
    return await r.json();
}
async function apiUpload(file,dir){
    const fd=new FormData();fd.append('file',file);fd.append('path',dir||'');
    const r=await fetch(API+'/files/upload',{method:'POST',body:fd});
    return await r.json();
}

// ── Tree rendering ──
function renderTree(){
    const el=document.getElementById('fm-tree');
    el.innerHTML='';
    renderNodes(el,state.tree,0);
}
function renderNodes(parent,nodes,depth){
    for(const n of nodes){
        const row=document.createElement('div');
        row.className='fm-ti'+(n.path===state.sel?' sel':'');
        row.style.paddingLeft=(depth*16+8)+'px';
        // chevron
        const chev=document.createElement('span');
        chev.className='fm-chev'+(n.is_dir&&n.expanded?' open':'');
        chev.innerHTML=n.is_dir?'\u25B6':'';
        row.appendChild(chev);
        // icon
        const ico=document.createElement('span');
        ico.className='fm-ico';
        ico.textContent=n.is_dir?(n.expanded?'\uD83D\uDCC2':'\uD83D\uDCC1'):fIcon(n.name);
        row.appendChild(ico);
        // name
        const nm=document.createElement('span');
        nm.className='fm-nm';
        nm.textContent=n.name;
        row.appendChild(nm);
        // mobile action button
        if(window.__isMobile){
            const act=document.createElement('span');
            act.textContent='\u22EE';
            act.style.cssText='margin-left:auto;padding:0 6px;font-size:16px;opacity:.5;flex-shrink:0';
            act.addEventListener('click',function(e){e.stopPropagation();const r=act.getBoundingClientRect();onTreeCtx({clientX:r.left,clientY:r.bottom,preventDefault:()=>{},stopPropagation:()=>{}},n)});
            row.appendChild(act);
        }
        // click
        row.addEventListener('click',function(e){e.stopPropagation();onTreeClick(n)});
        row.addEventListener('contextmenu',function(e){e.preventDefault();e.stopPropagation();onTreeCtx(e,n)});
        parent.appendChild(row);
        // children
        if(n.is_dir&&n.expanded&&n.children.length>0){
            renderNodes(parent,n.children,depth+1);
        }
    }
}
async function onTreeClick(node){
    state.sel=node.path;
    if(node.is_dir){
        if(!node.loaded){
            const entries=await apiList(node.path);
            node.children=entries.map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));
            node.loaded=true;
        }
        node.expanded=!node.expanded;
        renderTree();
    } else {
        openFile(node.path,node.name);
    }
}
function onTreeCtx(event,node){
    const items=[];
    if(node.is_dir){
        items.push({label:'new file',action:()=>{state.sel=node.path;FM.newFile()}});
        items.push({label:'new folder',action:()=>{state.sel=node.path;FM.newDir()}});
        items.push({sep:true});
    } else {
        items.push({label:'open',action:()=>openFile(node.path,node.name)});
        items.push({label:'download',action:()=>{window.location.href=API+'/files/download?path='+encodeURIComponent(node.path)}});
        items.push({sep:true});
    }
    items.push({label:'delete',cls:'red',action:()=>deleteItem(node.path,node.is_dir)});
    showCtx(event.clientX,event.clientY,items);
}
function findNode(nodes,path){
    for(const n of nodes){if(n.path===path)return n;if(n.is_dir&&n.children.length){const f=findNode(n.children,path);if(f)return f;}}return null;
}
function getSelDir(){
    if(!state.sel)return '';
    const n=findNode(state.tree,state.sel);
    if(n&&n.is_dir)return n.path;
    const p=state.sel.split('/');p.pop();return p.join('/');
}

// ── Tabs ──
function renderTabs(){
    const el=document.getElementById('fm-tabs');
    el.innerHTML='';
    for(const t of state.tabs){
        const tb=document.createElement('div');
        tb.className='fm-tb'+(t.path===state.active?' act':'');
        if(t.modified){const dot=document.createElement('span');dot.className='dot';tb.appendChild(dot);}
        const nm=document.createElement('span');nm.textContent=t.name;nm.style.maxWidth='160px';nm.style.overflow='hidden';nm.style.textOverflow='ellipsis';tb.appendChild(nm);
        const x=document.createElement('span');x.className='x';x.innerHTML='&times;';
        x.addEventListener('click',function(e){e.stopPropagation();closeTab(t.path)});
        tb.appendChild(x);
        tb.addEventListener('click',function(){switchTab(t.path)});
        tb.addEventListener('auxclick',function(e){if(e.button===1){e.preventDefault();closeTab(t.path)}});
        tb.addEventListener('contextmenu',function(e){e.preventDefault();tabCtx(e,t.path)});
        el.appendChild(tb);
    }
}
async function openFile(path,name){
    if(isBin(name)){window.location.href=API+'/files/download?path='+encodeURIComponent(path);return}
    const existing=state.tabs.find(t=>t.path===path);
    if(existing){switchTab(path);return}
    try{
        const data=await apiRead(path);
        if(data.error){showToast(data.error,'error');return}
        const content=b64DecodeUTF8(data.content);
        const lang=extToLang(name);
        let model=null;
        if(state.monaco&&!window.__isMobile){model=state.monaco.editor.createModel(content,lang)}
        state.tabs.push({path,name,lang,model,origContent:content,modified:false,truncated:data.truncated||false});
        switchTab(path);
    }catch(e){showToast('file read failed: '+e.message,'error')}
}
function switchTab(path){
    state.active=path;
    state.sel=path;
    const tab=state.tabs.find(t=>t.path===path);
    if(!tab)return;
    document.getElementById('fm-welcome').style.display='none';
    document.getElementById('fm-bar-path').textContent=tab.path;
    document.getElementById('fm-bar-lang').textContent=tab.lang;
    // mobile save/delete button display
    const saveBtn=document.getElementById('fm-mobile-save');
    const delBtn=document.getElementById('fm-mobile-del');
    if(saveBtn)saveBtn.style.display=window.__isMobile?'':'none';
    if(delBtn)delBtn.style.display=window.__isMobile?'':'none';
    if(window.__isMobile){
        // textarea fallback
        if(state.editor){state.editor.getContainerDomNode().style.display='none'}
        let ta=document.getElementById('fm-mobile-ta');
        if(!ta){
            ta=document.createElement('textarea');
            ta.id='fm-mobile-ta';
            ta.style.cssText='width:100%%;height:100%%;background:#1e1e1e;color:#d4d4d4;border:none;padding:12px;font-family:D2Coding,monospace;font-size:14px;resize:none;outline:none;box-sizing:border-box;tab-size:4;-moz-tab-size:4';
            ta.spellcheck=false;
            document.getElementById('fm-editor').appendChild(ta);
        }
        ta.style.display='';
        ta.value=tab.model?tab.model.getValue():tab.origContent;
        ta.oninput=function(){
            const mod=ta.value!==tab.origContent;
            if(tab.modified!==mod){tab.modified=mod;renderTabs()}
            if(tab.model)tab.model.setValue(ta.value);
        };
    } else if(tab.model&&state.editor){
        let ta=document.getElementById('fm-mobile-ta');if(ta)ta.style.display='none';
        state.editor.getContainerDomNode().style.display='';
        state.editor.setModel(tab.model);
        state.editor.focus();
    }
    renderTabs();
    renderTree();
}
function closeTab(path){
    const tab=state.tabs.find(t=>t.path===path);
    if(!tab)return;
    if(tab.modified&&!confirm('"'+tab.name+'" modify close without saving?'))return;
    if(tab.model)tab.model.dispose();
    const idx=state.tabs.indexOf(tab);
    state.tabs.splice(idx,1);
    if(state.active===path){
        if(state.tabs.length>0){
            switchTab(state.tabs[Math.min(idx,state.tabs.length-1)].path);
        } else {
            state.active='';
            document.getElementById('fm-welcome').style.display='';
            if(state.editor)state.editor.getContainerDomNode().style.display='none';
            const ta=document.getElementById('fm-mobile-ta');if(ta)ta.style.display='none';
            const saveBtn=document.getElementById('fm-mobile-save');if(saveBtn)saveBtn.style.display='none';
            const delBtn=document.getElementById('fm-mobile-del');if(delBtn)delBtn.style.display='none';
            document.getElementById('fm-bar-path').textContent='file none';
            document.getElementById('fm-bar-lang').textContent='';
            document.getElementById('fm-bar-pos').textContent='';
        }
    }
    renderTabs();
}
function tabCtx(event,path){
    showCtx(event.clientX,event.clientY,[
        {label:'save',action:()=>{switchTab(path);saveActive()}},
        {label:'close',action:()=>closeTab(path)},
        {label:'other tab all close',action:()=>{state.tabs.filter(t=>t.path!==path).forEach(t=>{t.modified=false;closeTab(t.path)})}},
        {sep:true},
        {label:'download',action:()=>{window.location.href=API+'/files/download?path='+encodeURIComponent(path)}},
    ]);
}

// ── Save ──
async function saveActive(){
    const tab=state.tabs.find(t=>t.path===state.active);
    if(!tab)return;
    let content;
    if(window.__isMobile){
        const ta=document.getElementById('fm-mobile-ta');
        if(!ta)return;
        content=ta.value;
    } else {
        if(!tab.model)return;
        content=tab.model.getValue();
    }
    try{
        const data=await apiWrite(tab.path,content,false);
        if(data.success){showToast('save complete: '+tab.name,'success');tab.origContent=content;tab.modified=false;renderTabs()}
        else{showToast(data.error||data.message,'error')}
    }catch(e){showToast('save failed: '+e.message,'error')}
}

// ── Delete ──
async function deleteItem(path,isDir){
    const name=path.split('/').pop();
    if(!confirm((isDir?'folder':'file')+' "'+name+'"() delete?'))return;
    try{
        const data=await apiDelete(path,isDir);
        if(data.success){
            showToast('deleted','success');
            if(!isDir){const t=state.tabs.find(t=>t.path===path);if(t){t.modified=false;closeTab(path)}}
            else{state.tabs.filter(t=>t.path.startsWith(path+'/')).forEach(t=>{t.modified=false;closeTab(t.path)})}
            await FM.refresh();
        }else{showToast(data.error||data.message,'error')}
    }catch(e){showToast('delete failed: '+e.message,'error')}
}

// ── Context menu ──
function showCtx(x,y,items){
    const el=document.getElementById('fm-ctx');
    el.innerHTML='';
    for(const it of items){
        if(it.sep){const s=document.createElement('div');s.className='fm-ctx-sep';el.appendChild(s);continue}
        const d=document.createElement('div');
        d.className='fm-ctx-i'+(it.cls?' '+it.cls:'');
        d.textContent=it.label;
        d.addEventListener('click',function(){el.style.display='none';it.action()});
        el.appendChild(d);
    }
    el.style.left=x+'px';el.style.top=y+'px';el.style.display='';
}
document.addEventListener('click',()=>{document.getElementById('fm-ctx').style.display='none'});

// ── Resize ──
(function(){
    const handle=document.getElementById('fm-resize');
    let startX,startW;
    handle.addEventListener('mousedown',function(e){
        startX=e.clientX;startW=state.sideW;
        function mv(e){state.sideW=Math.max(150,Math.min(600,startW+(e.clientX-startX)));document.getElementById('fm-side').style.width=state.sideW+'px'}
        function up(){document.removeEventListener('mousemove',mv);document.removeEventListener('mouseup',up)}
        document.addEventListener('mousemove',mv);document.addEventListener('mouseup',up);
    });
})();

// ── Drag & Drop ──
(function(){
    const wrap=document.getElementById('fm-wrap');
    const drop=document.getElementById('fm-drop');
    let cnt=0;
    wrap.addEventListener('dragenter',function(e){e.preventDefault();cnt++;drop.style.display=''});
    wrap.addEventListener('dragleave',function(e){e.preventDefault();cnt--;if(cnt<=0){cnt=0;drop.style.display='none'}});
    wrap.addEventListener('dragover',function(e){e.preventDefault()});
    wrap.addEventListener('drop',function(e){
        e.preventDefault();cnt=0;drop.style.display='none';
        const files=[];
        if(e.dataTransfer.items){for(let i=0;i<e.dataTransfer.items.length;i++){if(e.dataTransfer.items[i].kind==='file')files.push(e.dataTransfer.items[i].getAsFile())}}
        if(files.length)FM.upload(files);
    });
})();

// ── Keyboard ──
document.addEventListener('keydown',function(e){
    if((e.ctrlKey||e.metaKey)&&e.key==='s'){if(state.active){e.preventDefault();saveActive()}}
    if((e.ctrlKey||e.metaKey)&&e.key==='w'){if(state.active){e.preventDefault();closeTab(state.active)}}
});

// ── Public API ──
window.FM = {
    save(){saveActive()},
    deleteActive(){
        const tab=state.tabs.find(t=>t.path===state.active);
        if(tab)deleteItem(tab.path,false);
    },
    async refresh(){
        // collect expanded folder paths (restore after rebuild)
        const expanded=[];collectExpanded(state.tree,expanded);
        // add open tab folders
        state.tabs.forEach(t=>{const p=t.path.split('/');p.pop();while(p.length>0){expanded.push(p.join('/'));p.pop()}});
        const entries=await apiList('');
        state.tree=entries.map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));
        await reExpand(state.tree,expanded);
        renderTree();
    },
    async newFile(){
        const name=prompt('new file name:');if(!name)return;
        const dir=getSelDir();const path=dir?dir+'/'+name:name;
        try{
            const d=await apiWrite(path,'',true);
            if(d.success){showToast('create file','success');await FM.refresh();openFile(path,name)}
            else showToast(d.error||d.message,'error');
        }catch(e){showToast('failed: '+e.message,'error')}
    },
    async newDir(){
        const name=prompt('new folder name:');if(!name)return;
        const dir=getSelDir();const path=dir?dir+'/'+name:name;
        try{
            const d=await apiMkdir(path);
            if(d.success){showToast('folder created','success');await FM.refresh()}
            else showToast(d.error||d.message,'error');
        }catch(e){showToast('failed: '+e.message,'error')}
    },
    async upload(fileList){
        const upEl=document.getElementById('fm-upload');
        const txt=document.getElementById('fm-upload-text');
        const bar=document.getElementById('fm-upload-bar');
        upEl.style.display='';
        const dir=getSelDir();let done=0;
        for(let i=0;i<fileList.length;i++){
            txt.textContent=fileList[i].name+' ('+(i+1)+'/'+fileList.length+')';
            bar.value=Math.round(i/fileList.length*100);
            try{const d=await apiUpload(fileList[i],dir);if(d.success)done++;else showToast(fileList[i].name+': failed','error')}
            catch(e){showToast(fileList[i].name+' upload failed','error')}
        }
        bar.value=100;
        setTimeout(()=>{upEl.style.display='none'},1000);
        showToast(done+' file upload complete','success');
        await FM.refresh();
    }
};

function collectExpanded(nodes,out){
    for(const n of nodes){
        if(n.is_dir&&n.expanded)out.push(n.path);
        if(n.is_dir&&n.children.length>0)collectExpanded(n.children,out);
    }
}
async function reExpand(nodes,expandedPaths){
    // expandedPaths: previously expanded folder paths + folders of files open in tabs
    for(const n of nodes){
        if(n.is_dir&&expandedPaths.some(p=>p===n.path||p.startsWith(n.path+'/'))){
            const entries=await apiList(n.path);
            n.children=entries.map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));
            n.loaded=true;n.expanded=true;
            await reExpand(n.children,expandedPaths);
        }
    }
}

// ── Init ──
async function init(){
    // load root tree
    const entries=await apiList('');
    state.tree=entries.map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));
    renderTree();
    if(window.__isMobile){
        // mobile: Monaco skip — textarea fallback use
        state.monaco=null;state.editor=null;
        return;
    }
    state.monaco=await window.__monacoReady;
    // create editor
    const container=document.getElementById('fm-editor');
    state.editor=state.monaco.editor.create(container,{
        value:'',language:'plaintext',theme:'vs-dark',fontSize:14,
        fontFamily:"'D2Coding','Malgun Gothic','Microsoft YaHei','Yu Gothic','Noto Sans Mono',monospace",
        minimap:{enabled:true},wordWrap:'on',automaticLayout:true,
        scrollBeyondLastLine:false,renderWhitespace:'selection',tabSize:4,
        padding:{top:8,bottom:8},
    });
    state.editor.getContainerDomNode().style.display='none';
    // Ctrl+S
    state.editor.addCommand(state.monaco.KeyMod.CtrlCmd|state.monaco.KeyCode.KeyS,()=>saveActive());
    // cursor pos
    state.editor.onDidChangeCursorPosition(e=>{document.getElementById('fm-bar-pos').textContent='line '+e.position.lineNumber+', col '+e.position.column});
    // content change → modified flag
    state.editor.onDidChangeModelContent(()=>{
        const tab=state.tabs.find(t=>t.path===state.active);
        if(tab&&tab.model){const mod=tab.model.getValue()!==tab.origContent;if(tab.modified!==mod){tab.modified=mod;renderTabs()}}
    });
}
init();
})();
</script>`, instID)
}
