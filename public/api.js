export async function api(path, opts = {}) {
  const r = await fetch(`/api${path}`, {
    ...opts,
    headers: opts.body ? { "Content-Type": "application/json" } : {},
  });
  if (!r.ok) throw new Error(`${r.status}: ${await r.text()}`);
  return r.status === 204 ? null : r.json();
}

export const post = (path, body) => api(path, { method: "POST", body: JSON.stringify(body) });
export const patch = (path, body) => api(path, { method: "PATCH", body: JSON.stringify(body) });
export const del = (path) => api(path, { method: "DELETE" });
