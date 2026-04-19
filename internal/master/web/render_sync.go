package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

func buildSyncHistoryHTML(data map[string]interface{}, tableOnly bool) string {
	histI, ok := data["SyncHistory"]
	if !ok {
		histI, ok = data["History"]
	}
	if !ok || histI == nil {
		return syncEmptyHTML(tableOnly)
	}

	records, ok := histI.([]*store.SyncRecord)
	if !ok || len(records) == 0 {
		return syncEmptyHTML(tableOnly)
	}

	var rows string
	for _, r := range records {
		badge := statusBadgeHTML(r.Status)
		actionLabel := "deploy"
		if r.Action == "delete" {
			actionLabel = "delete"
		}
		sizeStr := formatFileSize(r.FileSize)
		timeStr := r.SyncedAt.Format("01/02 15:04:05")

		nodeStr := "-"
		if r.NodeID != nil {
			ns := *r.NodeID
			if len(ns) > 8 {
				ns = ns[:8]
			}
			nodeStr = ns
		}

		errStr := ""
		if r.ErrorMsg != nil && *r.ErrorMsg != "" {
			errStr = fmt.Sprintf(`<div class="text-xs text-error">%s</div>`, *r.ErrorMsg)
		}

		rows += fmt.Sprintf(`<tr>
			<td class="text-xs">%s</td>
			<td>
				<div class="text-sm font-mono">%s</div>
				<div class="text-xs opacity-60">%s</div>
				%s
			</td>
			<td class="text-xs">%s</td>
			<td class="text-xs opacity-60">%s</td>
			<td>%s</td>
		</tr>`, timeStr, r.FilePath, sizeStr, errStr, actionLabel, nodeStr, badge)
	}

	table := fmt.Sprintf(`
    <div class="overflow-x-auto">
        <table class="table table-zebra table-sm">
            <thead><tr><th>time</th><th>file</th><th></th><th>target</th><th>state</th></tr></thead>
            <tbody>%s</tbody>
        </table>
    </div>`, rows)

	if tableOnly {
		return table
	}
	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">sync history</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">full history (%d total)</h2>
        </div>
        <div hx-get="/htmx/sync-history" hx-trigger="every 10s" hx-swap="innerHTML">
        %s
        </div>
    </div></div>`, len(records), table)
}

func syncEmptyHTML(tableOnly bool) string {
	table := `
    <div class="overflow-x-auto">
        <table class="table table-zebra table-sm">
            <thead><tr><th>time</th><th>file</th><th></th><th>target</th><th>state</th></tr></thead>
            <tbody>
                <tr><td colspan="5" class="text-center text-gray-500">sync history is missing</td></tr>
            </tbody>
        </table>
    </div>`
	if tableOnly {
		return table
	}
	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">sync history</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">%s</div></div>`, table)
}


