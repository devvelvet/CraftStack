package web

import (
	"fmt"
	"strings"

	"craftstack/internal/master/store"
)

func buildInstancesHTML(data map[string]interface{}, tableOnly bool) string {
	instsI, ok := data["Instances"]
	if !ok || instsI == nil {
		return instancesEmptyHTML(data, tableOnly)
	}

	instances, ok := instsI.([]*store.Instance)
	if !ok || len(instances) == 0 {
		return instancesEmptyHTML(data, tableOnly)
	}

	// network info map extract
	nodeWGMap, _ := data["NodeWGMap"].(map[string]string)
	instNetCountMap, _ := data["InstNetCountMap"].(map[string]int)

	var rows string
	for _, inst := range instances {
		badge := statusBadgeHTML(inst.Status)
		memory := fmt.Sprintf("%s / %s", inst.MemoryMin, inst.MemoryMax)
		nodeShort := inst.NodeID
		if len(nodeShort) > 8 {
			nodeShort = nodeShort[:8] + "..."
		}

		// stateper control button
		var controls string
		switch inst.Status {
		case "running":
			controls = fmt.Sprintf(`
				<button class="btn btn-xs btn-warning whitespace-nowrap" onclick="controlInstance('%s','restart')">restart</button>
				<button class="btn btn-xs btn-error whitespace-nowrap" onclick="controlInstance('%s','stop')">stop</button>`,
				inst.ID, inst.ID)
		case "stopped", "crashed":
			controls = fmt.Sprintf(`
				<button class="btn btn-xs btn-success whitespace-nowrap" onclick="controlInstance('%s','start')">start</button>`,
				inst.ID)
		case "starting", "stopping":
			controls = `<span class="loading loading-spinner loading-xs"></span>`
		default:
			controls = fmt.Sprintf(`
				<button class="btn btn-xs btn-success whitespace-nowrap" onclick="controlInstance('%s','start')">start</button>`,
				inst.ID)
		}

		typeBadge := instanceTypeBadge(inst.InstanceType)

		// network column content
		var netCell string
		wgIP := ""
		if nodeWGMap != nil {
			wgIP = nodeWGMap[inst.NodeID]
		}
		netCount := 0
		if instNetCountMap != nil {
			netCount = instNetCountMap[inst.ID]
		}

		if wgIP != "" {
			dnsName := fmt.Sprintf("%s.craftstack.internal", inst.Name)
			netCell = fmt.Sprintf(`<div class="text-xs"><code class="text-emerald-400">%s</code></div><div class="text-xs opacity-60">%s</div>`, wgIP, dnsName)
			if netCount > 0 {
				netCell += fmt.Sprintf(`<span class="badge badge-info badge-xs mt-0.5">%d net</span>`, netCount)
			}
		} else if netCount > 0 {
			netCell = fmt.Sprintf(`<span class="badge badge-info badge-xs">%d net</span>`, netCount)
		} else {
			netCell = `<span class="text-xs text-gray-500">-</span>`
		}

		rows += fmt.Sprintf(`<tr>
			<td>
				<div class="flex items-center gap-2">
					<div class="w-2 h-2 rounded-full %s"></div>
					<a href="/instances/%s" class="link link-hover font-semibold">%s</a>
				</div>
			</td>
			<td>%s</td>
			<td class="hidden md:table-cell"><a href="/nodes/%s" class="link link-hover text-xs opacity-60">%s</a></td>
			<td class="hidden sm:table-cell">%d</td>
			<td>%s</td>
			<td class="text-xs hidden sm:table-cell">%s</td>
			<td class="hidden lg:table-cell">%s</td>
			<td>
				<div class="flex gap-1 flex-wrap">
					%s
					<a href="/instances/%s/console" class="btn btn-xs btn-outline btn-accent whitespace-nowrap">console</a>
					<button class="btn btn-xs btn-outline btn-error whitespace-nowrap" onclick="deleteInstance('%s','%s',false)">delete</button>
				</div>
			</td>
		</tr>`,
			statusDotClass(inst.Status),
			inst.ID, inst.Name,
			typeBadge,
			inst.NodeID, nodeShort, inst.Port, badge, memory,
			netCell,
			controls, inst.ID,
			inst.ID, inst.Name)
	}

	table := fmt.Sprintf(`
    <div class="overflow-x-auto">
        <table class="table table-zebra">
            <thead><tr><th>name</th><th>type</th><th class="hidden md:table-cell">node</th><th class="hidden sm:table-cell">port</th><th>state</th><th class="hidden sm:table-cell">memory</th><th class="hidden lg:table-cell">network</th><th>manage</th></tr></thead>
            <tbody>%s</tbody>
        </table>
    </div>`, rows)

	if tableOnly {
		return table
	}

	// online node list as agent option create
	agentOptions := buildAgentOptions(data)

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">instance management</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">all instance (%d)</h2>
            <button class="btn btn-primary btn-sm" onclick="document.getElementById('create-instance-modal').showModal()">new instance</button>
        </div>
        <div id="instances-table" hx-get="/htmx/instances-table" hx-trigger="every 10s" hx-swap="innerHTML">
        %s
        </div>
    </div></div>
    %s`, len(instances), table, buildCreateInstanceModal(agentOptions))
}

func instancesEmptyHTML(data map[string]interface{}, tableOnly bool) string {
	table := `
    <div class="overflow-x-auto">
        <table class="table table-zebra">
            <thead><tr><th>name</th><th>type</th><th class="hidden md:table-cell">node</th><th class="hidden sm:table-cell">port</th><th>state</th><th class="hidden sm:table-cell">memory</th><th class="hidden lg:table-cell">network</th><th>manage</th></tr></thead>
            <tbody>
                <tr><td colspan="8" class="text-center text-gray-500">settings instance is missing</td></tr>
            </tbody>
        </table>
    </div>`
	if tableOnly {
		return table
	}
	agentOptions := buildAgentOptions(data)
	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">instance management</h1>
    <div class="card bg-base-100 shadow-xl"><div class="card-body">
        <div class="flex justify-between items-center mb-4">
            <h2 class="card-title">all instance</h2>
            <button class="btn btn-primary btn-sm" onclick="document.getElementById('create-instance-modal').showModal()">new instance</button>
        </div>
        %s
    </div></div>
    %s`, table, buildCreateInstanceModal(agentOptions))
}

// buildAgentOptions generates <option> tags for online agents.
func buildAgentOptions(data map[string]interface{}) string {
	nodesI, ok := data["Nodes"]
	if !ok || nodesI == nil {
		return ""
	}
	nodes, ok := nodesI.([]*store.Node)
	if !ok {
		return ""
	}
	var opts string
	for _, n := range nodes {
		if n.Status == "online" {
			opts += fmt.Sprintf(`<option value="%s">%s</option>`, n.ID, n.Name)
		}
	}
	return opts
}

