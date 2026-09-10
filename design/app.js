import {DEFAULTS, tokens, apply, toToml, contrast, separation, optimiseHues} from './theme.js';

/* ══ state ══════════════════════════════════════════════════════════════ */
const clone = o => JSON.parse(JSON.stringify(o));
let theme = clone(DEFAULTS);
let mode  = 'dark';
let applying = false;

const FACES = {
  display:['Fraunces','Instrument Serif','Bricolage Grotesque','Newsreader','IBM Plex Sans'],
  ui:['Inter Tight','Schibsted Grotesk','Manrope','IBM Plex Sans','Bricolage Grotesque'],
  mono:['JetBrains Mono','IBM Plex Mono','Martian Mono'],
};

/* ══ apply, and keep applying if anything else flips the theme ══════════ */
function render(){
  applying = true;
  apply(theme, mode, document.documentElement);
  document.documentElement.dataset.motion = theme.motion ? '1' : '0';
  applying = false;
  paintPalette();
  const ex = document.getElementById('export');
  if (ex) ex.value = toToml(theme, mode);
}
/* the audit harness sets data-theme directly rather than clicking, so the
   engine has to notice an outside change or every reading is of one theme */
new MutationObserver(muts=>{
  if (applying) return;
  for (const m of muts) if (m.attributeName === 'data-theme'){
    const t = document.documentElement.getAttribute('data-theme');
    if (t && t !== mode){ mode = t; syncThemeButtons(); render(); }
  }
}).observe(document.documentElement, {attributes:true, attributeFilter:['data-theme']});

function syncThemeButtons(){
  document.getElementById('t-dark').setAttribute('aria-pressed', String(mode==='dark'));
  document.getElementById('t-light').setAttribute('aria-pressed', String(mode==='light'));
}
function setMode(t){
  const go=()=>{ mode=t; syncThemeButtons(); render(); };
  document.startViewTransition ? document.startViewTransition(go) : go();
}
document.getElementById('t-dark').onclick=()=>setMode('dark');
document.getElementById('t-light').onclick=()=>setMode('light');

/* ══ the lab ════════════════════════════════════════════════════════════ */
const CONTROLS = [
  ['Faces', [
    {k:'faces.display', t:'select', label:'Display', opts:FACES.display},
    {k:'faces.ui',      t:'select', label:'UI',      opts:FACES.ui},
    {k:'faces.mono',    t:'select', label:'Mono',    opts:FACES.mono},
  ]],
  ['Type', [
    {k:'type.base',       label:'Base', min:13, max:20, step:.5, unit:'px'},
    {k:'type.scale',      label:'Scale', min:1.1, max:1.5, step:.01},
    {k:'type.lineHeight', label:'Line', min:1.3, max:1.8, step:.01},
    {k:'type.dispTight',  label:'Display tracking', min:-.05, max:.02, step:.002, unit:'em'},
    {k:'type.uiTight',    label:'UI tracking', min:-.03, max:.03, step:.002, unit:'em'},
  ]],
  ['Shape', [
    {k:'shape.radius',  label:'Radius',  min:0, max:24, step:1, unit:'px'},
    {k:'shape.density', label:'Density', min:.75, max:1.4, step:.05},
    {k:'shape.gut',     label:'Gutter',  min:1, max:3, step:.1, unit:'rem'},
  ]],
  ['Hues', [
    {k:'hues.rotate',      label:'Rotate',    min:0, max:360, step:1, unit:'°'},
    {k:'hues.dark.L',      label:'Dark L',    min:.55, max:.95, step:.005},
    {k:'hues.dark.C',      label:'Dark C',    min:.02, max:.18, step:.005},
    {k:'hues.light.L',     label:'Light L',   min:.25, max:.7,  step:.005},
    {k:'hues.light.C',     label:'Light C',   min:.02, max:.2,  step:.005},
  ]],
  ['Surfaces', [
    {k:'surfaces.hue',        label:'Neutral hue', min:0, max:360, step:1, unit:'°'},
    {k:'surfaces.chroma',     label:'Neutral C',   min:0, max:.06, step:.002},
    {k:'surfaces.dark.bg',    label:'Dark ground', min:.10, max:.35, step:.005},
    {k:'surfaces.dark.panel', label:'Dark panel',  min:.14, max:.45, step:.005},
    {k:'surfaces.dark.fg',    label:'Dark text',   min:.80, max:1,   step:.005},
    {k:'surfaces.light.bg',   label:'Light ground',min:.88, max:1,   step:.004},
    {k:'surfaces.light.glow', label:'Light glow',  min:.85, max:1,   step:.004},
  ]],
  ['Motion', [
    {k:'motion', t:'check', label:'Animate'},
  ]],
];
const get = (o,p) => p.split('.').reduce((a,k)=>a[k], o);
const set = (o,p,v) => { const ks=p.split('.'); const last=ks.pop();
  ks.reduce((a,k)=>a[k], o)[last] = v; };

function buildLab(){
  const body = document.getElementById('labbody');
  body.innerHTML = CONTROLS.map(([group, items]) => `
    <fieldset><legend>${group}</legend>
    ${items.map(c=>{
      const v = get(theme, c.k);
      if (c.t==='select') return `<div class="ctl"><label for="c-${c.k}">${c.label}</label>
        <select id="c-${c.k}" data-k="${c.k}" data-t="select">
        ${c.opts.map(o=>`<option ${o===v?'selected':''}>${o}</option>`).join('')}</select>
        <output></output></div>`;
      if (c.t==='check') return `<div class="ctl"><label for="c-${c.k}">${c.label}</label>
        <input type="checkbox" id="c-${c.k}" data-k="${c.k}" data-t="check" ${v?'checked':''}>
        <output></output></div>`;
      return `<div class="ctl"><label for="c-${c.k}">${c.label}</label>
        <input type="range" id="c-${c.k}" data-k="${c.k}" min="${c.min}" max="${c.max}"
          step="${c.step}" value="${v}"><output id="o-${c.k}">${v}${c.unit||''}</output></div>`;
    }).join('')}</fieldset>`).join('');

  body.addEventListener('input', e=>{
    const el = e.target, k = el.dataset.k; if(!k) return;
    const t = el.dataset.t;
    const v = t==='select' ? el.value : t==='check' ? el.checked : parseFloat(el.value);
    set(theme, k, v);
    const out = document.getElementById('o-'+k);
    if (out){
      const c = CONTROLS.flatMap(g=>g[1]).find(x=>x.k===k);
      out.textContent = (typeof v==='number' ? (Math.abs(v)<1&&v!==0 ? v.toFixed(3) : v) : v)
        + (c?.unit||'');
    }
    render();
  });
}
document.getElementById('t-lab').onclick=()=>document.getElementById('lab').classList.toggle('open');
document.getElementById('labclose').onclick=()=>document.getElementById('lab').classList.remove('open');
document.getElementById('copytoml').onclick=async e=>{
  await navigator.clipboard.writeText(document.getElementById('export').value).catch(()=>{});
  e.target.textContent='Copied'; setTimeout(()=>e.target.textContent='Copy',1400);
};
document.getElementById('reset').onclick=()=>{ theme=clone(DEFAULTS); buildLab(); render(); };
document.getElementById('optimise').onclick=e=>{
  const before=separation(theme.hues.members.map(m=>tokv('h-'+m.name))).worst;
  optimiseHues(theme, mode);
  render();
  const after=separation(theme.hues.members.map(m=>tokv('h-'+m.name))).worst;
  e.target.textContent=`ΔE ${before.toFixed(1)} → ${after.toFixed(1)}`;
  setTimeout(()=>e.target.textContent='Optimise separation', 2600);
};

