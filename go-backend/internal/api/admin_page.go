package api

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/habeshahood/postiz-lite/internal/tenant"
)

func handleAdminPage(tm *tenant.TenantManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := resolveAdminConfig(tm)
		script := fmt.Sprintf(`<script>window.__ADMIN_CONFIG=%s;</script>`,
			adminConfigJSON(cfg))
		page := strings.Replace(adminPageHTML, "</head>", script+"\n</head>", 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(page))
	}
}

func resolveAdminConfig(tm *tenant.TenantManager) adminConfig {
	baseDomain := os.Getenv("ADMIN_BASE_DOMAIN")
	if baseDomain == "" {
		// Fall back to the first known tenant's host if available
		if tenants := tm.List(); len(tenants) > 0 {
			if hosts, ok := tenants[0]["hosts"].([]string); ok && len(hosts) > 0 {
				h := hosts[0]
				// Strip the leading subdomain to get the base domain
				if idx := strings.Index(h, "."); idx != -1 {
					baseDomain = h[idx+1:]
				} else {
					baseDomain = h
				}
			}
		}
	}
	if baseDomain == "" {
		baseDomain = "example.com"
	}
	uploadBase := os.Getenv("UPLOAD_BASE_DIR")
	if uploadBase == "" {
		uploadBase = "/home/petros/postiz-uploads"
	}
	return adminConfig{
		BaseDomain: baseDomain,
		DBHost:     "127.0.0.1:5432",
		DBUser:     "postiz-user",
		RedisHost:  "127.0.0.1:6379",
		UploadBase: uploadBase,
	}
}

func adminConfigJSON(cfg adminConfig) string {
	return fmt.Sprintf(`{"baseDomain":%q,"dbHost":%q,"dbUser":%q,"redisHost":%q,"uploadBase":%q}`,
		cfg.BaseDomain, cfg.DBHost, cfg.DBUser, cfg.RedisHost, cfg.UploadBase)
}

const adminPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Postiz Admin</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0a0a0f;color:#e4e4e7;display:flex;flex-direction:column;align-items:center;min-height:100vh;padding:3rem 1.5rem}
.wrap{width:100%;max-width:560px}
h1{font-size:2.4rem;letter-spacing:-0.5px;margin-bottom:0.3rem}
.sub{color:#52525b;font-size:0.9rem;margin-bottom:2.5rem}
.key-bar{display:flex;gap:0.5rem;margin-bottom:2rem}
.key-bar input{flex:1;padding:0.6rem 1rem;background:#18181b;border:1px solid #27272a;border-radius:10px;color:#e4e4e7;font-family:monospace;font-size:0.85rem}
.key-bar input:focus{outline:none;border-color:#7c3aed}
.add-row{display:flex;gap:0;margin-bottom:2rem;background:#18181b;border:1px solid #27272a;border-radius:14px;overflow:hidden;align-items:center}
.add-row input{flex:1;padding:0.85rem 1rem;background:transparent;border:none;color:#e4e4e7;font-size:1.05rem}
.add-row input:focus{outline:none}
.add-row .suffix{color:#52525b;font-size:0.85rem;white-space:nowrap;padding-right:0.5rem}
.add-row button{padding:0.85rem 1.5rem;background:#7c3aed;border:none;color:white;font-weight:600;cursor:pointer;font-size:0.95rem}
.add-row button:hover{background:#6d28d9}
.add-row button:disabled{background:#3f3f46;cursor:wait}
.tenant{display:flex;justify-content:space-between;align-items:center;padding:0.85rem 1.1rem;background:#18181b;border:1px solid #27272a;border-radius:12px;margin-bottom:0.4rem;transition:border-color 0.15s}
.tenant:hover{border-color:#3f3f46}
.t-name{font-weight:600}
.t-url{color:#7c3aed;font-size:0.8rem;margin-top:1px}
.t-url a{color:#7c3aed;text-decoration:none}
.t-url a:hover{text-decoration:underline}
.del{width:28px;height:28px;background:transparent;border:1px solid #27272a;border-radius:7px;color:#52525b;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:1rem}
.del:hover{border-color:#dc2626;color:#dc2626;background:#450a0a}
.msg{padding:0.65rem 1rem;border-radius:10px;margin-bottom:1rem;font-size:0.85rem;display:none}
.msg.ok{display:block;background:#052e16;color:#4ade80;border:1px solid #166534}
.msg.err{display:block;background:#450a0a;color:#f87171;border:1px solid #991b1b}
.empty{color:#27272a;text-align:center;padding:2.5rem;font-size:0.9rem}
.section{color:#52525b;text-transform:uppercase;letter-spacing:1.5px;font-size:0.7rem;margin-bottom:0.75rem;font-weight:600}
</style>
</head>
<body>
<div class="wrap">
<h1>Postiz</h1>
<p class="sub">Social media for every site</p>

<div class="key-bar">
<input type="password" id="key" placeholder="Admin key" onchange="localStorage.setItem('pk',this.value);loadTenants()" />
</div>

<div id="msg" class="msg"></div>

<div class="add-row">
<input type="text" id="name" placeholder="site name" autocomplete="off" />
<span class="suffix" id="domainSuffix">.example.com</span>
<button id="addBtn" onclick="add()">Add</button>
</div>

<div class="section">Sites</div>
<div id="list"><div class="empty">Connect with admin key</div></div>

<div style="margin-top:2.5rem">
<div class="section">Members</div>
<div style="margin-bottom:1rem">
  <select id="tenantSel" onchange="loadMembers()" style="padding:0.6rem 1rem;background:#18181b;border:1px solid #27272a;border-radius:10px;color:#e4e4e7;font-size:0.9rem;width:100%">
    <option value="">— select a site —</option>
  </select>
</div>
<div class="add-row" style="margin-bottom:1rem">
  <input type="text" id="mEmail" placeholder="email" autocomplete="off" />
  <input type="password" id="mPass" placeholder="password" style="flex:1;padding:0.85rem 1rem;background:transparent;border:none;border-left:1px solid #27272a;color:#e4e4e7;font-size:1.05rem" />
  <input type="text" id="mName" placeholder="name (optional)" style="flex:1;padding:0.85rem 1rem;background:transparent;border:none;border-left:1px solid #27272a;color:#e4e4e7;font-size:1.05rem" />
  <button id="addMemberBtn" onclick="addMember()">Add</button>
</div>
<div id="memberList"><div class="empty">Select a site to manage members</div></div>
</div>
</div>

<script>
const $=id=>document.getElementById(id);
const key=()=>$('key').value;
const hdr=()=>({Authorization:'Bearer '+key(),'Content-Type':'application/json'});

// Populate domain suffix from server-injected config
(function(){const cfg=window.__ADMIN_CONFIG||{};const s=$('domainSuffix');if(s)s.textContent='.'+(cfg.baseDomain||'example.com')})();

$('key').value=localStorage.getItem('pk')||'';
$('name').addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();add()}});
$('mEmail').addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();addMember()}});

function msg(t,ok){const m=$('msg');m.textContent=t;m.className='msg '+(ok?'ok':'err');setTimeout(()=>{m.style.display='none'},4000)}
function esc(s){const d=document.createElement('div');d.textContent=s;return d.innerHTML}

async function loadTenants(){
  if(!key()){$('list').innerHTML='<div class="empty">Connect with admin key</div>';return}
  try{
    const r=await fetch('/api/admin/tenants',{headers:hdr()});
    if(!r.ok){$('list').innerHTML='<div class="empty">Invalid key</div>';return}
    const d=await r.json();
    if(!d.length){$('list').innerHTML='<div class="empty">No sites yet</div>';return}
    $('list').innerHTML=d.map(t=>{
      const host=(t.hosts||[])[0]||'';
      const url=t.frontend_url||('https://'+host);
      return '<div class="tenant"><div><div class="t-name" style="cursor:pointer" onclick="selectTenant(\''+esc(t.id)+'\')">'+esc(t.id)+'</div><div class="t-url"><a href="'+esc(url)+'" target="_blank">'+esc(host)+'</a></div></div><button class="del" onclick="del(\''+esc(t.id)+'\')">&times;</button></div>';
    }).join('');
    // Repopulate tenant selector
    const sel=$('tenantSel');
    const prev=sel.value;
    sel.innerHTML='<option value="">— select a site —</option>'+d.map(t=>'<option value="'+esc(t.id)+'">'+esc(t.id)+'</option>').join('');
    if(prev)sel.value=prev;
  }catch(e){$('list').innerHTML='<div class="empty">Error</div>'}
}

function selectTenant(id){
  $('tenantSel').value=id;
  loadMembers();
}

async function loadMembers(){
  const tid=$('tenantSel').value;
  if(!tid){$('memberList').innerHTML='<div class="empty">Select a site to manage members</div>';return}
  try{
    const r=await fetch('/api/admin/tenants/'+tid+'/members',{headers:hdr()});
    if(!r.ok){$('memberList').innerHTML='<div class="empty">Failed to load</div>';return}
    const d=await r.json();
    if(!d.length){$('memberList').innerHTML='<div class="empty">No members yet</div>';return}
    $('memberList').innerHTML=d.map(u=>'<div class="tenant"><div><div class="t-name">'+esc(u.email)+'</div><div class="t-url">'+esc(u.role)+(u.name?(' — '+esc(u.name)):'')+'</div></div><button class="del" onclick="delMember(\''+esc(tid)+'\',\''+esc(u.id)+'\')">&times;</button></div>').join('');
  }catch(e){$('memberList').innerHTML='<div class="empty">Error</div>'}
}

async function addMember(){
  const tid=$('tenantSel').value;
  if(!tid){msg('Select a site first',false);return}
  const email=$('mEmail').value.trim();
  const pass=$('mPass').value;
  const name=$('mName').value.trim();
  if(!email||!pass){msg('Email and password required',false);return}
  const btn=$('addMemberBtn');btn.disabled=true;btn.textContent='...';
  try{
    const r=await fetch('/api/admin/tenants/'+tid+'/members',{method:'POST',headers:hdr(),body:JSON.stringify({email,password:pass,name})});
    const d=await r.json();
    if(r.ok){msg(email+' added',true);$('mEmail').value='';$('mPass').value='';$('mName').value='';loadMembers()}
    else msg(d.msg||'Failed',false)
  }catch(e){msg(e.message,false)}
  btn.disabled=false;btn.textContent='Add';
}

async function delMember(tid,uid){
  if(!confirm('Remove this member?'))return;
  await fetch('/api/admin/tenants/'+tid+'/members/'+uid,{method:'DELETE',headers:hdr()});
  loadMembers();
}

async function add(){
  const name=$('name').value.trim().toLowerCase().replace(/[^a-z0-9-]/g,'');
  if(!name){$('name').focus();return}
  const cfg=window.__ADMIN_CONFIG||{};
  const domain=name+'.'+(cfg.baseDomain||'example.com');
  const dbHost=cfg.dbHost||'127.0.0.1:5432';
  const dbUser=cfg.dbUser||'postiz-user';
  const redisHost=cfg.redisHost||'127.0.0.1:6379';
  const uploadBase=cfg.uploadBase||'/home/petros/postiz-uploads';
  const btn=$('addBtn');btn.disabled=true;btn.textContent='...';
  try{
    const r=await fetch('/api/admin/tenants',{method:'POST',headers:hdr(),body:JSON.stringify({
      id:name,hosts:[domain],
      database_url:'postgresql://'+dbUser+':postiz-password@'+dbHost+'/postiz-'+name,
      redis_url:'redis://'+redisHost,
      jwt_secret:[...crypto.getRandomValues(new Uint8Array(32))].map(b=>b.toString(16).padStart(2,'0')).join(''),
      frontend_url:'https://'+domain,
      upload_dir:uploadBase+'/'+name,
      social:{}
    })});
    const d=await r.json();
    if(r.ok){msg(name+' created',true);$('name').value='';loadTenants()}
    else msg(d.msg||'Failed',false)
  }catch(e){msg(e.message,false)}
  btn.disabled=false;btn.textContent='Add';
}

async function del(id){
  if(!confirm('Remove '+id+'?'))return;
  await fetch('/api/admin/tenants/'+id,{method:'DELETE',headers:hdr()});
  loadTenants();
}

if($('key').value)loadTenants();
</script>
</body>
</html>`