// buildCreateInstanceModal generates the create-instance modal HTML with type-aware dynamic form.
func buildCreateInstanceModal(agentOptions string) string {
	noAgentMsg := ""
	if agentOptions == "" {
		noAgentMsg = `<div class="alert alert-warning text-sm mb-4">online agent is missing. agent first executeplease.</div>`
	}

	return fmt.Sprintf(`
<dialog id="create-instance-modal" class="modal">
  <div class="modal-box max-w-lg" x-data="{
    instType: 'minecraft',
    defaultPorts: { minecraft: 25565, mysql: 3306, postgresql: 5432, mongodb: 27017, redis: 6379, kafka: 9092 },
    defaultImages: { minecraft: 'eclipse-temurin:21-jre', mysql: 'mysql:8.0', postgresql: 'postgres:16', mongodb: 'mongo:7', redis: 'redis:7', kafka: 'apache/kafka:3.7.0' },
    get isMinecraft() { return this.instType === 'minecraft'; },
    get isJavaBased() { return this.instType === 'minecraft' || this.instType === 'kafka'; },
    get defaultPort() { return this.defaultPorts[this.instType] || 25565; },
    get defaultImage() { return this.defaultImages[this.instType] || ''; }
  }">
    <form method="dialog"><button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2">X</button></form>
    <h3 class="text-lg font-bold mb-4">new create instance</h3>
    %s
    <div id="create-instance-result"></div>
    <div id="create-instance-form">

      <!-- instance type optional -->
      <div class="form-control mb-3">
        <label class="label"><span class="label-text font-semibold">instance type *</span></label>
        <select id="ci-type" class="select select-bordered w-full" x-model="instType" x-on:change="document.getElementById('ci-port').value = defaultPort">
          <option value="minecraft">Minecraft</option>
          <option value="mysql">MySQL</option>
          <option value="postgresql">PostgreSQL</option>
          <option value="mongodb">MongoDB</option>
          <option value="redis">Redis</option>
          <option value="kafka">Kafka</option>
        </select>
      </div>

      <!-- Docker image guide -->
      <div class="alert alert-info text-sm mb-3">
        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="stroke-current shrink-0 w-5 h-5"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
        <span>Docker container execute. image: <code class="font-mono text-xs" x-text="defaultImage"></code></span>
      </div>

      <div class="form-control mb-3">
        <label class="label"><span class="label-text font-semibold">agent (node) *</span></label>
        <select id="ci-agent" class="select select-bordered w-full" required>
          <option value="" disabled selected>agent optional</option>
          %s
        </select>
      </div>
      <div class="form-control mb-3">
        <label class="label"><span class="label-text font-semibold">instance name *</span></label>
        <input id="ci-name" type="text" class="input input-bordered w-full" placeholder="e.g.: survival" required>
        <label class="label"><span class="label-text-alt">English, number, hyphen only recommended</span></label>
      </div>

      <!-- Java version (Java based type) -->
      <template x-if="isJavaBased">
        <div class="form-control mb-3">
          <label class="label"><span class="label-text font-semibold">Java version</span></label>
          <select id="ci-java-version" class="select select-bordered w-full">
            <option value="21" selected>Java 21 (recommended)</option>
            <option value="17">Java 17</option>
            <option value="25">Java 25</option>
          </select>
        </div>
      </template>

      <!-- service version (middleware type) -->
      <template x-if="!isMinecraft">
        <div class="form-control mb-3">
          <label class="label"><span class="label-text font-semibold">service version</span></label>
          <input id="ci-service-version" type="text" class="input input-bordered w-full" placeholder="e.g.: 8.0.35, 7.0.15">
        </div>
      </template>

      <!-- Minecraft: JAR or ZIP upload -->
      <template x-if="isMinecraft">
        <div class="mb-3" x-data="{uploadMode: 'jar'}">
          <label class="label"><span class="label-text font-semibold">server file upload *</span></label>
          <div class="flex gap-2 mb-2">
            <button type="button" class="btn btn-xs" :class="uploadMode === 'jar' ? 'btn-primary' : 'btn-ghost'" @click="uploadMode = 'jar'">JAR file</button>
            <button type="button" class="btn btn-xs" :class="uploadMode === 'zip' ? 'btn-primary' : 'btn-ghost'" @click="uploadMode = 'zip'">existing server ZIP</button>
          </div>
          <template x-if="uploadMode === 'jar'">
            <div class="form-control">
              <input id="ci-jar" type="file" class="file-input file-input-bordered w-full" accept=".jar">
              <label class="label"><span class="label-text-alt">Paper, Spigot etc. server JAR files</span></label>
            </div>
          </template>
          <template x-if="uploadMode === 'zip'">
            <div class="form-control">
              <input id="ci-zip" type="file" class="file-input file-input-bordered w-full" accept=".zip">
              <label class="label"><span class="label-text-alt">existing server folder ZIP as compress upload (JAR + world + config file include)</span></label>
              <div class="alert alert-info text-xs mt-1 py-2">
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="stroke-current shrink-0 w-4 h-4"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                <span>Auto-detect JAR from ZIP. Compress the entire server folder or just contents.</span>
              </div>
            </div>
          </template>
          <input type="hidden" id="ci-upload-mode" x-bind:value="uploadMode">
        </div>
      </template>

      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 mb-3">
        <div class="form-control">
          <label class="label"><span class="label-text font-semibold">port</span></label>
          <input id="ci-port" type="number" class="input input-bordered w-full" x-bind:value="defaultPort" min="1" max="65535">
        </div>
        <template x-if="isJavaBased">
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">min memory</span></label>
            <input id="ci-mem-min" type="text" class="input input-bordered w-full" value="512M" placeholder="512M">
          </div>
        </template>
        <template x-if="isJavaBased">
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">max memory</span></label>
            <input id="ci-mem-max" type="text" class="input input-bordered w-full" value="2G" placeholder="2G">
          </div>
        </template>
      </div>

      <!-- Docker resource limit -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
        <div class="form-control">
          <label class="label"><span class="label-text font-semibold">Docker memory limit</span></label>
          <input id="ci-docker-memory" type="text" class="input input-bordered w-full" placeholder="e.g.: 3G (if empty JVM memory 1.5x)">
          <label class="label"><span class="label-text-alt">container all memory upper (JVM heap + metaspace + store)</span></label>
        </div>
        <div class="form-control">
          <label class="label"><span class="label-text font-semibold">Docker CPU limit</span></label>
          <input id="ci-docker-cpus" type="text" class="input input-bordered w-full" placeholder="e.g.: 2.0 (if empty, no limit)">
          <label class="label"><span class="label-text-alt">usable CPU core count (decimal allowed)</span></label>
        </div>
      </div>

      <!-- Java based: JVM flag -->
      <template x-if="isJavaBased">
        <div class="form-control mb-3">
          <label class="label"><span class="label-text font-semibold">JVM flag</span></label>
          <textarea id="ci-jvm-args" class="textarea textarea-bordered w-full font-mono text-xs" rows="3" placeholder="-XX:+UseG1GC&#10;-XX:+ParallelRefProcEnabled&#10;-XX:MaxGCPauseMillis=200"></textarea>
          <label class="label"><span class="label-text-alt">one per line (e.g.: -XX:+UseG1GC)</span></label>
        </div>
      </template>

      <!-- MySQL settings -->
      <template x-if="instType === 'mysql'">
        <div class="space-y-3 mb-3">
          <div class="divider text-xs">MySQL settings</div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">Root password</span></label>
            <input id="ci-mysql-root-password" type="password" class="input input-bordered w-full" placeholder="root password">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">data directory</span></label>
            <input id="ci-mysql-data-dir" type="text" class="input input-bordered w-full" placeholder="default use">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">add argument</span></label>
            <input id="ci-mysql-extra-args" type="text" class="input input-bordered w-full" placeholder="e.g.: --character-set-server=utf8mb4">
          </div>
        </div>
      </template>

      <!-- PostgreSQL settings -->
      <template x-if="instType === 'postgresql'">
        <div class="space-y-3 mb-3">
          <div class="divider text-xs">PostgreSQL settings</div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">admin password</span></label>
            <input id="ci-pg-password" type="password" class="input input-bordered w-full" placeholder="postgres password">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">data directory</span></label>
            <input id="ci-pg-data-dir" type="text" class="input input-bordered w-full" placeholder="default use">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">add argument</span></label>
            <input id="ci-pg-extra-args" type="text" class="input input-bordered w-full" placeholder="e.g.: -c shared_buffers=256MB">
          </div>
        </div>
      </template>

      <!-- MongoDB settings -->
      <template x-if="instType === 'mongodb'">
        <div class="space-y-3 mb-3">
          <div class="divider text-xs">MongoDB settings</div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">admin user</span></label>
            <input id="ci-mongo-admin-user" type="text" class="input input-bordered w-full" placeholder="admin">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">admin password</span></label>
            <input id="ci-mongo-admin-password" type="password" class="input input-bordered w-full" placeholder="password">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">data directory</span></label>
            <input id="ci-mongo-data-dir" type="text" class="input input-bordered w-full" placeholder="default use">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">add argument</span></label>
            <input id="ci-mongo-extra-args" type="text" class="input input-bordered w-full" placeholder="e.g.: --wiredTigerCacheSizeGB 1">
          </div>
        </div>
      </template>

      <!-- Redis settings -->
      <template x-if="instType === 'redis'">
        <div class="space-y-3 mb-3">
          <div class="divider text-xs">Redis settings</div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">password</span></label>
            <input id="ci-redis-password" type="password" class="input input-bordered w-full" placeholder="password (optional)">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">data directory</span></label>
            <input id="ci-redis-data-dir" type="text" class="input input-bordered w-full" placeholder="default use">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">add argument</span></label>
            <input id="ci-redis-extra-args" type="text" class="input input-bordered w-full" placeholder="e.g.: --maxmemory 256mb">
          </div>
        </div>
      </template>

      <!-- Kafka settings -->
      <template x-if="instType === 'kafka'">
        <div class="space-y-3 mb-3">
          <div class="divider text-xs">Kafka settings</div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">Broker ID</span></label>
            <input id="ci-kafka-broker-id" type="number" class="input input-bordered w-full" value="0" min="0">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">data directory</span></label>
            <input id="ci-kafka-data-dir" type="text" class="input input-bordered w-full" placeholder="default use">
          </div>
          <div class="form-control">
            <label class="label"><span class="label-text font-semibold">add argument</span></label>
            <input id="ci-kafka-extra-args" type="text" class="input input-bordered w-full" placeholder="add settings">
          </div>
        </div>
      </template>

      <!-- Docker customization (fold/expand) -->
      <div class="collapse collapse-arrow bg-base-200 mb-4" x-data="{open: false}">
        <input type="checkbox" x-model="open">
        <div class="collapse-title text-sm font-semibold">Docker customization (optional)</div>
        <div class="collapse-content">
          <div class="form-control mb-3">
            <label class="label"><span class="label-text">custom Dockerfile</span></label>
            <textarea id="ci-custom-dockerfile" class="textarea textarea-bordered w-full font-mono text-xs" rows="6" placeholder="FROM eclipse-temurin:21-jre&#10;COPY . /server&#10;WORKDIR /server&#10;CMD [&quot;java&quot;, &quot;-jar&quot;, &quot;server.jar&quot;, &quot;nogui&quot;]"></textarea>
            <label class="label"><span class="label-text-alt">if empty default image use</span></label>
          </div>
          <div class="form-control mb-3">
            <label class="label"><span class="label-text">custom docker-compose.yml</span></label>
            <textarea id="ci-custom-compose" class="textarea textarea-bordered w-full font-mono text-xs" rows="8" placeholder="version: '3.8'&#10;services:&#10;  app:&#10;    image: eclipse-temurin:21-jre&#10;    ports:&#10;      - '25565:25565'&#10;    volumes:&#10;      - ./:/server"></textarea>
            <label class="label"><span class="label-text-alt">if empty docker-compose use </span></label>
          </div>
        </div>
      </div>

      <div class="flex flex-wrap gap-4 mb-4">
        <label class="label cursor-pointer gap-2">
          <input id="ci-auto-start" type="checkbox" class="checkbox checkbox-sm" checked>
          <span class="label-text">auto-start</span>
        </label>
        <label class="label cursor-pointer gap-2">
          <input id="ci-auto-restart" type="checkbox" class="checkbox checkbox-sm" checked>
          <span class="label-text">auto-restart</span>
        </label>
        <label class="label cursor-pointer gap-2">
          <input id="ci-start-now" type="checkbox" class="checkbox checkbox-sm" checked>
          <span class="label-text">create after immediately start</span>
        </label>
      </div>
      <div class="modal-action">
        <button type="button" class="btn btn-primary" onclick="window._createInstance()">
          <span id="ci-btn-text">create instance</span>
          <span id="ci-btn-loading" class="loading loading-spinner loading-xs hidden"></span>
        </button>
        <form method="dialog"><button class="btn">cancel</button></form>
      </div>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop"><button>close</button></form>
</dialog>
<script>
window._createInstance = async function() {
  const instType = document.getElementById('ci-type').value;
  const agent = document.getElementById('ci-agent').value;
  const name = document.getElementById('ci-name').value.trim();
  const port = document.getElementById('ci-port').value;
  const resultDiv = document.getElementById('create-instance-result');
  const btnText = document.getElementById('ci-btn-text');
  const btnLoading = document.getElementById('ci-btn-loading');

  if (!agent) { resultDiv.innerHTML = '<div class="alert alert-warning text-sm">agent please select</div>'; return; }
  if (!name) { resultDiv.innerHTML = '<div class="alert alert-warning text-sm">instance name please enter</div>'; return; }

  const formData = new FormData();
  formData.append('agent_id', agent);
  formData.append('name', name);
  formData.append('instance_type', instType);
  formData.append('port', port);

  const isMinecraft = instType === 'minecraft';
  const isJavaBased = instType === 'minecraft' || instType === 'kafka';

  // Minecraft: JAR or ZIP required
  if (isMinecraft) {
    const uploadMode = (document.getElementById('ci-upload-mode') || {}).value || 'jar';
    if (uploadMode === 'zip') {
      const zipInput = document.getElementById('ci-zip');
      if (!zipInput || !zipInput.files || !zipInput.files[0]) {
        resultDiv.innerHTML = '<div class="alert alert-warning text-sm">server ZIP file please select</div>';
        return;
      }
      formData.append('server_zip', zipInput.files[0]);
    } else {
      const jarInput = document.getElementById('ci-jar');
      if (!jarInput || !jarInput.files || !jarInput.files[0]) {
        resultDiv.innerHTML = '<div class="alert alert-warning text-sm">JAR file please select</div>';
        return;
      }
      formData.append('server_jar', jarInput.files[0]);
    }
  }

  // Java based: memory, JVM flag, Java version
  if (isJavaBased) {
    const memMinEl = document.getElementById('ci-mem-min');
    const memMaxEl = document.getElementById('ci-mem-max');
    const jvmArgsEl = document.getElementById('ci-jvm-args');
    const javaVerEl = document.getElementById('ci-java-version');
    if (memMinEl) formData.append('memory_min', memMinEl.value.trim());
    if (memMaxEl) formData.append('memory_max', memMaxEl.value.trim());
    if (jvmArgsEl) formData.append('jvm_args', jvmArgsEl.value.trim());
    if (javaVerEl) formData.append('java_version', javaVerEl.value);
  }

  // Docker resource limit
  const dockerMemEl = document.getElementById('ci-docker-memory');
  const dockerCpuEl = document.getElementById('ci-docker-cpus');
  if (dockerMemEl && dockerMemEl.value.trim()) formData.append('docker_memory', dockerMemEl.value.trim());
  if (dockerCpuEl && dockerCpuEl.value.trim()) formData.append('docker_cpus', dockerCpuEl.value.trim());

  // Docker customization
  const dockerfileEl = document.getElementById('ci-custom-dockerfile');
  const composeEl = document.getElementById('ci-custom-compose');
  if (dockerfileEl && dockerfileEl.value.trim()) formData.append('custom_dockerfile', dockerfileEl.value.trim());
  if (composeEl && composeEl.value.trim()) formData.append('custom_compose', composeEl.value.trim());

  // service version
  if (!isMinecraft) {
    const svEl = document.getElementById('ci-service-version');
    if (svEl) formData.append('service_version', svEl.value.trim());
  }

  // typeper field
  if (instType === 'mysql') {
    const el1 = document.getElementById('ci-mysql-root-password');
    const el2 = document.getElementById('ci-mysql-data-dir');
    const el3 = document.getElementById('ci-mysql-extra-args');
    if (el1) formData.append('mysql_root_password', el1.value);
    if (el2) formData.append('mysql_data_dir', el2.value);
    if (el3) formData.append('mysql_extra_args', el3.value);
  } else if (instType === 'postgresql') {
    const el1 = document.getElementById('ci-pg-password');
    const el2 = document.getElementById('ci-pg-data-dir');
    const el3 = document.getElementById('ci-pg-extra-args');
    if (el1) formData.append('pg_password', el1.value);
    if (el2) formData.append('pg_data_dir', el2.value);
    if (el3) formData.append('pg_extra_args', el3.value);
  } else if (instType === 'mongodb') {
    const el1 = document.getElementById('ci-mongo-admin-user');
    const el2 = document.getElementById('ci-mongo-admin-password');
    const el3 = document.getElementById('ci-mongo-data-dir');
    const el4 = document.getElementById('ci-mongo-extra-args');
    if (el1) formData.append('mongo_admin_user', el1.value);
    if (el2) formData.append('mongo_admin_password', el2.value);
    if (el3) formData.append('mongo_data_dir', el3.value);
    if (el4) formData.append('mongo_extra_args', el4.value);
  } else if (instType === 'redis') {
    const el1 = document.getElementById('ci-redis-password');
    const el2 = document.getElementById('ci-redis-data-dir');
    const el3 = document.getElementById('ci-redis-extra-args');
    if (el1) formData.append('redis_password', el1.value);
    if (el2) formData.append('redis_data_dir', el2.value);
    if (el3) formData.append('redis_extra_args', el3.value);
  } else if (instType === 'kafka') {
    const el1 = document.getElementById('ci-kafka-broker-id');
    const el2 = document.getElementById('ci-kafka-data-dir');
    const el3 = document.getElementById('ci-kafka-extra-args');
    if (el1) formData.append('kafka_broker_id', el1.value);
    if (el2) formData.append('kafka_data_dir', el2.value);
    if (el3) formData.append('kafka_extra_args', el3.value);
  }

  const autoStart = document.getElementById('ci-auto-start').checked;
  const autoRestart = document.getElementById('ci-auto-restart').checked;
  formData.append('auto_start', autoStart ? 'true' : 'false');
  formData.append('auto_restart', autoRestart ? 'true' : 'false');

  const startNowEl = document.getElementById('ci-start-now');
  if (startNowEl) formData.append('start_after_create', startNowEl.checked ? 'true' : 'false');

  btnText.textContent = 'create during...';
  btnLoading.classList.remove('hidden');
  resultDiv.innerHTML = '';

  try {
    const resp = await fetch('/api/instances', { method: 'POST', body: formData });
    const data = await resp.json();
    if (data.status === 'success') {
      resultDiv.innerHTML = '<div class="alert alert-success text-sm">' + data.message + '</div>';
      setTimeout(() => { location.reload(); }, 1500);
    } else {
      resultDiv.innerHTML = '<div class="alert alert-error text-sm">' + data.message + '</div>';
    }
  } catch (e) {
    resultDiv.innerHTML = '<div class="alert alert-error text-sm">request failed: ' + e.message + '</div>';
  } finally {
    btnText.textContent = 'create instance';
    btnLoading.classList.add('hidden');
  }
};
</script>
`, noAgentMsg, agentOptions)
}