/* ══ the projection diagram ═════════════════════════════════════════════ */
const SURFACES=['CLI','MCP','HTTP','window','tray','toast','palette','cron','URL'];
(function(){
  const g=document.getElementById('dg'), cx=220, cy=182, R=140;
  let s=`<circle class="ring" cx="${cx}" cy="${cy}" r="36"/>
         <circle class="ring" cx="${cx}" cy="${cy}" r="36" style="animation-delay:1.7s"/>`;
  SURFACES.forEach((n,i)=>{
    const a=(-90+i*(360/SURFACES.length))*Math.PI/180;
    const x=cx+Math.cos(a)*R, y=cy+Math.sin(a)*R;
    const d=`M${cx+Math.cos(a)*48} ${cy+Math.sin(a)*48} L${x} ${y}`;
    s+=`<path class="wire" d="${d}"/>
        <path class="flow" d="${d}" style="animation-delay:${(i*.31).toFixed(2)}s"/>
        <rect class="node" x="${x-28}" y="${y-11.5}" width="56" height="23" rx="7"/>
        <text x="${x}" y="${y+4}" text-anchor="middle">${n}</text>`;
  });
  s+=`<circle class="core" cx="${cx}" cy="${cy}" r="44"/>
      <text class="big" x="${cx}" y="${cy-1}" text-anchor="middle">rig</text>
      <text x="${cx}" y="${cy+15}" text-anchor="middle">registry</text>`;
  g.innerHTML=s;
})();

/* ══ the window ═════════════════════════════════════════════════════════ */
const APPS={
 shelf:{hue:'steel',ver:'1.2.0',a:'412 MB',b:'migrated 19',ms:12,title:'Index',cov:'coverage: partial',
  cols:['Collection','Items','Last run','Size','Health'],
  rows:[['library','4,182','2h ago','318 MB',96],['study','1,204','2h ago','61 MB',88],
        ['archive','17,900','6d ago','33 MB',41],['inbox','12','4m ago','2 MB',100]],
  acts:['Stats','Reindex'],
  log:[['14:22:03','shelf','index  scanned 1,204 files'],['14:22:03','graft','run    nightly-audit started'],
       ['14:22:04','shelf','index  done in 812ms']]},
 graft:{hue:'sage',ver:'0.4.1',a:'3 runs',b:'queue 1',ms:4,title:'Runs',cov:'coverage: partial',
  cols:['Assignment','State','Started','Frames','Progress'],
  rows:[['nightly-audit','running','18m ago','41,208',62],['spec-sweep','done','2h ago','9,110',100],
        ['tidy-logbook','failed','1d ago','812',37],['rehearse-drill','queued','-','0',4]],
  acts:['Timeline','Run'],
  log:[['14:19:41','graft','run    nightly-audit started'],['14:21:02','graft','frame  41,208 appended'],
       ['14:22:03','graft','tok    124.6k in, 8.1k out']]},
 dispatch:{hue:'indigo',ver:'2.0.0',a:'11 open',b:'2 overdue',ms:9,title:'Assignments',cov:'coverage: full',
  cols:['Assignment','Owner','Due','Age','Done'],
  rows:[['rig spec pass','me','today','4h',72],['grabbit revival','me','Fri','30d',12],
        ['shelf pilot','me','next week','2d',5],['romsort','me','-','9d',2]],
  acts:['Board','New'],
  log:[['14:20:55','dispatch','health 3 checks missed'],['14:21:10','dispatch','state  degraded'],
       ['14:22:00','dispatch','retry  in 8s']]},
 archi:{hue:'lilac',ver:'0.9.2',a:'237 elems',b:'4 views',ms:6,title:'Model',cov:'coverage: partial',
  cols:['View','Elements','Edges','Changed','Coverage'],
  rows:[['context','18','24','2h ago',100],['containers','61','88','2h ago',94],
        ['components','142','311','6d ago',63],['deployment','16','19','21d ago',28]],
  acts:['Canvas','Validate'],
  log:[['14:18:02','archi','model  upsert_element x12'],['14:18:04','archi','check  0 dangling edges'],
       ['14:22:01','archi','idle']]},
 snapper:{hue:'teal',ver:'0.3.0',a:'stopped',b:'-',ms:0,title:'Captures',cov:'coverage: partial',
  cols:['Capture','Kind','When','Size','Annotated'],
  rows:[['tray-now','region','4m ago','213 KB',3],['plan-diff','window','2h ago','1.1 MB',100],
        ['desktop','full','2h ago','2.4 MB',3],['rail-states','region','1d ago','88 KB',100]],
  acts:['Open folder','Capture'],
  log:[['12:45:02','snapper','exit   stopped by user'],['12:44:58','snapper','write  desktop.png'],
       ['12:44:51','snapper','start  region capture']]},
 nudge:{hue:null,ver:'1.0.0',a:'3 watches',b:'0 firing',ms:2,title:'Watches',cov:'coverage: full',
  cols:['Path','Trigger','Last fire','Fires','Health'],
  rows:[['~/me/inbox','any write','4m ago','128',100],['~/dl','new file','1h ago','44',100],
        ['~/me/study','md change','3d ago','9',82],['~/tmp','stale 7d','-','0',60]],
  acts:['Log','Add watch'],
  log:[['14:18:31','nudge','watch  ~/me/inbox fired'],['14:18:31','nudge','claude 1 assignment queued'],
       ['14:22:02','nudge','idle']]},
};
const ORDER=['shelf','graft','dispatch','archi','snapper','nudge'];
const LABEL={ok:'idle',degraded:'3 health fails',failing:'exit 1',stopped:'stopped'};
const pane=document.getElementById('pane'), rail=document.getElementById('rail'),
      notch=document.getElementById('notch');
function renderPane(k){
  const a=APPS[k];
  pane.innerHTML=`<div class="gp-h"><h4>${a.title}</h4><span class="chip hue">${a.cov}</span>
    <span class="acts">${a.acts.map((t,i)=>
      `<button class="btn${i===a.acts.length-1?' primary':''}">${t}</button>`).join('')}</span></div>
    <table class="tbl"><thead><tr>${a.cols.map(c=>`<th>${c}</th>`).join('')}</tr></thead><tbody>
    ${a.rows.map(r=>`<tr><td>${r[0]}</td><td class="m">${r[1]}</td><td class="m">${r[2]}</td>
      <td class="m">${r[3]}</td><td><span class="bar"><i style="width:${r[4]}%"></i></span></td></tr>`).join('')}
    </tbody></table><div class="tail">${a.log.map(l=>
      `<div><span class="ts">${l[0]}</span><span class="pg">${l[1]}</span><span>${l[2]}</span></div>`
    ).join('')}</div>`;
}
function select(k){
  const a=APPS[k];
  const go=()=>{
    rail.querySelectorAll('.mark').forEach(b=>b.setAttribute('aria-current',String(b.dataset.app===k)));
    // a program never owns an alarm hue, and a program with no identity hue
    // leaves the shell achromatic - which is what the shell is anyway
    document.documentElement.style.setProperty('--hue', a.hue?`var(--h-${a.hue})`:'var(--fg)');
    document.documentElement.style.setProperty('--ohue', a.hue?`var(--o-${a.hue})`:'var(--fg)');
    document.getElementById('ctx-name').textContent=k;
    document.getElementById('ctx-ver').textContent=a.ver;
    document.getElementById('ctx-a').textContent=a.a;
    document.getElementById('ctx-b').textContent=a.b;
    document.getElementById('strip-r').textContent=a.ms?`${k} ${a.ms} ms`:`${k} stopped`;
    renderPane(k);
    notch.style.translate=`0 ${36+ORDER.indexOf(k)*44}px`;
  };
  document.startViewTransition ? document.startViewTransition(go) : go();
}
rail.addEventListener('click',e=>{const b=e.target.closest('.mark'); if(b) select(b.dataset.app)});

