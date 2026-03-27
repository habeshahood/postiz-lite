package api

import "net/http"

func handleAdminPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(adminPageHTML))
	}
}

const adminPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Postiz Admin</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    background: #0a0a0f;
    color: #e4e4e7;
    padding: 2rem;
    max-width: 900px;
    margin: 0 auto;
}
h1 { font-size: 1.8rem; margin-bottom: 0.5rem; }
.subtitle { color: #71717a; margin-bottom: 2rem; font-size: 0.9rem; }
.card {
    background: #18181b;
    border: 1px solid #27272a;
    border-radius: 12px;
    padding: 1.5rem;
    margin-bottom: 1.5rem;
}
.card h2 { font-size: 1.1rem; margin-bottom: 1rem; }
.tenant {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem 1rem;
    background: #0f0f13;
    border-radius: 8px;
    margin-bottom: 0.5rem;
}
.tenant-info { flex: 1; }
.tenant-id { font-weight: 600; }
.tenant-hosts { color: #a1a1aa; font-size: 0.85rem; margin-top: 2px; }
.btn {
    padding: 0.5rem 1rem;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 500;
}
.btn-primary { background: #7c3aed; color: white; }
.btn-primary:hover { background: #6d28d9; }
.btn-danger { background: #dc2626; color: white; }
.btn-danger:hover { background: #b91c1c; }
.btn-danger:disabled { background: #52525b; cursor: not-allowed; }
.form-group { margin-bottom: 0.75rem; }
.form-group label { display: block; font-size: 0.85rem; color: #a1a1aa; margin-bottom: 0.25rem; }
.form-group input {
    width: 100%;
    padding: 0.5rem 0.75rem;
    background: #0f0f13;
    border: 1px solid #3f3f46;
    border-radius: 8px;
    color: #e4e4e7;
    font-size: 0.9rem;
}
.form-group input:focus { outline: none; border-color: #7c3aed; }
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
.status { padding: 0.75rem; border-radius: 8px; margin-top: 1rem; font-size: 0.85rem; display: none; }
.status.success { display: block; background: #052e16; color: #4ade80; border: 1px solid #166534; }
.status.error { display: block; background: #450a0a; color: #f87171; border: 1px solid #991b1b; }
.empty { color: #52525b; text-align: center; padding: 2rem; }
.key-input { margin-bottom: 2rem; }
.key-input input {
    width: 100%; padding: 0.5rem 0.75rem; background: #0f0f13;
    border: 1px solid #3f3f46; border-radius: 8px; color: #e4e4e7;
    font-size: 0.9rem; font-family: monospace;
}
.key-input label { font-size: 0.85rem; color: #a1a1aa; margin-bottom: 0.25rem; display: block; }
.count { color: #71717a; font-size: 0.85rem; margin-left: 0.5rem; }
</style>
</head>
<body>
<h1>Postiz Admin</h1>
<p class="subtitle">Manage tenants across all your Postiz instances</p>

<div class="key-input">
    <label>Admin Key</label>
    <input type="password" id="adminKey" placeholder="Enter your ADMIN_KEY" />
</div>

<div class="card">
    <h2>Tenants <span class="count" id="tenantCount"></span></h2>
    <div id="tenantList"><div class="empty">Enter admin key to load tenants</div></div>
</div>

<div class="card">
    <h2>Add Tenant</h2>
    <form id="addForm">
        <div class="form-row">
            <div class="form-group">
                <label>Tenant ID</label>
                <input name="id" placeholder="my-client" required />
            </div>
            <div class="form-group">
                <label>Domain</label>
                <input name="host" placeholder="postiz.theirsite.com" required />
            </div>
        </div>
        <div class="form-row">
            <div class="form-group">
                <label>Admin Email</label>
                <input name="email" type="email" placeholder="admin@theirsite.com" required />
            </div>
            <div class="form-group">
                <label>Admin Password</label>
                <input name="password" type="password" placeholder="StrongPassword123" required />
            </div>
        </div>
        <div class="form-group">
            <label>Database URL</label>
            <input name="database_url" placeholder="postgresql://postiz-user:postiz-password@postiz-postgres:5432/postiz-TENANTID" required />
        </div>
        <div class="form-row">
            <div class="form-group">
                <label>Redis URL</label>
                <input name="redis_url" value="redis://postiz-redis:6379/2" />
            </div>
            <div class="form-group">
                <label>JWT Secret</label>
                <input name="jwt_secret" placeholder="auto-generated if empty" />
            </div>
        </div>
        <button type="submit" class="btn btn-primary">Create Tenant</button>
        <div id="addStatus" class="status"></div>
    </form>
</div>

<script>
const keyInput = document.getElementById('adminKey');
const tenantList = document.getElementById('tenantList');
const tenantCount = document.getElementById('tenantCount');
const addForm = document.getElementById('addForm');
const addStatus = document.getElementById('addStatus');

// Restore key from localStorage
keyInput.value = localStorage.getItem('postiz_admin_key') || '';
keyInput.addEventListener('input', () => {
    localStorage.setItem('postiz_admin_key', keyInput.value);
    loadTenants();
});

function headers() {
    return { 'Authorization': 'Bearer ' + keyInput.value, 'Content-Type': 'application/json' };
}

function makeTenantRow(t) {
    const row = document.createElement('div');
    row.className = 'tenant';

    const info = document.createElement('div');
    info.className = 'tenant-info';

    const idEl = document.createElement('div');
    idEl.className = 'tenant-id';
    idEl.textContent = t.id;

    const hostsEl = document.createElement('div');
    hostsEl.className = 'tenant-hosts';
    hostsEl.textContent = (t.hosts || []).join(', ');

    info.appendChild(idEl);
    info.appendChild(hostsEl);

    const btn = document.createElement('button');
    btn.className = 'btn btn-danger';
    btn.textContent = '\u00d7 Delete';
    btn.addEventListener('click', () => deleteTenant(t.id));

    row.appendChild(info);
    row.appendChild(btn);
    return row;
}

function setStatus(el, type, msg) {
    el.className = 'status ' + type;
    el.textContent = msg;
}

function setEmpty(msg) {
    const el = document.createElement('div');
    el.className = 'empty';
    el.textContent = msg;
    tenantList.replaceChildren(el);
}

async function loadTenants() {
    if (!keyInput.value) { setEmpty('Enter admin key to load tenants'); return; }
    try {
        const res = await fetch('/api/admin/tenants', { headers: headers() });
        if (!res.ok) { setEmpty('Invalid admin key'); return; }
        const tenants = await res.json();
        tenantCount.textContent = '(' + tenants.length + ')';
        if (tenants.length === 0) { setEmpty('No tenants yet'); return; }
        const frag = document.createDocumentFragment();
        tenants.forEach(t => frag.appendChild(makeTenantRow(t)));
        tenantList.replaceChildren(frag);
    } catch (e) {
        setEmpty('Error loading tenants');
    }
}

async function deleteTenant(id) {
    if (!confirm('Delete tenant "' + id + '"? This stops their service immediately.')) return;
    await fetch('/api/admin/tenants/' + id, { method: 'DELETE', headers: headers() });
    loadTenants();
}

addForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    const data = Object.fromEntries(new FormData(addForm));

    // Generate JWT secret if empty
    if (!data.jwt_secret) {
        data.jwt_secret = Array.from(crypto.getRandomValues(new Uint8Array(32))).map(b => b.toString(16).padStart(2,'0')).join('');
    }

    const body = {
        id: data.id,
        hosts: [data.host],
        database_url: data.database_url,
        redis_url: data.redis_url || 'redis://postiz-redis:6379',
        jwt_secret: data.jwt_secret,
        frontend_url: 'https://' + data.host,
        upload_dir: '/uploads/' + data.id,
        social: {},
        admin_email: data.email,
        admin_password: data.password,
    };

    try {
        const res = await fetch('/api/admin/tenants', { method: 'POST', headers: headers(), body: JSON.stringify(body) });
        const result = await res.json();
        if (res.ok) {
            setStatus(addStatus, 'success', 'Tenant "' + result.id + '" created! URL: ' + result.url);
            addForm.reset();
            loadTenants();
        } else {
            setStatus(addStatus, 'error', result.msg || 'Failed to create tenant');
        }
    } catch (e) {
        setStatus(addStatus, 'error', 'Network error: ' + e.message);
    }
});

// Load on page load
if (keyInput.value) loadTenants();
</script>
</body>
</html>`