// ─────────────────────────────────────────────────────────────
// instance detail
// ─────────────────────────────────────────────────────────────

func buildInstanceDetailHTML(data map[string]interface{}) string {
	instI, _ := data["Instance"]
	inst, _ := instI.(*store.Instance)
	if inst == nil {
		return `<div class="alert alert-error">instance not found</div>`
	}

	badge := statusBadgeHTML(inst.Status)
	memory := fmt.Sprintf("%s / %s", inst.MemoryMin, inst.MemoryMax)

	pidStr := "-"
	if inst.PID != nil {
		pidStr = fmt.Sprintf("%d", *inst.PID)
	}

	nodeShort := inst.NodeID
	if len(nodeShort) > 12 {
		nodeShort = nodeShort[:12] + "..."
	}

	// stateper control button render
	controls := buildInstanceControls(inst.ID, inst.Status)

	// backup list
	backupsHTML := buildBackupListHTML(data)

	// backup schedule state x
	backupScheduleBadge := `<span class="badge badge-ghost badge-sm">auto backup off</span>`
	if inst.BackupEnabled && inst.BackupCron != "" {
		backupScheduleBadge = fmt.Sprintf(`<span class="badge badge-success badge-sm">auto: %s</span>`, inst.BackupCron)
	}

	typeBadge := instanceTypeBadge(inst.InstanceType)

	// network info configuration
	networkInfoHTML := buildInstanceNetworkInfoHTML(data)

	// DB typeday when database manage link add
	dbManageLink := ""
	switch inst.InstanceType {
	case "mysql", "postgresql", "mongodb", "redis":
		dbManageLink = fmt.Sprintf(`<a href="/instances/%s/database" class="btn btn-outline btn-secondary w-full mt-2">database manage</a>`, inst.ID)
	}

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">instance: %s %s</h1>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <!-- server info -->
        <div class="lg:col-span-2 card bg-base-100 shadow-xl">
            <div class="card-body">
                <h2 class="card-title">server info</h2>
                <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 mt-4">
                    <div><span class="text-xs text-gray-500">name</span><div class="font-semibold">%s</div></div>
                    <div><span class="text-xs text-gray-500">type</span><div>%s</div></div>
                    <div><span class="text-xs text-gray-500">state</span><div id="inst-status">%s</div></div>
                    <div><span class="text-xs text-gray-500">port</span><div>%d</div></div>
                    <div><span class="text-xs text-gray-500">memory</span><div>%s</div></div>
                    <div><span class="text-xs text-gray-500">PID</span><div><code>%s</code></div></div>
                    <div><span class="text-xs text-gray-500">JAR</span><div><code class="text-xs">%s</code></div></div>
                    <div><span class="text-xs text-gray-500">Java</span><div><code class="text-xs">%s</code></div></div>
                    <div><span class="text-xs text-gray-500">work directory</span><div><code class="text-xs">%s</code></div></div>
                    <div><span class="text-xs text-gray-500">node</span><div><a href="/nodes/%s" class="link link-hover text-sm">%s</a></div></div>
                </div>
                %s
            </div>
        </div>

        <!-- control panel -->
        <div class="card bg-base-100 shadow-xl">
            <div class="card-body">
                <h2 class="card-title">control</h2>
                <div id="instance-controls" class="flex flex-col gap-2 mt-2"
                     hx-get="/htmx/instance-status/%s" hx-trigger="every 5s" hx-swap="innerHTML">
                    %s
                </div>
                <div class="divider"></div>
                <a href="/instances/%s/console" class="btn btn-outline btn-accent w-full">console open</a>
                <a href="/instances/%s/files" class="btn btn-outline btn-info w-full mt-2">file manage</a>
                %s
                <button class="btn btn-outline btn-warning w-full mt-2" onclick="document.getElementById('edit-instance-modal').showModal(); window._editInstance.load('%s')">settings modify</button>
                <div class="divider"></div>
                <button class="btn btn-error btn-outline w-full" onclick="deleteInstance('%s','%s',false)">delete instance</button>
                <button class="btn btn-error btn-outline btn-xs w-full mt-1" onclick="deleteInstance('%s','%s',true)">data include delete</button>
            </div>
        </div>
    </div>

    <!-- container resource metrics -->
    <div id="instance-metrics-section" class="card bg-base-100 shadow-xl mt-6">
        <div class="card-body">
            <h2 class="card-title">container resource monitoring</h2>
            <div id="metrics-offline-msg" class="hidden alert alert-warning text-sm mt-2">the instance offline. metrics collectcannot.</div>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
                <div><canvas id="chart-cpu" height="300"></canvas></div>
                <div><canvas id="chart-memory" height="300"></canvas></div>
                <div><canvas id="chart-network" height="300"></canvas></div>
                <div><canvas id="chart-disk" height="300"></canvas></div>
            </div>
        </div>
    </div>
    <script>
    (function() {
        const instanceId = '%s';
        let cpuChart, memChart, netChart, diskChart;

        const chartOpts = (title, yLabel) => ({
            responsive: true,
            animation: false,
            plugins: {
                legend: { labels: { color: '#a6adbb' } },
                title: { display: true, text: title, color: '#a6adbb' }
            },
            scales: {
                x: { display: true, ticks: { color: '#a6adbb', maxTicksLimit: 8 }, grid: { color: 'rgba(166,173,187,0.1)' } },
                y: { display: true, title: { display: true, text: yLabel, color: '#a6adbb' }, ticks: { color: '#a6adbb' }, grid: { color: 'rgba(166,173,187,0.1)' }, beginAtZero: true }
            }
        });

        function initCharts() {
            const ctx1 = document.getElementById('chart-cpu');
            const ctx2 = document.getElementById('chart-memory');
            const ctx3 = document.getElementById('chart-network');
            const ctx4 = document.getElementById('chart-disk');
            if (!ctx1) return;

            cpuChart = new Chart(ctx1, {
                type: 'line',
                data: { labels: [], datasets: [{ label: 'CPU (%%)', data: [], borderColor: '#36a2eb', backgroundColor: 'rgba(54,162,235,0.1)', fill: true, tension: 0.3, pointRadius: 0 }] },
                options: chartOpts('CPU use', '%%')
            });

            memChart = new Chart(ctx2, {
                type: 'line',
                data: { labels: [], datasets: [
                    { label: 'use (MB)', data: [], borderColor: '#ff6384', backgroundColor: 'rgba(255,99,132,0.1)', fill: true, tension: 0.3, pointRadius: 0 },
                    { label: 'limit (MB)', data: [], borderColor: '#ff6384', borderDash: [5,5], backgroundColor: 'transparent', tension: 0, pointRadius: 0 }
                ] },
                options: chartOpts('memory usage', 'MB')
            });

            netChart = new Chart(ctx3, {
                type: 'line',
                data: { labels: [], datasets: [
                    { label: 'receive (bytes)', data: [], borderColor: '#4bc0c0', backgroundColor: 'rgba(75,192,192,0.1)', fill: true, tension: 0.3, pointRadius: 0 },
                    { label: 'send (bytes)', data: [], borderColor: '#ff9f40', backgroundColor: 'rgba(255,159,64,0.1)', fill: true, tension: 0.3, pointRadius: 0 }
                ] },
                options: chartOpts('network I/O', 'bytes')
            });

            diskChart = new Chart(ctx4, {
                type: 'line',
                data: { labels: [], datasets: [
                    { label: 'read (bytes)', data: [], borderColor: '#9966ff', backgroundColor: 'rgba(153,102,255,0.1)', fill: true, tension: 0.3, pointRadius: 0 },
                    { label: 'write (bytes)', data: [], borderColor: '#ffcd56', backgroundColor: 'rgba(255,205,86,0.1)', fill: true, tension: 0.3, pointRadius: 0 }
                ] },
                options: chartOpts('disk I/O', 'bytes')
            });
        }

        function updateCharts(data) {
            const offlineMsg = document.getElementById('metrics-offline-msg');
            if (!data || (!data.current && (!data.history || data.history.length === 0))) {
                if (offlineMsg) offlineMsg.classList.remove('hidden');
                return;
            }
            if (offlineMsg) offlineMsg.classList.add('hidden');

            const history = data.history || [];
            const labels = history.map(function(h) {
                const d = new Date(h.recorded_at);
                return d.toLocaleTimeString('ko-KR', {hour:'2-digit', minute:'2-digit', second:'2-digit'});
            });

            if (cpuChart) {
                cpuChart.data.labels = labels;
                cpuChart.data.datasets[0].data = history.map(function(h) { return h.cpu_percent; });
                cpuChart.update();
            }
            if (memChart) {
                memChart.data.labels = labels;
                memChart.data.datasets[0].data = history.map(function(h) { return h.memory_used_mb; });
                memChart.data.datasets[1].data = history.map(function(h) { return h.memory_limit_mb; });
                memChart.update();
            }
            if (netChart) {
                netChart.data.labels = labels;
                netChart.data.datasets[0].data = history.map(function(h) { return h.net_rx_bytes; });
                netChart.data.datasets[1].data = history.map(function(h) { return h.net_tx_bytes; });
                netChart.update();
            }
            if (diskChart) {
                diskChart.data.labels = labels;
                diskChart.data.datasets[0].data = history.map(function(h) { return h.block_read_bytes; });
                diskChart.data.datasets[1].data = history.map(function(h) { return h.block_write_bytes; });
                diskChart.update();
            }
        }

        function fetchMetrics() {
            fetch('/api/instances/' + instanceId + '/metrics')
                .then(function(r) { return r.json(); })
                .then(function(data) { updateCharts(data); })
                .catch(function(e) { console.warn('metrics query failed:', e); });
        }

        initCharts();
        fetchMetrics();
        setInterval(fetchMetrics, 5000);
    })();
    </script>

    <!-- backup history -->
    <div class="card bg-base-100 shadow-xl mt-6">
        <div class="card-body">
            <div class="flex justify-between items-center">
                <div class="flex items-center gap-3">
                    <h2 class="card-title">backup history</h2>
                    %s
                </div>
                <button class="btn btn-sm btn-primary" onclick="createBackup('%s')">create backup</button>
            </div>
            <div id="backup-list" class="mt-4">
                %s
            </div>
        </div>
    </div>

    %s`,
		inst.Name, typeBadge,
		inst.Name, typeBadge, badge, inst.Port, memory, pidStr,
		inst.ServerJar, inst.JavaPath, inst.WorkDir,
		inst.NodeID, nodeShort,
		networkInfoHTML,
		inst.ID, controls,
		inst.ID, inst.ID,
		dbManageLink,
		inst.ID,
		inst.ID, inst.Name,
		inst.ID, inst.Name,
		inst.ID, // for metrics JS instanceId
		backupScheduleBadge, inst.ID, backupsHTML,
		buildEditInstanceModal(inst))
}

// buildInstanceNetworkInfoHTML generates the network info section for instance detail.
func buildInstanceNetworkInfoHTML(data map[string]interface{}) string {
	nodeWGAddress, _ := data["NodeWGAddress"].(string)
	nodeDockerSubnet, _ := data["NodeDockerSubnet"].(string)
	instNetworks, _ := data["InstanceNetworks"].([]*store.InstanceNetwork)

	// WG/Docker info without network if absent display no
	if nodeWGAddress == "" && nodeDockerSubnet == "" && len(instNetworks) == 0 {
		return ""
	}

	var html strings.Builder
	html.WriteString(`<div class="divider text-xs mt-4 mb-2">network info</div>`)
	html.WriteString(`<div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4">`)

	// WireGuard IP (CIDR  from IPonly extract)
	if nodeWGAddress != "" {
		wgIP := nodeWGAddress
		if idx := strings.Index(nodeWGAddress, "/"); idx > 0 {
			wgIP = nodeWGAddress[:idx]
		}
		html.WriteString(fmt.Sprintf(`<div><span class="text-xs text-gray-500">WireGuard IP</span><div><code class="text-emerald-400">%s</code></div></div>`, wgIP))
	}

	// Docker subnet
	if nodeDockerSubnet != "" {
		html.WriteString(fmt.Sprintf(`<div><span class="text-xs text-gray-500">Docker subnet</span><div><code class="text-cyan-400">%s</code></div></div>`, nodeDockerSubnet))
	}

	// connect network count
	if len(instNetworks) > 0 {
		html.WriteString(fmt.Sprintf(`<div><span class="text-xs text-gray-500">connect network</span><div><span class="badge badge-info badge-sm">%d</span></div></div>`, len(instNetworks)))
	}

	html.WriteString(`</div>`)

	// network detail table
	if len(instNetworks) > 0 {
		html.WriteString(`<div class="overflow-x-auto mt-3"><table class="table table-xs"><thead><tr>`)
		html.WriteString(`<th>network</th><th>alias</th><th>IP address</th>`)
		html.WriteString(`</tr></thead><tbody>`)
		for _, in := range instNetworks {
			netName := in.NetworkID
			// NetworkID from nodeID prefix remove network name display
			if idx := strings.Index(in.NetworkID, "-"); idx > 0 {
				netName = in.NetworkID[idx+1:]
			}
			alias := in.Alias
			if alias == "" {
				alias = "-"
			}
			ip := in.IPAddress
			if ip == "" {
				ip = `<span class="text-gray-500">auto</span>`
			} else {
				ip = fmt.Sprintf(`<code class="text-emerald-400">%s</code>`, ip)
			}
			html.WriteString(fmt.Sprintf(`<tr><td><code class="text-xs">%s</code></td><td>%s</td><td>%s</td></tr>`, netName, alias, ip))
		}
		html.WriteString(`</tbody></table></div>`)
	}

	// mesh DNS main name display
	inst, _ := data["Instance"].(*store.Instance)
	if inst != nil && nodeWGAddress != "" {
		dnsName := fmt.Sprintf("%s.craftstack.internal", inst.Name)
		html.WriteString(fmt.Sprintf(`<div class="mt-2"><span class="text-xs text-gray-500">mesh DNS</span><div><code class="text-purple-400 text-sm">%s</code></div></div>`, dnsName))
	}

	return html.String()
}