func buildSyncPageHTML(data map[string]interface{}) string {
	syncHistoryHTML := buildSyncHistoryHTML(data, true)

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">sync manage</h1>

<!-- sync mapping settings -->
<div class="card bg-base-100 shadow-xl mb-6" x-data="syncMappings()">
    <div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">sync mapping</h2>
            <button class="btn btn-sm btn-primary" @click="showAddForm = true">add mapping</button>
        </div>

        <!-- add/edit form -->
        <div x-show="showAddForm" class="bg-base-200 rounded-lg p-4 mb-4" x-cloak>
            <h3 class="font-bold mb-3" x-text="editingId ? 'mapping edit' : 'new add mapping'"></h3>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                    <label class="label"><span class="label-text">name</span></label>
                    <input type="text" x-model="form.name" class="input input-bordered input-sm w-full" placeholder="e.g.: plugins-sync" />
                </div>
                <div>
                    <label class="label"><span class="label-text">exclude pattern (comma separator)</span></label>
                    <input type="text" x-model="form.exclude" class="input input-bordered input-sm w-full" placeholder="e.g.: *.tmp,*.log" />
                </div>
            </div>

            <!-- source settings -->
            <div class="divider text-sm">source (basis server)</div>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div>
                    <label class="label"><span class="label-text">source type</span></label>
                    <select x-model="sourceType" class="select select-bordered select-sm w-full">
                        <option value="agent">agent server</option>
                        <option value="local">master  aslocal folder</option>
                    </select>
                </div>
                <template x-if="sourceType === 'agent'">
                    <div>
                        <label class="label"><span class="label-text">source instance</span></label>
                        <select x-model="form.source_instance_id" @change="onSourceInstanceChange()" class="select select-bordered select-sm w-full">
                            <option value="">instance optional...</option>
                            <template x-for="inst in allInstances" :key="inst.id">
                                <option :value="inst.id" x-text="inst.name + ' (' + inst.node_id.substring(0,8) + ')'"></option>
                            </template>
                        </select>
                    </div>
                </template>
                <template x-if="sourceType === 'agent'">
                    <div>
                        <label class="label"><span class="label-text">source path (instance work_dir basis)</span></label>
                        <div class="flex gap-1">
                            <input type="text" x-model="form.source_path" class="input input-bordered input-sm flex-1" placeholder="e.g.: plugins" />
                            <button type="button" class="btn btn-sm btn-outline" @click="browseSourcePath()" :disabled="!form.source_instance_id">find</button>
                        </div>
                    </div>
                </template>
                <template x-if="sourceType === 'local'">
                    <div>
                        <label class="label"><span class="label-text">master  aslocal path</span></label>
                        <input type="text" x-model="form.src" class="input input-bordered input-sm w-full" placeholder="e.g.: ./deploy/plugins" />
                    </div>
                </template>
                <div class="flex items-end">
                    <label class="label cursor-pointer gap-2">
                        <span class="label-text">enable</span>
                        <input type="checkbox" x-model="form.enabled" class="toggle toggle-success" />
                    </label>
                </div>
            </div>

            <div class="flex gap-2 mt-3">
                <button class="btn btn-sm btn-success" @click="saveMapping()">save</button>
                <button class="btn btn-sm btn-ghost" @click="cancelForm()">cancel</button>
            </div>
        </div>

        <!-- mapping list -->
        <div class="overflow-x-auto">
            <table class="table table-sm table-zebra">
                <thead>
                    <tr><th>name</th><th>source</th><th>target</th><th>state</th><th>manage</th></tr>
                </thead>
                <tbody>
                    <template x-if="mappings.length === 0">
                        <tr><td colspan="5" class="text-center text-gray-500">no registered mapping. "add mapping" clickplease.</td></tr>
                    </template>
                    <template x-for="m in mappings" :key="m.id">
                        <tr>
                            <td class="font-semibold" x-text="m.name"></td>
                            <td>
                                <template x-if="m.source_agent_id">
                                    <code class="text-xs" x-text="m.source_instance_id.substring(0,8) + ':' + m.source_path"></code>
                                </template>
                                <template x-if="!m.source_agent_id">
                                    <code class="text-xs" x-text="m.src + ' ( aslocal)'"></code>
                                </template>
                            </td>
                            <td>
                                <span class="badge badge-sm" x-text="(m.sync_targets || []).length + ' target'"></span>
                            </td>
                            <td>
                                <span class="badge whitespace-nowrap" :class="m.enabled ? 'badge-success' : 'badge-ghost'" x-text="m.enabled ? 'active' : 'inactive'"></span>
                            </td>
                            <td>
                                <!-- desktop: inin button -->
                                <div class="hidden md:flex gap-1 flex-wrap">
                                    <button class="btn btn-xs btn-outline btn-primary whitespace-nowrap" @click="executeSync(m)">sync</button>
                                    <button class="btn btn-xs btn-outline btn-accent whitespace-nowrap" @click="manageTargets(m)">target</button>
                                    <button class="btn btn-xs btn-outline btn-info whitespace-nowrap" @click="browseSrc(m)">source</button>
                                    <button class="btn btn-xs btn-outline btn-warning whitespace-nowrap" @click="editMapping(m)">edit</button>
                                    <button class="btn btn-xs btn-outline btn-error whitespace-nowrap" @click="deleteMapping(m)">delete</button>
                                </div>
                                <!-- mobile: dropdown -->
                                <div class="dropdown dropdown-end md:hidden">
                                    <label tabindex="0" class="btn btn-xs btn-outline">&#8942;</label>
                                    <ul tabindex="0" class="dropdown-content z-[1] menu p-1 shadow-lg bg-base-100 rounded-box w-32">
                                        <li><a @click.prevent="executeSync(m)">sync</a></li>
                                        <li><a @click.prevent="manageTargets(m)">target settings</a></li>
                                        <li><a @click.prevent="browseSrc(m)">source file</a></li>
                                        <li><a @click.prevent="editMapping(m)">edit</a></li>
                                        <li><a class="text-error" @click.prevent="deleteMapping(m)">delete</a></li>
                                    </ul>
                                </div>
                            </td>
                        </tr>
                    </template>
                </tbody>
            </table>
        </div>
    </div>
</div>

<!-- target management panel (when mapping selected) -->
<div class="card bg-base-100 shadow-xl mb-6" x-data="syncTargetManager()" x-show="selectedMapping" x-cloak>
    <div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">target settings: <span class="text-primary" x-text="mappingName"></span></h2>
            <div class="flex gap-2 items-center">
                <span class="text-sm opacity-60" x-text="countChecked() + '/' + countSelectable() + ' optional'"></span>
                <button class="btn btn-xs btn-outline" @click="toggleAll()" x-text="isAllChecked() ? 'all release' : 'all optional'"></button>
                <button class="btn btn-sm btn-success" @click="bulkSave()" :disabled="saving">
                    <span x-show="!saving">save</span>
                    <span x-show="saving" class="loading loading-spinner loading-xs"></span>
                </button>
            </div>
        </div>

        <!-- default target path -->
        <div class="flex items-center gap-3 mb-3 bg-base-200 rounded-lg px-4 py-2">
            <span class="text-sm font-semibold whitespace-nowrap">default path:</span>
            <input type="text" x-model="defaultDestPath" class="input input-bordered input-xs w-48" placeholder="." />
            <button class="btn btn-xs btn-ghost" @click="applyDefaultPath()" title="optional item default path batch apply">batch apply</button>
        </div>

        <!-- instance checkbox  -->
        <template x-if="items.length === 0">
            <div class="text-center text-gray-500 py-4">instance is missing.</div>
        </template>
        <div x-show="items.length > 0" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
            <template x-for="(item, idx) in items" :key="item.id">
                        <div class="flex items-center gap-2 bg-base-200 rounded-lg px-3 py-2"
                             :class="item.isSource ? 'opacity-40' : ''">
                            <input type="checkbox"
                                   class="checkbox checkbox-sm checkbox-primary"
                                   :checked="item.checked"
                                   :disabled="item.isSource"
                                   @change="setCheck(idx, $event.target.checked)" />
                            <div class="flex-1 min-w-0">
                                <div class="text-sm font-semibold truncate" x-text="item.name"></div>
                                <div class="text-xs opacity-50" x-text="item.isSource ? '(source)' : item.nodeName"></div>
                            </div>
                            <div class="flex gap-1 items-center flex-shrink-0">
                                <input type="text"
                                       class="input input-bordered input-xs w-20"
                                       x-model="item.destPath"
                                       :disabled="!item.checked || item.isSource"
                                       @input="_markDirty()"
                                       placeholder="." />
                                <button type="button" class="btn btn-xs btn-ghost px-1" title="folder find"
                                        :disabled="!item.checked || item.isSource"
                                        @click="browseDest(idx)">&#128193;</button>
                            </div>
                        </div>
            </template>
        </div>

        <!-- change guide -->
        <div x-show="dirty" class="alert alert-warning mt-3 py-2" x-cloak>
            <span class="text-sm">change exists. "save" press apply please.</span>
        </div>
    </div>
</div>

<!-- source folder VSCode-style file explorer (when mapping selected display) -->
<div id="sfb-root" x-data="syncFileBrowser()" x-show="selectedMapping" x-cloak class="mb-6">
    <div class="fm-wrap" id="sfb-wrap" style="height:60vh;">
        <div class="fm-drop" id="sfb-drop" style="display:none">
            <div style="text-align:center;color:#007acc">
                <div style="font-size:36px;margin-bottom:8px">&#8681;</div>
                <div style="font-size:16px;font-weight:600">drop files to auto sync</div>
            </div>
        </div>
        <button class="fm-mobile-toggle" onclick="var s=document.getElementById('sfb-side');s.classList.toggle('fm-side-open');">&#9776;</button>
        <div class="fm-side" id="sfb-side">
            <div class="fm-side-hd">
                <span x-text="'source: '+mappingName"></span>
                <div class="fm-acts">
                    <button onclick="SFB.newFile()" title="new file">&#43;</button>
                    <button onclick="SFB.newDir()" title="new folder">&#128193;</button>
                    <label title="upload" style="cursor:pointer">&#8593;<input type="file" style="display:none" onchange="SFB.upload(this.files);this.value=''" multiple></label>
                    <button onclick="SFB.refresh()" title="refresh">&#8635;</button>
                </div>
            </div>
            <div class="fm-tree" id="sfb-tree"></div>
            <div class="fm-upload" id="sfb-upload" style="display:none">
                <div id="sfb-upload-text"></div>
                <progress id="sfb-upload-bar" value="0" max="100"></progress>
            </div>
        </div>
        <div class="fm-resize" id="sfb-resize"></div>
        <div class="fm-main">
            <div class="fm-tabs" id="sfb-tabs"></div>
            <div class="fm-editor" id="sfb-editor">
                <div class="fm-welcome" id="sfb-welcome">
                    <svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
                    <div style="font-size:13px">file optional editplease</div>
                    <div style="font-size:11px;opacity:.5">save when automatically agent sync</div>
                </div>
            </div>
            <div class="fm-bar" id="sfb-bar">
                <span id="sfb-bar-path">file none</span>
                <span style="display:flex;align-items:center;gap:6px;">
                    <button id="sfb-mobile-save" style="display:none;background:#0e7;border:none;color:#000;padding:1px 10px;border-radius:3px;font-size:11px;cursor:pointer;font-weight:600" onclick="SFB.save()">save</button>
                    <button id="sfb-mobile-del" style="display:none;background:#f55;border:none;color:#fff;padding:1px 10px;border-radius:3px;font-size:11px;cursor:pointer" onclick="SFB.deleteActive()">delete</button>
                    <span id="sfb-bar-lang"></span> &nbsp; <span id="sfb-bar-pos"></span>
                </span>
            </div>
        </div>
    </div>
    <div class="fm-ctx" id="sfb-ctx" style="display:none"></div>
</div>

<!-- sync history -->
<div class="card bg-base-100 shadow-xl">
    <div class="card-body">
        <h2 class="card-title">recent sync history</h2>
        <div id="sync-history" hx-get="/htmx/sync-history" hx-trigger="every 10s" hx-swap="innerHTML">
            %s
        </div>
    </div>
</div>

<!-- folder picker modal -->
<dialog id="folder-picker-modal" class="modal">
    <div class="modal-box w-full max-w-md">
        <h3 class="font-bold text-lg mb-3" id="fp-title">folder optional</h3>
        <div id="fp-current" class="text-xs opacity-60 mb-2 font-mono">.</div>
        <div id="fp-tree" class="bg-base-200 rounded-lg p-2 max-h-64 overflow-y-auto min-h-32" style="font-size:13px"></div>
        <div id="fp-loading" class="text-center py-4" style="display:none"><span class="loading loading-spinner loading-sm"></span></div>
        <div id="fp-error" class="text-error text-sm py-2" style="display:none"></div>
        <div class="modal-action">
            <button class="btn btn-sm btn-ghost" onclick="FolderPicker.close()">cancel</button>
            <button class="btn btn-sm" onclick="FolderPicker.up()">parent folder</button>
            <button class="btn btn-sm btn-primary" onclick="FolderPicker.select()">optional</button>
        </div>
    </div>
    <form method="dialog" class="modal-backdrop"><button>close</button></form>
</dialog>

<script>
// ── folder picker (common) ──
window.FolderPicker = (function() {
    var _instanceId = '';
    var _currentPath = '.';
    var _callback = null;
    var _entries = [];

    function modal() { return document.getElementById('folder-picker-modal'); }
    function treeEl() { return document.getElementById('fp-tree'); }

    async function loadDir(path) {
        var tree = treeEl();
        var loading = document.getElementById('fp-loading');
        var errEl = document.getElementById('fp-error');
        tree.innerHTML = '';
        loading.style.display = '';
        errEl.style.display = 'none';
        try {
            var resp = await fetch('/api/instances/' + _instanceId + '/files?path=' + encodeURIComponent(path || ''));
            var data = await resp.json();
            _entries = (data.entries || []).filter(function(e) { return e.is_dir; });
            _entries.sort(function(a,b) { return a.name.localeCompare(b.name); });
            _currentPath = path || '.';
            document.getElementById('fp-current').textContent = _currentPath || '.';
            loading.style.display = 'none';
            renderEntries();
        } catch(err) {
            loading.style.display = 'none';
            errEl.textContent = 'folder list load cannot: ' + err.message;
            errEl.style.display = '';
        }
    }

    function renderEntries() {
        var tree = treeEl();
        tree.innerHTML = '';
        if (_entries.length === 0) {
            tree.innerHTML = '<div class="text-center opacity-50 py-4">sub folder none</div>';
            return;
        }
        for (var i = 0; i < _entries.length; i++) {
            var e = _entries[i];
            var row = document.createElement('div');
            row.style.cssText = 'padding:6px 8px;cursor:pointer;border-radius:6px;display:flex;align-items:center;gap:8px';
            row.onmouseenter = function() { this.style.background = 'rgba(255,255,255,0.1)'; };
            row.onmouseleave = function() { this.style.background = ''; };
            row.innerHTML = '<span style="font-size:16px">\uD83D\uDCC1</span><span>' + e.name + '</span>';
            (function(entry) {
                row.addEventListener('dblclick', function() { loadDir(entry.path); });
                row.addEventListener('click', function() {
                    // current optional display
                    var rows = treeEl().children;
                    for (var j = 0; j < rows.length; j++) rows[j].style.background = '';
                    this.style.background = 'rgba(0,122,204,0.3)';
                    _currentPath = entry.path;
                    document.getElementById('fp-current').textContent = _currentPath;
                });
            })(e);
            tree.appendChild(row);
        }
    }

    return {
        open: function(instanceId, currentValue, callback) {
            _instanceId = instanceId;
            _callback = callback;
            _currentPath = currentValue || '.';
            document.getElementById('fp-current').textContent = _currentPath;
            modal().showModal();
            loadDir(currentValue || '');
        },
        close: function() {
            modal().close();
        },
        up: function() {
            if (!_currentPath || _currentPath === '.' || _currentPath === '') {
                return;
            }
            var parts = _currentPath.replace(/\\/g, '/').split('/');
            parts.pop();
            var parent = parts.join('/') || '.';
            loadDir(parent === '.' ? '' : parent);
        },
        select: function() {
            if (_callback) _callback(_currentPath || '.');
            modal().close();
        }
    };
})();

function syncMappings() {
    return {
        mappings: [],
        allInstances: [],
        showAddForm: false,
        editingId: null,
        sourceType: 'agent',
        form: { name:'', src:'', dest:'.', targets:'*', exclude:'', enabled:true,
                source_agent_id:'', source_instance_id:'', source_path:'' },

        init() {
            this.loadMappings();
            this.loadInstances();
            var self = this;
            window.addEventListener('refresh-sync-mappings', function() { self.loadMappings(); });
        },

        async loadMappings() {
            try {
                const resp = await fetch('/api/sync/mappings');
                this.mappings = (await resp.json()) || [];
            } catch(e) { showToast('failed to load mapping', 'error'); }
        },

        async loadInstances() {
            try {
                const resp = await fetch('/api/instances');
                this.allInstances = (await resp.json()) || [];
                // beforereverse save (target map)
                window._allInstances = this.allInstances;
            } catch(e) {}
        },

        onSourceInstanceChange() {
            const inst = this.allInstances.find(i => i.id === this.form.source_instance_id);
            if (inst) {
                this.form.source_agent_id = inst.node_id;
            }
        },

        editMapping(m) {
            this.editingId = m.id;
            this.sourceType = m.source_agent_id ? 'agent' : 'local';
            this.form = {
                name: m.name, src: m.src, dest: m.dest, targets: m.targets,
                exclude: m.exclude, enabled: m.enabled,
                source_agent_id: m.source_agent_id || '',
                source_instance_id: m.source_instance_id || '',
                source_path: m.source_path || ''
            };
            this.showAddForm = true;
        },

        cancelForm() {
            this.showAddForm = false;
            this.editingId = null;
            this.sourceType = 'agent';
            this.form = { name:'', src:'', dest:'.', targets:'*', exclude:'', enabled:true,
                          source_agent_id:'', source_instance_id:'', source_path:'' };
        },

        async saveMapping() {
            // source type  per field cleanup
            const body = { ...this.form };
            if (this.sourceType === 'local') {
                body.source_agent_id = '';
                body.source_instance_id = '';
                body.source_path = '';
            } else {
                // agent source: src auto create (server from)
                body.src = '';
            }

            const url = this.editingId ? '/api/sync/mappings/' + this.editingId : '/api/sync/mappings';
            const method = this.editingId ? 'PUT' : 'POST';
            try {
                const resp = await fetch(url, { method, headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body) });
                if (resp.ok) {
                    showToast('mapping saved', 'success');
                    this.cancelForm();
                    this.loadMappings();
                } else {
                    const data = await resp.json();
                    showToast(data.error || 'save failed', 'error');
                }
            } catch(e) { showToast('save failed: ' + e.message, 'error'); }
        },

        async deleteMapping(m) {
            if (!confirm('"' + m.name + '" mapping delete?')) return;
            try {
                await fetch('/api/sync/mappings/' + m.id, { method: 'DELETE' });
                showToast('mapping deleted', 'success');
                this.loadMappings();
            } catch(e) { showToast('delete failed', 'error'); }
        },

        async executeSync(m) {
            if (!confirm('"' + m.name + '" sync execute?')) return;
            try {
                const resp = await fetch('/api/sync/mappings/' + m.id + '/execute', { method: 'POST' });
                const data = await resp.json();
                if (data.status === 'accepted') { showToast(data.message, 'success'); }
                else { showToast(data.error || data.message, 'error'); }
            } catch(e) { showToast('sync execute failed', 'error'); }
        },

        manageTargets(m) {
            window.dispatchEvent(new CustomEvent('manage-sync-targets', { detail: { id: m.id, name: m.name, source_instance_id: m.source_instance_id || '' } }));
        },

        browseSrc(m) {
            window.dispatchEvent(new CustomEvent('browse-sync-src', { detail: { id: m.id, name: m.name } }));
        },

        browseSourcePath() {
            if (!this.form.source_instance_id) return;
            var self = this;
            FolderPicker.open(this.form.source_instance_id, this.form.source_path || '', function(path) {
                self.form.source_path = path === '.' ? '' : path;
            });
        }
    };
}

