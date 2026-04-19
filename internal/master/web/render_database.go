package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

func buildDatabaseBrowserHTML(data map[string]interface{}) string {
	instI, _ := data["Instance"]
	inst, _ := instI.(*store.Instance)
	if inst == nil {
		return `<div class="alert alert-error">instance not found</div>`
	}

	typeBadge := instanceTypeBadge(inst.InstanceType)

	// default query and hint per type
	var defaultQuery, placeholder, listDbQuery, listTablesQuery string
	switch inst.InstanceType {
	case "mysql":
		defaultQuery = "SHOW DATABASES;"
		placeholder = "SQL query please enter (e.g.: SELECT * FROM users LIMIT 10;)"
		listDbQuery = "SHOW DATABASES;"
		listTablesQuery = "SHOW TABLES;"
	case "postgresql":
		defaultQuery = `\\list`
		placeholder = "SQL query please enter (e.g.: SELECT * FROM users LIMIT 10;)"
		listDbQuery = `\\list`
		listTablesQuery = `\\dt`
	case "mongodb":
		defaultQuery = "show dbs"
		placeholder = "MongoDB command please enter (e.g.: db.users.find().limit(10))"
		listDbQuery = "show dbs"
		listTablesQuery = "show collections"
	case "redis":
		defaultQuery = "INFO keyspace"
		placeholder = "Redis command please enter (e.g.: KEYS *, GET mykey)"
		listDbQuery = "INFO keyspace"
		listTablesQuery = "KEYS *"
	}

	return fmt.Sprintf(`<div class="flex items-center gap-2 sm:gap-3 mb-4 flex-wrap">
    <a href="/instances/%s" class="btn btn-ghost btn-sm">&larr;</a>
    <h1 class="text-lg sm:text-2xl font-bold">database: %s</h1>
    %s
</div>

<style>
#db-browser-app .db-top{flex:1;display:flex;gap:0;min-height:0;border:1px solid hsl(var(--bc)/0.2);border-radius:8px;overflow:hidden}
#db-sidebar{width:260px;min-width:200px;border-right:1px solid hsl(var(--bc)/0.2);display:flex;flex-direction:column;background:hsl(var(--b2))}
.db-mobile-toggle{display:none;background:hsl(var(--b2));border:1px solid hsl(var(--bc)/0.2);color:hsl(var(--bc));border-radius:4px;padding:4px 10px;font-size:16px;cursor:pointer}
@media(max-width:767px){
  #db-browser-app{height:auto !important;min-height:calc(100vh - 160px)}
  #db-browser-app .db-top{flex-direction:column}
  #db-sidebar{width:100%% !important;min-width:0 !important;max-height:0;overflow:hidden;border-right:none;border-bottom:1px solid hsl(var(--bc)/0.2);transition:max-height .2s}
  #db-sidebar.db-side-open{max-height:40vh}
  .db-mobile-toggle{display:inline-flex}
  #db-editor{height:140px !important}
}
</style>

<div id="db-browser-app" style="height: calc(100vh - 180px); display: flex; flex-direction: column; gap: 0;">
    <!-- top: sidebar + result area -->
    <div class="db-top">
        <!-- left sidebar: DB/table tree -->
        <div id="db-sidebar">
            <div style="padding: 8px 12px; border-bottom: 1px solid hsl(var(--bc) / 0.2); display: flex; align-items: center; justify-content: space-between;">
                <span class="text-sm font-bold">navigate</span>
                <button class="btn btn-ghost btn-xs" onclick="dbBrowser.refreshTree()" title="refresh">&#x21bb;</button>
            </div>
            <div id="db-tree" style="flex: 1; overflow-y: auto; padding: 4px 0; font-size: 13px; font-family: 'D2Coding', monospace;"></div>
        </div>

        <!-- right: result display area -->
        <div style="flex: 1; display: flex; flex-direction: column; min-width: 0;">
            <!-- tab bar -->
            <div id="db-tabs" style="display: flex; align-items: center; gap: 0; border-bottom: 1px solid hsl(var(--bc) / 0.2); background: hsl(var(--b2)); min-height: 36px; overflow-x: auto;">
                <button class="db-mobile-toggle" onclick="document.getElementById('db-sidebar').classList.toggle('db-side-open')" title="toggle navigation">&#9776;</button>
                <div style="padding: 0 12px; font-size: 12px; color: hsl(var(--bc) / 0.5);">result here display</div>
            </div>
            <!-- result table -->
            <div id="db-result" style="flex: 1; overflow: auto; position: relative;">
                <div style="display: flex; align-items: center; justify-content: center; height: 100%%; color: hsl(var(--bc) / 0.4); font-size: 14px;">
                    query executeif done result here display
                </div>
            </div>
        </div>
    </div>

    <!-- bottom: query area -->
    <div style="border: 1px solid hsl(var(--bc) / 0.2); border-radius: 8px; overflow: hidden; margin-top: 8px;">
        <div style="display: flex; align-items: center; justify-content: space-between; padding: 6px 12px; background: hsl(var(--b2)); border-bottom: 1px solid hsl(var(--bc) / 0.2);">
            <span class="text-sm font-bold">query data</span>
            <div style="display: flex; gap: 6px; align-items: center;">
                <span id="db-status" class="text-xs" style="color: hsl(var(--bc) / 0.5);">ready</span>
                <button class="btn btn-primary btn-sm" onclick="dbBrowser.executeQuery()" title="execute (Ctrl+Enter)">
                    &#9654; execute
                </button>
            </div>
        </div>
        <div id="db-editor" style="height: 180px;"></div>
    </div>
</div>

<script>
(function() {
    const instanceId = '%s';
    const instanceType = '%s';
    const defaultQuery = %s;
    const listDbQuery = %s;
    const listTablesQuery = %s;

    let editor = null;
    let queryHistory = [];
    let currentDb = '';

    // Monaco language mapping
    const langMap = {
        'mysql': 'sql',
        'postgresql': 'sql',
        'mongodb': 'javascript',
        'redis': 'plaintext'
    };

    const app = {
        init: function() {
            this.initEditor();
            this.refreshTree();
        },

        initEditor: function() {
            const container = document.getElementById('db-editor');
            if (!container) return;

            if (window.__isMobile) {
                // mobile: textarea fallback
                var ta = document.createElement('textarea');
                ta.id = 'db-mobile-ta';
                ta.value = defaultQuery;
                ta.placeholder = '%s';
                ta.style.cssText = 'width:100%%;height:100%%;background:#1e1e1e;color:#d4d4d4;border:none;padding:12px;font-family:D2Coding,monospace;font-size:14px;resize:none;outline:none;box-sizing:border-box;tab-size:2;-moz-tab-size:2';
                ta.spellcheck = false;
                container.appendChild(ta);
                return;
            }

            if (typeof monaco === 'undefined') return;

            editor = monaco.editor.create(container, {
                value: defaultQuery,
                language: langMap[instanceType] || 'sql',
                theme: 'vs-dark',
                fontFamily: "'D2Coding', 'Fira Code', monospace",
                fontSize: 14,
                lineNumbers: 'on',
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                wordWrap: 'on',
                automaticLayout: true,
                padding: { top: 8 },
                suggestOnTriggerCharacters: true,
                tabSize: 2,
                renderLineHighlight: 'line',
                overviewRulerBorder: false,
                scrollbar: { vertical: 'auto', horizontal: 'auto' },
                placeholder: '%s'
            });

            // Ctrl+Enter execute
            editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, function() {
                app.executeQuery();
            });
        },

        executeQuery: function() {
            var query;
            if (editor) {
                query = editor.getValue().trim();
            } else {
                var ta = document.getElementById('db-mobile-ta');
                if (ta) query = ta.value.trim();
            }
            if (!query) return;

            const statusEl = document.getElementById('db-status');
            if (statusEl) statusEl.textContent = 'running...';

            const startTime = Date.now();

            fetch('/api/instances/' + instanceId + '/query', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ query: query })
            })
            .then(function(r) { return r.json(); })
            .then(function(data) {
                const elapsed = Date.now() - startTime;
                if (statusEl) statusEl.textContent = (data.success ? 'complete' : 'error') + ' (' + elapsed + 'ms)';

                if (data.success) {
                    app.renderResult(data.output, query);
                    queryHistory.push({ query: query, time: new Date().toLocaleTimeString('ko-KR') });
                } else {
                    app.renderError(data.error);
                }
            })
            .catch(function(e) {
                if (statusEl) statusEl.textContent = 'error';
                app.renderError('request failed: ' + e.message);
            });
        },

        renderResult: function(output, query) {
            const container = document.getElementById('db-result');
            if (!container) return;

            // tab update
            const tabsEl = document.getElementById('db-tabs');
            if (tabsEl) {
                const shortQuery = query.length > 30 ? query.substring(0, 30) + '...' : query;
                tabsEl.innerHTML = '<div style="padding: 6px 16px; font-size: 12px; border-bottom: 2px solid hsl(var(--p)); font-weight: 600; white-space: nowrap;">' +
                    app.escapeHtml(shortQuery) + '</div>';
            }

            // output table as parse attempt
            const parsed = app.parseOutput(output);
            if (parsed && parsed.headers.length > 0 && parsed.rows.length > 0) {
                app.renderTable(container, parsed);
            } else {
                // raw output
                container.innerHTML = '<pre style="padding: 16px; margin: 0; font-family: \'D2Coding\', monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; overflow: auto; height: 100%%;">' +
                    app.escapeHtml(output || '(result none)') + '</pre>';
            }
        },

        renderError: function(error) {
            const container = document.getElementById('db-result');
            if (!container) return;
            container.innerHTML = '<div style="padding: 16px;"><div class="alert alert-error"><span>' +
                app.escapeHtml(error) + '</span></div></div>';
        },

        parseOutput: function(output) {
            if (!output || output.trim() === '') return null;

            const lines = output.split('\n').filter(function(l) { return l.trim() !== ''; });
            if (lines.length < 2) return null;

            // MySQL/PostgreSQL table format detect: separator(+--+--+ or ---+--- or ---|---)
            // MySQL: +----+------+ format
            if (lines[0].match(/^\+[-+]+\+$/)) {
                return app.parseMySQLTable(lines);
            }

            // PostgreSQL: header | separator | data
            if (lines.length >= 2 && lines[1].match(/^[-+]+$/)) {
                return app.parsePostgreSQLTable(lines);
            }

            // tab separator (MySQL --batch mode etc)
            if (lines[0].includes('\t')) {
                return app.parseTSV(lines);
            }

            // pipe(|) separator
            if (lines[0].includes('|')) {
                return app.parsePipeTable(lines);
            }

            return null;
        },

        parseMySQLTable: function(lines) {
            // MySQL format: +---+---+\n| h | h |\n+---+---+\n| d | d |\n+---+---+
            var headers = [];
            var rows = [];
            for (var i = 0; i < lines.length; i++) {
                var line = lines[i];
                if (line.match(/^\+[-+]+\+$/)) continue; // separator skip
                if (line.startsWith('|')) {
                    var cells = line.split('|').slice(1, -1).map(function(c) { return c.trim(); });
                    if (headers.length === 0) {
                        headers = cells;
                    } else {
                        rows.push(cells);
                    }
                }
            }
            return headers.length > 0 ? { headers: headers, rows: rows } : null;
        },

        parsePostgreSQLTable: function(lines) {
            // PostgreSQL: header1 | header2 \n --------+--------\n val1 | val2
            var headers = lines[0].split('|').map(function(c) { return c.trim(); });
            var rows = [];
            for (var i = 2; i < lines.length; i++) {
                if (lines[i].match(/^\(\d+ rows?\)$/)) continue; // (N rows) skip
                var cells = lines[i].split('|').map(function(c) { return c.trim(); });
                if (cells.length === headers.length) {
                    rows.push(cells);
                }
            }
            return headers.length > 0 ? { headers: headers, rows: rows } : null;
        },

        parseTSV: function(lines) {
            var headers = lines[0].split('\t');
            var rows = [];
            for (var i = 1; i < lines.length; i++) {
                rows.push(lines[i].split('\t'));
            }
            return { headers: headers, rows: rows };
        },

        parsePipeTable: function(lines) {
            var dataLines = lines.filter(function(l) { return l.includes('|') && !l.match(/^[-+|]+$/); });
            if (dataLines.length < 2) return null;
            var headers = dataLines[0].split('|').map(function(c) { return c.trim(); }).filter(function(c) { return c !== ''; });
            var rows = [];
            for (var i = 1; i < dataLines.length; i++) {
                var cells = dataLines[i].split('|').map(function(c) { return c.trim(); }).filter(function(c) { return c !== ''; });
                if (cells.length > 0) rows.push(cells);
            }
            return headers.length > 0 ? { headers: headers, rows: rows } : null;
        },

        renderTable: function(container, parsed) {
            var html = '<div style="overflow: auto; height: 100%%;"><table class="table table-xs table-pin-rows" style="font-family: \'D2Coding\', monospace; font-size: 13px;">';
            html += '<thead><tr style="background: hsl(var(--b2));">';
            html += '<th style="width: 40px; text-align: center; color: hsl(var(--bc) / 0.4);">#</th>';
            for (var h = 0; h < parsed.headers.length; h++) {
                html += '<th style="white-space: nowrap; padding: 6px 12px;">' + app.escapeHtml(parsed.headers[h]) + '</th>';
            }
            html += '</tr></thead><tbody>';
            for (var r = 0; r < parsed.rows.length; r++) {
                html += '<tr class="hover">';
                html += '<td style="text-align: center; color: hsl(var(--bc) / 0.3);">' + (r + 1) + '</td>';
                for (var c = 0; c < parsed.rows[r].length; c++) {
                    var val = parsed.rows[r][c];
                    var isNull = (val === 'NULL' || val === 'null' || val === '<null>');
                    var style = isNull ? 'color: hsl(var(--bc) / 0.3); font-style: italic;' : '';
                    html += '<td style="padding: 4px 12px; max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; ' + style + '" title="' + app.escapeHtml(val) + '">' + app.escapeHtml(val) + '</td>';
                }
                html += '</tr>';
            }
            html += '</tbody></table></div>';
            html += '<div style="padding: 4px 12px; border-top: 1px solid hsl(var(--bc) / 0.1); font-size: 11px; color: hsl(var(--bc) / 0.5);">' + parsed.rows.length + 'rows returned</div>';
            container.innerHTML = html;
        },

        refreshTree: function() {
            const treeEl = document.getElementById('db-tree');
            if (!treeEl) return;
            treeEl.innerHTML = '<div style="padding: 8px 16px; color: hsl(var(--bc) / 0.5); font-size: 12px;">loading...</div>';

            app.runQuery(listDbQuery, function(output) {
                if (!output) {
                    treeEl.innerHTML = '<div style="padding: 8px 16px; color: hsl(var(--er)); font-size: 12px;"> load failed</div>';
                    return;
                }
                app.buildTree(output);
            });
        },

        runQuery: function(query, callback) {
            fetch('/api/instances/' + instanceId + '/query', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ query: query })
            })
            .then(function(r) { return r.json(); })
            .then(function(data) {
                callback(data.success ? data.output : null, data.error);
            })
            .catch(function() { callback(null); });
        },

        buildTree: function(output) {
            const treeEl = document.getElementById('db-tree');
            if (!treeEl) return;

            var html = '';
            var lines = output.split('\n').filter(function(l) { return l.trim() !== ''; });

            if (instanceType === 'mysql') {
                // MySQL SHOW DATABASES output parse
                var dbs = [];
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    if (line.match(/^\+[-+]+\+$/) || line === 'Database') continue;
                    if (line.startsWith('|')) {
                        var dbName = line.replace(/^\||\|$/g, '').trim();
                        if (dbName && dbName !== 'Database') dbs.push(dbName);
                    } else if (!line.match(/^[-+]+$/) && line !== '') {
                        dbs.push(line);
                    }
                }
                for (var d = 0; d < dbs.length; d++) {
                    html += app.treeItem('db', dbs[d], 0, 'mysql');
                }
            } else if (instanceType === 'postgresql') {
                // PostgreSQL \\list output: name | owner | ...
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    if (line.match(/^[-+]+$/) || line.startsWith('Name') || line === '' || line.match(/^\(\d+ rows?\)$/)) continue;
                    if (line.includes('|')) {
                        var parts = line.split('|');
                        var dbName = parts[0].trim();
                        if (dbName && dbName !== 'Name' && !dbName.match(/^-+$/)) {
                            html += app.treeItem('db', dbName, 0, 'postgresql');
                        }
                    }
                }
            } else if (instanceType === 'mongodb') {
                // MongoDB show dbs
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    if (!line || line.startsWith('Proxy') || line.startsWith('Using') || line.startsWith('---')) continue;
                    var parts = line.split(/\s+/);
                    if (parts.length >= 1 && parts[0]) {
                        html += app.treeItem('db', parts[0], 0, 'mongodb');
                    }
                }
            } else if (instanceType === 'redis') {
                // Redis INFO keyspace — db0:keys=N,...
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    var match = line.match(/^(db\d+):keys=(\d+)/);
                    if (match) {
                        html += app.treeItem('db', match[1] + ' (' + match[2] + ' keys)', 0, 'redis');
                    }
                }
                if (!html) {
                    html += '<div style="padding: 8px 16px; font-size: 12px;">Redis  based — below query data use please</div>';
                }
            }

            if (!html) {
                html = '<div style="padding: 8px 16px; color: hsl(var(--bc) / 0.5); font-size: 12px;">item none</div>';
            }

            treeEl.innerHTML = html;
        },

        treeItem: function(type_, name, indent, dbType) {
            var paddingLeft = 8 + indent * 16;
            var icon = type_ === 'db' ? '\ud83d\uddc4' : (type_ === 'table' ? '\ud83d\udcca' : '\ud83d\udcc4');
            var clickAction = '';
            if (type_ === 'db') {
                clickAction = 'dbBrowser.selectDatabase(\'' + app.escapeJs(name) + '\')';
            } else if (type_ === 'table') {
                clickAction = 'dbBrowser.selectTable(\'' + app.escapeJs(name) + '\')';
            }
            return '<div class="db-tree-item" style="padding: 3px 8px 3px ' + paddingLeft + 'px; cursor: pointer; display: flex; align-items: center; gap: 6px; user-select: none;" ' +
                'onclick="' + clickAction + '" ' +
                'onmouseover="this.style.background=\'hsl(var(--bc) / 0.08)\'" onmouseout="this.style.background=\'\'">' +
                '<span style="font-size: 14px;">' + icon + '</span>' +
                '<span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">' + app.escapeHtml(name) + '</span>' +
                '</div>';
        },

        selectDatabase: function(dbName) {
            // name remove key count info (Redis: "db0 (5 keys)" -> "db0")
            var cleanName = dbName.replace(/\s*\(.*\)$/, '');
            currentDb = cleanName;

            // DB table/collection list  load
            var query = '';
            if (instanceType === 'mysql') {
                query = 'USE ' + cleanName + '; SHOW TABLES;';
            } else if (instanceType === 'postgresql') {
                query = '\\c ' + cleanName + ' && \\dt';
            } else if (instanceType === 'mongodb') {
                query = 'use ' + cleanName + '; show collections;';
            } else if (instanceType === 'redis') {
                query = 'SELECT ' + cleanName.replace('db', '') + '\nKEYS *';
                // Redis SELECT not handled well in redis-cli, execute query directly
                app.setEditorValue(query);
                return;
            }

            // data query setup
            app.setEditorValue(query);

            app.runQuery(query, function(output) {
                if (!output) return;
                // tree table list add
                app.expandDatabase(cleanName, output);
            });
        },

        expandDatabase: function(dbName, output) {
            const treeEl = document.getElementById('db-tree');
            if (!treeEl) return;

            // find DB item in existing tree
            var items = treeEl.querySelectorAll('.db-tree-item');
            var targetItem = null;
            for (var i = 0; i < items.length; i++) {
                var text = items[i].textContent.trim();
                if (text.includes(dbName)) {
                    targetItem = items[i];
                    break;
                }
            }

            if (!targetItem) return;

            // existing sub item remove
            var next = targetItem.nextSibling;
            while (next && next.classList && next.classList.contains('db-tree-child')) {
                var toRemove = next;
                next = next.nextSibling;
                toRemove.remove();
            }

            // table list parse
            var tables = [];
            var lines = output.split('\n').filter(function(l) { return l.trim() !== ''; });

            if (instanceType === 'mysql') {
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    if (line.match(/^\+[-+]+\+$/) || line.match(/^Tables_in_/i) || line.match(/^[-+]+$/)) continue;
                    if (line.startsWith('|')) {
                        var tbl = line.replace(/^\||\|$/g, '').trim();
                        if (tbl && !tbl.match(/^Tables_in_/i)) tables.push(tbl);
                    }
                }
            } else if (instanceType === 'postgresql') {
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    if (line.includes('|') && !line.match(/^[-+]+$/) && !line.startsWith('Schema')) {
                        var parts = line.split('|');
                        if (parts.length >= 2) {
                            var tbl = parts[1].trim();
                            if (tbl && tbl !== 'Name') tables.push(tbl);
                        }
                    }
                }
            } else if (instanceType === 'mongodb') {
                for (var i = 0; i < lines.length; i++) {
                    var line = lines[i].trim();
                    if (line && !line.startsWith('switched') && !line.startsWith('already') && !line.startsWith('Using')) {
                        tables.push(line);
                    }
                }
            }

            // insert sub item
            var fragment = document.createDocumentFragment();
            for (var t = 0; t < tables.length; t++) {
                var div = document.createElement('div');
                div.className = 'db-tree-item db-tree-child';
                div.style.cssText = 'padding: 3px 8px 3px 32px; cursor: pointer; display: flex; align-items: center; gap: 6px; user-select: none; font-size: 13px;';
                div.setAttribute('onmouseover', "this.style.background='hsl(var(--bc) / 0.08)'");
                div.setAttribute('onmouseout', "this.style.background=''");
                var tableName = tables[t];
                div.onclick = (function(tn) {
                    return function() { app.selectTable(tn); };
                })(tableName);
                div.innerHTML = '<span style="font-size: 14px;">\ud83d\udcca</span><span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">' + app.escapeHtml(tableName) + '</span>';
                fragment.appendChild(div);
            }

            if (targetItem.nextSibling) {
                treeEl.insertBefore(fragment, targetItem.nextSibling);
            } else {
                treeEl.appendChild(fragment);
            }
        },

        selectTable: function(tableName) {
            var query = '';
            if (instanceType === 'mysql') {
                if (currentDb) {
                    query = 'USE ' + currentDb + '; SELECT * FROM ' + tableName + ' LIMIT 50;';
                } else {
                    query = 'SELECT * FROM ' + tableName + ' LIMIT 50;';
                }
            } else if (instanceType === 'postgresql') {
                query = 'SELECT * FROM ' + tableName + ' LIMIT 50;';
            } else if (instanceType === 'mongodb') {
                if (currentDb) {
                    query = 'use ' + currentDb + '; db.' + tableName + '.find().limit(20).toArray();';
                } else {
                    query = 'db.' + tableName + '.find().limit(20).toArray();';
                }
            } else if (instanceType === 'redis') {
                query = 'GET ' + tableName;
            }

            app.setEditorValue(query);
            app.executeQuery();
        },

        setEditorValue: function(val) {
            if (editor) { editor.setValue(val); }
            else { var ta = document.getElementById('db-mobile-ta'); if (ta) ta.value = val; }
        },

        escapeHtml: function(str) {
            if (!str) return '';
            return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
        },

        escapeJs: function(str) {
            if (!str) return '';
            return str.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n');
        }
    };

    //  asglobal access
    window.dbBrowser = app;

    // initialize - wait for Monaco loading (mobile Monaco unnecessary)
    if (window.__isMobile || typeof monaco !== 'undefined') {
        app.init();
    } else {
        var waitMonaco = setInterval(function() {
            if (typeof monaco !== 'undefined') {
                clearInterval(waitMonaco);
                app.init();
            }
        }, 200);
    }
})();
</script>`,
		inst.ID, inst.Name, typeBadge,
		inst.ID, inst.InstanceType,
		jsStr(defaultQuery), jsStr(listDbQuery), jsStr(listTablesQuery),
		placeholder, placeholder)
}