// buildEditInstanceModal generates the edit-instance modal HTML.
func buildEditInstanceModal(inst *store.Instance) string {
	isMinecraft := inst.IsMinecraft()
	isJavaBased := inst.IsJavaBased()

	// typeper settings field create
	var typeFields string
	switch inst.InstanceType {
	case store.InstanceTypeMySQL:
		typeFields = fmt.Sprintf(`
		<div class="divider text-xs">MySQL settings</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">Root password</span></label>
			<input id="ei-mysql-root-password" type="password" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">add argument</span></label>
			<input id="ei-mysql-extra-args" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>`, inst.MySQLRootPassword, inst.MySQLExtraArgs)
	case store.InstanceTypePostgreSQL:
		typeFields = fmt.Sprintf(`
		<div class="divider text-xs">PostgreSQL settings</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">admin password</span></label>
			<input id="ei-pg-password" type="password" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">add argument</span></label>
			<input id="ei-pg-extra-args" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>`, inst.PGPassword, inst.PGExtraArgs)
	case store.InstanceTypeMongoDB:
		typeFields = fmt.Sprintf(`
		<div class="divider text-xs">MongoDB settings</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">admin user</span></label>
			<input id="ei-mongo-admin-user" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">admin password</span></label>
			<input id="ei-mongo-admin-password" type="password" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">add argument</span></label>
			<input id="ei-mongo-extra-args" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>`, inst.MongoAdminUser, inst.MongoAdminPassword, inst.MongoExtraArgs)
	case store.InstanceTypeRedis:
		typeFields = fmt.Sprintf(`
		<div class="divider text-xs">Redis settings</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">password</span></label>
			<input id="ei-redis-password" type="password" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">add argument</span></label>
			<input id="ei-redis-extra-args" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>`, inst.RedisPassword, inst.RedisExtraArgs)
	case store.InstanceTypeKafka:
		typeFields = fmt.Sprintf(`
		<div class="divider text-xs">Kafka settings</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">Broker ID</span></label>
			<input id="ei-kafka-broker-id" type="number" class="input input-bordered input-sm w-full" value="%d">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">add argument</span></label>
			<input id="ei-kafka-extra-args" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>`, inst.KafkaBrokerID, inst.KafkaExtraArgs)
	}

	// memory field (Java basedonly)
	memoryFields := ""
	if isJavaBased {
		memoryFields = fmt.Sprintf(`
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">min memory</span></label>
			<input id="ei-mem-min" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">max memory</span></label>
			<input id="ei-mem-max" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">JVM flag</span></label>
			<textarea id="ei-jvm-args" class="textarea textarea-bordered w-full font-mono text-xs" rows="3">%s</textarea>
		</div>`, inst.MemoryMin, inst.MemoryMax, inst.JVMArgs)
	}

	// service version (middlewareonly)
	versionField := ""
	if !isMinecraft {
		versionField = fmt.Sprintf(`
		<div class="form-control mb-3">
			<label class="label"><span class="label-text">service version</span></label>
			<input id="ei-service-version" type="text" class="input input-bordered input-sm w-full" value="%s">
		</div>`, inst.ServiceVersion)
	}

	autoStartChecked := ""
	if inst.AutoStart {
		autoStartChecked = "checked"
	}
	autoRestartChecked := ""
	if inst.AutoRestart {
		autoRestartChecked = "checked"
	}
	backupEnabledChecked := ""
	if inst.BackupEnabled {
		backupEnabledChecked = "checked"
	}

	jv17Selected, jv21Selected, jv25Selected := "", "", ""
	switch inst.JavaVersion {
	case "17":
		jv17Selected = "selected"
	case "21":
		jv21Selected = "selected"
	case "25":
		jv25Selected = "selected"
	}

	_ = isMinecraft // used above conditionally

	return fmt.Sprintf(`
<dialog id="edit-instance-modal" class="modal">
  <div class="modal-box max-w-lg">
    <form method="dialog"><button class="btn btn-sm btn-circle btn-ghost absolute right-2 top-2">X</button></form>
    <h3 class="text-lg font-bold mb-4">instance settings modify</h3>
    <div id="edit-instance-result"></div>

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">port</span><span class="label-text-alt text-warning">change </span></label>
      <input id="ei-port" type="number" class="input input-bordered input-sm w-full" value="%d" min="1" max="65535" disabled>
    </div>

    %s
    %s
    %s

    <div class="form-control mb-3">
      <label class="label"><span class="label-text font-semibold">shutdown command</span></label>
      <input id="ei-stop-command" type="text" class="input input-bordered input-sm w-full" value="%s">
    </div>

    <div class="flex flex-wrap gap-4 mb-4">
      <label class="label cursor-pointer gap-2">
        <input id="ei-auto-start" type="checkbox" class="checkbox checkbox-sm" %s>
        <span class="label-text">auto-start</span>
      </label>
      <label class="label cursor-pointer gap-2">
        <input id="ei-auto-restart" type="checkbox" class="checkbox checkbox-sm" %s>
        <span class="label-text">auto-restart</span>
      </label>
    </div>

    <div class="divider text-xs">backup schedule</div>
    <div class="flex flex-wrap gap-4 mb-3">
      <label class="label cursor-pointer gap-2">
        <input id="ei-backup-enabled" type="checkbox" class="checkbox checkbox-sm" %s>
        <span class="label-text">auto backup enable</span>
      </label>
    </div>
    <div class="form-control mb-3">
      <label class="label"><span class="label-text">Cron expression <span class="text-xs opacity-60">(min when day month day)</span></span></label>
      <input id="ei-backup-cron" type="text" class="input input-bordered input-sm w-full font-mono" value="%s" placeholder="0 */6 * * *">
      <label class="label"><span class="label-text-alt opacity-60">e.g.: 0 */6 * * * (every 6 hours), 0 3 * * * (mapday 03:00)</span></label>
    </div>
    <div class="form-control mb-3">
      <label class="label"><span class="label-text">max retention count</span></label>
      <input id="ei-backup-max-count" type="number" class="input input-bordered input-sm w-full" value="%d" min="1" max="100">
    </div>

    <div class="divider text-xs">Docker resource limit</div>
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
      <div class="form-control">
        <label class="label"><span class="label-text">Docker memory limit</span></label>
        <input id="ei-docker-memory" type="text" class="input input-bordered input-sm w-full" value="%s" placeholder="e.g.: 3G (if empty auto)">
        <label class="label"><span class="label-text-alt">if empty JVM max memory 1.5x</span></label>
      </div>
      <div class="form-control">
        <label class="label"><span class="label-text">Docker CPU limit</span></label>
        <input id="ei-docker-cpus" type="text" class="input input-bordered input-sm w-full" value="%s" placeholder="e.g.: 2.0 (if empty, no limit)">
        <label class="label"><span class="label-text-alt">usable CPU core count</span></label>
      </div>
    </div>

    <div class="divider text-xs">Java version / Docker customization</div>
    <div class="form-control mb-3">
      <label class="label"><span class="label-text">Java version</span></label>
      <select id="ei-java-version" class="select select-bordered select-sm w-full">
        <option value="">default (21)</option>
        <option value="17" %s>Java 17</option>
        <option value="21" %s>Java 21</option>
        <option value="25" %s>Java 25</option>
      </select>
    </div>
    <div class="form-control mb-3">
      <label class="label"><span class="label-text">custom Dockerfile</span></label>
      <textarea id="ei-custom-dockerfile" class="textarea textarea-bordered w-full font-mono text-xs" rows="5" placeholder="if empty default image use">%s</textarea>
    </div>
    <div class="form-control mb-3">
      <label class="label"><span class="label-text">custom docker-compose.yml</span></label>
      <textarea id="ei-custom-compose" class="textarea textarea-bordered w-full font-mono text-xs" rows="6" placeholder="if empty, unused ">%s</textarea>
    </div>

    <div class="modal-action">
      <button type="button" class="btn btn-primary" onclick="window._editInstance.save()">save</button>
      <form method="dialog"><button class="btn">cancel</button></form>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop"><button>close</button></form>
</dialog>
<script>
window._editInstance = {
    instanceId: '%s',
    instanceType: '%s',

    save: async function() {
        const resultDiv = document.getElementById('edit-instance-result');
        const body = {};

        // port immutable — send no
        body.stop_command = document.getElementById('ei-stop-command').value;
        body.auto_start = document.getElementById('ei-auto-start').checked;
        body.auto_restart = document.getElementById('ei-auto-restart').checked;

        // Java based: memory, JVM flag
        const memMin = document.getElementById('ei-mem-min');
        const memMax = document.getElementById('ei-mem-max');
        const jvmArgs = document.getElementById('ei-jvm-args');
        if (memMin) body.memory_min = memMin.value.trim();
        if (memMax) body.memory_max = memMax.value.trim();
        if (jvmArgs) body.jvm_args = jvmArgs.value.trim();

        // service version
        const sv = document.getElementById('ei-service-version');
        if (sv) body.service_version = sv.value.trim();

        // typeper field
        const t = this.instanceType;
        if (t === 'mysql') {
            const el1 = document.getElementById('ei-mysql-root-password');
            const el2 = document.getElementById('ei-mysql-extra-args');
            if (el1) body.mysql_root_password = el1.value;
            if (el2) body.mysql_extra_args = el2.value;
        } else if (t === 'postgresql') {
            const el1 = document.getElementById('ei-pg-password');
            const el2 = document.getElementById('ei-pg-extra-args');
            if (el1) body.pg_password = el1.value;
            if (el2) body.pg_extra_args = el2.value;
        } else if (t === 'mongodb') {
            const el1 = document.getElementById('ei-mongo-admin-user');
            const el2 = document.getElementById('ei-mongo-admin-password');
            const el3 = document.getElementById('ei-mongo-extra-args');
            if (el1) body.mongo_admin_user = el1.value;
            if (el2) body.mongo_admin_password = el2.value;
            if (el3) body.mongo_extra_args = el3.value;
        } else if (t === 'redis') {
            const el1 = document.getElementById('ei-redis-password');
            const el2 = document.getElementById('ei-redis-extra-args');
            if (el1) body.redis_password = el1.value;
            if (el2) body.redis_extra_args = el2.value;
        } else if (t === 'kafka') {
            const el1 = document.getElementById('ei-kafka-broker-id');
            const el2 = document.getElementById('ei-kafka-extra-args');
            if (el1) body.kafka_broker_id = parseInt(el1.value) || 0;
            if (el2) body.kafka_extra_args = el2.value;
        }

        // backup schedule
        body.backup_enabled = document.getElementById('ei-backup-enabled').checked;
        body.backup_cron = document.getElementById('ei-backup-cron').value.trim();
        body.backup_max_count = parseInt(document.getElementById('ei-backup-max-count').value) || 10;

        // Java version + Docker customization
        body.java_version = document.getElementById('ei-java-version').value;
        body.custom_dockerfile = document.getElementById('ei-custom-dockerfile').value;
        body.custom_compose = document.getElementById('ei-custom-compose').value;

        // Docker resource limit
        const dockerMem = document.getElementById('ei-docker-memory');
        const dockerCpus = document.getElementById('ei-docker-cpus');
        if (dockerMem) body.docker_memory = dockerMem.value.trim();
        if (dockerCpus) body.docker_cpus = dockerCpus.value.trim();

        try {
            const resp = await fetch('/api/instances/' + this.instanceId, {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(body)
            });
            const data = await resp.json();
            if (data.status === 'success') {
                resultDiv.innerHTML = '<div class="alert alert-success text-sm">' + data.message + '</div>';
                setTimeout(function() { location.reload(); }, 1500);
            } else {
                resultDiv.innerHTML = '<div class="alert alert-error text-sm">' + data.message + '</div>';
            }
        } catch(e) {
            resultDiv.innerHTML = '<div class="alert alert-error text-sm">request failed: ' + e.message + '</div>';
        }
    }
};
</script>`,
		inst.Port,
		versionField,
		memoryFields,
		typeFields,
		inst.StopCommand,
		autoStartChecked,
		autoRestartChecked,
		backupEnabledChecked,
		inst.BackupCron,
		inst.BackupMaxCount,
		inst.DockerMemory, inst.DockerCPUs,
		jv17Selected, jv21Selected, jv25Selected,
		inst.CustomDockerfile,
		inst.CustomCompose,
		inst.ID, inst.InstanceType)
}

