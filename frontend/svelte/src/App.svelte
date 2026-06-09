<script>
  import { onMount, tick } from 'svelte';
  import {
    Activity,
    AlertTriangle,
    Cpu,
    Database,
    Download,
    KeyRound,
    LogIn,
    LogOut,
    Package,
    Pencil,
    Plus,
    Play,
    RefreshCw,
    Save,
    Search,
    Server,
    Settings,
    Shield,
    Square,
    Trash2,
    User,
    UserPlus,
    Wifi,
    X,
    Zap
  } from '@lucide/svelte';
  import { api, clearToken, login, postJSON, readToken } from './api.js';

  const tabs = [
    { id: 'overview', label: 'Overview', icon: Server },
    { id: 'systemd', label: 'Systemd', icon: Activity },
    { id: 'network', label: 'Network', icon: Wifi },
    { id: 'firewall', label: 'Firewall', icon: Shield },
    { id: 'packages', label: 'Packages', icon: Package },
    { id: 'llm', label: 'Local LLM', icon: Cpu },
    { id: 'services', label: 'Services', icon: Settings },
    { id: 'users', label: 'Users', icon: User },
    { id: 'console', label: 'Console', icon: Database }
  ];

  const aptSearchPageSize = 10;

  let activeTab = 'overview';
  let authenticated = !!readToken();
  let session = null;
  let loginUsername = 'root';
  let loginPassword = '';
  let loginLoading = false;
  let loginError = '';
  let loginFocusedField = '';
  let loginFailedReaction = false;
  let loginReactionTimer = null;
  let alerts = [];
  let nextAlertID = 1;
  let loading = false;
  let liveLoading = false;
  let error = '';
  let savedAt = '';
  let version = null;
  let status = null;
  let services = [];
  let systemd = null;
  let systemdLoading = false;
  let network = null;
  let networkDraft = null;
  let previousNetworkSample = null;
  let firewall = null;
  let firewallDraft = null;
  let firewallLoading = false;
  let firewallRuleDraft = defaultFirewallRule();
  let aptState = null;
  let aptSearchQuery = 'curl';
  let aptSearchResults = [];
  let aptSearchPage = 1;
  let aptSearchWarning = '';
  let aptSearchLoading = false;
  let aptActionLoading = '';
  let aptActionPackage = '';
  let aptIndexUpdating = false;
  let ollama = null;
  let history = [];
  let ollamaActionLoading = '';
  let modelSearchQuery = 'llama';
  let modelSearchResults = [];
  let modelSearchSource = '';
  let modelSearchWarning = '';
  let modelSearchLoading = false;
  let modelPulling = '';
  let modelRunning = '';
  let modelStopping = '';
  let consoleLines = [];
  let consoleCommand = '';
  let consoleCwd = '/';
  let consoleRunning = false;
  let activeConsoleCommand = null;
  let activeConsoleRunID = '';
  let consoleElapsedMillis = 0;
  let consoleElapsedTimer = null;
  let consoleStopping = false;
  let terminalInput = null;
  let terminalViewport = null;
  let terminalEntries = [];
  let nextTerminalEntryID = 1;
  let users = [];
  let permissionOptions = [
    { id: 'node.read', label: 'Node status', description: 'Read node status, metrics, and inventory' },
    { id: 'network.manage', label: 'Network', description: 'Preview, save, and apply network settings' },
    { id: 'firewall.manage', label: 'Firewall', description: 'Preview, save, and apply host firewall rules' },
    { id: 'packages.manage', label: 'Packages', description: 'Install, remove, and upgrade APT packages' },
    { id: 'llm.manage', label: 'Local LLM', description: 'Read and apply local LLM runtime tuning' },
    { id: 'services.manage', label: 'Services', description: 'Read service state and perform service operations' },
    { id: 'console.run', label: 'Console', description: 'Run commands through the web console' }
  ];
  let userLoading = false;
  let userError = '';
  const defaultUserPermissions = ['node.read'];
  let showAddUser = false;
  let newUser = { username: '', password: '', confirm: '', permissions: [...defaultUserPermissions] };
  let editingUsername = '';
  let userEditDraft = { username: '', password: '', confirm: '', permissions: [...defaultUserPermissions] };

  function clone(value) {
    return JSON.parse(JSON.stringify(value || {}));
  }

  function pushConsole(level, message) {
    consoleLines = [
      { at: new Date().toLocaleTimeString(), level, message },
      ...consoleLines
    ].slice(0, 80);
  }

  function showErrorAlert(message, title = 'Error') {
    const text = String(message || '').trim();
    if (!text) return;
    const id = nextAlertID;
    nextAlertID += 1;
    alerts = [
      { id, title, message: text, at: new Date().toLocaleTimeString() },
      ...alerts
    ].slice(0, 4);
    window.setTimeout(() => dismissAlert(id), 7000);
  }

  function dismissAlert(id) {
    alerts = alerts.filter((alert) => alert.id !== id);
  }

  function setUserError(message) {
    userError = message;
    showErrorAlert(message, 'Users');
  }

  function resetNodeState() {
    version = null;
    status = null;
    services = [];
    systemd = null;
    systemdLoading = false;
    users = [];
    network = null;
    networkDraft = null;
    previousNetworkSample = null;
    firewall = null;
    firewallDraft = null;
    firewallLoading = false;
    firewallRuleDraft = defaultFirewallRule();
    aptState = null;
    aptSearchResults = [];
    aptSearchPage = 1;
    aptSearchWarning = '';
    aptSearchLoading = false;
    aptActionLoading = '';
    aptActionPackage = '';
    aptIndexUpdating = false;
    ollama = null;
    history = [];
    ollamaActionLoading = '';
    modelSearchResults = [];
    modelSearchSource = '';
    modelSearchWarning = '';
    modelSearchLoading = false;
    modelPulling = '';
    modelRunning = '';
    modelStopping = '';
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
      showErrorAlert(err.message, 'Console error');
    }
  }

  async function submitLogin() {
    loginLoading = true;
    loginError = '';
    loginFailedReaction = false;
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
      triggerLoginFailureReaction();
      showErrorAlert(err.message, 'Sign in failed');
    } finally {
      loginLoading = false;
    }
  }

  function triggerLoginFailureReaction() {
    if (loginReactionTimer) {
      window.clearTimeout(loginReactionTimer);
    }
    loginFailedReaction = true;
    loginReactionTimer = window.setTimeout(() => {
      loginFailedReaction = false;
      loginReactionTimer = null;
    }, 1800);
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
      const [nextVersion, nextStatus, nextServices, nextSystemd, nextNetwork, nextFirewall, nextPackages, nextOllama, nextHistory] = await Promise.all([
        api('/api2/json/version'),
        api('/api2/json/nodes/localhost/status'),
        api('/api2/json/nodes/localhost/services'),
        api('/api2/json/nodes/localhost/systemd/status'),
        api('/api2/json/nodes/localhost/network/status'),
        api('/api2/json/nodes/localhost/firewall/status'),
        api('/api2/json/nodes/localhost/packages/status'),
        api('/api2/json/nodes/localhost/ollama/status'),
        api('/api2/json/nodes/localhost/ollama/history?limit=120')
      ]);
      const [nextSession, nextUsers, nextPermissions] = await Promise.all([
        api('/api2/json/access/session'),
        api('/api2/json/access/users'),
        api('/api2/json/access/permissions')
      ]);
      version = nextVersion;
      session = nextSession;
      status = nextStatus;
      services = nextServices || [];
      systemd = nextSystemd;
      users = nextUsers || [];
      permissionOptions = nextPermissions || permissionOptions;
      network = withNetworkRates(nextNetwork);
      networkDraft = clone(nextNetwork?.config);
      firewall = nextFirewall;
      firewallDraft = normalizeFirewallDraft(nextFirewall?.config);
      aptState = nextPackages;
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

  async function refreshSystemdStatus() {
    if (systemdLoading) return;
    systemdLoading = true;
    try {
      systemd = await api('/api2/json/nodes/localhost/systemd/status');
      savedAt = new Date().toLocaleTimeString();
      const state = systemd?.summary?.state || systemd?.status || 'unknown';
      pushConsole(systemd?.status === 'ok' ? 'info' : 'warn', `systemctl status ${state}`);
    } catch (err) {
      handleAPIError(err);
    } finally {
      systemdLoading = false;
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

  function defaultFirewallRule() {
    return {
      name: '',
      enabled: true,
      direction: 'in',
      action: 'accept',
      protocol: 'tcp',
      port: '',
      source: '',
      destination: '',
      comment: ''
    };
  }

  function normalizeFirewallRule(rule = {}) {
    return {
      name: rule.name || '',
      enabled: rule.enabled !== false,
      direction: rule.direction || 'in',
      action: rule.action || 'accept',
      protocol: rule.protocol || 'tcp',
      port: rule.port || '',
      source: rule.source || '',
      destination: rule.destination || '',
      comment: rule.comment || ''
    };
  }

  function normalizeFirewallDraft(config = {}) {
    return {
      enabled: !!config.enabled,
      defaultIncoming: config.defaultIncoming || 'drop',
      defaultOutgoing: config.defaultOutgoing || 'accept',
      allowEstablished: config.allowEstablished !== false,
      allowLoopback: config.allowLoopback !== false,
      allowPing: config.allowPing !== false,
      rules: (config.rules || []).map(normalizeFirewallRule)
    };
  }

  async function refreshFirewallState() {
    firewallLoading = true;
    try {
      firewall = await api('/api2/json/nodes/localhost/firewall/status');
      firewallDraft = normalizeFirewallDraft(firewall?.config);
      savedAt = new Date().toLocaleTimeString();
      pushConsole('info', 'firewall state refreshed');
    } catch (err) {
      handleAPIError(err);
    } finally {
      firewallLoading = false;
    }
  }

  async function submitFirewall(dryRun, apply) {
    if (!firewallDraft || firewallLoading) return;
    firewallLoading = true;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/firewall/config', {
        ...firewallDraft,
        dryRun,
        apply
      });
      firewall = {
        ...(firewall || {}),
        config: result.config,
        rendered: result.rendered,
        ruleCount: result.config?.rules?.length || 0
      };
      firewallDraft = normalizeFirewallDraft(result.config);
      pushConsole(dryRun ? 'warn' : 'info', `firewall ${dryRun ? 'previewed' : 'saved'} rules=${firewallDraft.rules.length}`);
      if (result.applyResult) {
        const level = result.applyResult.status === 'failed' || result.applyResult.status === 'missing' ? 'error' : 'info';
        pushConsole(level, `firewall apply ${result.applyResult.status}`);
        if (level === 'error') {
          showErrorAlert(result.applyResult.error || 'firewall apply failed', 'Firewall');
        }
      }
      await refreshFirewallState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      firewallLoading = false;
    }
  }

  async function applyFirewallOnly(dryRun) {
    if (firewallLoading) return;
    firewallLoading = true;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/firewall/apply', { dryRun });
      const level = result.status === 'failed' || result.status === 'missing' ? 'error' : dryRun ? 'warn' : 'info';
      pushConsole(level, `firewall apply ${result.status}`);
      if (level === 'error') {
        showErrorAlert(result.error || 'firewall apply failed', 'Firewall');
      }
      await refreshFirewallState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      firewallLoading = false;
    }
  }

  function addFirewallRule() {
    if (!firewallDraft) return;
    const nextRule = normalizeFirewallRule(firewallRuleDraft);
    if (!nextRule.name.trim()) {
      nextRule.name = `${nextRule.protocol || 'rule'}-${nextRule.port || firewallDraft.rules.length + 1}`;
    }
    firewallDraft = {
      ...firewallDraft,
      rules: [...firewallDraft.rules, nextRule]
    };
    firewallRuleDraft = defaultFirewallRule();
  }

  function removeFirewallRule(index) {
    if (!firewallDraft) return;
    firewallDraft = {
      ...firewallDraft,
      rules: firewallDraft.rules.filter((_, ruleIndex) => ruleIndex !== index)
    };
  }

  async function refreshAptState() {
    aptState = await api('/api2/json/nodes/localhost/packages/status');
    savedAt = new Date().toLocaleTimeString();
  }

  function compactPackageOutput(value) {
    const lines = String(value || '').trim().split('\n').filter(Boolean);
    return lines.slice(-3).join(' / ').slice(0, 260);
  }

  function pushAptResult(result) {
    const packageName = result.package ? ` ${result.package}` : '';
    const level = result.status === 'ok' ? 'info' : result.status === 'dev-skip' ? 'warn' : 'error';
    pushConsole(level, `apt ${result.action}${packageName}: ${result.status}`);
    const output = compactPackageOutput(result.stderr || result.stdout);
    if (output) {
      pushConsole(level, output);
    }
    if (level === 'error') {
      showErrorAlert(output || `apt ${result.action}${packageName} failed`, 'Package action failed');
    }
  }

  async function searchAptPackages() {
    if (aptSearchLoading) return;
    aptSearchLoading = true;
    aptSearchWarning = '';
    try {
      const result = await api(`/api2/json/nodes/localhost/packages/search?q=${encodeURIComponent(aptSearchQuery.trim())}`);
      aptSearchResults = result.results || [];
      aptSearchPage = 1;
      aptSearchWarning = result.error || '';
      if (aptSearchWarning) {
        showErrorAlert(aptSearchWarning, 'Package search');
      }
      pushConsole('info', `apt search results=${aptSearchResults.length}`);
    } catch (err) {
      aptSearchResults = [];
      aptSearchPage = 1;
      aptSearchWarning = err.message;
      handleAPIError(err);
    } finally {
      aptSearchLoading = false;
    }
  }

  async function refreshAptIndex() {
    if (aptIndexUpdating || aptActionLoading) return;
    aptIndexUpdating = true;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/packages/index/update', {});
      pushAptResult(result);
      await refreshAptState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      aptIndexUpdating = false;
    }
  }

  async function installAptPackage(name) {
    const packageName = String(name || '').trim();
    if (!packageName || aptActionLoading) return;
    if (!window.confirm(`Install package ${packageName}?`)) return;
    aptActionLoading = 'install';
    aptActionPackage = packageName;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/packages/install', { name: packageName });
      pushAptResult(result);
      await refreshAptState();
      if (aptSearchQuery.trim()) {
        await searchAptPackages();
      }
    } catch (err) {
      handleAPIError(err);
    } finally {
      aptActionLoading = '';
      aptActionPackage = '';
    }
  }

  async function removeAptPackage(name) {
    const packageName = String(name || '').trim();
    if (!packageName || aptActionLoading) return;
    if (!window.confirm(`Remove package ${packageName}?`)) return;
    aptActionLoading = 'remove';
    aptActionPackage = packageName;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/packages/remove', { name: packageName });
      pushAptResult(result);
      await refreshAptState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      aptActionLoading = '';
      aptActionPackage = '';
    }
  }

  async function upgradeAptPackage(name) {
    const packageName = String(name || '').trim();
    if (!packageName || aptActionLoading) return;
    if (!window.confirm(`Update package ${packageName}?`)) return;
    aptActionLoading = 'upgrade';
    aptActionPackage = packageName;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/packages/upgrade', { name: packageName });
      pushAptResult(result);
      await refreshAptState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      aptActionLoading = '';
      aptActionPackage = '';
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

  async function refreshOllamaState() {
    const [nextOllama, nextHistory] = await Promise.all([
      api('/api2/json/nodes/localhost/ollama/status'),
      api('/api2/json/nodes/localhost/ollama/history?limit=120')
    ]);
    ollama = nextOllama;
    history = nextHistory || [];
    savedAt = new Date().toLocaleTimeString();
  }

  async function controlOllamaService(action) {
    if (ollamaActionLoading) return;
    ollamaActionLoading = action;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/ollama/service', { action });
      pushConsole(result.status === 'failed' ? 'error' : 'info', `ollama.service ${result.status}`);
      if (result.status === 'failed') {
        showErrorAlert(`ollama.service ${action} failed`, 'Ollama service');
      }
      await refreshOllamaState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      ollamaActionLoading = '';
    }
  }

  async function searchOllamaModels() {
    if (modelSearchLoading) return;
    modelSearchLoading = true;
    modelSearchWarning = '';
    try {
      const result = await api(`/api2/json/nodes/localhost/ollama/library/search?q=${encodeURIComponent(modelSearchQuery.trim())}`);
      modelSearchResults = result.results || [];
      modelSearchSource = result.source || '';
      modelSearchWarning = result.warning || '';
      if (modelSearchWarning) {
        showErrorAlert(modelSearchWarning, 'Model search');
      }
      pushConsole('info', `ollama model search results=${modelSearchResults.length} source=${modelSearchSource || 'unknown'}`);
    } catch (err) {
      modelSearchResults = [];
      modelSearchWarning = err.message;
      handleAPIError(err);
    } finally {
      modelSearchLoading = false;
    }
  }

  async function pullOllamaModel(model) {
    const name = String(model || '').trim();
    if (!name || modelPulling) return;
    modelPulling = name;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/ollama/models/pull', { model: name });
      if (result.readiness?.serviceStart?.status) {
        pushConsole('info', `ollama.service ${result.readiness.serviceStart.status}`);
      }
      pushConsole(result.installed ? 'info' : 'warn', `ollama install ${result.model}: ${result.status}`);
      if (result.ollama) {
        ollama = result.ollama;
      }
      await refreshOllamaState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      modelPulling = '';
    }
  }

  async function runOllamaModel(model) {
    const name = String(model || '').trim();
    if (!name || modelRunning) return;
    modelRunning = name;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/ollama/models/run', { model: name, keepAlive: '30m' });
      pushConsole('info', `ollama run requested: ${result.model}`);
      if (result.ollama) {
        ollama = result.ollama;
      }
      await refreshOllamaState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      modelRunning = '';
    }
  }

  async function stopOllamaModel(model) {
    const name = String(model || '').trim();
    if (!name || modelStopping) return;
    modelStopping = name;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/ollama/models/stop', { model: name });
      pushConsole('warn', `ollama stop requested: ${result.model}`);
      if (result.ollama) {
        ollama = result.ollama;
      }
      await refreshOllamaState();
    } catch (err) {
      handleAPIError(err);
    } finally {
      modelStopping = '';
    }
  }

  async function runConsoleCommand() {
    const command = consoleCommand.trim();
    if (!command || consoleRunning) return;
    const cwd = consoleCwd.trim() || '/';
    const entryID = nextTerminalEntryID;
    const runID = createConsoleRunID(entryID);
    nextTerminalEntryID += 1;
    consoleCommand = '';
    terminalEntries = [
      ...terminalEntries,
      {
        id: entryID,
        runID,
        at: new Date().toLocaleTimeString(),
        command,
        cwd,
        cancelled: false,
        exitCode: null,
        stderr: '',
        stdout: '',
        stopped: false,
        timedOut: false,
        pending: true
      }
    ].slice(-20);
    await scrollTerminalToBottom();
    consoleRunning = true;
    consoleStopping = false;
    activeConsoleRunID = runID;
    startConsoleProgress(command, cwd, runID);
    try {
      const result = await runTerminalCommand(command, cwd, runID);
      terminalEntries = terminalEntries.map((entry) => (
        entry.id === entryID
          ? { ...entry, ...result, pending: false }
          : entry
      ));
      const resultLabel = result.stopped
        ? 'command stopped'
        : result.timedOut
          ? 'command timed out'
          : `command exited ${result.exitCode}`;
      pushConsole(result.exitCode === 0 && !result.stopped && !result.timedOut ? 'info' : 'warn', `${resultLabel}: ${command}`);
      await scrollTerminalToBottom();
    } catch (err) {
      terminalEntries = terminalEntries.map((entry) => (
        entry.id === entryID
          ? {
              ...entry,
              cancelled: false,
              exitCode: -1,
              stderr: err.message,
              stdout: '',
              stopped: false,
              timedOut: false,
              pending: false
            }
          : entry
      ));
      handleAPIError(err);
      await scrollTerminalToBottom();
    } finally {
      stopConsoleProgress();
      consoleRunning = false;
      consoleStopping = false;
      activeConsoleRunID = '';
      focusTerminalInput();
    }
  }

  async function runTerminalCommand(command, cwd, runID) {
    if (isChangeDirectoryCommand(command)) {
      return changeTerminalDirectory(command, cwd, runID);
    }
    return postJSON('/api2/json/nodes/localhost/console/exec', { command, cwd, runID });
  }

  function isChangeDirectoryCommand(command) {
    if (command !== 'cd' && !command.startsWith('cd ')) return false;
    return !/[;&|<>`$()]/.test(command.slice(2));
  }

  async function changeTerminalDirectory(command, cwd, runID) {
    const target = command === 'cd' ? '~' : command.slice(2).trim() || '~';
    const result = await postJSON('/api2/json/nodes/localhost/console/exec', {
      command: `cd ${target} && pwd -P`,
      cwd,
      runID
    });
    if (result.exitCode === 0) {
      const nextCwd = String(result.stdout || '').trim().split('\n').filter(Boolean).pop();
      if (nextCwd) {
        consoleCwd = nextCwd;
      }
      return { ...result, command, cwd, stdout: '' };
    }
    return { ...result, command, cwd };
  }

  async function stopActiveConsoleCommand() {
    if (!consoleRunning || !activeConsoleRunID || consoleStopping) return;
    const runID = activeConsoleRunID;
    const command = activeConsoleCommand?.command || runID;
    consoleStopping = true;
    try {
      const result = await postJSON('/api2/json/nodes/localhost/console/stop', { runID });
      if (result.stopped) {
        pushConsole('warn', `command stop requested: ${command}`);
      } else {
        consoleStopping = false;
        pushConsole('warn', `no active command found to stop: ${command}`);
      }
    } catch (err) {
      consoleStopping = false;
      handleAPIError(err);
    }
  }

  function createConsoleRunID(entryID) {
    return `console-${Date.now()}-${entryID}`;
  }

  function focusTerminalInput() {
    window.setTimeout(() => terminalInput?.focus(), 0);
  }

  function focusTerminalOnClick(node) {
    function handleClick() {
      focusTerminalInput();
    }

    node.addEventListener('click', handleClick);
    return {
      destroy() {
        node.removeEventListener('click', handleClick);
      }
    };
  }

  async function scrollTerminalToBottom() {
    await tick();
    if (terminalViewport) {
      terminalViewport.scrollTop = terminalViewport.scrollHeight;
    }
  }

  function startConsoleProgress(command, cwd, runID) {
    stopConsoleProgress();
    const startedAt = Date.now();
    activeConsoleCommand = {
      command,
      cwd,
      at: new Date(startedAt).toLocaleTimeString(),
      runID,
      startedAt
    };
    consoleElapsedMillis = 0;
    consoleElapsedTimer = window.setInterval(() => {
      consoleElapsedMillis = Date.now() - startedAt;
    }, 500);
  }

  function stopConsoleProgress() {
    if (consoleElapsedTimer) {
      window.clearInterval(consoleElapsedTimer);
      consoleElapsedTimer = null;
    }
    activeConsoleCommand = null;
    consoleElapsedMillis = 0;
  }

  async function refreshUsers() {
    try {
      const [nextSession, nextUsers, nextPermissions] = await Promise.all([
        api('/api2/json/access/session'),
        api('/api2/json/access/users'),
        api('/api2/json/access/permissions')
      ]);
      session = nextSession;
      users = nextUsers || [];
      permissionOptions = nextPermissions || permissionOptions;
      userError = '';
    } catch (err) {
      userError = err.message;
      handleAPIError(err);
    }
  }

  function resetNewUser() {
    newUser = { username: '', password: '', confirm: '', permissions: [...defaultUserPermissions] };
  }

  function toggleAddUserForm() {
    userError = '';
    if (!canManageUsers) {
      showAddUser = false;
      setUserError('권한 부족: root 계정만 사용자를 추가할 수 있습니다.');
      return;
    }
    editingUsername = '';
    resetNewUser();
    showAddUser = true;
  }

  function cancelAddUser() {
    showAddUser = false;
    resetNewUser();
  }

  function handleModalKeydown(event, closeModal) {
    if (event.key === 'Escape' && !userLoading) {
      closeModal();
    }
  }

  function permissionLabel(id) {
    return permissionOptions.find((permission) => permission.id === id)?.label || id;
  }

  function orderedPermissionIDs(selected) {
    const next = permissionOptions
      .filter((permission) => selected.has(permission.id))
      .map((permission) => permission.id);
    return next.length > 0 ? next : [...defaultUserPermissions];
  }

  function permissionList(value) {
    const selected = new Set(Array.isArray(value) ? value : []);
    return orderedPermissionIDs(selected);
  }

  function permissionSummary(value) {
    return permissionList(value).map(permissionLabel).join(', ');
  }

  function setNewUserPermission(id, checked) {
    const selected = new Set(newUser.permissions || []);
    if (checked) {
      selected.add(id);
    } else {
      selected.delete(id);
    }
    newUser = { ...newUser, permissions: orderedPermissionIDs(selected) };
  }

  function isCurrentUser(username) {
    return username === session?.username;
  }

  function canEditAccount(username) {
    return canManageUsers || isCurrentUser(username);
  }

  function beginEditUser(user) {
    userError = '';
    if (!canEditAccount(user.username)) {
      editingUsername = '';
      setUserError('권한 부족: root 계정이 아니면 자기 계정만 수정할 수 있습니다.');
      return;
    }
    showAddUser = false;
    editingUsername = user.username;
    userEditDraft = {
      username: user.username,
      password: '',
      confirm: '',
      permissions: permissionList(user.permissions)
    };
  }

  function cancelEditUser() {
    editingUsername = '';
    userEditDraft = { username: '', password: '', confirm: '', permissions: [...defaultUserPermissions] };
  }

  function setEditUserPermission(id, checked) {
    const selected = new Set(userEditDraft.permissions || []);
    if (checked) {
      selected.add(id);
    } else {
      selected.delete(id);
    }
    userEditDraft = { ...userEditDraft, permissions: orderedPermissionIDs(selected) };
  }

  function editUserSaveDisabled() {
    if (userLoading) return true;
    const password = userEditDraft.password;
    const passwordInvalid = !!password && (password.length < 8 || password !== userEditDraft.confirm);
    if (passwordInvalid) return true;
    if (!canManageUsers) {
      return !password;
    }
    return false;
  }

  async function createUser() {
    userError = '';
    if (!canManageUsers) {
      setUserError('only root can manage users');
      return;
    }
    const username = newUser.username.trim();
    if (newUser.password !== newUser.confirm) {
      setUserError('password confirmation does not match');
      return;
    }
    userLoading = true;
    try {
      const created = await postJSON('/api2/json/access/users', {
        username,
        password: newUser.password,
        permissions: newUser.permissions
      });
      resetNewUser();
      showAddUser = false;
      pushConsole('info', `user created: ${created.username}`);
      await refreshUsers();
    } catch (err) {
      userError = err.message;
      handleAPIError(err);
    } finally {
      userLoading = false;
    }
  }

  async function updateUser() {
    userError = '';
    if (!canEditAccount(userEditDraft.username)) {
      setUserError('권한 부족: root 계정이 아니면 자기 계정만 수정할 수 있습니다.');
      return;
    }
    if (!userEditDraft.username) return;
    const password = userEditDraft.password;
    if (!canManageUsers && !password) {
      setUserError('변경할 비밀번호를 입력하세요.');
      return;
    }
    if (password || userEditDraft.confirm) {
      if (password !== userEditDraft.confirm) {
        setUserError('password confirmation does not match');
        return;
      }
      if (password.length < 8) {
        setUserError('password must be at least 8 characters');
        return;
      }
    }
    userLoading = true;
    try {
      const body = {
        username: userEditDraft.username
      };
      if (canManageUsers) {
        body.permissions = userEditDraft.permissions;
      }
      if (password) {
        body.password = password;
      }
      await postJSON('/api2/json/access/users/update', body);
      pushConsole('info', `user updated: ${userEditDraft.username}`);
      cancelEditUser();
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
    if (!canManageUsers) {
      setUserError('only root can manage users');
      return;
    }
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

  function formatMillis(value) {
    const millis = Math.max(0, Number(value || 0));
    if (millis < 1000) return `${Math.round(millis)} ms`;
    const totalSeconds = Math.floor(millis / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    if (minutes > 0) {
      return `${minutes}m ${String(seconds).padStart(2, '0')}s`;
    }
    return `${seconds}s`;
  }

  function formatAPIDate(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString();
  }

  function modelNameAliases(value) {
    const normalized = String(value || '').trim().toLowerCase();
    if (!normalized) return [];
    const aliases = [normalized];
    if (normalized.endsWith(':latest')) {
      aliases.push(normalized.slice(0, -7));
    } else if (!normalized.includes(':')) {
      aliases.push(`${normalized}:latest`);
    }
    return aliases;
  }

  function modelNameInSet(value, set) {
    return modelNameAliases(value).some((name) => set.has(name));
  }

  function loadedModelFor(value) {
    const aliases = new Set(modelNameAliases(value));
    return loadedModels.find((model) =>
      [...modelNameAliases(model.model), ...modelNameAliases(model.name)].some((name) => aliases.has(name))
    );
  }

  function modelDetailsSummary(model) {
    const details = model?.details || {};
    return [details.family, details.parameterSize, details.quantizationLevel].filter(Boolean).join(' / ') || '-';
  }

  function stateClass(state) {
    if (state === 'active' || state === 'ready' || state === 'api-online' || state === 'restarted' || state === 'running' || state === 'online' || state === 'ok') return 'ok';
    if (state === 'failed' || state === 'missing' || state === 'timeout' || state === 'degraded') return 'bad';
    return 'warn';
  }

  $: memory = status?.memory || {};
  $: swap = status?.swap || {};
  $: cpu = status?.cpu || {};
  $: os = status?.os || {};
  $: controlPlane = status?.controlPlane || {};
  $: systemdSummary = systemd?.summary || {};
  $: terminalUser = session?.username || 'root';
  $: terminalHost = os.hostname || 'localhost';
  $: installedAptPackages = aptState?.packages || [];
  $: aptTools = aptState?.tools || {};
  $: aptSearchPageCount = Math.max(1, Math.ceil(aptSearchResults.length / aptSearchPageSize));
  $: if (aptSearchPage > aptSearchPageCount) aptSearchPage = aptSearchPageCount;
  $: if (aptSearchPage < 1) aptSearchPage = 1;
  $: aptSearchStartIndex = (aptSearchPage - 1) * aptSearchPageSize;
  $: aptSearchEndIndex = Math.min(aptSearchStartIndex + aptSearchPageSize, aptSearchResults.length);
  $: paginatedAptSearchResults = aptSearchResults.slice(aptSearchStartIndex, aptSearchEndIndex);
  $: aptSearchPageNumbers = Array.from({ length: aptSearchPageCount }, (_, index) => index + 1);
  $: firewallConfig = firewall?.config || {};
  $: firewallTools = firewall?.tools || {};
  $: firewallService = firewall?.service || {};
  $: firewallRules = firewallDraft?.rules || [];
  $: enabledFirewallRules = firewallRules.filter((rule) => rule.enabled !== false);
  $: acceptedFirewallRules = enabledFirewallRules.filter((rule) => rule.action === 'accept');
  $: blockedFirewallRules = enabledFirewallRules.filter((rule) => rule.action === 'drop' || rule.action === 'reject');
  $: firewallDraftEnabled = firewallDraft?.enabled ?? firewallConfig.enabled;
  $: firewallDefaultIncoming = firewallDraft?.defaultIncoming || firewallConfig.defaultIncoming || 'drop';
  $: firewallDefaultOutgoing = firewallDraft?.defaultOutgoing || firewallConfig.defaultOutgoing || 'accept';
  $: firewallGuardState = !firewallDraftEnabled ? 'disabled' : firewallDefaultIncoming === 'accept' ? 'open' : 'guarded';
  $: firewallGuardLabel = firewallGuardState === 'disabled' ? 'Disabled' : firewallGuardState === 'open' ? 'Open ingress' : 'Guarded';
  $: kv = ollama?.kvCache || {};
  $: downloadedModels = ollama?.models || [];
  $: loadedModels = ollama?.loadedModels || [];
  $: loadedModelNameSet = new Set(loadedModels.flatMap((model) => [...modelNameAliases(model.model), ...modelNameAliases(model.name)]));
  $: installedModelNameSet = new Set(downloadedModels.flatMap((model) => modelNameAliases(model.name)));
  $: ollamaHeroState = ollama?.apiReachable ? 'api-online' : ollama?.service?.state || ollama?.platform?.state || 'unknown';
  $: ollamaHeroLabel = ollama?.apiReachable ? 'API online' : ollama?.service?.state || ollama?.platform?.state || 'Unknown';
  $: ollamaInstalledCount = ollama?.platform?.modelCount || downloadedModels.length;
  $: ollamaLoadedCount = ollama?.platform?.loadedModelCount || loadedModels.length;
  $: cpuReady = cpu.state === 'ready' || hasFiniteNumber(cpu.usedPercent);
  $: cpuUsedPercent = clampPercent(cpu.usedPercent);
  $: cpuGaugePercent = visibleGaugePercent(cpu.usedPercent, cpuReady);
  $: cpuUsageText = cpuReady ? formatPercent(cpu.usedPercent) : '-';
  $: memoryUsedPercent = ratioPercent(memory.used, memory.total);
  $: swapUsedPercent = ratioPercent(swap.used, swap.total);
  $: canManageUsers = !!session?.canManageUsers || session?.username === 'root';
  $: canEditDraftUser = !!editingUsername && (canManageUsers || editingUsername === session?.username);

  onMount(() => {
    if (authenticated) {
      refresh();
    }
    const interval = window.setInterval(refreshLive, 2000);
    return () => {
      window.clearInterval(interval);
      if (loginReactionTimer) {
        window.clearTimeout(loginReactionTimer);
      }
      stopConsoleProgress();
    };
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
    {#if authenticated}
      <nav class="top-nav" aria-label="Primary navigation">
        {#each tabs as tab}
          <button class:active={activeTab === tab.id} class="nav-item" on:click={() => (activeTab = tab.id)}>
            <svelte:component this={tab.icon} size={16} />
            <span>{tab.label}</span>
          </button>
        {/each}
      </nav>
    {/if}
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

  {#if alerts.length > 0}
    <div class="alert-stack" aria-live="assertive" aria-atomic="false">
      {#each alerts as alert (alert.id)}
        <section class="app-alert" role="alert">
          <div class="alert-icon">
            <AlertTriangle size={18} />
          </div>
          <div class="alert-body">
            <div class="alert-title">
              <strong>{alert.title}</strong>
              <span>{alert.at}</span>
            </div>
            <p>{alert.message}</p>
          </div>
          <button class="alert-close" type="button" on:click={() => dismissAlert(alert.id)} title="Dismiss alert">
            <X size={15} />
          </button>
        </section>
      {/each}
    </div>
  {/if}

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
          <input
            bind:value={loginUsername}
            autocomplete="username"
            on:focus={() => (loginFocusedField = 'username')}
            on:blur={() => (loginFocusedField = '')}
          />
        </label>
        <label>
          <span>Password</span>
          <input
            type="password"
            bind:value={loginPassword}
            autocomplete="current-password"
            on:focus={() => (loginFocusedField = 'password')}
            on:blur={() => (loginFocusedField = '')}
          />
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
      <div class="login-ambient" aria-hidden="true">
        <div
          class="login-character"
          class:watching-form={!!loginFocusedField}
          class:privacy-mode={loginFocusedField === 'password'}
          class:error-reaction={loginFailedReaction}
        >
          <div class="character-head">
            <div class="character-face">
              <span></span>
              <span></span>
            </div>
          </div>
          <div class="character-body"></div>
          <div class="character-base"></div>
        </div>
      </div>
    </main>
  {:else}
  <div class="workspace">
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
        <section class="overview-console grid two">
          <div class="firewall-hero overview-hero">
            <div class="firewall-hero-main">
              <span class="section-kicker">Node / Operator overview</span>
              <div class="firewall-hero-title">
                <Server size={26} />
                <div>
                  <h1>{os.hostname || 'Dionysus Node'}</h1>
                  <p>{controlPlane.stack || os.prettyName || 'control plane status'}</p>
                </div>
              </div>
              <div class="firewall-summary-grid">
                <div class={`metric-card ${status ? 'guarded' : 'disabled'}`}>
                  <span>Runtime</span>
                  <strong>{controlPlane.runtimeMode || 'unknown'}</strong>
                </div>
                <div class="metric-card">
                  <span>CPU</span>
                  <strong>{cpuUsageText}</strong>
                </div>
                <div class="metric-card">
                  <span>RAM</span>
                  <strong>{formatPercent(memoryUsedPercent)}</strong>
                </div>
                <div class="metric-card">
                  <span>Services</span>
                  <strong>{services.length}</strong>
                </div>
              </div>
            </div>

            <div class="firewall-companion overview-companion" aria-hidden="true">
              <div class="login-character overview-guide" class:watching-form={loading || liveLoading} class:privacy-mode={memoryUsedPercent >= 80 || swapUsedPercent >= 40}>
                <div class="character-head">
                  <div class="character-face">
                    <span></span>
                    <span></span>
                  </div>
                </div>
                <div class="character-body"></div>
                <div class="character-base"></div>
              </div>
            </div>
          </div>

          <article class="panel">
            <h2>Node</h2>
            <dl class="facts">
              <div><dt>Hostname</dt><dd>{os.hostname || '-'}</dd></div>
              <div><dt>OS</dt><dd>{os.prettyName || os.name || 'Dionysus target OS'}</dd></div>
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
                  <path class="gauge-fill cpu" class:idle={cpuReady && cpuUsedPercent === 0} d="M 18 90 A 76 76 0 0 1 170 90" pathLength="100" style={`stroke-dasharray: ${cpuGaugePercent} 100;`} />
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
                  <path class="gauge-fill ram" d="M 18 90 A 76 76 0 0 1 170 90" pathLength="100" style={`stroke-dasharray: ${clampPercent(memoryUsedPercent)} 100;`} />
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

          <article class="panel">
            <h2>Operations</h2>
            <dl class="facts">
              <div><dt>Firewall</dt><dd><span class={stateClass(firewallConfig.enabled ? 'active' : firewallService.state)}>{firewallConfig.enabled ? 'enabled' : firewallService.state || 'unknown'}</span></dd></div>
              <div><dt>Packages</dt><dd>{aptState?.installedCount ?? installedAptPackages.length} installed</dd></div>
              <div><dt>Ollama API</dt><dd><span class={stateClass(ollama?.apiReachable ? 'active' : ollama?.service?.state)}>{ollama?.apiStatus || ollama?.service?.state || 'unknown'}</span></dd></div>
              <div><dt>Services</dt><dd>{services.length} tracked</dd></div>
              <div><dt>Console</dt><dd><span class={session?.permissions?.includes('console.run') || canManageUsers ? 'ok' : 'warn'}>{session?.permissions?.includes('console.run') || canManageUsers ? 'available' : 'restricted'}</span></dd></div>
            </dl>
            <div class="button-row">
              <button class="primary" on:click={() => (activeTab = 'console')}>
                <Database size={15} />
                Open Console
              </button>
              <button on:click={() => (activeTab = 'systemd')}>
                <Activity size={15} />
                View Systemd
              </button>
            </div>
          </article>

          <article class="panel wide">
            <h2>Control Plane Paths</h2>
            <table>
              <tbody>
                <tr><th>Web root</th><td>{controlPlane.wwwRoot || '-'}</td></tr>
                <tr><th>Metrics DB</th><td>{controlPlane.metricsDb || '-'}</td></tr>
                <tr><th>Network config</th><td>{controlPlane.networkConfig || '-'}</td></tr>
                <tr><th>Firewall config</th><td>{firewallConfig.path || '-'}</td></tr>
                <tr><th>Ollama API</th><td>{ollama?.apiBase || '-'}</td></tr>
              </tbody>
            </table>
          </article>
        </section>
      {:else if activeTab === 'systemd'}
        <section class="grid two">
          <article class="panel">
            <div class="panel-title">
              <h2>Systemctl Status</h2>
              <button on:click={refreshSystemdStatus} disabled={systemdLoading || loading} title="Refresh systemctl status">
                <RefreshCw size={15} class={systemdLoading ? 'spin' : ''} />
                Refresh
              </button>
            </div>
            <dl class="facts">
              <div><dt>Command</dt><dd><code>{systemd?.command || 'systemctl status --no-pager --lines=80'}</code></dd></div>
              <div><dt>Status</dt><dd><span class={stateClass(systemd?.status)}>{systemd?.status || 'unknown'}</span></dd></div>
              <div><dt>Available</dt><dd><span class={systemd?.available ? 'ok' : 'bad'}>{systemd?.available ? 'yes' : 'no'}</span></dd></div>
              <div><dt>Exit code</dt><dd>{systemd?.exitCode ?? '-'}</dd></div>
              <div><dt>Duration</dt><dd>{formatMillis(systemd?.durationMillis)}</dd></div>
            </dl>
            {#if systemd?.status && systemd.status !== 'ok'}
              <div class="notice systemd-notice">
                <AlertTriangle size={16} />
                <span>{systemd?.stderr || systemd?.stdout || 'systemctl status did not complete successfully'}</span>
              </div>
            {/if}
          </article>

          <article class="panel">
            <h2>Manager Summary</h2>
            <dl class="facts">
              <div><dt>Host</dt><dd>{systemdSummary.host || os.hostname || '-'}</dd></div>
              <div><dt>State</dt><dd><span class={stateClass(systemdSummary.state || systemd?.status)}>{systemdSummary.stateText || systemdSummary.state || '-'}</span></dd></div>
              <div><dt>Units</dt><dd>{systemdSummary.units || '-'}</dd></div>
              <div><dt>Jobs</dt><dd>{systemdSummary.jobs || '-'}</dd></div>
              <div><dt>Failed</dt><dd>{systemdSummary.failed || '-'}</dd></div>
              <div><dt>Since</dt><dd>{systemdSummary.since || '-'}</dd></div>
              <div><dt>systemd</dt><dd>{systemdSummary.systemd || '-'}</dd></div>
              <div><dt>CGroup</dt><dd>{systemdSummary.cgroup || '-'}</dd></div>
            </dl>
          </article>

          <article class="panel wide">
            <h2>Raw Output</h2>
            <div class="command-output">
              {#if systemd?.stdout}
                <pre>{systemd.stdout}{systemd.stdoutTruncated ? '\n[stdout truncated]' : ''}</pre>
              {/if}
              {#if systemd?.stderr}
                <pre class="stderr">{systemd.stderr}{systemd.stderrTruncated ? '\n[stderr truncated]' : ''}</pre>
              {/if}
              {#if !systemd?.stdout && !systemd?.stderr}
                <span>No systemctl status output loaded.</span>
              {/if}
            </div>
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
      {:else if activeTab === 'firewall'}
        <section class="firewall-console">
          <div class="firewall-hero">
            <div class="firewall-hero-main">
              <span class="section-kicker">Security / Host firewall</span>
              <div class="firewall-hero-title">
                <Shield size={26} />
                <div>
                  <h1>Firewall Policy</h1>
                  <p>{firewallConfig.path || 'nftables policy'}</p>
                </div>
              </div>
              <div class="firewall-summary-grid">
                <div class={`metric-card ${firewallGuardState}`}>
                  <span>Policy</span>
                  <strong>{firewallGuardLabel}</strong>
                </div>
                <div class="metric-card">
                  <span>Default incoming</span>
                  <strong>{firewallDefaultIncoming}</strong>
                </div>
                <div class="metric-card">
                  <span>Default outgoing</span>
                  <strong>{firewallDefaultOutgoing}</strong>
                </div>
                <div class="metric-card">
                  <span>Managed rules</span>
                  <strong>{enabledFirewallRules.length}/{firewallRules.length}</strong>
                </div>
              </div>
            </div>

            <div class="firewall-companion" aria-hidden="true">
              <div class="login-character firewall-guardian">
                <div class="character-head">
                  <div class="character-face">
                    <span></span>
                    <span></span>
                  </div>
                </div>
                <div class="character-body"></div>
                <div class="character-shield">
                  <Shield size={38} />
                </div>
                <div class="character-base"></div>
              </div>
            </div>
          </div>

          {#if firewallConfig.error}
            <div class="notice firewall-notice">
              <AlertTriangle size={16} />
              <span>{firewallConfig.error}</span>
            </div>
          {/if}

          <div class="firewall-layout">
            <article class="panel firewall-policy-panel">
              <div class="panel-title">
                <h2>Policy defaults</h2>
                <button on:click={refreshFirewallState} disabled={firewallLoading || loading} title="Refresh firewall state">
                  <RefreshCw size={15} class={firewallLoading ? 'spin' : ''} />
                  Refresh
                </button>
              </div>
              {#if firewallDraft}
                <div class="policy-flow">
                  <div class="policy-node">
                    <span>Ingress fallback</span>
                    <strong>{firewallDraft.defaultIncoming}</strong>
                  </div>
                  <div class="policy-node">
                    <span>Rule match</span>
                    <strong>{acceptedFirewallRules.length} allow / {blockedFirewallRules.length} block</strong>
                  </div>
                  <div class="policy-node">
                    <span>Egress fallback</span>
                    <strong>{firewallDraft.defaultOutgoing}</strong>
                  </div>
                </div>
                <div class="form-grid firewall-policy-grid">
                  <label class="checkbox-row">
                    <input type="checkbox" bind:checked={firewallDraft.enabled} />
                    <span>Enabled</span>
                  </label>
                  <label>Default incoming
                    <select bind:value={firewallDraft.defaultIncoming}>
                      <option value="drop">Drop</option>
                      <option value="reject">Reject</option>
                      <option value="accept">Accept</option>
                    </select>
                  </label>
                  <label>Default outgoing
                    <select bind:value={firewallDraft.defaultOutgoing}>
                      <option value="accept">Accept</option>
                      <option value="drop">Drop</option>
                      <option value="reject">Reject</option>
                    </select>
                  </label>
                  <label class="checkbox-row">
                    <input type="checkbox" bind:checked={firewallDraft.allowEstablished} />
                    <span>Established</span>
                  </label>
                  <label class="checkbox-row">
                    <input type="checkbox" bind:checked={firewallDraft.allowLoopback} />
                    <span>Loopback</span>
                  </label>
                  <label class="checkbox-row">
                    <input type="checkbox" bind:checked={firewallDraft.allowPing} />
                    <span>ICMP ping</span>
                  </label>
                </div>
                <div class="button-row">
                  <button on:click={() => submitFirewall(true, false)} disabled={firewallLoading}>
                    <Settings size={15} />
                    Preview
                  </button>
                  <button on:click={() => submitFirewall(false, false)} disabled={firewallLoading}>
                    <Save size={15} />
                    Save
                  </button>
                  <button class="primary" on:click={() => submitFirewall(false, true)} disabled={firewallLoading}>
                    <Play size={15} />
                    Save and Apply
                  </button>
                </div>
              {:else}
                <p class="muted">No firewall policy loaded.</p>
              {/if}
            </article>

            <article class="panel firewall-runtime-panel">
              <h2>Runtime</h2>
              <dl class="facts">
                <div><dt>Config</dt><dd>{firewallConfig.path || '-'}</dd></div>
                <div><dt>Saved</dt><dd><span class={firewallConfig.exists ? 'ok' : 'warn'}>{firewallConfig.exists ? 'yes' : 'default'}</span></dd></div>
                <div><dt>State</dt><dd><span class={firewallConfig.enabled ? 'ok' : 'warn'}>{firewallConfig.enabled ? 'enabled' : 'disabled'}</span></dd></div>
                <div><dt>Service</dt><dd><span class={stateClass(firewallService.state)}>{firewallService.state || 'unknown'}</span></dd></div>
                <div><dt>nft</dt><dd><span class={firewallTools.nft ? 'ok' : 'bad'}>{firewallTools.nft ? 'available' : 'missing'}</span></dd></div>
                <div><dt>Rules</dt><dd>{firewall?.ruleCount ?? firewallRules.length}</dd></div>
              </dl>
              <div class="button-row">
                <button on:click={() => applyFirewallOnly(true)} disabled={firewallLoading}>
                  <Settings size={15} />
                  Preview Apply
                </button>
                <button on:click={() => applyFirewallOnly(false)} disabled={firewallLoading || !firewallTools.nft}>
                  <Play size={15} />
                  Apply Saved
                </button>
              </div>
            </article>
          </div>

          <article class="panel firewall-rules-panel">
            <div class="panel-title">
              <h2>Managed rules</h2>
              <span class="model-source">{firewallRules.length} rules</span>
            </div>
            {#if firewallDraft}
              <form class="firewall-rule-form" on:submit|preventDefault={addFirewallRule}>
                <label>
                  <span>Name</span>
                  <input bind:value={firewallRuleDraft.name} placeholder="ssh" />
                </label>
                <label>
                  <span>Direction</span>
                  <select bind:value={firewallRuleDraft.direction}>
                    <option value="in">In</option>
                    <option value="out">Out</option>
                  </select>
                </label>
                <label>
                  <span>Action</span>
                  <select bind:value={firewallRuleDraft.action}>
                    <option value="accept">Accept</option>
                    <option value="drop">Drop</option>
                    <option value="reject">Reject</option>
                  </select>
                </label>
                <label>
                  <span>Protocol</span>
                  <select bind:value={firewallRuleDraft.protocol}>
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                    <option value="icmp">ICMP</option>
                    <option value="any">Any</option>
                  </select>
                </label>
                <label>
                  <span>Port</span>
                  <input bind:value={firewallRuleDraft.port} placeholder="22 or 80,443" />
                </label>
                <label>
                  <span>Source</span>
                  <input bind:value={firewallRuleDraft.source} placeholder="0.0.0.0/0" />
                </label>
                <label>
                  <span>Destination</span>
                  <input bind:value={firewallRuleDraft.destination} placeholder="10.0.0.10" />
                </label>
                <button class="primary" type="submit" disabled={firewallLoading}>
                  <Plus size={15} />
                  Add
                </button>
              </form>
              <div class="table-scroll">
                <table class="firewall-table">
                  <thead><tr><th>On</th><th>Name</th><th>Dir</th><th>Action</th><th>Proto</th><th>Port</th><th>Source</th><th>Destination</th><th></th></tr></thead>
                  <tbody>
                    {#if firewallRules.length === 0}
                      <tr><td class="table-empty" colspan="9">No managed firewall rules</td></tr>
                    {:else}
                      {#each firewallRules as rule, index}
                        <tr>
                          <td class="check-cell"><input type="checkbox" bind:checked={rule.enabled} title="Toggle rule" /></td>
                          <td><input bind:value={rule.name} /></td>
                          <td>
                            <select bind:value={rule.direction}>
                              <option value="in">In</option>
                              <option value="out">Out</option>
                            </select>
                          </td>
                          <td>
                            <select bind:value={rule.action}>
                              <option value="accept">Accept</option>
                              <option value="drop">Drop</option>
                              <option value="reject">Reject</option>
                            </select>
                          </td>
                          <td>
                            <select bind:value={rule.protocol}>
                              <option value="tcp">TCP</option>
                              <option value="udp">UDP</option>
                              <option value="icmp">ICMP</option>
                              <option value="any">Any</option>
                            </select>
                          </td>
                          <td><input bind:value={rule.port} placeholder="22" /></td>
                          <td><input bind:value={rule.source} placeholder="0.0.0.0/0" /></td>
                          <td><input bind:value={rule.destination} placeholder="10.0.0.10" /></td>
                          <td>
                            <button on:click={() => removeFirewallRule(index)} disabled={firewallLoading} title="Delete rule">
                              <Trash2 size={15} />
                            </button>
                          </td>
                        </tr>
                      {/each}
                    {/if}
                  </tbody>
                </table>
              </div>
            {/if}
          </article>

          <div class="firewall-output-grid">
            <article class="panel">
              <h2>Rendered nftables config</h2>
              <div class="command-output firewall-output">
                {#if firewall?.rendered}
                  <pre>{firewall.rendered}</pre>
                {:else}
                  <span>No rendered firewall config loaded.</span>
                {/if}
              </div>
            </article>

            <article class="panel">
              <h2>Active ruleset</h2>
              <div class="command-output firewall-output">
                {#if firewall?.activeRuleset?.stdout}
                  <pre>{firewall.activeRuleset.stdout}{firewall.activeRuleset.truncated ? '\n[stdout truncated]' : ''}</pre>
                {:else if firewall?.activeRuleset?.error}
                  <pre class="stderr">{firewall.activeRuleset.error}</pre>
                {:else}
                  <span>{firewall?.activeRuleset?.available === false ? 'nft command is unavailable.' : 'No active ruleset output loaded.'}</span>
                {/if}
              </div>
            </article>
          </div>
        </section>
      {:else if activeTab === 'packages'}
        <section class="grid two">
          <article class="panel">
            <div class="panel-title">
              <h2>APT Packages</h2>
              <button on:click={refreshAptState} disabled={loading || !!aptActionLoading} title="Refresh package list">
                <RefreshCw size={15} />
                Refresh
              </button>
            </div>
            <dl class="facts">
              <div><dt>Installed</dt><dd>{aptState?.installedCount ?? installedAptPackages.length} packages</dd></div>
              <div><dt>Upgradeable</dt><dd>{aptState?.upgradeableCount || 0} packages</dd></div>
              <div><dt>APT tools</dt><dd><span class={aptState?.available ? 'ok' : 'bad'}>{aptState?.available ? 'available' : 'missing'}</span></dd></div>
              <div><dt>dpkg-query</dt><dd><span class={aptTools.dpkgQuery ? 'ok' : 'bad'}>{aptTools.dpkgQuery ? 'ready' : 'missing'}</span></dd></div>
              <div><dt>apt-cache</dt><dd><span class={aptTools.aptCache ? 'ok' : 'bad'}>{aptTools.aptCache ? 'ready' : 'missing'}</span></dd></div>
              <div><dt>apt-get</dt><dd><span class={aptTools.aptGet ? 'ok' : 'bad'}>{aptTools.aptGet ? 'ready' : 'missing'}</span></dd></div>
            </dl>
            {#if aptState?.error || aptState?.upgradeableError}
              <div class="notice package-notice">
                <AlertTriangle size={16} />
                <span>{aptState?.error || aptState?.upgradeableError}</span>
              </div>
            {/if}
          </article>

          <article class="panel">
            <div class="panel-title">
              <h2>Find and Install</h2>
              <button on:click={refreshAptIndex} disabled={aptIndexUpdating || !!aptActionLoading} title="Refresh APT package index">
                <RefreshCw size={15} class={aptIndexUpdating ? 'spin' : ''} />
                Refresh Index
              </button>
            </div>
            <form class="package-search" on:submit|preventDefault={searchAptPackages}>
              <label>
                <span>Search</span>
                <input bind:value={aptSearchQuery} placeholder="curl, nginx, sqlite3" />
              </label>
              <button class="primary" type="submit" disabled={aptSearchLoading}>
                <Search size={15} class={aptSearchLoading ? 'spin' : ''} />
                Search
              </button>
              <button type="button" on:click={() => installAptPackage(aptSearchQuery)} disabled={!aptSearchQuery.trim() || !!aptActionLoading}>
                <Download size={15} />
                Install
              </button>
            </form>
            {#if aptSearchWarning}
              <div class="notice package-notice">
                <AlertTriangle size={16} />
                <span>{aptSearchWarning}</span>
              </div>
            {/if}
            <table class="model-table package-table">
              <thead><tr><th>Package</th><th>State</th><th>Actions</th></tr></thead>
              <tbody>
                {#if aptSearchResults.length === 0}
                  <tr><td class="table-empty" colspan="3">No search results</td></tr>
                {:else}
                  {#each paginatedAptSearchResults as result (result.name)}
                    <tr>
                      <td class="model-name">
                        {result.name}
                        <small>{result.description || '-'}</small>
                      </td>
                      <td>
                        {#if result.installed}
                          <span class={result.upgradeable ? 'warn' : 'ok'}>{result.upgradeable ? 'update available' : 'installed'}</span>
                          {#if result.installedVersion}
                            <small>{result.installedVersion}</small>
                          {/if}
                        {:else}
                          <span class="warn">not installed</span>
                        {/if}
                      </td>
                      <td>
                        <button on:click={() => installAptPackage(result.name)} disabled={result.installed || !!aptActionLoading} title="Install package">
                          <Download size={15} />
                          {aptActionLoading === 'install' && aptActionPackage === result.name ? 'Installing' : 'Install'}
                        </button>
                      </td>
                    </tr>
                  {/each}
                {/if}
              </tbody>
            </table>
            {#if aptSearchResults.length > aptSearchPageSize}
              <div class="pager" aria-label="Package search pages">
                <span>{aptSearchStartIndex + 1}-{aptSearchEndIndex} of {aptSearchResults.length}</span>
                <div class="pager-pages">
                  {#each aptSearchPageNumbers as page}
                    <button
                      type="button"
                      class:active={aptSearchPage === page}
                      aria-current={aptSearchPage === page ? 'page' : undefined}
                      on:click={() => (aptSearchPage = page)}
                    >
                      {page}
                    </button>
                  {/each}
                </div>
              </div>
            {/if}
          </article>

          <article class="panel wide">
            <div class="panel-title">
              <h2>Installed Package List</h2>
              <span class="model-source">{aptState?.installedCount ?? installedAptPackages.length} installed</span>
            </div>
            <table class="model-table package-table">
              <thead><tr><th>Package</th><th>Version</th><th>Architecture</th><th>State</th><th>Actions</th></tr></thead>
              <tbody>
                {#if installedAptPackages.length === 0}
                  <tr><td class="table-empty" colspan="5">No installed packages detected</td></tr>
                {:else}
                  {#each installedAptPackages as pkg (pkg.name)}
                    <tr>
                      <td class="model-name">{pkg.name}</td>
                      <td>{pkg.version || '-'}</td>
                      <td>{pkg.architecture || '-'}</td>
                      <td>
                        <span class={pkg.upgradeable ? 'warn' : 'ok'}>{pkg.upgradeable ? 'update available' : 'current'}</span>
                        {#if pkg.candidateVersion}
                          <small>candidate {pkg.candidateVersion}</small>
                        {/if}
                      </td>
                      <td>
                        <div class="model-actions">
                          <button on:click={() => upgradeAptPackage(pkg.name)} disabled={!pkg.upgradeable || !!aptActionLoading} title="Update package">
                            <RefreshCw size={15} class={aptActionLoading === 'upgrade' && aptActionPackage === pkg.name ? 'spin' : ''} />
                            Update
                          </button>
                          <button on:click={() => removeAptPackage(pkg.name)} disabled={!!aptActionLoading} title="Remove package">
                            <Trash2 size={15} />
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  {/each}
                {/if}
              </tbody>
            </table>
          </article>
        </section>
      {:else if activeTab === 'llm'}
        <section class="llm-console">
          <div class="firewall-hero llm-hero">
            <div class="firewall-hero-main">
              <span class="section-kicker">Local LLM / Ollama</span>
              <div class="firewall-hero-title">
                <Cpu size={26} />
                <div>
                  <h1>Ollama Runtime</h1>
                  <p>{ollama?.apiBase || 'local model server'}</p>
                </div>
              </div>
              <div class="firewall-summary-grid">
                <div class={`metric-card ${ollama?.apiReachable ? 'guarded' : 'disabled'}`}>
                  <span>Runtime</span>
                  <strong>{ollamaHeroLabel}</strong>
                </div>
                <div class="metric-card">
                  <span>Service</span>
                  <strong>{ollama?.service?.state || 'unknown'}</strong>
                </div>
                <div class="metric-card">
                  <span>Installed</span>
                  <strong>{ollamaInstalledCount}</strong>
                </div>
                <div class="metric-card">
                  <span>Loaded</span>
                  <strong>{ollamaLoadedCount}</strong>
                </div>
              </div>
            </div>

            <div class="firewall-companion llm-companion" aria-hidden="true">
              <div class="login-character llm-guide" class:watching-form={ollama?.apiReachable} class:privacy-mode={ollamaLoadedCount > 0}>
                <div class="character-head">
                  <div class="character-face">
                    <span></span>
                    <span></span>
                  </div>
                </div>
                <div class="character-body"></div>
                <div class={`llm-core ${ollamaHeroState === 'api-online' ? 'online' : ''}`}>
                  <Cpu size={38} />
                </div>
                <div class="character-base"></div>
              </div>
            </div>
          </div>

          <div class="grid two">
          <article class="panel">
            <div class="panel-title">
              <h2>Ollama</h2>
              <div class="model-actions">
                <button on:click={() => controlOllamaService('start')} disabled={!!ollamaActionLoading} title="Start Ollama">
                  <Play size={15} />
                  Start
                </button>
                <button on:click={() => controlOllamaService('restart')} disabled={!!ollamaActionLoading} title="Restart Ollama">
                  <RefreshCw size={15} class={ollamaActionLoading === 'restart' ? 'spin' : ''} />
                  Restart
                </button>
                <button on:click={() => controlOllamaService('stop')} disabled={!!ollamaActionLoading} title="Stop Ollama">
                  <Square size={15} />
                  Stop
                </button>
              </div>
            </div>
            <dl class="facts">
              <div><dt>State</dt><dd><span class={stateClass(ollama?.platform?.state)}>{ollama?.platform?.state || 'not-detected'}</span></dd></div>
              <div><dt>Service</dt><dd><span class={stateClass(ollama?.service?.state)}>{ollama?.service?.state || 'unknown'}</span></dd></div>
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
            <div class="panel-title">
              <h2>Installed Models</h2>
              <button on:click={refreshOllamaState} disabled={loading || liveLoading} title="Refresh Ollama state">
                <RefreshCw size={15} />
                Refresh
              </button>
            </div>
            <table class="model-table">
              <thead><tr><th>Model</th><th>Runtime</th><th>Size</th><th>Details</th><th>Modified</th><th>Actions</th></tr></thead>
              <tbody>
                {#if downloadedModels.length === 0}
                  <tr><td class="table-empty" colspan="6">No installed models</td></tr>
                {:else}
                  {#each downloadedModels as model (model.name)}
                    <tr>
                      <td class="model-name">{model.name}</td>
                      <td>
                        <span class={stateClass(model.running || modelNameInSet(model.name, loadedModelNameSet) ? 'active' : 'inactive')}>
                          {model.running || modelNameInSet(model.name, loadedModelNameSet) ? 'loaded' : 'stopped'}
                        </span>
                        {#if loadedModelFor(model.name)?.expiresAt}
                          <small>until {formatAPIDate(loadedModelFor(model.name).expiresAt)}</small>
                        {/if}
                      </td>
                      <td>{formatBytes(model.size)}</td>
                      <td>{modelDetailsSummary(model)}</td>
                      <td>{formatAPIDate(model.modifiedAt)}</td>
                      <td>
                        <div class="model-actions">
                          <button on:click={() => runOllamaModel(model.name)} disabled={!ollama?.apiReachable || !!modelRunning || !!modelPulling} title="Load model">
                            <Play size={15} />
                            Run
                          </button>
                          <button on:click={() => stopOllamaModel(model.name)} disabled={!ollama?.apiReachable || !modelNameInSet(model.name, loadedModelNameSet) || !!modelStopping} title="Unload model">
                            <Square size={15} />
                            Stop
                          </button>
                        </div>
                      </td>
                    </tr>
                  {/each}
                {/if}
              </tbody>
            </table>
          </article>

          <article class="panel wide">
            <div class="panel-title">
              <h2>Install Models</h2>
              <span class="model-source">{modelSearchSource || 'local catalog'}</span>
            </div>
            <form class="model-search" on:submit|preventDefault={searchOllamaModels}>
              <label>
                <span>Search</span>
                <input bind:value={modelSearchQuery} placeholder="llama3.2, qwen3, nomic-embed-text" />
              </label>
              <button class="primary" type="submit" disabled={modelSearchLoading}>
                <Search size={15} class={modelSearchLoading ? 'spin' : ''} />
                Search
              </button>
              <button type="button" on:click={() => pullOllamaModel(modelSearchQuery)} disabled={!modelSearchQuery.trim() || !!modelPulling}>
                <Download size={15} />
                Install
              </button>
            </form>
            {#if modelSearchWarning}
              <div class="notice model-notice">
                <AlertTriangle size={16} />
                <span>{modelSearchWarning}</span>
              </div>
            {/if}
            <table class="model-table">
              <thead><tr><th>Model</th><th>Source</th><th>Local state</th><th>Actions</th></tr></thead>
              <tbody>
                {#if modelSearchResults.length === 0}
                  <tr><td class="table-empty" colspan="4">No search results</td></tr>
                {:else}
                  {#each modelSearchResults as result (result.pullName || result.name)}
                    <tr>
                      <td class="model-name">
                        {result.title || result.name}
                        <small>{result.description || result.pullName || result.name}</small>
                      </td>
                      <td>{result.source || '-'}</td>
                      <td>
                        {#if modelNameInSet(result.pullName || result.name, installedModelNameSet)}
                          <span class="ok">installed</span>
                        {:else}
                          <span class="warn">not installed</span>
                        {/if}
                        {#if modelNameInSet(result.pullName || result.name, loadedModelNameSet)}
                          <small>loaded</small>
                        {/if}
                      </td>
                      <td>
                        <button on:click={() => pullOllamaModel(result.pullName || result.name)} disabled={!!modelPulling} title="Install model">
                          <Download size={15} />
                          {modelPulling === (result.pullName || result.name) ? 'Installing' : 'Install'}
                        </button>
                      </td>
                    </tr>
                  {/each}
                {/if}
              </tbody>
            </table>
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
          </div>
        </section>
      {:else if activeTab === 'users'}
        <section class="grid two">
          <article class="panel wide">
            <div class="panel-title">
              <h2>User Accounts</h2>
              <button class="primary" on:click={toggleAddUserForm} disabled={userLoading}>
                <UserPlus size={15} />
                Add User
              </button>
            </div>
            {#if !canManageUsers}
              <div class="notice user-notice">
                <Shield size={16} />
                <span>Only root can add users, edit other users, or change permissions. You can edit your own password.</span>
              </div>
            {/if}
            {#if userError}
              <div class="notice user-notice">
                <AlertTriangle size={16} />
                <span>{userError}</span>
              </div>
            {/if}
            <table>
              <thead><tr><th>Username</th><th>Permissions</th><th>Created</th><th>Updated</th><th>Actions</th></tr></thead>
              <tbody>
                {#each users as user (user.username)}
                  <tr>
                    <td>{user.username}</td>
                    <td>{permissionSummary(user.permissions)}</td>
                    <td>{formatDate(user.createdAt)}</td>
                    <td>{formatDate(user.updatedAt)}</td>
                    <td>
                      <div class="user-row-actions">
                        <button on:click={() => beginEditUser(user)} disabled={userLoading}>
                          <Pencil size={15} />
                          Edit
                        </button>
                        <button on:click={() => deleteUser(user.username)} disabled={!canManageUsers || userLoading || user.username === session?.username || users.length <= 1} title="Delete user">
                          <Trash2 size={15} />
                        </button>
                      </div>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </article>
        </section>
        {#if showAddUser && canManageUsers}
          <div class="modal-backdrop" role="presentation" on:click={cancelAddUser}>
            <div
              class="modal-panel user-modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="add-user-title"
              tabindex="-1"
              on:click|stopPropagation
              on:keydown={(event) => handleModalKeydown(event, cancelAddUser)}
            >
              <header class="modal-header">
                <div>
                  <h2 id="add-user-title">Add User</h2>
                  <span>Create a console account and assign permissions.</span>
                </div>
                <button class="icon-button" type="button" on:click={cancelAddUser} disabled={userLoading} title="Close modal">
                  <X size={16} />
                </button>
              </header>
              <form class="user-form modal-user-form" on:submit|preventDefault={createUser}>
                <div class="user-form-grid">
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
                </div>
                <div class="permission-field">
                  <span>Permissions</span>
                  <div class="permission-grid">
                    {#each permissionOptions as permission}
                      <label title={permission.description}>
                        <input
                          type="checkbox"
                          checked={(newUser.permissions || []).includes(permission.id)}
                          on:change={(event) => setNewUserPermission(permission.id, event.currentTarget.checked)}
                        />
                        <span>{permission.label}</span>
                      </label>
                    {/each}
                  </div>
                </div>
                <div class="modal-actions">
                  <button type="button" on:click={cancelAddUser} disabled={userLoading}>
                    <X size={15} />
                    Cancel
                  </button>
                  <button class="primary" type="submit" disabled={userLoading || !newUser.username.trim() || newUser.password.length < 8 || newUser.confirm.length < 8}>
                    <UserPlus size={15} />
                    Create
                  </button>
                </div>
              </form>
            </div>
          </div>
        {/if}
        {#if editingUsername && canEditDraftUser}
          <div class="modal-backdrop" role="presentation" on:click={cancelEditUser}>
            <div
              class="modal-panel user-modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="edit-user-title"
              tabindex="-1"
              on:click|stopPropagation
              on:keydown={(event) => handleModalKeydown(event, cancelEditUser)}
            >
              <header class="modal-header">
                <div>
                  <h2 id="edit-user-title">Edit User</h2>
                  <span>{canManageUsers ? 'Update password and account permissions.' : 'Update your account password.'}</span>
                </div>
                <button class="icon-button" type="button" on:click={cancelEditUser} disabled={userLoading} title="Close modal">
                  <X size={16} />
                </button>
              </header>
              <form class="user-form modal-user-form" on:submit|preventDefault={updateUser}>
                <div class="user-form-grid">
                  <label>
                    <span>Username</span>
                    <input value={userEditDraft.username} disabled />
                  </label>
                  <label>
                    <span>New password</span>
                    <input type="password" bind:value={userEditDraft.password} autocomplete="new-password" placeholder="Leave blank to keep current password" />
                  </label>
                  <label>
                    <span>Confirm</span>
                    <input type="password" bind:value={userEditDraft.confirm} autocomplete="new-password" />
                  </label>
                </div>
                <div class="permission-field">
                  <span>Permissions</span>
                  <div class="permission-grid">
                    {#each permissionOptions as permission}
                      <label title={!canManageUsers ? 'Only root can change permissions' : userEditDraft.username === 'root' ? 'Root always has every permission' : permission.description}>
                        <input
                          type="checkbox"
                          checked={(userEditDraft.permissions || []).includes(permission.id)}
                          disabled={!canManageUsers || userEditDraft.username === 'root'}
                          on:change={(event) => setEditUserPermission(permission.id, event.currentTarget.checked)}
                        />
                        <span>{permission.label}</span>
                      </label>
                    {/each}
                  </div>
                </div>
                <div class="modal-actions">
                  <button type="button" on:click={cancelEditUser} disabled={userLoading}>
                    <X size={15} />
                    Cancel
                  </button>
                  <button class="primary" type="submit" disabled={editUserSaveDisabled()}>
                    <Save size={15} />
                    Save
                  </button>
                </div>
              </form>
            </div>
          </div>
        {/if}
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
          <div
            class="terminal"
            class:busy={consoleRunning}
            aria-busy={consoleRunning}
            bind:this={terminalViewport}
            use:focusTerminalOnClick
          >
            {#each terminalEntries as entry}
              <div class="terminal-entry" class:failed={!entry.pending && entry.exitCode !== 0 && !entry.stopped} class:stopped={entry.stopped}>
                <div class="terminal-command-line">
                  <span class="terminal-identity">{terminalUser}@{terminalHost}</span><span>:</span><span class="terminal-path">{entry.cwd || '/'}</span><strong>$</strong><span class="terminal-command-text">{entry.command}</span>
                </div>
                {#if entry.stdout}
                  <pre class="terminal-output">{entry.stdout}{entry.stdoutTruncated ? '\n[stdout truncated]' : ''}</pre>
                {/if}
                {#if entry.stderr}
                  <pre class="terminal-output stderr">{entry.stderr}{entry.stderrTruncated ? '\n[stderr truncated]' : ''}</pre>
                {/if}
                {#if entry.pending}
                  <div class="terminal-status-line">
                    {consoleStopping && entry.runID === activeConsoleRunID ? 'stopping' : 'running'} · {formatMillis(consoleElapsedMillis)}
                  </div>
                {:else if entry.stopped || entry.exitCode !== 0 || entry.timedOut || entry.stdoutTruncated || entry.stderrTruncated}
                  <div class="terminal-status-line">
                    {entry.stopped ? 'stopped' : `exit ${entry.exitCode}`} · {formatMillis(entry.durationMillis)}{entry.timedOut ? ' · timeout' : ''}
                  </div>
                {/if}
              </div>
            {/each}
            <form class="terminal-prompt" on:submit|preventDefault={runConsoleCommand}>
              <span class="terminal-identity">{terminalUser}@{terminalHost}</span><span>:</span><span class="terminal-path">{consoleCwd}</span>
              <strong>$</strong>
              <input
                bind:this={terminalInput}
                bind:value={consoleCommand}
                autocomplete="off"
                aria-label="Terminal command"
                disabled={consoleRunning}
                spellcheck="false"
              />
              {#if consoleRunning}
                <button
                  type="button"
                  class="terminal-stop-button"
                  title="Stop command"
                  aria-label="Stop running command"
                  disabled={consoleStopping}
                  on:click|stopPropagation={stopActiveConsoleCommand}
                >
                  <Square size={14} />
                </button>
              {/if}
            </form>
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
