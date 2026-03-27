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
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0a0a0f;color:#e4e4e7;display:flex;flex-direction:column;align-items:center;min-height:100vh;padding:3rem 1.5rem}
h1{font-size:2.2rem;letter-spacing:-0.5px;margin-bottom:0.3rem}
.sub{color:#52525b;font-size:0.9rem;margin-bottom:3rem}
.wrap{width:100%;max-width:640px}
.key-bar{display:flex;gap:0.5rem;margin-bottom:2.5rem}
.key-bar input{flex:1;padding:0.6rem 1rem;background:#18181b;border:1px solid #27272a;border-radius:10px;color:#e4e4e7;font-family:monospace;font-size:0.85rem}
.key-bar input:focus{outline:none;border-color:#7c3aed}
.key-bar button{padding:0.6rem 1.2rem;background:#27272a;border:none;border-radius:10px;color:#a1a1aa;cursor:pointer;font-size:0.85rem}
.key-bar button:hover{background:#3f3f46}
.add-bar{display:flex;gap:0.5rem;margin-bottom:2rem}
.add-bar input{flex:1;padding:0.75rem 1rem;background:#18181b;border:1px solid #27272a;border-radius:12px;color:#e4e4e7;font-size:1rem}
.add-bar input:focus{outline:none;border-color:#7c3aed}
.add-bar button{padding:0.75rem 1.5rem;background:#7c3aed;border:none;border-radius:12px;color:white;font-weight:600;cursor:pointer;font-size:1rem;white-space:nowrap}
.add-bar button:hover{background:#6d28d9}
.add-bar button:disabled{background:#3f3f46;cursor:wait}
.tenant{display:flex;justify-content:space-between;align-items:center;padding:1rem 1.25rem;background:#18181b;border:1px solid #27272a;border-radius:12px;margin-bottom:0.5rem;transition:border-color 0.15s}
.tenant:hover{border-color:#3f3f46}
.t-id{font-weight:600;font-size:1rem}
.t-host{color:#7c3aed;font-size:0.85rem;margin-top:2px}
.t-host a{color:#7c3aed;text-decoration:none}
.t-host a:hover{text-decoration:underline}
.del{width:32px;height:32px;background:transparent;border:1px solid #3f3f46;border-radius:8px;color:#71717a;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:1.1rem;transition:all 0.15s}
.del:hover{border-color:#dc2626;color:#dc2626;background:#450a0a}
.msg{padding:0.75rem 1rem;border-radius:10px;margin-bottom:1.5rem;font-size:0.85rem;display:none}
.msg.ok{display:block;background:#052e16;color:#4ade80;border:1px solid #166534}
.msg.err{display:block;background:#450a0a;color:#f87171;border:1px solid #991b1b}
.empty{color:#3f3f46;text-align:center;padding:3rem;font-size:0.95rem}
.count{color:#52525b;font-weight:400;font-size:1rem}
h2{font-size:1.1rem;color:#71717a;text-transform:uppercase;letter-spacing:1px;margin-bottom:1rem;font-weight:500}
</style>
</head>
<body>
<div class="wrap">
<h1>Postiz</h1>
<p class="sub">Multi-tenant social media management</p>

<div class="key-bar">
<input type="password" id="key" placeholder="Admin key" />
<button onclick="loadTenants()">Connect</button>
</div>

<div id="msg" class="msg"></div>

<div class="add-bar">
<input type="text" id="domain" placeholder="postiz.theirsite.com" />
<button id="addBtn" onclick="addTenant()">Add</button>
</div>

<h2>Sites <span class="count" id="count"></span></h2>
<div id="list"><div class="empty">Enter admin key and connect</div></div>
</div>

<script>
const $=id=>document.getElementById(id);
const key=()=>$('key').value;
const hdr=()=>({Authorization:'Bearer '+key(),'Content-Type':'application/json'});

$('key').value=localStorage.getItem('pk')||'';
$('key').addEventListener('change',()=>localStorage.setItem('pk',$('key').value));
$('domain').addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();addTenant()}});

function msg(text,ok){const m=$('msg');m.textContent=text;m.className='msg '+(ok?'ok':'err');setTimeout(()=>m.style.display='none',5000)}

async function loadTenants(){
  if(!key()){$('list').innerHTML='<div class="empty">Enter admin key</div>';return}
  try{
    const r=await fetch('/api/admin/tenants',{headers:hdr()});
    if(!r.ok){$('list').innerHTML='<div class="empty">Invalid key</div>';return}
    const d=await r.json();
    $('count').textContent='('+d.length+')';
    if(!d.length){$('list').innerHTML='<div class="empty">No sites yet — add one above</div>';return}
    $('list').innerHTML=d.map(t=>'<div class="tenant"><div><div class="t-id">'+esc(t.id)+'</div><div class="t-host"><a href="'+esc(t.frontend_url)+'" target="_blank">'+esc((t.hosts||[])[0]||'')+'</a></div></div><button class="del" onclick="del(\''+esc(t.id)+'\')" title="Remove">&times;</button></div>').join('');
  }catch(e){$('list').innerHTML='<div class="empty">Connection error</div>'}
}

function esc(s){const d=document.createElement('div');d.textContent=s;return d.innerHTML}

async function addTenant(){
  const domain=$('domain').value.trim();
  if(!domain){$('domain').focus();return}
  const id=domain.replace(/[^a-z0-9]/gi,'-').toLowerCase();
  const btn=$('addBtn');btn.disabled=true;btn.textContent='Creating...';
  try{
    const r=await fetch('/api/admin/tenants',{method:'POST',headers:hdr(),body:JSON.stringify({
      id:id,hosts:[domain],
      database_url:'postgresql://postiz-user:postiz-password@postiz-postgres:5432/postiz-'+id,
      redis_url:'redis://postiz-redis:6379',
      jwt_secret:crypto.getRandomValues(new Uint8Array(32)).reduce((s,b)=>s+b.toString(16).padStart(2,'0'),''),
      frontend_url:'https://'+domain,
      upload_dir:'/uploads/'+id,
      social:{},
      admin_email:'admin@'+domain.split('.').slice(-2).join('.'),
      admin_password:'Postiz2026!'
    })});
    const d=await r.json();
    if(r.ok){msg('Created '+d.id+' — point DNS to this server',true);$('domain').value='';loadTenants()}
    else msg(d.msg||'Failed','')
  }catch(e){msg('Network error: '+e.message,'')}
  btn.disabled=false;btn.textContent='Add';
}

async function del(id){
  if(!confirm('Remove "'+id+'"?'))return;
  await fetch('/api/admin/tenants/'+id,{method:'DELETE',headers:hdr()});
  loadTenants();
}

if($('key').value)loadTenants();
</script>
</body>
</html>`
