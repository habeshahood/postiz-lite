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
<span class="suffix">.postiz.1h00d.com</span>
<button id="addBtn" onclick="add()">Add</button>
</div>

<div class="section">Sites</div>
<div id="list"><div class="empty">Connect with admin key</div></div>
</div>

<script>
const $=id=>document.getElementById(id);
const key=()=>$('key').value;
const hdr=()=>({Authorization:'Bearer '+key(),'Content-Type':'application/json'});

$('key').value=localStorage.getItem('pk')||'';
$('name').addEventListener('keydown',e=>{if(e.key==='Enter'){e.preventDefault();add()}});

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
      return '<div class="tenant"><div><div class="t-name">'+esc(t.id)+'</div><div class="t-url"><a href="'+esc(url)+'" target="_blank">'+esc(host)+'</a></div></div><button class="del" onclick="del(\''+esc(t.id)+'\')">&times;</button></div>';
    }).join('');
  }catch(e){$('list').innerHTML='<div class="empty">Error</div>'}
}

async function add(){
  const name=$('name').value.trim().toLowerCase().replace(/[^a-z0-9-]/g,'');
  if(!name){$('name').focus();return}
  const domain=name+'.postiz.1h00d.com';
  const btn=$('addBtn');btn.disabled=true;btn.textContent='...';
  try{
    const r=await fetch('/api/admin/tenants',{method:'POST',headers:hdr(),body:JSON.stringify({
      id:name,hosts:[domain],
      database_url:'postgresql://postiz-user:postiz-password@postiz-postgres:5432/postiz-'+name,
      redis_url:'redis://postiz-redis:6379',
      jwt_secret:[...crypto.getRandomValues(new Uint8Array(32))].map(b=>b.toString(16).padStart(2,'0')).join(''),
      frontend_url:'https://'+domain,
      upload_dir:'/uploads/'+name,
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