function syncTargetManager() {
    return {
        selectedMapping: null,
        mappingName: '',
        sourceInstanceId: '',
        allInstances: [],
        existingTargets: [],
        // xcol based state: [{id, name, node_id, instance_type, checked, destPath, isSource}]
        items: [],
        defaultDestPath: '.',
        saving: false,
        dirty: false,
        _origSnapshot: '',
        _nodeNames: {},

        init() {
            const self = this;
            window.addEventListener('manage-sync-targets', function(ev) {
                self.selectedMapping = ev.detail.id;
                self.mappingName = ev.detail.name;
                self.sourceInstanceId = ev.detail.source_instance_id || '';
                self.allInstances = window._allInstances || [];
                self._loadNodesAndTargets();
            });
        },

        async _loadNodesAndTargets() {
            // Load node names
            try {
                const nr = await fetch('/api/nodes');
                const nodes = await nr.json();
                this._nodeNames = {};
                for (const n of nodes) this._nodeNames[n.id] = n.name;
            } catch(err) { this._nodeNames = {}; }
            await this.loadTargets();
        },

        async loadTargets() {
            try {
                const resp = await fetch('/api/sync/mappings/' + this.selectedMapping + '/targets');
                this.existingTargets = (await resp.json()) || [];
            } catch(err) { showToast('failed to load target list', 'error'); return; }

            // Build items array (sort by node name)
            const nn = this._nodeNames || {};
            const arr = [];
            for (const inst of this.allInstances) {
                const existing = this.existingTargets.find(t => t.instance_id === inst.id);
                const isSrc = inst.id === this.sourceInstanceId;
                const nid = inst.node_id || '';
                arr.push({
                    id: inst.id,
                    name: inst.name,
                    node_id: nid,
                    nodeName: nn[nid] || nid.substring(0,8),
                    instance_type: inst.instance_type || '',
                    checked: isSrc ? false : !!existing,
                    destPath: existing ? existing.dest_path : '.',
                    isSource: isSrc
                });
            }
            // sort per-node -> by name
            arr.sort((a,b) => a.nodeName.localeCompare(b.nodeName) || a.name.localeCompare(b.name));
            this.items = arr;
            this._origSnapshot = JSON.stringify(arr.map(i => ({c:i.checked,d:i.destPath})));
            this.dirty = false;
        },

        countChecked() {
            return this.items.filter(i => !i.isSource && i.checked).length;
        },

        countSelectable() {
            return this.items.filter(i => !i.isSource).length;
        },

        isAllChecked() {
            const sel = this.items.filter(i => !i.isSource);
            return sel.length > 0 && sel.every(i => i.checked);
        },

        setCheck(idx, val) {
            if (this.items[idx].isSource) return;
            this.items[idx].checked = val;
            this._markDirty();
        },

        toggleAll() {
            const newVal = !this.isAllChecked();
            for (let i = 0; i < this.items.length; i++) {
                if (!this.items[i].isSource) this.items[i].checked = newVal;
            }
            this._markDirty();
        },

        applyDefaultPath() {
            for (let i = 0; i < this.items.length; i++) {
                if (!this.items[i].isSource && this.items[i].checked) {
                    this.items[i].destPath = this.defaultDestPath || '.';
                }
            }
            this._markDirty();
        },

        _markDirty() {
            const snap = JSON.stringify(this.items.map(i => ({c:i.checked,d:i.destPath})));
            this.dirty = snap !== this._origSnapshot;
        },

        async bulkSave() {
            this.saving = true;
            const targets = [];
            for (const it of this.items) {
                if (it.checked && !it.isSource) {
                    targets.push({
                        agent_id: it.node_id,
                        instance_id: it.id,
                        dest_path: it.destPath || '.',
                        enabled: true
                    });
                }
            }
            try {
                const resp = await fetch('/api/sync/mappings/' + this.selectedMapping + '/targets/bulk', {
                    method: 'PUT',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({ targets })
                });
                if (resp.ok) {
                    const rdata = await resp.json();
                    showToast(rdata.count + ' target saved', 'success');
                    this._origSnapshot = JSON.stringify(this.items.map(i => ({c:i.checked,d:i.destPath})));
                    this.dirty = false;
                    // Refresh mapping list to update target count
                    window.dispatchEvent(new CustomEvent('refresh-sync-mappings'));
                } else {
                    const edata = await resp.json();
                    showToast(edata.error || 'save failed', 'error');
                }
            } catch(err) { showToast('save failed: ' + err.message, 'error'); }
            this.saving = false;
        },

        browseDest(idx) {
            var item = this.items[idx];
            if (!item || item.isSource || !item.checked) return;
            var self = this;
            FolderPicker.open(item.id, item.destPath || '', function(path) {
                self.items[idx].destPath = path === '.' ? '.' : path;
                self._markDirty();
            });
        }
    };
}

