package web

import (
	"fmt"
	"html"
	"strings"

	"craftstack/internal/master/store"
)

// ─────────────────────────────────────────────────────────────
// audit log page
// ─────────────────────────────────────────────────────────────

func buildAuditHTML(data map[string]interface{}) string {
	logsI, _ := data["AuditLogs"]
	logs, _ := logsI.([]*store.AuditLog)

	page := getInt(data, "Page")
	totalPages := getInt(data, "TotalPages")
	total := getInt(data, "Total")

	var rows string
	if len(logs) == 0 {
		rows = `<tr><td colspan="9" class="text-center text-gray-500">audit log is missing</td></tr>`
	} else {
		for _, l := range logs {
			timeStr := l.Timestamp.Format("2006-01-02 15:04:05")
			actionBadge := auditActionBadge(l.Action)

			fieldStr := l.FieldName
			if fieldStr == "" {
				fieldStr = "-"
			}

			oldVal := truncateValue(l.OldValue, 30)
			newVal := truncateValue(l.NewValue, 30)
			if oldVal == "" {
				oldVal = "-"
			}
			if newVal == "" {
				newVal = "-"
			}

			detail := truncateValue(l.Detail, 50)
			if detail == "" {
				detail = "-"
			}

			targetInfo := fmt.Sprintf(`<div class="text-sm font-semibold">%s</div><div class="text-xs opacity-60">%s #%s</div>`,
				html.EscapeString(l.TargetName), auditTargetTypeLabel(l.TargetType), html.EscapeString(truncateValue(l.TargetID, 20)))

			// rollback button / x
			rollbackCell := "-"
			if l.RolledBack {
				rollbackCell = `<span class="badge badge-ghost badge-sm">rollback</span>`
			} else if l.FieldName != "" && l.TargetType == "instance" {
				rollbackCell = fmt.Sprintf(
					`<button class="btn btn-xs btn-outline btn-warning whitespace-nowrap" onclick="rollbackAudit(%d)">rollback</button>`,
					l.ID)
			}

			rows += fmt.Sprintf(`<tr>
				<td class="text-xs whitespace-nowrap">%s</td>
				<td class="text-sm">%s</td>
				<td>%s</td>
				<td>%s</td>
				<td class="text-xs font-mono hidden md:table-cell">%s</td>
				<td class="text-xs font-mono hidden lg:table-cell">%s</td>
				<td class="text-xs font-mono hidden lg:table-cell">%s</td>
				<td class="text-xs hidden md:table-cell">%s</td>
				<td>%s</td>
			</tr>`,
				timeStr,
				html.EscapeString(l.Username),
				actionBadge,
				targetInfo,
				html.EscapeString(fieldStr),
				html.EscapeString(oldVal),
				html.EscapeString(newVal),
				html.EscapeString(detail),
				rollbackCell)
		}
	}

	// pagination
	var paginationHTML string
	if totalPages > 1 {
		var pages strings.Builder
		pages.WriteString(`<div class="join mt-4 flex justify-center">`)

		if page > 1 {
			pages.WriteString(fmt.Sprintf(`<a href="/audit?page=%d" class="join-item btn btn-sm">«</a>`, page-1))
		} else {
			pages.WriteString(`<button class="join-item btn btn-sm btn-disabled">«</button>`)
		}

		start := page - 2
		if start < 1 {
			start = 1
		}
		end := start + 4
		if end > totalPages {
			end = totalPages
		}
		if end-start < 4 {
			start = end - 4
			if start < 1 {
				start = 1
			}
		}

		for i := start; i <= end; i++ {
			if i == page {
				pages.WriteString(fmt.Sprintf(`<button class="join-item btn btn-sm btn-active">%d</button>`, i))
			} else {
				pages.WriteString(fmt.Sprintf(`<a href="/audit?page=%d" class="join-item btn btn-sm">%d</a>`, i, i))
			}
		}

		if page < totalPages {
			pages.WriteString(fmt.Sprintf(`<a href="/audit?page=%d" class="join-item btn btn-sm">»</a>`, page+1))
		} else {
			pages.WriteString(`<button class="join-item btn btn-sm btn-disabled">»</button>`)
		}

		pages.WriteString(`</div>`)
		paginationHTML = pages.String()
	}

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">audit log</h1>
    <div class="card bg-base-100 shadow-xl">
        <div class="card-body">
            <div class="flex justify-between items-center mb-4">
                <h2 class="card-title">change history (%d total)</h2>
            </div>
            <div class="overflow-x-auto">
                <table class="table table-zebra table-sm">
                    <thead>
                        <tr>
                            <th>time</th>
                            <th>user</th>
                            <th></th>
                            <th>target</th>
                            <th class="hidden md:table-cell">field</th>
                            <th class="hidden lg:table-cell">previousvalue</th>
                            <th class="hidden lg:table-cell">newvalue</th>
                            <th class="hidden md:table-cell">detail</th>
                            <th>rollback</th>
                        </tr>
                    </thead>
                    <tbody>%s</tbody>
                </table>
            </div>
            %s
        </div>
    </div>

    <script>
    async function rollbackAudit(id) {
        if (!confirm('Rollback this change? The field will restore to its previous value.')) return;
        try {
            const resp = await fetch('/api/audit/' + id + '/rollback', { method: 'POST' });
            const data = await resp.json();
            if (data.status === 'success') {
                showToast(data.message, 'success');
                setTimeout(() => location.reload(), 1000);
            } else {
                showToast(data.message || 'rollback failed', 'error');
            }
        } catch(e) {
            showToast('rollback request failed: ' + e.message, 'error');
        }
    }
    </script>`, total, rows, paginationHTML)
}

// auditActionBadge returns a DaisyUI badge for audit action types.
func auditActionBadge(action string) string {
	var class, label string
	switch action {
	case "create":
		class, label = "badge-success", "create"
	case "update":
		class, label = "badge-info", "modify"
	case "delete":
		class, label = "badge-error", "delete"
	case "start":
		class, label = "badge-success", "start"
	case "stop":
		class, label = "badge-warning", "stop"
	case "restart":
		class, label = "badge-warning", "restart"
	case "kill":
		class, label = "badge-error", "forceshutdown"
	case "backup":
		class, label = "badge-primary", "backup"
	case "restore":
		class, label = "badge-accent", "restore"
	case "connect":
		class, label = "badge-info", "connect"
	case "disconnect":
		class, label = "badge-ghost", "connectrelease"
	case "approve":
		class, label = "badge-success", "approve"
	case "reject":
		class, label = "badge-error", "reject"
	case "role_change":
		class, label = "badge-warning", "rolechange"
	default:
		class, label = "badge-ghost", action
	}
	return fmt.Sprintf(`<span class="badge %s badge-sm whitespace-nowrap">%s</span>`, class, label)
}

// auditTargetTypeLabel returns Korean label for target type.
func auditTargetTypeLabel(targetType string) string {
	switch targetType {
	case "instance":
		return "instance"
	case "network":
		return "network"
	case "user":
		return "user"
	case "node":
		return "node"
	case "mesh":
		return "mesh"
	case "sync":
		return "sync"
	case "backup":
		return "backup"
	case "file":
		return "file"
	default:
		return targetType
	}
}

// truncateValue truncates a string for display.
func truncateValue(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
