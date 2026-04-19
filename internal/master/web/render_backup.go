package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

func buildBackupsHTML(data map[string]interface{}) string {
	instsI, ok := data["Instances"]
	if !ok || instsI == nil {
		return `<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">backup management</h1>
        <div class="card bg-base-100 shadow-xl"><div class="card-body">
            <p class="text-gray-500">instance is missing.</p>
        </div></div>`
	}

	instances, ok := instsI.([]*store.Instance)
	if !ok || len(instances) == 0 {
		return `<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">backup management</h1>
        <div class="card bg-base-100 shadow-xl"><div class="card-body">
            <p class="text-gray-500">instance is missing.</p>
        </div></div>`
	}

	var cards string
	for _, inst := range instances {
		badge := statusBadgeHTML(inst.Status)
		cards += fmt.Sprintf(`
        <div class="card bg-base-100 shadow-xl">
            <div class="card-body">
                <div class="flex justify-between items-center">
                    <h3 class="card-title text-lg">%s %s</h3>
                    <button class="btn btn-sm btn-primary" onclick="createBackup('%s')">create backup</button>
                </div>
                <div id="backups-%s" class="mt-4"
                     hx-get="/htmx/backup-list/%s" hx-trigger="load, every 30s" hx-swap="innerHTML">
                    <div class="flex justify-center"><span class="loading loading-spinner"></span></div>
                </div>
            </div>
        </div>`, inst.Name, badge, inst.ID, inst.ID, inst.ID)
	}

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">backup management</h1>
    <div class="grid grid-cols-1 gap-6">%s</div>`, cards)
}

func buildBackupListHTML(data map[string]interface{}) string {
	bkI, ok := data["Backups"]
	if !ok || bkI == nil {
		return `<p class="text-sm text-gray-500">backup history is missing.</p>`
	}

	backups, ok := bkI.([]*store.Backup)
	if !ok || len(backups) == 0 {
		return `<p class="text-sm text-gray-500">backup history is missing.</p>`
	}

	var rows string
	for _, b := range backups {
		badge := statusBadgeHTML(b.Status)
		trigger := backupTriggerLabel(b.TriggerType)
		size := formatFileSize(b.FileSize)
		timeStr := b.CreatedAt.Format("2006-01-02 15:04:05")

		rows += fmt.Sprintf(`<tr>
			<td class="text-xs">%s</td>
			<td><code class="text-xs">%s</code></td>
			<td class="text-xs">%s</td>
			<td class="text-xs">%s</td>
			<td>%s</td>
			<td>
				<button class="btn btn-xs btn-outline btn-warning whitespace-nowrap" onclick="restoreBackup('%s','%s')">restore</button>
			</td>
		</tr>`, timeStr, b.FilePath, size, trigger, badge, b.InstanceID, b.FilePath)
	}

	return fmt.Sprintf(`
    <div class="overflow-x-auto">
        <table class="table table-zebra table-sm">
            <thead><tr><th>time</th><th>path</th><th>size</th><th>type</th><th>state</th><th>management</th></tr></thead>
            <tbody>%s</tbody>
        </table>
    </div>`, rows)
}