function syncFileBrowser() {
    return {
        selectedMapping: null,
        mappingName: '',
        init() {
            window.addEventListener('browse-sync-src', (e) => {
                this.selectedMapping = e.detail.id;
                this.mappingName = e.detail.name;
                SFB.open(e.detail.id, e.detail.name);
            });
        }
    };
}

// ── sync source file VSCode-style data (pure JS) ──
(function(){
const ss = {
    mid: null, tree: [], sel: '', tabs: [], active: '', editor: null, monaco: null, sideW: 260, inited: false
};

function isBin(n){
    const x=(n||'').split('.').pop().toLowerCase();
    return ['jar','zip','tar','gz','7z','rar','png','jpg','jpeg','gif','bmp','ico','webp','mp3','mp4','wav','avi','mkv','exe','dll','so','class','dat','db','sqlite','nbt','mca','mcr'].includes(x);
}
function fIcon(n){
    const x=(n||'').split('.').pop().toLowerCase();
    const m={java:'\u2615',jar:'\uD83D\uDCE6',yml:'\u2699',yaml:'\u2699',json:'{}',xml:'\uD83D\uDCC4',properties:'\u2699',js:'JS',ts:'TS',py:'\uD83D\uDC0D',sh:'>_',toml:'\u2699',ini:'\u2699',cfg:'\u2699',conf:'\u2699'};
    return m[x]||'\uD83D\uDCC4';
}
function sort(arr){return arr.sort((a,b)=>{if(a.is_dir!==b.is_dir)return a.is_dir?-1:1;return a.name.localeCompare(b.name)})}

async function apiList(path){const r=await fetch('/api/sync/files?mapping_id='+ss.mid+'&path='+encodeURIComponent(path||''));const d=await r.json();return sort(d.entries||[])}
async function apiRead(path){const r=await fetch('/api/sync/files/read?mapping_id='+ss.mid+'&path='+encodeURIComponent(path));return await r.json()}
async function apiWrite(path,content,createDirs){const r=await fetch('/api/sync/files?mapping_id='+ss.mid,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({path,content:btoa(unescape(encodeURIComponent(content))),mapping_id:ss.mid,create_dirs:!!createDirs})});return await r.json()}
async function apiDel(path,recursive){const r=await fetch('/api/sync/files?mapping_id='+ss.mid,{method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({path,recursive})});return await r.json()}
async function apiMkdir(path){const r=await fetch('/api/sync/files/mkdir',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path,mapping_id:ss.mid})});return await r.json()}
async function apiUpload(file,dir){const fd=new FormData();fd.append('file',file);fd.append('mapping_id',ss.mid);fd.append('path',dir||'');const r=await fetch('/api/sync/files/upload',{method:'POST',body:fd});return await r.json()}

