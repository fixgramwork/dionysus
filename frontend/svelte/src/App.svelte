<script>
  import { onMount } from 'svelte';
  import {
    Activity,
    AlertTriangle,
    Cpu,
    Database,
    KeyRound,
    Play,
    RefreshCw,
    Save,
    Server,
    Settings,
    Wifi,
    Zap
  } from '@lucide/svelte';
  import { api, postJSON, readToken, writeToken } from './api.js';

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Server },
    { id: 'network', label: 'Network', icon: Wifi },
    { id: 'llm', label: 'Local LLM', icon: Cpu },
    { id: 'services', label: 'Services', icon: Activity },
    { id: 'console', label: 'Console', icon: Database }
  ];

  let activeTab = 'overview';
  let token = readToken();
  let loading = false;
  let liveLoading = false;
  let error = '';
  let savedAt = '';
  let version = null;
  let status = null;
  let services = [];
  let network = null;
  let networkDraft = null;
  let ollama = null;
  let history = [];
  let consoleLines = [];

  function clone(value) {
    return JSON.parse(JSON.stringify(value || {}));
  }

  function rememberToken() {
    writeToken(token);
    pushConsole('info', token ? 'API token stored in this browser.' : 'API token cleared.');
  }

  function pushConsole(level, message) {
    consoleLines = [
      { at: new Date().toLocaleTimeString(), level, message },
      ...consoleLines
    ].slice(0, 80);
  }

  async function refresh() {
    loading = true;
    error = '';
    try {
      const [nextVersion, nextStatus, nextServices, nextNetwork, nextOllama, nextHistory] = await Promise.all([
        api('/api2/json/version'),
        api('/api2/json/nodes/localhost/status'),
        api('/api2/json/nodes/localhost/services'),
        api('/api2/json/nodes/localhost/network/status'),
        api('/api2/json/nodes/localhost/ollama/status'),
        api('/api2/json/nodes/localhost/ollama/history?limit=120')
      ]);
      version = nextVersion;
      status = nextStatus;
      services = nextServices || [];
      network = nextNetwork;
      networkDraft = clone(nextNetwork?.config);
      ollama = nextOllama;
      history = nextHistory || [];
      savedAt = new Date().toLocaleTimeString();
      pushConsole('info', 'Refreshed node state from API2.');
    } catch (err) {
      error = err.message;
      pushConsole('error', err.message);
    } finally {
      loading = false;
    }
  }

  async function refreshLive() {
    if (loading || liveLoading) return;
    liveLoading = true;
    try {
      status = await api('/api2/json/nodes/localhost/status');
      savedAt = new Date().toLocaleTimeString();
      error = '';
    } catch (err) {
      if (error !== err.message) {
        pushConsole('error', err.message);
      }
      error = err.message;
    } finally {
      liveLoading = false;
    }
  }

  async function submitNetwork(dryRun, apply) {
    if (!networkDraft) return;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/network/config', {
        ...networkDraft,
        dryRun,
        apply
      });
      pushConsole(dryRun ? 'warn' : 'info', `network ${dryRun ? 'previewed' : 'saved'} mode=${result.config.mode}`);
      if (result.applyResult) {
        pushConsole(result.applyResult.status === 'failed' ? 'error' : 'info', `network apply ${result.applyResult.status}`);
      }
      await refresh();
    } catch (err) {
      error = err.message;
      pushConsole('error', err.message);
    }
  }

  async function applyNetworkOnly(dryRun) {
    try {
      const result = await postJSON('/api2/json/nodes/localhost/network/apply', { dryRun });
      pushConsole(dryRun ? 'warn' : 'info', `network service ${result.status}`);
      await refresh();
    } catch (err) {
      error = err.message;
      pushConsole('error', err.message);
    }
  }

  async function optimizeKV(dryRun) {
    try {
      const result = await postJSON('/api2/json/nodes/localhost/ollama/optimize', { dryRun });
      pushConsole(dryRun ? 'warn' : 'info', `kv-cache profile ${result.status}`);
      for (const line of result.console || []) {
        pushConsole(line.level, line.message);
      }
      await refresh();
    } catch (err) {
      error = err.message;
      pushConsole('error', err.message);
    }
  }

  function formatBytes(value) {
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    let next = Number(value || 0);
    let unit = 0;
    while (next >= 1024 && unit < units.length - 1) {
      next /= 1024;
      unit += 1;
    }
    return `${next.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
  }

  function formatPercent(value) {
    return `${Number(value || 0).toFixed(1)}%`;
  }

  function ratioPercent(used, total) {
    const nextTotal = Number(total || 0);
    if (nextTotal <= 0) return 0;
    return (Number(used || 0) / nextTotal) * 100;
  }

  function formatSeconds(value) {
    const total = Math.max(0, Number(value || 0));
    const days = Math.floor(total / 86400);
    const hours = Math.floor((total % 86400) / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
  }

  function stateClass(state) {
    if (state === 'active' || state === 'ready' || state === 'api-online' || state === 'restarted') return 'ok';
    if (state === 'failed' || state === 'missing') return 'bad';
    return 'warn';
  }

  $: memory = status?.memory || {};
  $: swap = status?.swap || {};
  $: cpu = status?.cpu || {};
  $: os = status?.os || {};
  $: controlPlane = status?.controlPlane || {};
  $: kv = ollama?.kvCache || {};
  $: memoryUsedPercent = ratioPercent(memory.used, memory.total);
  $: swapUsedPercent = ratioPercent(swap.used, swap.total);

  onMount(() => {
    refresh();
    const interval = window.setInterval(refreshLive, 2000);
    return () => window.clearInterval(interval);
  });
</script>

<svelte:head>
  <title>Dionysus PVE Manager</title>
</svelte:head>

<div class="app-shell">
  <header class="topbar">
    <div class="brand">
      <Server size={19} />
      <span>Dionysus PVE Manager</span>
      {#if version}
        <small>{version.stack}</small>
      {/if}
    </div>
    <div class="top-actions">
      <label class="token-field">
        <KeyRound size={15} />
        <input type="password" placeholder="API token" bind:value={token} on:change={rememberToken} />
      </label>
      <button class="icon-button" on:click={refresh} disabled={loading} title="Refresh state">
        <RefreshCw size={16} class={loading ? 'spin' : ''} />
      </button>
    </div>
  </header>

  <div class="workspace">
    <aside class="sidebar">
      <div class="node-block">
        <strong>{os.hostname || 'localhost'}</strong>
        <span>{os.prettyName || 'Dionysus target OS'}</span>
      </div>
      {#each tabs as tab}
        <button class:active={activeTab === tab.id} class="nav-item" on:click={() => (activeTab = tab.id)}>
          <svelte:component this={tab.icon} size={16} />
          <span>{tab.label}</span>
        </button>
      {/each}
    </aside>

    <main class="content">
      <div class="status-strip">
        <div>
          <span class="label">Runtime</span>
          <strong>{controlPlane.runtimeMode || 'unknown'}</strong>
        </div>
        <div>
          <span class="label">CPU</span>
          <strong>{cpu.state === 'ready' ? formatPercent(cpu.usedPercent) : '-'}</strong>
        </div>
        <div>
          <span class="label">RAM</span>
          <strong>{formatPercent(memoryUsedPercent)}</strong>
        </div>
        <div>
          <span class="label">Updated</span>
          <strong>{savedAt || '-'}</strong>
        </div>
      </div>

      {#if error}
        <div class="notice">
          <AlertTriangle size={16} />
          <span>{error}</span>
        </div>
      {/if}

      {#if activeTab === 'overview'}
        <section class="grid two">
          <article class="panel">
            <h2>Node</h2>
            <dl class="facts">
              <div><dt>Hostname</dt><dd>{os.hostname || '-'}</dd></div>
              <div><dt>Kernel</dt><dd>{os.kernel?.name || '-'} {os.kernel?.release || ''}</dd></div>
              <div><dt>Architecture</dt><dd>{os.kernel?.architecture || '-'}</dd></div>
              <div><dt>Uptime</dt><dd>{formatSeconds(status?.uptime)}</dd></div>
              <div><dt>API stack</dt><dd>{controlPlane.stack || '-'}</dd></div>
            </dl>
          </article>

          <article class="panel">
            <h2>CPU</h2>
            <div class="metric-summary">
              <div>
                <span class="label">Usage</span>
                <strong>{cpu.state === 'ready' ? formatPercent(cpu.usedPercent) : '-'}</strong>
              </div>
              <span>{cpu.cores || 0} cores</span>
            </div>
            <div class="meter-row">
              <span>Total</span>
              <meter min="0" max="100" value={cpu.usedPercent || 0}></meter>
              <strong>{formatPercent(cpu.usedPercent)}</strong>
            </div>
            <div class="metric-breakdown">
              <span>User {formatPercent(cpu.userPercent)}</span>
              <span>System {formatPercent(cpu.systemPercent)}</span>
              <span>I/O wait {formatPercent(cpu.iowaitPercent)}</span>
              <span>Idle {formatPercent(cpu.idlePercent)}</span>
            </div>
            <p class="muted">{cpu.model || 'CPU model unavailable'} · sample {cpu.sampleMillis || 0} ms</p>
          </article>

          <article class="panel">
            <h2>Memory</h2>
            <div class="metric-summary">
              <div>
                <span class="label">RAM Used</span>
                <strong>{formatPercent(memoryUsedPercent)}</strong>
              </div>
              <span>{formatBytes(memory.available)} available</span>
            </div>
            <div class="meter-row">
              <span>RAM</span>
              <meter min="0" max={memory.total || 1} value={memory.used || 0}></meter>
              <strong>{formatPercent(memoryUsedPercent)}</strong>
            </div>
            <div class="meter-row">
              <span>Swap</span>
              <meter min="0" max={swap.total || 1} value={swap.used || 0}></meter>
              <strong>{formatPercent(swapUsedPercent)}</strong>
            </div>
            <div class="metric-breakdown">
              <span>RAM {formatBytes(memory.used)} / {formatBytes(memory.total)}</span>
              <span>Swap {formatBytes(swap.used)} / {formatBytes(swap.total)}</span>
              <span>Cache {formatBytes(memory.cached)}</span>
              <span>Load {(status?.loadavg || []).join(' ') || '-'}</span>
            </div>
          </article>

          <article class="panel wide">
            <h2>Control Plane Paths</h2>
            <table>
              <tbody>
                <tr><th>Web root</th><td>{controlPlane.wwwRoot || '-'}</td></tr>
                <tr><th>Metrics DB</th><td>{controlPlane.metricsDb || '-'}</td></tr>
                <tr><th>Network config</th><td>{controlPlane.networkConfig || '-'}</td></tr>
              </tbody>
            </table>
          </article>
        </section>
      {:else if activeTab === 'network'}
        <section class="grid two">
          <article class="panel">
            <h2>Network Config</h2>
            {#if networkDraft}
              <div class="form-grid">
                <label>Mode
                  <select bind:value={networkDraft.mode}>
                    <option value="none">None</option>
                    <option value="lan">LAN</option>
                    <option value="wifi">Wi-Fi</option>
                  </select>
                </label>
                <label>LAN interface
                  <input bind:value={networkDraft.lan.iface} />
                </label>
                <label>LAN IPv4 method
                  <select bind:value={networkDraft.lan.ipv4.method}>
                    <option value="dhcp">DHCP</option>
                    <option value="static">Static</option>
                    <option value="none">None</option>
                  </select>
                </label>
                <label>LAN address
                  <input bind:value={networkDraft.lan.ipv4.address} placeholder="192.168.10.20/24" />
                </label>
                <label>Gateway
                  <input bind:value={networkDraft.lan.ipv4.gateway} />
                </label>
                <label>DNS
                  <input bind:value={networkDraft.lan.ipv4.dns} />
                </label>
                <label>Wi-Fi interface
                  <input bind:value={networkDraft.wifi.iface} />
                </label>
                <label>Country
                  <input maxlength="2" bind:value={networkDraft.wifi.country} />
                </label>
                <label>SSID
                  <input bind:value={networkDraft.wifi.ssid} />
                </label>
                <label>PSK action
                  <select bind:value={networkDraft.wifi.pskAction}>
                    <option value="preserve">Preserve</option>
                    <option value="set">Set</option>
                    <option value="clear">Clear</option>
                  </select>
                </label>
                {#if networkDraft.wifi.pskAction === 'set'}
                  <label>Wi-Fi PSK
                    <input type="password" bind:value={networkDraft.wifi.psk} />
                  </label>
                {/if}
              </div>
              <div class="button-row">
                <button on:click={() => submitNetwork(true, false)}><Settings size={15} />Preview</button>
                <button on:click={() => submitNetwork(false, false)}><Save size={15} />Save</button>
                <button class="primary" on:click={() => submitNetwork(false, true)}><Play size={15} />Save and Apply</button>
              </div>
            {/if}
          </article>

          <article class="panel">
            <h2>Interfaces</h2>
            <table>
              <thead><tr><th>Name</th><th>State</th><th>Address</th><th>Traffic</th></tr></thead>
              <tbody>
                {#each network?.interfaces || [] as iface}
                  <tr>
                    <td>{iface.name}</td>
                    <td><span class={stateClass(iface.operstate)}>{iface.operstate || '-'}</span></td>
                    <td>{(iface.addresses || []).join(', ') || iface.address || '-'}</td>
                    <td>{formatBytes(iface.rxBytes)} in / {formatBytes(iface.txBytes)} out</td>
                  </tr>
                {/each}
              </tbody>
            </table>
            <div class="button-row">
              <button on:click={() => applyNetworkOnly(true)}><Settings size={15} />Preview Restart</button>
              <button on:click={() => applyNetworkOnly(false)}><Play size={15} />Restart Network</button>
            </div>
          </article>
        </section>
      {:else if activeTab === 'llm'}
        <section class="grid two">
          <article class="panel">
            <h2>Ollama</h2>
            <dl class="facts">
              <div><dt>State</dt><dd><span class={stateClass(ollama?.platform?.state)}>{ollama?.platform?.state || 'not-detected'}</span></dd></div>
              <div><dt>API</dt><dd>{ollama?.apiBase || '-'}</dd></div>
              <div><dt>Version</dt><dd>{ollama?.version || '-'}</dd></div>
              <div><dt>Models</dt><dd>{ollama?.platform?.modelCount || 0} local / {ollama?.platform?.loadedModelCount || 0} loaded</dd></div>
            </dl>
          </article>

          <article class="panel">
            <h2>KV-cache Profile</h2>
            <div class="meter-row">
              <span>Swap</span>
              <meter min="0" max={kv.swap?.total || 1} value={kv.swap?.used || 0}></meter>
              <strong>{formatPercent(kv.swap?.usedPercent)}</strong>
            </div>
            <p class="muted">Pending {kv.pendingCount || 0}, missing {kv.missingCount || 0}, ready {kv.readyCount || 0}</p>
            <div class="button-row">
              <button on:click={() => optimizeKV(true)}><Zap size={15} />Preview</button>
              <button class="primary" on:click={() => optimizeKV(false)}><Zap size={15} />Apply</button>
            </div>
          </article>

          <article class="panel wide">
            <h2>Kernel Targets</h2>
            <table>
              <thead><tr><th>Target</th><th>Current</th><th>Desired</th><th>Status</th><th>Reason</th></tr></thead>
              <tbody>
                {#each kv.targets || [] as target}
                  <tr>
                    <td>{target.label}</td>
                    <td>{target.current || '-'}</td>
                    <td>{target.target}</td>
                    <td><span class={stateClass(target.status)}>{target.status}</span></td>
                    <td>{target.reason}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </article>

          <article class="panel wide">
            <h2>Metrics History</h2>
            <table>
              <thead><tr><th>Sample</th><th>RAM used</th><th>Swap used</th><th>Ollama RSS</th><th>Loaded models</th></tr></thead>
              <tbody>
                {#each history.slice(-12) as row}
                  <tr>
                    <td>{new Date(row.collectedAt * 1000).toLocaleTimeString()}</td>
                    <td>{formatBytes(row.memory.used)}</td>
                    <td>{formatBytes(row.swap.used)}</td>
                    <td>{formatBytes(row.ollama.processRss)}</td>
                    <td>{row.ollama.loadedModelCount}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </article>
        </section>
      {:else if activeTab === 'services'}
        <section class="panel">
          <h2>Systemd Services</h2>
          <table>
            <thead><tr><th>Name</th><th>State</th></tr></thead>
            <tbody>
              {#each services as service}
                <tr>
                  <td>{service.name}</td>
                  <td><span class={stateClass(service.state)}>{service.state}</span></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </section>
      {:else}
        <section class="panel console-panel">
          <h2>Operator Console</h2>
          <div class="console">
            {#each consoleLines as line}
              <div class={line.level}><span>{line.at}</span>{line.message}</div>
            {/each}
          </div>
        </section>
      {/if}
    </main>
  </div>
</div>