func buildInstanceControls(instID, status string) string {
	var html string
	switch status {
	case "running":
		html = fmt.Sprintf(`
            <button class="btn btn-warning w-full" onclick="controlInstance('%s','restart')">restart</button>
            <button class="btn btn-error w-full" onclick="controlInstance('%s','stop')">stop</button>
            <button class="btn btn-outline btn-error btn-sm w-full" onclick="if(confirm('force shutdown?')) controlInstance('%s','kill')">force shutdown</button>`,
			instID, instID, instID)
	case "stopped", "crashed":
		html = fmt.Sprintf(`
            <button class="btn btn-success w-full" onclick="controlInstance('%s','start')">start</button>`,
			instID)
	case "starting":
		html = `<div class="flex items-center gap-2"><span class="loading loading-spinner"></span><span>start during...</span></div>`
	case "stopping":
		html = `<div class="flex items-center gap-2"><span class="loading loading-spinner"></span><span>shutdown during...</span></div>`
	default:
		html = fmt.Sprintf(`
            <button class="btn btn-success w-full" onclick="controlInstance('%s','start')">start</button>`,
			instID)
	}
	return fmt.Sprintf(`<div class="text-center mb-2">%s</div>%s`, statusBadgeHTML(status), html)
}

func buildInstanceStatusPartial(data map[string]interface{}) string {
	instI, _ := data["Instance"]
	inst, _ := instI.(*store.Instance)
	if inst == nil {
		return ""
	}
	return buildInstanceControls(inst.ID, inst.Status)
}