/* ══ rail cases ═════════════════════════════════════════════════════════ */
const CASES=[
 {cap:'<b>All well.</b> No colour anywhere but the current program&rsquo;s notch.',
  m:[['sh','ok',1],['gr','ok',0],['ar','ok',0],['nu','ok',0]]},
 {cap:'<b>Degraded.</b> One amber pip, and nothing moves.',
  m:[['sh','ok',1],['gr','degraded',0],['ar','ok',0],['nu','ok',0]]},
 {cap:'<b>Failing, and one stopped.</b> The red pip breathes; a stopped program keeps its '+
      'place and goes dashed, so the rail never re-orders under your hand.',
  m:[['sh','ok',1],['gr','failing',0],['ar','stopped',0],['nu','ok',0]]}];
document.getElementById('railrow').innerHTML=CASES.map(c=>`<div>
  <div class="railcase"><div class="rail"><span class="brand">R</span>
  ${c.m.map(([t,s,cur])=>`<span class="mark" data-state="${s}" ${cur?'aria-current="true"':''}>
    <span class="ico">${t}</span>${s==='degraded'||s==='failing'?'<i class="pip"></i>':''}</span>`).join('')}
  <i class="notch" style="translate:0 ${36+c.m.findIndex(x=>x[2])*44}px"></i>
  </div></div><span class="cap">${c.cap}</span></div>`).join('');

/* ══ tray ═══════════════════════════════════════════════════════════════ */
function menu(st, banner){
  const rows=ORDER.map(k=>{const s=st[k]||'ok';
    return `<div class="tm-row"><i class="g ${s}"></i><span class="nm">${k}</span>
    <span class="st">${s==='ok'?(APPS[k].ms?APPS[k].ms+'ms':'idle'):LABEL[s]}</span></div>`}).join('');
  const bad=Object.values(st).filter(s=>s==='degraded'||s==='failing').length;
  return `<div class="tm-head"><span class="lg">R</span><span class="t">rig</span>
    <span class="s">${6-bad} mounted &middot; ${bad} degraded</span></div>
    ${banner?`<div class="tm-banner"><i class="pip"></i><span>${banner}</span></div>`:''}
    ${rows}<div class="tm-sep"></div><div class="tm-lbl">Run</div>
    <div class="tm-act">shelf reindex<span class="k">7d</span></div>
    <div class="tm-act">graft run nightly-audit</div>
    <div class="needsyou"><div class="tm-sep"></div><div class="tm-lbl">Needs you</div>
      <div class="tm-act">dispatch: restart budget spent<span class="k">quarantined</span></div>
      <div class="tm-act">Open the crash panel</div></div>
    <div class="tm-sep"></div>
    <div class="tm-act">Open the window<span class="k">Ctrl Space</span></div>
    <div class="tm-act">Notifications<span class="k">3</span></div>
    <div class="tm-act">Do not disturb</div>`;
}
document.getElementById('traypop').innerHTML=menu({snapper:'stopped'});
document.getElementById('trayscene').innerHTML=`
 <div><div class="traymenu">${menu({snapper:'stopped'})}</div>
  <span class="cap"><b>At rest.</b> Not one saturated pixel: six hollow ticks, a stopped program
  still holding its place, and no &ldquo;needs you&rdquo; section at all - the CSS hides it with
  <code>:has()</code> when no row is in trouble.</span></div>
 <div><div class="traymenu">${menu({graft:'degraded',dispatch:'failing',snapper:'stopped'},
  '<b>rigd is restarting</b> after an upgrade. Back in about 2 s; the window and this menu keep '+
  'working, and nothing was refused.')}</div>
  <span class="cap"><b>Something wants you.</b> A section appears that is not there at rest, and
  the lifecycle notice is drawn by the window process - which survives a daemon restart precisely
  so this banner can exist.</span></div>`;

/* ══ toasts ═════════════════════════════════════════════════════════════ */
const T={
 info:{src:'shelf',t:'Index rebuilt',b:'4,182 items in 812 ms. 3 files skipped as unreadable.',acts:['Show skipped','Open shelf']},
 success:{src:'graft',t:'nightly-audit finished',b:'18 minutes, 0 findings above medium. Report written.',acts:['Read report']},
 warn:{src:'rigd',t:'graft failed 3 health checks',b:'Degraded. Two more and the restart budget starts.',acts:['Logs','Restart now']},
 error:{src:'dispatch',t:'Exited 1 during migration 020',b:'Rolled back. The database is untouched and the last 200 log lines are kept.',acts:['Crash panel','Copy trace id']},
 progress:{src:'grabbit',t:'Downloading 3 of 8',b:'42.1 MB/s &middot; about 4 minutes left',bar:true,acts:[]}};
// severity asks for a MEANING, never for a colour
const TONE={info:'steel',success:'sage',warn:'amber',error:'rust',progress:'teal'};
function build(kind){
  const d=T[kind], el=document.createElement('div');
  const dwell=Math.min(20, 10+5*Math.ceil(d.b.length/46));
  el.className='toast '+kind;
  el.style.setProperty('--tone',`var(--h-${TONE[kind]})`);
  el.style.setProperty('--otone',`var(--o-${TONE[kind]})`);
  el.style.setProperty('--dwell', dwell+'s');
  el.innerHTML=`<div class="th"><span class="badge">${kind}</span><span class="src">${d.src}</span>
    <button class="x" aria-label="Dismiss">&times;</button></div>
    <p class="tt">${d.t}</p><p class="tb">${d.b}</p>
    ${d.bar?'<div class="prog"><i></i></div>':''}
    ${d.acts.length?`<div class="row">${d.acts.map(a=>`<button class="btn">${a}</button>`).join('')}</div>`:''}
    <i class="dwell"></i>`;
  return el;
}
const deck=document.getElementById('deck');
function dismiss(el){
  if (el.dataset.going) return;
  el.dataset.going='1';
  el.classList.add('leaving');
  el.addEventListener('animationend',()=>el.remove(),{once:true});
  setTimeout(()=>el.remove(), 400);   // belt and braces if the animation is off
}
function fire(kind){
  // newest first in the DOM, and the deck runs top-down, so an unread notice
  // is never underneath one you have already seen
  const el=build(kind); deck.prepend(el);
  while(deck.children.length>5) deck.lastElementChild.remove();
  const dw=el.querySelector('.dwell');
  el.addEventListener('mouseenter',()=>dw.style.animationPlayState='paused');
  el.addEventListener('mouseleave',()=>dw.style.animationPlayState='running');
  dw.addEventListener('animationend',()=>dismiss(el));
  el.querySelector('.x').addEventListener('click',()=>dismiss(el));
}
// Esc dismisses the top of the deck, the way every notification centre does
addEventListener('keydown',e=>{
  if(e.key==='Escape' && deck.firstElementChild && !document.getElementById('lab').classList.contains('open'))
    dismiss(deck.firstElementChild);
});
document.querySelectorAll('[data-fire]').forEach(b=>b.onclick=()=>fire(b.dataset.fire));
document.getElementById('fire-all').onclick=()=>
  ['info','success','warn','error','progress'].forEach((k,i)=>setTimeout(()=>fire(k),i*170));
