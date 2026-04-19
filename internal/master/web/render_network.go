package web

import (
	"fmt"

	"craftstack/internal/master/store"
)

// ─────────────────────────────────────────────────────────────
// network manage page
// ─────────────────────────────────────────────────────────────

type networkWithCount struct {
	*store.Network
	InstanceCount int
	NodeName      string
}

func buildNetworksHTML(data map[string]interface{}, tableOnly bool) string {
	netsI, ok := data["Networks"]
	if !ok || netsI == nil {
		return networksEmptyHTML(data, tableOnly)
	}

	networks, ok := netsI.([]networkWithCount)
	if !ok || len(networks) == 0 {
		return networksEmptyHTML(data, tableOnly)
	}

	var rows string
	for _, n := range networks {
		driverBadge := networkDriverBadge(n.Driver)
		subnet := n.Subnet
		if subnet == "" {
			subnet = "auto"
		}

		rows += fmt.Sprintf(`<tr>
			<td class="font-semibold">%s</td>
			<td class="hidden sm:table-cell">%s</td>
			<td class="hidden sm:table-cell"><code class="text-xs">%s</code></td>
			<td class="hidden md:table-cell"><a href="/nodes/%s" class="link link-hover text-xs">%s</a></td>
			<td class="hidden md:table-cell"><span class="badge badge-sm badge-info">%d</span></td>
			<td class="text-xs hidden lg:table-cell">%s</td>
			<td>
				<div class="flex gap-1">
					<button class="btn btn-xs btn-outline btn-primary whitespace-nowrap" onclick="window._networkMgr.showConnect('%s','%s')">instance connect</button>
					<button class="btn btn-xs btn-outline btn-error whitespace-nowrap" onclick="window._networkMgr.deleteNetwork('%s','%s')">delete</button>
				</div>
			</td>
		</tr>`,
			n.Name, driverBadge, subnet,
			n.NodeID, n.NodeName,
			n.InstanceCount,
			formatTimeAgo(n.CreatedAt),
			n.ID, n.Name,
			n.ID, n.Name)
	}

	table := fmt.Sprintf(`
    <div class="overflow-x-auto">
        <table class="table table-zebra">
            <thead><tr><th>name</th><th class="hidden sm:table-cell">driver</th><th class="hidden sm:table-cell">subnet</th><th class="hidden md:table-cell">node</th><th class="hidden md:table-cell">instance</th><th class="hidden lg:table-cell">createday</th><th>manage</th></tr></thead>
            <tbody>%s</tbody>
        </table>
    </div>`, rows)

	if tableOnly {
		return table
	}

	// online node option + instance option (modal)
	agentOptions := buildAgentOptions(data)
	instanceOptions := buildInstanceOptions(data)

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">network manage</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">Docker network (%d)</h2>
            <button class="btn btn-primary btn-sm" onclick="document.getElementById('create-network-modal').showModal()">new network</button>
        </div>
        <div id="networks-table" hx-get="/htmx/networks-table" hx-trigger="every 15s" hx-swap="innerHTML">
        %s
        </div>
    </div></div>

    <!-- connect instance list (optional network) -->
    <div class="card bg-base-100 shadow-xl mt-6" id="connected-instances-card" style="display:none">
        <div class="card-body">
            <div class="flex justify-between items-center mb-4">
                <h2 class="card-title">connect instance: <span id="ci-network-name" class="text-primary"></span></h2>
                <button class="btn btn-sm btn-ghost" onclick="document.getElementById('connected-instances-card').style.display='none'">close</button>
            </div>
            <div id="connected-instances-list"></div>
        </div>
    </div>

    %s
    %s`,
		len(networks), table,
		buildCreateNetworkModal(agentOptions),
		buildConnectNetworkModal(instanceOptions))
}

func networksEmptyHTML(data map[string]interface{}, tableOnly bool) string {
	table := `
    <div class="overflow-x-auto">
        <table class="table table-zebra">
            <thead><tr><th>name</th><th class="hidden sm:table-cell">driver</th><th class="hidden sm:table-cell">subnet</th><th class="hidden md:table-cell">node</th><th class="hidden md:table-cell">instance</th><th class="hidden lg:table-cell">createday</th><th>manage</th></tr></thead>
            <tbody>
                <tr><td colspan="7" class="text-center text-gray-500">no registered network. "new network" click button to create.</td></tr>
            </tbody>
        </table>
    </div>`
	if tableOnly {
		return table
	}
	agentOptions := buildAgentOptions(data)
	instanceOptions := buildInstanceOptions(data)
	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">network manage</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">Docker network</h2>
            <button class="btn btn-primary btn-sm" onclick="document.getElementById('create-network-modal').showModal()">new network</button>
        </div>
        %s
    </div></div>
    %s
    %s`, table,
		buildCreateNetworkModal(agentOptions),
		buildConnectNetworkModal(instanceOptions))
}

