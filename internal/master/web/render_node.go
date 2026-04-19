package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

// ─────────────────────────────────────────────────────────────
// node
// ─────────────────────────────────────────────────────────────

func buildNodesHTML(data map[string]interface{}, tableOnly bool) string {
	nodesI, ok := data["Nodes"]
	if !ok || nodesI == nil {
		return nodesEmptyHTML(tableOnly)
	}

	nodes, ok := nodesI.([]*store.Node)
	if !ok || len(nodes) == 0 {
		return nodesEmptyHTML(tableOnly)
	}

	var rows string
	for _, n := range nodes {
		badge := statusBadgeHTML(n.Status)
		memoryGB := fmt.Sprintf("%.1f GB", float64(n.MemoryMB)/1024)
		ago := formatTimeAgo(n.UpdatedAt)
		rows += fmt.Sprintf(`<tr>
			<td>
				<div class="flex items-center gap-2">
					<div class="w-2 h-2 rounded-full %s"></div>
					<a href="/nodes/%s" class="link link-hover font-semibold">%s</a>
				</div>
			</td>
			<td class="hidden sm:table-cell"><code class="text-xs">%s</code></td>
			<td>%s</td>
			<td class="hidden md:table-cell">%dcore</td>
			<td class="hidden md:table-cell">%s</td>
			<td class="hidden lg:table-cell"><code class="text-xs opacity-60">%s</code></td>
			<td class="text-xs opacity-60 hidden sm:table-cell">%s</td>
			<td>
				<a href="/nodes/%s" class="btn btn-xs btn-outline btn-info whitespace-nowrap">detail</a>
			</td>
		</tr>`,
			statusDotClass(n.Status),
			n.ID, n.Name, n.Address, badge, n.CPUCores, memoryGB, n.OSInfo, ago, n.ID)
	}

	table := fmt.Sprintf(`
    <div class="overflow-x-auto">
        <table class="table table-zebra">
            <thead><tr><th>name</th><th class="hidden sm:table-cell">address</th><th>state</th><th class="hidden md:table-cell">CPU</th><th class="hidden md:table-cell">memory</th><th class="hidden lg:table-cell">OS</th><th class="hidden sm:table-cell">last response</th><th>manage</th></tr></thead>
            <tbody>%s</tbody>
        </table>
    </div>`, rows)

	if tableOnly {
		return table
	}
	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">node manage</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">register node (%d)</h2>
        </div>
        <div hx-get="/htmx/nodes-table" hx-trigger="every 10s" hx-swap="innerHTML">
        %s
        </div>
    </div></div>`, len(nodes), table)
}

func nodesEmptyHTML(tableOnly bool) string {
	table := `
    <div class="overflow-x-auto">
        <table class="table table-zebra">
            <thead><tr><th>name</th><th class="hidden sm:table-cell">address</th><th>state</th><th class="hidden md:table-cell">CPU</th><th class="hidden md:table-cell">memory</th><th class="hidden lg:table-cell">OS</th><th class="hidden sm:table-cell">last response</th><th>manage</th></tr></thead>
            <tbody>
                <tr><td colspan="8" class="text-center text-gray-500">the agent connectwhen node display</td></tr>
            </tbody>
        </table>
    </div>`
	if tableOnly {
		return table
	}
	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">node manage</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">%s</div></div>`, table)
}

// ─────────────────────────────────────────────────────────────
// node detail (SRE metrics include)
// ─────────────────────────────────────────────────────────────