['error','warn','progress','success','info'].forEach((k,i)=>setTimeout(()=>fire(k),250+i*110));
const st=build('success');
st.querySelector('.x').addEventListener('click',()=>dismiss(st));
document.getElementById('stagetoast').appendChild(st);

/* ══ palette, measured from the tokens actually painted ═════════════════ */
const USE={rust:'error · failing',amber:'warn · degraded',sage:'success · ok',
           teal:'progress · snapper',steel:'info · shelf',indigo:'dispatch',lilac:'archi'};
const GROUNDS=['bg','bg-2','panel','glow'];
const tokv=n=>getComputedStyle(document.documentElement).getPropertyValue('--'+n).trim();
function paintPalette(){
  const ms=theme.hues.members;
  document.getElementById('ring-n').textContent=ms.length;
  document.getElementById('swatches').innerHTML=ms.map(m=>`
    <div class="sw" style="--c:var(--h-${m.name})"><div class="band">${m.name}</div>
    <div class="meta"><span>${USE[m.name]||''}</span><b>${
      contrast(tokv('h-'+m.name), tokv('glow')).toFixed(2)}:1</b></div>
    <div class="meta"><span class="role">${m.role==='-'?'identity':m.role}${
      m.identity?'':' · reserved'}</span><span class="role">${m.angle}°</span></div></div>`).join('');

  const rows=[['fg',4.5],['fg-dim',4.5],['fg-faint',4.5],['border',3],['border-2',3],
    ...ms.map(m=>['h-'+m.name,4.5])];
  let worst=Infinity, fails=0;
  document.getElementById('ratiobody').innerHTML=rows.map(([n,need])=>{
    const cells=GROUNDS.map(g=>{
      const r=contrast(tokv(n), tokv(g)), ok=r>=need;
      if(!ok) fails++;
      worst=Math.min(worst, r/need*4.5);
      return `<td class="m ${ok?'pass':'bad'}">${r.toFixed(2)}${ok?'':' ✗'}</td>`;
    }).join('');
    return `<tr><td class="m">--${n}</td>${cells}<td class="m">${need.toFixed(1)}</td></tr>`;
  }).join('');

  // the second question: can two of them be told apart
  const hexes=ms.map(m=>tokv('h-'+m.name));
  const sep=separation(hexes);
  const w=worst.toFixed(2), sw=sep.worst.toFixed(1);
  document.getElementById('hero-ratio').textContent=w;
  document.getElementById('hero-sep').textContent=sw;
  document.getElementById('q-contrast').textContent=w+':1';
  document.getElementById('q-sep').textContent=sw;
  document.getElementById('q-pair').textContent=
    `${ms[sep.pair[0]].name} / ${ms[sep.pair[1]].name}`;

  const v=document.getElementById('verdict');
  v.classList.toggle('failing', fails>0);
  document.getElementById('v-worst').textContent=w;
  document.getElementById('v-sep').textContent=sw;
  document.getElementById('v-count').textContent=
    fails ? `${fails} cell${fails>1?'s':''} failing` : 'every token passes, both themes';
}

/* ══ go ═════════════════════════════════════════════════════════════════ */
buildLab();
render();
select('shelf');

/* ══ the notification centre ════════════════════════════════════════════ */
/* "Nothing is ever only a toast" is only true if the record is a different
   thing from the surface, so this list is the record and DND touches only
   the surface. A suppressed notification is tagged here, never dropped. */
let NOTES=[
 {pg:'graft',when:'14:31',unread:true,
  title:'nightly-audit is waiting on you',
  body:'Three files under <code>logbook/</code> changed on both sides. The run is parked, '
      +'not failed - it holds its lease until you answer.',
  acts:['Take mine','Take theirs','Open diff']},
 {pg:'dispatch',when:'14:22',unread:true,
  title:'dispatch went degraded',
  body:'Three health checks missed in a row. Restart 2 of 5 fires in 8s.',
  acts:['Restart now','Quarantine']},
 {pg:'archi',when:'13:58',unread:true,
  title:'the deployment view is 21 days stale',
  body:'16 elements, coverage 28%. Nothing has touched it since the last release.',
  acts:['Open view','Mute 30 days']},
 {pg:'shelf',when:'14:22',
  title:'index finished',
  body:'1,204 files in 812 ms. Nothing changed under <code>archive</code>.'},
 {pg:'snapper',when:'12:45',
  title:'snapper stopped',
  body:'Exit 0, stopped by you. It keeps its place in the rail rather than vanishing from it.',
  acts:['Start']},
 {pg:'nudge',when:'12:31',
  title:'~/me/inbox fired',
  body:'One assignment queued to graft.'},
 {pg:'shelf',when:'11:04',
  title:'secrets.get SHELF_TOKEN',
  body:'Granted, scoped to shelf. The value is <span class="nc-redact">never recorded</span> - '
      +'declared <code>sensitive</code>, so it never reached the log and cannot be searched here.'},
];
const NCPOOL=[
 {pg:'graft',title:'spec-sweep finished',
  body:'9,110 frames, 2 m 41 s. 124.6k tokens in, 8.1k out.'},
 {pg:'shelf',title:'reindex wants a confirmation',
  body:'<code>archive</code> is 17,900 items and 33 MB. This rewrites the whole segment.',
  acts:['Reindex','Not now']},
 {pg:'dispatch',title:'two assignments went overdue',
  body:'<code>grabbit revival</code> and <code>romsort</code>. Neither has moved in a week.',
  acts:['Open board']},
 {pg:'nudge',title:'~/dl saw a new file',
  body:'<code>chapter-04.pdf</code>, 2.1 MB. No rule matched, so nothing ran.'},
];
let ncPool=0, ncFilter='all', ncDnd=false;
const ncList=document.getElementById('nc-list'),
      ncQ=document.getElementById('nc-q'),
      ncCount=document.getElementById('nc-count'),
      ncHint=document.getElementById('nc-hint'),
      ncMore=document.getElementById('nc-more');

const ncClock=()=>{const d=new Date();
  return String(d.getHours()).padStart(2,'0')+':'+String(d.getMinutes()).padStart(2,'0')};