func buildInstanceOptions(data map[string]interface{}) string {
	instsI, ok := data["Instances"]
	if !ok || instsI == nil {
		return ""
	}
	instances, ok := instsI.([]*store.Instance)
	if !ok {
		return ""
	}
	var opts string
	for _, inst := range instances {
		nodeShort := inst.NodeID
		if len(nodeShort) > 8 {
			nodeShort = nodeShort[:8]
		}
		opts += fmt.Sprintf(`<option value="%s" data-node="%s">%s (%s)</option>`, inst.ID, inst.NodeID, inst.Name, nodeShort)
	}
	return opts
}

func networkDriverBadge(driver string) string {
	switch driver {
	case "bridge":
		return `<span class="badge badge-sm badge-primary">bridge</span>`
	case "overlay":
		return `<span class="badge badge-sm badge-secondary">overlay</span>`
	case "host":
		return `<span class="badge badge-sm badge-warning">host</span>`
	default:
		return fmt.Sprintf(`<span class="badge badge-sm">%s</span>`, driver)
	}
}

func buildCreateNetworkModal(agentOptions string) string {
	noAgentMsg := ""
	if agentOptions == "" {
		noAgentMsg = `<div class="alert alert-warning text-sm mb-4">online agent is missing.</div>`
	}

	return fmt.Sprintf(`
<dialog id="create-network-modal" class="modal">
  <div class="modal-box max-w-md">
    <form method="dialog"><button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2">X</button></form>
    <h3 class="text-lg font-bold mb-4">new create network</h3>
    %s
    <div id="create-network-result"></div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">node (agent) *</span></label>
      <select id="cn-node" class="select select-bordered w-full" required>
        <option value="" disabled selected>node optional</option>
        %s
      </select>
    </div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">network name *</span></label>
      <input id="cn-name" type="text" class="input input-bordered w-full" placeholder="e.g.: minecraft-net" required>
    </div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">driver</span></label>
      <select id="cn-driver" class="select select-bordered w-full">
        <option value="bridge" selected>bridge (same server my)</option>
        <option value="host">host (share host network)</option>
      </select>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
      <div class="form-control">
        <label class="label"><span class="label-text font-semibold">subnet</span></label>
        <input id="cn-subnet" type="text" class="input input-bordered w-full" placeholder="auto (e.g.: 172.20.0.0/16)">
      </div>
      <div class="form-control">
        <label class="label"><span class="label-text font-semibold">gateway</span></label>
        <input id="cn-gateway" type="text" class="input input-bordered w-full" placeholder="auto (e.g.: 172.20.0.1)">
      </div>
    </div>

    <div class="modal-action">
      <button type="button" class="btn btn-primary" onclick="window._networkMgr.createNetwork()">create</button>
      <form method="dialog"><button class="btn">cancel</button></form>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop"><button>close</button></form>
</dialog>`, noAgentMsg, agentOptions)
}