function renderTree(){
    const el=document.getElementById('sfb-tree');if(!el)return;el.innerHTML='';
    renderNodes(el,ss.tree,0);
}
function renderNodes(parent,nodes,depth){
    for(const n of nodes){
        const row=document.createElement('div');row.className='fm-ti'+(n.path===ss.sel?' sel':'');row.style.paddingLeft=(depth*16+8)+'px';
        const chev=document.createElement('span');chev.className='fm-chev'+(n.is_dir&&n.expanded?' open':'');chev.innerHTML=n.is_dir?'\u25B6':'';row.appendChild(chev);
        const ico=document.createElement('span');ico.className='fm-ico';ico.textContent=n.is_dir?(n.expanded?'\uD83D\uDCC2':'\uD83D\uDCC1'):fIcon(n.name);row.appendChild(ico);
        const nm=document.createElement('span');nm.className='fm-nm';nm.textContent=n.name;row.appendChild(nm);
        if(window.__isMobile){const act=document.createElement('span');act.textContent='\u22EE';act.style.cssText='margin-left:auto;padding:0 6px;font-size:16px;opacity:.5;flex-shrink:0';act.addEventListener('click',function(e){e.stopPropagation();const r=act.getBoundingClientRect();onTreeCtx({clientX:r.left,clientY:r.bottom,preventDefault:()=>{},stopPropagation:()=>{}},n)});row.appendChild(act)}
        row.addEventListener('click',function(e){e.stopPropagation();onTreeClick(n)});
        row.addEventListener('contextmenu',function(e){e.preventDefault();e.stopPropagation();onTreeCtx(e,n)});
        parent.appendChild(row);
        if(n.is_dir&&n.expanded&&n.children.length>0)renderNodes(parent,n.children,depth+1);
    }
}
async function onTreeClick(node){
    ss.sel=node.path;
    if(node.is_dir){if(!node.loaded){node.children=(await apiList(node.path)).map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));node.loaded=true}node.expanded=!node.expanded;renderTree()}
    else{openFile(node.path,node.name)}
}
function onTreeCtx(event,node){
    const items=[];
    if(node.is_dir){items.push({label:'new file',action:()=>{ss.sel=node.path;SFB.newFile()}});items.push({label:'new folder',action:()=>{ss.sel=node.path;SFB.newDir()}});items.push({sep:true})}
    else{items.push({label:'open',action:()=>openFile(node.path,node.name)});items.push({sep:true})}
    items.push({label:'delete',cls:'red',action:()=>deleteItem(node.path,node.is_dir)});
    showCtx(event.clientX,event.clientY,items);
}
function findNode(nodes,path){for(const n of nodes){if(n.path===path)return n;if(n.is_dir&&n.children.length){const f=findNode(n.children,path);if(f)return f}}return null}
function getSelDir(){if(!ss.sel)return '';const n=findNode(ss.tree,ss.sel);if(n&&n.is_dir)return n.path;const p=ss.sel.split('/');p.pop();return p.join('/')}