function ncNode(n){
  const el=document.createElement('div');
  el.className='nc-row'+(n.unread?'':' read');
  const hue=APPS[n.pg] && APPS[n.pg].hue;
  if(hue) el.style.setProperty('--pg',`var(--h-${hue})`);
  el.dataset.pg=n.pg;
  el.dataset.text=(n.pg+' '+n.title+' '+n.body).replace(/<[^>]+>/g,'').toLowerCase();
  el.innerHTML=
    `<div class="nc-top"><span class="nc-pg">${n.pg}</span>`
    +(n.held?'<span class="chip">suppressed</span>':'')
    +`<span class="nc-when">${n.when}</span></div>`
    +`<p class="nc-title">${n.title}</p><p class="nc-body">${n.body}</p>`;
  ncPaintActs(el,n);
  return el;
}
function ncPaintActs(el,n){
  el.querySelector('.nc-acts,.nc-done')?.remove();
  if(n.done){
    const d=document.createElement('div'); d.className='nc-done';
    d.innerHTML=`<span aria-hidden="true">&#10003;</span> answered &ldquo;${n.done.what}&rdquo;`
                +` &middot; ${n.done.at}`;
    el.append(d);
  }else if(n.acts && n.acts.length){
    const w=document.createElement('div'); w.className='nc-acts';
    w.innerHTML=n.acts.map((t,i)=>
      `<button class="btn${i===0?' primary':''}" data-act="${t}">${t}</button>`).join('');
    el.append(w);
  }
}
function ncSync(){
  const q=ncQ.value.trim().toLowerCase();
  ncList.querySelector('.nc-empty')?.remove();   // it carries no note; count it and ncSync throws
  let shown=0, unread=0, total=0;
  [...ncList.children].forEach(el=>{
    const n=el._n; total++;
    if(n.unread) unread++;
    const passF = ncFilter==='all' ? true
                : ncFilter==='unread' ? !!n.unread
                : ncFilter==='action' ? !!(n.acts && n.acts.length && !n.done)
                : !!n.held;
    const passQ = !q || el.dataset.text.includes(q);
    el.hidden = !(passF && passQ);
    if(!el.hidden) shown++;
  });
  if(!shown){
    const e=document.createElement('div'); e.className='nc-empty';
    e.textContent = q ? `Nothing matches "${ncQ.value.trim()}".`
                      : 'Nothing in this filter. The record is still complete.';
    ncList.append(e);
  }
  ncCount.textContent = (q||ncFilter!=='all')
    ? `${shown} of ${total}` : `${unread} unread · ${total} kept`;
  // the clipped last row is the scroll affordance every centre uses; saying so
  // is the difference between "there is more" and "this is broken"
  const over = ncList.scrollHeight - ncList.clientHeight > 4;
  ncMore.textContent = over ? 'scroll for the rest' : 'everything fits';
}
function ncAdd(n,front){
  const el=ncNode(n); el._n=n;
  front ? ncList.prepend(el) : ncList.append(el);
  ncSync();
}
NOTES.forEach(n=>ncAdd(n,false));

ncQ.addEventListener('input',ncSync);
document.getElementById('nc-filters').addEventListener('click',e=>{
  const b=e.target.closest('button[data-f]'); if(!b) return;
  ncFilter=b.dataset.f;
  e.currentTarget.querySelectorAll('button').forEach(x=>
    x.setAttribute('aria-pressed',String(x===b)));
  ncSync();
});
ncList.addEventListener('click',e=>{
  const b=e.target.closest('button[data-act]'); if(!b) return;
  const el=b.closest('.nc-row'), n=el._n;
  n.done={what:b.dataset.act,at:ncClock()};
  n.unread=false; el.classList.add('read');
  ncPaintActs(el,n); ncSync();
});
const dndBtn=document.getElementById('dnd');
dndBtn.addEventListener('click',()=>{
  ncDnd=!ncDnd;
  dndBtn.setAttribute('aria-pressed',String(ncDnd));
  ncHint.textContent = ncDnd
    ? 'Do Not Disturb is on. Nothing reaches the screen; the record still arrives, tagged.'
    : 'Search it, or answer a row and watch the action resolve in place.';
});
document.getElementById('nc-fire').addEventListener('click',()=>{
  const src=NCPOOL[ncPool++ % NCPOOL.length];
  const n={...src,when:ncClock(),unread:true,held:ncDnd,acts:src.acts?[...src.acts]:null};
  ncAdd(n,true);
  ncHint.textContent = ncDnd
    ? 'Suppressed: nothing reached the screen, and the row still arrived - tagged.'
    : 'That one also went up as a toast. The row is the copy that outlives it.';
});

/* ══ the crash panel ════════════════════════════════════════════════════ */
/* §18: exponential backoff inside a budget, and exhausting the budget is
   quarantine - a visible state with the full history and a manual restart,
   never a silent disappearance. The pane is the only thing that changes. */
const CRLOG=[
 ['14:21:58','migrate','020_assignment_owner: begin'],
 ['14:21:58','migrate','ALTER TABLE assignment ADD COLUMN owner TEXT'],
 ['14:21:59','migrate','backfill 11 rows from assignment_history'],
 ['14:22:00','sqlite ','constraint failed: assignment.owner NOT NULL',1],
 ['14:22:00','migrate','rollback 020 - the database is untouched',1],
 ['14:22:00','panic  ','migration 020 failed after rollback',1],
 ['14:22:00','exit   ','status 1, no signal'],
 ['14:22:00','rigd   ','dispatch exited after 4h 12m; 200 lines kept'],
];
const CRBASE=4, CRCAP=16, CRBUDGET=5;
const crPane=document.getElementById('crashpane'),
      crHint=document.getElementById('cr-hint'),
      crCtx=document.getElementById('cr-ctx'),
      crStrip=document.getElementById('cr-strip'),
      crStripR=document.getElementById('cr-strip-r');
let cr={attempt:2, left:CRBASE, span:CRBASE, state:'waiting'}, crTimer=null;