func buildConnectNetworkModal(instanceOptions string) string {
	return fmt.Sprintf(`
<dialog id="connect-network-modal" class="modal">
  <div class="modal-box max-w-md">
    <form method="dialog"><button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2">X</button></form>
    <h3 class="text-lg font-bold mb-4">instance network connect</h3>
    <div id="connect-network-result"></div>
    <input type="hidden" id="conn-network-id" />

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">network</span></label>
      <input id="conn-network-name" type="text" class="input input-bordered w-full" disabled>
    </div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">instance *</span></label>
      <select id="conn-instance" class="select select-bordered w-full" required>
        <option value="" disabled selected>instance optional</option>
        %s
      </select>
    </div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">network alias (DNS)</span></label>
      <input id="conn-alias" type="text" class="input input-bordered w-full" placeholder="if empty instance name use">
      <label class="label"><span class="label-text-alt">other from containers  alias as access available</span></label>
    </div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">fixed IP</span></label>
      <input id="conn-ip" type="text" class="input input-bordered w-full" placeholder="auto allocate (e.g.: 172.20.0.10)">
    </div>

    <div class="modal-action">
      <button type="button" class="btn btn-primary" onclick="window._networkMgr.connectInstance()">connect</button>
      <form method="dialog"><button class="btn">cancel</button></form>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop"><button>close</button></form>
</dialog>

<script>
window._networkMgr = {
  async createNetwork() {
    const node = document.getElementById('cn-node').value;
    const name = document.getElementById('cn-name').value.trim();
    const driver = document.getElementById('cn-driver').value;
    const subnet = document.getElementById('cn-subnet').value.trim();
    const gateway = document.getElementById('cn-gateway').value.trim();
    const resultDiv = document.getElementById('create-network-result');

    if (!node) { resultDiv.innerHTML = '<div class="alert alert-warning text-sm">node please select</div>'; return; }
    if (!name) { resultDiv.innerHTML = '<div class="alert alert-warning text-sm">network name please enter</div>'; return; }

    resultDiv.innerHTML = '<div class="flex items-center gap-2"><span class="loading loading-spinner loading-xs"></span> create during...</div>';

    try {
      const resp = await fetch('/api/networks', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ name, driver, subnet, gateway, node_id: node })
      });
      const data = await resp.json();
      if (data.status === 'success') {
        resultDiv.innerHTML = '<div class="alert alert-success text-sm">' + data.message + '</div>';
        setTimeout(() => location.reload(), 1000);
      } else {
        resultDiv.innerHTML = '<div class="alert alert-error text-sm">' + data.message + '</div>';
      }
    } catch(e) {
      resultDiv.innerHTML = '<div class="alert alert-error text-sm">request failed: ' + e.message + '</div>';
    }
  },

  async deleteNetwork(networkId, networkName) {
    if (!confirm('network "' + networkName + '"() delete?')) return;
    try {
      const resp = await fetch('/api/networks/' + networkId, { method: 'DELETE' });
      const data = await resp.json();
      if (data.status === 'success') {
        showToast(data.message, 'success');
        setTimeout(() => location.reload(), 500);
      } else {
        showToast(data.message, 'error');
      }
    } catch(e) { showToast('delete failed: ' + e.message, 'error'); }
  },

  showConnect(networkId, networkName) {
    document.getElementById('conn-network-id').value = networkId;
    document.getElementById('conn-network-name').value = networkName;
    document.getElementById('connect-network-result').innerHTML = '';
    document.getElementById('connect-network-modal').showModal();
  },

  async connectInstance() {
    const networkId = document.getElementById('conn-network-id').value;
    const instanceId = document.getElementById('conn-instance').value;
    const alias = document.getElementById('conn-alias').value.trim();
    const ipAddress = document.getElementById('conn-ip').value.trim();
    const resultDiv = document.getElementById('connect-network-result');

    if (!instanceId) { resultDiv.innerHTML = '<div class="alert alert-warning text-sm">instance please select</div>'; return; }

    resultDiv.innerHTML = '<div class="flex items-center gap-2"><span class="loading loading-spinner loading-xs"></span> connecting...</div>';

    try {
      const resp = await fetch('/api/networks/' + networkId + '/connect', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ instance_id: instanceId, alias, ip_address: ipAddress })
      });
      const data = await resp.json();
      if (data.status === 'success') {
        resultDiv.innerHTML = '<div class="alert alert-success text-sm">' + data.message + '</div>';
        setTimeout(() => location.reload(), 1000);
      } else {
        resultDiv.innerHTML = '<div class="alert alert-error text-sm">' + data.message + '</div>';
      }
    } catch(e) {
      resultDiv.innerHTML = '<div class="alert alert-error text-sm">request failed: ' + e.message + '</div>';
    }
  },

  async disconnectInstance(networkId, instanceId, instanceName) {
    if (!confirm('instance "' + instanceName + '"() network from connect release?')) return;
    try {
      const resp = await fetch('/api/networks/' + networkId + '/disconnect', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ instance_id: instanceId })
      });
      const data = await resp.json();
      if (data.status === 'success') {
        showToast(data.message, 'success');
        setTimeout(() => location.reload(), 500);
      } else {
        showToast(data.message, 'error');
      }
    } catch(e) { showToast('connect release failed: ' + e.message, 'error'); }
  }
};
</script>`, instanceOptions)
}
