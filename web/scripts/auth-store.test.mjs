import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { build } from 'esbuild';

// Bundle the real store and token storage; replace only the API boundary.
const bundle = await build({
  absWorkingDir: fileURLToPath(new URL('..', import.meta.url)),
  entryPoints: ['src/store/authStore.ts'],
  bundle: true,
  write: false,
  platform: 'node',
  format: 'cjs',
  packages: 'external',
  define: { 'import.meta.env.VITE_API_BASE_URL': 'undefined' },
  plugins: [{
    name: 'auth-api-test-boundary',
    setup(builder) {
      builder.onResolve({ filter: /\/api\/auth$/ }, () => ({ path: 'auth-api', namespace: 'test' }));
      builder.onLoad({ filter: /.*/, namespace: 'test' }, () => ({ contents: 'export const authApi = __authTest.api;' }));
    },
  }],
});

const tokenKey = 'sdu_alumni_access_token';
const user = { id: 1, account: 'alumni', role: 'alumni' };
function setup(overrides = {}, stored = []) {
  const values = new Map(stored);
  const reads = [];
  const writes = [];
  const api = {
    login: async () => ({ access_token: 'signed-token', user: { ...user, role: 'super_admin' } }),
    me: async () => user,
    logout: async () => {},
    ...overrides,
  };
  const window = { localStorage: {
    getItem: (key) => { reads.push(key); return values.get(key) ?? null; },
    setItem: (key, value) => { writes.push(key); values.set(key, value); },
    removeItem: (key) => values.delete(key),
  } };
  const module = { exports: {} };
  new Function('module', 'exports', 'require', 'window', '__authTest', bundle.outputFiles[0].text)(
    module, module.exports, createRequire(import.meta.url), window, { api },
  );
  return { store: module.exports.useAuthStore, values, reads, writes };
}

test('login uses /auth/me and persists only the token', async () => {
  const { store, values, writes } = setup();
  assert.equal(await store.getState().login({ account: 'alumni', password: 'test-password' }), user);
  assert.equal(store.getState().user.role, 'alumni');
  assert.deepEqual([...values], [[tokenKey, 'signed-token']]);
  assert.deepEqual(writes, [tokenKey]);
});

test('refresh ignores forged local roles and user profiles', async () => {
  const { store, reads } = setup({}, [
    [tokenKey, 'signed-token'],
    ['role', 'super_admin'],
    ['user', JSON.stringify({ ...user, role: 'super_admin' })],
    ['auth-storage', JSON.stringify({ state: { user: { ...user, role: 'super_admin' } } })],
  ]);
  assert.equal(await store.getState().ensureCurrentUser(), user);
  assert.equal(store.getState().user.role, 'alumni');
  assert.deepEqual(reads, [tokenKey]);
});

test('failed current-user validation discards the new token and old role', async () => {
  const failure = new Error('account disabled');
  const { store, values } = setup({ me: async () => { throw failure; } });
  store.setState({ user: { ...user, role: 'super_admin' } });
  await assert.rejects(store.getState().login({ account: 'alumni', password: 'test-password' }), failure);
  assert.equal(store.getState().user, null);
  assert.equal(values.has(tokenKey), false);
  assert.equal(store.getState().loading, false);
});

test('refresh fails closed when the server rejects the session', async () => {
  const { store, values } = setup({ me: async () => { throw new Error('unauthorized'); } }, [[tokenKey, 'invalid-token']]);
  assert.equal(await store.getState().ensureCurrentUser(), null);
  assert.equal(store.getState().user, null);
  assert.equal(values.has(tokenKey), false);
});

test('logout clears the role and token even if its API call fails', async () => {
  const { store, values } = setup({ logout: async () => { throw new Error('offline'); } }, [[tokenKey, 'signed-token']]);
  store.setState({ user });
  await assert.rejects(store.getState().logout(), /offline/);
  assert.equal(store.getState().user, null);
  assert.equal(values.has(tokenKey), false);
});