const crBackoff = a => Math.min(CRCAP, CRBASE * 2**(a-2));
function crFacts(){
  const q = cr.state==='quarantined';
  return [
    ['exit status','1','tone'],
    ['signal','none'],
    ['crashed at','14:22:00'],
    ['uptime','4h 12m'],
    ['restart', q ? `${CRBUDGET} of ${CRBUDGET} spent` : `${cr.attempt} of ${CRBUDGET}`, q?'tone':''],
    ['backoff', q ? 'budget exhausted' : `${CRBASE}s doubling, cap ${CRCAP}s`],
  ];
}
function crRender(){
  const q = cr.state==='quarantined';
  crPane.classList.toggle('done', q);
  const pct = cr.state==='waiting' ? Math.max(0, cr.left/cr.span*100) : 0;
  crPane.innerHTML =
    `<div class="crash-h"><h4>dispatch exited</h4>
      <span class="chip tone">${q?'quarantined':'exit 1'}</span>
      <span class="chip">migration 020</span>
      <span class="acts">
        <button class="btn" data-cr="trace">Copy trace id</button>
        <button class="btn" data-cr="history">Full history</button>
        <button class="btn primary" data-cr="${q?'start':'now'}">${q?'Start dispatch':'Restart now'}</button>
      </span></div>
     <div class="crash-facts">${crFacts().map(([k,v,t])=>
       `<div class="fact"><span>${k}</span><b class="${t||''}">${v}</b></div>`).join('')}</div>
     <div class="crash-log">${CRLOG.map(l=>
       `<div class="${l[3]?'hit':''}"><span class="ts">${l[0]}</span>`
       +`<span>${l[1]}</span><span>${l[2]}</span></div>`).join('')}</div>
     <div class="crash-foot"><div class="cd"><div class="lbl">
        <span>${q ? 'no further attempts - manual only'
                  : cr.state==='restarting' ? `restarting, attempt ${cr.attempt}`
                  : `attempt ${cr.attempt} of ${CRBUDGET} in ${cr.left}s`}</span>
        <span>${q ? 'quarantined 14:22:44' : `${cr.span}s backoff`}</span></div>
        <div class="track"><i style="width:${pct}%"></i></div></div>
        ${q ? '<span class="hint">It keeps its place in the rail, and every attempt is in '
             +'the history.</span>' : ''}</div>`;
  crCtx.textContent = q ? 'quarantined' : 'not running';
  crStrip.textContent = q ? '6 mounted · 1 quarantined' : '6 mounted · 1 failing';
  crStripR.textContent = q ? 'dispatch quarantined' : `dispatch restart ${cr.attempt}/${CRBUDGET}`;
}
function crStop(){ if(crTimer){ clearInterval(crTimer); crTimer=null; } }
function crQuarantine(){
  crStop(); cr.state='quarantined'; crRender();
  crHint.textContent='Budget spent after five attempts. Nothing disappeared: the program is '
    +'still listed, still selectable, and the reason is on the panel.';
}
function crTick(){
  if(cr.state!=='waiting') return;
  cr.left--;
  if(cr.left>0){ crRender(); return; }
  cr.state='restarting'; crRender();
  setTimeout(()=>{
    cr.attempt++;
    if(cr.attempt>CRBUDGET){ crQuarantine(); return; }
    cr.span=crBackoff(cr.attempt); cr.left=cr.span; cr.state='waiting'; crRender();
  }, 1100);
}
function crReset(){
  crStop(); cr={attempt:2,left:CRBASE,span:CRBASE,state:'waiting'}; crRender();
  crHint.textContent='The countdown is real. Watch the backoff double on every attempt until '
    +'the budget runs out.';
}
document.getElementById('cr-run').onclick=()=>{
  if(cr.state==='quarantined') crReset();
  crStop(); crTimer=setInterval(crTick,1000);
  crHint.textContent='Running. Each failed attempt doubles the wait, capped at '+CRCAP+'s.';
};
document.getElementById('cr-skip').onclick=crQuarantine;
document.getElementById('cr-reset').onclick=crReset;
crPane.addEventListener('click',e=>{
  const b=e.target.closest('button[data-cr]'); if(!b) return;
  const k=b.dataset.cr;
  if(k==='now'){ cr.left=1; crTick(); crStop(); crTimer=setInterval(crTick,1000); }
  else if(k==='start'){ crReset(); crHint.textContent=
    'Started by hand. The budget resets with it, and the quarantine is in the history.'; }
  else if(k==='trace'){ b.textContent='copied 8f3c1a9e'; setTimeout(()=>b.textContent='Copy trace id',1400); }
  else { crHint.textContent='Every attempt, its exit status and its log are in the history '
    +'(§15) - the panel is a view over it, not the only copy.'; }
});
crReset();

/* ══ the operator view ══════════════════════════════════════════════════ */
/* §15. The columns are the plan's columns. The interesting row is the one
   that is WAITING: on a machine running several agents, contention is the
   thing you open this view to find. */
const CLIENTS=[
 {id:'claude', kind:'agent', who:'claude', sub:'pid 41182 · claude --resume · uid 1000',
  since:'14:02', active:'3s ago', wire:'1.4 · stub 0.9.1',
  hold:'2 leases · crew rig-spec · 4 subs', wait:null,
  doing:'shelf.search 41 ms', rate:'3.1/s · 812 KB · 0 err',
  calls:[
   ['14:33:02','shelf.search','{q:"oklch"}','41 ms','12 hits'],
   ['14:32:58','peers.lease.acquire','{name:"deploy",ttl:"5m"}','3 ms','fenced 118'],
   ['14:31:40','secrets.get','{key:"SHELF_TOKEN"}','2 ms','REDACT'],
   ['14:31:12','dispatch.list','{state:"open"}','9 ms','11 rows'],
   ['14:30:04','graft.run','{assignment:"nightly-audit"}','6 ms','queued'],
  ]},
 {id:'tui', kind:'terminal', who:'rig tui', sub:'pid 39004 · rig tui · uid 1000',
  since:'09:14', active:'2m ago', wire:'1.4 · stub 0.9.1',
  hold:'1 sub', wait:null, doing:null, rate:'0.2/s · 9 KB · 0 err',
  calls:[
   ['14:31:08','programs.list','{}','1 ms','6 rows'],
   ['14:28:44','history.query','{since:"1h",limit:200}','14 ms','1,204 rows'],
  ]},
 {id:'window', kind:'window', who:'rig window', sub:'pid 38210 · rigd-window · uid 1000',
  since:'09:14', active:'now', wire:'1.4 · stub 0.9.1',
  hold:'9 subs', wait:null, doing:'dispatch.list 8 ms', rate:'1.4/s · 210 KB · 0 err',
  calls:[
   ['14:33:04','dispatch.list','{state:"open"}','8 ms','11 rows'],
   ['14:33:01','programs.health','{}','2 ms','1 degraded'],
  ]},
 {id:'deploy', kind:'script', who:'make deploy', sub:'pid 41990 · /bin/sh -c make deploy',
  since:'14:31', active:'now', wire:'1.4 · stub 0.9.1',
  hold:null, wait:'lease deploy, held by claude · 4m 12s',
  doing:null, rate:'0.1/s · 2 KB · 0 err',
  calls:[
   ['14:31:52','peers.lease.acquire','{name:"deploy",wait:"10m"}','blocked','waiting'],
   ['14:31:52','peers.holder','{name:"deploy"}','2 ms','claude, 4m 12s'],
  ]},
 {id:'shelf', kind:'program', who:'shelf', sub:'pid 38455 · shelf serve --rig',
  since:'09:14', active:'12s ago', wire:'1.4 · stub 0.9.1',
  hold:'grants: storage, filesystem', wait:null, doing:null,
  rate:'0.9/s · 44 KB · 3 denied',
  calls:[
   ['14:32:11','notify.post','{title:"index finished"}','1 ms','delivered'],
   ['14:29:03','network.dial','{host:"api.crates.io"}','0 ms','DENIED capability'],
  ]},
];
const opBody=document.getElementById('opbody'), opDetail=document.getElementById('opdetail');
let opSel='claude', opLooked=new Set();