// buildInstanceMetricsPartial renders an HTMX partial with current instance metrics.
func buildInstanceMetricsPartial(data map[string]interface{}) string {
	metricsI, _ := data["InstanceMetrics"]
	m, _ := metricsI.(*InstanceMetrics)
	if m == nil {
		return `<div class="text-sm text-gray-500">no metrics data (instance may not be running)</div>`
	}

	memPercent := float64(0)
	if m.MemoryLimitMB > 0 {
		memPercent = float64(m.MemoryUsedMB) / float64(m.MemoryLimitMB) * 100
	}
	cpuColor := progressColor(m.CPUPercent)
	memColor := progressColor(memPercent)

	return fmt.Sprintf(`
	<div class="grid grid-cols-1 md:grid-cols-3 gap-6">
		<div>
			<div class="flex justify-between mb-1">
				<span class="text-sm font-medium">CPU</span>
				<span class="text-sm font-bold">%.1f%%</span>
			</div>
			<progress class="progress %s w-full" value="%.0f" max="100"></progress>
		</div>
		<div>
			<div class="flex justify-between mb-1">
				<span class="text-sm font-medium">memory</span>
				<span class="text-sm font-bold">%s / %s (%.1f%%)</span>
			</div>
			<progress class="progress %s w-full" value="%.0f" max="100"></progress>
		</div>
		<div>
			<div class="flex justify-between mb-1">
				<span class="text-sm font-medium">network</span>
				<span class="text-sm font-bold">RX: %s / TX: %s</span>
			</div>
		</div>
	</div>`,
		m.CPUPercent, cpuColor, m.CPUPercent,
		formatMB(m.MemoryUsedMB), formatMB(m.MemoryLimitMB), memPercent,
		memColor, memPercent,
		formatFileSize(m.NetRxBytes), formatFileSize(m.NetTxBytes))
}

