package web

import (
	"fmt"
	"strings"

	"craftstack/internal/master/store"
)

// ─────────────────────────────────────────────────────────────
// login page (out none — standalone page)
// ─────────────────────────────────────────────────────────────

func buildLoginPageHTML(msg string) string {
	var alertHTML string
	if msg != "" {
		alertClass := "alert-info"
		if strings.Contains(msg, "not valid") || strings.Contains(msg, "failed") {
			alertClass = "alert-error"
		} else if strings.Contains(msg, "approve") {
			alertClass = "alert-warning"
		} else if strings.Contains(msg, "complete") {
			alertClass = "alert-success"
		}
		alertHTML = fmt.Sprintf(`<div class="alert %s text-sm mb-4"><span>%s</span></div>`, alertClass, msg)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ko" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>login - CraftStack</title>
    <link href="https://cdn.jsdelivr.net/npm/daisyui@4.12.14/dist/full.min.css" rel="stylesheet" type="text/css" />
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="min-h-screen bg-base-200 flex items-center justify-center">
    <div class="card w-full max-w-md bg-base-100 shadow-xl">
        <div class="card-body">
            <h2 class="card-title text-2xl font-bold text-center justify-center mb-2">CraftStack</h2>
            <p class="text-center text-sm opacity-60 mb-4">server manage platform login</p>
            %s
            <form method="POST" action="/login">
                <div class="form-control mb-3">
                    <label class="label"><span class="label-text">ID</span></label>
                    <input type="text" name="username" class="input input-bordered w-full" placeholder="admin" required autofocus>
                </div>
                <div class="form-control mb-4">
                    <label class="label"><span class="label-text">password</span></label>
                    <input type="password" name="password" class="input input-bordered w-full" placeholder="password" required>
                </div>
                <button type="submit" class="btn btn-primary w-full">login</button>
            </form>
            <div class="divider text-xs">OR</div>
            <a href="/register" class="btn btn-outline btn-sm w-full">sign up</a>
        </div>
    </div>
</body>
</html>`, alertHTML)
}

// ─────────────────────────────────────────────────────────────
// sign up page
// ─────────────────────────────────────────────────────────────

func buildRegisterPageHTML(msg string) string {
	var alertHTML string
	if msg != "" {
		alertHTML = fmt.Sprintf(`<div class="alert alert-error text-sm mb-4"><span>%s</span></div>`, msg)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ko" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>sign up - CraftStack</title>
    <link href="https://cdn.jsdelivr.net/npm/daisyui@4.12.14/dist/full.min.css" rel="stylesheet" type="text/css" />
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="min-h-screen bg-base-200 flex items-center justify-center">
    <div class="card w-full max-w-md bg-base-100 shadow-xl">
        <div class="card-body">
            <h2 class="card-title text-2xl font-bold text-center justify-center mb-2">sign up</h2>
            <p class="text-center text-sm opacity-60 mb-4">admin approve after usable</p>
            %s
            <form method="POST" action="/register">
                <div class="form-control mb-3">
                    <label class="label"><span class="label-text">ID</span></label>
                    <input type="text" name="username" class="input input-bordered w-full" placeholder="3~32" minlength="3" maxlength="32" required autofocus>
                </div>
                <div class="form-control mb-3">
                    <label class="label"><span class="label-text">password</span></label>
                    <input type="password" name="password" class="input input-bordered w-full" placeholder="4 or more" minlength="4" required>
                </div>
                <div class="form-control mb-4">
                    <label class="label"><span class="label-text">confirm password</span></label>
                    <input type="password" name="confirm_password" class="input input-bordered w-full" placeholder="confirm password" required>
                </div>
                <button type="submit" class="btn btn-primary w-full">sign up new</button>
            </form>
            <div class="divider text-xs">OR</div>
            <a href="/login" class="btn btn-outline btn-sm w-full">login as run</a>
        </div>
    </div>
</body>
</html>`, alertHTML)
}

// ─────────────────────────────────────────────────────────────
// user management page (admin only)
// ─────────────────────────────────────────────────────────────