const opCell = v => v ? v : '<span class="op-none">-</span>';
function opRows(){
  opBody.innerHTML=CLIENTS.map(c=>
   `<tr data-id="${c.id}" aria-selected="${c.id===opSel}" tabindex="0">
     <td><div class="op-who"><span class="op-kind">${c.kind}</span><b>${c.who}</b>
       <span>${c.sub}</span></div></td>
     <td class="m">${c.since}<br>${c.active}</td>
     <td class="m">${opCell(c.hold)}</td>
     <td class="m">${c.wait?`<span class="op-wait">${c.wait}</span>`:opCell(null)}</td>
     <td class="m">${opCell(c.doing)}</td>
     <td class="m">${c.rate}</td></tr>`).join('');
}
function opRenderDetail(){
  const c=CLIENTS.find(x=>x.id===opSel);
  const looked=opLooked.has(c.id);
  opDetail.innerHTML=
   `<h5>${c.who} &mdash; every call it made</h5>
    <p class="sub">Connected ${c.since}, wire ${c.wire}. Thirty days of history, then it ages
    out. A field a command declared <code>sensitive</code> is not hidden here - it was never
    written.</p>
    <div class="op-calls">${c.calls.map(k=>
      `<div><span class="ts">${k[0]}</span><span class="mth">${k[1]}</span>`
      +`<span class="arg">${k[2].replace(/</g,'&lt;')}</span><span class="ms">${k[3]}</span>`
      +`<span class="${k[4]==='REDACT'?'':'res'}">`
      +`${k[4]==='REDACT'?'<span class="nc-redact">never recorded</span>':k[4]}</span></div>`
     ).join('')}</div>
    ${looked?`<p class="op-audit"><b>Written to the audit log:</b> you opened
      ${c.who}'s history. Looking is an event, so the record of who read what is the same
      record as who did what.</p>`:''}`;
}
function opSelect(id){
  if(id!==opSel) opLooked.add(id);
  opSel=id; opRows(); opRenderDetail();
}
opBody.addEventListener('click',e=>{
  const tr=e.target.closest('tr[data-id]'); if(tr) opSelect(tr.dataset.id);
});
opBody.addEventListener('keydown',e=>{
  const tr=e.target.closest('tr[data-id]'); if(!tr) return;
  if(e.key==='Enter'||e.key===' '){ e.preventDefault(); opSelect(tr.dataset.id); }
});
opRows(); opRenderDetail();

/* ══ generated panes ════════════════════════════════════════════════════ */
/* §11's second pane tier, minus the two §01 already shows. The point is the
   pairing: the JSON on the left is the ONLY thing the program wrote, and the
   CLI line under it is built from that same object, not from a second spec. */
const gpCode=document.getElementById('gp-code'), gpLive=document.getElementById('gp-live'),
      gpCli=document.getElementById('gp-cli'), gpOwner=document.getElementById('gp-owner'),
      gpLines=document.getElementById('gp-lines');
let gpKind='form', gpTimer=null;

// a small JSON pretty-printer, coloured the way §12's toasts colour code
function gpJson(v, ind){
  const pad='  '.repeat(ind), pad1='  '.repeat(ind+1);
  const P=t=>`<span class="p">${t}</span>`;
  if(v===null) return '<span class="n">null</span>';
  if(typeof v==='boolean'||typeof v==='number') return `<span class="n">${v}</span>`;
  if(typeof v==='string') return `<span class="s">"${v.replace(/</g,'&lt;')}"</span>`;
  if(Array.isArray(v)){
    if(!v.length) return P('[]');
    const flat=v.every(x=>typeof x!=='object'||x===null);
    if(flat) return P('[')+v.map(x=>gpJson(x,0)).join(P(', '))+P(']');
    return P('[')+'\n'+v.map(x=>pad1+gpJson(x,ind+1)).join(P(',')+'\n')+'\n'+pad+P(']');
  }
  const ks=Object.keys(v);
  if(!ks.length) return P('{}');
  return P('{')+'\n'+ks.map(k=>
    `${pad1}<span class="k">"${k}"</span>${P(': ')}${gpJson(v[k],ind+1)}`
  ).join(P(',')+'\n')+'\n'+pad+P('}');
}

const GP={
 form:{owner:'shelf', decl:{
   command:'shelf.reindex', summary:'Rebuild the index for one collection',
   idempotent:true, destructive:false, sensitive:[],
   args:{
     collection:{type:'enum', of:['library','study','archive','inbox'], required:true,
                 help:'Which collection to rebuild'},
     full:{type:'bool', default:false, help:'Rewrite the whole segment, not just the delta'},
     since:{type:'date', default:null, help:'Only files touched after this'},
     workers:{type:'int', min:1, max:8, default:4, help:'Parallel readers'}}}},
 progress:{owner:'shelf', decl:{
   command:'shelf.reindex', progress:{
     kind:'determinate', unit:'files', total:'{{total}}', done:'{{done}}',
     rate:true, eta:true, cancellable:true,
     steps:['scan','read','embed','write','verify']}}},
 detail:{owner:'dispatch', decl:{
   view:'assignment', key:'id', title:'{{title}}',
   fields:[{name:'state', type:'enum', of:['open','blocked','done']},
           {name:'owner', type:'string'},
           {name:'due', type:'date'},
           {name:'age', type:'duration'},
           {name:'lease', type:'string', mono:true},
           {name:'notes', type:'markdown'}]}},
 status:{owner:'graft', decl:{
   status:{state:'{{state}}', since:'{{since}}',
           health:{interval:'10s', timeout:'2s', failures:'{{fails}}'},
           counters:['runs','queued','frames','tokens'],
           last_error:'{{last_error}}'}}},
 table:{owner:'shelf', decl:{
   view:'collections', rowKey:'name',
   columns:[{name:'Collection', field:'name'},
            {name:'Items', field:'items', align:'end'},
            {name:'Last run', field:'ran', type:'relative'},
            {name:'Size', field:'bytes', type:'bytes'},
            {name:'Health', field:'health', type:'percent', render:'bar'}]}},
 actions:{owner:'shelf', decl:{
   actions:[{command:'shelf.stats', label:'Stats', idempotent:true},
            {command:'shelf.reindex', label:'Reindex', idempotent:true, primary:true},
            {command:'shelf.forget', label:'Forget collection', destructive:true,
             confirm:'Type the collection name'}]}}};