function renderTabs(){
    const el=document.getElementById('sfb-tabs');if(!el)return;el.innerHTML='';
    for(const t of ss.tabs){
        const tb=document.createElement('div');tb.className='fm-tb'+(t.path===ss.active?' act':'');
        if(t.modified){const dot=document.createElement('span');dot.className='dot';tb.appendChild(dot)}
        const nm=document.createElement('span');nm.textContent=t.name;nm.style.maxWidth='160px';nm.style.overflow='hidden';nm.style.textOverflow='ellipsis';tb.appendChild(nm);
        const x=document.createElement('span');x.className='x';x.innerHTML='&times;';x.addEventListener('click',function(e){e.stopPropagation();closeTab(t.path)});tb.appendChild(x);
        tb.addEventListener('click',function(){switchTab(t.path)});tb.addEventListener('auxclick',function(e){if(e.button===1){e.preventDefault();closeTab(t.path)}});
        el.appendChild(tb);
    }
}
async function openFile(path,name){
    if(isBin(name)){showToast('binary file editcannot','warning');return}
    if(ss.tabs.find(t=>t.path===path)){switchTab(path);return}
    try{
        const data=await apiRead(path);if(data.error){showToast(data.error,'error');return}
        const content=b64DecodeUTF8(data.content);const lang=extToLang(name);
        let model=null;
        if(ss.monaco&&!window.__isMobile){model=ss.monaco.editor.createModel(content,lang)}
        ss.tabs.push({path,name,lang,model,origContent:content,modified:false});
        switchTab(path);
    }catch(e){showToast('file read failed: '+e.message,'error')}
}
function switchTab(path){
    ss.active=path;ss.sel=path;
    const tab=ss.tabs.find(t=>t.path===path);
    if(!tab)return;
    document.getElementById('sfb-welcome').style.display='none';
    document.getElementById('sfb-bar-path').textContent=tab.path;
    document.getElementById('sfb-bar-lang').textContent=tab.lang;
    const saveBtn=document.getElementById('sfb-mobile-save');
    const delBtn=document.getElementById('sfb-mobile-del');
    if(saveBtn)saveBtn.style.display=window.__isMobile?'':'none';
    if(delBtn)delBtn.style.display=window.__isMobile?'':'none';
    if(window.__isMobile){
        if(ss.editor){ss.editor.getContainerDomNode().style.display='none'}
        let ta=document.getElementById('sfb-mobile-ta');
        if(!ta){ta=document.createElement('textarea');ta.id='sfb-mobile-ta';ta.style.cssText='width:100%%;height:100%%;background:#1e1e1e;color:#d4d4d4;border:none;padding:12px;font-family:D2Coding,monospace;font-size:14px;resize:none;outline:none;box-sizing:border-box;tab-size:4;-moz-tab-size:4';ta.spellcheck=false;document.getElementById('sfb-editor').appendChild(ta)}
        ta.style.display='';
        ta.value=tab.model?tab.model.getValue():tab.origContent;
        ta.oninput=function(){const mod=ta.value!==tab.origContent;if(tab.modified!==mod){tab.modified=mod;renderTabs()}if(tab.model)tab.model.setValue(ta.value)};
    } else if(tab.model&&ss.editor){
        let ta=document.getElementById('sfb-mobile-ta');if(ta)ta.style.display='none';
        ss.editor.getContainerDomNode().style.display='';
        ss.editor.setModel(tab.model);ss.editor.focus();
    }
    renderTabs();renderTree();
}
function closeTab(path){
    const tab=ss.tabs.find(t=>t.path===path);if(!tab)return;
    if(tab.modified&&!confirm('"'+tab.name+'" close without saving?'))return;
    if(tab.model)tab.model.dispose();const idx=ss.tabs.indexOf(tab);ss.tabs.splice(idx,1);
    if(ss.active===path){
        if(ss.tabs.length>0){switchTab(ss.tabs[Math.min(idx,ss.tabs.length-1)].path)}
        else{ss.active='';document.getElementById('sfb-welcome').style.display='';if(ss.editor)ss.editor.getContainerDomNode().style.display='none';const ta=document.getElementById('sfb-mobile-ta');if(ta)ta.style.display='none';const sb=document.getElementById('sfb-mobile-save');if(sb)sb.style.display='none';const db=document.getElementById('sfb-mobile-del');if(db)db.style.display='none';document.getElementById('sfb-bar-path').textContent='file none';document.getElementById('sfb-bar-lang').textContent='';document.getElementById('sfb-bar-pos').textContent=''}
    }
    renderTabs();
}
async function saveActive(){
    const tab=ss.tabs.find(t=>t.path===ss.active);if(!tab)return;
    let content;
    if(window.__isMobile){const ta=document.getElementById('sfb-mobile-ta');if(!ta)return;content=ta.value}
    else{if(!tab.model)return;content=tab.model.getValue()}
    try{const data=await apiWrite(tab.path,content,false);if(data.status==='ok'){showToast('save complete — auto sync','success');tab.origContent=content;tab.modified=false;renderTabs()}else{showToast(data.error||'save failed','error')}}catch(e){showToast('save failed','error')}
}
async function deleteItem(path,isDir){
    if(!confirm((isDir?'folder':'file')+' "'+path.split('/').pop()+'"() delete?'))return;
    try{const d=await apiDel(path,isDir);if(d.status==='ok'||d.success){showToast('deleted','success');if(!isDir){const t=ss.tabs.find(t=>t.path===path);if(t){t.modified=false;closeTab(path)}}else{ss.tabs.filter(t=>t.path.startsWith(path+'/')).forEach(t=>{t.modified=false;closeTab(t.path)})}await SFB.refresh()}else showToast(d.error||'failed','error')}catch(e){showToast('delete failed','error')}
}
function showCtx(x,y,items){
    const el=document.getElementById('sfb-ctx');el.innerHTML='';
    for(const it of items){if(it.sep){const s=document.createElement('div');s.className='fm-ctx-sep';el.appendChild(s);continue}const d=document.createElement('div');d.className='fm-ctx-i'+(it.cls?' '+it.cls:'');d.textContent=it.label;d.addEventListener('click',function(){el.style.display='none';it.action()});el.appendChild(d)}
    el.style.left=x+'px';el.style.top=y+'px';el.style.display='';
}
document.addEventListener('click',()=>{const el=document.getElementById('sfb-ctx');if(el)el.style.display='none'});

