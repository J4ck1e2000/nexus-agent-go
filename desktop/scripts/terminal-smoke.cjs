// Exercise the actual application terminal manager in Electron against a
// loopback fixture. Only -F is injected to isolate the fixture's SSH config.
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { builtinModules } = require('node:module');

function waitFor(check, label, timeout = 15000) {
  return new Promise((resolve, reject) => {
    const start = Date.now();
    const timer = setInterval(() => {
      if (check()) { clearInterval(timer); resolve(); }
      else if (Date.now() - start > timeout) { clearInterval(timer); reject(new Error(`Timed out: ${label}`)); }
    }, 25);
  });
}

function run(executable, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(executable, args, { windowsHide: true, ...options });
    child.on('error', reject);
    child.on('exit', (code) => code === 0 ? resolve() : reject(new Error(`${path.basename(executable)} exited ${code}`)));
  });
}

async function nativeClient(configPath, managerPath, port, ptyModule) {
  const pty = require(ptyModule);
  const childProcess = require('node:child_process');
  const nativeExecFile = childProcess.execFile;
  const nativeSpawn = pty.spawn;
  let configChecks = 0;
  let systemConfigChecked = false;
  let ptyStarts = 0;
  const verifyEnvironment = (env) => {
    if (env.TERM !== 'xterm-256color' || env.NEXUS_SMOKE_APP_SECRET !== undefined) throw new Error('Incorrect SSH environment');
    if (process.platform === 'win32' && !Object.keys(env).some((key) => key.toUpperCase() === 'PROGRAMDATA')) {
      throw new Error('Missing Windows OpenSSH ProgramData');
    }
  };
  process.env.TERM = 'dumb';
  process.env.NEXUS_SMOKE_APP_SECRET = 'must-not-reach-ssh';
  childProcess.execFile = (file, args, options, callback) => {
    verifyEnvironment(options.env);
    if (!args.includes('-G')) throw new Error('Expected the real manager config preflight');
    configChecks++;
    if (process.platform === 'win32' && !systemConfigChecked) {
      systemConfigChecked = true;
      // This deliberately omits -F, reproducing the Windows ProgramData bug
      // in the ordinary system/user configuration path before fixture login.
      return nativeExecFile(file, ['-G', '--', '127.0.0.1'], options, (error, stdout, stderr) => {
        if (error) return callback(error, stdout, stderr);
        return nativeExecFile(file, ['-F', configPath, ...args], options, callback);
      });
    }
    return nativeExecFile(file, ['-F', configPath, ...args], options, callback);
  };
  pty.spawn = (file, args, options) => {
    verifyEnvironment(options.env);
    ptyStarts++;
    return nativeSpawn(file, ['-F', configPath, ...args], options);
  };
  const { LocalSshTerminalManager } = require(managerPath);
  let output = '';
  let exitCode;
  let sessionId;
  const manager = new LocalSshTerminalManager((channel, event) => {
    if (event.sessionId !== sessionId) return;
    if (channel === 'terminal:output') output = (output + event.data).slice(-32768);
    if (channel === 'terminal:exit') exitCode = event.exitCode;
  });
  try {
    const cases = [
      { alias: 'fixture', useSshConfig: true },
      { alias: 'fixture', useSshConfig: false },
      { alias: 'fixture-password', useSshConfig: false, password: true },
      { alias: 'fixture-hostkey', useSshConfig: true, confirmHost: true },
      { alias: 'fixture', useSshConfig: true, disconnect: true },
      { alias: 'fixture-wrongkey', useSshConfig: true, rejectHost: true },
    ];
    for (const scenario of cases) {
      output = '';
      exitCode = undefined;
      const started = await manager.start({ id: 1, sshHost: '127.0.0.1', sshPort: port }, {
        targetHost: scenario.alias, sshUser: 'terminal-smoke', useSshConfig: scenario.useSshConfig, cols: 100, rows: 30,
      });
      sessionId = started.sessionId;
      if (started.sshUser !== 'terminal-smoke') throw new Error('Incorrect resolved Linux account');
      manager.attach(sessionId);
      if (scenario.password) {
        await waitFor(() => output.toLowerCase().includes('password:'), 'interactive password prompt');
        manager.write(sessionId, 'fixture-password\r');
      }
      if (scenario.confirmHost) {
        await waitFor(() => output.includes('continue connecting'), 'host-key confirmation prompt');
        manager.write(sessionId, 'yes\r');
      }
      if (scenario.rejectHost) {
        await waitFor(() => exitCode !== undefined, 'changed host-key rejection');
        if (exitCode !== 255 || !output.includes('REMOTE HOST IDENTIFICATION HAS CHANGED')) {
          throw new Error('Changed host key was not rejected with visible diagnostics');
        }
        continue;
      }
      await waitFor(() => output.includes('fixture-ready'), 'SSH login/output');
      manager.write(sessionId, 'probe\r');
      await waitFor(() => output.includes('cols=100 rows=30'), 'terminal input');
      if (!output.includes('term=xterm-256color')) throw new Error('Incorrect remote terminal type');
      manager.resize(sessionId, 120, 40);
      const probing = setInterval(() => { if (exitCode === undefined) manager.write(sessionId, 'probe\r'); }, 100);
      try { await waitFor(() => output.includes('cols=120 rows=40'), 'terminal resize'); }
      finally { clearInterval(probing); }
      if (scenario.disconnect) manager.close(sessionId);
      else manager.write(sessionId, 'exit\r');
      await waitFor(() => exitCode !== undefined, 'terminal exit');
      if (!scenario.disconnect && exitCode !== 0) throw new Error(`SSH client exited ${exitCode}`);
    }
    if (configChecks !== cases.length || ptyStarts !== cases.length) throw new Error('Incomplete manager path coverage');
    console.log('PASS: actual Electron manager; filtered Windows environment, key/password auth, host-key prompt/rejection, config/direct modes, attach, input/output, resize, exit/disconnect');
  } catch (error) {
    // Fixture output cannot contain real user credentials.
    console.error(output);
    throw error;
  } finally {
    manager.closeAll();
    childProcess.execFile = nativeExecFile;
    pty.spawn = nativeSpawn;
  }
}

