package web

import "fmt"

// ─────────────────────────────────────────────────────────────
// dashboard
// ─────────────────────────────────────────────────────────────

func buildDashboardHTML(data map[string]interface{}) string {
	stats := buildDashboardStatsHTML(data)

	return fmt.Sprintf(`
    <h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">dashboard</h1>

    <!-- statistics card -->
    <div id="dashboard-stats" hx-get="/htmx/dashboard-stats" hx-trigger="every 10s" hx-swap="innerHTML">
        %s
    </div>

    <!-- recent activity -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mt-6">
        <div class="card bg-base-100 shadow-xl">
            <div class="card-body">
                <h2 class="card-title">node list</h2>
                <div id="nodes-table" hx-get="/htmx/nodes-table" hx-trigger="every 15s" hx-swap="innerHTML">
                    %s
                </div>
            </div>
        </div>
        <div class="card bg-base-100 shadow-xl">
            <div class="card-body">
                <h2 class="card-title">recent sync</h2>
                <div id="sync-history" hx-get="/htmx/sync-history" hx-trigger="every 10s" hx-swap="innerHTML">
                    %s
                </div>
            </div>
        </div>
    </div>
    `, stats,
		buildNodesHTML(data, true),
		buildSyncHistoryHTML(data, true),
	)
}

func buildDashboardStatsHTML(data map[string]interface{}) string {
	totalNodes := getInt(data, "TotalNodes")
	onlineNodes := getInt(data, "OnlineNodes")
	totalInstances := getInt(data, "TotalInstances")
	runningInstances := getInt(data, "RunningInstances")
	totalSyncs := getInt(data, "TotalSyncs")
	totalBackups := getInt(data, "TotalBackups")

	// system all health check
	healthClass := "text-accent"
	healthLabel := "normal"
	healthDesc := "all system normal operationsduring"
	if totalNodes > 0 && onlineNodes == 0 {
		healthClass = "text-error"
		healthLabel = "failure"
		healthDesc = "all node offline"
	} else if totalNodes > 0 && onlineNodes < totalNodes {
		healthClass = "text-warning"
		healthLabel = "warning"
		healthDesc = fmt.Sprintf("%d node offline", totalNodes-onlineNodes)
	} else if totalInstances > 0 && runningInstances == 0 {
		healthClass = "text-warning"
		healthLabel = "warning"
		healthDesc = "runningin instance none"
	}

	return fmt.Sprintf(`
    <div class="stats stats-vertical lg:stats-horizontal shadow w-full bg-base-100 overflow-x-auto">
        <div class="stat">
            <div class="stat-figure text-primary">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2"></path></svg>
            </div>
            <div class="stat-title">all node</div>
            <div class="stat-value text-primary">%d</div>
            <div class="stat-desc">%d online</div>
        </div>
        <div class="stat">
            <div class="stat-figure text-secondary">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
            </div>
            <div class="stat-title">instance</div>
            <div class="stat-value text-secondary">%d</div>
            <div class="stat-desc">%d executeduring</div>
        </div>
        <div class="stat">
            <div class="stat-figure text-info">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"></path></svg>
            </div>
            <div class="stat-title">sync</div>
            <div class="stat-value text-info">%d</div>
            <div class="stat-desc">all sync  entriescount</div>
        </div>
        <div class="stat">
            <div class="stat-figure text-success">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8"></path></svg>
            </div>
            <div class="stat-title">backup</div>
            <div class="stat-value text-success">%d</div>
            <div class="stat-desc">all backup count</div>
        </div>
        <div class="stat">
            <div class="stat-figure %s">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block w-8 h-8 stroke-current"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
            </div>
            <div class="stat-title">system state</div>
            <div class="stat-value %s">%s</div>
            <div class="stat-desc">%s</div>
        </div>
    </div>`, totalNodes, onlineNodes, totalInstances, runningInstances,
		totalSyncs, totalBackups,
		healthClass, healthClass, healthLabel, healthDesc)
}
