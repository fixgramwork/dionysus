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
  let consoleElapsedMillis = 0;
  let consoleElapsedTimer = null;
  let terminalInput = null;
  let terminalViewport = null;
  let terminalEntries = [];
  let users = [];
  let permissionOptions = [
    { id: 'node.read', label: 'Node status', description: 'Read node status, metrics, and inventory' },
    { id: 'network.manage', label: 'Network', description: 'Preview, save, and apply network settings' },
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
      showErrorAlert(err.message, 'Sign in failed');
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
      const [nextVersion, nextStatus, nextServices, nextSystemd, nextNetwork, nextPackages, nextOllama, nextHistory] = await Promise.all([
        api('/api2/json/version'),
        api('/api2/json/nodes/localhost/status'),
        api('/api2/json/nodes/localhost/services'),
        api('/api2/json/nodes/localhost/systemd/status'),
        api('/api2/json/nodes/localhost/network/status'),
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
      pushConsole('info', `ollama pull ${result.model}: ${result.status}`);
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
    consoleCommand = '';
    consoleRunning = true;
    startConsoleProgress(command, cwd);
    try {
      const result = await runTerminalCommand(command, cwd);
      terminalEntries = [
        ...terminalEntries,
        {
          at: new Date().toLocaleTimeString(),
          ...result
        }
      ].slice(-20);
      pushConsole(result.exitCode === 0 ? 'info' : 'warn', `command exited ${result.exitCode}: ${command}`);
      await scrollTerminalToBottom();
    } catch (err) {
      terminalEntries = [
        ...terminalEntries,
        {
          at: new Date().toLocaleTimeString(),
          command,
          cwd,
          exitCode: -1,
          stderr: err.message,
          stdout: '',
          timedOut: false
        }
      ].slice(-20);
      handleAPIError(err);
      await scrollTerminalToBottom();
    } finally {
      stopConsoleProgress();
      consoleRunning = false;
      focusTerminalInput();
    }
  }

  async function runTerminalCommand(command, cwd) {
    if (isChangeDirectoryCommand(command)) {
      return changeTerminalDirectory(command, cwd);
    }
    return postJSON('/api2/json/nodes/localhost/console/exec', { command, cwd });
  }

  function isChangeDirectoryCommand(command) {
    if (command !== 'cd' && !command.startsWith('cd ')) return false;
    return !/[;&|<>`$()]/.test(command.slice(2));
  }

  async function changeTerminalDirectory(command, cwd) {
    const target = command === 'cd' ? '~' : command.slice(2).trim() || '~';
    const result = await postJSON('/api2/json/nodes/localhost/console/exec', {
      command: `cd ${target} && pwd -P`,
      cwd
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

  function focusTerminalInput() {
    window.setTimeout(() => terminalInput?.focus(), 0);
  }

  async function scrollTerminalToBottom() {
    await tick();
    if (terminalViewport) {
      terminalViewport.scrollTop = terminalViewport.scrollHeight;
    }
  }

  function startConsoleProgress(command, cwd) {
    stopConsoleProgress();
    const startedAt = Date.now();
    activeConsoleCommand = {
      command,
      cwd,
      at: new Date(startedAt).toLocaleTimeString(),
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

  function beginEditUser(user) {
    userError = '';
    if (!canManageUsers) {
      editingUsername = '';
      setUserError('권한 부족: root 계정만 사용자 비밀번호와 권한을 수정할 수 있습니다.');
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
    if (!canManageUsers) {
      setUserError('only root can manage users');
      return;
    }
    if (!userEditDraft.username) return;
    const password = userEditDraft.password;
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
        username: userEditDraft.username,
        permissions: userEditDraft.permissions
      };
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
  $: installedAptPackages = aptState?.packages || [];
  $: aptTools = aptState?.tools || {};
  $: aptSearchPageCount = Math.max(1, Math.ceil(aptSearchResults.length / aptSearchPageSize));
  $: if (aptSearchPage > aptSearchPageCount) aptSearchPage = aptSearchPageCount;
  $: if (aptSearchPage < 1) aptSearchPage = 1;
  $: aptSearchStartIndex = (aptSearchPage - 1) * aptSearchPageSize;
  $: aptSearchEndIndex = Math.min(aptSearchStartIndex + aptSearchPageSize, aptSearchResults.length);
  $: paginatedAptSearchResults = aptSearchResults.slice(aptSearchStartIndex, aptSearchEndIndex);
  $: aptSearchPageNumbers = Array.from({ length: aptSearchPageCount }, (_, index) => index + 1);
  $: kv = ollama?.kvCache || {};
  $: downloadedModels = ollama?.models || [];
  $: loadedModels = ollama?.loadedModels || [];
  $: loadedModelNameSet = new Set(loadedModels.flatMap((model) => [...modelNameAliases(model.model), ...modelNameAliases(model.name)]));
  $: installedModelNameSet = new Set(downloadedModels.flatMap((model) => modelNameAliases(model.name)));
  $: cpuReady = cpu.state === 'ready' || hasFiniteNumber(cpu.usedPercent);
  $: cpuUsedPercent = clampPercent(cpu.usedPercent);
  $: cpuGaugePercent = visibleGaugePercent(cpu.usedPercent, cpuReady);
  $: cpuUsageText = cpuReady ? formatPercent(cpu.usedPercent) : '-';
  $: memoryUsedPercent = ratioPercent(memory.used, memory.total);
  $: swapUsedPercent = ratioPercent(swap.used, swap.total);
  $: canManageUsers = !!session?.canManageUsers || session?.username === 'root';

  onMount(() => {
    if (authenticated) {
      refresh();
    }
    const interval = window.setInterval(refreshLive, 2000);
    return () => {
      window.clearInterval(interval);
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
        <section class="grid two">
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
              <h2>Downloaded Models</h2>
              <button on:click={refreshOllamaState} disabled={loading || liveLoading} title="Refresh Ollama state">
                <RefreshCw size={15} />
                Refresh
              </button>
            </div>
            <table class="model-table">
              <thead><tr><th>Model</th><th>Runtime</th><th>Size</th><th>Details</th><th>Modified</th><th>Actions</th></tr></thead>
              <tbody>
                {#if downloadedModels.length === 0}
                  <tr><td class="table-empty" colspan="6">No downloaded models</td></tr>
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
              <button type="button" on:click={() => pullOllamaModel(modelSearchQuery)} disabled={!ollama?.apiReachable || !modelSearchQuery.trim() || !!modelPulling}>
                <Download size={15} />
                Download
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
                          <span class="ok">downloaded</span>
                        {:else}
                          <span class="warn">not installed</span>
                        {/if}
                        {#if modelNameInSet(result.pullName || result.name, loadedModelNameSet)}
                          <small>loaded</small>
                        {/if}
                      </td>
                      <td>
                        <button on:click={() => pullOllamaModel(result.pullName || result.name)} disabled={!ollama?.apiReachable || !!modelPulling} title="Download model">
                          <Download size={15} />
                          {modelPulling === (result.pullName || result.name) ? 'Downloading' : 'Download'}
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
                <span>Only root can add users, edit user information, or change permissions.</span>
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
        {#if editingUsername && canManageUsers}
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
                  <span>Update password and account permissions.</span>
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
                      <label title={userEditDraft.username === 'root' ? 'Root always has every permission' : permission.description}>
                        <input
                          type="checkbox"
                          checked={(userEditDraft.permissions || []).includes(permission.id)}
                          disabled={userEditDraft.username === 'root'}
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
                  <button class="primary" type="submit" disabled={userLoading || (!!userEditDraft.password && (userEditDraft.password.length < 8 || userEditDraft.password !== userEditDraft.confirm))}>
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
          <div class="terminal" class:busy={consoleRunning} aria-busy={consoleRunning} bind:this={terminalViewport}>
            {#if activeConsoleCommand}
              <div class="terminal-progress" role="status" aria-live="polite">
                <div class="terminal-progress-box">
                  <RefreshCw size={22} class="spin" />
                  <div>
                    <strong>Command running</strong>
                    <code>{activeConsoleCommand.cwd || '/'} $ {activeConsoleCommand.command}</code>
                    <span>started {activeConsoleCommand.at} · {formatMillis(consoleElapsedMillis)} elapsed</span>
                  </div>
                </div>
              </div>
            {/if}
            {#if terminalEntries.length === 0}
              <div class="terminal-empty">No commands executed in this browser session.</div>
            {/if}
            {#each terminalEntries as entry}
              <article class:failed={entry.exitCode !== 0}>
                <header>
                  <span>{entry.at}</span>
                  <strong>{entry.cwd || '/'} $ {entry.command}</strong>
                  <em>exit {entry.exitCode} · {formatMillis(entry.durationMillis)}{entry.timedOut ? ' · timeout' : ''}</em>
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
            <form class="terminal-prompt" on:submit|preventDefault={runConsoleCommand}>
              <span>{consoleCwd}</span>
              <strong>$</strong>
              <input
                bind:this={terminalInput}
                bind:value={consoleCommand}
                autocomplete="off"
                aria-label="Terminal command"
                disabled={consoleRunning}
                spellcheck="false"
              />
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
