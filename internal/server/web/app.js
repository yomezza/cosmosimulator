'use strict';
const $ = id => document.getElementById(id);
let mode = 'login', signedIn = false, pending = false;
async function api(path, body) {
 const response = await fetch('/api/' + path, {method:body === undefined ? 'GET':'POST',headers:body === undefined?{}:{'Content-Type':'application/json'},body:body === undefined?undefined:JSON.stringify(body)});
 const data = await response.json();
 if (!response.ok) {const error = new Error(data.error || 'Ошибка сервера');error.status=response.status;throw error;}
 return data;
}
function authMode(value) {
 mode=value;$('login-tab').classList.toggle('active',mode==='login');$('register-tab').classList.toggle('active',mode==='register');
 $('submit').textContent=mode==='login'?'Войти в терминал →':'Создать компанию →';
 const input=$('auth-form').elements.password;input.autocomplete=mode==='login'?'current-password':'new-password';input.minLength=mode==='login'?1:12;
 $('password-hint').textContent=mode==='login'?'Используйте данные вашей компании.':'Пароль: от 12 до 72 байт. Имя: латиница, цифры, _ и -.';$('auth-error').textContent='';
}
function showAuth(){signedIn=false;$('auth').hidden=false;$('game').hidden=true;}
function cell(row,value){const td=document.createElement('td');td.textContent=value;row.append(td);}
async function refresh(){
 if(pending)return;pending=true;
 try{const s=await api('state');signedIn=true;$('auth').hidden=true;$('game').hidden=false;$('company-name').textContent=s.user.username;$('role').textContent=s.user.role==='admin'?'АДМИНИСТРАТОР':'ИГРОК';$('admin').hidden=s.user.role!=='admin';
 for(const key of ['credits','ore'])$(key).textContent=s.user[key].toLocaleString('ru-RU');$('tick').textContent=String(s.tick);$('sell').disabled=s.user.ore===0;
 $('companies').replaceChildren();for(const company of s.companies){const tr=document.createElement('tr');cell(tr,company.username);cell(tr,company.bot?'АВТОНОМНАЯ':'ИГРОК');cell(tr,company.credits.toLocaleString('ru-RU'));cell(tr,company.ore.toLocaleString('ru-RU'));$('companies').append(tr);}
 $('connection').textContent='Данные обновлены: '+new Date().toLocaleTimeString('ru-RU')+' · Мир развивается, пока работает сервер.';
 }catch(e){if(e.status===401)showAuth();else if(signedIn)$('connection').textContent=e.message;else $('auth-error').textContent='Нет соединения с сервером: '+e.message;}finally{pending=false;}
}
$('login-tab').onclick=()=>authMode('login');$('register-tab').onclick=()=>authMode('register');
$('auth-form').onsubmit=async event=>{event.preventDefault();$('submit').disabled=true;$('auth-error').textContent='';try{const f=event.target;await api(mode,{username:f.elements.username.value,password:f.elements.password.value});f.elements.password.value='';await refresh();}catch(e){$('auth-error').textContent=e.message;}finally{$('submit').disabled=false;}};
$('logout').onclick=async()=>{try{await api('logout',{});showAuth();$('admin-users').replaceChildren();}catch(e){$('connection').textContent=e.message;}};
$('sell').onclick=async()=>{$('sell').disabled=true;try{await api('sell',{});$('game-message').textContent='Поставка выполнена. Кредиты зачислены.';await refresh();}catch(e){$('game-message').textContent=e.message;}finally{if(signedIn)await refresh();}};
$('refresh').onclick=refresh;
$('load-users').onclick=async()=>{try{const users=await api('admin/users');$('admin-users').replaceChildren();for(const u of users){const p=document.createElement('p');p.textContent=`#${u.id} · ${u.username} · ${u.bot?'bot':u.role}`;$('admin-users').append(p);}}catch(e){$('admin-users').textContent=e.message;}};
authMode('login');refresh();setInterval(()=>{if(signedIn)refresh();},5000);
