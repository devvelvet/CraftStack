package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

// buildMeshHTML generates the mesh network management page.
func buildMeshHTML(data map[string]interface{}) string {
	meshI, _ := data["Mesh"]
	mesh, _ := meshI.(*store.MeshNetwork)

	nodesI, _ := data["Nodes"]
	nodes, _ := nodesI.([]*store.Node)

	dnsI, _ := data["DNSRecords"]
	dnsRecords, _ := dnsI.([]*store.DNSRecord)

	// mesh status
	meshStatus := "inactive"
	meshBadge := "badge-ghost"
	if mesh != nil && mesh.Enabled {
		meshStatus = "active"
		meshBadge = "badge-success"
	}

	// node card
	var nodeCards string
	wgNodeCount := 0
	for _, n := range nodes {
		if n.WGPublicKey == "" {
			continue
		}
		wgNodeCount++

		statusBadge := `<span class="badge badge-ghost badge-sm">offline</span>`
		if n.Status == "online" {
			statusBadge = `<span class="badge badge-success badge-sm">online</span>`
		}

		wgAddr := n.WGAddress
		if wgAddr == "" {
			wgAddr = "notallocate"
		}
		dockerSub := n.DockerSubnet
		if dockerSub == "" {
			dockerSub = "notallocate"
		}

		pubKeyShort := n.WGPublicKey
		if len(pubKeyShort) > 16 {
			pubKeyShort = pubKeyShort[:16] + "..."
		}

		nodeCards += fmt.Sprintf(`
		<div class="card bg-base-200 shadow">
			<div class="card-body p-4">
				<div class="flex justify-between items-center">
					<h3 class="font-bold text-lg">%s</h3>
					%s
				</div>
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-2 mt-3 text-sm">
					<div>
						<span class="text-xs text-gray-500">WireGuard IP</span>
						<div class="font-mono">%s</div>
					</div>
					<div>
						<span class="text-xs text-gray-500">Docker subnet</span>
						<div class="font-mono">%s</div>
					</div>
					<div>
						<span class="text-xs text-gray-500">endpoint</span>
						<div class="font-mono text-xs">%s</div>
					</div>
					<div>
						<span class="text-xs text-gray-500">public key</span>
						<div class="font-mono text-xs">%s</div>
					</div>
				</div>
				<div class="mt-2">
					<button class="btn btn-xs btn-outline btn-info" onclick="checkWGStatus('%s')">state check</button>
				</div>
			</div>
		</div>`,
			n.Name, statusBadge,
			wgAddr, dockerSub,
			n.WGEndpoint, pubKeyShort,
			n.ID)
	}

	if nodeCards == "" {
		nodeCards = `<div class="text-center text-gray-500 py-8 col-span-full">
			WireGuard configuration node is missing. agent registerif done automatically settings.
		</div>`
	}

	// DNS record table
	var dnsRows string
	for _, r := range dnsRecords {
		nodeShort := r.NodeID
		if len(nodeShort) > 8 {
			nodeShort = nodeShort[:8]
		}
		instShort := r.InstanceID
		if len(instShort) > 16 {
			instShort = instShort[:16]
		}
		dnsRows += fmt.Sprintf(`<tr>
			<td class="font-mono font-semibold">%s</td>
			<td class="font-mono">%s</td>
			<td class="text-xs">%s</td>
			<td class="text-xs">%s</td>
			<td class="text-xs">%s</td>
			<td>
				<button class="btn btn-xs btn-outline btn-error" onclick="deleteDNS('%s')">delete</button>
			</td>
		</tr>`,
			r.FQDN, r.IPAddress, r.Name, instShort, nodeShort,
			r.InstanceID)
	}

	if dnsRows == "" {
		dnsRows = `<tr><td colspan="6" class="text-center text-gray-500">
			DNS record is missing. instance createand network connectif done automatically register.
		</td></tr>`
	}

	// notsettings node list (WG notconfiguration)
	var pendingNodes string
	for _, n := range nodes {
		if n.WGPublicKey != "" {
			continue
		}
		statusBadge := `<span class="badge badge-ghost badge-sm">offline</span>`
		if n.Status == "online" {
			statusBadge = `<span class="badge badge-warning badge-sm">WG wait</span>`
		}
		pendingNodes += fmt.Sprintf(`<tr>
			<td>%s %s</td>
			<td class="text-xs font-mono">%s</td>
			<td class="text-xs text-gray-500">WireGuard settings waiting (the agent onlineday when auto configure)</td>
		</tr>`, n.Name, statusBadge, n.Address)
	}

	domain := "craftstack.internal"
	if mesh != nil {
		domain = mesh.Domain
	}

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">mesh network</h1>

	<!-- mesh status summary -->
	<div class="stats stats-vertical sm:stats-horizontal shadow mb-6 w-full overflow-x-auto">
		<div class="stat">
			<div class="stat-title">mesh status</div>
			<div class="stat-value text-lg"><span class="badge %s">%s</span></div>
		</div>
		<div class="stat">
			<div class="stat-title">WG node</div>
			<div class="stat-value">%d</div>
		</div>
		<div class="stat">
			<div class="stat-title">DNS record</div>
			<div class="stat-value">%d</div>
		</div>
		<div class="stat">
			<div class="stat-title">main</div>
			<div class="stat-value text-lg font-mono">%s</div>
		</div>
	</div>

	<!-- WireGuard node -->
	<div class="card bg-base-100 shadow-xl mb-6">
		<div class="card-body">
			<h2 class="card-title">WireGuard node</h2>
			<p class="text-sm text-gray-500 mb-4">Each node connects via WireGuard tunnel so Docker containers can communicate directly.</p>
			<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				%s
			</div>
		</div>
	</div>

	<!-- wait node -->
	%s

	<!-- DNS record -->
	<div class="card bg-base-100 shadow-xl mb-6">
		<div class="card-body">
			<div class="flex justify-between items-center mb-4">
				<h2 class="card-title">DNS record (*.%s)</h2>
				<button class="btn btn-sm btn-primary" onclick="document.getElementById('add-dns-modal').showModal()">record add</button>
			</div>
			<p class="text-sm text-gray-500 mb-4">from containers <code class="font-mono text-primary">{name}.%s</code> tag as other node service accessible.</p>
			<div class="overflow-x-auto">
				<table class="table table-zebra table-sm">
					<thead><tr><th>FQDN</th><th>IP address</th><th>name</th><th>instance</th><th>node</th><th>manage</th></tr></thead>
					<tbody>%s</tbody>
				</table>
			</div>
		</div>
	</div>

	<!-- usage guide -->
	<div class="card bg-base-100 shadow-xl">
		<div class="card-body">
			<h2 class="card-title">usage</h2>
			<div class="text-sm space-y-2">
				<p>1. when agent registers with master <strong>automatically</strong> create WireGuard key and configure tunnel.</p>
				<p>2. instance createand network connectif done DNS record auto register.</p>
				<p>3. from inside containers <code class="font-mono bg-base-200 px-2 py-1 rounded">ping main-db.%s</code> like other node service accessible.</p>
				<p>4. each node <strong>UDP 51820</strong> port wall from col must exist .</p>
			</div>
		</div>
	</div>

	<!-- DNS add modal -->
	<dialog id="add-dns-modal" class="modal">
		<div class="modal-box max-w-md">
			<form method="dialog"><button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2">X</button></form>
			<h3 class="text-lg font-bold mb-4">DNS record count add</h3>
			<div id="add-dns-result"></div>
			<div class="form-control mb-3">
				<label class="label"><span class="label-text font-semibold">name *</span></label>
				<input id="dns-name" type="text" class="input input-bordered w-full" placeholder="e.g.: main-db">
				<label class="label"><span class="label-text-alt">.%s auto add</span></label>
			</div>
			<div class="form-control mb-3">
				<label class="label"><span class="label-text font-semibold">IP address *</span></label>
				<input id="dns-ip" type="text" class="input input-bordered w-full" placeholder="e.g.: 172.30.2.10">
			</div>
			<div class="modal-action">
				<button type="button" class="btn btn-primary" onclick="addDNSRecord()">add</button>
				<form method="dialog"><button class="btn">cancel</button></form>
			</div>
		</div>
		<form method="dialog" class="modal-backdrop"><button>close</button></form>
	</dialog>

	<script>
	async function checkWGStatus(nodeId) {
		try {
			const resp = await fetch('/api/mesh/nodes/' + nodeId + '/wireguard');
			const data = await resp.json();
			if (data.installed) {
				let msg = 'WireGuard: ' + (data.active ? 'active' : 'inactive');
				if (data.peers && data.peers.length > 0) {
					msg += '\npeer ' + data.peers.length + '';
					data.peers.forEach(function(p) {
						msg += '\n  - ' + p.endpoint + ' (' + (p.connected ? 'connected' : 'notconnect') + ')';
					});
				}
				alert(msg);
			} else {
				alert('WireGuard installis not set');
			}
		} catch(e) { alert('state check failed: ' + e.message); }
	}

	async function addDNSRecord() {
		const name = document.getElementById('dns-name').value.trim();
		const ip = document.getElementById('dns-ip').value.trim();
		const resultDiv = document.getElementById('add-dns-result');
		if (!name || !ip) { resultDiv.innerHTML = '<div class="alert alert-warning text-sm">name IP please enter</div>'; return; }
		try {
			const resp = await fetch('/api/mesh/dns', {
				method: 'POST',
				headers: {'Content-Type': 'application/json'},
				body: JSON.stringify({ name: name, ip_address: ip })
			});
			const data = await resp.json();
			if (data.status === 'success') {
				resultDiv.innerHTML = '<div class="alert alert-success text-sm">' + data.message + '</div>';
				setTimeout(function() { location.reload(); }, 1000);
			} else {
				resultDiv.innerHTML = '<div class="alert alert-error text-sm">' + data.message + '</div>';
			}
		} catch(e) { resultDiv.innerHTML = '<div class="alert alert-error text-sm">request failed: ' + e.message + '</div>'; }
	}

	async function deleteDNS(instanceId) {
		if (!confirm(' DNS record delete?')) return;
		try {
			const resp = await fetch('/api/mesh/dns/' + instanceId, { method: 'DELETE' });
			const data = await resp.json();
			if (data.status === 'success') { showToast(data.message, 'success'); setTimeout(function() { location.reload(); }, 500); }
			else { showToast(data.message, 'error'); }
		} catch(e) { showToast('delete failed: ' + e.message, 'error'); }
	}
	</script>`,
		meshBadge, meshStatus,
		wgNodeCount,
		len(dnsRecords),
		domain,
		nodeCards,
		buildPendingNodesSection(pendingNodes),
		domain, domain,
		dnsRows,
		domain,
		domain)
}

func buildPendingNodesSection(pendingNodes string) string {
	if pendingNodes == "" {
		return ""
	}
	return fmt.Sprintf(`
	<div class="card bg-base-100 shadow-xl mb-6">
		<div class="card-body">
			<h2 class="card-title">WireGuard notsettings node</h2>
			<div class="overflow-x-auto">
				<table class="table table-zebra table-sm">
					<thead><tr><th>node</th><th>address</th><th>state</th></tr></thead>
					<tbody>%s</tbody>
				</table>
			</div>
		</div>
	</div>`, pendingNodes)
}