// resize
(function(){
    const handle=document.getElementById('sfb-resize');if(!handle)return;
    handle.addEventListener('mousedown',function(e){
        const startX=e.clientX,startW=ss.sideW;
        function mv(e){ss.sideW=Math.max(150,Math.min(600,startW+(e.clientX-startX)));document.getElementById('sfb-side').style.width=ss.sideW+'px'}
        function up(){document.removeEventListener('mousemove',mv);document.removeEventListener('mouseup',up)}
        document.addEventListener('mousemove',mv);document.addEventListener('mouseup',up);
    });
})();
// drag-drop
(function(){
    const wrap=document.getElementById('sfb-wrap');const drop=document.getElementById('sfb-drop');
    if(!wrap||!drop)return;let cnt=0;
    wrap.addEventListener('dragenter',function(e){e.preventDefault();cnt++;drop.style.display=''});
    wrap.addEventListener('dragleave',function(e){e.preventDefault();cnt--;if(cnt<=0){cnt=0;drop.style.display='none'}});
    wrap.addEventListener('dragover',function(e){e.preventDefault()});
    wrap.addEventListener('drop',function(e){e.preventDefault();cnt=0;drop.style.display='none';const files=[];if(e.dataTransfer.items){for(let i=0;i<e.dataTransfer.items.length;i++){if(e.dataTransfer.items[i].kind==='file')files.push(e.dataTransfer.items[i].getAsFile())}}if(files.length)SFB.upload(files)});
})();
// keyboard
document.addEventListener('keydown',function(e){
    if(!ss.mid)return;
    if((e.ctrlKey||e.metaKey)&&e.key==='s'){if(ss.active){e.preventDefault();saveActive()}}
});