func buildNodeDetailHTML(data map[string]interface{}) string {
	nodeI, _ := data["Node"]
	node, _ := nodeI.(*store.Node)
	if node == nil {
		return `<div class="alert alert-error">node not found</div>`
	}

	badge := statusBadgeHTML(node.Status)
	memoryGB := fmt.Sprintf("%.1f GB", float64(node.MemoryMB)/1024)
	ago := formatTimeAgo(node.UpdatedAt)

	// member instance table
	instancesHTML := buildInstancesHTML(data, true)

	// sync history
	syncHTML := buildSyncHistoryHTML(data, true)

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">node: %s</h1>

    <!-- node info + metrics -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- default info -->
        <div class="card bg-base-100 shadow-xl">
            <div class="card-body">
                <h2 class="card-title">node info</h2>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-4">
                    <div><span class="text-xs text-gray-500">name</span><div class="font-semibold">%s</div></div>
                    <div><span class="text-xs text-gray-500">state</span><div>%s</div></div>
                    <div><span class="text-xs text-gray-500">address</span><div><code class="text-sm">%s</code></div></div>
                    <div><span class="text-xs text-gray-500">OS</span><div><code class="text-sm">%s</code></div></div>
                    <div><span class="text-xs text-gray-500">CPU</span><div>%dcore</div></div>
                    <div><span class="text-xs text-gray-500">total memory</span><div>%s</div></div>
                    <div><span class="text-xs text-gray-500">last response</span><div class="text-sm">%s</div></div>
                    <div><span class="text-xs text-gray-500">ID</span><div><code class="text-xs opacity-60">%s</code></div></div>
                </div>
            </div>
        </div>

        <!-- realtime resource metrics (HTMX polling) -->
        <div class="lg:col-span-2 card bg-base-100 shadow-xl">
            <div class="card-body">
                <h2 class="card-title">realtime resource</h2>
                <div id="node-metrics" hx-get="/htmx/node-metrics/%s" hx-trigger="every 5s" hx-swap="innerHTML">
                    %s
                </div>
            </div>
        </div>
    </div>

    <!-- Docker state -->
    <div class="card bg-base-100 shadow-xl mt-6">
        <div class="card-body">
            <h2 class="card-title">Docker state</h2>
            <div id="docker-status" x-data="{ status: null, loading: true, installing: false }">
                <template x-if="loading">
                    <div class="flex items-center gap-2 text-sm"><span class="loading loading-spinner loading-xs"></span> Docker state check during...</div>
                </template>
                <template x-if="!loading && status">
                    <div class="flex items-center gap-4">
                        <template x-if="status.installed && status.running">
                            <div class="flex items-center gap-2">
                                <div class="badge badge-success gap-1">running</div>
                                <span class="text-sm font-mono" x-text="status.version"></span>
                            </div>
                        </template>
                        <template x-if="status.installed && !status.running">
                            <div class="flex items-center gap-2">
                                <div class="badge badge-warning gap-1">install (daemon stop)</div>
                                <span class="text-sm" x-text="status.message"></span>
                            </div>
                        </template>
                        <template x-if="!status.installed">
                            <div class="flex items-center gap-3">
                                <div class="badge badge-error gap-1">notinstall</div>
                                <button class="btn btn-sm btn-primary" x-show="!installing"
                                    x-on:click="installing=true; fetch('/api/nodes/%s/docker/install',{method:'POST'}).then(r=>r.json()).then(d=>{installing=false; checkDocker();}).catch(e=>{installing=false;})">
                                    Docker install
                                </button>
                                <span class="loading loading-spinner loading-sm" x-show="installing"></span>
                                <span x-show="installing" class="text-sm">install during...</span>
                            </div>
                        </template>
                    </div>
                </template>
            </div>
            <script>
                (function() {
                    const el = document.getElementById('docker-status');
                    const alpine = el.__x || null;
                    function checkDocker() {
                        fetch('/api/nodes/%s/docker')
                            .then(r => r.json())
                            .then(d => {
                                const scope = Alpine.$data(el);
                                scope.status = d;
                                scope.loading = false;
                            })
                            .catch(e => {
                                const scope = Alpine.$data(el);
                                scope.status = { installed: false, running: false, message: 'API error' };
                                scope.loading = false;
                            });
                    }
                    window.checkDocker = checkDocker;
                    setTimeout(checkDocker, 500);
                })();
            </script>
        </div>
    </div>

    <!-- member instance -->
    <div class="card bg-base-100 shadow-xl mt-6">
        <div class="card-body">
            <h2 class="card-title">member instance</h2>
            %s
        </div>
    </div>

    <!-- sync history -->
    <div class="card bg-base-100 shadow-xl mt-6">
        <div class="card-body">
            <h2 class="card-title">recent sync history</h2>
            %s
        </div>
    </div>`,
		node.Name,
		node.Name, badge, node.Address, node.OSInfo,
		node.CPUCores, memoryGB, ago, node.ID,
		node.ID, buildNodeMetricsHTML(data),
		node.ID, node.ID,
		instancesHTML, syncHTML)
}

func buildNodeMetricsHTML(data map[string]interface{}) string {
	cpuPercent := getFloat(data, "CPUPercent")
	memPercent := getFloat(data, "MemPercent")
	memUsedMB := getInt64(data, "MemUsedMB")
	memTotalMB := getInt64(data, "MemTotalMB")
	diskPercent := getFloat(data, "DiskPercent")
	diskUsedMB := getInt64(data, "DiskUsedMB")
	diskTotalMB := getInt64(data, "DiskTotalMB")

	cpuColor := progressColor(cpuPercent)
	memColor := progressColor(memPercent)
	diskColor := progressColor(diskPercent)

	cpuLabel := fmt.Sprintf("%.1f%%", cpuPercent)
	memLabel := fmt.Sprintf("%.1f%%", memPercent)
	diskLabel := fmt.Sprintf("%.1f%%", diskPercent)

	return fmt.Sprintf(`
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
        <!-- CPU -->
        <div>
            <div class="flex justify-between mb-1">
                <span class="text-sm font-medium">CPU use</span>
                <span class="text-sm font-bold">%s</span>
            </div>
            <progress class="progress %s w-full" value="%.0f" max="100"></progress>
        </div>
        <!-- memory -->
        <div>
            <div class="flex justify-between mb-1">
                <span class="text-sm font-medium">memory</span>
                <span class="text-sm font-bold">%s / %s (%s)</span>
            </div>
            <progress class="progress %s w-full" value="%.0f" max="100"></progress>
        </div>
        <!-- disk -->
        <div>
            <div class="flex justify-between mb-1">
                <span class="text-sm font-medium">disk</span>
                <span class="text-sm font-bold">%s / %s (%s)</span>
            </div>
            <progress class="progress %s w-full" value="%.0f" max="100"></progress>
        </div>
    </div>`,
		cpuLabel, cpuColor, cpuPercent,
		formatMB(memUsedMB), formatMB(memTotalMB), memLabel, memColor, memPercent,
		formatMB(diskUsedMB), formatMB(diskTotalMB), diskLabel, diskColor, diskPercent)
}