async function main() {
  if (process.argv[2] === '--native-client') return nativeClient(process.argv[3], process.argv[4], Number(process.argv[5]), process.argv[6]);
  const root = path.resolve(__dirname, '../..');
  const tempRoot = path.resolve(os.tmpdir());
  const dir = fs.mkdtempSync(path.join(tempRoot, 'nexus-terminal-smoke-'));
  let fixture;
  try {
    const executable = path.join(dir, process.platform === 'win32' ? 'fixture.exe' : 'fixture');
    await run('go', ['build', '-o', executable, './testdata/terminal_ssh_fixture.go'], {
      cwd: root, stdio: 'inherit', env: { ...process.env, GOCACHE: path.join(tempRoot, 'nexus-agent-go-gocache') },
    });
    fixture = spawn(executable, ['-dir', dir], { windowsHide: true, stdio: ['ignore', 'pipe', 'inherit'] });
    let ready = '';
    let fixtureError;
    fixture.on('error', (error) => { fixtureError = error; });
    fixture.stdout.on('data', (data) => { ready += data.toString(); });
    await waitFor(() => ready.includes('\n') || fixtureError, 'loopback fixture');
    if (fixtureError) throw fixtureError;
    const { port } = JSON.parse(ready.trim().split('\n')[0]);
    if (process.platform === 'win32') {
      // Windows OpenSSH checks ACLs, rather than Unix mode 0600. Protect only
      // the disposable fixture key; never modify a user's existing SSH files.
      await run('icacls.exe', [path.join(dir, 'identity'), '/inheritance:r', '/grant:r', `${os.userInfo().username}:F`], { stdio: 'inherit' });
    }
    const configPath = path.join(dir, 'config');
    const sshPath = (name) => path.join(dir, name).replaceAll('\\', '/');
    fs.writeFileSync(path.join(dir, 'known_hosts_new'), '');
    const common = [
      '  HostName 127.0.0.1', `  Port ${port}`, '  User terminal-smoke',
      `  IdentityFile "${sshPath('identity')}"`, '  IdentitiesOnly yes', '  ConnectTimeout 5',
    ];
    fs.writeFileSync(configPath, [
      'Host fixture', ...common, `  UserKnownHostsFile "${sshPath('known_hosts')}"`,
      '  StrictHostKeyChecking yes', '  BatchMode yes', '  PreferredAuthentications publickey', '',
      'Host fixture-password', ...common, `  UserKnownHostsFile "${sshPath('known_hosts')}"`,
      '  StrictHostKeyChecking yes', '  BatchMode no', '  PreferredAuthentications password',
      '  PubkeyAuthentication no', '  NumberOfPasswordPrompts 1', '',
      'Host fixture-hostkey', ...common, `  UserKnownHostsFile "${sshPath('known_hosts_new')}"`,
      '  StrictHostKeyChecking ask', '  BatchMode no', '  PreferredAuthentications publickey', '',
      'Host fixture-wrongkey', ...common, `  UserKnownHostsFile "${sshPath('known_hosts_wrong')}"`,
      '  StrictHostKeyChecking yes', '  BatchMode yes', '  PreferredAuthentications publickey', '',
    ].join('\n'));
    const moduleOption = process.argv.indexOf('--pty-module');
    const ptyModule = moduleOption >= 0 ? process.argv[moduleOption + 1] : require.resolve('node-pty');
    const { build } = await import('vite');
    await build({
      configFile: false, root: path.resolve(__dirname, '..'), logLevel: 'error',
      build: {
        outDir: path.join(dir, 'manager'), emptyOutDir: false, target: 'node22', minify: false,
        lib: { entry: path.join(root, 'desktop/electron/services/local-ssh-terminal.ts'), formats: ['cjs'], fileName: () => 'manager.cjs' },
        rollupOptions: {
          external: ['node-pty', ...builtinModules, ...builtinModules.map((name) => `node:${name}`)],
          output: { paths: { 'node-pty': ptyModule.replaceAll('\\', '/') } },
        },
      },
    });
    const managerPath = path.join(dir, 'manager', 'manager.cjs');
    const clientEnvironment = { ...process.env, ELECTRON_RUN_AS_NODE: '1' };
    if (process.argv.includes('--full-electron')) delete clientEnvironment.ELECTRON_RUN_AS_NODE;
    await run(require('electron'), [__filename, '--native-client', configPath, managerPath, String(port), ptyModule], {
      cwd: path.resolve(__dirname, '..'), stdio: 'inherit', env: clientEnvironment,
    });
  } finally {
    if (fixture && fixture.exitCode === null) {
      const exited = new Promise((resolve) => fixture.once('exit', resolve));
      fixture.kill();
      await exited;
    }
    const resolved = path.resolve(dir);
    if (path.dirname(resolved) !== tempRoot || !path.basename(resolved).startsWith('nexus-terminal-smoke-')) {
      throw new Error('Refusing to remove a directory outside the temporary fixture root');
    }
    fs.rmSync(resolved, { recursive: true, force: true });
  }
}

const electronRuntime = require('electron');
const fullElectron = typeof electronRuntime !== 'string';
const ready = fullElectron ? electronRuntime.app.whenReady() : Promise.resolve();
ready.then(main).then(() => {
  // ConPTY may keep a native worker handle open in Electron's Node mode.
  if (fullElectron) electronRuntime.app.exit(0);
  else if (process.argv[2] === '--native-client') process.exit(0);
}).catch((error) => {
  console.error(error.message);
  if (fullElectron) electronRuntime.app.exit(1);
  else if (process.argv[2] === '--native-client') process.exit(1);
  process.exitCode = 1;
});