/* ── the six renderers ─────────────────────────────────────────────────── */
let gpForm={collection:'archive', full:true, since:'', workers:4};
function gpRenderForm(){
  const a=GP.form.decl.args;
  return `<div class="gform">
    <div class="gfield"><label for="gf-c">collection</label>
      <select id="gf-c">${a.collection.of.map(o=>
        `<option${o===gpForm.collection?' selected':''}>${o}</option>`).join('')}</select>
      <span class="help">${a.collection.help}</span></div>
    <div class="gfield"><label for="gf-s">since</label>
      <input id="gf-s" type="text" inputmode="numeric" placeholder="2026-08-01"
             value="${gpForm.since}"><span class="help">${a.since.help}</span></div>
    <div class="gfield"><label for="gf-w">workers</label>
      <div class="grange"><input id="gf-w" type="range" min="${a.workers.min}"
        max="${a.workers.max}" value="${gpForm.workers}"
        aria-label="workers"><output>${gpForm.workers}</output></div>
      <span class="help">${a.workers.help}</span></div>
    <div class="gfield"><label for="gf-f">full</label>
      <div class="gcheck"><input id="gf-f" type="checkbox"${gpForm.full?' checked':''}>
        <label for="gf-f" class="help">${a.full.help}</label></div></div>
  </div>
  <div class="gform-foot"><button class="btn primary" id="gf-run">Reindex</button>
    <span class="hint">Declared <code>idempotent</code>, so a retry is safe and the terminal
    says so in <code>--help</code>.</span></div>`;
}
let gpDone=1204, gpTotal=4182;
function gpRenderProgress(){
  const pct=Math.round(gpDone/gpTotal*100), step=Math.min(4, Math.floor(pct/22));
  const names=GP.progress.decl.progress.steps;
  return `<div class="gprog">
    <div class="top"><b>Reindexing archive</b><span>${pct}%</span></div>
    <span class="bar" style="width:100%"><i style="width:${pct}%"></i></span>
    <div class="sub"><span>${gpDone.toLocaleString()} of ${gpTotal.toLocaleString()} files</span>
      <span>1,480/s &middot; about ${Math.max(1,Math.round((gpTotal-gpDone)/1480))}s left</span></div>
  </div>
  <div class="gsteps">${names.map((n,i)=>
    `<span class="${i<step?'go':i===step?'on':''}">${i<step?'&#10003;':i===step?'&rarr;':'&middot;'} ${n}</span>`
   ).join('')}</div>
  <div class="gform-foot"><button class="btn" id="gp-cancel">Cancel</button>
    <span class="hint">Cancellable because the declaration says so. A command that cannot be
    cancelled does not get a button that lies about it.</span></div>`;
}
function gpRenderDetail(){
  const rows=[['state','<span class="chip hue">open</span>'],['owner','me'],
   ['due','today &middot; 4h left'],['age','4h 12m'],
   ['lease','deploy &middot; fenced 118 &middot; 5m ttl'],
   ['notes','Waiting on the §24 gate date. Nothing else blocks it.']];
  return `<div class="gp-h"><h4>rig spec pass</h4><span class="chip">assignment</span>
    <span class="acts"><button class="btn">Open</button>
    <button class="btn primary">Mark done</button></span></div>
   <dl class="gdl">${rows.map(([k,v])=>`<dt>${k}</dt><dd>${v}</dd>`).join('')}</dl>`;
}
function gpRenderStatus(){
  const cells=[['state','running','sage'],['since','09:14 &middot; 5h 19m',''],
   ['health','10s / 2s &middot; 0 fails',''],['runs','3',''],['queued','1',''],
   ['frames','41,208',''],['tokens','124.6k in &middot; 8.1k out',''],
   ['last error','none',''],];
  return `<div class="gp-h"><h4>graft</h4><span class="chip hue">coverage: partial</span>
    <span class="acts"><button class="btn">Logs</button>
    <button class="btn">Restart</button></span></div>
   <div class="gstat">${cells.map(([k,v,t])=>
     `<div class="fact"><span>${k}</span><b${t?` style="color:var(--h-${t})"`:''}>${v}</b></div>`
    ).join('')}</div>
   <span class="cap" style="margin:0">Healthy is the absence of colour, so only the one word
   that is good is coloured, and nothing here moves at rest.</span>`;
}
function gpRenderTable(){
  const a=APPS.shelf;
  return `<div class="gp-h"><h4>${a.title}</h4><span class="chip hue">${a.cov}</span></div>
   <table class="tbl"><thead><tr>${a.cols.map(c=>`<th>${c}</th>`).join('')}</tr></thead><tbody>
   ${a.rows.map(r=>`<tr><td>${r[0]}</td><td class="m">${r[1]}</td><td class="m">${r[2]}</td>
     <td class="m">${r[3]}</td><td><span class="bar"><i style="width:${r[4]}%"></i></span></td></tr>`
    ).join('')}</tbody></table>`;
}
function gpRenderActions(){
  return `<div class="gp-h"><h4>Actions</h4><span class="chip">3 declared</span></div>
   <div class="gform-foot" style="margin-top:0">
     <button class="btn">Stats</button>
     <button class="btn primary">Reindex</button>
     <button class="btn" id="gp-destroy">Forget collection</button></div>
   <p class="cap" style="margin-top:.9rem"><b>Destructive is a property, not a convention.</b>
   The third command declared <code>destructive</code> with a confirmation, so every surface
   asks - the window with a typed name, <code>rig run</code> with a prompt it will not skip
   without <code>--yes</code>, and MCP by refusing outright unless the agent was granted it.</p>
   <p class="cap" id="gp-destroy-out" style="margin-top:.5rem"></p>`;
}
const GPR={form:gpRenderForm, progress:gpRenderProgress, detail:gpRenderDetail,
           status:gpRenderStatus, table:gpRenderTable, actions:gpRenderActions};

function gpCliLine(){
  if(gpKind==='form'){
    const f=[`--collection=${gpForm.collection}`];
    if(gpForm.full) f.push('--full');
    if(gpForm.since.trim()) f.push(`--since=${gpForm.since.trim()}`);
    if(gpForm.workers!==4) f.push(`--workers=${gpForm.workers}`);
    return `rig run shelf.reindex ${f.join(' ')}`;
  }
  return {progress:'rig run shelf.reindex --collection=archive --watch',
          detail:'rig show dispatch assignment rig-spec-pass',
          status:'rig status graft',
          table:'rig list shelf collections',
          actions:'rig commands shelf'}[gpKind];
}
function gpStopTimer(){ if(gpTimer){ clearInterval(gpTimer); gpTimer=null; } }
function gpPaint(){
  gpOwner.textContent=GP[gpKind].owner;
  // one element per logical line, so a wrapped line can hang under its own indent
  gpCode.innerHTML=gpJson(GP[gpKind].decl,0).split('\n')
    .map(l=>`<span class="ln">${l||'&nbsp;'}</span>`).join('');
  gpLines.textContent=gpCode.querySelectorAll('.ln').length+' lines';
  gpLive.innerHTML=GPR[gpKind]();
  gpCli.textContent=gpCliLine();
  if(gpKind==='form') gpWireForm();
  if(gpKind==='progress') gpWireProgress();
  if(gpKind==='actions') gpWireActions();
}
function gpWireForm(){
  const c=document.getElementById('gf-c'), s=document.getElementById('gf-s'),
        w=document.getElementById('gf-w'), f=document.getElementById('gf-f');
  const upd=()=>{ gpForm={collection:c.value, since:s.value, workers:+w.value, full:f.checked};
                  w.nextElementSibling.textContent=w.value; gpCli.textContent=gpCliLine(); };
  [c,s,w,f].forEach(el=>el.addEventListener('input',upd));
  document.getElementById('gf-run').onclick=()=>{
    gpKind='progress'; gpDone=0;
    document.querySelectorAll('#gp-tabs [data-gp]').forEach(b=>
      b.setAttribute('aria-pressed',String(b.dataset.gp==='progress')));
    gpPaint();
  };
}
function gpWireProgress(){
  gpStopTimer();
  gpTimer=setInterval(()=>{
    if(gpKind!=='progress'){ gpStopTimer(); return; }
    gpDone=Math.min(gpTotal, gpDone+Math.round(gpTotal*0.06));
    gpLive.innerHTML=gpRenderProgress(); gpWireCancelOnly();
    if(gpDone>=gpTotal) gpStopTimer();
  },700);
  gpWireCancelOnly();
}
function gpWireCancelOnly(){
  const b=document.getElementById('gp-cancel');
  if(b) b.onclick=()=>{ gpStopTimer();
    gpLive.querySelector('.gprog .top b').textContent='Reindex cancelled';
    b.disabled=true; b.textContent='Cancelled'; };
}
function gpWireActions(){
  document.getElementById('gp-destroy').onclick=()=>{
    document.getElementById('gp-destroy-out').innerHTML=
      '<b>Confirmation required.</b> Type <code>archive</code> to forget it. The same command '
     +'over MCP is refused outright unless the agent holds the grant.';
  };
}
document.getElementById('gp-tabs').addEventListener('click',e=>{
  const b=e.target.closest('[data-gp]'); if(!b) return;
  gpStopTimer(); gpKind=b.dataset.gp;
  e.currentTarget.querySelectorAll('[data-gp]').forEach(x=>
    x.setAttribute('aria-pressed',String(x===b)));
  gpPaint();
});
gpPaint();