func buildUsersHTML(data map[string]interface{}) string {
	usersI, _ := data["Users"]
	users, _ := usersI.([]*store.User)

	var pendingRows, approvedRows string
	pendingCount := 0
	approvedCount := 0

	for _, u := range users {
		roleBadge := userRoleBadge(u.Role)
		createdAt := u.CreatedAt.Format("2006-01-02 15:04")

		if !u.Approved {
			pendingCount++
			pendingRows += fmt.Sprintf(`<tr>
				<td>%s</td>
				<td>%s</td>
				<td>%s</td>
				<td>
					<div class="flex gap-1">
						<button class="btn btn-xs btn-success" onclick="approveUser(%d)">approve</button>
						<button class="btn btn-xs btn-error" onclick="rejectUser(%d)">reject</button>
					</div>
				</td>
			</tr>`, u.Username, roleBadge, createdAt, u.ID, u.ID)
		} else {
			approvedCount++
			approvedRows += fmt.Sprintf(`<tr>
				<td><span class="font-semibold">%s</span></td>
				<td>%s</td>
				<td>%s</td>
				<td>
					<div class="flex gap-1 flex-wrap">
						<select class="select select-xs select-bordered" onchange="changeRole(%d, this.value)">
							<option value="admin" %s>admin</option>
							<option value="editor" %s>editor</option>
							<option value="viewer" %s>viewer</option>
						</select>
						<button class="btn btn-xs btn-outline" onclick="resetPassword(%d, '%s')">password initialize</button>
						<button class="btn btn-xs btn-error btn-outline" onclick="deleteUser(%d, '%s')">delete</button>
					</div>
				</td>
			</tr>`, u.Username, roleBadge, createdAt,
				u.ID, selected(u.Role, "admin"), selected(u.Role, "editor"), selected(u.Role, "viewer"),
				u.ID, u.Username,
				u.ID, u.Username)
		}
	}

	var pendingSection string
	if pendingCount > 0 {
		pendingSection = fmt.Sprintf(`
		<div class="card bg-base-100 shadow-xl mb-6">
			<div class="card-body">
				<h2 class="card-title">pending approval (%d)</h2>
				<div class="overflow-x-auto">
					<table class="table table-zebra">
						<thead><tr><th>ID</th><th>role</th><th>sign upday</th><th>manage</th></tr></thead>
						<tbody>%s</tbody>
					</table>
				</div>
			</div>
		</div>`, pendingCount, pendingRows)
	}

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">user management</h1>
	%s
	<div class="card bg-base-100 shadow-xl">
		<div class="card-body">
			<h2 class="card-title">approve user (%d)</h2>
			<div class="overflow-x-auto">
				<table class="table table-zebra">
					<thead><tr><th>ID</th><th>role</th><th>sign upday</th><th>manage</th></tr></thead>
					<tbody>%s</tbody>
				</table>
			</div>
		</div>
	</div>
	<script>
	async function approveUser(id) {
		if (!confirm(' user approve?')) return;
		const r = await fetch('/api/users/'+id+'/approve', {method:'POST'});
		const d = await r.json();
		alert(d.message);
		location.reload();
	}
	async function rejectUser(id) {
		if (!confirm(' user reject/delete?')) return;
		const r = await fetch('/api/users/'+id+'/reject', {method:'POST'});
		const d = await r.json();
		alert(d.message);
		location.reload();
	}
	async function deleteUser(id, name) {
		if (!confirm(name + ' user delete?')) return;
		const r = await fetch('/api/users/'+id, {method:'DELETE'});
		const d = await r.json();
		alert(d.message);
		location.reload();
	}
	async function changeRole(id, role) {
		const r = await fetch('/api/users/'+id+'/role', {
			method:'PUT', headers:{'Content-Type':'application/json'},
			body: JSON.stringify({role: role})
		});
		const d = await r.json();
		alert(d.message);
		if (d.status === 'success') location.reload();
	}
	async function resetPassword(id, name) {
		const pw = prompt(name + ' new password please enter (4 or more):');
		if (!pw || pw.length < 4) { alert('password 4 must be at least '); return; }
		const r = await fetch('/api/users/'+id+'/password', {
			method:'PUT', headers:{'Content-Type':'application/json'},
			body: JSON.stringify({new_password: pw})
		});
		const d = await r.json();
		alert(d.message);
	}
	</script>`, pendingSection, approvedCount, approvedRows)
}

// ─────────────────────────────────────────────────────────────
// profile page (edit own info)
// ─────────────────────────────────────────────────────────────

func buildProfileHTML(data map[string]interface{}) string {
	user, _ := data["Profile"].(*store.User)
	if user == nil {
		return `<div class="alert alert-error">user info load cannot</div>`
	}
	roleBadge := userRoleBadge(user.Role)

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">my profile</h1>
	<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
		<!-- info card -->
		<div class="card bg-base-100 shadow-xl">
			<div class="card-body">
				<h2 class="card-title">account info</h2>
				<div class="space-y-3">
					<div><span class="text-sm text-gray-500">ID</span><div class="font-semibold text-lg">%s</div></div>
					<div><span class="text-sm text-gray-500">role</span><div>%s</div></div>
					<div><span class="text-sm text-gray-500">sign upday</span><div class="text-sm">%s</div></div>
				</div>
			</div>
		</div>

		<!-- ID change -->
		<div class="card bg-base-100 shadow-xl">
			<div class="card-body">
				<h2 class="card-title">ID change</h2>
				<div id="username-result"></div>
				<div class="form-control mb-3">
					<label class="label"><span class="label-text">new ID</span></label>
					<input id="new-username" type="text" class="input input-bordered w-full" value="%s" minlength="3" maxlength="32">
				</div>
				<button class="btn btn-primary btn-sm" onclick="changeUsername()">change</button>
			</div>
		</div>

		<!-- change password -->
		<div class="card bg-base-100 shadow-xl md:col-span-2">
			<div class="card-body">
				<h2 class="card-title">change password</h2>
				<div id="password-result"></div>
				<div class="grid grid-cols-1 md:grid-cols-3 gap-3">
					<div class="form-control">
						<label class="label"><span class="label-text">current password</span></label>
						<input id="current-password" type="password" class="input input-bordered w-full">
					</div>
					<div class="form-control">
						<label class="label"><span class="label-text">new password</span></label>
						<input id="new-password" type="password" class="input input-bordered w-full" minlength="4">
					</div>
					<div class="form-control">
						<label class="label"><span class="label-text">new confirm password</span></label>
						<input id="confirm-password" type="password" class="input input-bordered w-full">
					</div>
				</div>
				<div class="mt-3">
					<button class="btn btn-primary btn-sm" onclick="changePassword()">change password</button>
				</div>
			</div>
		</div>
	</div>
	<script>
	async function changeUsername() {
		const username = document.getElementById('new-username').value.trim();
		if (!username || username.length < 3) { alert('ID 3 must be at least '); return; }
		const r = await fetch('/api/users/%d/username', {
			method:'PUT', headers:{'Content-Type':'application/json'},
			body: JSON.stringify({username: username})
		});
		const d = await r.json();
		document.getElementById('username-result').innerHTML =
			'<div class="alert alert-' + (d.status === 'success' ? 'success' : 'error') + ' text-sm mb-2">' + d.message + '</div>';
		if (d.status === 'success') setTimeout(() => location.href = '/login', 1500);
	}
	async function changePassword() {
		const cur = document.getElementById('current-password').value;
		const pw = document.getElementById('new-password').value;
		const pw2 = document.getElementById('confirm-password').value;
		if (!cur) { alert('current password please enter'); return; }
		if (pw.length < 4) { alert('new password 4 must be at least '); return; }
		if (pw !== pw2) { alert('new passwords do not match'); return; }
		const r = await fetch('/api/users/%d/password', {
			method:'PUT', headers:{'Content-Type':'application/json'},
			body: JSON.stringify({current_password: cur, new_password: pw})
		});
		const d = await r.json();
		document.getElementById('password-result').innerHTML =
			'<div class="alert alert-' + (d.status === 'success' ? 'success' : 'error') + ' text-sm mb-2">' + d.message + '</div>';
		if (d.status === 'success') setTimeout(() => location.href = '/login', 1500);
	}
	</script>`,
		user.Username, roleBadge, user.CreatedAt.Format("2006-01-02 15:04"),
		user.Username,
		user.ID, user.ID)
}

// ─────────────────────────────────────────────────────────────
// utility
// ─────────────────────────────────────────────────────────────

func userRoleBadge(role string) string {
	switch role {
	case store.RoleAdmin:
		return `<span class="badge badge-error badge-sm">admin</span>`
	case store.RoleEditor:
		return `<span class="badge badge-warning badge-sm">editor</span>`
	case store.RoleViewer:
		return `<span class="badge badge-info badge-sm">viewer</span>`
	default:
		return fmt.Sprintf(`<span class="badge badge-ghost badge-sm">%s</span>`, role)
	}
}

func selected(current, value string) string {
	if current == value {
		return "selected"
	}
	return ""
}