// ─────────────────────────────────────────────────────────────
// console
// ─────────────────────────────────────────────────────────────

func buildConsoleHTML(data map[string]interface{}) string {
	instI, _ := data["Instance"]
	inst, _ := instI.(*store.Instance)

	instName := "server"
	instID := ""
	if inst != nil {
		instName = inst.Name
		instID = inst.ID
	}

	return fmt.Sprintf(`<h1 class="text-xl sm:text-3xl font-bold mb-4 sm:mb-6">console: %s</h1>
    <div class="card bg-base-100 shadow-xl" x-data="{ command: '' }">
        <div class="card-body">
            <div id="console-output" class="console-output bg-base-300 rounded-lg p-4 overflow-y-auto font-mono text-sm" style="height: calc(100vh - 240px); min-height: 200px;">
                <p class="text-gray-500">server console connect during...</p>
            </div>
            <div class="flex gap-2 mt-4">
                <input type="text" x-model="command" placeholder="command please enter..."
                       class="input input-bordered flex-1"
                       @keydown.enter="$dispatch('send-command', { command: command }); command = ''" />
                <button class="btn btn-primary"
                        @click="$dispatch('send-command', { command: command }); command = ''">send</button>
            </div>
        </div>
    </div>
    <script>
        document.addEventListener('DOMContentLoaded', function() {
            const instanceId = '%s' || window.location.pathname.split('/')[2];
            const output = document.getElementById('console-output');
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws/logs/' + instanceId;

            let ws = null;
            let reconnectTimer = null;
            let reconnectDelay = 1000;
            const maxReconnectDelay = 15000;

            function appendLine(text, className) {
                const line = document.createElement('div');
                if (className) line.className = className;
                line.textContent = text;
                output.appendChild(line);
                output.scrollTop = output.scrollHeight;
                while (output.childElementCount > 1000) {
                    output.removeChild(output.firstChild);
                }
            }

            function connect() {
                if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
                    return;
                }
                ws = new WebSocket(wsUrl);

                ws.onopen = function() {
                    reconnectDelay = 1000;
                    // s "connect during..." message remove
                    var placeholder = output.querySelector('p.text-gray-500');
                    if (placeholder) placeholder.remove();
                    appendLine('[console connected]', 'text-success');
                };

                ws.onmessage = function(event) {
                    appendLine(event.data);
                };

                ws.onclose = function() {
                    ws = null;
                    appendLine('[master disconnected — ' + Math.round(reconnectDelay/1000) + 's after reconnect...]', 'text-warning');
                    scheduleReconnect();
                };

                ws.onerror = function(e) {
                    appendLine('[WebSocket error occurred]', 'text-error');
                };
            }

            function scheduleReconnect() {
                if (reconnectTimer) clearTimeout(reconnectTimer);
                reconnectTimer = setTimeout(function() {
                    reconnectTimer = null;
                    connect();
                    reconnectDelay = Math.min(reconnectDelay * 1.5, maxReconnectDelay);
                }, reconnectDelay);
            }

            // s connect
            connect();

            // 10s after connect not when message display
            setTimeout(function() {
                var placeholder = output.querySelector('p.text-gray-500');
                if (placeholder) {
                    placeholder.textContent = 'server connectcannot. page refresh or connect network please confirm.';
                }
            }, 10000);

            // control command when WebSocket keep — state notificationonly display
            document.addEventListener('instance-controlled', function(e) {
                var action = (e.detail && e.detail.action) || '';
                var labels = {start:'start',stop:'stop',restart:'restart',kill:'force shutdown'};
                var label = labels[action] || action || 'control';
                appendLine('[' + label + ' send command — waiting for server log...]', 'text-warning');
            });

            document.addEventListener('send-command', function(e) {
                if (ws && ws.readyState === WebSocket.OPEN && e.detail.command) {
                    ws.send(e.detail.command);
                    appendLine('> ' + e.detail.command, 'text-info');
                } else {
                    appendLine('[send command failed: master connect did not]', 'text-error');
                }
            });

            // cleanup on page leave
            window.addEventListener('beforeunload', function() {
                if (reconnectTimer) clearTimeout(reconnectTimer);
                if (ws) ws.close();
            });
        });
    </script>`, instName, instID)
}
