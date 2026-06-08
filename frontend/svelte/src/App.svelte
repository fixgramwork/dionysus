<script>
  import { onMount } from 'svelte';
  import {
    Activity,
    AlertTriangle,
    Cpu,
    Database,
    KeyRound,
    LogIn,
    LogOut,
    Play,
    RefreshCw,
    Save,
    Server,
    Settings,
    Trash2,
    User,
    Wifi,
    Zap
  } from '@lucide/svelte';
  import { api, clearToken, login, postJSON, readToken } from './api.js';

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Server },
    { id: 'network', label: 'Network', icon: Wifi },
    { id: 'llm', label: 'Local LLM', icon: Cpu },
    { id: 'services', label: 'Services', icon: Activity },
    { id: 'users', label: 'Users', icon: User },
    { id: 'console', label: 'Console', icon: Database }
  ];

  let activeTab = 'overview';
  let authenticated = !!readToken();
  let session = null;
  let loginUsername = 'root';
  let loginPassword = '';
  let loginLoading = false;
  let loginError = '';
  let loading = false;
  let liveLoading = false;
  let error = '';
  let savedAt = '';
  let version = null;
  let status = null;
  let services = [];
  let network = null;
  let networkDraft = null;
  let previousNetworkSample = null;
  let ollama = null;
  let history = [];
  let consoleLines = [];
  let consoleCommand = 'uname -a';
  let consoleCwd = '/';
  let consoleRunning = false;
  let terminalEntries = [];
  let users = [];
  let userLoading = false;
  let userError = '';
  let newUser = { username: '', password: '', confirm: '' };
  let passwordDrafts = {};

  function clone(value) {
    return JSON.parse(JSON.stringify(value || {}));
  }

  function pushConsole(level, message) {
    consoleLines = [
      { at: new Date().toLocaleTimeString(), level, message },
      ...consoleLines
    ].slice(0, 80);
  }

  function resetNodeState() {
    version = null;
    status = null;
    services = [];
    users = [];
    network = null;
    networkDraft = null;
    previousNetworkSample = null;
    ollama = null;
    history = [];
    savedAt = '';
  }

  function handleAPIError(err, log = true) {
    if (err.status === 401) {
      clearToken();
      authenticated = false;
      session = null;
      resetNodeState();
      loginError = err.message || 'Session expired';
    }
    error = err.message;
    if (log) {
      pushConsole('error', err.message);
    }
  }

  async function submitLogin() {
    loginLoading = true;
    loginError = '';
    error = '';
    try {
      session = await login(loginUsername.trim(), loginPassword);
      authenticated = true;
      loginPassword = '';
      pushConsole('info', `Signed in as ${session.username}.`);
      await refresh();
    } catch (err) {
      clearToken();
      authenticated = false;
      loginError = err.message;
      error = err.message;
    } finally {
      loginLoading = false;
    }
  }

  function logout() {
    clearToken();
    authenticated = false;
    session = null;
    resetNodeState();
    pushConsole('info', 'Signed out.');
  }

  async function refresh() {
    if (!authenticated) return;
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
      const [nextSession, nextUsers] = await Promise.all([
        api('/api2/json/access/session'),
        api('/api2/json/access/users')
      ]);
      version = nextVersion;
      session = nextSession;
      status = nextStatus;
      services = nextServices || [];
      users = nextUsers || [];
      network = withNetworkRates(nextNetwork);
      networkDraft = clone(nextNetwork?.config);
      ollama = nextOllama;
      history = nextHistory || [];
      savedAt = new Date().toLocaleTimeString();
      pushConsole('info', 'Refreshed node state from API2.');
    } catch (err) {
      handleAPIError(err);
    } finally {
      loading = false;
    }
  }

  function withNetworkRates(nextNetwork) {
    if (!nextNetwork) return nextNetwork;
    const sampledAt = Date.now();
    const elapsedSeconds = previousNetworkSample
      ? Math.max((sampledAt - previousNetworkSample.sampledAt) / 1000, 0.001)
      : 0;

    const interfaces = (nextNetwork.interfaces || []).map((iface) => {
      const previous = previousNetworkSample?.interfaces?.[iface.name];
      const rxBytes = Number(iface.rxBytes || 0);
      const txBytes = Number(iface.txBytes || 0);
      const rxDelta = previous ? Math.max(0, rxBytes - previous.rxBytes) : 0;
      const txDelta = previous ? Math.max(0, txBytes - previous.txBytes) : 0;
      return {
        ...iface,
        rxRateBytes: elapsedSeconds > 0 ? rxDelta / elapsedSeconds : 0,
        txRateBytes: elapsedSeconds > 0 ? txDelta / elapsedSeconds : 0
      };
    });

    previousNetworkSample = {
      sampledAt,
      interfaces: Object.fromEntries(
        interfaces.map((iface) => [
          iface.name,
          {
            rxBytes: Number(iface.rxBytes || 0),
            txBytes: Number(iface.txBytes || 0)
          }
        ])
      )
    };

    return { ...nextNetwork, interfaces };
  }

  async function refreshLive() {
    if (!authenticated) return;
    if (loading || liveLoading) return;
    liveLoading = true;
    try {
      status = await api('/api2/json/nodes/localhost/status');
      savedAt = new Date().toLocaleTimeString();
      error = '';
    } catch (err) {
      if (error !== err.message) {
        handleAPIError(err);
      } else {
        handleAPIError(err, false);
      }
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
      handleAPIError(err);
    }
  }

  async function applyNetworkOnly(dryRun) {
    try {
      const result = await postJSON('/api2/json/nodes/localhost/network/apply', { dryRun });
      pushConsole(dryRun ? 'warn' : 'info', `network service ${result.status}`);
      await refresh();
    } catch (err) {
      handleAPIError(err);
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
      handleAPIError(err);
    }
  }

  async function runConsoleCommand() {
    const command = consoleCommand.trim();
    if (!command || consoleRunning) return;
    const cwd = consoleCwd.trim() || '/';
    consoleRunning = true;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/console/exec', { command, cwd });
      terminalEntries = [
        {
          at: new Date().toLocaleTimeString(),
          ...result
        },
        ...terminalEntries
      ].slice(0, 20);
      pushConsole(result.exitCode === 0 ? 'info' : 'warn', `command exited ${result.exitCode}: ${command}`);
    } catch (err) {
      terminalEntries = [
        {
          at: new Date().toLocaleTimeString(),
          command,
          cwd,
          exitCode: -1,
          stderr: err.message,
          stdout: '',
          timedOut: false
        },
        ...terminalEntries
      ].slice(0, 20);
      handleAPIError(err);
    } finally {
      consoleRunning = false;
    }
  }

  async function refreshUsers() {
    try {
      const [nextSession, nextUsers] = await Promise.all([
        api('/api2/json/access/session'),
        api('/api2/json/access/users')
      ]);
      session = nextSession;
      users = nextUsers || [];
      userError = '';
    } catch (err) {
      userError = err.message;
      handleAPIError(err);
    }
  }

  async function createUser() {
    userError = '';
    const username = newUser.username.trim();
    if (newUser.password !== newUser.confirm) {
      userError = 'password confirmation does not match';
      return;
    }
    userLoading = true;
    try {
      const created = await postJSON('/api2/json/access/users', {
        username,
        password: newUser.password
      });
      newUser = { username: '', password: '', confirm: '' };
      pushConsole('info', `user created: ${created.username}`);
      await refreshUsers();
    } catch (err) {
      userError = err.message;
      handleAPIError(err);
    } finally {
      userLoading = false;
    }
  }

  function setPasswordDraft(username, value) {
    passwordDrafts = { ...passwordDrafts, [username]: value };
  }

  async function updateUserPassword(username) {
    const password = passwordDrafts[username] || '';
    userError = '';
    userLoading = true;
    try {
      await postJSON('/api2/json/access/users/password', { username, password });
      passwordDrafts = { ...passwordDrafts, [username]: '' };
      pushConsole('info', `password updated: ${username}`);
      await refreshUsers();
    } catch (err) {
      userError = err.message;
      handleAPIError(err);
    } finally {
      userLoading = false;
    }
  }

  async function deleteUser(username) {
    if (!window.confirm(`Delete user ${username}?`)) return;
    userError = '';
    userLoading = true;
    try {
      await postJSON('/api2/json/access/users/delete', { username });
      pushConsole('warn', `user deleted: ${username}`);
      await refreshUsers();
    } catch (err) {
      userError = err.message;
      handleAPIError(err);
    } finally {
      userLoading = false;
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

  function clampPercent(value) {
    const next = Number(value || 0);
    if (!Number.isFinite(next)) return 0;
    return Math.max(0, Math.min(100, next));
  }

  function hasFiniteNumber(value) {
    return Number.isFinite(Number(value));
  }

  function visibleGaugePercent(value, ready = true) {
    if (!ready) return 0;
    const next = clampPercent(value);
    return next === 0 ? 1.5 : next;
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

  function formatDate(value) {
    const timestamp = Number(value || 0);
    if (timestamp <= 0) return '-';
    return new Date(timestamp * 1000).toLocaleString();
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
  $: cpuReady = cpu.state === 'ready' || hasFiniteNumber(cpu.usedPercent);
  $: cpuUsedPercent = clampPercent(cpu.usedPercent);
  $: cpuGaugePercent = visibleGaugePercent(cpu.usedPercent, cpuReady);
  $: cpuUsageText = cpuReady ? formatPercent(cpu.usedPercent) : '-';
  $: memoryUsedPercent = ratioPercent(memory.used, memory.total);
  $: swapUsedPercent = ratioPercent(swap.used, swap.total);

  onMount(() => {
    if (authenticated) {
      refresh();
    }
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
      {#if authenticated}
        <span class="session-user">
          <User size={15} />
          {session?.username || 'signed in'}
        </span>
        <button class="icon-button" on:click={refresh} disabled={loading} title="Refresh state">
          <RefreshCw size={16} class={loading ? 'spin' : ''} />
        </button>
        <button class="icon-button" on:click={logout} title="Sign out">
          <LogOut size={16} />
        </button>
      {:else}
        <span class="session-user">
          <KeyRound size={15} />
          Sign in required
        </span>
      {/if}
    </div>
  </header>

  {#if !authenticated}
    <main class="login-screen">
      <form class="login-panel" on:submit|preventDefault={submitLogin}>
        <div class="login-heading">
          <KeyRound size={22} />
          <div>
            <h1>Dionysus Console</h1>
            <span>Sign in to continue</span>
          </div>
        </div>
        <label>
          <span>Username</span>
          <input bind:value={loginUsername} autocomplete="username" />
        </label>
        <label>
          <span>Password</span>
          <input type="password" bind:value={loginPassword} autocomplete="current-password" />
        </label>
        {#if loginError}
          <div class="notice login-error">
            <AlertTriangle size={16} />
            <span>{loginError}</span>
          </div>
        {/if}
        <button class="primary login-submit" type="submit" disabled={loginLoading || !loginUsername.trim() || !loginPassword}>
          <LogIn size={16} />
          Sign in
        </button>
      </form>
    </main>
  {:else}
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
          <strong>{cpuUsageText}</strong>
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
            <div class="gauge-layout">
              <div class="arc-gauge">
                <svg viewBox="0 0 188 108" role="img" aria-label={`CPU usage ${cpuUsageText}`}>
                  <path class="gauge-track" d="M 18 90 A 76 76 0 0 1 170 90" pathLength="100" />
                  <path class="gauge-fill cpu" class:idle={cpuReady && cpuUsedPercent === 0} d="M 18 90 A 76 76 0 0 1 170 90" pathLength="100" stroke-dasharray={`${cpuGaugePercent} 100`} />
                </svg>
                <div class="gauge-readout">
                  <span>CPU</span>
                  <strong>{cpuUsageText}</strong>
                </div>
              </div>
              <div class="gauge-facts">
                <span>{cpuReady ? `${cpu.cores || 0} cores` : '-'}</span>
                <span>User {cpuReady ? formatPercent(cpu.userPercent) : '-'}</span>
                <span>System {cpuReady ? formatPercent(cpu.systemPercent) : '-'}</span>
              </div>
            </div>
            <div class="metric-breakdown">
              <span>User {cpuReady ? formatPercent(cpu.userPercent) : '-'}</span>
              <span>System {cpuReady ? formatPercent(cpu.systemPercent) : '-'}</span>
              <span>I/O wait {cpuReady ? formatPercent(cpu.iowaitPercent) : '-'}</span>
              <span>Idle {cpuReady ? formatPercent(cpu.idlePercent) : '-'}</span>
            </div>
            <p class="muted">{cpuReady ? (cpu.model || 'CPU model unavailable') : 'CPU sample unavailable'} · sample {cpuReady ? (cpu.sampleMillis || 0) : 0} ms</p>
          </article>

          <article class="panel">
            <h2>Memory</h2>
            <div class="gauge-layout">
              <div class="arc-gauge">
                <svg viewBox="0 0 188 108" role="img" aria-label={`RAM usage ${formatPercent(memoryUsedPercent)}`}>
                  <path class="gauge-track" d="M 18 90 A 76 76 0 0 1 170 90" pathLength="100" />
                  <path class="gauge-fill ram" d="M 18 90 A 76 76 0 0 1 170 90" pathLength="100" stroke-dasharray={`${clampPercent(memoryUsedPercent)} 100`} />
                </svg>
                <div class="gauge-readout">
                  <span>RAM</span>
                  <strong>{formatPercent(memoryUsedPercent)}</strong>
                </div>
              </div>
              <div class="gauge-facts">
                <span>{formatBytes(memory.available)} available</span>
                <span>{formatBytes(memory.used)} used</span>
                <span>{formatBytes(memory.total)} total</span>
              </div>
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
              <thead><tr><th>Name</th><th>State</th><th>Address</th><th>Rate</th><th>Total</th></tr></thead>
              <tbody>
                {#each network?.interfaces || [] as iface}
                  <tr>
                    <td>{iface.name}</td>
                    <td><span class={stateClass(iface.operstate)}>{iface.operstate || '-'}</span></td>
                    <td>{(iface.addresses || []).join(', ') || iface.address || '-'}</td>
                    <td>{formatBytes(iface.rxRateBytes)}/s in / {formatBytes(iface.txRateBytes)}/s out</td>
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
      {:else if activeTab === 'users'}
        <section class="grid two">
          <article class="panel">
            <h2>Add User</h2>
            <form class="user-form" on:submit|preventDefault={createUser}>
              <label>
                <span>Username</span>
                <input bind:value={newUser.username} autocomplete="off" spellcheck="false" />
              </label>
              <label>
                <span>Password</span>
                <input type="password" bind:value={newUser.password} autocomplete="new-password" />
              </label>
              <label>
                <span>Confirm</span>
                <input type="password" bind:value={newUser.confirm} autocomplete="new-password" />
              </label>
              {#if userError}
                <div class="notice user-error">
                  <AlertTriangle size={16} />
                  <span>{userError}</span>
                </div>
              {/if}
              <div class="button-row compact">
                <button class="primary" type="submit" disabled={userLoading || !newUser.username.trim() || newUser.password.length < 8 || newUser.confirm.length < 8}>
                  <User size={15} />
                  Add User
                </button>
              </div>
            </form>
          </article>

          <article class="panel wide">
            <h2>User Accounts</h2>
            <table>
              <thead><tr><th>Username</th><th>Created</th><th>Updated</th><th>Password</th><th>Delete</th></tr></thead>
              <tbody>
                {#each users as user}
                  <tr>
                    <td>{user.username}</td>
                    <td>{formatDate(user.createdAt)}</td>
                    <td>{formatDate(user.updatedAt)}</td>
                    <td>
                      <div class="user-row-actions">
                        <input
                          type="password"
                          autocomplete="new-password"
                          placeholder="New password"
                          value={passwordDrafts[user.username] || ''}
                          on:input={(event) => setPasswordDraft(user.username, event.currentTarget.value)}
                        />
                        <button on:click={() => updateUserPassword(user.username)} disabled={userLoading || (passwordDrafts[user.username] || '').length < 8}>
                          <Save size={15} />
                          Set
                        </button>
                      </div>
                    </td>
                    <td>
                      <button on:click={() => deleteUser(user.username)} disabled={userLoading || user.username === session?.username || users.length <= 1} title="Delete user">
                        <Trash2 size={15} />
                      </button>
                    </td>
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
          <h2>Web CLI</h2>
          <form class="cli-form" on:submit|preventDefault={runConsoleCommand}>
            <label class="cwd-input">
              <span>cwd</span>
              <input bind:value={consoleCwd} autocomplete="off" spellcheck="false" />
            </label>
            <label class="command-input">
              <span>$</span>
              <input bind:value={consoleCommand} autocomplete="off" spellcheck="false" />
            </label>
            <button class="primary" type="submit" disabled={consoleRunning || !consoleCommand.trim()}>
              <Play size={15} />
              Run
            </button>
          </form>

          <div class="terminal">
            {#if terminalEntries.length === 0}
              <div class="terminal-empty">No commands executed in this browser session.</div>
            {/if}
            {#each terminalEntries as entry}
              <article class:failed={entry.exitCode !== 0}>
                <header>
                  <span>{entry.at}</span>
                  <strong>{entry.cwd || '/'} $ {entry.command}</strong>
                  <em>exit {entry.exitCode} · {entry.durationMillis || 0} ms{entry.timedOut ? ' · timeout' : ''}</em>
                </header>
                {#if entry.stdout}
                  <pre>{entry.stdout}{entry.stdoutTruncated ? '\n[stdout truncated]' : ''}</pre>
                {/if}
                {#if entry.stderr}
                  <pre class="stderr">{entry.stderr}{entry.stderrTruncated ? '\n[stderr truncated]' : ''}</pre>
                {/if}
                {#if !entry.stdout && !entry.stderr}
                  <pre class="muted-output">(no output)</pre>
                {/if}
              </article>
            {/each}
          </div>
        </section>

        <section class="panel console-panel">
          <h2>Operator Log</h2>
          <div class="console-log">
            {#each consoleLines as line}
              <div class={line.level}><span>{line.at}</span>{line.message}</div>
            {/each}
          </div>
        </section>
      {/if}
    </main>
  </div>
  {/if}
</div>