async function initEditor(){
    if(ss.inited)return;ss.inited=true;
    if(window.__isMobile){ss.monaco=null;ss.editor=null;return}
    ss.monaco=await window.__monacoReady;
    const container=document.getElementById('sfb-editor');if(!container)return;
    ss.editor=ss.monaco.editor.create(container,{
        value:'',language:'plaintext',theme:'vs-dark',fontSize:14,
        fontFamily:"'D2Coding','Malgun Gothic','Microsoft YaHei','Yu Gothic',monospace",
        minimap:{enabled:true},wordWrap:'on',automaticLayout:true,
        scrollBeyondLastLine:false,renderWhitespace:'selection',tabSize:4,padding:{top:8,bottom:8},
    });
    ss.editor.getContainerDomNode().style.display='none';
    ss.editor.addCommand(ss.monaco.KeyMod.CtrlCmd|ss.monaco.KeyCode.KeyS,()=>saveActive());
    ss.editor.onDidChangeCursorPosition(e=>{document.getElementById('sfb-bar-pos').textContent='line '+e.position.lineNumber+', col '+e.position.column});
    ss.editor.onDidChangeModelContent(()=>{const tab=ss.tabs.find(t=>t.path===ss.active);if(tab&&tab.model){const mod=tab.model.getValue()!==tab.origContent;if(tab.modified!==mod){tab.modified=mod;renderTabs()}}});
}

function collectExpanded(nodes,out){for(const n of nodes){if(n.is_dir&&n.expanded)out.push(n.path);if(n.is_dir&&n.children.length>0)collectExpanded(n.children,out)}}
async function reExpand(nodes,expandedPaths){
    for(const n of nodes){if(n.is_dir&&expandedPaths.some(p=>p===n.path||p.startsWith(n.path+'/'))){n.children=(await apiList(n.path)).map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));n.loaded=true;n.expanded=true;await reExpand(n.children,expandedPaths)}}
}

window.SFB = {
    save(){saveActive()},
    deleteActive(){const tab=ss.tabs.find(t=>t.path===ss.active);if(tab)deleteItem(tab.path,false)},
    async open(mappingId, name){
        ss.mid=mappingId;
        await initEditor();
        const entries=await apiList('');
        ss.tree=entries.map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));
        renderTree();
    },
    async refresh(){
        if(!ss.mid)return;
        const expanded=[];collectExpanded(ss.tree,expanded);
        ss.tabs.forEach(t=>{const p=t.path.split('/');p.pop();while(p.length>0){expanded.push(p.join('/'));p.pop()}});
        const entries=await apiList('');
        ss.tree=entries.map(e=>({name:e.name,path:e.path,is_dir:e.is_dir,expanded:false,loaded:false,children:[]}));
        await reExpand(ss.tree,expanded);renderTree();
    },
    async newFile(){
        const name=prompt('new file name:');if(!name)return;const dir=getSelDir();const path=dir?dir+'/'+name:name;
        try{const d=await apiWrite(path,'',true);if(d.status==='ok'){showToast('create file','success');await SFB.refresh();openFile(path,name)}else showToast(d.error||'failed','error')}catch(e){showToast('failed','error')}
    },
    async newDir(){
        const name=prompt('new folder name:');if(!name)return;const dir=getSelDir();const path=dir?dir+'/'+name:name;
        try{const d=await apiMkdir(path);if(d.status==='ok'){showToast('folder created','success');await SFB.refresh()}else showToast(d.error||'failed','error')}catch(e){showToast('failed','error')}
    },
    async upload(fileList){
        const upEl=document.getElementById('sfb-upload'),txt=document.getElementById('sfb-upload-text'),bar=document.getElementById('sfb-upload-bar');
        if(upEl)upEl.style.display='';const dir=getSelDir();let done=0;
        for(let i=0;i<fileList.length;i++){if(txt)txt.textContent=fileList[i].name+' ('+(i+1)+'/'+fileList.length+')';if(bar)bar.value=Math.round(i/fileList.length*100);try{await apiUpload(fileList[i],dir);done++}catch(e){showToast(fileList[i].name+' failed','error')}}
        if(bar)bar.value=100;setTimeout(()=>{if(upEl)upEl.style.display='none'},1000);
        showToast(done+' upload complete — auto sync','success');await SFB.refresh();
    }
};
})();
</script>`, syncHistoryHTML)
}
